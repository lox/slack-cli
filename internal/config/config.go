package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type WorkspaceAuth struct {
	Token        string `json:"token,omitempty"`
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
	Team         string `json:"team,omitempty"`
	TeamID       string `json:"team_id,omitempty"`
	URL          string `json:"url,omitempty"`
}

type Config struct {
	Token            string                   `json:"token,omitempty"`
	ClientID         string                   `json:"client_id,omitempty"`
	ClientSecret     string                   `json:"client_secret,omitempty"`
	CurrentWorkspace string                   `json:"current_workspace,omitempty"`
	Workspaces       map[string]WorkspaceAuth `json:"workspaces,omitempty"`
	path             string
}

func configPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "slack-cli", "config.json"), nil
}

func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	cfg := &Config{path: path}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	cfg.migrateLegacyToken()
	cfg.cleanupLegacyDefaultWorkspaceAlias()

	return cfg, nil
}

func normalizeWorkspaceKey(workspace string) string {
	return strings.ToLower(strings.TrimSpace(workspace))
}

func (c *Config) migrateLegacyToken() {
	if c.Token == "" || len(c.Workspaces) > 0 {
		return
	}

	if c.Workspaces == nil {
		c.Workspaces = map[string]WorkspaceAuth{}
	}

	c.Workspaces["default"] = WorkspaceAuth{Token: c.Token}
	if c.CurrentWorkspace == "" {
		c.CurrentWorkspace = "default"
	}
}

func (c *Config) cleanupLegacyDefaultWorkspaceAlias() {
	if len(c.Workspaces) <= 1 {
		return
	}

	legacy, ok := c.Workspaces["default"]
	if !ok {
		return
	}

	// Keep explicit default profiles; only remove bare legacy aliases.
	if legacy.Team != "" || legacy.TeamID != "" || legacy.URL != "" || legacy.ClientID != "" || legacy.ClientSecret != "" {
		return
	}

	delete(c.Workspaces, "default")

	if c.CurrentWorkspace == "default" {
		c.CurrentWorkspace = ""
		if legacy.Token != "" {
			for key, auth := range c.Workspaces {
				if auth.Token == legacy.Token && auth.Token != "" {
					c.CurrentWorkspace = key
					break
				}
			}
		}
		if c.CurrentWorkspace == "" {
			keys := make([]string, 0, len(c.Workspaces))
			for key := range c.Workspaces {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			if len(keys) > 0 {
				c.CurrentWorkspace = keys[0]
			}
		}
	}

	if c.CurrentWorkspace != "" {
		if auth, ok := c.Workspaces[c.CurrentWorkspace]; ok {
			c.Token = auth.Token
			return
		}
	}

	c.Token = ""
}

func (c *Config) SetWorkspaceAuth(workspace string, auth WorkspaceAuth) {
	workspace = c.workspaceKeyOrInput(workspace)
	if workspace == "" {
		return
	}

	if c.Workspaces == nil {
		c.Workspaces = map[string]WorkspaceAuth{}
	}

	c.Workspaces[workspace] = auth
	c.CurrentWorkspace = workspace
	c.Token = auth.Token
}

func (c *Config) SetWorkspaceCredentials(workspace, clientID, clientSecret string) {
	workspace = c.workspaceKeyOrInput(workspace)
	if workspace == "" {
		return
	}

	if c.Workspaces == nil {
		c.Workspaces = map[string]WorkspaceAuth{}
	}

	auth := c.Workspaces[workspace]
	auth.ClientID = strings.TrimSpace(clientID)
	auth.ClientSecret = strings.TrimSpace(clientSecret)
	c.Workspaces[workspace] = auth
	c.CurrentWorkspace = workspace
}

func (c *Config) OAuthCredentialsForWorkspace(workspace string) (clientID, clientSecret, resolvedWorkspace string, err error) {
	workspace = normalizeWorkspaceKey(workspace)

	if workspace != "" {
		resolvedWorkspace = c.workspaceKey(workspace)
		if resolvedWorkspace == "" {
			return "", "", "", fmt.Errorf("no workspace configured for %q", workspace)
		}

		auth := c.Workspaces[resolvedWorkspace]
		if auth.ClientID == "" || auth.ClientSecret == "" {
			return "", "", "", fmt.Errorf("no OAuth credentials configured for workspace %q", resolvedWorkspace)
		}

		return auth.ClientID, auth.ClientSecret, resolvedWorkspace, nil
	}

	if c.CurrentWorkspace != "" {
		auth := c.Workspaces[c.CurrentWorkspace]
		if auth.ClientID != "" && auth.ClientSecret != "" {
			return auth.ClientID, auth.ClientSecret, c.CurrentWorkspace, nil
		}
	}

	if c.ClientID != "" && c.ClientSecret != "" {
		return c.ClientID, c.ClientSecret, "", nil
	}

	return "", "", "", fmt.Errorf("no OAuth credentials configured")
}

func (c *Config) workspaceKeyOrInput(workspace string) string {
	workspace = normalizeWorkspaceKey(workspace)
	if workspace == "" {
		return ""
	}

	if resolved := c.workspaceKey(workspace); resolved != "" {
		return resolved
	}

	return workspace
}

func (c *Config) workspaceKey(workspace string) string {
	workspace = normalizeWorkspaceKey(workspace)
	if workspace == "" {
		return ""
	}

	if _, ok := c.Workspaces[workspace]; ok {
		return workspace
	}

	if !strings.Contains(workspace, ".") {
		hostWorkspace := workspace + ".slack.com"
		if _, ok := c.Workspaces[hostWorkspace]; ok {
			return hostWorkspace
		}
	}

	for key, auth := range c.Workspaces {
		if strings.EqualFold(auth.TeamID, workspace) {
			return key
		}
	}

	return ""
}

func (c *Config) TokenForWorkspace(workspace string) (token string, resolvedWorkspace string, err error) {
	workspace = normalizeWorkspaceKey(workspace)

	if workspace != "" {
		resolvedWorkspace = c.workspaceKey(workspace)
		if resolvedWorkspace != "" {
			auth := c.Workspaces[resolvedWorkspace]
			if auth.Token != "" {
				return auth.Token, resolvedWorkspace, nil
			}
		}

		return "", "", fmt.Errorf("no token configured for workspace %q", workspace)
	}

	if c.CurrentWorkspace != "" {
		if auth, ok := c.Workspaces[c.CurrentWorkspace]; ok && auth.Token != "" {
			return auth.Token, c.CurrentWorkspace, nil
		}
	}

	if c.Token != "" {
		return c.Token, "default", nil
	}

	return "", "", fmt.Errorf("not logged in. Run 'slack-cli auth login' first")
}

func (c *Config) Save() error {
	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(c.path, data, 0600)
}
