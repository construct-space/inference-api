# Construct Graph — data layer

The Construct Graph is a hosted PostgreSQL-backed data service. Models are defined in TypeScript in each space and pushed to the Graph service. Verified against `packages/construct-cli/src/commands/graph/generate.ts` and `packages/graph/src/composable.ts` (CLI v1.7.5).

## Define a model

```ts
// src/models/Note.ts
import { defineModel, field, access } from '@construct-space/graph'

export const Note = defineModel('note', {
  title:    field.string().required(),
  content:  field.string(),
  pinned:   field.boolean(),
  position: field.json(),
  page_id:  field.string().index(),
  owner_id: field.string().index(),
}, {
  access: {
    read:   access.authenticated(),
    create: access.authenticated(),
    update: access.authenticated(),
    delete: access.owner(),
  },
  scopes: ['org'],
})
```

Re-export from `src/models/index.ts`:

```ts
export { Note } from './Note'
export { Tag } from './Tag'
```

## Field types

| Token | Emits | Notes |
|---|---|---|
| `string` | `field.string()` | text |
| `int` | `field.int()` | integer |
| `number` | `field.number()` | float |
| `boolean` (alias `bool`) | `field.boolean()` | true/false |
| `date` | `field.date()` | NOT `datetime` |
| `json` | `field.json()` | arbitrary JSON-serializable |
| `enum` | `field.enum(['a','b','c'])` | string enum |

## Modifiers (chainable)

`.required()`, `.unique()`, `.index()`, `.email()`, `.url()`, `.default(value)`

## Relations

```ts
import { relation } from '@construct-space/graph'

post:    relation.belongsTo(Post),    // single parent, no FK arg, no arrow
comments: relation.hasMany(Comment),  // collection
```

## Access rules

Levels: `public`, `authenticated`, `owner`, `member`, `admin`, `none`.
Operations: `read`, `create`, `update`, `delete`.

```ts
access: {
  read:   access.member(),
  create: access.owner(),
  update: access.owner(),
  delete: access.admin(),
}
```

## useGraph client API

```ts
import { useGraph } from '@construct-space/graph'
import { Note } from './models/Note'

const notes = useGraph(Note)

await notes.find({ where: { pinned: true }, order: { created_at: 'desc' } })
await notes.findOne(id)
await notes.create({ title: 'Hi', content: 'world' })
await notes.update(id, { title: 'Hello' })
await notes.remove(id)              // NEVER .delete(id)
await notes.count()
notes.subscribe(({ event, record }) => { /* live updates */ })
await notes.query('...', { ... })   // raw GraphQL
await notes.mutate('...', { ... })
```

## CLI scaffolding

```bash
construct graph init                                # initialize graph in a space
construct graph generate Note title:string:required content:string pinned:boolean
construct graph g Comment body:string post:belongsTo:Post
construct graph g Task status:enum:todo,doing,done priority:int
construct graph push                                # register models with the service
construct graph migrate                             # diff + apply schema changes
construct graph migrate --apply                     # apply destructive changes
construct graph fork <new-space-id>                 # change graph space id
```

The `generate` `--access` flag: `--access read:member,create:member,update:owner,delete:admin`

## Common patterns

**Author-owned records**: add `owner_id: field.string().index()`, set `access.delete: access.owner()`.

**Soft pagination**: `find({ limit, offset, order })`.

**Live list**: `subscribe` on the page, push events into a Vue ref. Don't poll.

**Composables that share the client**:

```ts
// src/composables/useNotes.ts
import { useGraph } from '@construct-space/graph'
import { Note } from '../models/Note'

export function useNotes() {
  const client = useGraph(Note)
  const notes = ref<Note[]>([])
  const load = async () => { notes.value = await client.find() }
  return { notes, load, create: client.create, remove: client.remove }
}
```
