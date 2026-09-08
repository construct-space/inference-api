# Space widgets — sidebar tiles

Widgets are the small read-only tiles shown on the Construct home screen / sidebar. Each space declares its widgets in the manifest, ships one Vue file per supported size.

## Manifest declaration

```json
"widgets": [
  {
    "id": "summary",
    "name": "Notes Summary",
    "description": "Total count + recent pinned",
    "icon": "i-lucide-sticky-note",
    "defaultSize": "4x1",
    "sizes": {
      "2x1": "widgets/summary/2x1.vue",
      "4x1": "widgets/summary/4x1.vue",
      "4x2": "widgets/summary/4x2.vue"
    }
  }
]
```

- `id` — unique within the space.
- `sizes` — keys are `WxH`; values are Vue files relative to space root.
- `defaultSize` — what the user gets when they first add the widget.
- Common grid sizes: `2x1`, `4x1`, `4x2`, `2x2`.

## Sizing intuition

| Size | What fits |
|---|---|
| `2x1` | One number + one label. KPI tile. |
| `4x1` | KPI + one row of context (last item, status). |
| `2x2` | Small list (3 rows) or single chart. |
| `4x2` | List of 5-8 rows, or KPI + chart, or 2 KPIs side-by-side. |

## Minimal `4x1.vue`

```vue
<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useGraph } from '@construct-space/graph'
import { Note } from '../../src/models/Note'

const count = ref(0)
const latest = ref('')

onMounted(async () => {
  const client = useGraph(Note)
  count.value = await client.count()
  const [first] = await client.find({ limit: 1, order: { created_at: 'desc' } })
  latest.value = first?.title ?? '—'
})
</script>

<template>
  <div class="h-full flex items-center justify-between p-3">
    <div>
      <div class="text-xs uppercase text-gray-500">Notes</div>
      <div class="text-2xl font-semibold">{{ count }}</div>
    </div>
    <div class="text-sm text-gray-600 truncate max-w-[60%] text-right">
      {{ latest }}
    </div>
  </div>
</template>
```

## Rules

- **Read-only summary state.** Click opens the full space. Don't put forms or destructive buttons in a widget.
- **Self-contained.** A widget must work even if the user has never opened the parent space. Use `useGraph(Model)` directly; don't rely on Pinia state from the host.
- **No top-level scroll.** Fit the size. If the data is too long, truncate.
- **One render path.** Don't load assets that vary by theme — use `useTheme()` and CSS variables so theme switches are instant.
- **Lazy queries.** Use `count` over `find` where possible; widgets render dozens at a time on the home screen.

## Multi-size files: share the composable

```ts
// src/composables/useSummary.ts
import { ref, onMounted } from 'vue'
import { useGraph } from '@construct-space/graph'
import { Note } from '../models/Note'

export function useSummary() {
  const count = ref(0)
  const latest = ref<any>(null)
  onMounted(async () => {
    const c = useGraph(Note)
    count.value = await c.count()
    const [first] = await c.find({ limit: 1, order: { created_at: 'desc' } })
    latest.value = first ?? null
  })
  return { count, latest }
}
```

Then each size file imports and renders.
