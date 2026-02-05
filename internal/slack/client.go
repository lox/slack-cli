package slack

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const slackAPIBase = "https://slack.com/api"

type Client struct {
	userToken  string
	httpClient *http.Client
}

func NewClient(userToken string) *Client {
	return &Client{
		userToken: userToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) request(method string, params url.Values) ([]byte, error) {
	req, err := http.NewRequest("GET", slackAPIBase+"/"+method+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.userToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

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
		return nil, fmt.Errorf("slack API error: %s", slackResp.Error)
	}

	return body, nil
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

func (c *Client) GetConversationReplies(channel, threadTS string, limit int) (*RepliesResponse, error) {
	params := url.Values{}
	params.Set("channel", channel)
	params.Set("ts", threadTS)
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}

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

func (c *Client) GetConversationHistory(channel string, limit int) (*HistoryResponse, error) {
	params := url.Values{}
	params.Set("channel", channel)
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}

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

func (c *Client) ListConversations(types string, limit int) (*ConversationsResponse, error) {
	params := url.Values{}
	if types != "" {
		params.Set("types", types)
	} else {
		params.Set("types", "public_channel,private_channel")
	}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
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
	params := url.Values{}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
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
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var result struct {
		OK          bool   `json:"ok"`
		Error       string `json:"error"`
		AuthedUser  struct {
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

	// Format: /archives/C123ABC/p1234567890123456
	for i, part := range parts {
		if part == "archives" && i+2 < len(parts) {
			channel = parts[i+1]
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

	// Format with thread_ts in query param
	if threadTS := u.Query().Get("thread_ts"); threadTS != "" {
		for i, part := range parts {
			if part == "archives" && i+1 < len(parts) {
				return parts[i+1], threadTS, nil
			}
		}
	}

	return "", "", fmt.Errorf("could not parse thread URL: %s", threadURL)
}
