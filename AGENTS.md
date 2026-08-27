# Coupler — Agent Operating Guide

## What this repo is

MADFAM's **Agent Tool Plane (ATP)**. Fourth platform alongside Enclii, Janua, and Selva.

## Hard rules

1. **Never** store OAuth refresh tokens in Coupler Postgres — Janua ConnectedAccount only.
2. **Never** import Enclii `switchyard-api` or Janua API code — SDK HTTP clients only.
3. **Never** call `kubectl` or mutate cluster state — operator actions proxy to Enclii `providers.*`.
4. **Never** depend on Composio Cloud APIs.
5. User-zone tools use prefix `coupler.*`; operator-zone uses `madfam.ops.*`.

## Entrypoints

| Path | Purpose |
|------|---------|
| `apps/gateway/` | REST API at coupler-api.madfam.io |
| `apps/landing/` | Public site at coupler.madfam.io |
| `packages/mcp-server/` | Cursor/Claude stdio MCP |
| `packages/sdk-typescript/` | `@madfam/coupler` consumer SDK |
| `connectors/` | Tier-1 connector manifests + runtime |
| `docs/openapi/coupler-v1.yaml` | Public API contract |
| `enclii.yaml` | Enclii onboard manifest |

## Phase gates

- **P0:** CI green, gateway `/health`, manifests load, MCP lists tools
- **P1 (janua):** Token delegation API live
- **P2:** Live execute for GitHub + Slack
- **P3:** Selva CouplerToolBackend (feature-flagged)

## Auth

- Verify Janua RS256 JWTs via `https://auth.madfam.io/.well-known/jwks.json`
- **Audience: a configured SET**, not a single value. The gateway verifier
  (`apps/gateway/internal/auth/jwks.go`) accepts any `aud` in the set from
  `COUPLER_JANUA_AUDIENCE` (comma-separated), defaulting to
  `{coupler-api, coupler-gateway}`. A single-audience deployment still isolates
  correctly — one value stays valid (per the merged `acceptedAudiences` /
  `audienceOK` tests).
- **One audience per caller** (owner ruling, 2026-08-27): every caller of the
  Coupler API carries its own `aud`, which makes `aud` an auditable caller
  identity at the gateway door. `coupler-api` = Angelia/Moirai execution calls
  (angelia-coupler client); `coupler-gateway` = the gateway's own internal ops.
  The verifier carries the audience the *token* presented (not the configured
  set) into `Claims.Aud` so downstream handlers and audit records can tell the
  callers apart. Adding a caller means registering a NEW audience and adding it
  to the set — never re-using an existing caller's.
- Register client: `janua.client.yaml` — now registers **coupler-gateway** with
  `audience: coupler-gateway` and **2-segment** scopes
  (`coupler:tools_execute`, `coupler:connections_read`; Janua's validator is
  `namespace:action`, exactly one colon).
