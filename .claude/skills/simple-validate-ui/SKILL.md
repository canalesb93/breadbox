---
name: simple-validate-ui
description: >
  Fast UI evidence for the v2 SPA. One Bun command captures the page at desktop,
  tablet, and mobile viewports and writes a single composite JPEG to `tmp/` —
  ready to embed in a PR. Triggers: "validate the v2 page", "screenshot the v2
  SPA", "responsive evidence for /v2/...", "show how the v2 page looks on all
  sizes". Use this in place of validate-ui when the change lives in `web/`
  (the React/Vite SPA).
---

# Simple validate UI

Single-shot responsive evidence for the v2 SPA. The script lives at
`web/scripts/validate.ts` and uses Playwright's bundled headless Chromium to:

1. Open the page at `http://localhost:8080<path>` (auto-detects port; `BASE_URL` overrides).
2. Log in with dev creds (`admin@example.com` / `password`) if redirected to `/login`.
3. Capture three viewport-only screenshots: **desktop 1280×800**, **tablet 768×1024**, **mobile 390×844**.
4. Composite them side-by-side via a Playwright-rendered HTML page.
5. Write a single JPEG (q=85, viewport-only) to `tmp/validate-<slug>-<timestamp>.jpg`.

## When to use this vs `validate-ui`

| Use this skill              | Use `validate-ui`              |
| --------------------------- | ------------------------------ |
| Anything in `web/` (v2 SPA) | Anything in `internal/templates/`, `static/css/`, `static/js/admin/` (v1 admin UI) |
| Need responsive evidence    | Need a single-viewport capture |
| Don't need a CDN-hosted URL yet | Need a permanent URL embeddable in a PR comment |

Pair this with `github-image-hosting` (img402.dev) when a PR-attachable URL is needed.

## Steps

### 1. Make sure the SPA is reachable

Either:

- Backend with built SPA: `make build && ./breadbox serve` (or `make dev` after `make web`). Default URL `http://localhost:8080`.
- Vite dev server (HMR): `make web-dev` → set `BASE_URL=http://localhost:5173`.

The script probes `/health/live` first and prints a clear message if nothing answers.

### 2. Run

```bash
cd web && bun run validate /v2/transactions
# → tmp/validate-v2-transactions-<ts>.jpg
```

`PATH` is positional. Defaults to `/v2/`.

Useful overrides:

```bash
BASE_URL=http://localhost:5173 bun run validate /v2/         # vite dev
BB_USER=other@x.com BB_PASS=... bun run validate /v2/        # different creds
PORT=8081 bun run validate /v2/                              # worktree port
```

### 3. (Optional) attach to a PR

Use the `github-image-hosting` skill (img402.dev) when the change ships in a PR.
For local review, the JPEG in `tmp/` is enough.

## Conventions

- **Output**: always `tmp/validate-<slug>-<ISO-stamp>.jpg`. `<slug>` is the path with slashes folded to dashes (`/v2/transactions` → `v2-transactions`).
- **Composite layout**: dark background, three figures with viewport labels, gap 16px, padding 16px, JPEG q=85.
- **Auth**: only auto-fills the login form when redirected from a non-`/login` path. Asking for `/login` directly captures the form as-is.
- **Idempotent install**: `bun install` once in `web/` (playwright is a devDep). On the first run `bunx playwright install chromium` may be needed if the browser cache is empty — the script does not auto-install, by design.

## Adding a new viewport

Edit the `VIEWPORTS` array at the top of `web/scripts/validate.ts`. Order in the array = left-to-right order in the composite.

## Don't

- Don't use this for the v1 admin UI (templ / Alpine pages) — selectors for forms and shells differ. Use `validate-ui`.
- Don't enlarge captures with `fullPage: true` here. The composite is meant to be reviewable in a PR; tall full-page strips become unreadable. If you need below-the-fold evidence, scroll the page in a separate capture flow.
- Don't commit anything in `tmp/` — it's gitignored (shared with `air`'s build output).
