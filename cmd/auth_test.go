package cmd

import (
	"testing"

	"github.com/lox/slack-cli/internal/config"
)

func TestResolveWorkspaceForLogout(t *testing.T) {
	cfg := &config.Config{
		CurrentWorkspace: "default",
		Workspaces: map[string]config.WorkspaceAuth{
			"default":             {Token: "legacy-token"},
			"buildkite.slack.com": {Token: "xoxp", TeamID: "TBUILD"},
		},
	}

	t.Run("empty request uses current workspace", func(t *testing.T) {
		got, err := resolveWorkspaceForLogout(cfg, "")
		if err != nil {
			t.Fatalf("resolveWorkspaceForLogout returned error: %v", err)
		}
		if got != "default" {
			t.Fatalf("expected default, got %q", got)
		}
	})

	t.Run("team ID resolves to workspace key", func(t *testing.T) {
		got, err := resolveWorkspaceForLogout(cfg, "TBUILD")
		if err != nil {
			t.Fatalf("resolveWorkspaceForLogout returned error: %v", err)
		}
		if got != "buildkite.slack.com" {
			t.Fatalf("expected buildkite.slack.com, got %q", got)
		}
	})

	t.Run("unknown workspace returns error", func(t *testing.T) {
		_, err := resolveWorkspaceForLogout(cfg, "typo.slack.com")
		if err == nil {
			t.Fatalf("expected error for unknown workspace")
		}
	})
}

func TestWorkspaceURLForDisplay(t *testing.T) {
	t.Run("prefers stored URL", func(t *testing.T) {
		got := workspaceURLForDisplay("buildkite.slack.com", config.WorkspaceAuth{URL: "https://buildkite.slack.com/"}, "")
		if got != "https://buildkite.slack.com/" {
			t.Fatalf("expected stored URL, got %q", got)
		}
	})

	t.Run("uses fallback URL when workspace auth URL missing", func(t *testing.T) {
		got := workspaceURLForDisplay("default", config.WorkspaceAuth{}, "https://buildkite.slack.com/")
		if got != "https://buildkite.slack.com/" {
			t.Fatalf("expected fallback URL, got %q", got)
		}
	})

	t.Run("builds URL from workspace host", func(t *testing.T) {
		got := workspaceURLForDisplay("buildkite.slack.com", config.WorkspaceAuth{}, "")
		if got != "https://buildkite.slack.com/" {
			t.Fatalf("expected host-derived URL, got %q", got)
		}
	})

	t.Run("falls back to key for non-host workspaces", func(t *testing.T) {
		got := workspaceURLForDisplay("default", config.WorkspaceAuth{}, "")
		if got != "default" {
			t.Fatalf("expected key fallback, got %q", got)
		}
	})
}

func TestGetOAuthCredentials(t *testing.T) {
	t.Run("uses workspace-scoped credentials first", func(t *testing.T) {
		cfg := &config.Config{
			Workspaces: map[string]config.WorkspaceAuth{
				"buildkite.slack.com": {ClientID: "id", ClientSecret: "secret", TeamID: "TBUILD"},
			},
		}

		id, secret, workspace, found, err := getOAuthCredentials(cfg, "TBUILD", "", "", true)
		if err != nil {
			t.Fatalf("getOAuthCredentials returned error: %v", err)
		}
		if !found {
			t.Fatalf("expected credentials to be found")
		}
		if id != "id" || secret != "secret" {
			t.Fatalf("expected workspace credentials, got %q/%q", id, secret)
		}
		if workspace != "buildkite.slack.com" {
			t.Fatalf("expected resolved workspace buildkite.slack.com, got %q", workspace)
		}
	})

	t.Run("uses explicit flags over config", func(t *testing.T) {
		cfg := &config.Config{}
		id, secret, workspace, found, err := getOAuthCredentials(cfg, "buildkite.slack.com", "flag-id", "flag-secret", true)
		if err != nil {
			t.Fatalf("getOAuthCredentials returned error: %v", err)
		}
		if !found {
			t.Fatalf("expected credentials to be found")
		}
		if id != "flag-id" || secret != "flag-secret" {
			t.Fatalf("expected flag credentials, got %q/%q", id, secret)
		}
		if workspace != "buildkite.slack.com" {
			t.Fatalf("expected workspace buildkite.slack.com, got %q", workspace)
		}
	})

	t.Run("falls back to env vars", func(t *testing.T) {
		t.Setenv("SLACK_CLIENT_ID", "env-id")
		t.Setenv("SLACK_CLIENT_SECRET", "env-secret")

		cfg := &config.Config{}
		id, secret, _, found, err := getOAuthCredentials(cfg, "", "", "", true)
		if err != nil {
			t.Fatalf("getOAuthCredentials returned error: %v", err)
		}
		if !found {
			t.Fatalf("expected credentials to be found")
		}
		if id != "env-id" || secret != "env-secret" {
			t.Fatalf("expected env credentials, got %q/%q", id, secret)
		}
	})

	t.Run("returns not found when workspace is missing config", func(t *testing.T) {
		cfg := &config.Config{}
		_, _, _, found, err := getOAuthCredentials(cfg, "missing.slack.com", "", "", true)
		if err != nil {
			t.Fatalf("expected no error when credentials are missing, got %v", err)
		}
		if found {
			t.Fatalf("expected credentials to be missing")
		}
	})

	t.Run("can skip current workspace credentials", func(t *testing.T) {
		t.Setenv("SLACK_CLIENT_ID", "")
		t.Setenv("SLACK_CLIENT_SECRET", "")

		cfg := &config.Config{
			CurrentWorkspace: "buildkite.slack.com",
			Workspaces: map[string]config.WorkspaceAuth{
				"buildkite.slack.com": {ClientID: "id", ClientSecret: "secret"},
			},
		}

		_, _, _, found, err := getOAuthCredentials(cfg, "", "", "", false)
		if err != nil {
			t.Fatalf("expected no error when skipping current workspace credentials, got %v", err)
		}
		if found {
			t.Fatalf("expected credentials to be missing when current workspace lookup is skipped")
		}
	})
}

func TestResetAllAuth(t *testing.T) {
	cfg := &config.Config{
		Token:            "legacy-token",
		ClientID:         "global-id",
		ClientSecret:     "global-secret",
		CurrentWorkspace: "buildkite.slack.com",
		Workspaces: map[string]config.WorkspaceAuth{
			"buildkite.slack.com": {Token: "xoxp", ClientID: "id", ClientSecret: "secret"},
		},
	}

	resetAllAuth(cfg)

	if len(cfg.Workspaces) != 0 {
		t.Fatalf("expected all workspaces to be removed")
	}
	if cfg.CurrentWorkspace != "" || cfg.Token != "" {
		t.Fatalf("expected current workspace/token to be cleared")
	}
	if cfg.ClientID != "" || cfg.ClientSecret != "" {
		t.Fatalf("expected global client credentials to be cleared")
	}
}

func TestShouldSetWorkspaceAsDefault(t *testing.T) {
	tests := []struct {
		name            string
		previousCurrent string
		workspaceHost   string
		want            bool
	}{
		{name: "first login", previousCurrent: "", workspaceHost: "buildkite.slack.com", want: true},
		{name: "same workspace", previousCurrent: "buildkite.slack.com", workspaceHost: "buildkite.slack.com", want: true},
		{name: "different workspace", previousCurrent: "buildkite-corp.slack.com", workspaceHost: "buildkite.slack.com", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldSetWorkspaceAsDefault(tt.previousCurrent, tt.workspaceHost)
			if got != tt.want {
				t.Fatalf("shouldSetWorkspaceAsDefault(%q, %q) = %v, want %v", tt.previousCurrent, tt.workspaceHost, got, tt.want)
			}
		})
	}
}
