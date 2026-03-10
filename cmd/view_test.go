package cmd

import (
	"strings"
	"testing"

	"github.com/lox/slack-cli/internal/slack"
)

func TestFormatMessageBody_TextOnly(t *testing.T) {
	cmd := &ViewCmd{Raw: true}

	got := cmd.formatMessageBody(slack.Message{
		Text: "hello world",
	})

	if got != "hello world" {
		t.Fatalf("formatMessageBody() = %q, want %q", got, "hello world")
	}
}

func TestNormalizeInlineImagesMode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "default empty", input: "", want: "auto"},
		{name: "trim and lowercase", input: "  ALWAYS  ", want: "always"},
		{name: "auto", input: "auto", want: "auto"},
		{name: "never", input: "never", want: "never"},
		{name: "invalid", input: "sometimes", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeInlineImagesMode(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("normalizeInlineImagesMode(%q) expected error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeInlineImagesMode(%q) returned error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("normalizeInlineImagesMode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatMessageBody_FilesAndImages(t *testing.T) {
	cmd := &ViewCmd{Raw: true}

	got := cmd.formatMessageBody(slack.Message{
		Text: "see attached",
		Files: []slack.File{
			{
				Title:     "Screenshot",
				Mimetype:  "image/png",
				Permalink: "https://files.example/image",
			},
			{
				Name:      "spec.pdf",
				Mimetype:  "application/pdf",
				Permalink: "https://files.example/pdf",
			},
		},
		Attachments: []slack.Attachment{
			{
				ImageURL: "https://img.example/diagram.png",
				Title:    "Diagram",
			},
		},
		Blocks: []slack.Block{
			{
				Type:     "image",
				ImageURL: "https://img.example/block.png",
				AltText:  "Block image",
			},
		},
	})

	checks := []string{
		"see attached",
		"**Attachments**",
		"- Image: [Screenshot](https://files.example/image)",
		"- File: [spec.pdf](https://files.example/pdf)",
		"- Image: [Diagram](https://img.example/diagram.png)",
		"- Image: [Block image](https://img.example/block.png)",
	}

	for _, check := range checks {
		if !strings.Contains(got, check) {
			t.Fatalf("formatMessageBody() missing %q in:\n%s", check, got)
		}
	}
}

func TestFormatMessageBody_AttachmentsOnly(t *testing.T) {
	cmd := &ViewCmd{Raw: true}

	got := cmd.formatMessageBody(slack.Message{
		Files: []slack.File{
			{
				Name:               "image-no-permalink.png",
				Mimetype:           "image/png",
				URLPrivateDownload: "https://files.example/download",
			},
		},
	})

	want := "**Attachments**\n- Image: [image-no-permalink.png](https://files.example/download)"
	if got != want {
		t.Fatalf("formatMessageBody() = %q, want %q", got, want)
	}
}

func TestMessageInlineImageURLs(t *testing.T) {
	cmd := &ViewCmd{Raw: true}

	got := cmd.messageInlineImageURLs(slack.Message{
		Files: []slack.File{
			{
				Mimetype:           "image/png",
				URLPrivate:         "https://files.slack.com/files-pri/T123/F123/private.png",
				URLPrivateDownload: "https://files.example/download.png",
				Permalink:          "https://files.example/permalink.png",
			},
			{
				Mimetype:   "application/pdf",
				URLPrivate: "https://files.example/doc.pdf",
			},
		},
		Attachments: []slack.Attachment{
			{
				ImageURL: "https://img.example/attachment.png",
			},
			{
				ImageURL: "https://files.slack.com/files-pri/T123/F123/attachment.png",
			},
		},
		Blocks: []slack.Block{
			{
				Type:     "image",
				ImageURL: "https://img.example/block.png",
			},
			{
				Type:     "image",
				ImageURL: "https://files.slack.com/files-pri/T123/F456/block.png",
			},
		},
	})

	want := []string{
		"https://files.slack.com/files-pri/T123/F123/private.png",
		"https://files.slack.com/files-pri/T123/F123/attachment.png",
		"https://files.slack.com/files-pri/T123/F456/block.png",
	}

	if len(got) != len(want) {
		t.Fatalf("messageInlineImageURLs() len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("messageInlineImageURLs()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
