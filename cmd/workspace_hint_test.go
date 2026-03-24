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
		if !strings.Contains(err.Error(), "Run 'slack-cli auth login' for that workspace") {
			t.Fatalf("expected slack-cli auth hint, got %q", err.Error())
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

func TestAugmentCrossWorkspaceChannelHint(t *testing.T) {
	baseErr := errors.New("slack API error: channel_not_found")

	t.Run("adds hint when channel may be in another configured workspace", func(t *testing.T) {
		ctx := &Context{Config: &config.Config{
			CurrentWorkspace: "buildkite.slack.com",
			Workspaces: map[string]config.WorkspaceAuth{
				"buildkite.slack.com":      {Token: "xoxp-primary"},
				"buildkite-corp.slack.com": {Token: "xoxp-corp"},
			},
		}}

		err := ctx.augmentCrossWorkspaceChannelHint("", baseErr)
		if !strings.Contains(err.Error(), "This channel may exist in another workspace") {
			t.Fatalf("expected cross-workspace hint, got %q", err.Error())
		}
		if !strings.Contains(err.Error(), "--workspace buildkite-corp.slack.com") {
			t.Fatalf("expected workspace flag suggestion, got %q", err.Error())
		}
	})

	t.Run("does not add hint when explicit workspace is set", func(t *testing.T) {
		ctx := &Context{
			Workspace: "buildkite.slack.com",
			Config: &config.Config{
				CurrentWorkspace: "buildkite.slack.com",
				Workspaces: map[string]config.WorkspaceAuth{
					"buildkite.slack.com":      {Token: "xoxp-primary"},
					"buildkite-corp.slack.com": {Token: "xoxp-corp"},
				},
			},
		}

		err := ctx.augmentCrossWorkspaceChannelHint("", baseErr)
		if err.Error() != baseErr.Error() {
			t.Fatalf("expected original error, got %q", err.Error())
		}
	})

	t.Run("does not add hint when there are no other token-backed workspaces", func(t *testing.T) {
		ctx := &Context{Config: &config.Config{
			CurrentWorkspace: "buildkite.slack.com",
			Workspaces: map[string]config.WorkspaceAuth{
				"buildkite.slack.com": {Token: "xoxp-primary"},
			},
		}}

		err := ctx.augmentCrossWorkspaceChannelHint("", baseErr)
		if err.Error() != baseErr.Error() {
			t.Fatalf("expected original error, got %q", err.Error())
		}
	})

	t.Run("uses URL workspace as current context", func(t *testing.T) {
		ctx := &Context{Config: &config.Config{
			CurrentWorkspace: "buildkite.slack.com",
			Workspaces: map[string]config.WorkspaceAuth{
				"buildkite.slack.com":      {Token: "xoxp-primary"},
				"buildkite-corp.slack.com": {Token: "xoxp-corp"},
			},
		}}

		err := ctx.augmentCrossWorkspaceChannelHint("https://buildkite-corp.slack.com/archives/C0AMP05SJKX/p1773973307481399", baseErr)
		if !strings.Contains(err.Error(), "Current workspace is buildkite-corp.slack.com") {
			t.Fatalf("expected URL workspace to be treated as current, got %q", err.Error())
		}
		if !strings.Contains(err.Error(), "--workspace buildkite.slack.com") {
			t.Fatalf("expected alternate workspace suggestion, got %q", err.Error())
		}
	})
}
