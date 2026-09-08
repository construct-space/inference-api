# Nuxt 3 — Apoc patterns

Nuxt 3 is the default full-stack web framework for non-space projects. Vue 3 composition API, file-based routing, server routes in `server/api/`.

## Project skeleton

```
my-app/
├── nuxt.config.ts
├── package.json
├── tsconfig.json
├── app.vue
├── assets/
├── components/
├── composables/
├── pages/
│   ├── index.vue
│   └── about.vue
├── server/
│   └── api/
│       └── hello.ts
└── public/
```

## Bun install + scripts

```bash
bun create nuxt my-app
cd my-app
bun install
bun run dev          # localhost:3000
bun run build
bun run preview
```

## Page

```vue
<script setup lang="ts">
const { data: notes } = await useFetch('/api/notes')
</script>

<template>
  <div class="p-6">
    <h1 class="text-2xl font-bold">Notes</h1>
    <ul>
      <li v-for="n in notes" :key="n.id">{{ n.title }}</li>
    </ul>
  </div>
</template>
```

## Server route

```ts
// server/api/notes.ts
export default defineEventHandler(async (event) => {
  return [{ id: 1, title: 'Hello' }]
})
```

## Patterns

- **Auto-imports** are on by default — don't `import { ref } from 'vue'` in components.
- **`useFetch` SSR-safe** — runs once on server, hydrates on client.
- **`$fetch`** for non-reactive imperative calls (event handlers).
- **`useState('key', () => ...)`** for shared SSR-safe state.
- **`useRuntimeConfig()`** for env-driven config; public keys go under `public:` in `nuxt.config.ts`.

## Tailwind v4

```bash
bun add -d @nuxtjs/tailwindcss
```

`nuxt.config.ts`:

```ts
export default defineNuxtConfig({
  modules: ['@nuxtjs/tailwindcss'],
})
```

## Don't propose

- Vuex / Pinia before `useState` is shown insufficient.
- `axios` — use `useFetch` / `$fetch`.
- Class components.
- Manual `import` of Vue / Nuxt composables that are auto-imported.
