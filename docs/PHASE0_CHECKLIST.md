# Coupler Phase 0 execution checklist

Derived from `enclii/docs/strategy/COUPLER_EXECUTION_CHECKLIST.md`.

## Repository

- [x] Create `madfam-org/coupler` (public, AGPL-3.0)
- [x] Monorepo scaffold per AGENT_TOOL_PLANE §3.2
- [x] README, AGENTS.md, ECOSYSTEM.md
- [x] CI: Go test + lint gateway, Python syntax check MCP

## Gateway (`apps/gateway`)

- [x] `GET /health`
- [x] `GET /v1/tools` — catalog from connector manifests
- [x] `GET /v1/tools/search?q=`
- [x] `POST /v1/tools/execute` — dry_run only until Janua P1
- [ ] JWT verification against Janua JWKS (stub accepts missing token in dev)
- [ ] Enclii onboard + production deploy

## Connectors

- [x] `connectors/github/manifest.yaml` — stub tools
- [x] `connectors/slack/manifest.yaml` — stub tools
- [ ] Live GitHub API execute (P2)
- [ ] Live Slack API execute (P2)

## MCP (`packages/mcp-server`)

- [x] `list_tools` — mirrors gateway catalog
- [x] `execute_tool` — dry_run via gateway or local fallback
- [ ] Staging gateway URL config via `COUPLER_GATEWAY_URL`

## Janua integration (blocker)

- [ ] `ConnectedAccount` model + delegation API ([janua COUPLER_PROGRAM](https://github.com/madfam-org/janua/blob/main/docs/COUPLER_PROGRAM.md))
- [x] `janua.client.yaml` bootstrap

## Selva integration

- [ ] `CouplerToolBackend` in selva-office (P3)
- [ ] Documented in `selva-office/docs/COUPLER_INTEGRATION.md`

## Domains (TBD at onboard)

- Candidate: `api.coupler.madfam.io`, `coupler.madfam.io`
