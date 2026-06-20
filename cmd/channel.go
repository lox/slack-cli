package cmd

import (
	"fmt"
	"slices"
	"strings"

	"github.com/lox/slack-cli/internal/output"
	"github.com/lox/slack-cli/internal/slack"
)

type ChannelCmd struct {
	List ChannelListCmd `cmd:"" help:"List channels you're a member of"`
	Read ChannelReadCmd `cmd:"" help:"Read recent messages from a channel"`
	Info ChannelInfoCmd `cmd:"" help:"Show channel information"`
}

type ChannelListCmd struct {
	Limit int  `help:"Maximum number of channels to list" default:"100"`
	JSON  bool `help:"Output as pretty JSON array" short:"j" xor:"format"`
	JSONL bool `help:"Output as JSON Lines, one channel per line" xor:"format"`
}

func (c *ChannelListCmd) Run(ctx *Context) error {
	client, err := ctx.NewClient("")
	if err != nil {
		return err
	}
	resp, err := client.ListConversations("public_channel,private_channel", c.Limit)
	if err != nil {
		return fmt.Errorf("failed to list channels: %w", err)
	}

	if c.JSONL {
		i := 0
		return output.EmitJSONLStream(func() (output.Channel, bool, error) {
			if i >= len(resp.Channels) {
				return output.Channel{}, false, nil
			}
			ch := output.ToChannel(resp.Channels[i])
			i++
			return ch, true, nil
		})
	}

	if c.JSON {
		records := make([]output.Channel, 0, len(resp.Channels))
		for _, ch := range resp.Channels {
			records = append(records, output.ToChannel(ch))
		}
		return output.EmitJSON(records)
	}

	for _, ch := range resp.Channels {
		prefix := "#"
		if ch.IsPrivate {
			prefix = "🔒"
		}
		fmt.Printf("%s%s (%d members) - %s\n", prefix, ch.Name, ch.NumMembers, ch.Purpose.Value)
	}

	return nil
}

type ChannelReadCmd struct {
	Channel  string `arg:"" help:"Channel name, ID, or Slack URL"`
	Limit    int    `help:"Number of messages to show" default:"20"`
	Markdown bool   `help:"Output as markdown" short:"m" xor:"format"`
	JSON     bool   `help:"Output as pretty JSON array, oldest first" short:"j" xor:"format"`
	JSONL    bool   `help:"Output as JSON Lines, oldest first" xor:"format"`
}

func (c *ChannelReadCmd) Run(ctx *Context) error {
	channelID, urlHint, err := parseChannelReference(c.Channel)
	if err != nil {
		return err
	}

	client, err := ctx.NewClient(urlHint)
	if err != nil {
		return err
	}
	resolver := slack.NewResolver(client)

	channelName := ""
	// Resolve channel name to ID if needed
	if !isSlackChannelID(channelID) {
		// Try to find by name
		resp, err := client.ListConversations("public_channel,private_channel", 1000)
		if err != nil {
			return fmt.Errorf("failed to list channels: %w", err)
		}
		for _, ch := range resp.Channels {
			if ch.Name == channelID {
				channelName = ch.Name
				channelID = ch.ID
				break
			}
		}
	}

	history, err := client.GetConversationHistory(channelID, c.Limit)
	if err != nil {
		err = ctx.augmentChannelNotFoundError(urlHint, err)
		err = ctx.augmentCrossWorkspaceChannelHint(urlHint, err)
		return fmt.Errorf("failed to get channel history: %w", err)
	}

	if c.JSON || c.JSONL {
		chRef := output.ChannelRefFromID(resolver, channelID, channelName)
		conv := output.MessageConverter{Resolver: resolver, Channel: chRef}
		ordered := slices.Clone(history.Messages)
		slices.Reverse(ordered)
		if c.JSONL {
			i := 0
			return output.EmitJSONLStream(func() (output.Message, bool, error) {
				if i >= len(ordered) {
					return output.Message{}, false, nil
				}
				m := conv.Convert(ordered[i])
				i++
				return m, true, nil
			})
		}
		return output.EmitJSON(conv.ConvertAll(ordered))
	}

	if c.Markdown {
		fmt.Print(c.formatHistoryAsMarkdown(history.Messages, resolver))
		return nil
	}

	// Print messages in reverse order (oldest first)
	for i := len(history.Messages) - 1; i >= 0; i-- {
		msg := history.Messages[i]
		user := resolver.ResolveUser(msg.User)
		fmt.Printf("[%s] %s: %s\n", msg.TS, user, resolver.FormatText(msg.BodyText()))
	}

	return nil
}

func (c *ChannelReadCmd) formatHistoryAsMarkdown(messages []slack.Message, resolver *slack.Resolver) string {
	var sb strings.Builder

	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		username := resolver.ResolveUser(msg.User)
		text := resolver.FormatText(msg.BodyText())

		fmt.Fprintf(&sb, "**%s** _%s_\n\n", username, msg.TS)
		fmt.Fprintf(&sb, "%s\n\n", text)
		if msg.ReplyCount > 0 {
			fmt.Fprintf(&sb, "_(%d replies)_\n\n", msg.ReplyCount)
		}
		if i > 0 {
			sb.WriteString("---\n\n")
		}
	}

	return sb.String()
}

type ChannelInfoCmd struct {
	Channel string `arg:"" help:"Channel name, ID, or Slack URL"`
	JSON    bool   `help:"Output as pretty JSON object" short:"j" xor:"format"`
	JSONL   bool   `help:"Output as a single JSON Lines record" xor:"format"`
}

func (c *ChannelInfoCmd) Run(ctx *Context) error {
	channelID, urlHint, err := parseChannelReference(c.Channel)
	if err != nil {
		return err
	}

	client, err := ctx.NewClient(urlHint)
	if err != nil {
		return err
	}

	if !isSlackChannelID(channelID) {
		resp, err := client.ListConversations("public_channel,private_channel", 1000)
		if err != nil {
			return fmt.Errorf("failed to list channels: %w", err)
		}
		for _, ch := range resp.Channels {
			if ch.Name == channelID {
				channelID = ch.ID
				break
			}
		}
	}

	info, err := client.GetConversationInfo(channelID)
	if err != nil {
		err = ctx.augmentChannelNotFoundError(urlHint, err)
		err = ctx.augmentCrossWorkspaceChannelHint(urlHint, err)
		return fmt.Errorf("failed to get channel info: %w", err)
	}

	rec := output.ToChannel(*info)
	if c.JSONL {
		return output.EmitJSONL([]output.Channel{rec})
	}
	if c.JSON {
		return output.EmitJSON(rec)
	}

	fmt.Printf("Name: #%s\n", info.Name)
	fmt.Printf("ID: %s\n", info.ID)
	fmt.Printf("Members: %d\n", info.NumMembers)
	fmt.Printf("Private: %v\n", info.IsPrivate)
	if info.Topic.Value != "" {
		fmt.Printf("Topic: %s\n", info.Topic.Value)
	}
	if info.Purpose.Value != "" {
		fmt.Printf("Purpose: %s\n", info.Purpose.Value)
	}

	return nil
}

func parseChannelReference(channel string) (channelID string, urlHint string, err error) {
	trimmed := strings.TrimSpace(channel)
	if trimmed == "" {
		return "", "", fmt.Errorf("channel is required")
	}

	if strings.HasPrefix(trimmed, "https://") || strings.HasPrefix(trimmed, "http://") {
		info, parseErr := parseSlackURL(trimmed)
		if parseErr != nil {
			return "", "", fmt.Errorf("failed to parse channel URL: %w", parseErr)
		}
		return info.Channel, trimmed, nil
	}

	return strings.TrimPrefix(trimmed, "#"), "", nil
}

func isSlackChannelID(channelID string) bool {
	return strings.HasPrefix(channelID, "C") || strings.HasPrefix(channelID, "G") || strings.HasPrefix(channelID, "D")
}
