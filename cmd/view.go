package cmd

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/lox/slack-cli/internal/output"
	"github.com/lox/slack-cli/internal/slack"
	"golang.org/x/term"
)

type ViewCmd struct {
	URL          string `arg:"" help:"Slack URL (message, thread, or channel)"`
	Markdown     bool   `help:"Output as markdown instead of terminal formatting" short:"m"`
	Limit        int    `help:"Maximum messages to show for channels/threads" default:"20"`
	Raw          bool   `help:"Don't resolve user/channel mentions" short:"r"`
	InlineImages string `help:"Inline image rendering mode: auto, always, never" default:"auto" enum:"auto,always,never"`

	resolver *slack.Resolver
}

type slackURLInfo struct {
	Channel   string
	MessageTS string
	ThreadTS  string
	IsThread  bool
}

const (
	maxInlineImageBytes = 10 << 20 // 10 MiB
	maxInlineRGBABytes  = 40 << 20 // 40 MiB decoded RGBA payload
	inlineImageChunkLen = 4096
	kittyImageFormatPNG = 100
	kittyImageFormatRaw = 32

	defaultInlineImageCols = 32
	minInlineImageCols     = 20
	maxInlineImageCols     = 48
	maxInlineImageRows     = 24
)

type inlineImagePayload struct {
	format int
	width  int
	height int
	data   []byte
}

func parseSlackURL(rawURL string) (*slackURLInfo, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	// Strict host check: must be exactly slack.com or a subdomain of slack.com
	host := strings.ToLower(u.Host)
	if host != "slack.com" && !strings.HasSuffix(host, ".slack.com") {
		return nil, fmt.Errorf("not a Slack URL")
	}

	info := &slackURLInfo{}

	path := strings.Trim(u.Path, "/")
	parts := strings.Split(path, "/")

	// Find channel ID after "archives"
	for i, part := range parts {
		if part == "archives" && i+1 < len(parts) {
			info.Channel = parts[i+1]

			// Check for message timestamp (p1234567890123456)
			if i+2 < len(parts) && strings.HasPrefix(parts[i+2], "p") {
				ts := parts[i+2][1:]
				if len(ts) >= 10 {
					info.MessageTS = ts[:10] + "." + ts[10:]
				}
			}
			break
		}
	}

	// Check for thread_ts in query params
	if threadTS := u.Query().Get("thread_ts"); threadTS != "" {
		info.ThreadTS = threadTS
		info.IsThread = true
	}

	// If message has replies, it's a thread parent
	if info.MessageTS != "" && info.ThreadTS == "" {
		info.ThreadTS = info.MessageTS
	}

	if info.Channel == "" {
		return nil, fmt.Errorf("could not parse channel from URL: %s", rawURL)
	}

	return info, nil
}

func (c *ViewCmd) Run(ctx *Context) error {
	info, err := parseSlackURL(c.URL)
	if err != nil {
		return err
	}

	client, err := ctx.NewClient(c.URL)
	if err != nil {
		return err
	}
	c.resolver = slack.NewResolver(client)

	// Get channel info for context
	channel, err := client.GetConversationInfo(info.Channel)
	if err != nil {
		err = ctx.augmentChannelNotFoundError(c.URL, err)
		return fmt.Errorf("failed to get channel info: %w", err)
	}

	if c.Markdown {
		md := c.buildMarkdown(client, channel, info)
		fmt.Print(md)
		return nil
	}

	inlineImagesMode, err := normalizeInlineImagesMode(c.InlineImages)
	if err != nil {
		return err
	}

	if c.shouldRenderInlineImages(inlineImagesMode) {
		return c.renderGhosttyView(client, channel, info)
	}

	md := c.buildMarkdown(client, channel, info)
	return output.RenderMarkdown(md)
}

func normalizeInlineImagesMode(mode string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	if normalized == "" {
		normalized = "auto"
	}

	switch normalized {
	case "auto", "always", "never":
		return normalized, nil
	default:
		return "", fmt.Errorf("invalid inline image mode %q (expected auto, always, or never)", mode)
	}
}

func (c *ViewCmd) shouldRenderInlineImages(mode string) bool {
	switch mode {
	case "never":
		return false
	case "always":
		return term.IsTerminal(int(os.Stdout.Fd()))
	case "auto":
		return supportsGhosttyInlineImages()
	default:
		return false
	}
}

func (c *ViewCmd) buildMarkdown(client *slack.Client, channel *slack.Channel, info *slackURLInfo) string {
	var sb strings.Builder

	// Header
	channelName := "#" + channel.Name
	if channel.IsIM || channel.IsMPIM {
		channelName = "DM"
	}
	fmt.Fprintf(&sb, "# %s\n\n", channelName)

	if info.MessageTS != "" {
		c.buildThreadMarkdown(&sb, client, info)
	} else {
		c.buildChannelMarkdown(&sb, client, info.Channel)
	}

	return sb.String()
}

func (c *ViewCmd) buildThreadMarkdown(sb *strings.Builder, client *slack.Client, info *slackURLInfo) {
	replies, err := client.GetConversationReplies(info.Channel, info.ThreadTS, c.Limit)
	if err != nil {
		fmt.Fprintf(sb, "Error: %v\n", err)
		return
	}

	for i, msg := range replies.Messages {
		username := c.resolver.ResolveUser(msg.User)
		timestamp := c.formatTimestamp(msg.TS)
		body := c.formatMessageBody(msg)

		if i == 0 {
			fmt.Fprintf(sb, "**%s** _%s_\n\n", username, timestamp)
			if body != "" {
				fmt.Fprintf(sb, "%s\n\n", body)
			}
			if len(replies.Messages) > 1 {
				fmt.Fprintf(sb, "---\n\n**%d replies**\n\n", len(replies.Messages)-1)
			}
		} else {
			fmt.Fprintf(sb, "> **%s** _%s_\n>\n", username, timestamp)
			// Indent multiline body in blockquote.
			if body != "" {
				lines := strings.Split(body, "\n")
				for _, line := range lines {
					fmt.Fprintf(sb, "> %s\n", line)
				}
			}
			sb.WriteString("\n")
		}
	}
}

func (c *ViewCmd) buildChannelMarkdown(sb *strings.Builder, client *slack.Client, channelID string) {
	history, err := client.GetConversationHistory(channelID, c.Limit)
	if err != nil {
		fmt.Fprintf(sb, "Error: %v\n", err)
		return
	}

	// Reverse to show oldest first
	for i := len(history.Messages) - 1; i >= 0; i-- {
		msg := history.Messages[i]
		username := c.resolver.ResolveUser(msg.User)
		timestamp := c.formatTimestamp(msg.TS)
		body := c.formatMessageBody(msg)

		fmt.Fprintf(sb, "**%s** _%s_\n\n", username, timestamp)
		if body != "" {
			fmt.Fprintf(sb, "%s\n\n", body)
		}

		if msg.ReplyCount > 0 {
			fmt.Fprintf(sb, "_(%d replies)_\n\n", msg.ReplyCount)
		}
		sb.WriteString("---\n\n")
	}
}

func (c *ViewCmd) formatMessageBody(msg slack.Message) string {
	parts := make([]string, 0, 2)

	text := strings.TrimSpace(c.formatText(msg.Text))
	if text != "" {
		parts = append(parts, text)
	}

	attachments := c.formatMessageAttachments(msg)
	if attachments != "" {
		parts = append(parts, attachments)
	}

	return strings.Join(parts, "\n\n")
}

func (c *ViewCmd) formatMessageAttachments(msg slack.Message) string {
	lines := make([]string, 0, len(msg.Files)+len(msg.Attachments)+len(msg.Blocks))
	seen := make(map[string]struct{})

	addLine := func(kind, label, link string) {
		label = strings.TrimSpace(strings.ReplaceAll(label, "\n", " "))
		if label == "" {
			label = strings.ToLower(kind)
		}

		// De-duplicate repeated references that can appear in files/blocks/attachments.
		key := kind + "|" + label + "|" + link
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}

		if link != "" {
			lines = append(lines, fmt.Sprintf("- %s: [%s](%s)", kind, label, link))
			return
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", kind, label))
	}

	for _, file := range msg.Files {
		label := strings.TrimSpace(file.Title)
		if label == "" {
			label = strings.TrimSpace(file.Name)
		}
		link := strings.TrimSpace(file.Permalink)
		if link == "" {
			link = strings.TrimSpace(file.URLPrivate)
		}
		if link == "" {
			link = strings.TrimSpace(file.URLPrivateDownload)
		}

		kind := "File"
		if strings.HasPrefix(strings.ToLower(file.Mimetype), "image/") {
			kind = "Image"
		}
		addLine(kind, label, link)
	}

	for _, attachment := range msg.Attachments {
		if attachment.ImageURL == "" && attachment.TitleLink == "" {
			continue
		}
		label := attachment.Title
		if label == "" {
			label = attachment.Text
		}
		if label == "" {
			label = attachment.Fallback
		}
		link := strings.TrimSpace(attachment.ImageURL)
		kind := "Image"
		if link == "" {
			link = strings.TrimSpace(attachment.TitleLink)
			kind = "File"
		}
		addLine(kind, label, link)
	}

	for _, block := range msg.Blocks {
		if block.Type != "image" || block.ImageURL == "" {
			continue
		}
		label := block.AltText
		if label == "" && block.Title != nil {
			label = block.Title.Text
		}
		addLine("Image", label, strings.TrimSpace(block.ImageURL))
	}

	if len(lines) == 0 {
		return ""
	}
	return "**Attachments**\n" + strings.Join(lines, "\n")
}

func (c *ViewCmd) formatText(text string) string {
	if c.Raw {
		return text
	}
	return c.resolver.FormatText(text)
}

func (c *ViewCmd) formatTimestamp(ts string) string {
	parts := strings.Split(ts, ".")
	if len(parts) == 0 {
		return ts
	}

	var sec int64
	_, _ = fmt.Sscanf(parts[0], "%d", &sec)
	t := time.Unix(sec, 0)

	now := time.Now()
	if t.Year() == now.Year() && t.YearDay() == now.YearDay() {
		return t.Format("3:04 PM")
	}
	if t.Year() == now.Year() {
		return t.Format("Jan 2, 3:04 PM")
	}
	return t.Format("Jan 2, 2006 3:04 PM")
}

func (c *ViewCmd) renderGhosttyView(client *slack.Client, channel *slack.Channel, info *slackURLInfo) error {
	channelName := "#" + channel.Name
	if channel.IsIM || channel.IsMPIM {
		channelName = "DM"
	}
	if err := output.RenderMarkdown(fmt.Sprintf("# %s\n\n", channelName)); err != nil {
		return err
	}

	if info.MessageTS != "" {
		return c.renderGhosttyThread(client, info)
	}
	return c.renderGhosttyChannel(client, info.Channel)
}

func (c *ViewCmd) renderGhosttyThread(client *slack.Client, info *slackURLInfo) error {
	replies, err := client.GetConversationReplies(info.Channel, info.ThreadTS, c.Limit)
	if err != nil {
		return fmt.Errorf("failed to get thread: %w", err)
	}

	for i, msg := range replies.Messages {
		var sb strings.Builder

		username := c.resolver.ResolveUser(msg.User)
		timestamp := c.formatTimestamp(msg.TS)
		body := c.formatMessageBody(msg)

		if i == 0 {
			fmt.Fprintf(&sb, "**%s** _%s_\n\n", username, timestamp)
			if body != "" {
				fmt.Fprintf(&sb, "%s\n\n", body)
			}
		} else {
			fmt.Fprintf(&sb, "> **%s** _%s_\n>\n", username, timestamp)
			if body != "" {
				lines := strings.Split(body, "\n")
				for _, line := range lines {
					fmt.Fprintf(&sb, "> %s\n", line)
				}
			}
			sb.WriteString("\n")
		}

		if err := output.RenderMarkdown(sb.String()); err != nil {
			return err
		}
		c.renderInlineImages(client, c.messageInlineImageURLs(msg))

		if i == 0 && len(replies.Messages) > 1 {
			if err := output.RenderMarkdown(fmt.Sprintf("---\n\n**%d replies**\n\n", len(replies.Messages)-1)); err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *ViewCmd) renderGhosttyChannel(client *slack.Client, channelID string) error {
	history, err := client.GetConversationHistory(channelID, c.Limit)
	if err != nil {
		return fmt.Errorf("failed to get channel history: %w", err)
	}

	for i := len(history.Messages) - 1; i >= 0; i-- {
		msg := history.Messages[i]
		username := c.resolver.ResolveUser(msg.User)
		timestamp := c.formatTimestamp(msg.TS)
		body := c.formatMessageBody(msg)

		var sb strings.Builder
		fmt.Fprintf(&sb, "**%s** _%s_\n\n", username, timestamp)
		if body != "" {
			fmt.Fprintf(&sb, "%s\n\n", body)
		}
		if msg.ReplyCount > 0 {
			fmt.Fprintf(&sb, "_(%d replies)_\n\n", msg.ReplyCount)
		}

		if err := output.RenderMarkdown(sb.String()); err != nil {
			return err
		}
		c.renderInlineImages(client, c.messageInlineImageURLs(msg))

		if err := output.RenderMarkdown("---\n\n"); err != nil {
			return err
		}
	}

	return nil
}

func supportsGhosttyInlineImages() bool {
	termProgram := strings.ToLower(strings.TrimSpace(os.Getenv("TERM_PROGRAM")))
	termName := strings.ToLower(strings.TrimSpace(os.Getenv("TERM")))

	isGhostty := termProgram == "ghostty" || strings.Contains(termName, "ghostty")
	if !isGhostty {
		return false
	}

	return term.IsTerminal(int(os.Stdout.Fd()))
}

func (c *ViewCmd) renderInlineImages(client *slack.Client, imageURLs []string) {
	if len(imageURLs) == 0 {
		return
	}

	printed := false
	for _, imageURL := range imageURLs {
		if err := c.renderInlineImage(client, imageURL); err != nil {
			continue
		}
		printed = true
	}

	if printed {
		fmt.Println()
	}
}

func (c *ViewCmd) messageInlineImageURLs(msg slack.Message) []string {
	seen := make(map[string]struct{})
	links := []string{}

	add := func(link string) {
		link = strings.TrimSpace(link)
		if link == "" {
			return
		}
		if !isSlackHostedURL(link) {
			return
		}
		if _, ok := seen[link]; ok {
			return
		}
		seen[link] = struct{}{}
		links = append(links, link)
	}

	for _, file := range msg.Files {
		if !isImageFile(file) {
			continue
		}

		link := strings.TrimSpace(file.URLPrivate)
		if link == "" {
			link = strings.TrimSpace(file.URLPrivateDownload)
		}
		if link == "" {
			link = strings.TrimSpace(file.Permalink)
		}
		add(link)
	}

	for _, attachment := range msg.Attachments {
		add(attachment.ImageURL)
	}

	for _, block := range msg.Blocks {
		if block.Type != "image" {
			continue
		}
		add(block.ImageURL)
	}

	return links
}

func isImageFile(file slack.File) bool {
	mimeType := strings.ToLower(strings.TrimSpace(file.Mimetype))
	if strings.HasPrefix(mimeType, "image/") {
		return true
	}

	switch strings.ToLower(strings.TrimSpace(file.Filetype)) {
	case "png", "jpg", "jpeg", "gif":
		return true
	default:
		return false
	}
}

func isSlackHostedURL(rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}

	scheme := strings.ToLower(strings.TrimSpace(u.Scheme))
	if scheme != "https" {
		return false
	}

	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host == "" {
		return false
	}
	return host == "slack.com" || strings.HasSuffix(host, ".slack.com")
}

func contentMediaType(contentType string) string {
	return strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
}

func validateInlineImageDimensions(width, height int) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("image has invalid dimensions")
	}

	decodedSize := int64(width) * int64(height) * 4
	if decodedSize <= 0 || decodedSize > maxInlineRGBABytes {
		return fmt.Errorf("decoded image exceeds limit (%d bytes)", maxInlineRGBABytes)
	}

	return nil
}

func decodeInlineImageAsRGBA(imageData []byte) ([]byte, int, int, error) {
	width, height, err := imageDimensions(imageData)
	if err != nil {
		return nil, 0, 0, err
	}
	if err := validateInlineImageDimensions(width, height); err != nil {
		return nil, 0, 0, err
	}

	img, _, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		return nil, 0, 0, err
	}

	bounds := img.Bounds()
	decodedWidth := bounds.Dx()
	decodedHeight := bounds.Dy()
	if err := validateInlineImageDimensions(decodedWidth, decodedHeight); err != nil {
		return nil, 0, 0, err
	}

	rgba := image.NewRGBA(image.Rect(0, 0, decodedWidth, decodedHeight))
	draw.Draw(rgba, rgba.Bounds(), img, bounds.Min, draw.Src)

	return rgba.Pix, decodedWidth, decodedHeight, nil
}

func imageDimensions(imageData []byte) (int, int, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(imageData))
	if err != nil {
		return 0, 0, err
	}

	if config.Width <= 0 || config.Height <= 0 {
		return 0, 0, fmt.Errorf("image has invalid dimensions")
	}

	return config.Width, config.Height, nil
}

func buildInlineImagePayload(imageData []byte, contentType string) (*inlineImagePayload, error) {
	if len(imageData) == 0 {
		return nil, fmt.Errorf("image download was empty")
	}

	mediaType := contentMediaType(contentType)
	if mediaType == "" {
		mediaType = contentMediaType(http.DetectContentType(imageData))
	}

	if mediaType != "" && !strings.HasPrefix(mediaType, "image/") {
		return nil, fmt.Errorf("not an image content type: %s", mediaType)
	}

	if mediaType == "image/png" {
		width, height, err := imageDimensions(imageData)
		if err != nil {
			return nil, fmt.Errorf("unsupported inline image format %q: %w", mediaType, err)
		}
		if err := validateInlineImageDimensions(width, height); err != nil {
			return nil, err
		}

		return &inlineImagePayload{
			format: kittyImageFormatPNG,
			width:  width,
			height: height,
			data:   imageData,
		}, nil
	}

	rgbaData, width, height, err := decodeInlineImageAsRGBA(imageData)
	if err != nil {
		if mediaType != "" {
			return nil, fmt.Errorf("unsupported inline image format %q: %w", mediaType, err)
		}
		return nil, fmt.Errorf("unsupported inline image format: %w", err)
	}

	return &inlineImagePayload{
		format: kittyImageFormatRaw,
		width:  width,
		height: height,
		data:   rgbaData,
	}, nil
}

func inlineImageColumnsForTerminalWidth(width int) int {
	if width <= 0 {
		return defaultInlineImageCols
	}

	cols := width / 3
	if cols < minInlineImageCols {
		cols = minInlineImageCols
	}
	if cols > maxInlineImageCols {
		cols = maxInlineImageCols
	}

	return cols
}

func inlineImageRowsForPayload(payload *inlineImagePayload, cols int) int {
	if payload == nil || payload.width <= 0 || payload.height <= 0 || cols <= 0 {
		return 0
	}

	rows := int(math.Round(float64(payload.height) * float64(cols) / (float64(payload.width) * 2.0)))
	if rows < 1 {
		rows = 1
	}
	if rows > maxInlineImageRows {
		rows = maxInlineImageRows
	}

	return rows
}

func firstInlineImageChunkParams(payload *inlineImagePayload, cols, rows, hasMore int) string {
	if payload.format == kittyImageFormatRaw {
		if rows > 0 {
			return fmt.Sprintf("a=T,f=%d,s=%d,v=%d,c=%d,r=%d,m=%d", payload.format, payload.width, payload.height, cols, rows, hasMore)
		}
		return fmt.Sprintf("a=T,f=%d,s=%d,v=%d,c=%d,m=%d", payload.format, payload.width, payload.height, cols, hasMore)
	}

	if rows > 0 {
		return fmt.Sprintf("a=T,f=%d,c=%d,r=%d,m=%d", payload.format, cols, rows, hasMore)
	}

	return fmt.Sprintf("a=T,f=%d,c=%d,m=%d", payload.format, cols, hasMore)
}

func (c *ViewCmd) renderInlineImage(client *slack.Client, imageURL string) error {
	imageData, contentType, err := client.DownloadPrivateFile(imageURL, maxInlineImageBytes)
	if err != nil {
		return err
	}

	payload, err := buildInlineImagePayload(imageData, contentType)
	if err != nil {
		return err
	}

	cols := inlineImageColumnsForTerminalWidth(0)
	if width, _, sizeErr := term.GetSize(int(os.Stdout.Fd())); sizeErr == nil && width > 0 {
		cols = inlineImageColumnsForTerminalWidth(width)
	}
	rows := inlineImageRowsForPayload(payload, cols)

	encoded := base64.StdEncoding.EncodeToString(payload.data)
	firstChunk := true

	for len(encoded) > 0 {
		chunkLen := inlineImageChunkLen
		if chunkLen > len(encoded) {
			chunkLen = len(encoded)
		}
		chunk := encoded[:chunkLen]
		encoded = encoded[chunkLen:]

		hasMore := 0
		if len(encoded) > 0 {
			hasMore = 1
		}

		var params string
		if firstChunk {
			params = firstInlineImageChunkParams(payload, cols, rows, hasMore)
			firstChunk = false
		} else {
			params = fmt.Sprintf("m=%d", hasMore)
		}

		_, _ = fmt.Printf("\x1b_G%s;%s\x1b\\", params, chunk)
	}

	fmt.Println()
	return nil
}
