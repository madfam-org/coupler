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
- Audience: `coupler-api`
- Register client: `janua.client.yaml`
