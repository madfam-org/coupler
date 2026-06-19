# Coupler Phase 0–2 execution checklist

## Repository

- [x] Create `madfam-org/coupler` (public, AGPL-3.0)
- [x] Monorepo scaffold per AGENT_TOOL_PLANE §3.2
- [x] README, AGENTS.md, ECOSYSTEM.md
- [x] CI: Go test + lint gateway, Python syntax check MCP
- [x] Docs: SELVA_TOOLING_AUDIT, SEPARATION_OF_CONCERNS, IMPLEMENTATION_ROADMAP

## Gateway (`apps/gateway`)

- [x] `GET /health`
- [x] `GET /v1/tools` — catalog from connector manifests
- [x] `GET /v1/tools/search?q=`
- [x] `POST /v1/tools/execute` — dry_run + live execute
- [x] JWT verification against Janua JWKS (`internal/auth/jwks.go`)
- [x] Janua delegation client (`internal/janua/`)
- [x] Executor dispatch GitHub + Slack (`internal/executor/`)
- [ ] Enclii onboard + production deploy

## Connectors

- [x] `connectors/github/manifest.yaml`
- [x] `connectors/slack/manifest.yaml`
- [x] Live GitHub API execute (P2)
- [x] Live Slack API execute (P2)

## MCP (`packages/mcp-server`)

- [x] `list_tools` — mirrors gateway catalog
- [x] `execute_tool` — dry_run via gateway or local fallback
- [x] Labspace `coupler-mcp` in `.cursor/mcp.json`

## Janua integration

- [x] `ConnectedAccount` model + migration `008_connected_accounts`
- [x] `GET/DELETE /api/v1/connections`
- [x] `POST /api/v1/connections/{id}/token` (service token + X-Acting-User-Id)
- [x] OAuthAccount → ConnectedAccount sync bridge
- [ ] Dedicated OAuth authorize/callback for new connections (use `/connections/sync/{provider}` for now)
- [x] `janua.client.yaml` bootstrap

## Selva integration

- [x] `CouplerToolBackend` in selva-office
- [x] Registry discovery behind `SELVA_COUPLER_TOOLS_ENABLED`
- [x] `docs/COUPLER_INTEGRATION.md` updated with audit cross-links
- [ ] Worker `set_coupler_user_jwt()` wiring (P3b)

## Domains (TBD at onboard)

- Candidate: `api.coupler.madfam.io`, `coupler.madfam.io`
