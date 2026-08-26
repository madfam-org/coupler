// Package telegram is coupler's Telegram Bot API client. It exists because
// Telegram bots do not fit the janua OAuth-delegation model every other
// connector uses: a bot token is a single static secret minted once via
// BotFather, not a per-user authorize flow with a refresh token to broker.
// There is no ConnectedAccount to resolve here — the token is held
// server-side as a gateway secret (COUPLER_TELEGRAM_BOT_TOKEN) and shared by
// every caller. See docs/CONNECTOR_TELEGRAM.md for the full rationale.
//
// The owner has named Telegram MADFAM's primary human-notification channel
// (Slack is unusable for him); this client is the interim general-purpose
// "send a Telegram message" tool until the angelia Matrix bridge ships.
package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	// MaxMessageLength is Telegram's hard cap on sendMessage text bodies.
	MaxMessageLength = 4096

	apiBase = "https://api.telegram.org"
)

// ErrNotConfigured signals the bot token is missing — callers must fail
// closed, never fall back to an unauthenticated call.
var ErrNotConfigured = errors.New("telegram: COUPLER_TELEGRAM_BOT_TOKEN not configured")

// ErrMessageTooLong signals the text argument exceeds Telegram's 4096-char
// cap. Enforced client-side so the failure is immediate and the token-bearing
// URL is never even constructed for a request Telegram would reject anyway.
var ErrMessageTooLong = fmt.Errorf("telegram: text exceeds %d character limit", MaxMessageLength)

// ErrInvalidParseMode signals a parse_mode outside Telegram's own enum.
var ErrInvalidParseMode = errors.New("telegram: parse_mode must be MarkdownV2 or HTML")

var validParseModes = map[string]bool{
	"MarkdownV2": true,
	"HTML":       true,
}

// APIError carries a Telegram Bot API error response. RetryAfter is set only
// for 429 responses (Retry-After / parameters.retry_after) and is surfaced
// to the caller rather than acted on — this client never auto-retries.
type APIError struct {
	StatusCode  int
	Description string
	RetryAfter  int // seconds; 0 unless the error is a 429
}

func (e *APIError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("telegram api %d: %s (retry_after=%ds)", e.StatusCode, e.Description, e.RetryAfter)
	}
	return fmt.Sprintf("telegram api %d: %s", e.StatusCode, e.Description)
}

// Client calls the Telegram Bot API using a server-held token.
type Client struct {
	token      string
	base       string // defaults to apiBase; overridable only for tests
	httpClient *http.Client
}

// NewClientFromEnv builds a Client from COUPLER_TELEGRAM_BOT_TOKEN, mirroring
// triggers.NewZoomHandlerFromEnv: with no token configured the client reports
// IsConfigured() == false and every call fails closed with ErrNotConfigured
// rather than making an unauthenticated request.
func NewClientFromEnv() *Client {
	return &Client{
		token:      os.Getenv("COUPLER_TELEGRAM_BOT_TOKEN"),
		base:       apiBase,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// NewClient builds a Client from an explicit token — used by tests to avoid
// touching process environment.
func NewClient(token string) *Client {
	return &Client{token: token, base: apiBase, httpClient: &http.Client{Timeout: 15 * time.Second}}
}

// NewClientForTest builds a Client pointed at an arbitrary base URL (an
// httptest server) instead of api.telegram.org. Exported specifically so
// other packages' tests (e.g. executor_test.go, which must prove the
// executor never routes telegram tools through janua) can exercise a real
// Client without touching api.telegram.org or process environment.
func NewClientForTest(token, base string) *Client {
	return &Client{token: token, base: base, httpClient: &http.Client{Timeout: 5 * time.Second}}
}

// IsConfigured reports whether a bot token is present.
func (c *Client) IsConfigured() bool {
	return c.token != ""
}

// SendMessageArgs are the validated inputs to SendMessage.
type SendMessageArgs struct {
	ChatID              string
	Text                string
	ParseMode           string // optional; "" means Telegram's default
	DisableNotification bool
}

// SendMessageResult is the normalized, token-free result surfaced to callers.
type SendMessageResult struct {
	OK        bool  `json:"ok"`
	MessageID int64 `json:"message_id"`
}

// SendMessage posts to the Bot API's sendMessage endpoint. It validates
// length and parse_mode BEFORE building any request, so a rejected call never
// even constructs the token-bearing URL.
func (c *Client) SendMessage(ctx context.Context, args SendMessageArgs) (SendMessageResult, error) {
	if !c.IsConfigured() {
		return SendMessageResult{}, ErrNotConfigured
	}
	if args.ChatID == "" {
		return SendMessageResult{}, errors.New("telegram: chat_id required")
	}
	if args.Text == "" {
		return SendMessageResult{}, errors.New("telegram: text required")
	}
	if len(args.Text) > MaxMessageLength {
		return SendMessageResult{}, ErrMessageTooLong
	}
	if args.ParseMode != "" && !validParseModes[args.ParseMode] {
		return SendMessageResult{}, ErrInvalidParseMode
	}

	payload := map[string]any{
		"chat_id": args.ChatID,
		"text":    args.Text,
	}
	if args.ParseMode != "" {
		payload["parse_mode"] = args.ParseMode
	}
	if args.DisableNotification {
		payload["disable_notification"] = true
	}

	var out struct {
		OK     bool `json:"ok"`
		Result struct {
			MessageID int64 `json:"message_id"`
		} `json:"result"`
	}
	if err := c.call(ctx, "sendMessage", payload, &out); err != nil {
		return SendMessageResult{}, err
	}
	return SendMessageResult{OK: out.OK, MessageID: out.Result.MessageID}, nil
}

// GetMeResult is the normalized, token-free result of a getMe probe.
type GetMeResult struct {
	OK       bool   `json:"ok"`
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

// GetMe is a zero-argument credential-validity probe (GET/POST getMe) for
// arming checks and acceptance tests: a 200 with ok:true proves the
// configured token is live without sending any message.
func (c *Client) GetMe(ctx context.Context) (GetMeResult, error) {
	if !c.IsConfigured() {
		return GetMeResult{}, ErrNotConfigured
	}
	var out struct {
		OK     bool `json:"ok"`
		Result struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
		} `json:"result"`
	}
	if err := c.call(ctx, "getMe", nil, &out); err != nil {
		return GetMeResult{}, err
	}
	return GetMeResult{OK: out.OK, ID: out.Result.ID, Username: out.Result.Username}, nil
}

// call POSTs to the Bot API. The token lives ONLY inside this function's
// local url variable, which is never logged, never placed in an error
// string, and never echoed back in a Result — every error path below
// constructs its message from the RESPONSE BODY (which Telegram never
// echoes the token into) or a static string, not from the request URL.
func (c *Client) call(ctx context.Context, method string, payload any, out any) error {
	// SECURITY: Telegram's Bot API puts the token in the URL PATH
	// (bot<TOKEN>/method), not a header. This is the one place that string
	// is assembled. It must never be passed to log.Print*, fmt.Errorf,
	// or any Result/APIError field — every return below is built from
	// resp.StatusCode and the parsed response body instead.
	url := c.base + "/bot" + c.token + "/" + method

	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("telegram: encode request: %w", err)
		}
		body = strings.NewReader(string(raw))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		// http.NewRequestWithContext's own error can embed the URL (it wraps
		// url.Parse failures with the input string). Our base+method are
		// fixed and safe, so this path is unreachable in practice, but never
		// return err directly here regardless — return a static message.
		return errors.New("telegram: failed to build request")
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// net/http errors from Do() embed the request URL in their Error()
		// string (e.g. `Post "https://.../bot<TOKEN>/...": dial tcp ...`).
		// Never return it as-is — collapse to a token-free message.
		return fmt.Errorf("telegram: request failed: %s", redactURLError(err))
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode >= 300 {
		return apiErrorFromResponse(resp.StatusCode, data)
	}

	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("telegram: decode response: %w", err)
		}
	}
	return nil
}

func apiErrorFromResponse(status int, data []byte) *APIError {
	var parsed struct {
		Description string `json:"description"`
		Parameters  struct {
			RetryAfter int `json:"retry_after"`
		} `json:"parameters"`
	}
	_ = json.Unmarshal(data, &parsed)

	desc := parsed.Description
	if desc == "" {
		switch status {
		case http.StatusUnauthorized:
			desc = "unauthorized (bad or revoked bot token)"
		case http.StatusNotFound:
			desc = "chat not found"
		case http.StatusTooManyRequests:
			desc = "rate limited"
		default:
			desc = "telegram api error"
		}
	}

	apiErr := &APIError{StatusCode: status, Description: desc}
	if status == http.StatusTooManyRequests {
		apiErr.RetryAfter = parsed.Parameters.RetryAfter
	}
	return apiErr
}

// redactURLError strips anything that looks like a bot token
// (`/bot<digits>:<secret>/`) out of a net/http transport error before it is
// allowed anywhere near a log line or returned error.
func redactURLError(err error) string {
	msg := err.Error()
	const marker = "/bot"
	idx := strings.Index(msg, marker)
	if idx == -1 {
		return msg
	}
	end := strings.Index(msg[idx+len(marker):], "/")
	if end == -1 {
		return msg[:idx] + "/bot***REDACTED***"
	}
	return msg[:idx] + "/bot***REDACTED***" + msg[idx+len(marker)+end:]
}
