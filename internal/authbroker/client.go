// Package authbroker authenticates CLI sessions through an HTTPS OAuth broker.
package authbroker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var identifier = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)
var confirmationCode = regexp.MustCompile(`^[A-F0-9]{8}$`)

// Session contains only the information safe to show to the person logging in.
type Session struct {
	BrowserURL       string
	ConfirmationCode string
}

// Client communicates with one explicitly trusted OAuth broker origin.
type Client struct {
	origin string
	http   *http.Client
	wait   func(context.Context, time.Duration) error
}

// New requires an HTTPS origin, without credentials, a path, query or fragment.
func New(origin string) (*Client, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, fmt.Errorf("--auth-server must be an HTTPS origin without credentials, path, query or fragment")
	}
	return &Client{
		origin: "https://" + u.Host,
		http: &http.Client{
			Timeout: 30 * time.Second,
			// Redirects must never move polling credentials to another endpoint.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		wait: wait,
	}, nil
}

// Login creates a session, shows browser instructions, and polls for a token.
// Delivery is at most once. Any ambiguous failure requires a fresh login.
func (c *Client) Login(ctx context.Context, teamID string, show func(Session) error) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	body, err := json.Marshal(map[string]string{"team_id": teamID})
	if err != nil {
		return "", err
	}
	status, _, data, err := c.request(ctx, "/v1/sessions", string(body), "")
	if err != nil {
		return "", err
	}
	if status != http.StatusCreated {
		return "", responseError(status, data)
	}
	var session struct {
		ID               string `json:"session_id"`
		BrowserURL       string `json:"browser_url"`
		PollToken        string `json:"poll_token"`
		ConfirmationCode string `json:"confirmation_code"`
		ExpiresIn        int    `json:"expires_in"`
		Interval         int    `json:"interval"`
	}
	if json.Unmarshal(data, &session) != nil || !identifier.MatchString(session.ID) ||
		!identifier.MatchString(session.PollToken) || !confirmationCode.MatchString(session.ConfirmationCode) ||
		session.BrowserURL != c.origin+"/authorize?session="+session.ID ||
		session.ExpiresIn < 1 || session.ExpiresIn > 600 || session.Interval < 3 || session.Interval > session.ExpiresIn {
		return "", fmt.Errorf("invalid authentication session response")
	}
	ctx, expire := context.WithTimeout(ctx, time.Duration(session.ExpiresIn)*time.Second)
	defer expire()
	if err := show(Session{BrowserURL: session.BrowserURL, ConfirmationCode: session.ConfirmationCode}); err != nil {
		return "", err
	}
	interval := time.Duration(session.Interval) * time.Second
	delay := interval
	for {
		if err := c.wait(ctx, delay); err != nil {
			return "", fmt.Errorf("authentication stopped: %w", err)
		}
		status, headers, data, err := c.request(ctx, "/v1/sessions/"+session.ID+"/poll", "", session.PollToken)
		if err != nil {
			return "", fmt.Errorf("token delivery could not be confirmed; start a new login: %w", err)
		}
		delay = interval
		switch status {
		case http.StatusAccepted:
			var pending struct {
				Status string `json:"status"`
			}
			if json.Unmarshal(data, &pending) != nil || pending.Status != "pending" {
				return "", fmt.Errorf("invalid polling response; start a new login")
			}
		case http.StatusTooManyRequests:
			seconds, err := strconv.Atoi(headers.Get("Retry-After"))
			if err != nil || seconds < 1 || seconds > 600 {
				return "", fmt.Errorf("invalid polling retry delay; start a new login")
			}
			delay = max(interval, time.Duration(seconds)*time.Second)
		case http.StatusOK:
			var result struct {
				AccessToken string `json:"access_token"`
				TokenType   string `json:"token_type"`
				TeamID      string `json:"team_id"`
				UserID      string `json:"user_id"`
			}
			if json.Unmarshal(data, &result) != nil || result.AccessToken == "" || result.TokenType != "user" ||
				result.TeamID != teamID || result.UserID == "" {
				return "", fmt.Errorf("invalid token delivery; start a new login")
			}
			return result.AccessToken, nil
		default:
			return "", responseError(status, data)
		}
	}
}

func (c *Client) request(ctx context.Context, path, body, token string) (int, http.Header, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.origin+path, strings.NewReader(body))
	if err != nil {
		return 0, nil, nil, fmt.Errorf("invalid authentication request")
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		// Do not expose transport errors: they can contain URLs or credentials.
		return 0, nil, nil, fmt.Errorf("auth server request failed")
	}
	defer resp.Body.Close() //nolint:errcheck
	const limit = 16 * 1024
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || len(data) > limit {
		return 0, nil, nil, fmt.Errorf("could not read auth server response")
	}
	return resp.StatusCode, resp.Header, data, nil
}

func responseError(status int, data []byte) error {
	var result struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(data, &result) == nil {
		// Only show known protocol errors; arbitrary responses may contain secrets.
		switch result.Error {
		case "invalid_request", "unsupported_workspace", "access_denied", "workspace_mismatch",
			"oauth_failed", "unsupported_token", "insufficient_scope", "invalid_poll_token", "session_gone",
			"slow_down", "capacity_exceeded":
			return fmt.Errorf("auth server: %s; start a new login", result.Error)
		}
	}
	return fmt.Errorf("auth server returned HTTP %d; start a new login", status)
}

func wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
