# Construct infrastructure — high-level map

For Oracle and operationally-aware Apoc tasks. Do not assume any of this when speaking to users.

## Services (api/*)

Each is a Go service with `Dockerfile`, deploys via CapRover to construct-network. Owns its own GitHub repo `construct-space/<name>-api`.

| Service | Purpose |
|---|---|
| `accounts` | Users, sessions, OAuth, passkeys, 2FA |
| `developer` | Publishers, space lifecycle (pending/approved/rejected) |
| `source` | Orgs, projects, teams, invites, feed-items |
| `oracle` | Staff admin gateway — proxies admin endpoints from other services |
| `provider` | Provider catalog (LLM upstreams), Construct-managed model registry |
| `marketplace` | Public catalog, editorial overrides, collections |
| `graph` | Hosted PostgreSQL-backed data service for spaces |
| `delivery` | Transactional email — DKIM/SPF/DMARC, per-tenant API keys |
| `domains` | Registrar + DNS + redirects |
| `telemetry` | Usage / model-usage / perf / errors / top-N |
| `storage` | Asset storage |
| `integration` | Webhook ingress + outbound integrations |
| `status` | Public status page |
| `website` | Marketing site backend |
| `billing` | Polar metered billing for AI credits |
| `inference` | LLM gateway (this service) — Together AI proxy + skills + agents |

## Frontends (web/*)

| Site | Hosts |
|---|---|
| `my.lisaos.dev` | Unified user portal |
| `oracle.lisaos.dev` | Staff admin |
| `delivery.lisaos.dev` | Tenant delivery UI |

Plus marketing + blog (Ghost) + status page.

## Identity scopes

- `cat_*` — identity token (Authorization: Bearer)
- `csk_live_*` — publisher ownership key (X-API-Key)
- A request may carry both. `AuthContext { identity, publisher? }`.

## Cross-service auth

Every internal service shares `INTERNAL_SHARED_SECRET` and writes `X-Auth-*` headers when proxying user context.

## Known constraints

- delivery-1 is a separate Docker swarm; `srv-captain--*` names do not resolve to it from CONSTRUCT-MAIN. Inter-VPS calls go through the construct-network Hetzner private network.
- graph DB lives on a dedicated VPS (graph-1) for Postgres.
- Spaces live in `~/Spaces/space-*` on dev machines; each is its own git repo.
