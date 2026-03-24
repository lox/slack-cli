package cmd

import (
	"fmt"
	"sort"
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
			return fmt.Errorf("%w. Workspace %s is not configured. Run 'slack-cli auth login' for that workspace or pass --workspace", err, host)
		}
		return err
	}

	if teamID != "" {
		if _, _, lookupErr := ctx.Config.TokenForWorkspace(teamID); lookupErr != nil {
			return fmt.Errorf("%w. Workspace %s is not configured. Run 'slack-cli auth login' for that workspace or pass --workspace", err, teamID)
		}
	}

	return err
}

func (ctx *Context) augmentCrossWorkspaceChannelHint(rawURL string, err error) error {
	if err == nil {
		return nil
	}
	if strings.TrimSpace(ctx.Workspace) != "" {
		return err
	}
	if !strings.Contains(err.Error(), "slack API error: channel_not_found") {
		return err
	}

	current := ctx.workspaceUsedForRequest(rawURL)
	if current == "" {
		return err
	}

	otherWorkspaces := make([]string, 0, len(ctx.Config.Workspaces))
	for key, auth := range ctx.Config.Workspaces {
		if key == current || strings.TrimSpace(auth.Token) == "" {
			continue
		}
		otherWorkspaces = append(otherWorkspaces, key)
	}
	if len(otherWorkspaces) == 0 {
		return err
	}
	sort.Strings(otherWorkspaces)

	return fmt.Errorf("%w. This channel may exist in another workspace. Current workspace is %s; try one of: --workspace %s", err, current, strings.Join(otherWorkspaces, " or --workspace "))
}

func (ctx *Context) workspaceUsedForRequest(rawURL string) string {
	if ctx.Config == nil {
		return ""
	}

	if host, teamID, err := slack.ExtractWorkspaceRef(rawURL); err == nil {
		if host != "" {
			if resolved, resolveErr := ctx.Config.ResolveWorkspace(host); resolveErr == nil {
				return resolved
			}
			return host
		}
		if teamID != "" {
			if resolved, resolveErr := ctx.Config.ResolveWorkspace(teamID); resolveErr == nil {
				return resolved
			}
			return teamID
		}
	}

	return strings.TrimSpace(ctx.Config.CurrentWorkspace)
}
