package cmd

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/enescakir/emoji"
	"github.com/lox/slack-cli/internal/output"
	"github.com/lox/slack-cli/internal/slack"
)

type ViewCmd struct {
	URL      string `arg:"" help:"Slack URL (message, thread, or channel)"`
	Markdown bool   `help:"Output as markdown instead of terminal formatting" short:"m"`
	Limit    int    `help:"Maximum messages to show for channels/threads" default:"20"`
	Raw      bool   `help:"Don't resolve user/channel mentions" short:"r"`
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

	if !strings.Contains(u.Host, "slack.com") {
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
	if ctx.Config.Token == "" {
		return fmt.Errorf("not logged in. Run 'slack auth login' first")
	}

	info, err := parseSlackURL(c.URL)
	if err != nil {
		return err
	}

	client := slack.NewClient(ctx.Config.Token)

	// Get channel info for context
	channel, err := client.GetConversationInfo(info.Channel)
	if err != nil {
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
	sb.WriteString(fmt.Sprintf("# %s\n\n", channelName))

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
		sb.WriteString(fmt.Sprintf("Error: %v\n", err))
		return
	}

	for i, msg := range replies.Messages {
		username := c.resolveUser(client, msg.User)
		timestamp := c.formatTimestamp(msg.TS)
		text := c.formatText(client, msg.Text)

		if i == 0 {
			sb.WriteString(fmt.Sprintf("**%s** _%s_\n\n", username, timestamp))
			sb.WriteString(fmt.Sprintf("%s\n\n", text))
			if len(replies.Messages) > 1 {
				sb.WriteString(fmt.Sprintf("---\n\n**%d replies**\n\n", len(replies.Messages)-1))
			}
		} else {
			sb.WriteString(fmt.Sprintf("> **%s** _%s_\n>\n", username, timestamp))
			// Indent multiline text in blockquote
			lines := strings.Split(text, "\n")
			for _, line := range lines {
				sb.WriteString(fmt.Sprintf("> %s\n", line))
			}
			sb.WriteString("\n")
		}
	}
}

func (c *ViewCmd) buildChannelMarkdown(sb *strings.Builder, client *slack.Client, channelID string) {
	history, err := client.GetConversationHistory(channelID, c.Limit)
	if err != nil {
		sb.WriteString(fmt.Sprintf("Error: %v\n", err))
		return
	}

	// Reverse to show oldest first
	for i := len(history.Messages) - 1; i >= 0; i-- {
		msg := history.Messages[i]
		username := c.resolveUser(client, msg.User)
		timestamp := c.formatTimestamp(msg.TS)
		text := c.formatText(client, msg.Text)

		sb.WriteString(fmt.Sprintf("**%s** _%s_\n\n", username, timestamp))
		sb.WriteString(fmt.Sprintf("%s\n\n", text))

		if msg.ReplyCount > 0 {
			sb.WriteString(fmt.Sprintf("_(%d replies)_\n\n", msg.ReplyCount))
		}
		sb.WriteString("---\n\n")
	}
}

var userCache = make(map[string]string)
var channelCache = make(map[string]string)

func (c *ViewCmd) formatText(client *slack.Client, text string) string {
	if c.Raw {
		return text
	}

	// Resolve user mentions: <@U123ABC> -> @username
	result := text
	for {
		start := strings.Index(result, "<@")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], ">")
		if end == -1 {
			break
		}
		end += start

		mention := result[start : end+1]
		userID := result[start+2 : end]

		// Handle <@U123|name> format
		if pipeIdx := strings.Index(userID, "|"); pipeIdx != -1 {
			userID = userID[:pipeIdx]
		}

		displayName := c.resolveUser(client, userID)
		result = strings.Replace(result, mention, "@"+displayName, 1)
	}

	// Resolve channel mentions: <#C123ABC|channel-name> -> #channel-name
	for {
		start := strings.Index(result, "<#")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], ">")
		if end == -1 {
			break
		}
		end += start

		mention := result[start : end+1]
		channelPart := result[start+2 : end]

		var channelName string
		if pipeIdx := strings.Index(channelPart, "|"); pipeIdx != -1 {
			channelName = channelPart[pipeIdx+1:]
		} else {
			channelName = c.resolveChannel(client, channelPart)
		}

		result = strings.Replace(result, mention, "#"+channelName, 1)
	}

	// Resolve URL links: <http://example.com|label> -> label (http://example.com)
	for {
		start := strings.Index(result, "<http")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], ">")
		if end == -1 {
			break
		}
		end += start

		link := result[start : end+1]
		linkContent := result[start+1 : end]

		if pipeIdx := strings.Index(linkContent, "|"); pipeIdx != -1 {
			url := linkContent[:pipeIdx]
			label := linkContent[pipeIdx+1:]
			result = strings.Replace(result, link, label+" ("+url+")", 1)
		} else {
			result = strings.Replace(result, link, linkContent, 1)
		}
	}

	// Convert emoji shortcodes: :smile: -> 😊
	result = emoji.Parse(result)

	return result
}

func (c *ViewCmd) resolveChannel(client *slack.Client, channelID string) string {
	if name, ok := channelCache[channelID]; ok {
		return name
	}

	channel, err := client.GetConversationInfo(channelID)
	if err != nil {
		channelCache[channelID] = channelID
		return channelID
	}

	channelCache[channelID] = channel.Name
	return channel.Name
}

func (c *ViewCmd) resolveUser(client *slack.Client, userID string) string {
	if userID == "" {
		return "bot"
	}

	if name, ok := userCache[userID]; ok {
		return name
	}

	user, err := client.GetUserInfo(userID)
	if err != nil {
		userCache[userID] = userID
		return userID
	}

	name := user.Profile.DisplayName
	if name == "" {
		name = user.RealName
	}
	if name == "" {
		name = user.Name
	}

	userCache[userID] = name
	return name
}

func (c *ViewCmd) formatTimestamp(ts string) string {
	parts := strings.Split(ts, ".")
	if len(parts) == 0 {
		return ts
	}

	var sec int64
	fmt.Sscanf(parts[0], "%d", &sec)
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
