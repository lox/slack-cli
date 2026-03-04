package config

import "testing"

func TestTokenForWorkspace(t *testing.T) {
	t.Run("returns token for explicit workspace host", func(t *testing.T) {
		cfg := &Config{
			Workspaces: map[string]WorkspaceAuth{
				"buildkite.slack.com": {Token: "xoxp-buildkite", TeamID: "TBUILD"},
			},
		}

		tok, workspace, err := cfg.TokenForWorkspace("buildkite.slack.com")
		if err != nil {
			t.Fatalf("TokenForWorkspace returned error: %v", err)
		}
		if tok != "xoxp-buildkite" {
			t.Fatalf("expected token xoxp-buildkite, got %q", tok)
		}
		if workspace != "buildkite.slack.com" {
			t.Fatalf("expected workspace buildkite.slack.com, got %q", workspace)
		}
	})

	t.Run("resolves by team ID", func(t *testing.T) {
		cfg := &Config{
			Workspaces: map[string]WorkspaceAuth{
				"buildkite.slack.com": {Token: "xoxp-buildkite", TeamID: "TBUILD"},
			},
		}

		tok, workspace, err := cfg.TokenForWorkspace("TBUILD")
		if err != nil {
			t.Fatalf("TokenForWorkspace returned error: %v", err)
		}
		if tok != "xoxp-buildkite" {
			t.Fatalf("expected token xoxp-buildkite, got %q", tok)
		}
		if workspace != "buildkite.slack.com" {
			t.Fatalf("expected workspace buildkite.slack.com, got %q", workspace)
		}
	})

	t.Run("resolves by short workspace name", func(t *testing.T) {
		cfg := &Config{
			Workspaces: map[string]WorkspaceAuth{
				"buildkite.slack.com": {Token: "xoxp-buildkite"},
			},
		}

		tok, workspace, err := cfg.TokenForWorkspace("buildkite")
		if err != nil {
			t.Fatalf("TokenForWorkspace returned error: %v", err)
		}
		if tok != "xoxp-buildkite" {
			t.Fatalf("expected token xoxp-buildkite, got %q", tok)
		}
		if workspace != "buildkite.slack.com" {
			t.Fatalf("expected workspace buildkite.slack.com, got %q", workspace)
		}
	})

	t.Run("uses current workspace when workspace not provided", func(t *testing.T) {
		cfg := &Config{
			CurrentWorkspace: "lox.slack.com",
			Workspaces: map[string]WorkspaceAuth{
				"lox.slack.com": {Token: "xoxp-lox", TeamID: "TLOX"},
			},
		}

		tok, workspace, err := cfg.TokenForWorkspace("")
		if err != nil {
			t.Fatalf("TokenForWorkspace returned error: %v", err)
		}
		if tok != "xoxp-lox" {
			t.Fatalf("expected token xoxp-lox, got %q", tok)
		}
		if workspace != "lox.slack.com" {
			t.Fatalf("expected workspace lox.slack.com, got %q", workspace)
		}
	})

	t.Run("falls back to legacy token", func(t *testing.T) {
		cfg := &Config{Token: "legacy-token"}

		tok, workspace, err := cfg.TokenForWorkspace("")
		if err != nil {
			t.Fatalf("TokenForWorkspace returned error: %v", err)
		}
		if tok != "legacy-token" {
			t.Fatalf("expected token legacy-token, got %q", tok)
		}
		if workspace != "default" {
			t.Fatalf("expected workspace default, got %q", workspace)
		}
	})
}

func TestSetWorkspaceAuth(t *testing.T) {
	cfg := &Config{}
	cfg.SetWorkspaceAuth("Buildkite.Slack.com", WorkspaceAuth{Token: "xoxp", TeamID: "T123"})

	if cfg.CurrentWorkspace != "buildkite.slack.com" {
		t.Fatalf("expected current workspace buildkite.slack.com, got %q", cfg.CurrentWorkspace)
	}
	if cfg.Token != "xoxp" {
		t.Fatalf("expected legacy token mirror xoxp, got %q", cfg.Token)
	}
	auth, ok := cfg.Workspaces["buildkite.slack.com"]
	if !ok {
		t.Fatalf("expected workspace key buildkite.slack.com to be stored")
	}
	if auth.TeamID != "T123" {
		t.Fatalf("expected team ID T123, got %q", auth.TeamID)
	}
}

func TestMigrateLegacyToken(t *testing.T) {
	cfg := &Config{Token: "legacy-token"}
	cfg.migrateLegacyToken()

	if cfg.CurrentWorkspace != "default" {
		t.Fatalf("expected current workspace default, got %q", cfg.CurrentWorkspace)
	}
	auth, ok := cfg.Workspaces["default"]
	if !ok {
		t.Fatalf("expected default workspace to be created")
	}
	if auth.Token != "legacy-token" {
		t.Fatalf("expected legacy token to be migrated, got %q", auth.Token)
	}
}

func TestCleanupLegacyDefaultWorkspaceAlias(t *testing.T) {
	t.Run("removes duplicated default alias after real workspace exists", func(t *testing.T) {
		cfg := &Config{
			Token:            "xoxp-shared",
			CurrentWorkspace: "default",
			Workspaces: map[string]WorkspaceAuth{
				"default":             {Token: "xoxp-shared"},
				"buildkite.slack.com": {Token: "xoxp-shared", TeamID: "TBUILD", URL: "https://buildkite.slack.com/"},
			},
		}

		cfg.cleanupLegacyDefaultWorkspaceAlias()

		if _, ok := cfg.Workspaces["default"]; ok {
			t.Fatalf("expected legacy default workspace alias to be removed")
		}
		if cfg.CurrentWorkspace != "buildkite.slack.com" {
			t.Fatalf("expected current workspace to switch to buildkite.slack.com, got %q", cfg.CurrentWorkspace)
		}
		if cfg.Token != "xoxp-shared" {
			t.Fatalf("expected mirrored token to remain xoxp-shared, got %q", cfg.Token)
		}
	})

	t.Run("keeps default when token does not match another workspace", func(t *testing.T) {
		cfg := &Config{
			CurrentWorkspace: "default",
			Workspaces: map[string]WorkspaceAuth{
				"default":             {Token: "xoxp-default"},
				"buildkite.slack.com": {Token: "xoxp-other", TeamID: "TBUILD", URL: "https://buildkite.slack.com/"},
			},
		}

		cfg.cleanupLegacyDefaultWorkspaceAlias()

		if _, ok := cfg.Workspaces["default"]; !ok {
			t.Fatalf("expected default workspace to be kept when token differs")
		}
	})
}

func TestSetWorkspaceCredentials(t *testing.T) {
	cfg := &Config{
		Workspaces: map[string]WorkspaceAuth{
			"buildkite.slack.com": {Token: "xoxp", TeamID: "TBUILD"},
		},
	}

	cfg.SetWorkspaceCredentials("TBUILD", "client-id", "client-secret")

	auth := cfg.Workspaces["buildkite.slack.com"]
	if auth.ClientID != "client-id" {
		t.Fatalf("expected client-id, got %q", auth.ClientID)
	}
	if auth.ClientSecret != "client-secret" {
		t.Fatalf("expected client-secret, got %q", auth.ClientSecret)
	}
	if cfg.CurrentWorkspace != "buildkite.slack.com" {
		t.Fatalf("expected current workspace buildkite.slack.com, got %q", cfg.CurrentWorkspace)
	}
}

func TestOAuthCredentialsForWorkspace(t *testing.T) {
	t.Run("returns workspace-specific credentials", func(t *testing.T) {
		cfg := &Config{
			Workspaces: map[string]WorkspaceAuth{
				"buildkite.slack.com": {ClientID: "id", ClientSecret: "secret", TeamID: "TBUILD"},
			},
		}

		id, secret, workspace, err := cfg.OAuthCredentialsForWorkspace("TBUILD")
		if err != nil {
			t.Fatalf("OAuthCredentialsForWorkspace returned error: %v", err)
		}
		if id != "id" || secret != "secret" {
			t.Fatalf("expected id/secret, got %q/%q", id, secret)
		}
		if workspace != "buildkite.slack.com" {
			t.Fatalf("expected buildkite.slack.com, got %q", workspace)
		}
	})

	t.Run("resolves credentials by short workspace name", func(t *testing.T) {
		cfg := &Config{
			Workspaces: map[string]WorkspaceAuth{
				"buildkite.slack.com": {ClientID: "id", ClientSecret: "secret"},
			},
		}

		id, secret, workspace, err := cfg.OAuthCredentialsForWorkspace("buildkite")
		if err != nil {
			t.Fatalf("OAuthCredentialsForWorkspace returned error: %v", err)
		}
		if id != "id" || secret != "secret" {
			t.Fatalf("expected id/secret, got %q/%q", id, secret)
		}
		if workspace != "buildkite.slack.com" {
			t.Fatalf("expected buildkite.slack.com, got %q", workspace)
		}
	})

	t.Run("falls back to global credentials", func(t *testing.T) {
		cfg := &Config{ClientID: "global-id", ClientSecret: "global-secret"}

		id, secret, workspace, err := cfg.OAuthCredentialsForWorkspace("")
		if err != nil {
			t.Fatalf("OAuthCredentialsForWorkspace returned error: %v", err)
		}
		if id != "global-id" || secret != "global-secret" {
			t.Fatalf("expected global creds, got %q/%q", id, secret)
		}
		if workspace != "" {
			t.Fatalf("expected empty workspace for global fallback, got %q", workspace)
		}
	})
}
