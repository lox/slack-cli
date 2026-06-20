package output

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/lox/slack-cli/internal/slack"
)

func TestChannelTypeFor(t *testing.T) {
	tests := []struct {
		name string
		in   slack.Channel
		want string
	}{
		{"public channel", slack.Channel{IsChannel: true}, "channel"},
		{"private channel", slack.Channel{IsPrivate: true}, "private_channel"},
		{"group", slack.Channel{IsGroup: true}, "private_channel"},
		{"im", slack.Channel{IsIM: true}, "im"},
		{"mpim", slack.Channel{IsMPIM: true}, "mpim"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ChannelTypeFor(tt.in); got != tt.want {
				t.Fatalf("ChannelTypeFor(%+v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestChannelTypeFromID(t *testing.T) {
	tests := map[string]string{
		"":     "",
		"D123": "im",
		// Everything other than D is ambiguous from the ID prefix alone:
		// C covers public and private channels, G covers mpim and legacy
		// private channels. Callers must resolve the full slack.Channel
		// when they need the distinction.
		"C123": "",
		"G123": "",
		"U123": "",
	}
	for in, want := range tests {
		if got := ChannelTypeFromID(in); got != want {
			t.Fatalf("ChannelTypeFromID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestToChannel(t *testing.T) {
	ch := slack.Channel{
		ID:         "C1",
		Name:       "general",
		IsPrivate:  false,
		NumMembers: 7,
		Topic:      slack.Topic{Value: "chatting"},
		Purpose:    slack.Topic{Value: "all-hands"},
	}
	got := ToChannel(ch)
	if got.ID != "C1" || got.Name != "general" || got.NumMembers != 7 {
		t.Fatalf("unexpected channel: %+v", got)
	}
	if got.Type != "channel" || got.Topic != "chatting" || got.Purpose != "all-hands" {
		t.Fatalf("unexpected channel: %+v", got)
	}
}

func TestToUser(t *testing.T) {
	u := slack.User{
		ID:       "U1",
		Name:     "alice",
		RealName: "Alice",
		TZ:       "UTC",
		Profile: slack.Profile{
			DisplayName: "al",
			Email:       "a@example.com",
			Title:       "eng",
		},
	}
	got := ToUser(u)
	if got.DisplayName != "al" || got.Email != "a@example.com" || got.Title != "eng" {
		t.Fatalf("unexpected user: %+v", got)
	}
}

func TestChannelRefFromIDFallsBackToIDPrefix(t *testing.T) {
	tests := []struct {
		id   string
		want string
	}{
		// Only D IDs are unambiguous. C and G IDs cover multiple
		// conversation kinds so the type stays empty until a full
		// slack.Channel is resolved.
		{"D1", "im"},
		{"C1", ""},
		{"G1", ""},
	}
	for _, tt := range tests {
		ref := ChannelRefFromID(nil, tt.id, "")
		if ref.ID != tt.id || ref.Type != tt.want {
			t.Fatalf("ChannelRefFromID(%q) = %+v, want type %q", tt.id, ref, tt.want)
		}
	}
}

func TestChannelRefFromIDPreservesName(t *testing.T) {
	ref := ChannelRefFromID(nil, "C1", "general")
	if ref.Name != "general" {
		t.Fatalf("expected name preserved, got %+v", ref)
	}
}

func TestToFileRef(t *testing.T) {
	f := slack.File{
		ID:        "F1",
		Name:      "a.png",
		Title:     "Hi",
		Mimetype:  "image/png",
		Permalink: "https://x.slack.com/files/U1/F1/a.png",
	}
	got := ToFileRef(f)
	if got.ID != "F1" || got.Name != "a.png" || got.Mimetype != "image/png" {
		t.Fatalf("unexpected file ref: %+v", got)
	}
}

func TestMessageConverterPopulatesFields(t *testing.T) {
	conv := MessageConverter{
		Channel:   &ChannelRef{ID: "C1", Name: "general", Type: "channel"},
		Workspace: "example.slack.com",
	}
	got := conv.Convert(slack.Message{
		Type:       "message",
		User:       "U1",
		Text:       "hello <@U2>",
		TS:         "100.000001",
		ThreadTS:   "100.000001",
		ReplyCount: 3,
		Files: []slack.File{{
			ID:        "F1",
			Name:      "a.png",
			Mimetype:  "image/png",
			Permalink: "https://x.slack.com/F1",
		}},
	})

	if got.TS != "100.000001" || got.UserID != "U1" || got.ReplyCount != 3 {
		t.Fatalf("unexpected message: %+v", got)
	}
	if got.TextRaw != "hello <@U2>" {
		t.Fatalf("expected raw text preserved, got %q", got.TextRaw)
	}
	if got.Text != got.TextRaw {
		t.Fatalf("expected text == raw when resolver is nil, got %q vs %q", got.Text, got.TextRaw)
	}
	if got.Workspace != "example.slack.com" {
		t.Fatalf("expected workspace populated, got %q", got.Workspace)
	}
	if got.Channel == nil || got.Channel.ID != "C1" {
		t.Fatalf("expected channel ref, got %+v", got.Channel)
	}
	if len(got.Files) != 1 || got.Files[0].ID != "F1" {
		t.Fatalf("expected one file ref, got %+v", got.Files)
	}
}

func TestMessageConverterIncludesScopeAndType(t *testing.T) {
	conv := MessageConverter{
		Channel: &ChannelRef{ID: "D1", Type: "im"},
	}
	got := conv.Convert(slack.Message{
		Type: "message",
		User: "U1",
		Text: "hi",
		TS:   "100",
	})
	if got.Channel == nil || got.Channel.ID != "D1" || got.Channel.Type != "im" {
		t.Fatalf("expected scope channel included, got %+v", got.Channel)
	}
	if got.Type != "message" {
		t.Fatalf("expected type included, got %q", got.Type)
	}
	if got.TextRaw != "hi" {
		t.Fatalf("expected text_raw included, got %q", got.TextRaw)
	}
}

func TestMessageConverterPreservesSubtype(t *testing.T) {
	conv := MessageConverter{}
	got := conv.Convert(slack.Message{
		Type:    "message",
		Subtype: "bot_message",
		Text:    "deploy finished",
		TS:      "100",
	})
	if got.Subtype != "bot_message" {
		t.Fatalf("expected subtype preserved, got %q", got.Subtype)
	}
}

func TestMessageConverterUsesBodyText(t *testing.T) {
	conv := MessageConverter{}
	got := conv.Convert(slack.Message{
		Type: "message",
		Text: "LongMessage.Part",
		TS:   "100",
		Blocks: []slack.Block{
			{
				Type: "section",
				Text: &slack.BlockText{Type: "mrkdwn", Text: "LongMessage.Part"},
			},
			{
				Type: "section",
				Text: &slack.BlockText{Type: "mrkdwn", Text: "Two + ordering"},
			},
		},
	})
	if got.Text != "LongMessage.PartTwo + ordering" {
		t.Fatalf("expected body text in JSON record, got %q", got.Text)
	}
}

func TestMessageConverterPrefersMessageChannelWhenConverterHasNone(t *testing.T) {
	conv := MessageConverter{}
	got := conv.Convert(slack.Message{
		TS:      "100",
		Text:    "x",
		Channel: &slack.Channel{ID: "C9", Name: "from-msg"},
	})
	if got.Channel == nil || got.Channel.ID != "C9" || got.Channel.Name != "from-msg" {
		t.Fatalf("expected channel from message ref, got %+v", got.Channel)
	}
}

func TestMessageConverterOmitsEmptyUserForBotMessages(t *testing.T) {
	conv := MessageConverter{}
	got := conv.Convert(slack.Message{TS: "100", Text: "bot"})
	if got.UserID != "" || got.User != "" {
		t.Fatalf("expected empty user for bot, got user_id=%q user=%q", got.UserID, got.User)
	}
}

func TestMessageJSONShape(t *testing.T) {
	m := Message{
		TS:     "100.000001",
		User:   "alice",
		UserID: "U1",
		Text:   "hi",
		Channel: &ChannelRef{
			ID:   "C1",
			Name: "general",
			Type: "channel",
		},
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := []string{
		`"ts":"100.000001"`,
		`"user":"alice"`,
		`"user_id":"U1"`,
		`"channel":{`,
		`"type":"channel"`,
	}
	s := string(b)
	for _, w := range want {
		if !strings.Contains(s, w) {
			t.Fatalf("expected %q in %s", w, s)
		}
	}
	// text_raw and optional fields should stay off when empty.
	if strings.Contains(s, `"text_raw"`) {
		t.Fatalf("expected text_raw omitted when empty, got %s", s)
	}
	if strings.Contains(s, `"workspace"`) {
		t.Fatalf("expected workspace omitted when empty, got %s", s)
	}
}
