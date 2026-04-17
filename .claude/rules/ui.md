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
- **CSS is embedded into the binary** via `static/embed.go` (`//go:embed all:css favicon.svg`). After `make css` you must **restart the server** for changes to take effect — a browser hard-reload alone won't help because the running binary still serves the stale embedded copy.

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

**Restart `make dev`** after template / CSS / Alpine edits before capturing — the binary serves embedded CSS and reloading the browser alone won't pick up the change (see "CSS build" above).
