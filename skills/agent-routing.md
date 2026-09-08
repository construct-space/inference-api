# Agent routing — the dispatcher's tool

How Trinity hands off to a specialist. The orchestrator exposes a single tool:

```json
{
  "type": "function",
  "function": {
    "name": "route_to_agent",
    "description": "Hand the conversation to a specialist agent. Used by the dispatcher (Trinity) — do not call from a specialist.",
    "parameters": {
      "type": "object",
      "properties": {
        "agent": {
          "type": "string",
          "enum": ["apoc", "mouse", "oracle"],
          "description": "The specialist that should handle this task."
        },
        "rationale": {
          "type": "string",
          "description": "One sentence explaining the routing decision."
        },
        "hint": {
          "type": "string",
          "description": "Optional: extra context for the specialist (e.g. 'call mouse mid-task for the design IR')."
        }
      },
      "required": ["agent", "rationale"]
    }
  }
}
```

## Routing matrix (canonical)

| Task shape | Agent |
|---|---|
| New code, edits, builds, scaffolds, refactors, fixes | apoc |
| Pure design, layout, visual hierarchy, design tokens, IR | mouse |
| Operational: deploys, telemetry, audit, fleet inventory | oracle |
| Design + code combined | apoc with `hint: "call mouse first for IR"` |
| Single short ambiguous question | apoc |

## When NOT to route

- Greetings / chitchat with no task — let Trinity respond inline ("I'm Trinity, the router. What do you want to build?").
- Out-of-scope requests (cooking recipes, world news) — refuse politely, don't route.

## Multi-turn

Once routed, Trinity is out of the loop. The orchestrator may bring Trinity back if the user switches topic; until then the specialist owns the conversation.
