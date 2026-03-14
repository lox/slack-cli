package slack

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsSlackHostedURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{name: "slack root", url: "https://slack.com/file.png", want: true},
		{name: "slack subdomain", url: "https://files.slack.com/file.png", want: true},
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
