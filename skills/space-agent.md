# In-space agent — config and skills

A Construct space can ship its own agent that runs inside the desktop app's brain when the user opens the space. The agent's persona, skills, and safety hooks live next to the space code.

## Layout

```
my-space/
├── space.manifest.json     ← references "agent" + "skills"
└── agent/
    ├── config.md           ← agent identity + behavior
    ├── skills/
    │   └── default.md      ← skill loaded by default
    │   └── custom.md       ← optional, loaded on demand
    └── hooks/
        └── safety.json     ← optional input/output guards
```

Manifest:

```json
{
  "agent": "agent/config.md",
  "skills": ["agent/skills/default.md"]
}
```

## `agent/config.md`

```markdown
---
name: Notes Assistant
description: Helps the user create, organize, and find notes
temperature: 0.3
---

You are the Notes assistant inside Construct. Your job: help the user
create, search, and organize their notes. You can call any action
exported from `src/actions.ts` (createNote, listNotes, etc.).

Style: short replies, no preamble. When the user is vague, suggest
two concrete options rather than asking open questions.
```

## `agent/skills/default.md`

Just markdown. First non-`#` line is the description shown in the skill menu.

```markdown
Common note workflows.

When the user says "remind me," create a Reminder with the due_at
field set. When they say "find," prefer searching by tag before
searching by title.
```

## Action exposure

Every exported action in `src/actions.ts` is automatically available as a tool to the in-space agent. Name + description + params come from the action's declaration. No extra registration step.

## Safety hooks (`agent/hooks/safety.json`)

```json
{
  "input": {
    "blocklist": ["password", "credit card"]
  },
  "output": {
    "redact": ["email", "phone"]
  }
}
```

Hooks run inside the brain, outside the model. Use sparingly — they're a backstop, not a feature.

## When to ship an in-space agent

Ship one when:

- The space has actions worth invoking by voice/chat.
- The data model is small enough that the user benefits from natural-language queries.

Skip the in-space agent when:

- The space is a passive viewer (no mutations).
- The default Construct agent (Apoc-style) covers the same surface.

## Don't

- Don't duplicate Apoc — keep in-space agents focused on the space's domain.
- Don't bake secrets into `config.md`. Use `useSpaceConfig` for keys.
- Don't write a skill that just repeats action descriptions — that data already flows from `src/actions.ts`.
