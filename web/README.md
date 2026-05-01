# Breadbox web prototype

Standalone Vite + React + TypeScript SPA hitting the existing Go backend at
`/api/v1/*`. Sits alongside (does **not** replace) the current `html/template`
admin UI.

Stack: React 18, TypeScript 5, Vite 5, TanStack Router, TanStack Query.

## Run

```sh
# 1. Backend (in repo root)
make dev   # or: go run ./cmd/breadbox serve   — listens on $PORT (8081 in worktree, 8080 in main)

# 2. Frontend (in web/)
cd web
bun install
bun run dev    # vite on http://localhost:5173, proxies /api/* to backend
```

If your backend is on a non-default port:

```sh
BREADBOX_BACKEND_PORT=8082 bun run dev
```

## Auth

The header has a small input — paste an API key (`bb_...`) to load real data.
Stored in `localStorage` and sent as `X-API-Key` on every request. Hit "Reset"
in the header to clear it.

Generate a key from the existing admin UI: `Settings → API Keys → New`.

## What's here

- `src/api.ts` — typed fetch wrapper + `ApiError` envelope matching `internal/api/response.go`.
- `src/main.tsx` — TanStack Router + Query bootstrap.
- `src/routes/Accounts.tsx` — table view of `/api/v1/accounts`.
- `src/routes/Categories.tsx` — grid view of `/api/v1/categories` with client-side filter.
- `src/components/Layout.tsx` — chrome (nav + API-key input).

Inline styles only — no Tailwind/DaisyUI in the prototype to keep it dependency-light.
For a real migration, drop Tailwind v4 in here (the existing `static/css/styles.css` config is portable).

## What's NOT here yet (intentional)

- Production embed into Go binary (would use `embed.FS` + SPA fallback handler).
- OpenAPI codegen for `Account`/`Category` types — hand-typed for the prototype.
- Auth via session cookies (would need same-origin deploy).
- File-based routing via `@tanstack/router-plugin` — code-based for clarity.

## Next steps if pursued

1. Add OpenAPI spec or hand-rolled JSON Schema → generate TS types.
2. Wire up `embed.FS` in Go to serve `web/dist/` at `/web` (or `/`) with SPA fallback.
3. Pull in Tailwind + a UI primitives lib (shadcn/ui or similar).
4. Build out transactions page (TanStack Table + cursor pagination).
