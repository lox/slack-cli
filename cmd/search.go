package cmd

import (
	"fmt"

	"github.com/lox/slack-cli/internal/slack"
)

type SearchCmd struct {
	Query string `arg:"" help:"Search query (supports Slack search syntax: from:@user, in:#channel, etc.)"`
	Limit int    `help:"Maximum number of results" default:"20"`
}

func (c *SearchCmd) Run(ctx *Context) error {
	if ctx.Config.Token == "" {
		return fmt.Errorf("not logged in. Run 'slack auth login' first")
	}

	client := slack.NewClient(ctx.Config.Token)
	resp, err := client.SearchMessages(c.Query, c.Limit)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	if resp.Messages.Total == 0 {
		fmt.Println("No messages found.")
		return nil
	}

	fmt.Printf("Found %d messages:\n\n", resp.Messages.Total)

	for _, match := range resp.Messages.Matches {
		channel := match.Channel.Name
		if channel == "" {
			channel = match.Channel.ID
		}
		fmt.Printf("#%s [%s]\n", channel, match.TS)
		fmt.Printf("  %s: %s\n", match.Username, match.Text)
		if match.Permalink != "" {
			fmt.Printf("  %s\n", match.Permalink)
		}
		fmt.Println()
	}

	return nil
}
