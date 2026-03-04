package slack

import (
	"fmt"
	"net/url"
	"strings"
)

// ExtractWorkspaceRef returns workspace host and/or team ID from a Slack URL.
func ExtractWorkspaceRef(rawURL string) (workspaceHost string, teamID string, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid URL: %w", err)
	}

	host := strings.ToLower(u.Host)
	if host != "slack.com" && !strings.HasSuffix(host, ".slack.com") {
		return "", "", fmt.Errorf("not a Slack URL")
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) >= 2 && parts[0] == "client" && strings.HasPrefix(parts[1], "T") {
		teamID = parts[1]
	}

	if host != "slack.com" && host != "app.slack.com" {
		workspaceHost = host
	}

	return workspaceHost, teamID, nil
}
