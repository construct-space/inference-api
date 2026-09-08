# inference-api

Construct's LLM inference gateway. Routes requests to Together AI (and later: Fireworks, self-hosted vLLM on Hetzner) with skill-based system prompt injection.

Sits at `llm.lisaos.dev` in production; speaks OpenAI-ish chat-completions shape with a thin Construct-shaped wrapper on top.

## Local dev

```bash
cp .env.example .env
# paste TOGETHER_API_KEY into .env
go run .
```

Server listens on `:4300`.

## Endpoints

### `POST /api/chat`

Non-streaming chat completion. Body:

```json
{
  "model": "Qwen/Qwen3-Coder-480B-A35B-Instruct-FP8",
  "skills": ["apoc"],
  "messages": [
    { "role": "user", "content": "build a Nuxt page with a sign-in form" }
  ],
  "max_tokens": 2048,
  "temperature": 0.2
}
```

- `model` — optional. Defaults to `DEFAULT_MODEL` env (Qwen3-Coder-480B).
- `skills` — optional. List of skill names; each is loaded from `skills/<name>.md` and prepended as a system message in order.
- All other fields pass through to Together.

### `POST /api/chat/stream`

Same body. Returns SSE stream from Together passed straight through.

### `GET /api/models`

Curated list of upstream models the gateway knows about, with pricing hints.

### `GET /api/skills` / `GET /api/skills/{name}`

List skills (name + description) / fetch a single skill's full body.

### `GET /api/agents` / `GET /api/agents/{name}`

List agents / fetch a single agent definition (frontmatter + identity body). An agent declares its default model, pre-loaded skills, recommended tools, and a persona. Calling `/api/chat` with `{ "agent": "apoc", "messages": [...] }` expands that for you.

### `GET /api/health`

Liveness.

## Agents

An agent is a markdown file with YAML frontmatter. The frontmatter declares the default model + pre-loaded skills + recommended tools + temperature; the body is the agent's identity prompt.

```markdown
---
name: apoc
description: Construct code-authoring operator
model: Qwen/Qwen3-Coder-480B-A35B-Instruct-FP8
skills: [apoc, space-anatomy, construct-graph, construct-cli, actions, one-shot-space]
tools: [web_search, web_fetch]
temperature: 0.2
---

You are Apoc...
```

Current roster:

| Agent | Role | Default model |
|---|---|---|
| `apoc` | Code-authoring operator (spaces, web apps, small backends) | Qwen3-Coder-480B-A35B-FP8 |
| `mouse` | Designer — emits structured design IR | Kimi-K2.6 (multimodal) |
| `trinity` | Router — picks the specialist and hands off | Qwen3.6-Plus |
| `oracle` | Internal operator — fleet status, telemetry, audit | Qwen3-Coder-480B-A35B-FP8 |

To invoke:

```bash
curl -s http://localhost:4300/api/chat \
  -H 'Content-Type: application/json' \
  -d '{
    "agent": "apoc",
    "messages": [{"role":"user","content":"build me a notes space"}]
  }' | jq -r '.choices[0].message.content'
```

Caller-supplied `model`, `temperature`, `skills`, `tools` override the agent defaults.

## Skills

Skills are just markdown files in `skills/`. The first non-`#` line is the description shown in `/api/skills`; the whole file is injected as a system message when requested.

Add a new skill = drop a `.md` file. No restart needed (registry caches lazily; bounce the process if you want a clean reload).

Current Apoc skill set:

| Skill | Purpose |
|---|---|
| `apoc` | Apoc identity + house style + anti-patterns |
| `space-anatomy` | Construct space file layout + manifest shape |
| `construct-graph` | Graph SDK: defineModel, field types, useGraph, relations |
| `construct-cli` | CLI commands (bun + construct) |
| `actions` | `src/actions.ts` canonical shape |
| `external-apis` | Calling external HTTP APIs from a space |
| `one-shot-space` | End-to-end recipe: prompt → working space |
| `nuxt` | Nuxt 3 patterns |
| `go-stdlib` | Go HTTP service patterns |

See `recipes/apoc-tools.md` for the tool surface a host orchestrator should expose so Apoc can execute (write files, run shell, push graph, install space).

## Tool calls

`POST /api/chat` accepts OpenAI-shaped `tools` + `tool_choice` and forwards to the upstream. Tool calls come back in `choices[0].message.tool_calls`. The orchestrator executes each call and feeds a `role:"tool"` message with `tool_call_id` back on the next request — standard OpenAI tool-loop.

### Disk + shell tools (host-provided)

Apoc needs `write_file`, `read_file`, `list_files`, `run_shell`, etc. These are NOT in inference-api — they live in the host runtime:

- Inside construct-app: the operator already exposes 22 builtin tools (read/write/edit/bash/glob/grep/git/lsp/task/memory/web_fetch/web_search/ask_user/coordinate). Apoc plugs straight into them.
- Standalone host: the orchestrator implements them per `recipes/apoc-tools.md`.

inference-api never touches disk on the user's machine.

### Built-in network tools (gateway-provided)

inference-api hosts the network tools server-side so any caller benefits:

- `POST /api/web/search` — DuckDuckGo HTML-lite, no API key. Body `{"query":"...", "limit":10}`. Returns `{ results: [{title, url, snippet}] }`. Mirrors the implementation in construct-app's brain.
- `POST /api/web/fetch` — HTTP GET, HTML stripped to text, 200KB cap, http(s) only. Body `{"url":"...", "raw":false}`.
- `GET /api/tools/builtin` — returns the OpenAI tool specs for `web_search` + `web_fetch`. Merge into your `tools` array; route those tool_calls back to `/api/web/*`.

Override the search backend with `WEB_SEARCH_URL` if you want to point at a different DDG-compatible endpoint or proxy.

## Models — current routing

The router is currently dumb: caller specifies a model, or falls back to `DEFAULT_MODEL`. Smart routing (pick model by task signature) lands once we have telemetry from real usage.

| Model | When |
|---|---|
| Qwen3-Coder-480B-A35B-FP8 | Default. Code-specialized, symmetric $2/$2 pricing. |
| DeepSeek V4 Pro | Hard reasoning, 512K context. Cached input $0.20/M. |
| Qwen3.6-Plus | Whole-repo / 1M context tasks. Cheap input $0.50/M. |
| Kimi K2.6 | Vision input (designs, screenshots). |
| gpt-oss-120B | Cheap overflow / autocomplete. |

## Capture (for finetune + analysis)

Every request/response is logged as a JSONL line to `CAPTURE_DIR` (default `./logs/io/YYYY-MM-DD.jsonl`). A background goroutine uploads finalized day-files to Cloudflare R2 hourly.

Record shape:

```json
{
  "id": "uuid",
  "ts": "2026-05-17T19:45:00Z",
  "duration_ms": 1234,
  "agent": "apoc",
  "model": "Qwen/Qwen3-Coder-480B-A35B-Instruct-FP8",
  "skills_loaded": ["apoc", "space-anatomy", "construct-graph", ...],
  "temperature": 0.2,
  "max_tokens": 4096,
  "stream": false,
  "messages": [{"role":"system","content":"..."}, ...],
  "tools_offered": [...],
  "response": {
    "content": "...",
    "tool_calls": [...],
    "finish_reason": "stop"
  },
  "usage": { "prompt_tokens": 1234, "completion_tokens": 567, "total_tokens": 1801 },
  "ok": true,
  "caller": { "user_id": "...", "org_id": "..." },
  "upstream_response_id": "..."
}
```

Caller identity is read from `X-User-ID` / `X-Org-ID` / `X-Source` headers — the gateway in front of inference-api should fill these in.

### Privacy / opt-out

- `CAPTURE_ENABLED=false` kills the subsystem entirely.
- Per-request opt-out: include `"capture": false` in the chat body. Default is to capture everything (we're pre-product; volume matters).
- Local-first: a network blip never drops a record. R2 is the durable mirror; local JSONL is the source of truth.
- Marker files (`<date>.jsonl.uploaded`) track upload state so restarts don't double-upload.

### R2 setup (Cloudflare)

In the R2 dashboard:
1. Create a bucket (e.g. `construct-inference-logs`).
2. Manage R2 API tokens → create one with **Object Read & Write** on this bucket.
3. The endpoint is `https://<account-id>.r2.cloudflarestorage.com` — drop `https://` for the env var.

```bash
R2_ENDPOINT=abc123def.r2.cloudflarestorage.com
R2_BUCKET=construct-inference-logs
R2_ACCESS_KEY=<token-access-key>
R2_SECRET_KEY=<token-secret>
R2_REGION=auto
R2_PREFIX=inference/
```

### Building the finetune dataset

Day-files in R2 follow `inference/<date>.jsonl`. A simple converter at `ConstructData/scripts/import_inference.py` (TODO) can:
1. Pull day-files from R2 (zero egress on Cloudflare).
2. Filter by agent (`apoc`, `mouse`, ...).
3. Drop short / error rows.
4. Optionally pair accepted vs rejected outputs for DPO.
5. Emit SFT-shaped `messages` rows into `ConstructData/<agent>/<date>.jsonl`.

## Quick test (curl)

```bash
curl -s http://localhost:4300/api/chat \
  -H 'Content-Type: application/json' \
  -d '{
    "skills": ["apoc"],
    "messages": [{"role":"user","content":"hello, who are you?"}]
  }' | jq
```
