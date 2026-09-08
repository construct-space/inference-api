# Tailwind v4 — what's different from v3

Tailwind v4 is the default in Construct's stack. The CSS engine, config format, and a handful of class names changed from v3 — don't write v3 patterns.

## Install (Nuxt / Next / vanilla Vite)

```bash
bun add -d tailwindcss @tailwindcss/vite
```

`vite.config.ts`:
```ts
import tailwindcss from '@tailwindcss/vite'
export default { plugins: [tailwindcss()] }
```

In `src/main.css`:
```css
@import "tailwindcss";
```

No `tailwind.config.js` needed for the default install. Use it only when you need custom tokens.

## Config — CSS-first

v4 moved theme tokens into CSS, not JS:

```css
@import "tailwindcss";

@theme {
  --color-brand-500: oklch(0.72 0.18 250);
  --font-display: "InterDisplay", sans-serif;
  --spacing: 4px;
}
```

A `tailwind.config.js` is still supported via `@config "./tailwind.config.js"` but new projects should use `@theme` blocks.

## Class changes that bite v3 muscle memory

- **Default border color** is `currentColor` in v4, not `gray-200`. Add an explicit `border-gray-200` if you relied on the default.
- **`bg-opacity-50`** → `bg-black/50` (slash syntax, already common but now required).
- **`text-opacity-*`, `placeholder-opacity-*`** → slash syntax.
- **`shadow-sm`** is heavier in v4; `shadow-2xs` and `shadow-xs` are the new lightweights.
- **`rounded`** alias for `rounded-sm` is gone — be explicit.
- **`outline`** by default uses `currentColor` width 1px; `outline-none` no longer hides focus rings — use `outline-hidden`.
- **Container queries** are built-in: `@container { @md:flex-row }` without a plugin.

## Arbitrary values

Same as v3: `bg-[oklch(0.7_0.2_250)]`, `w-[42rem]`, `[mask-type:luminance]`. Prefer tokens.

## Layer order

```css
@layer base, components, utilities;
```

`@layer components { .btn { @apply px-3 py-1 rounded; } }` — `@apply` still works, use sparingly.

## Don't propose

- `tailwind init` — there's no JS init for v4 by default.
- `postcss.config.js` with `tailwindcss` and `autoprefixer` plugins — v4 plugs into Vite directly.
- Migration helpers from v3 unless asked. New projects start clean.
