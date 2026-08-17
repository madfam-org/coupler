// Package triggers is coupler's inbound half — the first piece of the
// "triggers" pillar in the ATP charter. A trigger endpoint authenticates a
// PROVIDER (webhook signature), never a janua user, which is why these
// handlers mount OUTSIDE the gateway's JWT middleware and carry their own
// verification.
//
// First source: Zoom. meeting.ended events become time-entry DRAFTS in nauta
// (its /api/integrations/coupler/time-draft door) — candidates for the hours
// ledger that a platform operator confirms in the cockpit. Coupler asserts
// nothing about billability; it relays what Zoom said, signed and deduped.
package triggers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"
)

// ZoomHandler verifies Zoom webhooks and dispatches meeting.ended to nauta.
type ZoomHandler struct {
	secretToken  string
	nautaURL     string
	nautaToken   string
	httpClient   *http.Client
	now          func() time.Time
	maxSkew      time.Duration
	maxBodyBytes int64
}

// NewZoomHandlerFromEnv builds the handler from the gateway's environment.
// With no ZOOM_WEBHOOK_SECRET_TOKEN the handler FAILS CLOSED (503 to
// everything): a misdeployed secret must read as an outage, never as an
// unauthenticated event sink.
func NewZoomHandlerFromEnv() *ZoomHandler {
	return &ZoomHandler{
		secretToken:  os.Getenv("ZOOM_WEBHOOK_SECRET_TOKEN"),
		nautaURL:     os.Getenv("COUPLER_NAUTA_INGEST_URL"),
		nautaToken:   os.Getenv("COUPLER_NAUTA_SERVICE_TOKEN"),
		httpClient:   &http.Client{Timeout: 10 * time.Second},
		now:          time.Now,
		maxSkew:      5 * time.Minute,
		maxBodyBytes: 1 << 20,
	}
}

type zoomEnvelope struct {
	Event   string `json:"event"`
	Payload struct {
		PlainToken string `json:"plainToken"`
		Object     struct {
			UUID      string `json:"uuid"`
			Topic     string `json:"topic"`
			HostEmail string `json:"host_email"`
			StartTime string `json:"start_time"`
			EndTime   string `json:"end_time"`
			Duration  int    `json:"duration"`
		} `json:"object"`
	} `json:"payload"`
}

func (h *ZoomHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.secretToken == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "trigger_not_configured"})
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, h.maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unreadable_body"})
		return
	}

	// Zoom signs every delivery, validation pings included:
	// v0=hex(hmac_sha256(secret, "v0:{ts}:{body}")), with the timestamp bounded
	// against replay.
	if !h.signatureOK(r, body) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_signature"})
		return
	}

	var env zoomEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}

	switch env.Event {
	case "endpoint.url_validation":
		// The challenge: prove secret possession by HMACing Zoom's plainToken.
		mac := hmac.New(sha256.New, []byte(h.secretToken))
		mac.Write([]byte(env.Payload.PlainToken))
		writeJSON(w, http.StatusOK, map[string]string{
			"plainToken":     env.Payload.PlainToken,
			"encryptedToken": hex.EncodeToString(mac.Sum(nil)),
		})
	case "meeting.ended":
		h.handleMeetingEnded(w, env)
	default:
		// Subscribed-but-unhandled events are acknowledged, not errored — a
		// 5xx would teach Zoom to retry traffic nobody consumes.
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "event": env.Event})
	}
}

func (h *ZoomHandler) signatureOK(r *http.Request, body []byte) bool {
	ts := r.Header.Get("x-zm-request-timestamp")
	sig := r.Header.Get("x-zm-signature")
	if ts == "" || sig == "" {
		return false
	}
	epoch, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return false
	}
	skew := h.now().Sub(time.Unix(epoch, 0))
	if skew < -h.maxSkew || skew > h.maxSkew {
		return false
	}
	mac := hmac.New(sha256.New, []byte(h.secretToken))
	fmt.Fprintf(mac, "v0:%s:", ts)
	mac.Write(body)
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(expected), []byte(sig)) == 1
}

func (h *ZoomHandler) handleMeetingEnded(w http.ResponseWriter, env zoomEnvelope) {
	obj := env.Payload.Object
	if obj.UUID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_meeting_uuid"})
		return
	}
	if h.nautaURL == "" || h.nautaToken == "" {
		// Verified event, nowhere to send it: 500 so Zoom retries once the
		// dispatch target is configured — a verified meeting must not vanish.
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "dispatch_not_configured"})
		return
	}

	minutes := obj.Duration
	occurredOn := h.now()
	if start, errS := time.Parse(time.RFC3339, obj.StartTime); errS == nil {
		if end, errE := time.Parse(time.RFC3339, obj.EndTime); errE == nil {
			// The actual wall clock beats Zoom's scheduled duration.
			minutes = int(math.Round(end.Sub(start).Minutes()))
			occurredOn = end
		}
	}
	if minutes < 1 {
		minutes = 1
	}
	if minutes > 1440 {
		minutes = 1440
	}

	draft := map[string]any{
		"source":      "zoom",
		"externalRef": obj.UUID,
		"occurredOn":  occurredOn.UTC().Format(time.RFC3339),
		"minutes":     minutes,
	}
	if obj.Topic != "" {
		draft["topic"] = obj.Topic
	}
	if obj.HostEmail != "" {
		draft["participantEmails"] = []string{obj.HostEmail}
	}

	payload, _ := json.Marshal(draft)
	req, err := http.NewRequest(http.MethodPost, h.nautaURL, bytes.NewReader(payload))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "dispatch_build_failed"})
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-coupler-service-token", h.nautaToken)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		log.Printf("triggers/zoom: nauta dispatch failed for meeting %q: %v", obj.UUID, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "dispatch_failed"})
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	if resp.StatusCode >= 500 {
		// Nauta down: let Zoom's retry machinery carry the event.
		log.Printf("triggers/zoom: nauta %d for meeting %q", resp.StatusCode, obj.UUID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "dispatch_unavailable"})
		return
	}
	if resp.StatusCode >= 400 {
		// Nauta REFUSED (bad token, invalid draft). Retrying the same event
		// cannot fix it — acknowledge to Zoom, keep the refusal loud in logs.
		log.Printf("triggers/zoom: nauta refused meeting %q: %d %s", obj.UUID, resp.StatusCode, string(respBody))
		writeJSON(w, http.StatusOK, map[string]string{"status": "refused_downstream"})
		return
	}

	log.Printf("triggers/zoom: draft dispatched for meeting %q (%d min)", obj.UUID, minutes)
	writeJSON(w, http.StatusOK, map[string]string{"status": "accepted"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
