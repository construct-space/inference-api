# One-shot: prompt → working Construct space

End-to-end recipe for taking a one-line request and producing a runnable space. Apoc executes this as a sequence of tool calls; if no tools are available, output every file inline.

## The recipe (plan → scaffold → emit → run)

**Plan** (steps 1-7) — decide what the space contains:

1. **Pick an `id`** — kebab-case, unique. From the prompt or ask the user. The space dir on disk is **`space-<id>`** (e.g. id `bookmarks` → dir `space-bookmarks`).
2. **Pick scope** — `["app"]` for personal/local; `["org"]` for team data; `["app", "org"]` for both.
3. **Identify entities** — what's the noun? `Note`, `Task`, `Bookmark`. One model per entity.
4. **Identify actions** — what verbs operate on entities? `createNote`, `archiveNote`, etc.
5. **Identify pages** — what views does the user see? Index always exists; add `settings` if config needed.
6. **Identify external APIs** — anything outside Construct? Add a composable per API.
7. **Identify widgets** — what summary belongs on the home strip? Default to one `summary` widget.

**Scaffold** — always start with the CLI scaffold; do not invent the directory from scratch:

```bash
construct new space-<id>       # creates the dir with package.json, tsconfig, vite.config, etc.
cd space-<id>
```

**Emit files in canonical order** (step 8) — overwrite or add to the scaffolded dir.

**Run** — the install/build/install/dev shell block from "The shell commands" section below.

## File emission order (always)

1. `space.manifest.json`
2. `package.json`
3. `src/models/<Entity>.ts` (one file per model)
4. `src/models/index.ts` (re-exports)
5. `src/actions.ts`
6. `src/pages/index.vue`
7. `src/pages/<other>.vue` (if any)
8. `src/composables/<useX>.ts` (if external APIs or shared logic)
9. `widgets/summary/4x1.vue` (and `2x1.vue` if listed)
10. `agent/config.md`, `agent/skills/default.md` (if `agent` set in manifest)
11. `README.md`

`src/entry.ts` is NEVER hand-written — `construct build` generates it.

## Minimal `package.json`

```json
{
  "name": "@construct-spaces/notes",
  "version": "0.1.0",
  "private": true,
  "type": "module",
  "scripts": {
    "build": "construct build",
    "dev": "construct dev",
    "publish:space": "construct publish"
  },
  "dependencies": {
    "@construct-space/graph": "^0.3.0",
    "@construct-space/sdk": "^0.5.0",
    "@construct-space/ui": "^0.3.5",
    "vue": "^3.5.0"
  },
  "devDependencies": {
    "@construct-space/cli": "^1.7.0",
    "typescript": "^5.6.0",
    "vite": "^5.4.0",
    "vue-tsc": "^2.1.0"
  }
}
```

## Minimal index.vue

```vue
<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useGraph } from '@construct-space/graph'
import { Note } from '../models/Note'

const client = useGraph(Note)
const notes = ref<any[]>([])
const title = ref('')

async function load() { notes.value = await client.find() }
async function add() {
  if (!title.value.trim()) return
  await client.create({ title: title.value })
  title.value = ''
  await load()
}

onMounted(load)
</script>

<template>
  <div class="p-4 space-y-4">
    <div class="flex gap-2">
      <input v-model="title" class="border rounded px-2 py-1 flex-1" placeholder="New note" />
      <button @click="add" class="px-3 py-1 rounded bg-black text-white">Add</button>
    </div>
    <ul class="space-y-1">
      <li v-for="n in notes" :key="n.id" class="border rounded p-2">{{ n.title }}</li>
    </ul>
  </div>
</template>
```

## Minimal widget — `widgets/summary/4x1.vue`

```vue
<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useGraph } from '@construct-space/graph'
import { Note } from '../../src/models/Note'

const count = ref(0)
onMounted(async () => { count.value = await useGraph(Note).count() })
</script>

<template>
  <div class="p-3 h-full flex items-center justify-between">
    <div class="text-sm text-gray-600">Notes</div>
    <div class="text-2xl font-semibold">{{ count }}</div>
  </div>
</template>
```

## The shell commands

Spaces live at `~/Spaces/space-<id>` on disk. The full one-shot sequence:

```bash
construct new space-<id>       # scaffold
cd space-<id>
bun install
construct graph init
construct graph push
construct build
construct install
construct dev                  # iterate
```

When asked to "one-shot a notes space," output the **scaffold command first**, then every file (manifest, package.json, model, actions, page, widget, README) in canonical emission order, then the install/build/install shell block.

## Verification

After emitting, the user should be able to copy-paste the shell block and end up with a working space:

```bash
bun install && construct build && construct install
```

…and see the space in the Construct desktop sidebar. If `construct validate` fails, the manifest is wrong — go back to step 1.
