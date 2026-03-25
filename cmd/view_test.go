package cmd

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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

func TestMessageInlineImageURLs_RequiresHTTPS(t *testing.T) {
	cmd := &ViewCmd{Raw: true}

	got := cmd.messageInlineImageURLs(slack.Message{
		Attachments: []slack.Attachment{
			{ImageURL: "http://files.slack.com/files-pri/T123/F123/plain-http-attachment.png"},
			{ImageURL: "https://files.slack.com/files-pri/T123/F124/https-attachment.png"},
		},
		Blocks: []slack.Block{
			{Type: "image", ImageURL: "http://files.slack.com/files-pri/T123/F125/plain-http-block.png"},
			{Type: "image", ImageURL: "https://files.slack.com/files-pri/T123/F126/https-block.png"},
		},
	})

	want := []string{
		"https://files.slack.com/files-pri/T123/F124/https-attachment.png",
		"https://files.slack.com/files-pri/T123/F126/https-block.png",
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

func TestRenderInlineImage_UsesRGBAFormatForJPEG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255})

	var imageBuf bytes.Buffer
	if err := jpeg.Encode(&imageBuf, img, nil); err != nil {
		t.Fatalf("jpeg.Encode() returned error: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(imageBuf.Bytes())
	}))
	defer server.Close()

	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() returned error: %v", err)
	}
	defer readPipe.Close() //nolint:errcheck

	oldStdout := os.Stdout
	os.Stdout = writePipe
	t.Cleanup(func() {
		os.Stdout = oldStdout
	})

	cmd := &ViewCmd{Raw: true}
	client := slack.NewClient("[REDACTED:slack-access-token]")

	renderErr := cmd.renderInlineImage(client, server.URL)
	_ = writePipe.Close()
	outputBytes, readErr := io.ReadAll(readPipe)

	if renderErr != nil {
		t.Fatalf("renderInlineImage() returned error: %v", renderErr)
	}
	if readErr != nil {
		t.Fatalf("io.ReadAll() returned error: %v", readErr)
	}

	output := string(outputBytes)
	if !strings.Contains(output, "a=T,f=32,s=1,v=1") {
		t.Fatalf("renderInlineImage() output missing raw RGBA image metadata: %q", output)
	}
	if strings.Contains(output, "a=T,f=100") {
		t.Fatalf("renderInlineImage() output should not mark JPEG data as PNG: %q", output)
	}
}

func TestInlineImageColumnsForTerminalWidth(t *testing.T) {
	tests := []struct {
		name  string
		width int
		want  int
	}{
		{name: "unknown width uses default", width: 0, want: 32},
		{name: "small width uses minimum", width: 40, want: 20},
		{name: "medium width scales to one third", width: 120, want: 40},
		{name: "very large width is capped", width: 300, want: 48},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inlineImageColumnsForTerminalWidth(tt.width)
			if got != tt.want {
				t.Fatalf("inlineImageColumnsForTerminalWidth(%d) = %d, want %d", tt.width, got, tt.want)
			}
		})
	}
}

func TestInlineImageRowsForPayload(t *testing.T) {
	tests := []struct {
		name    string
		payload inlineImagePayload
		cols    int
		want    int
	}{
		{name: "unknown dimensions disables row constraint", payload: inlineImagePayload{}, cols: 40, want: 0},
		{name: "landscape image", payload: inlineImagePayload{width: 1600, height: 900}, cols: 48, want: 14},
		{name: "portrait image is capped", payload: inlineImagePayload{width: 900, height: 1600}, cols: 48, want: 24},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inlineImageRowsForPayload(&tt.payload, tt.cols)
			if got != tt.want {
				t.Fatalf("inlineImageRowsForPayload(%+v, %d) = %d, want %d", tt.payload, tt.cols, got, tt.want)
			}
		})
	}
}

func TestFirstInlineImageChunkParams(t *testing.T) {
	t.Run("raw payload includes dimensions and row constraint", func(t *testing.T) {
		payload := &inlineImagePayload{format: kittyImageFormatRaw, width: 640, height: 480}
		got := firstInlineImageChunkParams(payload, 48, 18, 1)
		want := "a=T,f=32,s=640,v=480,c=48,r=18,m=1"
		if got != want {
			t.Fatalf("firstInlineImageChunkParams() = %q, want %q", got, want)
		}
	})

	t.Run("png payload includes row constraint", func(t *testing.T) {
		payload := &inlineImagePayload{format: kittyImageFormatPNG, width: 1200, height: 800}
		got := firstInlineImageChunkParams(payload, 40, 12, 0)
		want := "a=T,f=100,c=40,r=12,m=0"
		if got != want {
			t.Fatalf("firstInlineImageChunkParams() = %q, want %q", got, want)
		}
	})

	t.Run("unknown row omits row constraint", func(t *testing.T) {
		payload := &inlineImagePayload{format: kittyImageFormatPNG}
		got := firstInlineImageChunkParams(payload, 32, 0, 0)
		want := "a=T,f=100,c=32,m=0"
		if got != want {
			t.Fatalf("firstInlineImageChunkParams() = %q, want %q", got, want)
		}
	})
}

func TestBuildInlineImagePayload_RejectsOversizedDimensionsBeforeDecode(t *testing.T) {
	const fakeMagic = "FAKEIMG!"
	const decodeCalledMessage = "decode should not be called"

	image.RegisterFormat(
		"oversized-fake-inline-image",
		fakeMagic,
		func(io.Reader) (image.Image, error) {
			return nil, errors.New(decodeCalledMessage)
		},
		func(io.Reader) (image.Config, error) {
			return image.Config{Width: 20000, Height: 20000}, nil
		},
	)

	_, err := buildInlineImagePayload([]byte(fakeMagic+"payload"), "image/fake")
	if err == nil {
		t.Fatalf("buildInlineImagePayload() expected error for oversized dimensions")
	}

	if !strings.Contains(err.Error(), "decoded image exceeds limit") {
		t.Fatalf("buildInlineImagePayload() error = %q, want contains %q", err.Error(), "decoded image exceeds limit")
	}

	if strings.Contains(err.Error(), decodeCalledMessage) {
		t.Fatalf("buildInlineImagePayload() unexpectedly decoded full image before applying size guard: %q", err.Error())
	}
}

func TestBuildInlineImagePayload_PNGRejectsOversizedDimensions(t *testing.T) {
	const fakeMagic = "FAKEPNG!"

	image.RegisterFormat(
		"oversized-fake-png-inline-image",
		fakeMagic,
		func(io.Reader) (image.Image, error) {
			return nil, errors.New("decode should not be called")
		},
		func(io.Reader) (image.Config, error) {
			return image.Config{Width: 20000, Height: 20000}, nil
		},
	)

	_, err := buildInlineImagePayload([]byte(fakeMagic+"payload"), "image/png")
	if err == nil {
		t.Fatalf("buildInlineImagePayload() expected error for oversized PNG dimensions")
	}

	if !strings.Contains(err.Error(), "decoded image exceeds limit") {
		t.Fatalf("buildInlineImagePayload() error = %q, want contains %q", err.Error(), "decoded image exceeds limit")
	}
}

func TestIsImageFile_SupportedInlineFormats(t *testing.T) {
	tests := []struct {
		name string
		file slack.File
		want bool
	}{
		{name: "png by mimetype", file: slack.File{Mimetype: "image/png"}, want: true},
		{name: "jpeg by filetype", file: slack.File{Filetype: "jpeg"}, want: true},
		{name: "gif by filetype", file: slack.File{Filetype: "gif"}, want: true},
		{name: "webp unsupported", file: slack.File{Filetype: "webp"}, want: false},
		{name: "bmp unsupported", file: slack.File{Filetype: "bmp"}, want: false},
		{name: "svg unsupported", file: slack.File{Filetype: "svg"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isImageFile(tt.file)
			if got != tt.want {
				t.Fatalf("isImageFile(%+v) = %v, want %v", tt.file, got, tt.want)
			}
		})
	}
}
