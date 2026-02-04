package cmd

import (
	"fmt"

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
}

func (c *ThreadReadCmd) Run(ctx *Context) error {
	if ctx.Config.Token == "" {
		return fmt.Errorf("not logged in. Run 'slack auth login' first")
	}

	client := slack.NewClient(ctx.Config.Token)

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

	replies, err := client.GetConversationReplies(channelID, threadTS, c.Limit)
	if err != nil {
		return fmt.Errorf("failed to get thread: %w", err)
	}

	for _, msg := range replies.Messages {
		user := msg.User
		if user == "" {
			user = "bot"
		}
		fmt.Printf("[%s] %s: %s\n", msg.TS, user, msg.Text)
	}

	return nil
}
