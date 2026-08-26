# Telegram connector — `coupler.telegram.*`

MADFAM's primary human-notification channel is Telegram (Slack does not work
for the owner). This connector is the **interim general-purpose "send a
Telegram message" tool** for the whole ecosystem — Selva agents, services,
and operators all reach it through coupler's normal `/v1/tools/execute`
path — until the **angelia Matrix bridge** (pre-product) ships and takes over
as the strategic channel.

## Credential model — why this is NOT a janua-delegated connector

Every other coupler connector (GitHub, Slack) authenticates by **janua OAuth
token delegation**: a user connects their account once via janua's
ConnectedAccount flow, and coupler resolves + delegates a short-lived access
token per call (see [SEPARATION_OF_CONCERNS.md](SEPARATION_OF_CONCERNS.md)).

**A Telegram bot token does not fit that model.** It is a single static
secret minted once via [@BotFather](https://t.me/BotFather) — there is no
per-user OAuth authorize screen, no refresh token, and no "connect your
Telegram account" flow to broker. The token identifies the *bot*, not an
end user; identity and scoping for a message happen at the `chat_id` level
(the target chat/channel), not at a per-connected-account level.

Modeling it as a fake janua ConnectedAccount would misrepresent what's
actually happening. Instead, following the precedent already established by
the [Zoom trigger](TRIGGERS_ZOOM.md) for the same "not per-user OAuth"
situation, the token is held **server-side as a gateway secret**:

```
vault (secret/coupler: telegram_bot_token)
  → ExternalSecret coupler-gateway-secrets (key: telegram-bot-token)
    → k8s Secret
      → env COUPLER_TELEGRAM_BOT_TOKEN (optional: true)
        → apps/gateway/internal/telegram.Client
```

`executor.Execute()` branches on `tool.Connector == "telegram"` **before**
its shared janua `ResolveConnectionID` / `DelegateToken` calls, so a
telegram tool call never touches janua at all. Tools therefore take only
`chat_id` + `text` (+ optional formatting flags) — there is no
`connection_id` argument for this connector, and the dry-run plan reports
`"auth": "server_static_token"` instead of `"janua_token_delegation"`.

This is a deliberate, narrower trust model than the OAuth connectors: **the
token is shared by every caller** who can reach coupler's authenticated
`/v1/tools/execute` (still gated by a janua user JWT — Telegram statelessness
does not weaken coupler's own inbound auth). It is appropriate for a
single-bot, ecosystem-wide notification tool; it would NOT be appropriate if
Telegram later needed per-user "send as me" semantics — that would need a
genuinely different design (likely the angelia bridge itself).

## Tools

### `coupler.telegram.send_message`

Sends a text message to a chat via `POST /bot<token>/sendMessage`.

| Argument | Type | Required | Notes |
|---|---|---|---|
| `chat_id` | string | yes | Telegram chat ID or `@channelusername` |
| `text` | string | yes | Max **4096 characters** — enforced client-side before any HTTP call |
| `parse_mode` | string | no | `MarkdownV2` or `HTML` only (validated against Telegram's own enum) |
| `disable_notification` | boolean | no | Silent delivery |

Returns `{"ok": true, "message_id": <int>}` on success.

### `coupler.telegram.get_me`

Zero-argument credential-validity probe (`GET/POST /bot<token>/getMe`).
Returns `{"ok": true, "id": <bot id>, "username": <bot username>}`. Use this
to arm a deployment or in an acceptance test — it proves the configured
token is live without sending any message.

## Error mapping

Telegram Bot API errors are surfaced honestly, never swallowed or retried:

| Telegram response | Coupler behavior |
|---|---|
| `400` chat not found / bad request | Returned as-is with Telegram's `description` |
| `401` unauthorized | Returned as-is — bad or revoked bot token |
| `429` too many requests | Returned as-is **with `retry_after` (seconds) surfaced** — coupler does **not** auto-retry; the caller (agent/service/operator) decides |
| No `COUPLER_TELEGRAM_BOT_TOKEN` configured | Fails closed immediately, no request attempted |
| `text` over 4096 chars | Rejected client-side before any HTTP call |
| Invalid `parse_mode` | Rejected client-side before any HTTP call |

## Security: the bot token never appears in logs, errors, or results

Telegram puts the token in the URL **path** (`/bot<TOKEN>/method`), not a
header — the one place that matters for redaction. `apps/gateway/internal/telegram/client.go`:

- Builds the token-bearing URL **only** inside its unexported `call()`
  method, in a local variable never passed to `log.Print*`, `fmt.Errorf`, or
  any returned struct field.
- Every error path returns a message built from the **response body**
  (Telegram never echoes the token back) or a static string — never from the
  request URL or a raw `error` returned by `http.Client.Do`, since Go's own
  transport errors normally embed the full request URL (a
  `redactURLError` helper strips it defensively as a second layer).
- `APIError` (the type returned for all non-2xx responses) only ever carries
  `StatusCode` + `Description` (+ `RetryAfter` for 429s) — structurally
  incapable of holding a token.
- Client-side validation (length, `chat_id`/`text` presence, `parse_mode`
  enum) runs **before** the token-bearing URL is even constructed, so a
  rejected call never touches the network at all.

Tests in `client_test.go` and `executor_test.go` assert this directly,
including a genuine `net/http` transport-level failure (connection refused)
which normally embeds the request URL — confirmed redacted.

## Operator setup (one-time)

> Follows the same **vault-first, merge-second** rule as the Zoom trigger:
> External Secrets Operator fails the WHOLE `coupler-gateway-secrets` object
> if any ONE referenced vault property is missing. Write the vault property
> in step 2 **before** merging/deploying this connector's k8s changes.

1. **Create the bot** — message [@BotFather](https://t.me/BotFather) on
   Telegram, `/newbot`, follow the prompts. Copy the token it gives you
   (looks like `123456789:AAFakeExampleTokenFormatOnly`).
2. **Vault** — write the property the ExternalSecret reads
   (`secret/coupler`): `telegram_bot_token` (the token from step 1).
3. **Message the bot first (private chats only)** — Telegram bots cannot
   initiate a conversation with a user; the target user/chat must send the
   bot a message (or add it to a group/channel) at least once before
   `send_message` can deliver to that `chat_id`. Use
   `coupler.telegram.get_me` to confirm the token is live, then send a test
   message to the bot from the intended recipient to learn/confirm the
   `chat_id` (e.g. via `https://api.telegram.org/bot<token>/getUpdates`).
4. Deploy — the pod's `COUPLER_TELEGRAM_BOT_TOKEN` env is `optional: true`
   at the k8s level, so it starts either way, but every `coupler.telegram.*`
   call fails closed with "not configured" until step 2 is done.

## What this deliberately does not do

- **No per-user OAuth, no janua ConnectedAccount.** See the credential-model
  section above — the token is one server-held secret shared by every
  caller, not a per-connected-account grant. This does not violate coupler's
  "no OAuth refresh tokens in Coupler Postgres" rule (`AGENTS.md`) because
  there is no OAuth refresh token here at all — the bot token is a static
  API key managed like the Zoom webhook secret.
- **No automatic retry on `429`.** `retry_after` is surfaced to the caller;
  coupler does not implement backoff/retry logic for this connector.
- **No message editing, deletion, media, or inline-keyboard support.** Only
  `sendMessage` (text) and `getMe` exist today — the minimum viable
  "notify a human" tool.
- **Not the long-term channel.** This connector is explicitly interim.
  Once the **angelia Matrix bridge** ships, it becomes MADFAM's strategic
  notification path; this Telegram connector remains coupler's
  general-purpose fallback/utility tool rather than being torn out, since
  Selva agents and services may still want to reach Telegram directly.
