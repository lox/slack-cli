package slack

import "testing"

// newTestResolver creates a Resolver with pre-populated caches (no API calls needed).
func newTestResolver(users map[string]string, channels map[string]string) *Resolver {
	if users == nil {
		users = make(map[string]string)
	}
	if channels == nil {
		channels = make(map[string]string)
	}
	return &Resolver{
		userCache:    users,
		channelCache: channels,
	}
}

func TestFormatText(t *testing.T) {
	tests := []struct {
		name     string
		users    map[string]string
		channels map[string]string
		input    string
		want     string
	}{
		{
			name:  "plain text unchanged",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "user mention resolved",
			users: map[string]string{"U123": "alice"},
			input: "hey <@U123> check this",
			want:  "hey @alice check this",
		},
		{
			name:  "user mention with fallback name used when unknown",
			users: map[string]string{"U999": "U999"}, // simulate failed lookup (cached as raw ID)
			input: "hey <@U999|bob>",
			want:  "hey @bob",
		},
		{
			name:  "user mention with fallback ignored when resolved",
			users: map[string]string{"U123": "alice"},
			input: "hey <@U123|old-name>",
			want:  "hey @alice",
		},
		{
			name:  "display name containing <@ does not loop",
			users: map[string]string{"U123": "tricky<@name"},
			input: "hi <@U123>",
			want:  "hi @tricky<@name",
		},
		{
			name:     "channel mention with pipe",
			channels: map[string]string{"C456": "general"},
			input:    "see <#C456|general>",
			want:     "see #general",
		},
		{
			name:     "channel mention without pipe",
			channels: map[string]string{"C456": "general"},
			input:    "see <#C456>",
			want:     "see #general",
		},
		{
			name:  "URL with label",
			input: "visit <https://example.com|Example>",
			want:  "visit Example (https://example.com)",
		},
		{
			name:  "URL without label",
			input: "visit <https://example.com>",
			want:  "visit https://example.com",
		},
		{
			name:  "emoji shortcode",
			input: "great :thumbsup:",
			want:  "great 👍",
		},
		{
			name:     "multiple mentions",
			users:    map[string]string{"U1": "alice", "U2": "bob"},
			channels: map[string]string{"C1": "general"},
			input:    "<@U1> and <@U2> in <#C1|general>",
			want:     "@alice and @bob in #general",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestResolver(tt.users, tt.channels)
			got := r.FormatText(tt.input)
			if got != tt.want {
				t.Errorf("FormatText(%q)\n got: %q\nwant: %q", tt.input, got, tt.want)
			}
		})
	}
}
