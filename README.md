# Coupler — MADFAM Agent Tool Plane (ATP)

**Coupler** is MADFAM's sovereign **Agent Tool Plane**: delegated SaaS tool execution, MCP, sandbox, and triggers — without Composio or embedding connector logic in Enclii or Janua.

| Platform | Owns |
|----------|------|
| **Janua** | Identity, ConnectedAccount vault, OAuth broker, token delegation |
| **Enclii** | Deploy, observe, operator `providers.*` / `ops.*` |
| **Selva** | LLM routing and agent orchestration |
| **Coupler** | Tool registry, search, execute, MCP, sandbox, triggers |

## Status

**Phase 0 — Bootstrap** (this repo)

- Gateway skeleton with `/health`, tool catalog from connector manifests, dry-run execute
- MCP server stub (`packages/mcp-server`)
- TypeScript SDK stub (`packages/sdk-typescript`)
- Tier-1 connector manifests: GitHub, Slack
- Enclii + Janua client bootstrap files

**Blocker for production execute:** Janua Phase 1 ConnectedAccount / token delegation ([janua COUPLER_PROGRAM](https://github.com/madfam-org/janua/blob/main/docs/COUPLER_PROGRAM.md)).

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
