package output

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/glamour"
	"golang.org/x/term"
)

type MarkdownRenderer struct {
	renderer *glamour.TermRenderer
}

func NewMarkdownRenderer() (*MarkdownRenderer, error) {
	width := 80
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		width = w
		if width > 120 {
			width = 120
		}
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
		glamour.WithStylesFromJSONBytes([]byte(`{"document":{"margin":0}}`)),
	)
	if err != nil {
		return nil, fmt.Errorf("creating markdown renderer: %w", err)
	}

	return &MarkdownRenderer{renderer: r}, nil
}

func (m *MarkdownRenderer) Render(content string) (string, error) {
	out, err := m.renderer.Render(content)
	if err != nil {
		return "", fmt.Errorf("rendering markdown: %w", err)
	}

	return strings.TrimSpace(out), nil
}

func (m *MarkdownRenderer) RenderAndPrint(content string) error {
	out, err := m.Render(content)
	if err != nil {
		return err
	}
	fmt.Println(out)
	return nil
}

func RenderMarkdown(content string) error {
	r, err := NewMarkdownRenderer()
	if err != nil {
		return err
	}
	return r.RenderAndPrint(content)
}
