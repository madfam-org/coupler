# Selva agent tooling audit (2026-06-19)

Canonical cross-repo reference for Coupler separation-of-concerns. Selva consumer plan: `selva-office/docs/COUPLER_INTEGRATION.md`.

## Executive summary

Selva ships **~268 registered built-in tools** (`packages/tools`) plus **6 MADFAM ecosystem adapters** (`packages/inference/madfam_inference/adapters/`). There is **no unified LLM tool-calling loop** across worker graphs; most LangGraph workflows import tools directly. MCP infrastructure exists but is **not wired** into `ToolRegistry` at runtime.

**Coupler does not replace Selva builtins.** It absorbs **user-delegated third-party SaaS** execution that today uses shared bot tokens or direct HTTP in Selva.

---

## Code map

| Path | Role |
|------|------|
| `packages/tools/src/selva_tools/registry.py` | Singleton registry, `discover_builtins()` |
| `packages/tools/src/selva_tools/builtins/` | 84 modules, ~268 tools |
| `packages/tools/src/selva_tools/mcp/client.py` | `McpToolAdapter`, `discover_mcp_tools()` — **unused in prod** |
| `packages/workflows/mcp_config.json` | Tavily, GitHub MCP, filesystem — Analyst stub only |
| `packages/inference/madfam_inference/adapters/` | Karafiel, Dhanam, PhyndCRM, Tezca, Crawler |
| `packages/calendar/` | Google/Microsoft calendar OAuth |
| `apps/workers/selva_workers/graphs/` | Per-graph direct tool imports |
| `apps/nexus-api/nexus_api/routers/gateway.py` | 18-channel **ingress** webhooks (not agent execute) |

---

## Built-in categories (stay in Selva)

| Category | Examples | Owner |
|----------|----------|-------|
| MADFAM ecosystem | Karafiel CFDI, Dhanam checkout, PhyndCRM leads | Selva adapters → platform APIs |
| Platform infra | K8s, Cloudflare, Enclii, ArgoCD, `github_admin` | Selva (long-term: Enclii `madfam.ops.*`) |
| Campaign / CRM graphs | `crm.py`, `sales.py`, `operations.py` | Selva orchestration |
| Meta / HITL | `tool_catalog`, `hitl_introspection`, `factory_manifest` | Selva |
| Outbound gateway | Resend email with voice_mode, tenant identity | Selva messaging plane |
| Observability | Prometheus, Loki, Sentry, Grafana | Selva platform ops |

---

## Migrate to Coupler (user-delegated SaaS)

These tools embed **direct third-party HTTP** or **shared bot tokens** and should route through Coupler when `SELVA_COUPLER_TOOLS_ENABLED=true`:

| Selva tool / module | Today | Coupler replacement |
|---------------------|-------|---------------------|
| `builtins/slack.py` (`slack_message`) | `SLACK_BOT_TOKEN` | `coupler.slack.post_message` + Janua connection |
| `builtins/discord.py` | Bot token / webhook | `coupler.discord.*` (future connector) |
| `builtins/telegram.py` | `TELEGRAM_BOT_TOKEN` | `coupler.telegram.*` (future) |
| `builtins/calendar_tools.py` + `packages/calendar/` | Per-user OAuth env | `coupler.google.calendar.*` (future) |
| `mcp_config.json` → `github` MCP | `GITHUB_TOKEN` subprocess | `coupler.github.list_repos` etc. |
| `builtins/reddit_tools.py` | PRAW / persona keys | Coupler connector (future) |
| `builtins/mastodon_tools.py` | App passwords | Coupler connector (future) |
| `builtins/bluesky_tools.py` | AT Protocol passwords | Coupler connector (future) |

**Phase P4 refactor:** deprecate direct paths behind feature flag; keep fallback until Coupler staging gate passes.

---

## Explicitly NOT Coupler

| Surface | Why |
|---------|-----|
| `gateway.py` ingress (Slack events, WhatsApp, Matrix…) | Inbound channel routing, not delegated execute |
| Resend `email_tools` | Selva outbound product surface |
| `github_admin.py` | MADFAM org operations (platform) |
| Karafiel / Dhanam / PhyndCRM adapters | MADFAM ecosystem contracts |
| Worker `BashTool` / `GitTool` | Local workspace execution |
| Tavily `web_search` | Search primitive (may stay builtin or use Coupler search connector later) |

---

## MCP gap (Selva internal refactor)

1. `discover_mcp_tools()` never called — dead path alongside `web_search` duplicate.
2. ACP Analyst builds MCP bootstrap snippet but does not execute it.
3. **Recommendation:** either wire MCP at worker startup **or** remove stub; prefer **Coupler MCP** for dev/Cursor and **Coupler REST** for production agents.

---

## Tool resolution gaps (Selva internal refactor)

| Mechanism | Status |
|-----------|--------|
| `ToolRegistry.get_specs()` + LLM loop | Not wired in workers |
| YAML `tools:` on agent nodes | Schema exists; handler ignores |
| `PluginManager` tools | Not merged into registry |
| Audience guard (`PLATFORM` vs `TENANT`) | **Working** — keep |

**P3b target:** `resolve_tools_for_task(skill_ids, audience, coupler_enabled)` unifies builtins + Coupler proxies + skills permissions.

---

## Environment variables (Coupler-related)

| Variable | Repo | Purpose |
|----------|------|---------|
| `SELVA_COUPLER_TOOLS_ENABLED` | Selva | Feature flag (default `false`) |
| `COUPLER_BASE_URL` | Selva | Gateway URL |
| `COUPLER_GATEWAY_URL` | Coupler MCP | MCP → gateway |
| `COUPLER_JANUA_*` | Coupler gateway | Issuer, audience, API URL, service token |
| `SLACK_BOT_TOKEN`, `GITHUB_TOKEN`, … | Selva | **Legacy** — remove after P4 parity |

See [SEPARATION_OF_CONCERNS.md](./SEPARATION_OF_CONCERNS.md) for the full boundary matrix.
