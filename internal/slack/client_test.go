package slack

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseThreadURL(t *testing.T) {
	t.Run("message permalink uses message timestamp", func(t *testing.T) {
		channel, threadTS, err := ParseThreadURL("https://buildkite.slack.com/archives/C123/p1773973307481399")
		if err != nil {
			t.Fatalf("ParseThreadURL returned error: %v", err)
		}
		if channel != "C123" {
			t.Fatalf("expected channel C123, got %q", channel)
		}
		if threadTS != "1773973307.481399" {
			t.Fatalf("expected threadTS 1773973307.481399, got %q", threadTS)
		}
	})

	t.Run("reply permalink prefers thread_ts query parameter", func(t *testing.T) {
		channel, threadTS, err := ParseThreadURL("https://buildkite.slack.com/archives/C123/p1773999999000000?thread_ts=1773973307.481399&cid=C123")
		if err != nil {
			t.Fatalf("ParseThreadURL returned error: %v", err)
		}
		if channel != "C123" {
			t.Fatalf("expected channel C123, got %q", channel)
		}
		if threadTS != "1773973307.481399" {
			t.Fatalf("expected threadTS 1773973307.481399, got %q", threadTS)
		}
	})
}

func TestIsSlackHostedURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{name: "slack root", url: "https://slack.com/file.png", want: true},
		{name: "slack subdomain", url: "https://files.slack.com/file.png", want: true},
		{name: "http slack subdomain", url: "http://files.slack.com/file.png", want: false},
		{name: "uppercase host", url: "https://FILES.SLACK.COM/file.png", want: true},
		{name: "external host", url: "https://example.com/file.png", want: false},
		{name: "empty", url: "", want: false},
		{name: "invalid", url: "://bad-url", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSlackHostedURL(tt.url)
			if got != tt.want {
				t.Fatalf("isSlackHostedURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestDownloadPrivateFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("123456"))
	}))
	defer server.Close()

	client := &Client{
		userToken:  "xoxp-test-token",
		httpClient: server.Client(),
	}

	t.Run("within size limit", func(t *testing.T) {
		body, contentType, err := client.DownloadPrivateFile(server.URL, 6)
		if err != nil {
			t.Fatalf("DownloadPrivateFile returned error: %v", err)
		}
		if string(body) != "123456" {
			t.Fatalf("DownloadPrivateFile body = %q, want %q", string(body), "123456")
		}
		if contentType != "image/png" {
			t.Fatalf("DownloadPrivateFile contentType = %q, want %q", contentType, "image/png")
		}
	})

	t.Run("exceeds size limit", func(t *testing.T) {
		_, _, err := client.DownloadPrivateFile(server.URL, 5)
		if err == nil {
			t.Fatalf("DownloadPrivateFile expected error when payload exceeds size limit")
		}
		if !strings.Contains(err.Error(), "download exceeds limit") {
			t.Fatalf("DownloadPrivateFile error = %q, want contains %q", err.Error(), "download exceeds limit")
		}
	})

	t.Run("invalid limit", func(t *testing.T) {
		_, _, err := client.DownloadPrivateFile(server.URL, 0)
		if err == nil {
			t.Fatalf("DownloadPrivateFile expected error for invalid maxBytes")
		}
		if !strings.Contains(err.Error(), "maxBytes must be > 0") {
			t.Fatalf("DownloadPrivateFile error = %q, want contains %q", err.Error(), "maxBytes must be > 0")
		}
	})
}

func TestDownloadPrivateFile_AuthorizationHeaderPolicy(t *testing.T) {
	tests := []struct {
		name     string
		fileURL  string
		wantAuth string
	}{
		{name: "https slack host sends token", fileURL: "https://files.slack.com/files-pri/T123/F123/file.png", wantAuth: "Bearer [REDACTED:slack-access-token]"},
		{name: "http slack host does not send token", fileURL: "http://files.slack.com/files-pri/T123/F123/file.png", wantAuth: ""},
		{name: "https external host does not send token", fileURL: "https://example.com/file.png", wantAuth: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotAuth string

			client := &Client{
				userToken: "[REDACTED:slack-access-token]",
				httpClient: &http.Client{
					Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
						gotAuth = req.Header.Get("Authorization")
						return &http.Response{
							StatusCode: http.StatusOK,
							Status:     "200 OK",
							Header:     http.Header{"Content-Type": []string{"image/png"}},
							Body:       io.NopCloser(strings.NewReader("ok")),
							Request:    req,
						}, nil
					}),
				},
			}

			_, _, err := client.DownloadPrivateFile(tt.fileURL, 2)
			if err != nil {
				t.Fatalf("DownloadPrivateFile() returned error: %v", err)
			}

			if gotAuth != tt.wantAuth {
				t.Fatalf("DownloadPrivateFile() authorization header = %q, want %q", gotAuth, tt.wantAuth)
			}
		})
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
