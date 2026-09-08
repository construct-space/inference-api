# Design IR — schema reference

The full schema for Mouse's output and Apoc's input. JSON, versioned, render-target-agnostic.

## Top level

```json
{
  "version": "0.1",
  "meta": { "title": "Notes", "description": "Sticky notes UI" },
  "root": { "type": "frame", "layout": {...}, "children": [...] }
}
```

## Node types

### `frame`
A sized region. Has its own layout. Use for pages, sections, cards.

```json
{
  "type": "frame",
  "id": "page-root",
  "layout": { "direction": "column", "gap": 16, "padding": 24 },
  "size": { "width": "fill", "height": "fill" },
  "background": { "fill": "{color.surface.0}" },
  "radius": "{radius.md}",
  "children": [...]
}
```

### `stack`
Auto-layout container, no inherent visual.

```json
{ "type": "stack", "layout": { "direction": "row", "gap": 8, "align": "center" }, "children": [...] }
```

### `text`
```json
{ "type": "text", "value": "Hello world", "token": "heading.lg" }
{ "type": "text", "value": "...", "token": "body.md", "color": "{color.text.muted}", "max_lines": 2 }
```

### `image`
```json
{ "type": "image", "src": "{asset.avatar_placeholder}", "alt": "User avatar", "fit": "cover" }
```

### `vector`
```json
{ "type": "vector", "svg": "<path d='M...'/>", "size": { "width": 24, "height": 24 } }
```

For icons, prefer `{ "type": "component", "name": "Icon", "props": { "name": "sticky-note" } }`.

### `component`
```json
{ "type": "component", "name": "Button", "props": { "variant": "primary", "label": "Save" } }
```

### `list`
Data-bound repeat. `of` references a model name; renderer wires the query.

```json
{
  "type": "list",
  "of": "Note",
  "where": { "pinned": true },
  "order": { "created_at": "desc" },
  "item": {
    "type": "component",
    "name": "Card",
    "props": { "title": "{{title}}", "body": "{{content}}" }
  }
}
```

`{{field}}` templating inside `item` references model fields.

## Size

`"size": { "width": <spec>, "height": <spec> }`

`<spec>` values: number (px), `"fill"`, `"hug"`, `{ "min": n, "max": m }`, `{ "token": "size.card" }`.

## Background / border / radius

```json
"background": { "fill": "{color.surface.1}" },
"border":     { "color": "{color.border.subtle}", "width": 1 },
"radius":     "{radius.md}",
"shadow":     "{shadow.sm}"
```

## Interaction (optional)

Mouse can hint at interactivity; Apoc wires it.

```json
{
  "type": "component",
  "name": "Button",
  "props": { "label": "New note" },
  "on": { "click": { "action": "createNote" } }
}
```

`action` matches an exported name in `src/actions.ts`.

## Validation

A renderer must reject any IR with:
- missing `version`
- missing `root`
- a node `type` not in the allowed list
- raw color hex outside an explicit `"raw": true` field

When in doubt, fail loudly rather than guess.
