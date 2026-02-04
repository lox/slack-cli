package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/lox/slack-cli/internal/slack"
)

// OAuth app credentials - users of this CLI share this app
// Override with SLACK_CLIENT_ID and SLACK_CLIENT_SECRET env vars
const (
	defaultClientID     = "10456709258641.10443798280178"
	defaultClientSecret = "63b3702b93b60311b36dbae96e272084"
	oauthRedirectPort   = "8338"
	oauthRedirectURL    = "http://localhost:" + oauthRedirectPort + "/callback"
)

func getOAuthCredentials() (clientID, clientSecret string) {
	clientID = os.Getenv("SLACK_CLIENT_ID")
	if clientID == "" {
		clientID = defaultClientID
	}
	clientSecret = os.Getenv("SLACK_CLIENT_SECRET")
	if clientSecret == "" {
		clientSecret = defaultClientSecret
	}
	return
}

// Scopes needed for the CLI
var oauthScopes = []string{
	"channels:history",
	"channels:read",
	"groups:history",
	"groups:read",
	"search:read",
	"users:read",
	"users:read.email",
}

type AuthCmd struct {
	Login  AuthLoginCmd  `cmd:"" help:"Authenticate with Slack via OAuth"`
	Logout AuthLogoutCmd `cmd:"" help:"Remove stored credentials"`
	Status AuthStatusCmd `cmd:"" help:"Show authentication status"`
}

type AuthLoginCmd struct{}

func (c *AuthLoginCmd) Run(ctx *Context) error {
	clientID, clientSecret := getOAuthCredentials()
	if clientID == "" || clientSecret == "" {
		return fmt.Errorf("OAuth not configured. Set SLACK_CLIENT_ID and SLACK_CLIENT_SECRET environment variables, or configure defaults in cmd/auth.go")
	}

	// Create channel to receive the auth code
	codeChan := make(chan string, 1)
	errChan := make(chan error, 1)

	// Start local server to handle OAuth callback
	listener, err := net.Listen("tcp", ":"+oauthRedirectPort)
	if err != nil {
		return fmt.Errorf("failed to start local server: %w", err)
	}

	server := &http.Server{}
	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
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
		fmt.Fprint(w, `<!DOCTYPE html>
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

	go func() {
		if err := server.Serve(listener); err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Build OAuth URL
	scopeStr := ""
	for i, s := range oauthScopes {
		if i > 0 {
			scopeStr += ","
		}
		scopeStr += s
	}
	authURL := fmt.Sprintf(
		"https://slack.com/oauth/v2/authorize?client_id=%s&user_scope=%s&redirect_uri=%s",
		clientID, scopeStr, oauthRedirectURL,
	)

	fmt.Println("Opening browser for Slack authentication...")
	fmt.Printf("If browser doesn't open, visit:\n%s\n\n", authURL)

	// Open browser
	openBrowser(authURL)

	// Wait for callback or timeout
	select {
	case code := <-codeChan:
		server.Shutdown(context.Background())
		return c.exchangeCodeForToken(ctx, code, clientID, clientSecret)
	case err := <-errChan:
		server.Shutdown(context.Background())
		return err
	case <-time.After(5 * time.Minute):
		server.Shutdown(context.Background())
		return fmt.Errorf("authentication timed out")
	}
}

func (c *AuthLoginCmd) exchangeCodeForToken(ctx *Context, code, clientID, clientSecret string) error {
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

	ctx.Config.Token = token
	if err := ctx.Config.Save(); err != nil {
		return fmt.Errorf("failed to save token: %w", err)
	}

	fmt.Printf("Logged in as %s in workspace %s\n", user.User, user.Team)
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
	}
	if cmd != nil {
		cmd.Start()
	}
}

type AuthLogoutCmd struct{}

func (c *AuthLogoutCmd) Run(ctx *Context) error {
	ctx.Config.Token = ""
	if err := ctx.Config.Save(); err != nil {
		return fmt.Errorf("failed to clear token: %w", err)
	}
	fmt.Println("Logged out successfully")
	return nil
}

type AuthStatusCmd struct{}

func (c *AuthStatusCmd) Run(ctx *Context) error {
	if ctx.Config.Token == "" {
		fmt.Println("Not logged in. Run 'slack auth login' to authenticate.")
		return nil
	}

	client := slack.NewClient(ctx.Config.Token)
	user, err := client.AuthTest()
	if err != nil {
		fmt.Printf("Token invalid: %v\n", err)
		return nil
	}

	fmt.Printf("Logged in as %s in workspace %s\n", user.User, user.Team)
	return nil
}
