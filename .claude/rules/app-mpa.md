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

## JS islands (esbuild-via-Go, Node-free)

JS is a last resort, scoped to **named islands**: tiny, dependency-free TS modules that hydrate
server-rendered DOM and wire *behavior only*. An island must never own navigation — it only
triggers real `location.href` navigations, so the browser keeps owning history/scroll/bfcache.
With JS off, the island simply doesn't appear and everything stays reachable (pure progressive
enhancement). Islands shipped: the ⌘K command palette (`palette`), and the agent-run live-transcript
SSE consumer (`agent-run-stream`, see "Streaming surfaces" below).

**Where things live**
- Source TS: `internal/webapp/assets/js/islands/<name>.ts` (one entrypoint per island).
- Build program: `internal/webapp/cmd/buildjs` (own `package main`) — uses esbuild's Go API
  (`github.com/evanw/esbuild/pkg/api`), no Node/npm.
- Built bundles: `internal/webapp/static/js/islands/<name>-<hash>.js` (content-hashed, **committed**,
  like `static/css/app.css`). Embedded via `//go:embed all:static`.
- Manifest: `internal/webapp/static/js/islands/manifest.go` (`package islands`, generated, committed)
  — a `map[string]string` of logical name → hashed filename. Resolve URLs in templ via
  `layout.IslandSrc("<name>")` (`internal/webapp/layout/islands.go`), which falls back to
  `<name>.js` if the manifest has no entry yet.

**Build / caching**
- `make webapp-js` runs the buildjs program (bundle + minify + hash + regenerate manifest.go).
- Folded into `make generate` (rebuilds when any `*.ts` is newer than `manifest.go`), into `make
  build` (transitively via `generate`), and into `make dev-watch` (a 1s poll rebuilds on TS edits).
- Content-hashed names get `Cache-Control: immutable` automatically — `embed.go::looksFingerprinted`
  already special-cases `…-<hash>.js`. That's why we hash instead of using stable names.
- The `<script type="module" src={ IslandSrc("...") }>` tag lives in `layout/base.templ`.

**Adding another island** (e.g. the drag-drop rule builder)
1. Write `internal/webapp/assets/js/islands/<name>.ts` — tiny, no npm deps, hydrate a server
   element, never hijack link nav.
2. Add the `<script type="module" src={ IslandSrc("<name>") }>` to `base.templ` (global) or the
   specific page's templ (scoped).
3. Add any `.<name>*` styles to `assets/css/input.css` using design tokens (never raw colors).
4. Run `make webapp-js` (or `make generate`) — it bundles, hashes, and regenerates `manifest.go`.
   Commit the new bundle + the updated manifest alongside your `.templ`/CSS changes.

## Streaming surfaces (SSE) — sync progress, agent runs, activity timeline

SSE is the v3 real-time mechanism, scoped to **streaming surfaces only** — never static CRUD. The
shape below is hand-rolled `text/event-stream` (the Datastar Go SDK is *allowed* but unnecessary for
append-only growth; reach for it only when a surface needs server-driven fragment diffing rather than
appending). First surface shipped: **agent run live transcript** (`agents_stream.go`).

**The contract — server-first, SSE-enhances, degrades to static**
1. **The page renders current state server-side.** No SSE, no JS → the page still shows everything
   on disk/in the DB right now, plus a real `<a href>` **Refresh** link. "Unsupported" means "plain,"
   never "broken." (For the transcript: the lines already written are rendered at first paint.)
2. **A handler streams only NEW data as SSE.** `Content-Type: text/event-stream`, `Cache-Control:
   no-store`, `X-Accel-Buffering: no`. Get an `http.Flusher`, `Flush()` after each frame. Emit named
   events: `event: <name>\ndata: <payload>\n\n`. End the stream with a terminal event (e.g.
   `event: done`) when the underlying work reaches a terminal state, then return. Honor
   `r.Context().Done()` so a client disconnect tears the goroutine down.
3. **The client passes a cursor so the server never replays.** The island counts the
   server-rendered nodes and connects with `?from=<n>`; the handler skips the first `n` items.
   EventSource auto-reconnects on transient drops, and the cursor keeps reconnects gap-free.
4. **A tiny page-scoped island consumes it.** `data-stream-live="true"` + `data-stream-url` on the
   target element gate hydration; the island opens an `EventSource`, appends a node per `event:
   line`, and flips the status pill on `event: done`. With JS off the element has the data attrs but
   nothing reads them — the static render stands. The island is page-scoped (`<script type="module">`
   rendered only when `Live`), not global in `base.templ`.

**Why tail-a-file for agent transcripts:** the sidecar already writes the run's NDJSON transcript
line-by-line (`internal/agent/sidecar.go`); the handler just re-reads past the cursor each tick
(~750ms) and checks run status for the terminal condition. Path resolution mirrors the REST handler
(`internal/api/agents.go`): prefer `agent_runs.transcript_path`, else fall back to
`<agent.transcript_dir>/<run.ID>.ndjson` — the column is only persisted on completion, so the
fallback is what makes an *in-progress* run streamable at all.

**Routing:** register stream routes via the surface's own registrar (e.g. `registerAgentStream`
called from `registerAgents`), not the central `Router` in `handler.go`. Never render secrets into a
stream (no tokens, no API keys).

**Remaining streaming surfaces (follow-ups):** sync progress (SSE that re-renders the sync-status
fragment on sync-engine events / an interval) and the activity timeline (append new events). Both
follow the same server-first + cursor + terminal-event contract.

## Do not

- Don't import or reuse v1 admin (`internal/admin`, `internal/templates`) markup/CSS. Clean start.
- Don't add a client router, hijack links, or touch `history`.
- Don't raw-color anything; use tokens. Don't fork components; compose.
- Don't reach for Datastar/SSE on non-streaming surfaces, or an island where a native element works.
- Don't gate `/app` mounting anywhere but the existing `!a.Config.NoDashboard` block in `router.go`.
