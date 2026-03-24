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
