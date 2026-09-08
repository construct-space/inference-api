# actions.ts — Construct space actions

Actions are the unit of "stuff a space does." They power toolbar buttons, agent tool calls, and external triggers. Canonical shape verified against ConstructData golden + real spaces.

## Canonical shape

```ts
// src/actions.ts
import { useGraph } from '@construct-space/graph'
import { Note } from './models/Note'

export const actions = {
  createNote: {
    description: 'Create a new note.',
    params: {
      title:   { type: 'string',  required: true },
      content: { type: 'string' },
    },
    run: async ({ title, content }: { title: string; content?: string }) => {
      const note = await useGraph(Note).create({ title, content })
      return { ok: true, note }
    },
  },

  listNotes: {
    description: 'List all notes.',
    params: {},
    run: async () => {
      const notes = await useGraph(Note).find()
      return { notes }
    },
  },

  updateNote: {
    description: 'Update fields on a note by id.',
    params: {
      id:    { type: 'string', required: true },
      patch: { type: 'object', required: true, description: 'Partial Note fields' },
    },
    run: async ({ id, patch }: { id: string; patch: Record<string, unknown> }) => {
      const note = await useGraph(Note).update(id, patch)
      return { note }
    },
  },

  deleteNote: {
    description: 'Delete a note by id.',
    params: {
      id: { type: 'string', required: true },
    },
    run: async ({ id }: { id: string }) => {
      await useGraph(Note).remove(id)
      return { ok: true }
    },
  },
}
```

## Param types

`string`, `int`, `number`, `boolean`, `object`, `array`, `enum` (with `values`).

```ts
params: {
  status: { type: 'enum', values: ['todo', 'doing', 'done'], required: true },
  tags:   { type: 'array', items: { type: 'string' } },
}
```

## Rules

- Action names are **camelCase** verbs: `createNote`, `inviteMember`, `archiveProject`.
- `description` must be one short sentence — it's what the agent sees in the tool menu.
- `run` is always `async`. Return a plain JSON-serializable object.
- Inside `run`, use `useGraph(Model)` from `@construct-space/graph` for data access.
- For external HTTP, use `fetch` (the SDK exposes `fetch` already). See `external-apis` skill.
- Never throw raw errors back to the caller — wrap in `{ ok: false, error: '...' }` for expected failures, throw for true bugs.

## Pages don't import actions

Actions are agent-facing wrappers. Pages do **not** import or call actions — pages talk to the graph + composables directly. **There is no `useActions()` composable.** The SDK does not export one — inventing the import will fail with `useActions is not a function` at runtime.

❌ **Wrong** (this is fabricated; `useActions` does not exist):

```ts
import { useActions } from '@construct-space/sdk'   // ← will fail
const actions = useActions()
await actions.createNote({ title: 'X' })
```

✅ **Right** — page calls the same underlying graph/composables that the action wraps:

```ts
// src/pages/index.vue
<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useGraph } from '@construct-space/graph'
import { Note } from '../models/Note'

const client = useGraph(Note)
const notes = ref<any[]>([])
const title = ref('')

async function load()   { notes.value = await client.find() }
async function create() {
  if (!title.value.trim()) return
  await client.create({ title: title.value })
  title.value = ''
  await load()
}

onMounted(load)
</script>
```

The same `client.create(...)` call is what `actions.createNote.run` does — pages and actions are parallel surfaces over the same data. The action exists so the **agent** can run the operation via `space_run_action`. UI calls the data layer directly.

## Wiring to toolbar

Add to `space.manifest.json`:

```json
"toolbar": [
  { "id": "notes-new", "icon": "i-lucide-plus", "label": "New Note", "action": "createNote" }
]
```

The `"action"` value matches the exported action name.

## Wiring to permissions (optional)

For spaces that need fine-grained perms, manifest:

```json
"permissions": {
  "actions": {
    "createNote": "notes:write",
    "deleteNote": "notes:admin"
  },
  "catalog": [
    { "id": "notes:write", "label": "Create & edit", "group": "Notes" },
    { "id": "notes:admin", "label": "Admin",         "group": "Notes" }
  ]
}
```

## Agent tool exposure

Every exported action is automatically available to the in-space agent as a tool, identified by name with the `description` + `params` schema. No extra registration.
