# coupler — Ecosystem Context

> **Agent Tool Plane (ATP) — delegated SaaS tool execution, MCP, sandbox, triggers.**

**Pillar:** Intelligence / Agent tools  
**Type:** platform  
**Status:** bootstrap → staging (P2 live execute shipped 2026-06-19)

## What this repo is

Coupler is MADFAM's fourth platform: Composio-class capabilities (tool catalog, execute, MCP, triggers) without embedding SaaS connector logic in Enclii (PaaS) or Janua (identity). Selva agents and ecosystem apps consume Coupler via HTTP or MCP.

### Deployed services (target)

| Service | Public domain | Container port |
|---------|---------------|----------------|
| `coupler-gateway` | `api.coupler.madfam.io` (TBD at onboard) | 8787 |

**Kubernetes namespace:** `coupler` (pending Enclii onboard)

### Upstream dependencies

- **Janua** — JWT verification, ConnectedAccount vault, OAuth, token delegation
- **Enclii** — deploy, domains, operator ops proxy (`madfam.ops.*`)
- **Selva** — optional tool search embeddings (Phase 4)

### Downstream consumers

- **selva-office** — `CouplerToolBackend` for delegated SaaS tools
- **Ecosystem apps** — via `@madfam/coupler` SDK or MCP
- **Cursor / Claude** — via `packages/mcp-server`

### Key environment variables

- `COUPLER_JANUA_ISSUER` — default `https://auth.madfam.io`
- `COUPLER_JANUA_AUDIENCE` — `coupler-api`
- `COUPLER_ENCLII_API_URL` — operator proxy target (Phase 4)
- `DATABASE_URL` — Coupler Postgres (Phase 2)

## MADFAM platform map (Coupler row)

| Platform | Repo | Role |
|----------|------|------|
| **Coupler** | `madfam-org/coupler` | ATP — tool registry, execute, MCP, sandbox, triggers |
| **Janua** | `madfam-org/janua` | Identity + ConnectedAccount vault |
| **Enclii** | `madfam-org/enclii` | PaaS control plane |
| **Selva** | `madfam-org/selva-office` | LLM + agent orchestration |

## Document provenance

Bootstrap 2026-06-19. Canonical architecture: `enclii/docs/strategy/AGENT_TOOL_PLANE.md`.
