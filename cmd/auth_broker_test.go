package cmd

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/lox/slack-cli/internal/config"
	"github.com/lox/slack-cli/internal/slack"
)

func TestBrokerTeamID(t *testing.T) {
	cfg := &config.Config{Workspaces: map[string]config.WorkspaceAuth{
		"example.slack.com": {TeamID: "TEXAMPLE"},
		"other.slack.com":   {},
	}}
	for _, workspace := range []string{"TEXAMPLE", "example.slack.com", "example"} {
		if team, err := brokerTeamID(cfg, workspace); err != nil || team != "TEXAMPLE" {
			t.Fatalf("workspace %q: team=%q error=%v", workspace, team, err)
		}
	}
	for _, workspace := range []string{"", "other.slack.com", "unknown.slack.com", "T/unsafe"} {
		if _, err := brokerTeamID(cfg, workspace); err == nil {
			t.Errorf("accepted workspace without known team ID: %q", workspace)
		}
	}
}

func TestBrokerLoginRejectsAppCredentials(t *testing.T) {
	c := AuthLoginCmd{AuthServer: "https://auth.example.com", ClientSecret: "example-secret"}
	if err := c.Run(&Context{}); err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("expected credentials conflict, got %v", err)
	}
}

func TestSaveBrokerLoginValidatesWorkspaceAndStoresNoAppCredentials(t *testing.T) {
	for _, team := range []string{"TEXAMPLE", "TOTHER"} {
		t.Run(team, func(t *testing.T) {
			cfg := loadTempConfig(t)
			cfg.SetWorkspaceAuth("example.slack.com", config.WorkspaceAuth{
				Token: "old-token", TeamID: "TEXAMPLE", ClientID: "old-client", ClientSecret: "old-secret",
			})
			if err := cfg.Save(); err != nil {
				t.Fatal(err)
			}
			ctx := &Context{Config: cfg, ClientFactory: func(token string) *slack.Client {
				if token != "example-user-token" {
					t.Error("auth.test did not use the received token")
				}
				return slack.NewClientWithHTTPClient(token, &http.Client{Transport: authTransport(func(r *http.Request) (*http.Response, error) {
					if r.URL.Path != "/api/auth.test" || r.Header.Get("Authorization") != "Bearer example-user-token" {
						t.Error("unexpected token validation request")
					}
					body := fmt.Sprintf(`{"ok":true,"url":"https://example.slack.com/","team":"Example","team_id":%q,"user":"person","user_id":"UPERSON"}`, team)
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
				})})
			}}
			c := AuthLoginCmd{}
			err := c.saveLogin(ctx, "example-user-token", "", "", true, false, "TEXAMPLE", "TEXAMPLE", bufio.NewReader(strings.NewReader("")))
			if (err == nil) != (team == "TEXAMPLE") {
				t.Fatalf("wrong validation outcome: %v", err)
			}
			reloaded, err := config.Load()
			if err != nil {
				t.Fatal(err)
			}
			auth := reloaded.Workspaces["example.slack.com"]
			if team == "TEXAMPLE" {
				if auth.Token != "example-user-token" || auth.ClientID != "" || auth.ClientSecret != "" {
					t.Fatal("broker login did not replace token and remove app credentials")
				}
			} else if auth.Token != "old-token" || auth.ClientSecret != "old-secret" {
				t.Fatal("rejected login modified saved credentials")
			}
		})
	}
}

type authTransport func(*http.Request) (*http.Response, error)

func (f authTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
