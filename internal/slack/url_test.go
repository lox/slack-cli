package slack

import "testing"

func TestExtractWorkspaceRef(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		host    string
		teamID  string
		wantErr bool
	}{
		{
			name:   "workspace host URL",
			url:    "https://buildkite.slack.com/archives/C123/p1234567890123456",
			host:   "buildkite.slack.com",
			teamID: "",
		},
		{
			name:   "app slack URL with team",
			url:    "https://app.slack.com/client/T123/C456/thread/C456-1234567890.123456",
			host:   "",
			teamID: "T123",
		},
		{
			name:    "non slack URL",
			url:     "https://example.com/path",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, teamID, err := ExtractWorkspaceRef(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("ExtractWorkspaceRef returned error: %v", err)
			}
			if host != tt.host {
				t.Fatalf("expected host %q, got %q", tt.host, host)
			}
			if teamID != tt.teamID {
				t.Fatalf("expected teamID %q, got %q", tt.teamID, teamID)
			}
		})
	}
}
