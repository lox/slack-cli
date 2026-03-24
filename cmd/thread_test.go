package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/lox/slack-cli/internal/config"
)

func TestThreadReadCmdAugmentReadError(t *testing.T) {
	baseErr := errors.New("slack API error: channel_not_found")

	t.Run("adds cross-workspace hint when URL is not provided", func(t *testing.T) {
		ctx := &Context{Config: &config.Config{
			CurrentWorkspace: "buildkite.slack.com",
			Workspaces: map[string]config.WorkspaceAuth{
				"buildkite.slack.com":      {Token: "xoxp-primary"},
				"buildkite-corp.slack.com": {Token: "xoxp-corp"},
			},
		}}

		cmd := &ThreadReadCmd{}
		err := cmd.augmentReadError(ctx, baseErr)
		if !strings.Contains(err.Error(), "This channel may exist in another workspace") {
			t.Fatalf("expected cross-workspace hint, got %q", err.Error())
		}
		if !strings.Contains(err.Error(), "--workspace buildkite-corp.slack.com") {
			t.Fatalf("expected workspace suggestion, got %q", err.Error())
		}
	})

	t.Run("keeps URL workspace configuration hint", func(t *testing.T) {
		ctx := &Context{Config: &config.Config{Workspaces: map[string]config.WorkspaceAuth{}}}
		cmd := &ThreadReadCmd{URL: "https://buildkite-corp.slack.com/archives/C0AMP05SJKX/p1773973307481399"}

		err := cmd.augmentReadError(ctx, baseErr)
		if !strings.Contains(err.Error(), "Workspace buildkite-corp.slack.com is not configured") {
			t.Fatalf("expected workspace configuration hint, got %q", err.Error())
		}
	})
}
