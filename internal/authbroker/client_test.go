package authbroker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLogin(t *testing.T) {
	const id = "session_identifier_123456789"
	const secret = "private_polling_credential_123456789"
	var origin string
	polls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method %s", r.Method)
		}
		if r.URL.Path == "/v1/sessions" {
			if r.Header.Get("Authorization") != "" {
				t.Error("session creation included polling credential")
			}
			var request map[string]string
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request["team_id"] != "TEXAMPLE" || len(request) != 1 {
				t.Error("session request did not specify only the requested workspace")
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprintf(w, `{"session_id":%q,"browser_url":%q,"poll_token":%q,"confirmation_code":"AB12CD34","expires_in":600,"interval":3}`, id, origin+"/authorize?session="+id, secret)
			return
		}
		if r.URL.Path != "/v1/sessions/"+id+"/poll" || r.Header.Get("Authorization") != "Bearer "+secret {
			t.Errorf("incorrect polling request: %s", r.URL.Path)
		}
		polls++
		switch polls {
		case 1:
			w.WriteHeader(http.StatusAccepted)
			_, _ = fmt.Fprint(w, `{"status":"pending"}`)
		case 2:
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = fmt.Fprint(w, `{"error":"slow_down"}`)
		default:
			_, _ = fmt.Fprint(w, `{"access_token":"xoxp-example-token","token_type":"user","team_id":"TEXAMPLE","user_id":"UEXAMPLE","scope":"channels:read"}`)
		}
	}))
	defer server.Close()
	origin = server.URL
	client, err := New(origin)
	if err != nil {
		t.Fatal(err)
	}
	client.http.Transport = server.Client().Transport
	var delays []time.Duration
	client.wait = func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return nil
	}
	shown := false
	token, err := client.Login(context.Background(), "TEXAMPLE", func(session Session) error {
		shown = true
		if session.BrowserURL != origin+"/authorize?session="+id || session.ConfirmationCode != "AB12CD34" {
			t.Fatalf("incorrect browser instructions: %+v", session)
		}
		return nil
	})
	if err != nil || token != "xoxp-example-token" || !shown || polls != 3 {
		t.Fatalf("login: token received=%t shown=%t polls=%d error=%v", token != "", shown, polls, err)
	}
	if fmt.Sprint(delays) != "[3s 3s 7s]" {
		t.Fatalf("poll delays: %v", delays)
	}
}

func TestLoginRejectsUnsafeResponsesWithoutRetry(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"wrong workspace", 200, `{"access_token":"xoxp-secret","token_type":"user","team_id":"TOTHER","user_id":"UEXAMPLE"}`},
		{"bot token", 200, `{"access_token":"xoxb-secret","token_type":"bot","team_id":"TEXAMPLE","user_id":"UEXAMPLE"}`},
		{"truncated delivery", 200, `{"access_token":"xoxp-secret"`},
		{"expired", 410, `{"error":"session_gone"}`},
		{"reflected error", 400, `{"error":"xoxp-secret"}`},
		{"redirect", 307, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var origin string
			polls := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/sessions" {
					w.WriteHeader(201)
					_, _ = fmt.Fprintf(w, `{"session_id":"session_identifier_123456","browser_url":%q,"poll_token":"private_polling_credential_123456","confirmation_code":"AB12CD34","expires_in":600,"interval":3}`, origin+"/authorize?session=session_identifier_123456")
					return
				}
				polls++
				w.Header().Set("Location", origin+"/leaked")
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			origin = server.URL
			client, _ := New(origin)
			client.http.Transport = server.Client().Transport
			client.wait = func(context.Context, time.Duration) error { return nil }
			token, err := client.Login(context.Background(), "TEXAMPLE", func(Session) error { return nil })
			if err == nil || token != "" || polls != 1 {
				t.Fatalf("expected terminal rejection: polls=%d token received=%t error=%v", polls, token != "", err)
			}
			if strings.Contains(err.Error(), "xoxp-secret") {
				t.Fatal("error exposed response credential")
			}
		})
	}
}

func TestOriginValidation(t *testing.T) {
	for _, value := range []string{"http://example.com", "https://user:secret@example.com", "https://example.com/path", "https://example.com?token=secret", "https://example.com#fragment"} {
		if _, err := New(value); err == nil {
			t.Errorf("accepted unsafe origin %q", value)
		}
	}
}

func TestWaitCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := wait(ctx, time.Hour); err != context.Canceled {
		t.Fatalf("cancelled wait returned %v", err)
	}
}

func TestLoginRejectsInvalidSessionBeforeShowingBrowser(t *testing.T) {
	for _, field := range []string{"browser_url", "session_id", "confirmation_code", "expires_in", "interval"} {
		t.Run(field, func(t *testing.T) {
			var origin string
			requests := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				session := map[string]any{
					"session_id": "session_identifier_123456", "poll_token": "private_polling_credential_123456",
					"browser_url":       origin + "/authorize?session=session_identifier_123456",
					"confirmation_code": "AB12CD34", "expires_in": 600, "interval": 3,
				}
				switch field {
				case "browser_url":
					session[field] = "https://other.example.com/authorize?session=session_identifier_123456"
				case "session_id":
					session[field] = "../different-endpoint"
				case "confirmation_code":
					session[field] = "\x1b[31m"
				case "expires_in":
					session[field] = 601
				case "interval":
					session[field] = 0
				}
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(session)
			}))
			defer server.Close()
			origin = server.URL
			client, _ := New(origin)
			client.http.Transport = server.Client().Transport
			_, err := client.Login(context.Background(), "TEXAMPLE", func(Session) error {
				t.Error("displayed unsafe browser instructions")
				return fmt.Errorf("unexpected display")
			})
			if err == nil || requests != 1 {
				t.Fatalf("unsafe session not rejected immediately: requests=%d err=%v", requests, err)
			}
		})
	}
}
