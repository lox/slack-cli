package slack

import "testing"

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
