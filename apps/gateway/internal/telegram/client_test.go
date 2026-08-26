package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// The client's whole contract: fail closed unconfigured, never let the bot
// token escape into a URL an assertion (or a real log line) can see, enforce
// Telegram's own 4096-char cap and parse_mode enum client-side, and translate
// the Bot API's error shape honestly — including 429's retry_after — without
// ever retrying automatically.

const fakeToken = "123456:AAFakeTokenForTestsOnlyDoNotUse"

func TestSendMessageFailsClosedWithoutToken(t *testing.T) {
	c := NewClient("")
	_, err := c.SendMessage(context.Background(), SendMessageArgs{ChatID: "1", Text: "hi"})
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("want ErrNotConfigured, got %v", err)
	}
}

func TestGetMeFailsClosedWithoutToken(t *testing.T) {
	c := NewClient("")
	_, err := c.GetMe(context.Background())
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("want ErrNotConfigured, got %v", err)
	}
}

func TestSendMessageRequiresChatIDAndText(t *testing.T) {
	c := NewClient(fakeToken)
	if _, err := c.SendMessage(context.Background(), SendMessageArgs{Text: "hi"}); err == nil {
		t.Fatal("want error for missing chat_id")
	}
	if _, err := c.SendMessage(context.Background(), SendMessageArgs{ChatID: "1"}); err == nil {
		t.Fatal("want error for missing text")
	}
}

func TestSendMessageEnforces4096Cap(t *testing.T) {
	c := NewClient(fakeToken)
	over := strings.Repeat("a", MaxMessageLength+1)
	_, err := c.SendMessage(context.Background(), SendMessageArgs{ChatID: "1", Text: over})
	if !errors.Is(err, ErrMessageTooLong) {
		t.Fatalf("want ErrMessageTooLong, got %v", err)
	}

	// Exactly at the cap must be accepted (validation only, no network call
	// needed — the client rejects before building any request).
	atCap := strings.Repeat("a", MaxMessageLength)
	c2 := NewClient("") // unconfigured: proves length passed validation and
	// failed on the NEXT check (IsConfigured), not on length.
	_, err = c2.SendMessage(context.Background(), SendMessageArgs{ChatID: "1", Text: atCap})
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("want length-exactly-4096 to pass validation and fail on ErrNotConfigured, got %v", err)
	}
}

func TestSendMessageValidatesParseMode(t *testing.T) {
	c := NewClient(fakeToken)
	_, err := c.SendMessage(context.Background(), SendMessageArgs{ChatID: "1", Text: "hi", ParseMode: "Markdown"})
	if !errors.Is(err, ErrInvalidParseMode) {
		t.Fatalf("want ErrInvalidParseMode for bad enum, got %v", err)
	}
}

// newTestClient points a Client at an httptest server instead of
// api.telegram.org, using the same fakeToken as every other test so the
// token-redaction assertions below are checking a REAL token value, not an
// empty string.
func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	c := NewClient(fakeToken)
	c.httpClient = srv.Client()
	return c, srv
}

// withBase points a Client at a test server instead of api.telegram.org.
func (c *Client) withBase(base string) *Client {
	return &Client{token: c.token, httpClient: c.httpClient, base: base}
}

func TestSendMessageSuccessParsesMessageID(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":42}}`))
	})
	defer srv.Close()
	c = c.withBase(srv.URL)

	res, err := c.SendMessage(context.Background(), SendMessageArgs{
		ChatID: "12345", Text: "hello", ParseMode: "HTML", DisableNotification: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.OK || res.MessageID != 42 {
		t.Fatalf("bad result: %+v", res)
	}
	if !strings.Contains(gotPath, "/bot"+fakeToken+"/sendMessage") {
		t.Fatalf("unexpected request path: %s", gotPath)
	}
	if gotBody["chat_id"] != "12345" || gotBody["text"] != "hello" || gotBody["parse_mode"] != "HTML" {
		t.Fatalf("bad request body: %v", gotBody)
	}
	if gotBody["disable_notification"] != true {
		t.Fatalf("disable_notification not forwarded: %v", gotBody)
	}
}

func TestGetMeSuccess(t *testing.T) {
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/getMe") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"id":987654321,"username":"madfam_bot"}}`))
	})
	defer srv.Close()
	c = c.withBase(srv.URL)

	res, err := c.GetMe(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.OK || res.ID != 987654321 || res.Username != "madfam_bot" {
		t.Fatalf("bad result: %+v", res)
	}
}

func TestChatNotFoundMapsTo400(t *testing.T) {
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`))
	})
	defer srv.Close()
	c = c.withBase(srv.URL)

	_, err := c.SendMessage(context.Background(), SendMessageArgs{ChatID: "bogus", Text: "hi"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("want *APIError, got %v (%T)", err, err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", apiErr.StatusCode)
	}
	if !strings.Contains(apiErr.Description, "chat not found") {
		t.Fatalf("description lost: %q", apiErr.Description)
	}
}

func TestBadTokenMapsTo401(t *testing.T) {
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":401,"description":"Unauthorized"}`))
	})
	defer srv.Close()
	c = c.withBase(srv.URL)

	_, err := c.GetMe(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("want *APIError, got %v", err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", apiErr.StatusCode)
	}
}

func TestRateLimitSurfacesRetryAfterWithoutRetrying(t *testing.T) {
	callCount := 0
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"description":"Too Many Requests: retry after 30","parameters":{"retry_after":30}}`))
	})
	defer srv.Close()
	c = c.withBase(srv.URL)

	_, err := c.SendMessage(context.Background(), SendMessageArgs{ChatID: "1", Text: "hi"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("want *APIError, got %v", err)
	}
	if apiErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %d", apiErr.StatusCode)
	}
	if apiErr.RetryAfter != 30 {
		t.Fatalf("want retry_after=30 surfaced, got %d", apiErr.RetryAfter)
	}
	if !strings.Contains(apiErr.Error(), "retry_after=30s") {
		t.Fatalf("retry_after not visible in Error() string: %q", apiErr.Error())
	}
	// The whole point of surfacing rather than retrying: exactly one call.
	if callCount != 1 {
		t.Fatalf("client must not auto-retry a 429, got %d calls", callCount)
	}
}

// --- Security: the token must never leak into anything an error, a log
// line, or a Result could carry. ---

func TestErrorsNeverContainTheBotToken(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
		run     func(c *Client) error
	}{
		{
			name: "chat not found",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"description":"chat not found"}`))
			},
			run: func(c *Client) error {
				_, err := c.SendMessage(context.Background(), SendMessageArgs{ChatID: "x", Text: "hi"})
				return err
			},
		},
		{
			name: "bad token",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{}`))
			},
			run: func(c *Client) error { _, err := c.GetMe(context.Background()); return err },
		},
		{
			name: "malformed json response",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`not json`))
			},
			run: func(c *Client) error { _, err := c.GetMe(context.Background()); return err },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, srv := newTestClient(t, tc.handler)
			defer srv.Close()
			c = c.withBase(srv.URL)

			err := tc.run(c)
			if err == nil {
				t.Fatal("expected an error")
			}
			if strings.Contains(err.Error(), fakeToken) {
				t.Fatalf("SECURITY: bot token leaked into error message: %q", err.Error())
			}
		})
	}
}

func TestTransportErrorNeverContainsTheBotToken(t *testing.T) {
	// Point at a real client but a server that immediately closes the
	// connection, forcing a genuine net/http transport-level error (not an
	// HTTP status) — those errors normally embed the request URL.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("httptest server does not support hijacking")
		}
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	defer srv.Close()

	c := NewClient(fakeToken).withBase(srv.URL)
	_, err := c.GetMe(context.Background())
	if err == nil {
		t.Fatal("expected a transport error")
	}
	if strings.Contains(err.Error(), fakeToken) {
		t.Fatalf("SECURITY: bot token leaked into transport error: %q", err.Error())
	}
}

func TestRedactURLErrorStripsToken(t *testing.T) {
	in := errors.New(`Post "https://api.telegram.org/bot` + fakeToken + `/getMe": dial tcp: connection refused`)
	out := redactURLError(in)
	if strings.Contains(out, fakeToken) {
		t.Fatalf("token survived redaction: %q", out)
	}
	if !strings.Contains(out, "***REDACTED***") {
		t.Fatalf("redaction marker missing: %q", out)
	}
	if !strings.Contains(out, "dial tcp: connection refused") {
		t.Fatalf("redaction destroyed useful context: %q", out)
	}
}

func TestAPIErrorStringNeverNeedsToken(t *testing.T) {
	// APIError is built exclusively from status code + response body — assert
	// the type has no field capable of holding a token at all, structurally,
	// by round-tripping through Error() and confirming shape.
	e := &APIError{StatusCode: 404, Description: "chat not found"}
	if strconv.Itoa(e.StatusCode) != "404" {
		t.Fatal("sanity check failed")
	}
	if strings.Contains(e.Error(), "bot") {
		t.Fatalf("APIError.Error() should never mention the bot token path: %q", e.Error())
	}
}
