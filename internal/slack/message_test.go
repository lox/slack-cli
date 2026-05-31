package slack

import "testing"

func TestMessageBodyTextUsesLongerSectionBlocks(t *testing.T) {
	msg := Message{
		Text: "LongMessage.Part",
		Blocks: []Block{
			{
				Type: "section",
				Text: &BlockText{Type: "mrkdwn", Text: "LongMessage.Part"},
			},
			{
				Type: "section",
				Text: &BlockText{Type: "mrkdwn", Text: "Two + ordering"},
			},
		},
	}

	got := msg.BodyText()
	want := "LongMessage.PartTwo + ordering"
	if got != want {
		t.Fatalf("BodyText() = %q, want %q", got, want)
	}
}

func TestMessageBodyTextKeepsFallbackWhenBlocksAreShorter(t *testing.T) {
	msg := Message{
		Text: "complete fallback text",
		Blocks: []Block{
			{
				Type: "section",
				Text: &BlockText{Type: "mrkdwn", Text: "short"},
			},
		},
	}

	if got := msg.BodyText(); got != msg.Text {
		t.Fatalf("BodyText() = %q, want fallback %q", got, msg.Text)
	}
}
