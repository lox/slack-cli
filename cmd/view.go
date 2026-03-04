package cmd

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/lox/slack-cli/internal/output"
	"github.com/lox/slack-cli/internal/slack"
)

type ViewCmd struct {
	URL      string `arg:"" help:"Slack URL (message, thread, or channel)"`
	Markdown bool   `help:"Output as markdown instead of terminal formatting" short:"m"`
	Limit    int    `help:"Maximum messages to show for channels/threads" default:"20"`
	Raw      bool   `help:"Don't resolve user/channel mentions" short:"r"`

	resolver *slack.Resolver
}

type slackURLInfo struct {
	Channel   string
	MessageTS string
	ThreadTS  string
	IsThread  bool
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

	// Build markdown content, then render appropriately
	md := c.buildMarkdown(client, channel, info)

	if c.Markdown {
		fmt.Print(md)
		return nil
	}
	return output.RenderMarkdown(md)
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
		text := c.formatText(msg.Text)

		if i == 0 {
			fmt.Fprintf(sb, "**%s** _%s_\n\n", username, timestamp)
			fmt.Fprintf(sb, "%s\n\n", text)
			if len(replies.Messages) > 1 {
				fmt.Fprintf(sb, "---\n\n**%d replies**\n\n", len(replies.Messages)-1)
			}
		} else {
			fmt.Fprintf(sb, "> **%s** _%s_\n>\n", username, timestamp)
			// Indent multiline text in blockquote
			lines := strings.Split(text, "\n")
			for _, line := range lines {
				fmt.Fprintf(sb, "> %s\n", line)
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
		text := c.formatText(msg.Text)

		fmt.Fprintf(sb, "**%s** _%s_\n\n", username, timestamp)
		fmt.Fprintf(sb, "%s\n\n", text)

		if msg.ReplyCount > 0 {
			fmt.Fprintf(sb, "_(%d replies)_\n\n", msg.ReplyCount)
		}
		sb.WriteString("---\n\n")
	}
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
