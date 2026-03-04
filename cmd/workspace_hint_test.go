package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/lox/slack-cli/internal/config"
)

func TestAugmentChannelNotFoundError(t *testing.T) {
	baseErr := errors.New("slack API error: channel_not_found")

	t.Run("adds config hint for unmapped workspace URL", func(t *testing.T) {
		ctx := &Context{Config: &config.Config{Workspaces: map[string]config.WorkspaceAuth{}}}
		err := ctx.augmentChannelNotFoundError("https://buildkite.slack.com/archives/C123/p1234567890123456", baseErr)
		if !strings.Contains(err.Error(), "Workspace buildkite.slack.com is not configured") {
			t.Fatalf("expected workspace configuration hint, got %q", err.Error())
		}
	})

	t.Run("does not add hint when workspace is mapped", func(t *testing.T) {
		ctx := &Context{Config: &config.Config{Workspaces: map[string]config.WorkspaceAuth{
			"buildkite.slack.com": {Token: "xoxp"},
		}}}
		err := ctx.augmentChannelNotFoundError("https://buildkite.slack.com/archives/C123/p1234567890123456", baseErr)
		if err.Error() != baseErr.Error() {
			t.Fatalf("expected original error, got %q", err.Error())
		}
	})

	t.Run("does not add hint when explicit workspace is set", func(t *testing.T) {
		ctx := &Context{
			Workspace: "buildkite.slack.com",
			Config:    &config.Config{Workspaces: map[string]config.WorkspaceAuth{}},
		}
		err := ctx.augmentChannelNotFoundError("https://buildkite.slack.com/archives/C123/p1234567890123456", baseErr)
		if err.Error() != baseErr.Error() {
			t.Fatalf("expected original error, got %q", err.Error())
		}
	})

	t.Run("does not add hint for non channel_not_found errors", func(t *testing.T) {
		otherErr := errors.New("slack API error: not_in_channel")
		ctx := &Context{Config: &config.Config{Workspaces: map[string]config.WorkspaceAuth{}}}
		err := ctx.augmentChannelNotFoundError("https://buildkite.slack.com/archives/C123/p1234567890123456", otherErr)
		if err.Error() != otherErr.Error() {
			t.Fatalf("expected original error, got %q", err.Error())
		}
	})
}
