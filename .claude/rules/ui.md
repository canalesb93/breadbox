---
paths:
  - "internal/templates/**"
  - "internal/admin/**"
  - "input.css"
  - "static/**"
---

# Admin UI

## Stack

- `html/template` (standard library), no templating engine.
- **DaisyUI 5 + Tailwind CSS v4** via `tailwindcss-extra` standalone CLI. **No Node.js**.
- **Alpine.js v3** via CDN for interactivity. No build step.
- **Lucide** icons via CDN. `data-lucide="icon-name"` attributes replaced with inline SVG by `lucide.createIcons()`.

## CSS build

- `make css` compiles `input.css` → `static/css/styles.css`.
- `make css-watch` for dev (rebuilds on change).
- Dockerfile runs `make css` in the build stage. Don't commit `styles.css` changes — it's a build artifact.
- **CSS is embedded into the binary** via `static/embed.go` (`//go:embed all:css favicon.svg`). In a plain `make dev` server you must **restart** after `make css` for changes to take effect — a browser hard-reload alone won't help because the running binary serves the stale embedded copy. `make dev-watch` avoids this (see below).

## Hot-reload dev loop — prefer this for UI work

`make dev-watch` runs three things together so UI edits apply without restarting the server:

1. **`tailwindcss-extra --watch`** — rebuilds `static/css/styles.css` on every `input.css` or template change.
2. **`air`** — rebuilds and restarts the Go binary on `*.go` changes only (config: `.air.toml`). HTML/CSS edits do **not** trigger a Go rebuild.
3. **`BREADBOX_DEV_RELOAD=1`** — makes the running binary serve templates and static files from disk (`internal/templates/` and `static/`) instead of the embedded FS. Templates are re-parsed on every request so `.html` edits apply on reload.

Typical agent loop becomes: edit `.html` or `input.css` → reload browser. No restart, no `make css`, no rebuild.

Caveats:
- Run `make dev-watch` from the **repo root**. The dev-reload paths are relative (`internal/templates`, `static`). Override with `BREADBOX_TEMPLATES_DIR` / `BREADBOX_STATIC_DIR` if needed.
- `BREADBOX_DEV_RELOAD=1` is dev-only. Never set it in prod — it disables the embedded FS and re-parses templates per request (slow, and broken if the source tree isn't present).
- Go code changes still trigger a ~1–2s restart via air. If you edited only HTML/CSS and see a restart, check whether a template change touched `*.go` (unlikely) or something else in the Go build graph.
- For CI / production builds, `make dev-watch` is irrelevant — embedded FS is always used, and the server behaves exactly as before.

Use `make dev` (no watch) when you specifically want the production embedded-FS behavior — e.g. validating an embed regression.

## Footguns

### Never use `bg-base-200/50` (or any `/opacity` modifier) on `<select>` elements

Alpha transparency renders as **fully transparent** in browsers on `<select>`. Use solid `bg-base-200`. `<input>` handles `/50` fine — the bug is specific to `<select>`.

### SPA progress bar requires manual cleanup on error paths

`base.html` has a global progress bar and content fade (opacity/blur/pointer-events) that **auto-starts on internal link clicks**. When JS does async work (e.g., `fetch` + `window.location.href` on success), every error path **must** call:

```js
window.bbProgress.finish();
document.querySelector('main').style.opacity = '';
document.querySelector('main').style.filter = '';
document.querySelector('main').style.pointerEvents = '';
```

Otherwise the progress bar trickles forever and the page stays blurred/unclickable. Convention: define a `restorePageState()` helper at the top of each Alpine component and call it on every error/early-return.

## Conventions

### DaisyUI components (replaces old `bb-*` classes)

Use these built-in components: `drawer` (sidebar), `stat` (metric cards), `table` (data tables), `badge` (status), `menu` (nav), `card` (sections), `modal` (dialogs), `toast` + `alert` (notifications), `steps` (wizard progress), `collapse` (accordions).

### Custom `@apply` classes

Keep these in `input.css` for app-specific repeated patterns:
- `.bb-filter-bar`, `.bb-pagination`, `.bb-action-bar`, `.bb-amount`, `.bb-info-grid`.

Add new ones only when a pattern appears 3+ times — premature abstraction otherwise.

### Spacing tokens

Use `--bb-gap-xs` (0.25rem) through `--bb-gap-xl` (2rem) defined in `:root`. Don't hardcode spacing.

### Dark mode

DaisyUI `light`/`dark` themes with `prefers-color-scheme` auto-switch. **No hardcoded `data-theme`** — let the OS drive it. Badge and flash colors use DaisyUI semantic classes (`badge-success`, `alert-error`, etc.) so they adapt automatically.

### Template helpers

- `BaseTemplateData(r, sm, currentPage, pageTitle)` → `map[string]any`. Handlers extend the returned map.
- `statusBadge()` / `syncBadge()` render status chips — use these instead of copy-pasted if-chains.
- `errorMessage()` maps provider error codes to human-friendly strings.
- `configSource()` renders the env/db/default badge next to config values.

### Replacing `alert()` / `confirm()`

Use Alpine inline patterns (modal + `x-data` state). Never blocking browser dialogs — they're hostile in an admin context and ignore dark mode.

## Keyboard shortcuts

New pages must register their scope. In the page's root `x-data`, set the scope on init and reset on destroy:

```html
<div x-data x-init="$store.shortcuts.setScope('my-page')" x-destroy="$store.shortcuts.setScope('global')"></div>
```

All shortcut bindings go through the Alpine store, never raw listeners:

```js
document.addEventListener('alpine:init', function () {
  var reg = Alpine.store('shortcuts');
  if (!reg) return;
  reg.register({
    id: 'my-page.do-thing',
    keys: 'd',
    description: 'Do the thing',
    group: 'Actions',
    scope: 'my-page',
    action: function () { doTheThing(); },
  });
});
```

Never add a fresh `addEventListener('keydown', ...)` for a UI shortcut — the global dispatcher in `base.html` already handles input-field short-circuiting, overlay suppression, touch-device gating, and chord state. Raw listeners bypass all of it.

Visible `kbd` hints must use the `Kbd` / `KbdChord` components in `internal/templates/components/kbd.templ` (or guard with `x-show="!$store.device.isTouch"`) so they disappear on touch devices. Don't hand-roll new `<kbd>` spans in new code.

Full reference, including the canonical global + per-page shortcut tables and architecture notes, lives in `docs/keyboard-shortcuts.md`.

## Modal & AJAX patterns

Admin list pages (reviews, rules, reports) use AJAX actions with card fade-out animations via Alpine. See `/reviews` (`reviewQueue()` component) for the reference pattern: inline approve/reject/skip/dismiss, optimistic UI, fade transitions, error recovery via `restorePageState()`.

## Icons

Lucide names only. Nav-level icons are stable; don't rename on a whim (users build muscle memory). Current nav: `home`, `credit-card`, `receipt`, `folder`, `link-2` (account links), `users`, `key`, `bot` (MCP), `list-filter` (rules), `inbox` (reviews), `flag` (reports), `scroll` (sync logs), `settings`.

## Validation / PR evidence

UI changes must be validated in a real browser before the task is reported done, and the PR must include a screenshot. Use the `validate-ui` skill — it drives Chrome DevTools MCP end-to-end. **Never** fall back to `screencapture` / AppleScript.

**Default flow**:

1. `list_pages` → `select_page`/`new_page` at the target URL.
2. `wait_for` on expected text so the capture doesn't race render.
3. `resize_page` to a controlled breakpoint before every capture:
   - Desktop: `1280x800` (default) or `1440x900` for wide layouts.
   - Mobile: `390x844`. Required whenever the change is responsive or touches mobile-specific styles.
   - Tablet: `768x1024` when the change crosses the `md` breakpoint.
4. `take_screenshot(filePath, format: "jpeg", quality: 85, fullPage: false)` — viewport-only by default. Use `fullPage: true` only when the change is genuinely below the fold and scrolling wouldn't convey it (wrap those in `<details>` in the PR body).
5. Upload via the `github-image-hosting` skill (img402).

**Embedding in the PR** — always inline HTML so the rendered width is bounded:

```html
<img src="https://i.img402.dev/<id>.jpg" width="800" alt="<page> — after">
```

Before/after diffs: use the side-by-side table pattern documented in the `validate-ui` skill. Responsive changes: include both desktop and mobile.

Do NOT use `![alt](url)` — GitHub renders the full native size and tall captures become painful to review. `{width=...}` kramdown syntax and `style="..."` attributes are silently stripped by GitHub's sanitizer.

**Picking up your edits before capture**:
- If the server is running under `make dev-watch`, template/CSS/Alpine edits are already live — just reload the browser.
- If it's running under `make dev`, restart it first — the binary serves embedded CSS and reloading the browser alone won't pick up the change (see "CSS build" above).
