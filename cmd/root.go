package cmd

import (
	"fmt"
	"strings"

	"github.com/lox/slack-cli/internal/config"
	"github.com/lox/slack-cli/internal/slack"
)

type Context struct {
	Config    *config.Config
	Workspace string
}

func (ctx *Context) NewClient(urlHint string) (*slack.Client, error) {
	token, err := ctx.resolveToken(urlHint)
	if err != nil {
		return nil, err
	}

	return slack.NewClient(token), nil
}

func (ctx *Context) resolveToken(urlHint string) (string, error) {
	workspaceHint := strings.TrimSpace(ctx.Workspace)
	if urlHint != "" {
		host, teamID, err := slack.ExtractWorkspaceRef(urlHint)
		if err == nil {
			if workspaceHint == "" && host != "" {
				workspaceHint = host
			}
			if workspaceHint == "" && teamID != "" {
				workspaceHint = teamID
			}
		}
	}

	token, _, err := ctx.Config.TokenForWorkspace(workspaceHint)
	if err != nil {
		if workspaceHint != "" && strings.TrimSpace(ctx.Workspace) == "" && ctx.shouldFallbackToCurrentWorkspaceForURLHint() {
			fallbackToken, _, fallbackErr := ctx.Config.TokenForWorkspace("")
			if fallbackErr == nil {
				return fallbackToken, nil
			}
		}

		if workspaceHint != "" {
			return "", fmt.Errorf("%w. Run 'slack-cli auth login' for that workspace or pass --workspace", err)
		}
		return "", err
	}

	return token, nil
}

func (ctx *Context) shouldFallbackToCurrentWorkspaceForURLHint() bool {
	if ctx.Config == nil {
		return false
	}

	tokenBackedCount := 0
	tokenBackedWorkspace := ""
	for key, auth := range ctx.Config.Workspaces {
		if strings.TrimSpace(auth.Token) == "" {
			continue
		}
		tokenBackedCount++
		tokenBackedWorkspace = key
		if tokenBackedCount > 1 {
			return false
		}
	}

	// Preserve legacy behaviour for migrated single-workspace configs.
	return tokenBackedCount == 1 && tokenBackedWorkspace == "default"
}

type CLI struct {
	Workspace string     `help:"Workspace host (e.g. buildkite.slack.com) or team ID" short:"w"`
	Auth      AuthCmd    `cmd:"" help:"Authentication commands"`
	View      ViewCmd    `cmd:"" help:"View any Slack URL (message, thread, or channel)"`
	Channel   ChannelCmd `cmd:"" help:"Channel commands"`
	Search    SearchCmd  `cmd:"" help:"Search messages"`
	Thread    ThreadCmd  `cmd:"" help:"Thread commands"`
	User      UserCmd    `cmd:"" help:"User commands"`
	Version   VersionCmd `cmd:"" help:"Show version"`
}

type VersionCmd struct {
	Version string `kong:"hidden,default='${version}'"`
}

func (c *VersionCmd) Run(ctx *Context) error {
	println("slack-cli version " + c.Version)
	return nil
}
