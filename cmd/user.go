package cmd

import (
	"fmt"

	"github.com/lox/slack-cli/internal/slack"
)

type UserCmd struct {
	List UserListCmd `cmd:"" help:"List users in the workspace"`
	Info UserInfoCmd `cmd:"" help:"Show user information"`
}

type UserListCmd struct {
	Limit int `help:"Maximum number of users to list" default:"100"`
}

func (c *UserListCmd) Run(ctx *Context) error {
	if ctx.Config.Token == "" {
		return fmt.Errorf("not logged in. Run 'slack auth login' first")
	}

	client := slack.NewClient(ctx.Config.Token)
	resp, err := client.ListUsers(c.Limit)
	if err != nil {
		return fmt.Errorf("failed to list users: %w", err)
	}

	for _, user := range resp.Members {
		if user.Deleted || user.IsBot {
			continue
		}
		name := user.RealName
		if name == "" {
			name = user.Name
		}
		fmt.Printf("@%s - %s (%s)\n", user.Name, name, user.Profile.Title)
	}

	return nil
}

type UserInfoCmd struct {
	User string `arg:"" help:"User ID or email"`
}

func (c *UserInfoCmd) Run(ctx *Context) error {
	if ctx.Config.Token == "" {
		return fmt.Errorf("not logged in. Run 'slack auth login' first")
	}

	client := slack.NewClient(ctx.Config.Token)

	var user *slack.User
	var err error

	// Check if it looks like an email
	if len(c.User) > 0 && c.User[0] != 'U' && contains(c.User, "@") {
		user, err = client.LookupUserByEmail(c.User)
	} else {
		user, err = client.GetUserInfo(c.User)
	}

	if err != nil {
		return fmt.Errorf("failed to get user info: %w", err)
	}

	fmt.Printf("Name: %s\n", user.RealName)
	fmt.Printf("Username: @%s\n", user.Name)
	fmt.Printf("ID: %s\n", user.ID)
	if user.Profile.Title != "" {
		fmt.Printf("Title: %s\n", user.Profile.Title)
	}
	if user.Profile.Email != "" {
		fmt.Printf("Email: %s\n", user.Profile.Email)
	}
	if user.TZ != "" {
		fmt.Printf("Timezone: %s\n", user.TZ)
	}

	return nil
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
