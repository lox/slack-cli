package cmd

import (
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
	"time"

	"github.com/lox/slack-cli/internal/slack"
)

const (
	oauthRedirectPort = "8338"
	oauthRedirectURL  = "http://localhost:" + oauthRedirectPort + "/callback"
)

func getOAuthCredentials() (clientID, clientSecret string, err error) {
	clientID = os.Getenv("SLACK_CLIENT_ID")
	clientSecret = os.Getenv("SLACK_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		return "", "", fmt.Errorf("SLACK_CLIENT_ID and SLACK_CLIENT_SECRET environment variables must be set")
	}
	return clientID, clientSecret, nil
}

func generateOAuthState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
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
	clientID, clientSecret, err := getOAuthCredentials()
	if err != nil {
		return err
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
	default:
		log.Printf("Warning: don't know how to open browser on %s", runtime.GOOS)
		return
	}
	if err := cmd.Start(); err != nil {
		log.Printf("Warning: failed to open browser: %v", err)
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
