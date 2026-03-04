package cmd

import (
	"fmt"
	"strings"

	"github.com/lox/slack-cli/internal/slack"
)

func (ctx *Context) augmentChannelNotFoundError(rawURL string, err error) error {
	if err == nil {
		return nil
	}
	if strings.TrimSpace(ctx.Workspace) != "" {
		return err
	}
	if !strings.Contains(err.Error(), "slack API error: channel_not_found") {
		return err
	}

	host, teamID, parseErr := slack.ExtractWorkspaceRef(rawURL)
	if parseErr != nil {
		return err
	}

	if host != "" {
		if _, _, lookupErr := ctx.Config.TokenForWorkspace(host); lookupErr != nil {
			return fmt.Errorf("%w. Workspace %s is not configured. Run 'slack auth login' for that workspace or pass --workspace", err, host)
		}
		return err
	}

	if teamID != "" {
		if _, _, lookupErr := ctx.Config.TokenForWorkspace(teamID); lookupErr != nil {
			return fmt.Errorf("%w. Workspace %s is not configured. Run 'slack auth login' for that workspace or pass --workspace", err, teamID)
		}
	}

	return err
}
