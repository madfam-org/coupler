package executor

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/madfam-org/coupler/apps/gateway/internal/registry"
	"github.com/madfam-org/coupler/apps/gateway/internal/telegram"
)

func TestIntArg(t *testing.T) {
	args := map[string]any{"number": float64(42), "limit": "10"}
	if got := intArg(args, "number"); got != 42 {
		t.Fatalf("float64: got %d", got)
	}
	if got := intArg(args, "limit"); got != 10 {
		t.Fatalf("string: got %d", got)
	}
	if got := intArg(args, "missing"); got != 0 {
		t.Fatalf("missing: got %d", got)
	}
}

// Telegram is the one connector that must NEVER go through janua delegation:
// there is no ConnectedAccount for a static bot token. These tests exercise
// Execute() end-to-end with a nil janua client to prove the branch is taken
// unconditionally for tool.Connector == "telegram" — if the branch were ever
// removed or misordered, these would panic on the nil janua client instead of
// silently reaching for a fake OAuth connection.

const fakeToken = "999999:AAFakeExecutorTestToken"

func newTelegramExecutor(t *testing.T, base string) *Executor {
	t.Helper()
	tc := telegram.NewClientForTest(fakeToken, base)
	return &Executor{janua: nil, telegram: tc, client: &http.Client{}}
}

func TestExecuteTelegramNeverTouchesJanua(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, fakeToken) == false {
			t.Errorf("token not in expected path position: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7}}`))
	}))
	defer srv.Close()

	e := newTelegramExecutor(t, srv.URL)
	tool := registry.Tool{Name: "coupler.telegram.send_message", Connector: "telegram"}

	// janua is nil above: if Execute() ever called e.janua.ResolveConnectionID
	// for a telegram tool, this would panic (nil pointer deref) rather than
	// silently succeed — the test's real assertion is "this does not panic."
	res, err := e.Execute(context.Background(), tool, Request{
		Arguments:    map[string]any{"chat_id": "42", "text": "hello"},
		ActingUserID: "u1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Connector != "telegram" || res.Tool != "coupler.telegram.send_message" {
		t.Fatalf("bad result shape: %+v", res)
	}
	if res.Output["message_id"] != int64(7) {
		t.Fatalf("message_id not propagated: %v", res.Output)
	}
}

func TestExecuteTelegramGetMe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"result":{"id":1,"username":"madfam_bot"}}`))
	}))
	defer srv.Close()

	e := newTelegramExecutor(t, srv.URL)
	tool := registry.Tool{Name: "coupler.telegram.get_me", Connector: "telegram"}

	res, err := e.Execute(context.Background(), tool, Request{ActingUserID: "u1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Output["username"] != "madfam_bot" {
		t.Fatalf("bad output: %v", res.Output)
	}
}

func TestExecuteTelegramMessageTooLongNeverReachesNetwork(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	e := newTelegramExecutor(t, srv.URL)
	tool := registry.Tool{Name: "coupler.telegram.send_message", Connector: "telegram"}

	over := strings.Repeat("a", telegram.MaxMessageLength+1)
	_, err := e.Execute(context.Background(), tool, Request{
		Arguments:    map[string]any{"chat_id": "42", "text": over},
		ActingUserID: "u1",
	})
	if err == nil {
		t.Fatal("expected an error for over-length text")
	}
	if called {
		t.Fatal("4096-char cap must be enforced BEFORE any HTTP call")
	}
}

func TestExecuteTelegramSurfaces429WithRetryAfter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"description":"Too Many Requests","parameters":{"retry_after":12}}`))
	}))
	defer srv.Close()

	e := newTelegramExecutor(t, srv.URL)
	tool := registry.Tool{Name: "coupler.telegram.send_message", Connector: "telegram"}

	_, err := e.Execute(context.Background(), tool, Request{
		Arguments:    map[string]any{"chat_id": "42", "text": "hi"},
		ActingUserID: "u1",
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	var apiErr *telegram.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("want *telegram.APIError, got %T: %v", err, err)
	}
	if apiErr.RetryAfter != 12 {
		t.Fatalf("retry_after not surfaced through executor: %d", apiErr.RetryAfter)
	}
}

// SECURITY: the executor's Result.Output (what handleExecute serializes back
// to the caller) must never carry the bot token in any field, and Execute's
// returned error must never carry it either — this is the boundary an actual
// HTTP response body crosses on the way out of coupler.
func TestExecuteTelegramNeverLeaksTokenInResultOrError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"description":"chat not found"}`))
	}))
	defer srv.Close()

	e := newTelegramExecutor(t, srv.URL)
	tool := registry.Tool{Name: "coupler.telegram.send_message", Connector: "telegram"}

	res, err := e.Execute(context.Background(), tool, Request{
		Arguments:    map[string]any{"chat_id": "bogus", "text": "hi"},
		ActingUserID: "u1",
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), fakeToken) {
		t.Fatalf("SECURITY: token leaked into Execute() error: %q", err.Error())
	}
	for k, v := range res.Output {
		if s, ok := v.(string); ok && strings.Contains(s, fakeToken) {
			t.Fatalf("SECURITY: token leaked into Result.Output[%q]: %q", k, s)
		}
	}
}
