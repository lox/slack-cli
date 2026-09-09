package slack

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestRequestRetriesRateLimitsAndReplaysPOSTBody(t *testing.T) {
	calls := 0
	var waits []time.Duration
	client := &Client{
		userToken: "xoxp-test-token",
		sleep: func(duration time.Duration) {
			waits = append(waits, duration)
		},
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("read request body: %v", err)
				}
				if got, want := string(body), "value=hello"; got != want {
					t.Fatalf("request body on call %d = %q, want %q", calls, got, want)
				}
				if calls < 3 {
					return &http.Response{
						StatusCode: http.StatusTooManyRequests,
						Status:     "429 Too Many Requests",
						Header:     http.Header{"Retry-After": []string{fmt.Sprint(calls * 2)}},
						Body:       io.NopCloser(strings.NewReader("rate limited")),
						Request:    req,
					}, nil
				}
				return jsonResponse(req, `{"ok":true}`)
			}),
		},
	}

	if _, err := client.requestPost("test.method", url.Values{"value": {"hello"}}); err != nil {
		t.Fatalf("requestPost returned error: %v", err)
	}
	if want := []time.Duration{2 * time.Second, 4 * time.Second}; !slices.Equal(waits, want) {
		t.Errorf("retry waits = %v, want %v", waits, want)
	}
}

func TestRequestRetryPolicy(t *testing.T) {
	transportErr := errors.New("connection failed")
	tests := []struct {
		name         string
		status       int
		retryAfter   string
		transportErr error
		wantCalls    int
		wantWaits    int
		wantError    string
	}{
		{name: "exhausted", status: 429, retryAfter: "0", wantCalls: 4, wantWaits: 3, wantError: "rate limited after 3 retries"},
		{name: "invalid Retry-After", status: 429, retryAfter: "later", wantCalls: 1, wantError: "valid Retry-After"},
		{name: "server error", status: 500, wantCalls: 1, wantError: "HTTP 500"},
		{name: "transport error", transportErr: transportErr, wantCalls: 1, wantError: "connection failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls, waits := 0, 0
			client := &Client{
				sleep: func(time.Duration) { waits++ },
				httpClient: &http.Client{
					Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
						calls++
						if calls > tt.wantCalls {
							t.Fatalf("unexpected attempt %d, want at most %d", calls, tt.wantCalls)
						}
						if tt.transportErr != nil {
							return nil, tt.transportErr
						}
						return &http.Response{
							StatusCode: tt.status,
							Header:     http.Header{"Retry-After": []string{tt.retryAfter}},
							Body:       http.NoBody,
							Request:    req,
						}, nil
					}),
				},
			}
			_, err := client.request("test.method", nil)
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("request error = %v, want %q", err, tt.wantError)
			}
			if tt.transportErr != nil && !errors.Is(err, tt.transportErr) {
				t.Errorf("request error = %v, want wrapped transport error", err)
			}
			if calls != tt.wantCalls || waits != tt.wantWaits {
				t.Errorf("calls/waits = %d/%d, want %d/%d", calls, waits, tt.wantCalls, tt.wantWaits)
			}
		})
	}
}

func TestParseThreadURL(t *testing.T) {
	t.Run("message permalink uses message timestamp", func(t *testing.T) {
		channel, threadTS, err := ParseThreadURL("https://buildkite.slack.com/archives/C123/p1773973307481399")
		if err != nil {
			t.Fatalf("ParseThreadURL returned error: %v", err)
		}
		if channel != "C123" {
			t.Fatalf("expected channel C123, got %q", channel)
		}
		if threadTS != "1773973307.481399" {
			t.Fatalf("expected threadTS 1773973307.481399, got %q", threadTS)
		}
	})

	t.Run("reply permalink prefers thread_ts query parameter", func(t *testing.T) {
		channel, threadTS, err := ParseThreadURL("https://buildkite.slack.com/archives/C123/p1773999999000000?thread_ts=1773973307.481399&cid=C123")
		if err != nil {
			t.Fatalf("ParseThreadURL returned error: %v", err)
		}
		if channel != "C123" {
			t.Fatalf("expected channel C123, got %q", channel)
		}
		if threadTS != "1773973307.481399" {
			t.Fatalf("expected threadTS 1773973307.481399, got %q", threadTS)
		}
	})
}

func TestIsSlackHostedURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{name: "slack root", url: "https://slack.com/file.png", want: true},
		{name: "slack subdomain", url: "https://files.slack.com/file.png", want: true},
		{name: "http slack subdomain", url: "http://files.slack.com/file.png", want: false},
		{name: "uppercase host", url: "https://FILES.SLACK.COM/file.png", want: true},
		{name: "external host", url: "https://example.com/file.png", want: false},
		{name: "empty", url: "", want: false},
		{name: "invalid", url: "://bad-url", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSlackHostedURL(tt.url)
			if got != tt.want {
				t.Fatalf("isSlackHostedURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestDownloadPrivateFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("123456"))
	}))
	defer server.Close()

	client := &Client{
		userToken:  "xoxp-test-token",
		httpClient: server.Client(),
	}

	t.Run("within size limit", func(t *testing.T) {
		body, contentType, err := client.DownloadPrivateFile(server.URL, 6)
		if err != nil {
			t.Fatalf("DownloadPrivateFile returned error: %v", err)
		}
		if string(body) != "123456" {
			t.Fatalf("DownloadPrivateFile body = %q, want %q", string(body), "123456")
		}
		if contentType != "image/png" {
			t.Fatalf("DownloadPrivateFile contentType = %q, want %q", contentType, "image/png")
		}
	})

	t.Run("exceeds size limit", func(t *testing.T) {
		_, _, err := client.DownloadPrivateFile(server.URL, 5)
		if err == nil {
			t.Fatalf("DownloadPrivateFile expected error when payload exceeds size limit")
		}
		if !strings.Contains(err.Error(), "download exceeds limit") {
			t.Fatalf("DownloadPrivateFile error = %q, want contains %q", err.Error(), "download exceeds limit")
		}
	})

	t.Run("invalid limit", func(t *testing.T) {
		_, _, err := client.DownloadPrivateFile(server.URL, 0)
		if err == nil {
			t.Fatalf("DownloadPrivateFile expected error for invalid maxBytes")
		}
		if !strings.Contains(err.Error(), "maxBytes must be > 0") {
			t.Fatalf("DownloadPrivateFile error = %q, want contains %q", err.Error(), "maxBytes must be > 0")
		}
	})
}

func TestDownloadPrivateFile_AuthorizationHeaderPolicy(t *testing.T) {
	tests := []struct {
		name     string
		fileURL  string
		wantAuth string
	}{
		{name: "https slack host sends token", fileURL: "https://files.slack.com/files-pri/T123/F123/file.png", wantAuth: "Bearer [REDACTED:slack-access-token]"},
		{name: "http slack host does not send token", fileURL: "http://files.slack.com/files-pri/T123/F123/file.png", wantAuth: ""},
		{name: "https external host does not send token", fileURL: "https://example.com/file.png", wantAuth: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotAuth string

			client := &Client{
				userToken: "[REDACTED:slack-access-token]",
				httpClient: &http.Client{
					Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
						gotAuth = req.Header.Get("Authorization")
						return &http.Response{
							StatusCode: http.StatusOK,
							Status:     "200 OK",
							Header:     http.Header{"Content-Type": []string{"image/png"}},
							Body:       io.NopCloser(strings.NewReader("ok")),
							Request:    req,
						}, nil
					}),
				},
			}

			_, _, err := client.DownloadPrivateFile(tt.fileURL, 2)
			if err != nil {
				t.Fatalf("DownloadPrivateFile() returned error: %v", err)
			}

			if gotAuth != tt.wantAuth {
				t.Fatalf("DownloadPrivateFile() authorization header = %q, want %q", gotAuth, tt.wantAuth)
			}
		})
	}
}

func TestDownloadPrivateFileToWriter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("streamed"))
	}))
	defer server.Close()

	client := &Client{
		userToken:  "xoxp-test-token",
		httpClient: server.Client(),
	}

	var builder strings.Builder
	contentType, written, err := client.DownloadPrivateFileToWriter(server.URL, &builder)
	if err != nil {
		t.Fatalf("DownloadPrivateFileToWriter returned error: %v", err)
	}
	if contentType != "text/plain" {
		t.Fatalf("unexpected contentType %q", contentType)
	}
	if written != int64(len("streamed")) {
		t.Fatalf("unexpected bytes written %d", written)
	}
	if builder.String() != "streamed" {
		t.Fatalf("unexpected body %q", builder.String())
	}
}

func TestTransferHTTPClient_RemovesOverallTimeout(t *testing.T) {
	client := &Client{
		userToken: "xoxp-test-token",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	transferClient := client.transferHTTPClient()
	if transferClient == client.httpClient {
		t.Fatalf("transferHTTPClient should return a distinct client copy")
	}
	if transferClient.Timeout != 0 {
		t.Fatalf("transferHTTPClient timeout = %v, want 0", transferClient.Timeout)
	}
	if client.httpClient.Timeout != 30*time.Second {
		t.Fatalf("base client timeout mutated to %v", client.httpClient.Timeout)
	}
}

func TestOpenConversation_UsesPOSTFormEncoding(t *testing.T) {
	client := &Client{
		userToken: "xoxp-test-token",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodPost {
					t.Fatalf("expected POST, got %s", req.Method)
				}
				if req.URL.Path != "/api/conversations.open" {
					t.Fatalf("expected /api/conversations.open, got %s", req.URL.Path)
				}
				if got := req.Header.Get("Authorization"); got != "Bearer xoxp-test-token" {
					t.Fatalf("unexpected Authorization header %q", got)
				}

				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("failed to read request body: %v", err)
				}
				values, err := url.ParseQuery(string(body))
				if err != nil {
					t.Fatalf("failed to parse request body: %v", err)
				}
				if values.Get("users") != "U123" {
					t.Fatalf("expected users=U123, got %q", values.Get("users"))
				}
				if values.Get("return_im") != "true" {
					t.Fatalf("expected return_im=true, got %q", values.Get("return_im"))
				}

				return jsonResponse(req, `{"ok":true,"channel":{"id":"D123","user":"U123","is_im":true}}`)
			}),
		},
	}

	resp, err := client.OpenConversation([]string{"U123"}, true)
	if err != nil {
		t.Fatalf("OpenConversation returned error: %v", err)
	}
	if resp.Channel.ID != "D123" {
		t.Fatalf("expected channel D123, got %q", resp.Channel.ID)
	}
}

func TestListFiles(t *testing.T) {
	client := &Client{
		userToken: "xoxp-test-token",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodGet {
					t.Fatalf("expected GET, got %s", req.Method)
				}
				if req.URL.Path != "/api/files.list" {
					t.Fatalf("expected /api/files.list, got %s", req.URL.Path)
				}
				if req.URL.Query().Get("count") != "20" {
					t.Fatalf("expected count=20, got %q", req.URL.Query().Get("count"))
				}
				return jsonResponse(req, `{"ok":true,"files":[{"id":"F123","name":"report.txt","title":"Report","size":42}]}`)
			}),
		},
	}

	resp, err := client.ListFiles(20)
	if err != nil {
		t.Fatalf("ListFiles returned error: %v", err)
	}
	if len(resp.Files) != 1 || resp.Files[0].ID != "F123" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestListConversationsPage(t *testing.T) {
	client := &Client{
		userToken: "xoxp-test-token",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/api/conversations.list" {
					t.Fatalf("expected /api/conversations.list, got %s", req.URL.Path)
				}
				if req.URL.Query().Get("cursor") != "page-2" {
					t.Fatalf("expected cursor=page-2, got %q", req.URL.Query().Get("cursor"))
				}
				return jsonResponse(req, `{"ok":true,"channels":[{"id":"C123","name":"general"}],"response_metadata":{"next_cursor":"page-3"}}`)
			}),
		},
	}

	resp, err := client.ListConversationsPage("public_channel,private_channel", 1000, "page-2")
	if err != nil {
		t.Fatalf("ListConversationsPage returned error: %v", err)
	}
	if resp.ResponseMetadata.NextCursor != "page-3" {
		t.Fatalf("unexpected next cursor %q", resp.ResponseMetadata.NextCursor)
	}
}

func TestGetFileInfo(t *testing.T) {
	client := &Client{
		userToken: "xoxp-test-token",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/api/files.info" {
					t.Fatalf("expected /api/files.info, got %s", req.URL.Path)
				}
				if req.URL.Query().Get("file") != "F123" {
					t.Fatalf("expected file=F123, got %q", req.URL.Query().Get("file"))
				}
				return jsonResponse(req, `{"ok":true,"file":{"id":"F123","name":"report.txt","title":"Report","size":42}}`)
			}),
		},
	}

	file, err := client.GetFileInfo("F123")
	if err != nil {
		t.Fatalf("GetFileInfo returned error: %v", err)
	}
	if file.ID != "F123" || file.Name != "report.txt" {
		t.Fatalf("unexpected file: %+v", file)
	}
}

func TestDeleteFile(t *testing.T) {
	client := &Client{
		userToken: "xoxp-test-token",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodPost {
					t.Fatalf("expected POST, got %s", req.Method)
				}
				if req.URL.Path != "/api/files.delete" {
					t.Fatalf("expected /api/files.delete, got %s", req.URL.Path)
				}
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("failed to read request body: %v", err)
				}
				values, err := url.ParseQuery(string(body))
				if err != nil {
					t.Fatalf("failed to parse request body: %v", err)
				}
				if values.Get("file") != "F123" {
					t.Fatalf("expected file=F123, got %q", values.Get("file"))
				}
				return jsonResponse(req, `{"ok":true}`)
			}),
		},
	}

	if err := client.DeleteFile("F123"); err != nil {
		t.Fatalf("DeleteFile returned error: %v", err)
	}
}

func TestGetUploadURLExternal(t *testing.T) {
	client := &Client{
		userToken: "xoxp-test-token",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodPost {
					t.Fatalf("expected POST, got %s", req.Method)
				}
				if req.URL.Path != "/api/files.getUploadURLExternal" {
					t.Fatalf("expected /api/files.getUploadURLExternal, got %s", req.URL.Path)
				}
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("failed to read request body: %v", err)
				}
				values, err := url.ParseQuery(string(body))
				if err != nil {
					t.Fatalf("failed to parse request body: %v", err)
				}
				if values.Get("filename") != "report.txt" {
					t.Fatalf("expected filename=report.txt, got %q", values.Get("filename"))
				}
				if values.Get("length") != "42" {
					t.Fatalf("expected length=42, got %q", values.Get("length"))
				}
				return jsonResponse(req, `{"ok":true,"upload_url":"https://files.slack.com/upload/v1/abc","file_id":"F123"}`)
			}),
		},
	}

	resp, err := client.GetUploadURLExternal("report.txt", 42)
	if err != nil {
		t.Fatalf("GetUploadURLExternal returned error: %v", err)
	}
	if resp.UploadURL != "https://files.slack.com/upload/v1/abc" || resp.FileID != "F123" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestUploadExternalFile(t *testing.T) {
	var gotContentType string
	var gotLength int64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		gotContentType = r.Header.Get("Content-Type")
		gotLength = r.ContentLength
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read upload body: %v", err)
		}
		if string(body) != "hello" {
			t.Fatalf("unexpected upload body %q", string(body))
		}
		_, _ = w.Write([]byte("OK - 5"))
	}))
	defer server.Close()

	client := &Client{httpClient: server.Client()}
	if err := client.UploadExternalFile(server.URL, "report.txt", strings.NewReader("hello"), 5); err != nil {
		t.Fatalf("UploadExternalFile returned error: %v", err)
	}
	if gotContentType != "text/plain; charset=utf-8" {
		t.Fatalf("unexpected Content-Type %q", gotContentType)
	}
	if gotLength != 5 {
		t.Fatalf("unexpected Content-Length %d", gotLength)
	}
}

func TestCompleteUploadExternal(t *testing.T) {
	client := &Client{
		userToken: "xoxp-test-token",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodPost {
					t.Fatalf("expected POST, got %s", req.Method)
				}
				if req.URL.Path != "/api/files.completeUploadExternal" {
					t.Fatalf("expected /api/files.completeUploadExternal, got %s", req.URL.Path)
				}
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("failed to read request body: %v", err)
				}
				values, err := url.ParseQuery(string(body))
				if err != nil {
					t.Fatalf("failed to parse request body: %v", err)
				}
				if values.Get("channel_id") != "C123" {
					t.Fatalf("expected channel_id=C123, got %q", values.Get("channel_id"))
				}
				if values.Get("initial_comment") != "hello" {
					t.Fatalf("expected initial_comment=hello, got %q", values.Get("initial_comment"))
				}
				if values.Get("thread_ts") != "123.456" {
					t.Fatalf("expected thread_ts=123.456, got %q", values.Get("thread_ts"))
				}
				if values.Get("files") != `[{"id":"F123","title":"Report"}]` {
					t.Fatalf("unexpected files payload %q", values.Get("files"))
				}
				return jsonResponse(req, `{"ok":true,"files":[{"id":"F123","name":"report.txt","title":"Report"}]}`)
			}),
		},
	}

	resp, err := client.CompleteUploadExternal("F123", "Report", "C123", "hello", "123.456")
	if err != nil {
		t.Fatalf("CompleteUploadExternal returned error: %v", err)
	}
	if len(resp.Files) != 1 || resp.Files[0].ID != "F123" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestListUsersPage(t *testing.T) {
	client := &Client{
		userToken: "xoxp-test-token",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/api/users.list" {
					t.Fatalf("expected /api/users.list, got %s", req.URL.Path)
				}
				if req.URL.Query().Get("cursor") != "page-2" {
					t.Fatalf("expected cursor=page-2, got %q", req.URL.Query().Get("cursor"))
				}
				return jsonResponse(req, `{"ok":true,"members":[{"id":"U123","name":"alice"}],"response_metadata":{"next_cursor":"page-3"}}`)
			}),
		},
	}

	resp, err := client.ListUsersPage(1000, "page-2")
	if err != nil {
		t.Fatalf("ListUsersPage returned error: %v", err)
	}
	if resp.ResponseMetadata.NextCursor != "page-3" {
		t.Fatalf("unexpected next cursor %q", resp.ResponseMetadata.NextCursor)
	}
}

func jsonResponse(req *http.Request, body string) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
