package slack

import "testing"

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
