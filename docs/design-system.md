# Design System Specification

## 1. Overview

The Breadbox admin dashboard uses **DaisyUI 5 + Tailwind CSS v4** with **Alpine.js v3** for interactivity. DaisyUI provides semantic component classes (drawer, table, badge, menu, modal) with built-in dark mode.

**Constraints:**
- No Node.js, no npm, no bundler — uses `tailwindcss-extra` standalone binary
- Single `make css` build step (like `sqlc generate` or `go generate`)
- Must work on mobile browsers
- Single developer maintaining it

## 2. Build Setup

### Binary

Use [`tailwindcss-extra`](https://github.com/dobicinaitis/tailwind-cli-extra) — a standalone binary bundling Tailwind CSS v4 + DaisyUI v5. No Node.js required.

**Install:**
```bash
# macOS ARM64 (Apple Silicon)
curl -sLo tailwindcss-extra https://github.com/dobicinaitis/tailwind-cli-extra/releases/latest/download/tailwindcss-extra-macos-arm64
chmod +x tailwindcss-extra

# Linux x64 (Docker/CI)
curl -sLo tailwindcss-extra https://github.com/dobicinaitis/tailwind-cli-extra/releases/latest/download/tailwindcss-extra-linux-x64
chmod +x tailwindcss-extra
```

Platform binaries available: macOS x64/ARM64, Linux x64/ARM64 (glibc + musl), Windows x64.

Also available via Homebrew: `brew tap dobicinaitis/tailwind-cli-extra && brew install tailwindcss-extra`

### Input CSS

`input.css` at project root:

```css
@import "tailwindcss";
@plugin "daisyui" {
  themes: false;
}
@plugin "daisyui/theme" { name: "breadbox-light"; default: true; ... }
@plugin "daisyui/theme" { name: "breadbox-dark"; prefersdark: true; ... }

/* App-specific component classes */
@layer components {
  .bb-filter-bar { ... }    /* Filter form layout */
  .bb-card { ... }           /* Border-based card (not shadow) */
  .bb-amount { ... }         /* Tabular-nums right-aligned amounts */
  .bb-info-grid { ... }      /* Detail page key-value grids */
  .bb-paginator { ... }      /* Numbered pagination with page range */
  .bb-page-header { ... }    /* Page title + action row */
  /* ... see input.css for full list */
}
```

### Makefile Targets

```makefile
TAILWIND_BIN := ./tailwindcss-extra

.PHONY: css css-watch css-install

css-install:
	@if [ ! -f $(TAILWIND_BIN) ]; then \
		echo "Downloading tailwindcss-extra..."; \
		curl -sLo $(TAILWIND_BIN) https://github.com/dobicinaitis/tailwind-cli-extra/releases/latest/download/tailwindcss-extra-$$(uname -s | tr '[:upper:]' '[:lower:]')-$$(uname -m | sed 's/x86_64/x64/' | sed 's/aarch64/arm64/'); \
		chmod +x $(TAILWIND_BIN); \
	fi

css: css-install
	$(TAILWIND_BIN) -i input.css -o static/css/styles.css --minify

css-watch: css-install
	$(TAILWIND_BIN) -i input.css -o static/css/styles.css --watch
```

### Output

Generated CSS goes to `static/css/styles.css`. This file should be committed to the repo (avoids needing the binary in CI for simple deploys) OR added to `.gitignore` with `make css` in the Dockerfile build stage.

**Recommended:** Commit the generated CSS. It's small (DaisyUI base is ~34KB gzipped) and avoids CI complexity.

### Dockerfile Integration

```dockerfile
# In the build stage, after Go binary is compiled:
COPY input.css .
RUN curl -sLo tailwindcss-extra https://github.com/dobicinaitis/tailwind-cli-extra/releases/latest/download/tailwindcss-extra-linux-x64 \
    && chmod +x tailwindcss-extra \
    && ./tailwindcss-extra -i input.css -o static/css/styles.css --minify
```

### Static File Serving

The Go server already serves `static/` via `http.FileServer`. The generated CSS is available at `/static/css/styles.css`.

## 3. Theme

### Theme Pairing

- **Light:** `breadbox-light` — custom shadcn/ui neutral palette, set as default
- **Dark:** `breadbox-dark` — custom dark palette, auto-switches via `prefers-color-scheme`

Auto-switches based on `prefers-color-scheme`. Users can also force a theme with `data-theme="light"` or `data-theme="dark"` on `<html>`.

### Custom Properties Retained

Some `--bb-*` variables are kept for app-specific tokens not covered by DaisyUI themes:

```css
:root {
  --bb-gap-xs: 0.25rem;
  --bb-gap-sm: 0.5rem;
  --bb-gap-md: 1rem;
  --bb-gap-lg: 1.5rem;
  --bb-gap-xl: 2rem;
}
```

DaisyUI semantic colors (`success`, `error`, `warning`, `info`, `primary`, `secondary`) handle light/dark variants automatically.

## 4. Component Conventions

### Buttons

Two sizes only: `btn-sm` (default) and `btn-xs` (compact contexts like table cells).

| Context | Classes |
|---|---|
| Primary action | `btn btn-primary btn-sm rounded-xl` |
| Secondary/ghost | `btn btn-ghost btn-sm rounded-xl` |
| Destructive | `btn btn-error btn-sm rounded-xl` (add `btn-soft` for softer look) |
| Compact inline (table rows) | `btn btn-ghost btn-xs rounded-lg` |
| Icon-only (standard) | `btn btn-ghost btn-sm btn-square rounded-xl` |
| Icon-only (compact) | `btn btn-ghost btn-xs btn-square rounded-lg` |
| Outline | `btn btn-outline btn-sm rounded-xl` |

**Rounding:** `btn-sm` → `rounded-xl`, `btn-xs` → `rounded-lg`.

**Icon + text gap:** `btn-sm` → `gap-2`, `btn-xs` → `gap-1.5`.

**Full-width / oversized buttons** (login CTA, modal actions): May omit `btn-sm` and use custom sizing — these are intentional exceptions.

### Badges

| Context | Classes |
|---|---|
| Status (connection, sync, review) | `badge badge-soft badge-{color} badge-sm` |
| Metadata labels (scope, source, type) | `badge badge-ghost badge-xs` |
| Counts / numbers | `badge badge-{color} badge-xs` |

**No extra rounding** — DaisyUI badges have their own radius. Never add `rounded-lg` or `rounded-xl` to badges.

**`badge-soft`** is used for all semantic status badges (success/error/warning/info). Plain `badge badge-{color}` is for counts and indicators.

**Template functions** (`statusBadge()`, `syncBadge()`, `configSource()`) emit standardized badge HTML:
```go
statusBadge("active")  → <span class="badge badge-soft badge-success badge-sm">Active</span>
syncBadge("error")     → <span class="badge badge-soft badge-error badge-sm">error</span>
configSource(src, key) → <span class="badge badge-ghost badge-sm">from env</span>
```

### Cards (`bb-card`)

Base class: `bb-card` — provides `bg-base-100 rounded-xl border border-base-300` with smooth transitions.

| Pattern | Classes | Internal padding |
|---|---|---|
| Simple content | `bb-card p-5` | Direct content, no sections |
| Compact (stat cards) | `bb-card p-4` | Metrics, small info blocks |
| Forms (centered) | `bb-card p-6` or `bb-card p-8` | Auth forms, setup wizards |
| Empty states | `bb-card p-12 text-center` | Large centered messages |
| Sectioned (header + body) | `bb-card p-0 overflow-hidden` | Children: `px-4 sm:px-5 py-3` (header), `px-4 sm:px-5 py-4` (body) |
| Table container | `bb-card p-0 overflow-hidden` | Table handles its own padding |
| Collapsible panels | `bb-card p-0 overflow-hidden` | Toggle header + body sections |

**Sectioned cards** use `border-t border-base-300/50` for dividers between sections. Always add `overflow-hidden` when using `p-0` to prevent rounded border clipping.

**Interactive cards** add `bb-card--interactive` for hover effects (cursor change, background shift).

### Modals

All `<dialog>` elements use:
- Container: `<dialog id="..." class="modal modal-bottom sm:modal-middle">`
- Content: `<div class="modal-box rounded-xl max-w-lg">` (use `<form>` wrapper only when the modal IS a form)
- Rounding: always `rounded-xl`
- Close backdrop: `<form method="dialog" class="modal-backdrop"><button>close</button></form>`

Custom overlay dialogs (confirm, shortcuts, category picker) in `base.html` use their own CSS classes and are not standard modals.

### Form Controls

| Element | Standard classes |
|---|---|
| Text inputs | `input input-bordered w-full rounded-xl` |
| Filter inputs | `input input-sm input-bordered w-full` (styled by `.bb-filter-bar` CSS) |
| Compact inputs (rules) | `input input-bordered input-xs rounded-lg` |
| Selects | `select select-bordered w-full rounded-xl` |
| Filter selects | `select select-sm select-bordered w-full` |
| Textareas | `textarea textarea-bordered rounded-xl w-full` |

**Background:** No `bg-base-200/50` on standard form inputs — only on read-only, disabled, or inline-edit inputs.

**Labels** use three patterns depending on context:
- Filter bars: `.bb-filter-label` (defined in `input.css`)
- Form fields: DaisyUI `<label class="label">` with `<span class="label-text">`
- Simple forms: `<label class="text-sm font-medium text-base-content/70 mb-1.5 block">`

### Icon Sizes

| Context | Size |
|---|---|
| Inline with text (badges, labels) | `w-3.5 h-3.5` |
| In buttons (`btn-sm`) | `w-4 h-4` |
| In buttons (`btn-xs`) | `w-3.5 h-3.5` |
| Section headers / standalone | `w-5 h-5` |
| Empty state illustrations | `w-8 h-8` |
| Sidebar nav | Managed by CSS (`.bb-sidebar-link` rules) — don't set manually |

### Transitions (Alpine.js)

| Context | Approach |
|---|---|
| Collapsible sections (filter panels, accordions, disclosures) | `x-collapse` |
| Tab panels, wizard steps | Explicit `x-transition:enter` with `ease-out duration-200` |
| Dropdowns / popovers | Explicit transitions with scale + opacity |
| Modals | Handled by DaisyUI — no Alpine transitions needed |
| Toast / notifications | Already handled in `base.html` |
| Simple show/hide (spinners, help text) | No transition — instant is fine |

### Empty States

Standard pattern for "no data" screens:
```html
<div class="bb-card p-12 text-center">
  <div class="flex flex-col items-center">
    <div class="w-14 h-14 rounded-xl bg-base-200 flex items-center justify-center mb-4">
      <i data-lucide="..." class="w-7 h-7 text-base-content/30"></i>
    </div>
    <h3 class="text-base font-semibold mb-1">Title</h3>
    <p class="text-base-content/50 text-sm mb-5 max-w-sm">Description text.</p>
    <a href="..." class="btn btn-primary btn-sm rounded-xl gap-2">
      <i data-lucide="plus" class="w-4 h-4"></i> CTA Button
    </a>
  </div>
</div>
```

For filtered "no results" states (e.g., transactions with active filters), a compact version with just icon + text is acceptable.

## 5. Layout Patterns

### Base Layout (Drawer Sidebar)

```html
<div class="drawer lg:drawer-open">
  <!-- Toggle (hidden checkbox) -->
  <input id="bb-drawer" type="checkbox" class="drawer-toggle" />

  <!-- Main content -->
  <div class="drawer-content">
    <!-- Mobile navbar (hidden on lg+) -->
    <div class="navbar bg-base-100 lg:hidden border-b border-base-300">
      <div class="flex-none">
        <label for="bb-drawer" class="btn btn-square btn-ghost">
          <i data-lucide="menu" class="w-5 h-5"></i>
        </label>
      </div>
      <div class="flex-1">
        <span class="text-lg font-bold">Breadbox</span>
      </div>
    </div>
    <!-- Page content -->
    <main class="p-6 max-w-5xl">
      {{block "content" .}}{{end}}
    </main>
  </div>

  <!-- Sidebar -->
  <div class="drawer-side">
    <label for="bb-drawer" aria-label="close sidebar" class="drawer-overlay"></label>
    <aside class="bg-base-200 min-h-full w-60 p-4 flex flex-col">
      <!-- Brand -->
      <a href="/admin/" class="text-xl font-bold px-4 py-2 mb-4">
        <i data-lucide="package" class="w-5 h-5 inline-block mr-1"></i>
        Breadbox
      </a>
      <!-- Nav -->
      <ul class="menu menu-md flex-1">
        <li class="menu-title">Data</li>
        <li><a href="/admin/"><i data-lucide="layout-dashboard"></i> Dashboard</a></li>
        <li><a href="/admin/connections"><i data-lucide="link"></i> Connections</a></li>
        <li><a href="/admin/transactions"><i data-lucide="receipt"></i> Transactions</a></li>
        <li><a href="/admin/users"><i data-lucide="users"></i> Family Members</a></li>
        <li class="menu-title">System</li>
        <li><a href="/admin/api-keys"><i data-lucide="key"></i> API Keys</a></li>
        <li><a href="/admin/sync-logs"><i data-lucide="refresh-cw"></i> Sync Logs</a></li>
        <li><a href="/admin/settings"><i data-lucide="settings"></i> Settings</a></li>
      </ul>
      <!-- Sign out -->
      <form method="POST" action="/admin/logout" class="mt-auto px-2">
        <button type="submit" class="btn btn-ghost btn-sm w-full justify-start">
          <i data-lucide="log-out"></i> Sign Out
        </button>
      </form>
    </aside>
  </div>
</div>
```

### Wizard Layout

```html
<div class="min-h-screen flex items-center justify-center p-4 bg-base-200">
  <div class="card bg-base-100 shadow-xl w-full max-w-md">
    <div class="card-body">
      <!-- Progress steps -->
      <ul class="steps steps-horizontal w-full mb-6">
        <li class="step step-primary">Account</li>
        <li class="step step-primary">Providers</li>
        <li class="step">Sync</li>
        <li class="step">Webhooks</li>
        <li class="step">Done</li>
      </ul>
      <!-- Step content -->
      {{block "content" .}}{{end}}
    </div>
  </div>
</div>
```

## 6. Table Guidelines

### Base Table

All data tables use:
```html
<div class="overflow-x-auto">
  <table class="table table-zebra table-sm">
    <thead>
      <tr>
        <th>Column</th>
        ...
      </tr>
    </thead>
    <tbody>
      <tr class="hover:bg-base-200">
        <td>Value</td>
        ...
      </tr>
    </tbody>
  </table>
</div>
```

Note: DaisyUI 5 removed the built-in `hover` table class. Use Tailwind `hover:bg-base-200` on `<tr>` elements.

### Size Variants

- `table-sm`: Default for most pages (connections, API keys, sync logs, family members)
- `table-md`: Transaction list (more data per row)
- `table-xs`: Embedded tables (accounts within connection detail)

### Sticky Headers

For tables that can grow long (transactions, sync logs):
```html
<div class="overflow-x-auto max-h-[70vh]">
  <table class="table table-zebra table-sm table-pin-rows">
```

### Amount Columns

```html
<td class="bb-amount">{{formatAmount .Amount}}</td>
```

Transaction rows use `bb-tx-amount` and `bb-tx-amount--income` for styled amounts.

### Status Badges in Tables

```html
<td>{{statusBadge .Status}}</td>
<!-- Renders: <span class="badge badge-soft badge-success badge-sm">Active</span> -->
```

Status badges use `badge-soft badge-sm`. See Component Conventions above for the full badge pattern.

## 7. Form Patterns

### Filter Bar

Used on transactions, sync logs, account detail:
```html
<div class="bb-filter-bar">
  <label>
    Start Date
    <input type="date" name="start_date" class="input input-sm input-bordered" />
  </label>
  <label>
    Status
    <select name="status" class="select select-sm select-bordered">
      <option value="">All</option>
      ...
    </select>
  </label>
  <button type="submit" class="btn btn-sm btn-primary self-end">Filter</button>
</div>
```

### Inline Edit

For connection detail (display name, sync interval):
```html
<div class="flex items-center gap-2" x-data="{ saving: false }">
  <input type="text" class="input input-sm input-bordered w-48" ... />
  <span x-show="saving" class="loading loading-spinner loading-xs"></span>
</div>
```

### Form Validation

Use DaisyUI form control patterns:
```html
<label class="label">
  <span class="label-text">Password</span>
</label>
<input type="password" class="input input-bordered" required />
<!-- Error state -->
<input type="password" class="input input-bordered input-error" />
<label class="label">
  <span class="label-text-alt text-error">Password must be at least 8 characters</span>
</label>
```

## 8. Icon System

### Lucide Icons

**CDN:** `https://cdn.jsdelivr.net/npm/lucide@latest/dist/umd/lucide.min.js`

Pin to a specific version in production for stability.

**Usage:**
```html
<i data-lucide="icon-name" class="w-4 h-4"></i>

<!-- At end of body -->
<script src="https://cdn.jsdelivr.net/npm/lucide@latest/dist/umd/lucide.min.js"></script>
<script>lucide.createIcons();</script>
```

After `createIcons()`, each `<i data-lucide="...">` is replaced with an inline `<svg>`.

**Size control:** See the Icon Sizes table in Component Conventions (§4) for the canonical convention. Quick reference:
- In `btn-sm`: `w-4 h-4`
- In `btn-xs` / inline: `w-3.5 h-3.5`
- Section headers: `w-5 h-5`
- Empty states: `w-8 h-8`

### Icon Inventory

| Context | Icon | Lucide Name |
|---|---|---|
| Dashboard nav | Layout grid | `layout-dashboard` |
| Connections nav | Chain link | `link` |
| Transactions nav | Receipt | `receipt` |
| Family Members nav | Group | `users` |
| API Keys nav | Key | `key` |
| Sync Logs nav | Refresh | `refresh-cw` |
| Settings nav | Gear | `settings` |
| Sign Out | Door arrow | `log-out` |
| Mobile menu toggle | Hamburger | `menu` |
| Brand icon | Package | `package` |
| Stat: Accounts | Building | `building-2` |
| Stat: Transactions | Trending | `trending-up` |
| Stat: Last Sync | Clock | `clock` |
| Stat: Needs Attention | Alert | `alert-triangle` |
| Empty state | Inbox | `inbox` |
| Add / Create | Plus | `plus` |
| Edit | Pencil | `pencil` |
| Delete / Remove | Trash | `trash-2` |
| Sync | Refresh | `refresh-cw` |
| External link | Arrow up-right | `external-link` |
| Copy | Clipboard | `clipboard` |
| Success toast | Check circle | `check-circle` |
| Error toast | X circle | `x-circle` |
| Info toast | Info | `info` |

### Alpine.js Re-initialization

When Alpine.js dynamically adds elements with `data-lucide` attributes (e.g., toast messages), call `lucide.createIcons()` after DOM insertion:

```html
<div x-init="$watch('show', v => { if(v) $nextTick(() => lucide.createIcons()) })">
```

## 9. Toast / Notification Pattern

Global toast in `base.html` — centered floating pill at the bottom of the viewport with type-specific Lucide icons (check, x-circle, alert-triangle, info). Auto-dismisses after 3 seconds with fade+slide transition.

**Dispatch from anywhere:**
```js
window.dispatchEvent(new CustomEvent('bb-toast', {
  detail: { message: 'Sync triggered', type: 'success' }
}));
```

Supported types: `success` (default), `error`, `warning`, `info`.

**Inline feedback:** For trivial instant actions (copy-to-clipboard), use the icon swap pattern instead of a toast: swap the icon to a checkmark with `text-success` for 2 seconds. See `agent_wizard.html` for the reference implementation.

## 10. Typography

DaisyUI inherits Tailwind's type scale. Key decisions:

- **Body text:** Default (`text-base`, 16px) — no override needed
- **Page titles:** `text-2xl font-bold` (or `text-xl` on mobile)
- **Section headers:** `text-lg font-semibold`
- **Stat values:** `stat-value` class (DaisyUI handles sizing)
- **Amounts:** `tabular-nums` via `.bb-amount` class (fixed-width digits for alignment)
- **Small text:** `text-sm` for labels, metadata, timestamps
- **Monospace:** `font-mono` for API keys, transaction IDs, code snippets

## 11. Async Button Loading

Use `bbButtonLoading(btn)` / `bbButtonDone(btn)` for async button feedback:

```js
// Before fetch
bbButtonLoading(btn);  // saves content, shows spinner, disables

fetch('/api/...').then(() => {
  bbButtonDone(btn);   // restores content + re-initializes Lucide icons
}).catch(() => {
  bbButtonDone(btn);
});
```

## 12. Alerts

All alerts use: `<div role="alert" class="alert alert-{type} rounded-xl mb-6">`.

Flash messages (`partials/flash.html`) and inline alerts follow the same pattern. Use `alert-soft` for less prominent inline warnings.
