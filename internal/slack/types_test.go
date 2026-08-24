package slack

import (
	"encoding/json"
	"testing"
)

func TestHistoryResponseUnmarshalsBlockTitles(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantType string
		wantText string
	}{
		{
			name:     "string title",
			body:     "{\"ok\":true,\"messages\":[{\"blocks\":[{\"type\":\"image\",\"title\":\"A plain title\"}]}]}",
			wantType: "plain_text",
			wantText: "A plain title",
		},
		{
			name:     "composition object title",
			body:     "{\"ok\":true,\"messages\":[{\"blocks\":[{\"type\":\"image\",\"title\":{\"type\":\"plain_text\",\"text\":\"An object title\"}}]}]}",
			wantType: "plain_text",
			wantText: "An object title",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var response HistoryResponse
			if err := json.Unmarshal([]byte(tt.body), &response); err != nil {
				t.Fatalf("json.Unmarshal returned error: %v", err)
			}

			title := response.Messages[0].Blocks[0].Title
			if title == nil {
				t.Fatal("expected block title, got nil")
			}
			if title.Type != tt.wantType {
				t.Errorf("title.Type = %q, want %q", title.Type, tt.wantType)
			}
			if title.Text != tt.wantText {
				t.Errorf("title.Text = %q, want %q", title.Text, tt.wantText)
			}
		})
	}
}
