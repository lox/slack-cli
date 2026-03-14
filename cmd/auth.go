package cmd

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/lox/slack-cli/internal/config"
	"github.com/lox/slack-cli/internal/slack"
)

const (
	oauthRedirectPort = "8338"
	oauthRedirectURL  = "http://localhost:" + oauthRedirectPort + "/callback"
)

func getOAuthCredentials(cfg *config.Config, workspaceRef, flagClientID, flagClientSecret string, useCurrentWorkspace bool) (clientID, clientSecret, resolvedWorkspace string, found bool, err error) {
	flagClientID = strings.TrimSpace(flagClientID)
	flagClientSecret = strings.TrimSpace(flagClientSecret)
	workspaceRef = strings.TrimSpace(workspaceRef)

	if flagClientID != "" || flagClientSecret != "" {
		if flagClientID == "" || flagClientSecret == "" {
			return "", "", "", false, fmt.Errorf("both --client-id and --client-secret must be provided")
		}
		return flagClientID, flagClientSecret, workspaceRef, true, nil
	}

	if workspaceRef == "" && useCurrentWorkspace {
		workspaceRef = cfg.CurrentWorkspace
	}

	if workspaceRef != "" || useCurrentWorkspace {
		if clientID, clientSecret, resolvedWorkspace, err := cfg.OAuthCredentialsForWorkspace(workspaceRef); err == nil {
			return clientID, clientSecret, resolvedWorkspace, true, nil
		}
	}

	// Environment overrides global config for CI and per-shell auth.
	clientID = os.Getenv("SLACK_CLIENT_ID")
	clientSecret = os.Getenv("SLACK_CLIENT_SECRET")
	if clientID != "" && clientSecret != "" {
		return clientID, clientSecret, workspaceRef, true, nil
	}

	if cfg.ClientID != "" && cfg.ClientSecret != "" {
		return cfg.ClientID, cfg.ClientSecret, workspaceRef, true, nil
	}

	return "", "", workspaceRef, false, nil
}

func generateOAuthState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func printSlackAppSetupGuide() {
	fmt.Println()
	fmt.Println("Slack App Configuration")
	fmt.Println("-----------------------")
	fmt.Println("This CLI requires a Slack app for OAuth authentication.")
	fmt.Println()
	fmt.Println("To create an app:")
	fmt.Println("  1. Go to https://api.slack.com/apps")
	fmt.Println("  2. Click 'Create New App' > 'From a manifest'")
	fmt.Println("  3. Select your workspace")
	fmt.Println("  4. Paste the manifest from:")
	fmt.Println("     https://github.com/lox/slack-cli/blob/main/slack-app-manifest.yaml")
	fmt.Println("  5. Click 'Create'")
	fmt.Println("  6. Go to 'Basic Information' to find your credentials")
	fmt.Println()
}

func promptSetDefaultWorkspace(reader *bufio.Reader, workspaceHost string) (bool, error) {
	fmt.Printf("Set %s as the default workspace? [y/N]: ", workspaceHost)
	choice, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(choice), "y") || strings.EqualFold(strings.TrimSpace(choice), "yes"), nil
}

func isLegacyDefaultWorkspaceAlias(auth config.WorkspaceAuth) bool {
	return auth.Team == "" && auth.TeamID == "" && auth.URL == "" && auth.ClientID == "" && auth.ClientSecret == ""
}

func shouldSetWorkspaceAsDefault(cfg *config.Config, previousCurrent, workspaceHost string, replace bool) bool {
	previousCurrent = strings.TrimSpace(strings.ToLower(previousCurrent))
	workspaceHost = strings.TrimSpace(strings.ToLower(workspaceHost))

	if previousCurrent == "" || previousCurrent == workspaceHost {
		return true
	}

	if cfg == nil || previousCurrent != "default" {
		return false
	}

	legacyAuth, legacyOK := cfg.Workspaces[previousCurrent]
	if !legacyOK || !isLegacyDefaultWorkspaceAlias(legacyAuth) {
		return false
	}

	if replace {
		// Replacing a migrated legacy default login should keep the newly authenticated workspace as default.
		return true
	}

	workspaceAuth, workspaceOK := cfg.Workspaces[workspaceHost]
	if !workspaceOK {
		return false
	}

	return legacyAuth.Token != "" && legacyAuth.Token == workspaceAuth.Token
}

func requestedWorkspaceMatchesAuthResult(requestedWorkspace, resolvedWorkspace, authenticatedHost, authenticatedTeamID string) bool {
	expected := strings.TrimSpace(strings.ToLower(resolvedWorkspace))
	if expected == "" {
		expected = strings.TrimSpace(strings.ToLower(requestedWorkspace))
	}

	if expected == "" {
		return true
	}
	if expected == "default" {
		return true
	}

	authenticatedHost = strings.TrimSpace(strings.ToLower(authenticatedHost))
	authenticatedTeamID = strings.TrimSpace(authenticatedTeamID)

	if expected == authenticatedHost || strings.EqualFold(expected, authenticatedTeamID) {
		return true
	}

	if !strings.Contains(expected, ".") && expected+".slack.com" == authenticatedHost {
		return true
	}

	return false
}

func workspaceKeyFromAuthResult(userURL, teamID, team string) string {
	if host, _, err := slack.ExtractWorkspaceRef(userURL); err == nil && host != "" {
		return strings.ToLower(strings.TrimSpace(host))
	}

	if strings.TrimSpace(teamID) != "" {
		return strings.ToLower(strings.TrimSpace(teamID))
	}

	team = strings.TrimSpace(team)
	if team == "" {
		return ""
	}

	return strings.ToLower(strings.ReplaceAll(team, " ", "-"))
}

// Scopes needed for the CLI
var oauthScopes = []string{
	"channels:history",
	"channels:read",
	"files:read",
	"groups:history",
	"groups:read",
	"im:history",
	"im:read",
	"mpim:history",
	"mpim:read",
	"search:read",
	"users:read",
	"users:read.email",
}

type AuthCmd struct {
	Login  AuthLoginCmd  `cmd:"" help:"Authenticate with Slack via OAuth"`
	Logout AuthLogoutCmd `cmd:"" help:"Remove stored credentials"`
	Status AuthStatusCmd `cmd:"" help:"Show authentication status"`
}

func resetAllAuth(cfg *config.Config) {
	cfg.Workspaces = map[string]config.WorkspaceAuth{}
	cfg.CurrentWorkspace = ""
	cfg.Token = ""
	cfg.ClientID = ""
	cfg.ClientSecret = ""
}

type AuthLoginCmd struct {
	ClientID     string `help:"Slack app client ID"`
	ClientSecret string `help:"Slack app client secret"`
	Replace      bool   `help:"Replace existing login for the target workspace"`
	AddNew       bool   `help:"Add another workspace instead of replacing current login"`
}

func (c *AuthLoginCmd) Run(ctx *Context) error {
	if c.Replace && c.AddNew {
		return fmt.Errorf("--replace and --add-new cannot be used together")
	}

	reader := bufio.NewReader(os.Stdin)
	workspaceRef := strings.TrimSpace(ctx.Workspace)
	replace := c.Replace
	useCurrentWorkspaceCredentials := true

	if workspaceRef == "" && !replace && !c.AddNew {
		if current := ctx.Config.CurrentWorkspace; current != "" {
			if auth, ok := ctx.Config.Workspaces[current]; ok && auth.Token != "" {
				display := workspaceURLForDisplay(current, auth, "")
				fmt.Printf("Existing login found for %s. Replace it instead of adding a new workspace? [y/N]: ", display)
				choice, err := reader.ReadString('\n')
				if err != nil {
					return fmt.Errorf("failed to read choice: %w", err)
				}
				fmt.Println()
				replace = strings.EqualFold(strings.TrimSpace(choice), "y") || strings.EqualFold(strings.TrimSpace(choice), "yes")
				if replace {
					workspaceRef = current
				} else {
					useCurrentWorkspaceCredentials = false
				}
			}
		}
	}

	if c.AddNew && workspaceRef == "" {
		useCurrentWorkspaceCredentials = false
	}

	if replace && workspaceRef == "" {
		workspaceRef = strings.TrimSpace(ctx.Config.CurrentWorkspace)
		if workspaceRef == "" {
			return fmt.Errorf("no current workspace to replace; pass --workspace or omit --replace")
		}
	}

	clientID, clientSecret, resolvedWorkspace, found, err := getOAuthCredentials(ctx.Config, workspaceRef, c.ClientID, c.ClientSecret, useCurrentWorkspaceCredentials)
	if err != nil {
		return err
	}
	if !found {
		printSlackAppSetupGuide()

		fmt.Print("Client ID: ")
		clientIDInput, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read client ID: %w", err)
		}
		clientID = strings.TrimSpace(clientIDInput)

		fmt.Print("Client Secret: ")
		clientSecretInput, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read client secret: %w", err)
		}
		clientSecret = strings.TrimSpace(clientSecretInput)

		if clientID == "" || clientSecret == "" {
			return fmt.Errorf("client ID and client secret are required")
		}
	}
	// Generate CSRF state parameter
	state, err := generateOAuthState()
	if err != nil {
		return fmt.Errorf("failed to generate OAuth state: %w", err)
	}

	// Create channel to receive the auth code
	codeChan := make(chan string, 1)
	errChan := make(chan error, 1)

	// Start local server to handle OAuth callback (bind to localhost only)
	listener, err := net.Listen("tcp", "127.0.0.1:"+oauthRedirectPort)
	if err != nil {
		return fmt.Errorf("failed to start local server: %w", err)
	}

	// Use dedicated ServeMux instead of global DefaultServeMux
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		// Verify CSRF state parameter
		if r.URL.Query().Get("state") != state {
			http.Error(w, "Invalid state parameter", http.StatusBadRequest)
			errChan <- fmt.Errorf("OAuth state mismatch - possible CSRF attack")
			return
		}

		code := r.URL.Query().Get("code")
		if code == "" {
			errMsg := r.URL.Query().Get("error")
			if errMsg == "" {
				errMsg = "no code received"
			}
			http.Error(w, "Authentication failed: "+errMsg, http.StatusBadRequest)
			errChan <- fmt.Errorf("authentication failed: %s", errMsg)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head>
	<meta charset="utf-8">
	<title>slack-cli</title>
	<style>
		body {
			font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
			display: flex;
			justify-content: center;
			align-items: center;
			min-height: 100vh;
			margin: 0;
			background: #f4f4f4;
		}
		.container {
			text-align: center;
			background: white;
			padding: 3rem 4rem;
			border-radius: 8px;
			box-shadow: 0 2px 10px rgba(0,0,0,0.1);
		}
		.check {
			font-size: 4rem;
			color: #2eb67d;
		}
		h1 { margin: 1rem 0 0.5rem; color: #1d1c1d; }
		p { color: #616061; }
	</style>
</head>
<body>
	<div class="container">
		<div class="check">&#10003;</div>
		<h1>Authentication successful</h1>
		<p>You can close this window and return to your terminal.</p>
	</div>
</body>
</html>`)
		codeChan <- code
	})

	server := &http.Server{Handler: mux}

	go func() {
		if err := server.Serve(listener); err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Build OAuth URL with state parameter
	scopeStr := ""
	for i, s := range oauthScopes {
		if i > 0 {
			scopeStr += ","
		}
		scopeStr += s
	}
	authURL := fmt.Sprintf(
		"https://slack.com/oauth/v2/authorize?client_id=%s&user_scope=%s&redirect_uri=%s&state=%s",
		clientID, scopeStr, oauthRedirectURL, state,
	)

	fmt.Println("Opening browser for Slack authentication...")
	fmt.Printf("If browser doesn't open, visit:\n%s\n\n", authURL)

	// Open browser
	openBrowser(authURL)

	// Wait for callback or timeout
	select {
	case code := <-codeChan:
		_ = server.Shutdown(context.Background())
		return c.exchangeCodeForToken(ctx, code, clientID, clientSecret, replace, c.AddNew, workspaceRef, resolvedWorkspace, reader)
	case err := <-errChan:
		_ = server.Shutdown(context.Background())
		return err
	case <-time.After(5 * time.Minute):
		_ = server.Shutdown(context.Background())
		return fmt.Errorf("authentication timed out")
	}
}

func (c *AuthLoginCmd) exchangeCodeForToken(ctx *Context, code, clientID, clientSecret string, replace bool, addNew bool, requestedWorkspace, resolvedWorkspace string, reader *bufio.Reader) error {
	token, err := slack.ExchangeOAuthCode(clientID, clientSecret, code, oauthRedirectURL)
	if err != nil {
		return fmt.Errorf("failed to exchange code for token: %w", err)
	}

	// Verify the token works
	client := slack.NewClient(token)
	user, err := client.AuthTest()
	if err != nil {
		return fmt.Errorf("token validation failed: %w", err)
	}

	workspaceHost := workspaceKeyFromAuthResult(user.URL, user.TeamID, user.Team)
	if workspaceHost == "" {
		return fmt.Errorf("unable to determine workspace from Slack auth response")
	}

	if !requestedWorkspaceMatchesAuthResult(requestedWorkspace, resolvedWorkspace, workspaceHost, user.TeamID) {
		expected := strings.TrimSpace(resolvedWorkspace)
		if expected == "" {
			expected = strings.TrimSpace(requestedWorkspace)
		}
		if expected == "" {
			expected = "<unknown>"
		}

		authenticated := workspaceHost
		if strings.TrimSpace(user.TeamID) != "" {
			authenticated = fmt.Sprintf("%s (%s)", workspaceHost, user.TeamID)
		}

		return fmt.Errorf("authenticated workspace %s does not match requested workspace %s", authenticated, expected)
	}

	previousCurrent := ctx.Config.CurrentWorkspace
	shouldSetDefault := shouldSetWorkspaceAsDefault(ctx.Config, previousCurrent, workspaceHost, replace)
	if !replace && previousCurrent != "" && !shouldSetDefault {
		setDefault, promptErr := promptSetDefaultWorkspace(reader, workspaceHost)
		if promptErr != nil {
			return fmt.Errorf("failed to read default workspace choice: %w", promptErr)
		}
		shouldSetDefault = setDefault
	}

	if existing, ok := ctx.Config.Workspaces[workspaceHost]; ok && existing.Token != "" {
		if addNew {
			return fmt.Errorf("workspace %s is already configured; rerun without --add-new or pass --replace", workspaceHost)
		}
		if !replace {
			fmt.Printf("Workspace %s is already configured. Replace existing login? [y/N]: ", workspaceHost)
			choice, readErr := reader.ReadString('\n')
			if readErr != nil {
				return fmt.Errorf("failed to read replace confirmation: %w", readErr)
			}
			normalized := strings.TrimSpace(choice)
			if !strings.EqualFold(normalized, "y") && !strings.EqualFold(normalized, "yes") {
				return fmt.Errorf("login cancelled")
			}
		}
	}

	ctx.Config.SetWorkspaceAuth(workspaceHost, config.WorkspaceAuth{
		Token:        token,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Team:         user.Team,
		TeamID:       user.TeamID,
		URL:          user.URL,
	})
	if err := ctx.Config.Save(); err != nil {
		return fmt.Errorf("failed to save token: %w", err)
	}

	if !shouldSetDefault {
		ctx.Config.CurrentWorkspace = previousCurrent
		if previousCurrent != "" {
			ctx.Config.Token = ctx.Config.Workspaces[previousCurrent].Token
		} else {
			ctx.Config.Token = ""
		}
		if err := ctx.Config.Save(); err != nil {
			return fmt.Errorf("failed to save default workspace selection: %w", err)
		}
	}

	fmt.Printf("Logged in as %s in workspace %s (%s)\n", user.User, user.Team, workspaceHost)
	if !shouldSetDefault && previousCurrent != "" {
		fmt.Printf("Default workspace remains %s\n", previousCurrent)
	}
	return nil
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		log.Printf("Warning: don't know how to open browser on %s", runtime.GOOS)
		return
	}
	if err := cmd.Start(); err != nil {
		log.Printf("Warning: failed to open browser: %v", err)
	}
}

type AuthLogoutCmd struct {
	All bool `help:"Log out of all configured workspaces and clear saved OAuth app credentials"`
}

func resolveWorkspaceForLogout(cfg *config.Config, requestedWorkspace string) (string, error) {
	workspace := strings.TrimSpace(requestedWorkspace)
	if workspace == "" {
		return cfg.CurrentWorkspace, nil
	}

	resolvedWorkspace, err := cfg.ResolveWorkspace(workspace)
	if err != nil {
		return "", err
	}
	return resolvedWorkspace, nil
}

func (c *AuthLogoutCmd) Run(ctx *Context) error {
	if c.All {
		if strings.TrimSpace(ctx.Workspace) != "" {
			return fmt.Errorf("--all cannot be used with --workspace")
		}

		resetAllAuth(ctx.Config)

		if err := ctx.Config.Save(); err != nil {
			return fmt.Errorf("failed to clear auth configuration: %w", err)
		}

		fmt.Println("Logged out from all workspaces")
		return nil
	}

	workspace, err := resolveWorkspaceForLogout(ctx.Config, ctx.Workspace)
	if err != nil {
		return fmt.Errorf("failed to resolve workspace for logout: %w", err)
	}

	if workspace != "" {
		delete(ctx.Config.Workspaces, strings.ToLower(workspace))
		if ctx.Config.CurrentWorkspace == strings.ToLower(workspace) {
			ctx.Config.CurrentWorkspace = ""
			if len(ctx.Config.Workspaces) > 0 {
				keys := make([]string, 0, len(ctx.Config.Workspaces))
				for k := range ctx.Config.Workspaces {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				ctx.Config.CurrentWorkspace = keys[0]
			}
		}
	}

	ctx.Config.Token = ""
	if ctx.Config.CurrentWorkspace != "" {
		ctx.Config.Token = ctx.Config.Workspaces[ctx.Config.CurrentWorkspace].Token
	}

	if err := ctx.Config.Save(); err != nil {
		return fmt.Errorf("failed to clear token: %w", err)
	}

	if workspace == "" {
		fmt.Println("Logged out successfully")
		return nil
	}
	fmt.Printf("Logged out workspace %s\n", workspace)
	return nil
}

type AuthStatusCmd struct{}

func workspaceURLForDisplay(workspaceKey string, auth config.WorkspaceAuth, fallbackURL string) string {
	if strings.TrimSpace(auth.URL) != "" {
		return auth.URL
	}

	if strings.TrimSpace(fallbackURL) != "" {
		return fallbackURL
	}

	if strings.HasSuffix(strings.ToLower(workspaceKey), ".slack.com") {
		return "https://" + workspaceKey + "/"
	}

	return workspaceKey
}

func (c *AuthStatusCmd) Run(ctx *Context) error {
	requestedWorkspace := strings.TrimSpace(ctx.Workspace)
	token, resolvedWorkspace, err := ctx.Config.TokenForWorkspace(requestedWorkspace)
	if err != nil {
		fmt.Println("Not logged in. Run 'slack-cli auth login' to authenticate.")
		return nil
	}

	client := slack.NewClient(token)
	user, err := client.AuthTest()
	if err != nil {
		fmt.Printf("Token invalid: %v\n", err)
		return nil
	}

	resolvedDisplay := resolvedWorkspace
	if auth, ok := ctx.Config.Workspaces[resolvedWorkspace]; ok {
		resolvedDisplay = workspaceURLForDisplay(resolvedWorkspace, auth, user.URL)
	}

	fmt.Printf("Logged in as %s in workspace %s (%s)\n", user.User, user.Team, resolvedDisplay)

	if len(ctx.Config.Workspaces) > 1 {
		keys := make([]string, 0, len(ctx.Config.Workspaces))
		for k := range ctx.Config.Workspaces {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fmt.Println("Configured workspaces:")
		for _, key := range keys {
			auth := ctx.Config.Workspaces[key]
			fallbackURL := ""
			if key == resolvedWorkspace {
				fallbackURL = user.URL
			}
			display := workspaceURLForDisplay(key, auth, fallbackURL)
			current := ""
			if key == ctx.Config.CurrentWorkspace {
				current = " (default)"
			}
			fmt.Printf("- %s%s\n", display, current)
		}
	}

	return nil
}
