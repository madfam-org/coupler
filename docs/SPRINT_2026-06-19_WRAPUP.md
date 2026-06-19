# Sprint wrap-up — Coupler platform bootstrap (2026-06-19)

**Sprint theme:** Stand up MADFAM's fourth platform (Coupler ATP), document Selva separation-of-concerns, ship P1–P3 integration skeleton, deploy to staging/prod, and establish public domains.

**Ecosystem handoff (ops):** [`internal-devops/runbooks/2026-06-19-coupler-platform-sprint-wrapup.md`](https://github.com/madfam-org/internal-devops/blob/main/runbooks/2026-06-19-coupler-platform-sprint-wrapup.md)

---

## Executive summary

| Outcome | Status |
|---------|--------|
| `madfam-org/coupler` repo created (AGPL, public) | Done |
| Phase 0 scaffold → Phase 2 live execute (GitHub + Slack) | Done |
| Janua P1 ConnectedAccount + token delegation API | Done (code shipped; migration on deploy) |
| Selva P3a `CouplerToolBackend` + feature flag | Done |
| Labspace MCP Phase 0 (`madfam-ecosystem`, `enclii-mcp`) | Done (local workspace) |
| Gateway promoted to production GitOps | Done |
| Janua API promoted (P1 migration path) | Done |
| Public domains + landing site | Done (`coupler.madfam.io`, `coupler-api.madfam.io`) |
| Enclii onboard + DNS cutover | Pending (`ENCLII_API_TOKEN`) |
| Selva workers prod promote (Coupler backend) | Pending (30min soak gate) |

---

## Repos touched

| Repo | HEAD (sprint) | What shipped |
|------|---------------|--------------|
| **coupler** | `c2fe47d` | Gateway P2, landing, k8s GitOps, docs, MCP package, SDK |
| **janua** | `26d0959b` | `008_connected_accounts`, `/api/v1/connections/*`, delegation |
| **selva-office** | `eb81a49` | `CouplerToolBackend`, registry discovery, tests, integration doc |
| **internal-devops** | (this sprint) | Ecosystem sprint runbook |
| **enclii** | (this sprint) | `AGENT_TOOL_PLANE.md` domain convention update |

### Local labspace only (not git-versioned)

| Path | Change |
|------|--------|
| `/Users/Aldo/labspace/.cursor/mcp.json` | `madfam-ecosystem`, `enclii-mcp`, `coupler-mcp` |
| `/Users/Aldo/labspace/mcp/ecosystem-server/repos.json` | Coupler inventory + domains |

---

## Architecture delivered

### Platform boundaries (fourth pillar)

```
Janua     → identity + ConnectedAccount vault
Enclii    → deploy, observe, madfam.ops.*
Selva     → LLM routing, builtins, ecosystem adapters
Coupler   → coupler.* delegated SaaS, MCP, triggers
```

**Hard rules:** no refresh tokens in Coupler; no Enclii/Janua server imports; no Composio Cloud.

### Public surface

| URL | Service |
|-----|---------|
| https://coupler.madfam.io | Marketing landing (`apps/landing`) |
| https://coupler-api.madfam.io | REST gateway (`apps/gateway`) |

**DNS policy:** avoid 3-level subdomains (`api.coupler.*`); use `coupler-api.madfam.io`.

### Live tools (connectors)

- `coupler.github.list_repos`, `get_issue`, `create_issue`
- `coupler.slack.post_message`, `list_channels`

Execute path: user JWT → gateway → Janua `POST /connections/{id}/token` → SaaS API.

---

## Documentation index (canonical)

| Document | Repo |
|----------|------|
| [SELVA_TOOLING_AUDIT.md](./SELVA_TOOLING_AUDIT.md) | coupler |
| [SEPARATION_OF_CONCERNS.md](./SEPARATION_OF_CONCERNS.md) | coupler |
| [IMPLEMENTATION_ROADMAP.md](./IMPLEMENTATION_ROADMAP.md) | coupler |
| [PHASE0_CHECKLIST.md](./PHASE0_CHECKLIST.md) | coupler |
| [COUPLER_INTEGRATION.md](https://github.com/madfam-org/selva-office/blob/main/docs/COUPLER_INTEGRATION.md) | selva-office |
| [COUPLER_PROGRAM.md](https://github.com/madfam-org/janua/blob/main/docs/COUPLER_PROGRAM.md) | janua |
| [AGENT_TOOL_PLANE.md](https://github.com/madfam-org/enclii/blob/main/docs/strategy/AGENT_TOOL_PLANE.md) | enclii |

---

## Selva refactor map (deferred P4)

**Stays in Selva:** Karafiel, Dhanam, PhyndCRM, Resend outbound, platform infra, gateway ingress, ~268 builtins.

**Migrate to Coupler:** `slack.py` (bot token), GitHub MCP in `mcp_config.json`, calendar OAuth, social tools.

**Selva internal cleanup:** wire or remove unused `McpToolAdapter`; unify tool resolver; worker `set_coupler_user_jwt()`.

---

## Tests added

| Repo | Tests |
|------|-------|
| coupler | `executor_test.go`, registry tests, CI landing/nginx/script checks |
| janua | `test_connections_router.py` |
| selva-office | `test_coupler_backend.py` |

---

## CI / deploy

### Coupler workflows

- `ci.yml` — gateway, MCP, SDK, landing, scripts
- `staging-deploy.yml` — parallel gateway + landing build, digest patch
- `promote-to-prod.yml` — sync all images staging → prod

### Promotion history (sprint)

| Component | Result |
|-----------|--------|
| Janua API | Promoted (break-glass) |
| Coupler gateway | Promoted to prod kustomization |
| Coupler landing | Staging build pending first green deploy after sprint commit |
| Selva workers | Blocked on 30min soak (re-run promote workflow) |

---

## Open items (next sprint)

1. **Enclii onboard** — `enclii onboard --repo madfam-org/coupler` with prod API token
2. **K8s secret** — `coupler-gateway-secrets` / `janua-service-token` in `coupler` namespace
3. **Janua migration** — verify `008_connected_accounts` applied in prod after API promote
4. **Selva workers promote** — after soak:
   ```bash
   gh workflow run promote-to-prod.yml -R madfam-org/selva-office \
     -f component=workers \
     -f reason="Coupler P3a after soak"
   ```
5. **Selva P3b** — `set_coupler_user_jwt()` in worker task context
6. **Feature flag** — `SELVA_COUPLER_TOOLS_ENABLED=true` in prod workers env
7. **OAuth authorize flow** — dedicated `/connections/{provider}/authorize` (today: OAuthAccount sync bridge)
8. **P4** — deprecate direct SaaS paths in Selva builtins

---

## Quick verification

```bash
# API
curl -s https://coupler-api.madfam.io/health | jq

# Landing
curl -sI https://coupler.madfam.io | head -5

# Dry-run execute (needs user JWT in prod)
curl -s -X POST https://coupler-api.madfam.io/v1/tools/execute \
  -H 'Content-Type: application/json' \
  -d '{"tool":"coupler.github.list_repos","dry_run":true}' | jq
```

---

## Sprint narrative

We verified Coupler did not exist, created `madfam-org/coupler`, and bootstrapped the Agent Tool Plane as a sovereign Composio-class alternative. We audited Selva's ~268 built-in tools and drew a clear line: MADFAM ecosystem workflows stay in Selva; user-delegated SaaS moves to Coupler.

Janua gained the ConnectedAccount vault and service-only token delegation. The gateway gained JWKS verification, Janua delegation client, and live GitHub/Slack executors. Selva gained a feature-flagged `CouplerToolBackend` without embedding connector SDKs.

We established labspace-level MCP for ecosystem context, Enclii CLI, and Coupler dev access. Production GitOps landed for the gateway; domains were corrected to the two-level `coupler-api` pattern; and an engaging public landing went live at `coupler.madfam.io`.

**The superpower is now scaffolded in code and docs.** Full production cutover completes when Enclii onboard, secrets, Selva worker promote, and Janua migration are closed.
