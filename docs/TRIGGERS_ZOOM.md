# Zoom trigger — meeting.ended → nauta time drafts

The first inbound trigger in the ATP charter. Coupler receives Zoom's
`meeting.ended` webhook, verifies it, and relays it as a **time-entry draft**
to nauta's machine door (`/api/integrations/coupler/time-draft`). A platform
operator confirms or discards each draft in the nauta cockpit; nothing coupler
relays ever touches the hours ledger by itself (nauta D15).

## Flow

```
Zoom cloud ──meeting.ended (HMAC-signed)──▶ coupler-gateway /v1/triggers/zoom
                                               │  verify x-zm-signature (±5 min)
                                               │  compute wall-clock minutes
                                               ▼
                nauta /api/integrations/coupler/time-draft (service token)
                                               │  dedupe on (source, externalRef)
                                               │  attribute by participant email
                                               ▼
                     TimeEntryDraft ──operator confirm──▶ TimeEntry (ledger)
```

Retry semantics: nauta 5xx → coupler answers Zoom 5xx (Zoom retries, event
survives an outage). Nauta 4xx → coupler answers 200 `refused_downstream`
(a permanent refusal must not become a retry storm) and logs loudly.

## Operator setup (one-time)

1. **Zoom Marketplace app** — [marketplace.zoom.us](https://marketplace.zoom.us)
   → Develop → Build App → **Server-to-Server OAuth** (any name, e.g.
   `madfam-coupler-triggers`). No scopes are needed for webhooks alone.
2. **Event subscription** — in the app's *Feature* → *Event Subscriptions*:
   - Add subscription, method **Webhook**.
   - Endpoint URL: `https://coupler-api.madfam.io/v1/triggers/zoom`
   - Events: **End Meeting** (`meeting.ended`).
   - Copy the app's **Secret Token** (Feature page).
3. **Vault** — write the two properties the ExternalSecret reads
   (`secret/coupler`): `zoom_webhook_secret_token` (from step 2) and
   `nauta_service_token` (mint: `openssl rand -hex 32`).
4. **Nauta side** — set the SAME `nauta_service_token` value as
   `COUPLER_SERVICE_TOKEN` in nauta's secret store. Both routes fail closed
   until their secret exists.
5. Back in Zoom, press **Validate** on the endpoint — the gateway answers the
   `endpoint.url_validation` challenge once its secret is deployed.

## Failure doctrine

- No `ZOOM_WEBHOOK_SECRET_TOKEN` → every request answers 503 (fail closed).
- Bad or stale signature (>5 min skew) → 401, nothing dispatched.
- Verified event with no dispatch target configured → 500, so Zoom retries
  until the target exists; a verified meeting must not vanish silently.
- Duplicate deliveries collapse in nauta on `(source, externalRef)` —
  the Zoom meeting UUID.

## What this deliberately does not do

- No Zoom API calls (no OAuth scopes, no janua ConnectedAccount yet). The
  webhook payload alone carries what a draft needs; API enrichment
  (participant lists, recordings) is a later phase and will ride janua
  delegation like every other connector.
- No billability judgment. Coupler relays; the operator decides.
