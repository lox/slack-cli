package cmd

import (
	"strings"
	"testing"

	"github.com/lox/slack-cli/internal/config"
)

func TestResolveTokenFallsBackToDefaultForUnmappedURLWorkspace(t *testing.T) {
	ctx := &Context{
		Config: &config.Config{
			CurrentWorkspace: "default",
			Workspaces: map[string]config.WorkspaceAuth{
				"default": {Token: "legacy-token"},
			},
		},
	}

	token, err := ctx.resolveToken("https://buildkite.slack.com/archives/C123/p1234567890123456")
	if err != nil {
		t.Fatalf("resolveToken returned error: %v", err)
	}
	if token != "legacy-token" {
		t.Fatalf("expected legacy-token fallback, got %q", token)
	}
}

func TestResolveTokenExplicitWorkspaceStillErrors(t *testing.T) {
	ctx := &Context{
		Workspace: "missing.slack.com",
		Config: &config.Config{
			CurrentWorkspace: "default",
			Workspaces: map[string]config.WorkspaceAuth{
				"default": {Token: "legacy-token"},
			},
		},
	}

	_, err := ctx.resolveToken("")
	if err == nil {
		t.Fatalf("expected error for unknown explicit workspace")
	}
	if !strings.Contains(err.Error(), "Run 'slack-cli auth login' for that workspace") {
		t.Fatalf("expected slack-cli auth hint, got %q", err.Error())
	}
}
