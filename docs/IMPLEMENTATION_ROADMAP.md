# Coupler implementation roadmap

**Last updated:** 2026-06-19  
**Repo:** https://github.com/madfam-org/coupler

## Phase status

| Phase | Scope | Status |
|-------|-------|--------|
| **P0** | Repo, gateway stubs, manifests, MCP, CI | ✅ Done |
| **P1** | Janua ConnectedAccount + token delegation | ✅ Done (2026-06-19 sprint) |
| **P2** | Live GitHub + Slack execute | ✅ Done (2026-06-19 sprint) |
| **P3** | Selva `CouplerToolBackend` + feature flag | ✅ Done (P3a + P3b worker JWT) |
| **P4** | Selva SaaS refactor + `madfam.ops.*` Enclii proxy | Planned |
| **P5** | Staging synthetics, Enclii onboard, prod domain | 🚧 Partial (gateway prod; onboard pending) |

## P0 deliverables (complete)

- [x] `madfam-org/coupler` public AGPL repo
- [x] Gateway: `/health`, `/v1/tools`, `/v1/tools/search`, `/v1/tools/execute` (dry-run)
- [x] Connector manifests: GitHub (3 tools), Slack (2 tools)
- [x] `packages/mcp-server` stdio MCP
- [x] `packages/sdk-typescript` stub
- [x] `enclii.yaml`, `janua.client.yaml`, `k8s/production/gateway.yaml`
- [x] Docs: architecture, Selva audit, separation matrix

## P1 — Janua keyring (blocker)

| ID | Deliverable | Location |
|----|-------------|----------|
| J1-1 | Migration `connected_accounts`, `provider_types` | `janua/apps/api/alembic/versions/008_*` |
| J1-2 | `ConnectedAccount` model | `janua/apps/api/app/models/connected_account.py` |
| J1-3 | `GET/DELETE /api/v1/connections` | `janua/.../routers/v1/connections.py` |
| J1-4 | OAuth authorize + callback OR OAuthAccount sync | same router |
| J1-5 | `POST /api/v1/connections/{id}/token` | same router |
| J1-6 | Audit on delegation | `ActivityLog` |

**P1 gate:** Coupler executes `coupler.github.list_repos` with delegated token.

## P2 — Coupler live execute

| ID | Deliverable | Location |
|----|-------------|----------|
| C2-1 | Janua delegation HTTP client | `coupler/apps/gateway/internal/janua/` |
| C2-2 | JWT verification (JWKS) | `coupler/apps/gateway/internal/auth/` |
| C2-3 | Executor dispatch | `coupler/apps/gateway/internal/executor/` |
| C2-4 | GitHub connector runtime | `executor/github.go` |
| C2-5 | Slack connector runtime | `executor/slack.go` |

## P3 — Selva consumer

| ID | Deliverable | Location |
|----|-------------|----------|
| S3-1 | `CouplerToolBackend` | `selva-office/packages/tools/.../backends/coupler.py` |
| S3-2 | `CouplerProxyTool` + registry discovery | `registry.py` |
| S3-3 | Feature flag `SELVA_COUPLER_TOOLS_ENABLED` | `.env.example` |
| S3-4 | Unit tests with mocked gateway | `packages/tools/tests/test_coupler_backend.py` |
| S3-5 | Labspace `coupler-mcp` in `.cursor/mcp.json` | `labspace/.cursor/mcp.json` |

## P4 — Selva refactor (post-P3 gate)

- Deprecate `slack_message` direct path when Coupler flag on
- Remove `github` from `mcp_config.json` in favor of Coupler MCP
- Document parity checklist per connector

## P5 — Production

- `enclii onboard` → `coupler-api.madfam.io`
- Register `coupler-gateway` OAuth client in Janua prod
- Staging synthetic: Selva worker → Coupler → Slack

## Cross-repo doc index

| Document | Repo |
|----------|------|
| `AGENT_TOOL_PLANE.md` | enclii |
| `COUPLER_REMEDIATION_PLAN.md` | enclii |
| `COUPLER_PROGRAM.md` | janua |
| `ADR-002_UNIVERSAL_KEYRING.md` | janua |
| `COUPLER_INTEGRATION.md` | selva-office |
| `SELVA_TOOLING_AUDIT.md` | coupler (this repo) |
| `SEPARATION_OF_CONCERNS.md` | coupler (this repo) |
| `SPRINT_2026-06-19_WRAPUP.md` | coupler (this repo) |
