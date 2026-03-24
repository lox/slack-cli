package cmd

import "testing"

func TestParseChannelReference(t *testing.T) {
	t.Run("channel name strips leading hash", func(t *testing.T) {
		channelID, urlHint, err := parseChannelReference("#project-namespace-capacity")
		if err != nil {
			t.Fatalf("parseChannelReference returned error: %v", err)
		}
		if channelID != "project-namespace-capacity" {
			t.Fatalf("expected channel name without hash, got %q", channelID)
		}
		if urlHint != "" {
			t.Fatalf("expected empty urlHint, got %q", urlHint)
		}
	})

	t.Run("channel URL extracts channel ID and preserves URL hint", func(t *testing.T) {
		channelID, urlHint, err := parseChannelReference("https://buildkite-corp.slack.com/archives/C0AMP05SJKX/p1773973307481399")
		if err != nil {
			t.Fatalf("parseChannelReference returned error: %v", err)
		}
		if channelID != "C0AMP05SJKX" {
			t.Fatalf("expected channel ID C0AMP05SJKX, got %q", channelID)
		}
		if urlHint != "https://buildkite-corp.slack.com/archives/C0AMP05SJKX/p1773973307481399" {
			t.Fatalf("expected urlHint to preserve original URL, got %q", urlHint)
		}
	})

	t.Run("invalid channel URL returns parse error", func(t *testing.T) {
		_, _, err := parseChannelReference("https://example.com/archives/C0AMP05SJKX")
		if err == nil {
			t.Fatalf("expected parse error for non-Slack URL")
		}
	})
}

func TestIsSlackChannelID(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{input: "C123", want: true},
		{input: "G123", want: true},
		{input: "D123", want: true},
		{input: "project-namespace-capacity", want: false},
	}

	for _, tt := range tests {
		if got := isSlackChannelID(tt.input); got != tt.want {
			t.Fatalf("isSlackChannelID(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
