package cmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lox/slack-cli/internal/config"
)

func loadTempConfig(t *testing.T) *config.Config {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load returned error: %v", err)
	}

	return cfg
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe returned error: %v", err)
	}

	os.Stdout = writer
	defer func() {
		os.Stdout = originalStdout
	}()

	fn()

	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close returned error: %v", err)
	}

	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll returned error: %v", err)
	}

	return string(output)
}

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

	t.Run("resolves workspace without requiring a token", func(t *testing.T) {
		cfgWithoutToken := &config.Config{
			Workspaces: map[string]config.WorkspaceAuth{
				"buildkite.slack.com": {TeamID: "TBUILD"},
			},
		}

		got, err := resolveWorkspaceForLogout(cfgWithoutToken, "TBUILD")
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

	t.Run("falls back to global config credentials for missing workspace", func(t *testing.T) {
		cfg := &config.Config{ClientID: "global-id", ClientSecret: "global-secret"}

		id, secret, workspace, found, err := getOAuthCredentials(cfg, "missing.slack.com", "", "", true)
		if err != nil {
			t.Fatalf("getOAuthCredentials returned error: %v", err)
		}
		if !found {
			t.Fatalf("expected credentials to be found")
		}
		if id != "global-id" || secret != "global-secret" {
			t.Fatalf("expected global credentials, got %q/%q", id, secret)
		}
		if workspace != "missing.slack.com" {
			t.Fatalf("expected workspace ref to be preserved, got %q", workspace)
		}
	})

	t.Run("env vars override global config credentials", func(t *testing.T) {
		t.Setenv("SLACK_CLIENT_ID", "env-id")
		t.Setenv("SLACK_CLIENT_SECRET", "env-secret")

		cfg := &config.Config{ClientID: "global-id", ClientSecret: "global-secret"}

		id, secret, workspace, found, err := getOAuthCredentials(cfg, "missing.slack.com", "", "", true)
		if err != nil {
			t.Fatalf("getOAuthCredentials returned error: %v", err)
		}
		if !found {
			t.Fatalf("expected credentials to be found")
		}
		if id != "env-id" || secret != "env-secret" {
			t.Fatalf("expected env credentials to override global config, got %q/%q", id, secret)
		}
		if workspace != "missing.slack.com" {
			t.Fatalf("expected workspace ref to be preserved, got %q", workspace)
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

	t.Run("does not reuse other workspace credentials for explicit workspace", func(t *testing.T) {
		t.Setenv("SLACK_CLIENT_ID", "")
		t.Setenv("SLACK_CLIENT_SECRET", "")

		cfg := &config.Config{
			Workspaces: map[string]config.WorkspaceAuth{
				"buildkite.slack.com": {ClientID: "id", ClientSecret: "secret"},
			},
		}

		_, _, workspace, found, err := getOAuthCredentials(cfg, "buildkite-corp.slack.com", "", "", true)
		if err != nil {
			t.Fatalf("expected no error when explicit workspace is missing credentials, got %v", err)
		}
		if found {
			t.Fatalf("expected explicit workspace lookup to require workspace-specific, env, or global credentials")
		}
		if workspace != "buildkite-corp.slack.com" {
			t.Fatalf("expected workspace ref to be preserved, got %q", workspace)
		}
	})

	t.Run("does not reuse other workspace credentials when current workspace lookup is skipped", func(t *testing.T) {
		t.Setenv("SLACK_CLIENT_ID", "")
		t.Setenv("SLACK_CLIENT_SECRET", "")

		cfg := &config.Config{
			CurrentWorkspace: "buildkite.slack.com",
			Workspaces: map[string]config.WorkspaceAuth{
				"buildkite.slack.com": {ClientID: "id", ClientSecret: "secret"},
			},
		}

		_, _, workspace, found, err := getOAuthCredentials(cfg, "", "", "", false)
		if err != nil {
			t.Fatalf("expected no error when skipping current workspace credentials, got %v", err)
		}
		if found {
			t.Fatalf("expected credentials to remain missing when current workspace lookup is skipped")
		}
		if workspace != "" {
			t.Fatalf("expected empty workspace ref to stay empty, got %q", workspace)
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

func TestAuthLogoutRunPreservesStoredOAuthCredentials(t *testing.T) {
	cfg := loadTempConfig(t)
	cfg.CurrentWorkspace = "buildkite.slack.com"
	cfg.Token = "xoxp-buildkite"
	cfg.Workspaces = map[string]config.WorkspaceAuth{
		"buildkite.slack.com": {
			Token:        "xoxp-buildkite",
			ClientID:     "client-id",
			ClientSecret: "client-secret",
			Team:         "Buildkite",
			TeamID:       "TBUILD",
			URL:          "https://buildkite.slack.com/",
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("cfg.Save returned error: %v", err)
	}

	if err := (&AuthLogoutCmd{}).Run(&Context{Config: cfg}); err != nil {
		t.Fatalf("AuthLogoutCmd.Run returned error: %v", err)
	}

	reloaded, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load returned error: %v", err)
	}

	auth, ok := reloaded.Workspaces["buildkite.slack.com"]
	if !ok {
		t.Fatalf("expected logged out workspace to be preserved")
	}
	if auth.Token != "" {
		t.Fatalf("expected workspace token to be cleared, got %q", auth.Token)
	}
	if auth.ClientID != "client-id" || auth.ClientSecret != "client-secret" {
		t.Fatalf("expected stored OAuth credentials to remain, got %q/%q", auth.ClientID, auth.ClientSecret)
	}
	if reloaded.CurrentWorkspace != "buildkite.slack.com" {
		t.Fatalf("expected current workspace to remain buildkite.slack.com, got %q", reloaded.CurrentWorkspace)
	}
	if reloaded.Token != "" {
		t.Fatalf("expected mirrored legacy token to be cleared, got %q", reloaded.Token)
	}

	id, secret, workspace, found, err := getOAuthCredentials(reloaded, "", "", "", true)
	if err != nil {
		t.Fatalf("getOAuthCredentials returned error: %v", err)
	}
	if !found {
		t.Fatalf("expected saved credentials to be reusable after logout")
	}
	if id != "client-id" || secret != "client-secret" {
		t.Fatalf("expected saved credentials, got %q/%q", id, secret)
	}
	if workspace != "buildkite.slack.com" {
		t.Fatalf("expected current workspace credentials to resolve to buildkite.slack.com, got %q", workspace)
	}
}

func TestAuthLogoutRunPromotesAnotherLoggedInWorkspace(t *testing.T) {
	cfg := loadTempConfig(t)
	cfg.CurrentWorkspace = "buildkite.slack.com"
	cfg.Token = "xoxp-buildkite"
	cfg.Workspaces = map[string]config.WorkspaceAuth{
		"buildkite.slack.com": {
			Token:        "xoxp-buildkite",
			ClientID:     "buildkite-id",
			ClientSecret: "buildkite-secret",
			TeamID:       "TBUILD",
		},
		"corp.slack.com": {
			Token:        "xoxp-corp",
			ClientID:     "corp-id",
			ClientSecret: "corp-secret",
			TeamID:       "TCORP",
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("cfg.Save returned error: %v", err)
	}

	if err := (&AuthLogoutCmd{}).Run(&Context{Config: cfg}); err != nil {
		t.Fatalf("AuthLogoutCmd.Run returned error: %v", err)
	}

	reloaded, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load returned error: %v", err)
	}

	if reloaded.CurrentWorkspace != "corp.slack.com" {
		t.Fatalf("expected current workspace to switch to corp.slack.com, got %q", reloaded.CurrentWorkspace)
	}
	if reloaded.Token != "xoxp-corp" {
		t.Fatalf("expected mirrored legacy token xoxp-corp, got %q", reloaded.Token)
	}

	buildkite := reloaded.Workspaces["buildkite.slack.com"]
	if buildkite.Token != "" {
		t.Fatalf("expected logged out workspace token to be cleared, got %q", buildkite.Token)
	}
	if buildkite.ClientID != "buildkite-id" || buildkite.ClientSecret != "buildkite-secret" {
		t.Fatalf("expected logged out workspace credentials to be preserved, got %q/%q", buildkite.ClientID, buildkite.ClientSecret)
	}
}

func TestAuthStatusRunShowsConfiguredLoggedOutWorkspaces(t *testing.T) {
	ctx := &Context{
		Config: &config.Config{
			CurrentWorkspace: "buildkite.slack.com",
			Workspaces: map[string]config.WorkspaceAuth{
				"buildkite.slack.com": {
					ClientID:     "support-id",
					ClientSecret: "support-secret",
					URL:          "https://buildkite.slack.com/",
				},
				"buildkite-corp.slack.com": {
					ClientID:     "corp-id",
					ClientSecret: "corp-secret",
					URL:          "https://buildkite-corp.slack.com/",
				},
			},
		},
	}

	output := captureStdout(t, func() {
		if err := (&AuthStatusCmd{}).Run(ctx); err != nil {
			t.Fatalf("AuthStatusCmd.Run returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Not logged in. Run 'slack-cli auth login' to authenticate.") {
		t.Fatalf("expected not logged in message, got %q", output)
	}
	if !strings.Contains(output, "Configured workspaces:") {
		t.Fatalf("expected configured workspaces list, got %q", output)
	}
	if !strings.Contains(output, "https://buildkite.slack.com/ (default, logged out)") {
		t.Fatalf("expected default logged-out workspace entry, got %q", output)
	}
	if !strings.Contains(output, "https://buildkite-corp.slack.com/ (logged out)") {
		t.Fatalf("expected non-default logged-out workspace entry, got %q", output)
	}
}

func TestAuthStatusRunShowsRequestedConfiguredWorkspaceWithoutToken(t *testing.T) {
	ctx := &Context{
		Workspace: "buildkite",
		Config: &config.Config{
			CurrentWorkspace: "buildkite-corp.slack.com",
			Workspaces: map[string]config.WorkspaceAuth{
				"buildkite.slack.com": {
					ClientID:     "support-id",
					ClientSecret: "support-secret",
					URL:          "https://buildkite.slack.com/",
				},
				"buildkite-corp.slack.com": {
					Token:        "xoxp-corp",
					ClientID:     "corp-id",
					ClientSecret: "corp-secret",
					URL:          "https://buildkite-corp.slack.com/",
				},
			},
		},
	}

	output := captureStdout(t, func() {
		if err := (&AuthStatusCmd{}).Run(ctx); err != nil {
			t.Fatalf("AuthStatusCmd.Run returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Workspace https://buildkite.slack.com/ is configured but not logged in.") {
		t.Fatalf("expected configured but not logged in message, got %q", output)
	}
	if !strings.Contains(output, "slack-cli --workspace buildkite.slack.com auth login") {
		t.Fatalf("expected workspace-specific login hint, got %q", output)
	}
	if !strings.Contains(output, "https://buildkite.slack.com/ (logged out)") {
		t.Fatalf("expected logged-out workspace entry, got %q", output)
	}
}

func TestShouldSetWorkspaceAsDefault(t *testing.T) {
	tests := []struct {
		name            string
		cfg             *config.Config
		previousCurrent string
		workspaceHost   string
		replace         bool
		want            bool
	}{
		{name: "first login", cfg: &config.Config{}, previousCurrent: "", workspaceHost: "buildkite.slack.com", replace: false, want: true},
		{name: "same workspace", cfg: &config.Config{}, previousCurrent: "buildkite.slack.com", workspaceHost: "buildkite.slack.com", replace: false, want: true},
		{name: "different workspace", cfg: &config.Config{}, previousCurrent: "buildkite-corp.slack.com", workspaceHost: "buildkite.slack.com", replace: false, want: false},
		{
			name: "legacy default alias matching token treated as same workspace",
			cfg: &config.Config{Workspaces: map[string]config.WorkspaceAuth{
				"default":             {Token: "xoxp-shared"},
				"buildkite.slack.com": {Token: "xoxp-shared", TeamID: "TBUILD"},
			}},
			previousCurrent: "default",
			workspaceHost:   "buildkite.slack.com",
			replace:         false,
			want:            true,
		},
		{
			name: "legacy default alias replace without mapped workspace keeps new default",
			cfg: &config.Config{Workspaces: map[string]config.WorkspaceAuth{
				"default": {Token: "xoxp-legacy"},
			}},
			previousCurrent: "default",
			workspaceHost:   "buildkite.slack.com",
			replace:         true,
			want:            true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldSetWorkspaceAsDefault(tt.cfg, tt.previousCurrent, tt.workspaceHost, tt.replace)
			if got != tt.want {
				t.Fatalf("shouldSetWorkspaceAsDefault(%q, %q) = %v, want %v", tt.previousCurrent, tt.workspaceHost, got, tt.want)
			}
		})
	}
}

func TestRequestedWorkspaceMatchesAuthResult(t *testing.T) {
	tests := []struct {
		name              string
		requested         string
		resolvedRequested string
		authHost          string
		authTeamID        string
		want              bool
	}{
		{name: "no requested workspace", authHost: "buildkite.slack.com", authTeamID: "TBUILD", want: true},
		{name: "explicit host matches", requested: "buildkite.slack.com", authHost: "buildkite.slack.com", authTeamID: "TBUILD", want: true},
		{name: "short host matches", requested: "buildkite", authHost: "buildkite.slack.com", authTeamID: "TBUILD", want: true},
		{name: "team ID matches", requested: "TBUILD", authHost: "buildkite.slack.com", authTeamID: "TBUILD", want: true},
		{name: "resolved workspace host wins", requested: "TBUILD", resolvedRequested: "buildkite.slack.com", authHost: "buildkite.slack.com", authTeamID: "TBUILD", want: true},
		{name: "legacy default alias matches any authenticated workspace", requested: "default", authHost: "buildkite.slack.com", authTeamID: "TBUILD", want: true},
		{name: "mismatch", requested: "buildkite-corp", authHost: "buildkite.slack.com", authTeamID: "TBUILD", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := requestedWorkspaceMatchesAuthResult(tt.requested, tt.resolvedRequested, tt.authHost, tt.authTeamID)
			if got != tt.want {
				t.Fatalf("requestedWorkspaceMatchesAuthResult(%q, %q, %q, %q) = %v, want %v", tt.requested, tt.resolvedRequested, tt.authHost, tt.authTeamID, got, tt.want)
			}
		})
	}
}

func TestWorkspaceKeyFromAuthResult(t *testing.T) {
	t.Run("prefers parsed workspace host from URL", func(t *testing.T) {
		got := workspaceKeyFromAuthResult("https://buildkite.slack.com/", "TBUILD", "Buildkite")
		if got != "buildkite.slack.com" {
			t.Fatalf("expected buildkite.slack.com, got %q", got)
		}
	})

	t.Run("falls back to team ID when URL cannot be parsed", func(t *testing.T) {
		got := workspaceKeyFromAuthResult("", "TBUILD", "Buildkite")
		if got != "tbuild" {
			t.Fatalf("expected lowercase team ID fallback, got %q", got)
		}
	})

	t.Run("does not return legacy default key", func(t *testing.T) {
		got := workspaceKeyFromAuthResult("", "", "")
		if got != "" {
			t.Fatalf("expected empty fallback, got %q", got)
		}
	})
}
