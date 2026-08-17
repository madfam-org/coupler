package triggers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The trigger's whole contract: fail closed unconfigured, admit only
// signed-and-fresh deliveries, answer the validation challenge, relay
// meeting.ended as a draft with the actual wall clock, and translate nauta's
// answer into Zoom's retry semantics (5xx retries, 4xx does not).

const secret = "zoom-secret"

var frozenNow = time.Date(2026, 8, 17, 18, 0, 0, 0, time.UTC)

func newHandler(nautaURL string) *ZoomHandler {
	return &ZoomHandler{
		secretToken:  secret,
		nautaURL:     nautaURL,
		nautaToken:   "nauta-token",
		httpClient:   &http.Client{Timeout: 5 * time.Second},
		now:          func() time.Time { return frozenNow },
		maxSkew:      5 * time.Minute,
		maxBodyBytes: 1 << 20,
	}
}

func sign(ts string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "v0:%s:", ts)
	mac.Write(body)
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}

func deliver(t *testing.T, h *ZoomHandler, body []byte, mutate func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/triggers/zoom", strings.NewReader(string(body)))
	ts := strconv.FormatInt(frozenNow.Unix(), 10)
	req.Header.Set("x-zm-request-timestamp", ts)
	req.Header.Set("x-zm-signature", sign(ts, body))
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestFailsClosedWithoutSecret(t *testing.T) {
	h := newHandler("http://unused")
	h.secretToken = ""
	rec := deliver(t, h, []byte(`{"event":"meeting.ended"}`), nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rec.Code)
	}
}

func TestRejectsBadSignature(t *testing.T) {
	h := newHandler("http://unused")
	rec := deliver(t, h, []byte(`{"event":"meeting.ended"}`), func(r *http.Request) {
		r.Header.Set("x-zm-signature", "v0=deadbeef")
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestRejectsStaleTimestamp(t *testing.T) {
	h := newHandler("http://unused")
	body := []byte(`{"event":"meeting.ended"}`)
	stale := strconv.FormatInt(frozenNow.Add(-time.Hour).Unix(), 10)
	rec := deliver(t, h, body, func(r *http.Request) {
		r.Header.Set("x-zm-request-timestamp", stale)
		r.Header.Set("x-zm-signature", sign(stale, body))
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for stale ts, got %d", rec.Code)
	}
}

func TestURLValidationChallenge(t *testing.T) {
	h := newHandler("http://unused")
	body := []byte(`{"event":"endpoint.url_validation","payload":{"plainToken":"abc123"}}`)
	rec := deliver(t, h, body, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var out map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("abc123"))
	if out["plainToken"] != "abc123" || out["encryptedToken"] != hex.EncodeToString(mac.Sum(nil)) {
		t.Fatalf("bad challenge answer: %v", out)
	}
}

func meetingEndedBody() []byte {
	return []byte(`{"event":"meeting.ended","payload":{"object":{
		"uuid":"mtg-uuid-1","topic":"Kickoff","host_email":"aldo@madfam.io",
		"start_time":"2026-08-17T16:00:00Z","end_time":"2026-08-17T17:02:00Z","duration":45}}}`)
}

func TestMeetingEndedDispatchesDraft(t *testing.T) {
	var got map[string]any
	var gotToken string
	nauta := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("x-coupler-service-token")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"draftId":"d1"}`))
	}))
	defer nauta.Close()

	rec := deliver(t, newHandler(nauta.URL), meetingEndedBody(), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if gotToken != "nauta-token" {
		t.Fatalf("service token not forwarded, got %q", gotToken)
	}
	// 62 real minutes beat the scheduled 45.
	if got["minutes"].(float64) != 62 {
		t.Fatalf("want wall-clock 62 min, got %v", got["minutes"])
	}
	if got["externalRef"] != "mtg-uuid-1" || got["source"] != "zoom" {
		t.Fatalf("bad draft identity: %v", got)
	}
	emails, _ := got["participantEmails"].([]any)
	if len(emails) != 1 || emails[0] != "aldo@madfam.io" {
		t.Fatalf("host email not relayed: %v", got["participantEmails"])
	}
}

func TestNautaOutageAsksZoomToRetry(t *testing.T) {
	nauta := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer nauta.Close()
	rec := deliver(t, newHandler(nauta.URL), meetingEndedBody(), nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 (Zoom retries), got %d", rec.Code)
	}
}

func TestNautaRefusalIsAcknowledgedNotRetried(t *testing.T) {
	nauta := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer nauta.Close()
	rec := deliver(t, newHandler(nauta.URL), meetingEndedBody(), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 ack for downstream refusal, got %d", rec.Code)
	}
	var out map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out["status"] != "refused_downstream" {
		t.Fatalf("refusal must stay visible in the answer, got %v", out)
	}
}

func TestUnhandledEventIsAcknowledged(t *testing.T) {
	called := false
	nauta := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { called = true }))
	defer nauta.Close()
	rec := deliver(t, newHandler(nauta.URL), []byte(`{"event":"meeting.started","payload":{"object":{"uuid":"x"}}}`), nil)
	if rec.Code != http.StatusOK || called {
		t.Fatalf("want 200 without dispatch, got %d (dispatched=%v)", rec.Code, called)
	}
}
