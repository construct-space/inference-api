# Next 14 (app router) — Apoc patterns

Next is the React-side equivalent of Nuxt for Apoc. Default to **App Router** (RSC) for new projects. Pages Router only on explicit request.

## Skeleton

```
my-app/
├── package.json
├── tsconfig.json
├── next.config.ts
├── app/
│   ├── layout.tsx
│   ├── page.tsx
│   ├── api/
│   │   └── hello/route.ts
│   └── notes/
│       ├── page.tsx
│       └── [id]/page.tsx
├── components/
└── public/
```

## Install

```bash
bun create next-app my-app
cd my-app
bun install
bun run dev          # localhost:3000
bun run build && bun run start
```

## Server component (default)

```tsx
// app/notes/page.tsx
async function getNotes() {
  const res = await fetch('https://api.example.com/notes', { cache: 'no-store' })
  return res.json()
}

export default async function NotesPage() {
  const notes = await getNotes()
  return (
    <main className="p-6">
      <h1 className="text-2xl font-bold">Notes</h1>
      <ul>{notes.map((n: any) => <li key={n.id}>{n.title}</li>)}</ul>
    </main>
  )
}
```

## Client component (interactive)

```tsx
'use client'
import { useState } from 'react'

export function Counter() {
  const [n, setN] = useState(0)
  return <button onClick={() => setN(n + 1)}>{n}</button>
}
```

`'use client'` at the top — required for hooks, event handlers, browser APIs.

## API route (App Router)

```ts
// app/api/notes/route.ts
import { NextResponse } from 'next/server'

export async function GET() {
  return NextResponse.json([{ id: 1, title: 'Hello' }])
}

export async function POST(req: Request) {
  const body = await req.json()
  return NextResponse.json({ created: body }, { status: 201 })
}
```

## Server actions

```tsx
// app/notes/actions.ts
'use server'
export async function createNote(formData: FormData) {
  const title = formData.get('title') as string
  // db.insert(...)
}
```

```tsx
import { createNote } from './actions'

export default function Page() {
  return (
    <form action={createNote}>
      <input name="title" />
      <button>Add</button>
    </form>
  )
}
```

## Patterns

- **Data fetching**: in server components with plain `fetch` (Next dedupes + caches).
- **Cache hints**: `{ cache: 'no-store' }` for fresh; `{ next: { revalidate: 60 } }` for ISR-like.
- **State**: `useState` first; reach for Zustand only when prop-drilling becomes painful. No Redux.
- **Forms**: server actions + `useFormStatus` / `useFormState` from `react-dom`.
- **Auth**: NextAuth (now `Auth.js`) is the default; Clerk if the user names it.

## Don't propose

- Pages Router (`pages/api/`, `getServerSideProps`) for new projects.
- `axios` — `fetch` is fine.
- Class components.
- Redux unless the user explicitly mentions it.
