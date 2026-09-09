package slack

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/buildkite/roko"
)

const (
	slackAPIBase        = "https://slack.com/api"
	maxRateLimitRetries = 3
)

type Client struct {
	userToken  string
	httpClient *http.Client
	sleep      func(time.Duration)
}

type APIError struct {
	Method string
	Code   string
}

func (e *APIError) Error() string {
	return "slack API error: " + e.Code
}

func IsAPIError(err error, code string) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Code == code
}

func NewClient(userToken string) *Client {
	return &Client{
		userToken: userToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func NewClientWithHTTPClient(userToken string, httpClient *http.Client) *Client {
	client := NewClient(userToken)
	if httpClient != nil {
		client.httpClient = httpClient
	}
	return client
}

func (c *Client) transferHTTPClient() *http.Client {
	if c.httpClient == nil {
		return &http.Client{}
	}

	clientCopy := *c.httpClient
	clientCopy.Timeout = 0
	return &clientCopy
}

func (c *Client) request(method string, params url.Values) ([]byte, error) {
	return c.requestWithMethod(http.MethodGet, method, params)
}

func (c *Client) requestPost(method string, params url.Values) ([]byte, error) {
	return c.requestWithMethod(http.MethodPost, method, params)
}

func (c *Client) requestWithMethod(httpMethod, method string, params url.Values) ([]byte, error) {
	requestURL := slackAPIBase + "/" + method
	if params == nil {
		params = url.Values{}
	}

	if httpMethod == http.MethodGet && len(params) > 0 {
		requestURL += "?" + params.Encode()
	}

	retrier := roko.NewRetrier(
		roko.WithMaxAttempts(maxRateLimitRetries+1),
		roko.WithStrategy(roko.Constant(0)),
		roko.WithSleepFunc(c.sleep),
	)
	var resp *http.Response
	err := retrier.Do(func(r *roko.Retrier) error {
		var bodyReader io.Reader
		if httpMethod == http.MethodPost {
			bodyReader = strings.NewReader(params.Encode())
		}

		req, err := http.NewRequest(httpMethod, requestURL, bodyReader)
		if err != nil {
			r.Break()
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+c.userToken)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err = c.httpClient.Do(req)
		if err != nil {
			r.Break()
			return fmt.Errorf("failed to send request: %w", err)
		}

		if resp.StatusCode != http.StatusTooManyRequests {
			return nil
		}
		retryAfter, retryErr := parseRetryAfter(resp.Header.Get("Retry-After"))
		if err := resp.Body.Close(); err != nil {
			r.Break()
			return fmt.Errorf("failed to close rate-limited response: %w", err)
		}
		if retryErr != nil {
			r.Break()
			return fmt.Errorf("slack API returned HTTP 429 without a valid Retry-After header: %w", retryErr)
		}
		r.SetNextInterval(retryAfter)
		return fmt.Errorf("slack API remained rate limited after %d retries", maxRateLimitRetries)
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("slack API returned HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var slackResp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &slackResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !slackResp.OK {
		return nil, &APIError{Method: method, Code: slackResp.Error}
	}

	return body, nil
}

func parseRetryAfter(value string) (time.Duration, error) {
	seconds, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || seconds < 0 {
		return 0, fmt.Errorf("invalid duration %q", value)
	}
	return time.Duration(seconds) * time.Second, nil
}

func (c *Client) DownloadPrivateFile(fileURL string, maxBytes int) ([]byte, string, error) {
	req, err := http.NewRequest("GET", fileURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}

	if isSlackHostedURL(fileURL) {
		req.Header.Set("Authorization", "Bearer "+c.userToken)
	}

	resp, err := c.transferHTTPClient().Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("slack file download returned HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	if maxBytes <= 0 {
		return nil, "", fmt.Errorf("maxBytes must be > 0")
	}

	limitedReader := io.LimitReader(resp.Body, int64(maxBytes)+1)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read response: %w", err)
	}
	if len(body) > maxBytes {
		return nil, "", fmt.Errorf("download exceeds limit (%d bytes)", maxBytes)
	}

	return body, resp.Header.Get("Content-Type"), nil
}

func (c *Client) DownloadPrivateFileToWriter(fileURL string, writer io.Writer) (string, int64, error) {
	req, err := http.NewRequest("GET", fileURL, nil)
	if err != nil {
		return "", 0, fmt.Errorf("failed to create request: %w", err)
	}

	if isSlackHostedURL(fileURL) {
		req.Header.Set("Authorization", "Bearer "+c.userToken)
	}

	resp, err := c.transferHTTPClient().Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", 0, fmt.Errorf("slack file download returned HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	written, err := io.Copy(writer, resp.Body)
	if err != nil {
		return "", written, fmt.Errorf("failed to stream response: %w", err)
	}

	return resp.Header.Get("Content-Type"), written, nil
}

func (c *Client) UploadExternalFile(uploadURL, filename string, body io.Reader, contentLength int64) error {
	req, err := http.NewRequest(http.MethodPost, uploadURL, body)
	if err != nil {
		return fmt.Errorf("failed to create upload request: %w", err)
	}

	contentType := mime.TypeByExtension(filepath.Ext(strings.TrimSpace(filename)))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	req.Header.Set("Content-Type", contentType)
	req.ContentLength = contentLength

	resp, err := c.transferHTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload file bytes: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("file upload returned HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	return nil
}

func isSlackHostedURL(rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}

	scheme := strings.ToLower(strings.TrimSpace(u.Scheme))
	if scheme != "https" {
		return false
	}

	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host == "" {
		return false
	}

	return host == "slack.com" || strings.HasSuffix(host, ".slack.com")
}

func (c *Client) AuthTest() (*AuthTestResponse, error) {
	body, err := c.request("auth.test", url.Values{})
	if err != nil {
		return nil, err
	}

	var result AuthTestResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse auth.test response: %w", err)
	}

	return &result, nil
}

type HistoryParams struct {
	Channel   string
	Limit     int
	Oldest    string
	Latest    string
	Inclusive bool
}

type RepliesParams struct {
	Channel   string
	ThreadTS  string
	Limit     int
	Oldest    string
	Latest    string
	Inclusive bool
}

func applyHistoryParams(v url.Values, channel string, limit int, oldest, latest string, inclusive bool) {
	v.Set("channel", channel)
	if limit > 0 {
		v.Set("limit", fmt.Sprintf("%d", limit))
	}
	if oldest != "" {
		v.Set("oldest", oldest)
	}
	if latest != "" {
		v.Set("latest", latest)
	}
	if inclusive {
		v.Set("inclusive", "true")
	}
}

func (c *Client) GetConversationReplies(p RepliesParams) (*RepliesResponse, error) {
	params := url.Values{}
	applyHistoryParams(params, p.Channel, p.Limit, p.Oldest, p.Latest, p.Inclusive)
	params.Set("ts", p.ThreadTS)

	body, err := c.request("conversations.replies", params)
	if err != nil {
		return nil, err
	}

	var result RepliesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse replies response: %w", err)
	}

	return &result, nil
}

func (c *Client) GetConversationHistory(p HistoryParams) (*HistoryResponse, error) {
	params := url.Values{}
	applyHistoryParams(params, p.Channel, p.Limit, p.Oldest, p.Latest, p.Inclusive)

	body, err := c.request("conversations.history", params)
	if err != nil {
		return nil, err
	}

	var result HistoryResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse history response: %w", err)
	}

	return &result, nil
}

func (c *Client) GetConversationInfo(channel string) (*Channel, error) {
	params := url.Values{}
	params.Set("channel", channel)

	body, err := c.request("conversations.info", params)
	if err != nil {
		return nil, err
	}

	var result struct {
		Channel Channel `json:"channel"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse conversation info response: %w", err)
	}

	return &result.Channel, nil
}

func (c *Client) GetUserInfo(userID string) (*User, error) {
	params := url.Values{}
	params.Set("user", userID)

	body, err := c.request("users.info", params)
	if err != nil {
		return nil, err
	}

	var result struct {
		User User `json:"user"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse user response: %w", err)
	}

	return &result.User, nil
}

func (c *Client) SearchMessages(query string, count int) (*SearchResponse, error) {
	params := url.Values{}
	params.Set("query", query)
	if count > 0 {
		params.Set("count", fmt.Sprintf("%d", count))
	}

	body, err := c.request("search.messages", params)
	if err != nil {
		return nil, err
	}

	var result SearchResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse search response: %w", err)
	}

	return &result, nil
}

func (c *Client) ListFiles(limit int) (*FilesListResponse, error) {
	params := url.Values{}
	if limit > 0 {
		params.Set("count", fmt.Sprintf("%d", limit))
	}

	body, err := c.request("files.list", params)
	if err != nil {
		return nil, err
	}

	var result FilesListResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse files.list response: %w", err)
	}

	return &result, nil
}

func (c *Client) GetFileInfo(fileID string) (*File, error) {
	params := url.Values{}
	params.Set("file", fileID)

	body, err := c.request("files.info", params)
	if err != nil {
		return nil, err
	}

	var result FileInfoResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse files.info response: %w", err)
	}

	return &result.File, nil
}

func (c *Client) DeleteFile(fileID string) error {
	params := url.Values{}
	params.Set("file", fileID)

	if _, err := c.requestPost("files.delete", params); err != nil {
		return err
	}

	return nil
}

func (c *Client) ListConversations(types string, limit int) (*ConversationsResponse, error) {
	return c.ListConversationsPage(types, limit, "")
}

func (c *Client) ListConversationsPage(types string, limit int, cursor string) (*ConversationsResponse, error) {
	params := url.Values{}
	if types != "" {
		params.Set("types", types)
	} else {
		params.Set("types", "public_channel,private_channel")
	}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	if strings.TrimSpace(cursor) != "" {
		params.Set("cursor", cursor)
	}

	body, err := c.request("conversations.list", params)
	if err != nil {
		return nil, err
	}

	var result ConversationsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse conversations response: %w", err)
	}

	return &result, nil
}

func (c *Client) OpenConversation(users []string, returnIM bool) (*OpenConversationResponse, error) {
	params := url.Values{}
	if len(users) > 0 {
		params.Set("users", strings.Join(users, ","))
	}
	if returnIM {
		params.Set("return_im", "true")
	}

	body, err := c.requestPost("conversations.open", params)
	if err != nil {
		return nil, err
	}

	var result OpenConversationResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse conversations.open response: %w", err)
	}

	return &result, nil
}

func (c *Client) GetUploadURLExternal(filename string, length int64) (*GetUploadURLExternalResponse, error) {
	params := url.Values{}
	params.Set("filename", filename)
	params.Set("length", fmt.Sprintf("%d", length))

	body, err := c.requestPost("files.getUploadURLExternal", params)
	if err != nil {
		return nil, err
	}

	var result GetUploadURLExternalResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse files.getUploadURLExternal response: %w", err)
	}

	return &result, nil
}

func (c *Client) CompleteUploadExternal(fileID, title, channelID, initialComment, threadTS string) (*CompleteUploadExternalResponse, error) {
	params := url.Values{}
	filesPayload, err := json.Marshal([]map[string]string{{
		"id":    fileID,
		"title": title,
	}})
	if err != nil {
		return nil, fmt.Errorf("failed to encode upload completion payload: %w", err)
	}
	params.Set("files", string(filesPayload))
	if channelID != "" {
		params.Set("channel_id", channelID)
	}
	if initialComment != "" {
		params.Set("initial_comment", initialComment)
	}
	if threadTS != "" {
		params.Set("thread_ts", threadTS)
	}

	body, err := c.requestPost("files.completeUploadExternal", params)
	if err != nil {
		return nil, err
	}

	var result CompleteUploadExternalResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse files.completeUploadExternal response: %w", err)
	}

	return &result, nil
}

func (c *Client) LookupUserByEmail(email string) (*User, error) {
	params := url.Values{}
	params.Set("email", email)

	body, err := c.request("users.lookupByEmail", params)
	if err != nil {
		return nil, err
	}

	var result struct {
		User User `json:"user"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse user response: %w", err)
	}

	return &result.User, nil
}

func (c *Client) ListUsers(limit int) (*UsersResponse, error) {
	return c.ListUsersPage(limit, "")
}

func (c *Client) ListUsersPage(limit int, cursor string) (*UsersResponse, error) {
	params := url.Values{}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	if strings.TrimSpace(cursor) != "" {
		params.Set("cursor", cursor)
	}

	body, err := c.request("users.list", params)
	if err != nil {
		return nil, err
	}

	var result UsersResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse users response: %w", err)
	}

	return &result, nil
}

// ExchangeOAuthCode exchanges an OAuth authorization code for an access token
func ExchangeOAuthCode(clientID, clientSecret, code, redirectURI string) (string, error) {
	params := url.Values{}
	params.Set("client_id", clientID)
	params.Set("client_secret", clientSecret)
	params.Set("code", code)
	params.Set("redirect_uri", redirectURI)

	resp, err := http.PostForm(slackAPIBase+"/oauth.v2.access", params)
	if err != nil {
		return "", fmt.Errorf("failed to exchange code: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var result struct {
		OK         bool   `json:"ok"`
		Error      string `json:"error"`
		AuthedUser struct {
			AccessToken string `json:"access_token"`
		} `json:"authed_user"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.OK {
		return "", fmt.Errorf("oauth error: %s", result.Error)
	}

	if result.AuthedUser.AccessToken == "" {
		return "", fmt.Errorf("no user access token in response")
	}

	return result.AuthedUser.AccessToken, nil
}

// ParseThreadURL extracts channel ID and thread timestamp from a Slack thread URL
// Supports formats like:
// - https://workspace.slack.com/archives/C123ABC/p1234567890123456
// - https://app.slack.com/client/T123/C123ABC/thread/C123ABC-1234567890.123456
func ParseThreadURL(threadURL string) (channel string, threadTS string, err error) {
	u, err := url.Parse(threadURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid URL: %w", err)
	}

	// Strict host check: must be exactly slack.com or a subdomain of slack.com
	host := strings.ToLower(u.Host)
	if host != "slack.com" && !strings.HasSuffix(host, ".slack.com") {
		return "", "", fmt.Errorf("not a Slack URL")
	}

	path := strings.Trim(u.Path, "/")
	parts := strings.Split(path, "/")

	for i, part := range parts {
		if part == "archives" && i+1 < len(parts) {
			channel = parts[i+1]
			break
		}
	}

	// Reply permalinks include both the reply ts in the path and the parent
	// thread ts in query params; always prefer thread_ts when present.
	if queryThreadTS := strings.TrimSpace(u.Query().Get("thread_ts")); channel != "" && queryThreadTS != "" {
		return channel, queryThreadTS, nil
	}

	// Format: /archives/C123ABC/p1234567890123456
	for i, part := range parts {
		if part == "archives" && i+2 < len(parts) {
			ts := parts[i+2]
			if strings.HasPrefix(ts, "p") {
				// Convert p1234567890123456 to 1234567890.123456
				ts = ts[1:]
				if len(ts) >= 10 {
					threadTS = ts[:10] + "." + ts[10:]
				}
			}
			if channel != "" && threadTS != "" {
				return channel, threadTS, nil
			}
		}
	}

	return "", "", fmt.Errorf("could not parse thread URL: %s", threadURL)
}
