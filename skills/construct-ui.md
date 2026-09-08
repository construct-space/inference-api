# Construct UI — the @construct-space/ui primitives

The shared UI library used by every Construct space and by the desktop app. CSS is injected by JS; there is no global Tailwind bundle. Components by name, not raw divs.

## Install

```bash
bun add @construct-space/ui
```

## Theme

```ts
import { useTheme } from '@construct-space/ui'

const { theme, setTheme, mode, setMode } = useTheme()
// theme: 'construct' | 'mono' | ... ; mode: 'light' | 'dark' | 'system'
```

`useTheme` is the single source of truth. Do not override CSS variables yourself — set theme + mode and let the lib write tokens.

## Component cheat sheet

```vue
<Button variant="primary | secondary | ghost | danger" size="sm | md | lg" :loading="false" @click="...">
  Save
</Button>

<Input v-model="title" placeholder="..." :error="..." />
<Textarea v-model="body" rows="4" />
<Select v-model="status" :options="[{ value: 'todo', label: 'Todo' }, ...]" />
<Switch v-model="enabled" />
<Checkbox v-model="checked" label="..." />

<Card padding="md" radius="md">
  <template #header>Title</template>
  <template #default>Body</template>
  <template #footer>Actions</template>
</Card>

<Badge variant="info | success | warning | danger">Beta</Badge>
<Tag color="brand">tag</Tag>

<Avatar :src="user.avatar" :name="user.name" size="md" />
<Icon name="sticky-note" size="md" />

<Tabs v-model="tab" :tabs="[{ id: 'all', label: 'All' }, ...]" />
<Modal v-model:open="open" title="...">
  <p>...</p>
</Modal>
<Drawer v-model:open="open" position="right">...</Drawer>
<Tooltip content="...">Hover me</Tooltip>
<Spinner size="md" />

<EmptyState icon="inbox" title="No notes yet" description="Add one to get started" />
<Toast variant="success">Saved</Toast>            <!-- via useToast() composable -->
```

## Rules

- Always use Construct UI components in Construct contexts. Don't reach for `<button class="...">`.
- Slots over class overrides. If you need a non-trivial visual change, ask the user; don't hack CSS.
- Icons via the `Icon` component with a lucide-style `name` prop. Don't import lucide directly.
- `useToast()` for one-off notifications; don't roll your own toast container.

## Composing

```vue
<Card>
  <template #header>
    <div class="flex items-center justify-between w-full">
      <h3 class="text-lg font-semibold">Notes</h3>
      <Button variant="ghost" size="sm" @click="add">
        <Icon name="plus" /> New
      </Button>
    </div>
  </template>
  <div class="flex flex-col gap-2">
    <Input v-model="title" placeholder="New note" @keydown.enter="add" />
    <Card v-for="n in notes" :key="n.id" padding="sm">{{ n.title }}</Card>
  </div>
</Card>
```

Tailwind utility classes are fine for layout/spacing inside components. Color/typography should come from theme tokens, not arbitrary Tailwind values.
