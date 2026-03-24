package cmd

import (
	"fmt"
	"strings"

	"github.com/lox/slack-cli/internal/slack"
)

type ThreadCmd struct {
	Read ThreadReadCmd `cmd:"" help:"Read a thread by URL or channel+timestamp"`
}

type ThreadReadCmd struct {
	URL       string `arg:"" optional:"" help:"Thread URL (e.g., https://workspace.slack.com/archives/C123/p1234567890)"`
	Channel   string `help:"Channel ID" short:"c"`
	Timestamp string `help:"Thread timestamp" short:"t"`
	Limit     int    `help:"Maximum number of replies" default:"100"`
	Markdown  bool   `help:"Output as markdown" short:"m"`
}

func (c *ThreadReadCmd) Run(ctx *Context) error {
	var channelID, threadTS string
	var err error

	if c.URL != "" {
		channelID, threadTS, err = slack.ParseThreadURL(c.URL)
		if err != nil {
			return fmt.Errorf("failed to parse thread URL: %w", err)
		}
	} else if c.Channel != "" && c.Timestamp != "" {
		channelID = c.Channel
		threadTS = c.Timestamp
	} else {
		return fmt.Errorf("provide either a thread URL or --channel and --timestamp")
	}

	client, err := ctx.NewClient(c.URL)
	if err != nil {
		return err
	}
	resolver := slack.NewResolver(client)

	replies, err := client.GetConversationReplies(channelID, threadTS, c.Limit)
	if err != nil {
		err = c.augmentReadError(ctx, err)
		return fmt.Errorf("failed to get thread: %w", err)
	}

	if c.Markdown {
		fmt.Print(c.formatRepliesAsMarkdown(replies.Messages, resolver))
		return nil
	}

	for _, msg := range replies.Messages {
		user := resolver.ResolveUser(msg.User)
		fmt.Printf("[%s] %s: %s\n", msg.TS, user, resolver.FormatText(msg.Text))
	}

	return nil
}

func (c *ThreadReadCmd) augmentReadError(ctx *Context, err error) error {
	err = ctx.augmentChannelNotFoundError(c.URL, err)
	err = ctx.augmentCrossWorkspaceChannelHint(c.URL, err)
	return err
}

func (c *ThreadReadCmd) formatRepliesAsMarkdown(messages []slack.Message, resolver *slack.Resolver) string {
	var sb strings.Builder

	for i, msg := range messages {
		username := resolver.ResolveUser(msg.User)
		text := resolver.FormatText(msg.Text)

		if i == 0 {
			fmt.Fprintf(&sb, "**%s** _%s_\n\n", username, msg.TS)
			fmt.Fprintf(&sb, "%s\n\n", text)
			if len(messages) > 1 {
				fmt.Fprintf(&sb, "---\n\n**%d replies**\n\n", len(messages)-1)
			}
			continue
		}

		fmt.Fprintf(&sb, "> **%s** _%s_\n>\n", username, msg.TS)
		for _, line := range strings.Split(text, "\n") {
			fmt.Fprintf(&sb, "> %s\n", line)
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
