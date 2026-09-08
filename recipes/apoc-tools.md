# Apoc tool surface

What tools a host orchestrator should expose so Apoc can one-shot a Construct space end-to-end. The inference-api passes OpenAI-shaped `tools` through to the upstream model; the orchestrator implements them and feeds results back as `role:"tool"` messages.

Split by who owns each tool:

- **Host (construct-app or your orchestrator)**: `write_file`, `read_file`, `list_files`, `run_shell`, `load_skill`. construct-app's brain already implements all of these as part of its 22 builtin tools.
- **inference-api (gateway, server-side)**: `web_search`, `web_fetch`. Fetch the OpenAI specs from `GET /api/tools/builtin` and merge into `tools`. When the model emits a tool_call named `web_search` or `web_fetch`, POST it to `/api/web/search` / `/api/web/fetch` instead of running locally.

## Minimum tools for one-shot

```json
{
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "write_file",
        "description": "Write a file. Creates parent dirs. Overwrites if exists.",
        "parameters": {
          "type": "object",
          "properties": {
            "path": { "type": "string", "description": "Relative to the space root." },
            "content": { "type": "string" }
          },
          "required": ["path", "content"]
        }
      }
    },
    {
      "type": "function",
      "function": {
        "name": "read_file",
        "description": "Read a file from the current space.",
        "parameters": {
          "type": "object",
          "properties": { "path": { "type": "string" } },
          "required": ["path"]
        }
      }
    },
    {
      "type": "function",
      "function": {
        "name": "list_files",
        "description": "List files under a directory (relative to space root).",
        "parameters": {
          "type": "object",
          "properties": { "path": { "type": "string", "default": "." } }
        }
      }
    },
    {
      "type": "function",
      "function": {
        "name": "run_shell",
        "description": "Run a shell command in the current space directory. Stream stdout+stderr back.",
        "parameters": {
          "type": "object",
          "properties": {
            "command": { "type": "string", "description": "Whole command string, e.g. 'bun install'" },
            "timeout_ms": { "type": "integer", "default": 120000 }
          },
          "required": ["command"]
        }
      }
    },
    {
      "type": "function",
      "function": {
        "name": "load_skill",
        "description": "Load an additional skill into context by name (e.g. 'construct-graph').",
        "parameters": {
          "type": "object",
          "properties": { "name": { "type": "string" } },
          "required": ["name"]
        }
      }
    }
  ]
}
```

## Optional but high-value tools

- `graph_push` — wraps `construct graph push`, parses output.
- `graph_migrate(apply: bool)` — wraps `construct graph migrate [--apply]`.
- `space_install` — wraps `construct install`.
- `space_validate` — wraps `construct validate`.
- `space_publish(bump?: patch|minor|major, private?: bool)` — wraps `construct publish`.
- `fetch(url, method?, headers?, body?)` — external HTTP, sandboxed.
- `ask_user(question)` — interactive clarification when ambiguous.

## Safety

- `run_shell` MUST be scoped to the space dir (refuse `cd`, `..`, absolute paths) or run in a container.
- `write_file` MUST refuse paths outside the space root.
- All destructive operations (`rm`, `--apply`, `publish`) should require explicit user confirmation in the orchestrator UI before execution, not just trust the model.

## One-shot sequence (typical)

1. Apoc → `load_skill("space-anatomy")`
2. Apoc → `load_skill("construct-graph")` + `load_skill("actions")` + `load_skill("one-shot-space")`
3. Apoc → `run_shell("construct scaffold notes")`
4. Apoc → `write_file("space.manifest.json", ...)`
5. Apoc → `write_file("src/models/Note.ts", ...)`
6. Apoc → `write_file("src/models/index.ts", ...)`
7. Apoc → `write_file("src/actions.ts", ...)`
8. Apoc → `write_file("src/pages/index.vue", ...)`
9. Apoc → `write_file("widgets/summary/4x1.vue", ...)`
10. Apoc → `run_shell("bun install")`
11. Apoc → `run_shell("construct graph push")`
12. Apoc → `run_shell("construct build && construct install")`
13. Apoc → text summary to user: "Notes space installed. Open Construct."

## Wire it through inference-api

The inference-api forwards `tools` + `tool_choice` to the upstream model as-is. To use:

```bash
curl -s http://localhost:4300/api/chat \
  -H 'Content-Type: application/json' \
  -d '{
    "skills": ["apoc", "space-anatomy", "construct-graph", "actions", "one-shot-space"],
    "tools": [ /* the tools above */ ],
    "tool_choice": "auto",
    "messages": [
      { "role": "user", "content": "build me a Construct space called notes with title + content + pinned, an action to create a note, an index page, and a 4x1 summary widget showing total count." }
    ]
  }'
```

The response will contain `choices[0].message.tool_calls` when Apoc wants to act. Execute the call, append a `{ "role": "tool", "tool_call_id": "...", "content": "..." }` message, and call again. Loop until `finish_reason: "stop"`.
