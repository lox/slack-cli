package slack

import "strings"

// BodyText returns the most complete text Slack exposed for a message.
func (m Message) BodyText() string {
	blockText := m.SectionBlockText()
	if strings.TrimSpace(blockText) != "" && len(blockText) > len(m.Text) {
		return blockText
	}

	return m.Text
}

func (m Message) SectionBlockText() string {
	var sb strings.Builder

	for _, block := range m.Blocks {
		if block.Type != "section" || block.Text == nil {
			continue
		}
		sb.WriteString(block.Text.Text)
	}

	return sb.String()
}
