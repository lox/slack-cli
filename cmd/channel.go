package cmd

import (
	"fmt"
	"strings"

	"github.com/lox/slack-cli/internal/slack"
)

type ChannelCmd struct {
	List ChannelListCmd `cmd:"" help:"List channels you're a member of"`
	Read ChannelReadCmd `cmd:"" help:"Read recent messages from a channel"`
	Info ChannelInfoCmd `cmd:"" help:"Show channel information"`
}

type ChannelListCmd struct {
	Limit int `help:"Maximum number of channels to list" default:"100"`
}

func (c *ChannelListCmd) Run(ctx *Context) error {
	if ctx.Config.Token == "" {
		return fmt.Errorf("not logged in. Run 'slack auth login' first")
	}

	client := slack.NewClient(ctx.Config.Token)
	resp, err := client.ListConversations("public_channel,private_channel", c.Limit)
	if err != nil {
		return fmt.Errorf("failed to list channels: %w", err)
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
	Channel string `arg:"" help:"Channel name or ID"`
	Limit   int    `help:"Number of messages to show" default:"20"`
}

func (c *ChannelReadCmd) Run(ctx *Context) error {
	if ctx.Config.Token == "" {
		return fmt.Errorf("not logged in. Run 'slack auth login' first")
	}

	client := slack.NewClient(ctx.Config.Token)

	// Resolve channel name to ID if needed
	channelID := c.Channel
	if strings.HasPrefix(c.Channel, "#") {
		channelID = strings.TrimPrefix(c.Channel, "#")
	}
	if !strings.HasPrefix(channelID, "C") && !strings.HasPrefix(channelID, "G") {
		// Try to find by name
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

	history, err := client.GetConversationHistory(channelID, c.Limit)
	if err != nil {
		return fmt.Errorf("failed to get channel history: %w", err)
	}

	// Print messages in reverse order (oldest first)
	for i := len(history.Messages) - 1; i >= 0; i-- {
		msg := history.Messages[i]
		user := msg.User
		if user == "" {
			user = "bot"
		}
		fmt.Printf("[%s] %s: %s\n", msg.TS, user, msg.Text)
	}

	return nil
}

type ChannelInfoCmd struct {
	Channel string `arg:"" help:"Channel name or ID"`
}

func (c *ChannelInfoCmd) Run(ctx *Context) error {
	if ctx.Config.Token == "" {
		return fmt.Errorf("not logged in. Run 'slack auth login' first")
	}

	client := slack.NewClient(ctx.Config.Token)

	channelID := strings.TrimPrefix(c.Channel, "#")

	info, err := client.GetConversationInfo(channelID)
	if err != nil {
		return fmt.Errorf("failed to get channel info: %w", err)
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
