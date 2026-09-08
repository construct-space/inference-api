# Calling external APIs from a Construct space

Spaces are sandboxed Vue apps. External HTTP calls go through `fetch`.

## Pick a free, no-key API when one exists

Default to APIs that need no signup and no API key. Examples:

- Weather → `api.open-meteo.com` (no key)
- Geocoding → `geocoding-api.open-meteo.com` (no key)
- Currency → `api.exchangerate.host` (no key)
- IP location → `ipapi.co/json` (no key)

If the only viable API requires a key, see "Secrets" below — but prefer the no-key option even if it means a slightly different shape.

## The pattern

```ts
// src/composables/useWeather.ts
// Open-Meteo — free, no key needed.
export async function getForecast(lat: number, lon: number) {
  const url = `https://api.open-meteo.com/v1/forecast?latitude=${lat}&longitude=${lon}&current=temperature_2m,weather_code`
  const res = await fetch(url)
  if (!res.ok) throw new Error(`weather: ${res.status}`)
  return res.json()
}
```

## In an action

```ts
// src/actions.ts
import { geocode, getForecast } from './composables/useWeather'

export const actions = {
  forecast: {
    description: 'Get current weather for a city.',
    params: { city: { type: 'string', required: true } },
    run: async ({ city }: { city: string }) => {
      const geo = await geocode(city)
      if (!geo) return { ok: false, error: `City not found: ${city}` }
      const data = await getForecast(geo.lat, geo.lon)
      return { ok: true, temp: data.current.temperature_2m }
    },
  },
}
```

## Secrets

There is **no per-space secret store** in the SDK today. The `@construct-space/sdk` package does **not** export `useSpaceConfig` — don't import it.

If the API truly needs a key:

- Read it from the user via a `pages/settings.vue` input bound to a single-row settings model in the graph. The user enters their own key; you store it in their personal graph data.
- Or expose it as an `agent` config field if the in-space agent needs it.
- Never hardcode keys. Never commit `.env` with real values.

For most v1 spaces, picking a no-key API is the right tradeoff.

## Auth patterns

**API key (header)**:

```ts
const res = await fetch(url, {
  headers: { Authorization: `Bearer ${apiKey}` },
})
```

**OAuth (per-user)**: use the Construct identity bridge — `useIdentity().getProviderToken('google')`. Lets the user grant once; token refresh handled by Construct.

**Webhooks (inbound)**: not in v1. Use polling from a scheduled action, or run a tiny backend service for the webhook receiver and have it call your space's published action endpoint.

## CORS / proxying

Some APIs reject browser origins. If you hit CORS, route through a small Construct provider service (Go or FastAPI) deployed on construct-network and call that from the space.

## Rate limiting

`fetch` is unbounded — add your own throttle for chatty APIs. Use a Construct Graph model (`ApiCall` with `timestamp`) to track + cap if needed.

## Common gotchas

- `fetch` runs in the desktop app's Vue context (Chromium) — assume browser-quality DNS, no Node-only modules.
- Errors stream to the in-space agent if uncaught. Wrap in `try/catch` only at boundaries.
- Don't `await` a `subscribe` from a Graph model — it's a long-lived listener, not a promise.
- For JSON APIs, always check `res.ok` before `.json()`. Non-200 with a JSON body looks fine to the parser and silently propagates bad data.
