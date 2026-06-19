# Coupler — MADFAM Agent Tool Plane (ATP)

**Coupler** is MADFAM's sovereign **Agent Tool Plane**: delegated SaaS tool execution, MCP, sandbox, and triggers — without Composio or embedding connector logic in Enclii or Janua.

| Platform | Owns |
|----------|------|
| **Janua** | Identity, ConnectedAccount vault, OAuth broker, token delegation |
| **Enclii** | Deploy, observe, operator `providers.*` / `ops.*` |
| **Selva** | LLM routing and agent orchestration |
| **Coupler** | Tool registry, search, execute, MCP, sandbox, triggers |

## Status

**Phase 2 — Live execute** (GitHub + Slack via Janua delegation)

- Gateway with JWKS auth, Janua delegation client, connector executor
- MCP server + TypeScript SDK
- Janua `ConnectedAccount` API (P1)
- Selva `CouplerToolBackend` (P3a, feature-flagged)

**Next:** Enclii onboard, worker JWT wiring (P3b), Selva P4 SaaS refactor.

See [docs/IMPLEMENTATION_ROADMAP.md](docs/IMPLEMENTATION_ROADMAP.md) and [docs/SELVA_TOOLING_AUDIT.md](docs/SELVA_TOOLING_AUDIT.md).

## Architecture

See [docs/architecture/OVERVIEW.md](docs/architecture/OVERVIEW.md) and the canonical plan in [enclii AGENT_TOOL_PLANE](https://github.com/madfam-org/enclii/blob/main/docs/strategy/AGENT_TOOL_PLANE.md).

## Quick start (local)

```bash
# Gateway
cd apps/gateway
go run ./cmd/gateway

curl -s http://localhost:8787/health | jq
curl -s http://localhost:8787/v1/tools | jq
curl -s -X POST http://localhost:8787/v1/tools/execute \
  -H 'Content-Type: application/json' \
  -d '{"tool":"coupler.github.list_repos","dry_run":true,"arguments":{}}' | jq

# MCP server (stdio — for Cursor)
python3 packages/mcp-server/server.py
```

## Trust zones

| Prefix | Zone | Auth |
|--------|------|------|
| `coupler.{app}.{action}` | User delegated SaaS | Janua user JWT + connection |
| `madfam.ops.{domain}.{action}` | Operator infra | Admin JWT → Enclii proxy (Phase 4) |
| `madfam.app.{repo}.{action}` | Ecosystem-registered | Service JWT |

## License

AGPL-3.0-only — see [LICENSE](LICENSE).
