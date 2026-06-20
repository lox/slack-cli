package cmd

import (
	"fmt"

	"github.com/lox/slack-cli/internal/output"
	"github.com/lox/slack-cli/internal/slack"
)

type SearchCmd struct {
	Query string `arg:"" help:"Search query (supports Slack search syntax: from:@user, in:#channel, etc.)"`
	Limit int    `help:"Maximum number of results" default:"20"`
	JSON  bool   `help:"Output as pretty JSON array" short:"j" xor:"format"`
	JSONL bool   `help:"Output as JSON Lines, one match per line" xor:"format"`
}

func (c *SearchCmd) Run(ctx *Context) error {
	client, err := ctx.NewClient("")
	if err != nil {
		return err
	}
	resolver := slack.NewResolver(client)
	// Preload the channel list so per-match channel.type population on
	// the JSON path does not fan out to N+1 conversations.info requests.
	if c.JSON || c.JSONL {
		if err := resolver.PreloadChannels("public_channel,private_channel,mpim,im"); err != nil {
			resolver.DisableChannelInfoLookup()
		}
	}
	resp, err := client.SearchMessages(c.Query, c.Limit)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	if c.JSON || c.JSONL {
		return c.emitStructured(resolver, resp)
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
		fmt.Printf("  %s: %s\n", match.Username, resolver.FormatText(match.Text))
		if match.Permalink != "" {
			fmt.Printf("  %s\n", match.Permalink)
		}
		fmt.Println()
	}

	return nil
}

func (c *SearchCmd) emitStructured(resolver *slack.Resolver, resp *slack.SearchResponse) error {
	if c.JSONL {
		i := 0
		return output.EmitJSONLStream(func() (output.Message, bool, error) {
			if i >= len(resp.Messages.Matches) {
				return output.Message{}, false, nil
			}
			m := searchMatchToMessage(resolver, resp.Messages.Matches[i])
			i++
			return m, true, nil
		})
	}

	records := make([]output.Message, 0, len(resp.Messages.Matches))
	for _, match := range resp.Messages.Matches {
		records = append(records, searchMatchToMessage(resolver, match))
	}
	return output.EmitJSON(records)
}

func searchMatchToMessage(resolver *slack.Resolver, match slack.SearchMatch) output.Message {
	var workspace string
	if match.Permalink != "" {
		host, _, err := slack.ExtractWorkspaceRef(match.Permalink)
		if err == nil {
			workspace = host
		}
	}

	display := match.Username
	if display == "" && match.User != "" && resolver != nil {
		display = resolver.ResolveUser(match.User)
	}

	text := match.Text
	if resolver != nil {
		text = resolver.FormatText(match.Text)
	}

	rec := output.Message{
		TS:        match.TS,
		Type:      match.Type,
		Subtype:   match.Subtype,
		User:      display,
		UserID:    match.User,
		Text:      text,
		TextRaw:   match.Text,
		Channel:   output.ChannelRefFromID(resolver, match.Channel.ID, match.Channel.Name),
		Workspace: workspace,
		Permalink: match.Permalink,
	}
	return rec
}
