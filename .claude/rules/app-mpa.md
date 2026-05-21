---
paths:
  - "internal/webapp/**"
  - "internal/webapp/assets/**"
---

# v3 webapp — browser-native MPA (`internal/webapp`, mounted at `/app`)

The v3 admin UI: a true multi-page app, server-rendered with templ, styled with Tailwind v4
(Node-free standalone CLI) in the shadcn/Basecoat design language. It replaces the v2 React SPA
(`web/`, `/v2`). Plan of record: `~/Documents/obsidian/Breadbox/planned-features/v3-browser-native-mpa-plan.md`.
Sprint state: `.claude/v3-sprint-state.md`.

## The one rule that drives everything: the browser owns navigation

Every URL is a real document. **Never** add a client router, intercept link clicks for nav, or
manipulate history. Real `<a href>` + real server routes. Back/forward/scroll/bfcache are the
browser's job — that is the entire reason this rewrite exists. If you find yourself reaching for JS
to manage navigation, stop: you're rebuilding the SPA mistake.

## Browser-native first

Before reaching for JS, use the platform:
- Page transitions → `@view-transition { navigation: auto }` (in `app.css`) + `view-transition-name`.
- Prefetch → Speculation Rules (`<script type="speculationrules">` in `layout/base.templ`).
- Modals → `<dialog>` + `showModal()`. Menus/popovers → Popover API (+ JS placement fallback in
  `app.js`, since CSS Anchor Positioning isn't in Safari/FF yet). Accordions → `<details>`.
- Forms → native `<form>` POST/GET + Constraint Validation API + **server-side** validation with
  error re-render (re-render the page/fragment with field errors; no JSON round-trip, no client
  validation libs).
- Mobile drawer → CSS-only checkbox toggle (`#nav-toggle` peer). It auto-closes on navigation
  because every nav is a fresh document. No JS.
JS is a last resort, scoped to named islands (Phase 4: ⌘K palette, drag-drop) bundled by
esbuild-via-Go. Datastar + SSE is scoped to streaming surfaces only (sync progress, agent runs,
activity timeline) — never reach for it on static CRUD.

Everything degrades to plain-but-fully-functional. "Unsupported" must mean "plain," never "broken."

## Package layout

- `internal/webapp/*.go` — chi subrouter + handlers (`handler.go`, `middleware.go`,
  `auth_handlers.go`, `pages_handlers.go`, `helpers.go`). Handlers call `a.Service` (the shared
  service layer) directly — no HTTP round-trips, no duplicate data access.
- `pages/*.templ` — one component per page/route (+ `pages/format.go` for view helpers like
  `Money`, `TitleCase`, `Deref`). Pages compose `layout` + `components` and read `service` types.
- `components/*.templ` — reusable templ components (icons + Basecoat-styled primitives).
- `layout/*.templ` + `layout/nav.go` — app shell (`Base` head, `Shell` sidebar/topbar) + nav IA.
- `assets/css/input.css` — Tailwind source. `static/` — built/embedded assets (`//go:embed all:static`).

Handlers shape input/response → `webapp`; stateful behavior → `internal/service`; never duplicate
service logic in a handler.

## Components are sacred — compose, never fork (forward-thinking + reusable)

The look lives in **semantic CSS classes** in `input.css` (`.btn`, `.card`, `.badge`, `.input`,
`.data-table`, `.menu`, …) using design tokens (`bg-primary`, `text-muted-foreground`) — **never raw
colors**. templ components in `components/` are thin wrappers over those classes. To add UI:
1. Reuse an existing primitive/class.
2. If a pattern recurs 3+ times, add a primitive (templ wrapper) or a class — once, in one place.
3. Never copy-paste a variant into a page. If a page needs a tweak, parameterize the component.
`btn`/`badge` are registered as Tailwind `@utility` so other classes can `@apply` them (v4 `@apply`
only resolves utilities, not `@layer components` classes — this bites; remember it).

## templ workflow

- Author `.templ`; run `make templ` (runs `templ generate` then injects the build tag — see below).
- Run `templ fmt .` before committing. Commit generated `*_templ.go` siblings.
- Never hand-edit `*_templ.go`.
- One `style=` attribute per element (templ won't merge duplicates — the browser keeps only the first).

## Build (Node-free) + build tags

- CSS: `make webapp-css` (`tailwind-cli-extra` standalone, no node_modules). `make generate` rebuilds
  it when stale; `make dev-watch` watches it. Output `internal/webapp/static/css/app.css` is embedded.
- **Every webapp `.go` file carries `//go:build !headless && !lite`** (dashboard-only + server-only),
  matching `internal/admin`/`web`. The `make templ` target auto-injects this into webapp `*_templ.go`.
  `stubs_headless.go` (`//go:build headless`) provides `New`/`Router` so `internal/api` imports it
  unconditionally. Don't drop these tags or the CI matrix (default/headless/lite) goes red.

## Auth, session, CSRF

- Shares the `alexedwards/scs` session cookie with v1 admin and the SPA (single login across `/`,
  `/v2`, `/app`). Pass the same `sm` instance into `webapp.New(a, sm)`.
- Authenticated routes use `requireAuth` → real server-side 303 to `/app/login?next=<dest>`. No JS
  gate, so the SPA's "401 redirect trap" class of bug cannot exist. `next` is open-redirect-guarded
  via `safeNext` (must be a same-site `/app` path).
- Login reuses `queries.GetAuthAccountByUsername` + bcrypt + `sm.RenewToken` +
  `admin.SetLoginSessionKeys` — same session establishment as v1/SPA. Don't reimplement it.
- CSRF: `requireSameOrigin` on unsafe methods (SameSite=Lax + Origin/Referer host check via
  `mw.SameOrigin`). Native same-origin form POSTs pass by construction.

## Theme

`bb_theme` cookie (`light`/`dark`/`system`) read server-side in `themeClass` → `.dark` on `<html>`
for no-flash first paint. `app.js` sets the cookie + toggles on click. `system` resolves client-side.

## Mobile/iOS (ported from the SPA's hard-won lessons — they still apply)

`dvh` units (`min-h-dvh`), `env(safe-area-inset-*)` padding on the shell, 44pt min tap targets on
coarse pointers (in `app.css`), `enterkeyhint`/`inputmode`/`autocomplete` on inputs,
`prefers-reduced-motion` disables View Transitions. Validate every surface at 390×844 too.

## Validation / PR evidence (required)

Validate in a real browser before reporting done; attach screenshots to the PR.
- Server: build (`go build -o breadbox ./cmd/breadbox`) + run on a worktree port
  (`SERVER_PORT=<port> ./breadbox serve`, env from `.local.env`). Dev login: `admin@example.com` / `password`.
- Use the Chrome DevTools MCP (`mcp__plugin_chrome-devtools-mcp_chrome-devtools__*`). Capture desktop
  (1280×800) **and** mobile (390×844); include dark mode when relevant; check `list_console_messages`
  for errors. Upload via the `github-image-hosting` skill (img402) — never the GitHub release-asset CDN.
- Inline images in PR bodies with bounded width: `<img src="https://i.img402.dev/<id>.jpg" width="800">`.

## Do not

- Don't import or reuse v1 admin (`internal/admin`, `internal/templates`) markup/CSS. Clean start.
- Don't add a client router, hijack links, or touch `history`.
- Don't raw-color anything; use tokens. Don't fork components; compose.
- Don't reach for Datastar/SSE on non-streaming surfaces, or an island where a native element works.
- Don't gate `/app` mounting anywhere but the existing `!a.Config.NoDashboard` block in `router.go`.
