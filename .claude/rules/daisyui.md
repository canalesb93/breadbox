---
paths:
  - "internal/templates/**"
  - "internal/admin/**"
  - "input.css"
  - "static/**"
  - "Makefile"
---

# DaisyUI adherence + UI dev loop

The v1 admin UI is built on **DaisyUI 5 + Tailwind CSS v4**. This rule
covers both: (a) which classes/components to use and which patterns to
avoid (daisy-first decision tree, below), and (b) the **dev loop
gotchas** that bite when you actually run the server to validate
changes (CSS rebuild, templ regen, port hygiene — at the bottom of
this file).

Short version: **daisy first, custom only when daisy can't reach**,
document every exception, and **always rebuild CSS after touching
templ files**.

## The decision tree

Before reaching for inline Tailwind utilities or writing a new
`bb-*` class, walk this in order:

1. **Is there a DaisyUI component for this?** Check
   [daisyui.com/llms.txt](https://daisyui.com/llms.txt). If yes →
   use it. Visual tuning (radii, palette, density) goes through the
   [daisy theme builder](https://daisyui.com/theme-generator/) — do
   not stamp `rounded-*` modifiers per caller.
2. **Is there a DaisyUI modifier for the variant?** `btn-soft`,
   `btn-dash`, `badge-outline`, `card-border`, `tabs-border`,
   `collapse-arrow`, `kbd-xs`, `skeleton-text`, etc. Prefer modifier
   over hand-rolled tone.
3. **Does an existing `bb-*` class already capture this pattern?**
   Search `input.css` and the templ tree before authoring a new one.
   See "Justified `bb-*` extensions" below.
4. **Is the pattern repeated 3+ times in the codebase?** If yes,
   author a templ component (preferred — typed, testable) or a new
   `bb-*` CSS class. **Do not author a `bb-*` class for a single
   caller** — the spec's "3+ uses" threshold is non-negotiable.
5. **Falling all the way through** → inline Tailwind utilities are
   fine for one-off layout glue.

## Components: daisy-first inventory

Always use the daisy component (with our standard overlays only):

| Daisy | Standard overlay | Notes |
|---|---|---|
| `btn` | `btn-sm` → `gap-2`; `btn-xs` → `gap-1.5` | Two sizes only. Icon size `w-4 h-4` (sm) / `w-3.5 h-3.5` (xs). Radius is daisy default — no per-caller `rounded-*`. |
| `badge` | `badge-soft badge-{tone} badge-sm` for status; `badge-ghost badge-xs` for metadata; solid `badge-{tone} badge-xs` for counts | Never add `rounded-lg`/`rounded-xl` to a badge. |
| `alert` | `alert alert-{tone} rounded-xl` for page-level; `alert-soft` for less prominent inline | Use `bb-form-error` inside a form card (tighter). |
| `modal` | `modal modal-bottom sm:modal-middle`; `modal-box rounded-xl` | Use for all confirm/dialog UX. See "Anti-patterns" below for the bespoke shells we're retiring. |
| `dropdown` + `menu` | `dropdown-content menu bg-base-100 rounded-xl shadow-lg border border-base-300 z-50 w-44 p-1` | Standard overflow-menu shape. |
| `table` | `table table-sm table-zebra` + `hover:bg-base-200` on `<tr>` | `table-md` for transaction list; `table-xs` for embedded. |
| `toast` | `toast toast-center toast-bottom` | One toast pattern. |
| `tooltip` | `tooltip tooltip-top` + `data-tip="…"` | Don't roll your own tooltip. |
| `loading` | `loading loading-spinner loading-xs/sm/md` | Only `loading-spinner` in practice — acceptable. |
| `drawer` | `drawer lg:drawer-open` | Sidebar layout — single use in `base.html`. For a right-side slide-over sheet use `components.Drawer`, NOT this. |
| `steps` | `steps steps-horizontal` | Wizard progress. CSV import should be on this. |
| `tabs` | `tabs tabs-border` (or `tabs-box` for filter-tabs) | **Adopt — currently 0 callers.** |
| `stat` / `stats` | `stats stats-horizontal` | **Adopt for dashboard tiles — currently 0 callers.** |
| `skeleton` | `skeleton` + Tailwind sizing | **Adopt — `bb-skeleton*` is being retired.** |
| `kbd` | `kbd kbd-xs/sm` | **Adopt — `bb-*-kbd` duplication is being retired.** |
| `join` | `join` + `join-item btn` | For segmented controls + pagination — use instead of new `.bb-*-toggle` classes. |
| `fieldset` + `fieldset-legend` | Use the legend, drop the parallel `<label>` | Currently 19 fieldsets use a redundant external label. |
| `checkbox`, `radio`, `toggle`, `textarea`, `file-input` | Native daisy with our `rounded-xl` where applicable | Faithful. |

## Justified `bb-*` extensions (do not rewrite to daisy)

These exist because daisy doesn't cover the case. Keep using them; do
**not** open a PR migrating them to daisy.

| `bb-*` class | Why kept |
|---|---|
| `.bb-card` | shadcn flat-border aesthetic + dark-mode `color-mix` lift. Daisy `card` ships shadow + `border-base-200`. Used in 82 files. |
| `.bb-tag` (+ `bb-tag-sm`/`-lg`/`-ghost`/`-interactive`/`-add`/`-remove`) | data-driven `--tag-color` from DB; daisy badges only support palette colours. |
| `.bb-tx-avatar` (+ variants) | data-driven category color via `color-mix(--avatar-color, …)`. |
| `.bb-tx-row*` | imperative bulk-selection driven by JS store; daisy has no row primitive. |
| `.bb-sidebar*` | hover/active/icon-opacity choreography + iOS-26 scroll fade gradients daisy `menu` doesn't expose. |
| `.bb-timeline*` | GitHub-style continuous rail through 24/28px tiles. Daisy `timeline` is a different shape. |
| `.bb-comment-bubble` | flat top-left corner pointing at rail avatar. |
| `.bb-progress-bar` | fixed-top SPA navigation indicator (YouTube/GitHub style). |
| `.bb-mobile-navbar` | sticky + backdrop-blur + safe-area inset on top of daisy `navbar`. |
| `.bb-icon-header__tile` (+ tone modifiers) | 40×40 colored tile slot inside form cards. |
| `.bb-form-input` / `.bb-form-select` | focus-bg shift on top of daisy `input` / `select`. |
| `.bb-action-row` | sectioned-card bottom action row. |
| `.bb-danger-card` | soft-error tinted card modifier. |
| `.bb-wizard-*`, `.bb-error-*` | isolated wizard/error page layouts. |
| `.bb-page-header` | page title row primitive (being promoted to a templ component soon). |
| `.bb-info-grid` | detail-page key-value grid. |
| `.bb-filter-bar`, `.bb-filter-label`, `.bb-filter-toggle`, `.bb-filter-form` | filter row scaffold. |
| `.bb-amount` (+ `bb-tx-amount*`) | tabular-nums + per-state styling for currency values. |

If you find yourself wanting to write a new `bb-*` class that
overlaps with daisy, **stop**: either adopt the daisy primitive or
write a templ component that wraps it.
**Right-side slide-over → `components.Drawer`.** Daisy's `drawer` is
the sidebar layout (the one in `base.html`), not a right-anchored
sheet. For focused inline create/edit flows use the shared
`components.Drawer` + `DrawerHeader` + `DrawerFooter` templ
components, opened via the global `$store.drawers` store. Don't
hand-roll the backdrop+panel+slide chrome again — see
`/design/c/drawers` and `docs/design-system.md`.


## Anti-patterns (actively being removed)

Do not write more of these. Migration PRs are queued in the
design-system sprint.

- **Bespoke modal shells.** `.bb-confirm-dialog`, `.bb-cmdk-dialog`,
  `.bb-shortcuts-dialog`, `.bb-catpicker-dialog`,
  `.bb-tagpicker-dialog` — ~250 LOC of duplicate CSS. New global
  dialogs use `<dialog class="modal">` with `modal-box`.
- **`bb-skeleton*` family.** 9 hand-rolled size variants with custom
  shimmer keyframe. Use daisy `skeleton` + `skeleton-text` with
  Tailwind sizing.
- **Hand-rolled tabs.** `mcp_guide` uses `btn-primary`/`btn-ghost`
  toggle. New tab UIs use daisy `tabs tabs-border`.
- **Hand-rolled paginator buttons.** `.bb-paginator__btn*`. Use
  daisy `join` + `join-item btn btn-sm` + `btn-active`.
- **Hand-rolled kbd badges.** `.bb-cmdk-kbd`, `.bb-shortcuts-kbd`,
  `.bb-cmdk-footer kbd`, `.bb-catpicker-footer kbd` — all duplicates
  of `.bb-kbd`. Use the shared `Kbd`/`KbdCombo` templ components
  (`internal/templates/components/kbd.templ`).
- **Hand-rolled rounded-full chips that bypass `badge`.** Several
  sites use `inline-flex … rounded-full bg-{tone}/10 text-{tone}` —
  use `badge badge-soft badge-{tone}`.
- **Hand-rolled empty-state scaffolds.** `bb-card p-12 text-center
  flex flex-col items-center` ... etc. 18 callers, 5 variants. Use
  the upcoming `EmptyState` templ component.
- **Hand-rolled page-header `<div><h1><p>` blocks.** 37 callers.
  Use the upcoming `PageHeader` templ component.
- **Multiple destructive-confirm patterns.** Inline-confirm + daisy
  modal + `bb-confirm-*` overlay coexist. Standardise on
  `bb-confirm-*` (richest UX, already wired in `base.html`).

## DaisyUI 5 specifics

- **`input-bordered` / `select-bordered` are redundant in daisy 5.**
  The bordered look is now the default. The classes still ship for
  backwards compat — drop them when touching nearby code.
- **`appearance: base-select` strips backgrounds in dark mode.** Our
  `.select::picker(select)` overrides in `input.css:1024-1046`
  patch this. **Keep them — do not delete.**
- **Daisy 5 ships new components we should use:** `validator`,
  `status`, `list`, `filter`, `card-dash`. Don't roll equivalents.

## Spec'd overrides (the only `!important` and only deep overrides)

These are documented load-bearing overrides — leave them alone.
Don't pile new `!important` onto daisy classes.

| Selector | Why |
|---|---|
| `.btn { background-clip: padding-box; transition: …; !important }` + `.btn:active:not(:disabled):not([aria-haspopup]) { transform: translateY(1px) !important }` | Nova-flavored press feedback. `translateY` (not `scale`) so ring/border treatments stay crisp; `bg-clip-padding` so the ring doesn't bleed at press; `:not([aria-haspopup])` so dropdown triggers don't bob when opening a menu. `!important` because daisy ships same-specificity transitions. |
| `.join:has(> .btn-{tone}, …) { box-shadow: ring + drop }` + items inside get `box-shadow: none !important` | Lifts daisy's per-button depth shadow onto the `.join` container so split-buttons render as one unit (no seam at the inner edges of join-items). Scoped via `:has()` to joins containing a solid-tone button — flat joins (pagination, ghost segmented) stay flat. |
| `.modal-box { box-shadow: …; !important }` | Nova ring + soft drop replaces daisy's default shadow-2xl. `!important` because daisy's plugin layer emits a same-specificity rule that wins on source order otherwise. |
| `.dropdown-content { box-shadow: ring + drop }` (NO `!important`) | Nova default elevation for floating menus. Deliberately no `!important` so per-caller `shadow-lg`/`shadow-xl` (the daisyui.md-mandated standard dropdown shape, settings_modal tab picker, etc.) still wins by specificity. |
| `.modal-backdrop { transition: opacity 0.2s ease !important }` | Small backdrop polish. |
| `[x-collapse] { transition: height …; !important }` | Alpine's `x-collapse` plugin sets inline styles; the rule overrides. |
| `.drawer-side { z-index: 40 !important }` | Stacks above the sticky mobile navbar (z-30). |
| `.checkbox { transition: bg + border only }` (`input.css:1136`) | Scope daisy's `transition: all` to avoid paint thrash on tx rows. |
| `.select::picker(select)` dark-mode block | Workaround for daisy 5 `appearance: base-select` bug in dark mode. |
| `.bb-timeline-prominent` overrides at the bottom of `input.css` | Scaled-up timeline tile/leading targets. |

## When daisy isn't enough

Open a sprint PR — **don't** write a new `bb-*` class quietly. The
checklist:

1. Author the templ component (preferred) or `bb-*` class.
2. Add it to the `/design` sandbox (`internal/templates/components/pages/design_sections.templ` — new section in
   `DesignSections()` in `design_types.go`) with a representative
   variant matrix.
3. Update `docs/design-system.md` with the canonical usage explaining why daisy can't reach this case.

The sandbox-first rule ensures every new shared component has at
least one place reviewers can see it without reading the call site.

## UI dev loop — gotchas that bite every session

Validating UI changes means running the server and rendering them.
These are the failure modes that cost the most debugging time during
the design-system sprint.

### Rebuild CSS after any templ edit

`make dev` runs `templ generate` but **also runs `make css`** (added
in the sprint). `make dev-watch` runs `tailwindcss-extra --watch`
alongside `air` so it's handled automatically. The footgun: an ad-hoc
session that boots the binary directly (`go run ./cmd/breadbox serve`
or a prebuilt binary) **skips both steps**.

- Tailwind v4 scans `*_templ.go` (the generated Go output) for class
  names. Templ source files alone are not enough — generation must
  run first, then Tailwind must scan.
- If `static/css/styles.css` is older than your newest `*_templ.go`,
  every utility class you just introduced is **missing from the
  bundle** and the page renders without them. The breakage is
  silent — no error, just wrong layout.

When booting the server manually:

```sh
templ generate          # writes *_templ.go from .templ sources
make css                # rebuilds styles.css against the latest classes
go run ./cmd/breadbox serve   # or your prebuilt binary
```

If you set `BREADBOX_DEV_RELOAD=1`, the running binary reads
`static/css/styles.css` from disk on every request — so a fresh
`make css` is picked up by a browser reload without restarting Go.
That's the fastest iteration loop when you're only tweaking CSS or
templ markup.

### Templ files are gitignored — regenerate after every checkout

`*_templ.go` is in `.gitignore`. After `git checkout <branch>` (or a
fresh worktree), the templ output files don't exist yet. Building
without first running `templ generate` will either fail (missing
symbols) or, worse, succeed against the stale outputs from the
previous branch. **Always run `templ generate` before `go build` on
a fresh tree.**

`make generate` (which most other targets depend on) handles this
automatically. The trap is invoking `go build` or `go run` directly
without going through the Makefile.

### Validate in a real browser — don't trust JPEG contrast

Screenshots are JPEG-encoded. Subtle opacity (`text-base-content/50`,
soft surfaces) can read as "the page is dim/broken" in a JPEG when
the page is actually fine. Before diagnosing a dim/contrast
regression, **verify with `getComputedStyle`**:

```js
const el = document.querySelector('.bb-page-title');
const cs = getComputedStyle(el);
// Expect oklch(0.145 0 0) and opacity 1 in light mode.
({ color: cs.color, opacity: cs.opacity });
```

In this sprint a "dim page" diagnosis was actually a JPEG quirk —
the colors were correct. Cost: one wrong-direction debugging detour.

### Kill stale dev servers before booting yours

Multiple sessions tend to leave breadbox binaries running on
8080–8099. Before booting, check what's listening:

```sh
lsof -iTCP -sTCP:LISTEN -P | grep -E '808[0-9]|breadbox'
```

Kill stale ones (the user has confirmed it's fine in this repo's
session model) with `kill <pid>`, then take the freed port. Don't
pick a port outside 8080–8099 — that range is what other dev
tooling expects.

### Recover ENCRYPTION_KEY without restarting providers

If a `breadbox serve` instance is already running, grab its key
rather than minting a new one (existing encrypted tokens won't
decrypt under a fresh key):

```sh
ps eww -p $(pgrep -f 'breadbox serve' | head -1) \
  | tr ' ' '\n' \
  | grep '^ENCRYPTION_KEY=' \
  | cut -d= -f2
```

Pair with the standard DATABASE_URL and `SERVER_PORT=<your port>`:

```sh
DATABASE_URL='postgres://breadbox:breadbox@localhost:5432/breadbox?sslmode=disable' \
ENCRYPTION_KEY='<recovered>' \
SERVER_PORT=8089 \
BREADBOX_DEV_RELOAD=1 \
$TMPDIR/breadbox-mine serve
```

### sqlc-generated files can look stale via the build cache

If `go build` complains about a missing field in a `db.*Params`
struct (e.g. `unknown field ErrorMessage`), the most likely cause is
a stale Go build cache — not a real divergence. Run `sqlc generate`
(or `make sqlc`) once; if the generated file is bit-identical, the
cache invalidates and the build succeeds on the retry.

This is rare but it cost real time in this sprint — flag if you see
"unknown field" on a stable branch.

### Sandbox / .git ref-lock errors

Running `git push`, `git checkout -b`, or `git fetch` can hit
`Operation not permitted` on `.git/refs/...lock` because Claude
Code's sandbox blocks certain `.git` writes. The fix is one of:

- Set `dangerouslyDisableSandbox: true` on the specific Bash call
  that needs to write to `.git/refs/...`. Don't blanket-disable.
- Or use `gh` for PR-side operations (it goes through the GitHub
  API, not local refs).

### Process recap

For UI sprint work specifically:

1. `git checkout <branch>` → `templ generate` (regenerates outputs).
2. Make your templ / CSS edits.
3. `make dev` (rebuilds CSS + Go + boots) or `make dev-watch` (auto).
4. Browser to `http://localhost:<PORT>/<route>` and verify visually.
5. Sanity-check colors / opacity via `getComputedStyle` if anything
   looks off before assuming a regression.
6. Take screenshots via Chrome DevTools MCP for PR evidence; embed
   per `.claude/rules/ui.md` (validate-ui skill).

The sandbox at `/design` and per-component standalone viewer at
`/design/c/{slug}` are the canonical proving ground. New components
land there first, then propagate to live pages.

## References

- `docs/design-system.md` — canonical spec
- `.claude/rules/ui.md` — general admin UI conventions (validation
  skill, browser automation backends, screenshot upload flow)
- `https://daisyui.com/llms.txt` — canonical machine-readable
  component list (always check before writing custom)
