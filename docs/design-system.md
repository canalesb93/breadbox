# Design System Specification

## 1. Overview

Breadbox migrates from Pico CSS (classless) to **DaisyUI 5 + Tailwind CSS v4** for the admin dashboard. This provides dashboard-native components (drawer, stat, table, badge, menu, modal, toast) with built-in dark mode and responsive behavior.

**Why the switch:**
- Pico CSS is a content/reading framework — its classless approach produces a "documentation site" aesthetic, not a dashboard
- DaisyUI provides semantic component classes (`drawer`, `stat`, `table`, `badge`, `menu`) purpose-built for application UIs
- Built-in dark mode with theme pairing (light + dark auto-switch)
- No JavaScript included — pure CSS, works alongside Alpine.js

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
  themes: light --default, dark --prefersdark;
}

/* App-specific component classes */
@layer components {
  .bb-filter-bar {
    @apply flex flex-wrap items-end gap-4 mb-6;
  }

  .bb-filter-bar label {
    @apply flex flex-col gap-1 text-sm;
  }

  .bb-filter-bar select,
  .bb-filter-bar input {
    @apply select select-sm select-bordered w-auto min-w-[8rem];
  }

  .bb-pagination {
    @apply flex items-center justify-between mt-4;
  }

  .bb-action-bar {
    @apply flex items-center gap-2 mb-4;
  }

  .bb-amount {
    @apply text-right tabular-nums;
  }

  .bb-amount--debit {
    @apply text-right tabular-nums;
  }

  .bb-amount--credit {
    @apply text-right tabular-nums text-success;
  }

  .bb-info-grid {
    @apply grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm;
  }

  .bb-info-grid dt {
    @apply font-medium text-base-content/70;
  }

  .bb-info-grid dd {
    @apply text-base-content;
  }
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

- **Light:** `light` — DaisyUI default light theme, clean and neutral
- **Dark:** `dark` — DaisyUI default dark theme with good contrast

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

These may be gradually replaced by Tailwind utility classes (`gap-1`, `gap-2`, etc.) as templates are migrated.

### Color Mapping

| Old `--bb-color-*` | DaisyUI Equivalent |
|---|---|
| `--bb-color-success` (#2e7d32) | `success` theme color (used as `badge-success`, `alert-success`, `text-success`) |
| `--bb-color-error` (#c62828) | `error` theme color |
| `--bb-color-warning` (#e65100) | `warning` theme color |
| `--bb-color-cyan` (#00838f) | `info` theme color |

DaisyUI handles light/dark variants automatically — no manual `@media (prefers-color-scheme: dark)` overrides needed.

## 4. Component Mapping

### Current `bb-*` → DaisyUI

| Current Class | DaisyUI Replacement | Notes |
|---|---|---|
| `.bb-layout` | `drawer lg:drawer-open` | Wraps entire page layout |
| `.bb-sidebar` | `drawer-side` + `menu bg-base-200` | Sidebar container + nav list |
| `.bb-nav` | `menu` | `<ul class="menu">` with `<li><a>` items |
| `.bb-nav a[aria-current]` | `menu-active` or Tailwind `bg-primary text-primary-content` | Active nav item |
| `.bb-main` | `drawer-content` | Main content area |
| `.bb-stats` | `stats stats-vertical lg:stats-horizontal` | Stat card container |
| `.bb-stat` | `stat` | Individual stat card |
| `.bb-stat-label` | `stat-title` | Stat label text |
| `.bb-stat-value` | `stat-value` | Stat large number |
| `.bb-stat--warn` | `stat` + custom border class | Add `border-l-4 border-warning` |
| `.bb-badge` | `badge` | Base badge |
| `.bb-badge--success` | `badge badge-success` | Green badge |
| `.bb-badge--error` | `badge badge-error` | Red badge |
| `.bb-badge--warning` | `badge badge-warning` | Yellow badge |
| `.bb-badge--muted` | `badge badge-ghost` | Gray/muted badge |
| `.bb-page-header` | Tailwind flex: `flex items-center justify-between mb-6` | Page title + action button row |
| `.bb-empty` | `card bg-base-200` + centered content | Or `alert` with icon |
| `[data-flash="success"]` | `alert alert-success` | Flash messages |
| `[data-flash="error"]` | `alert alert-error` | Flash messages |
| `[data-flash="info"]` | `alert alert-info` | Flash messages |
| `<article>` (Pico card) | `card bg-base-100 shadow-sm` + `card-body` | Settings sections, detail panels |

### Template Helper Updates

The Go template functions `statusBadge()` and `syncBadge()` in `internal/admin/templates.go` need to emit DaisyUI classes:

```go
// Before
"active":  `<span class="bb-badge bb-badge--success">Active</span>`
// After
"active":  `<span class="badge badge-success">Active</span>`
```

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
<!-- Negative amounts (credits) get color -->
<td class="bb-amount--credit">-$42.50</td>
```

### Status Badges in Tables

```html
<td>{{statusBadge .Status}}</td>
<!-- Renders: <span class="badge badge-success badge-sm">Active</span> -->
```

Badges in tables should use `badge-sm` for compact density.

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

**Size control:** Use Tailwind width/height classes on the `<i>` element:
- Sidebar nav: `w-4 h-4`
- Stat card icons: `w-8 h-8`
- Button icons: `w-4 h-4`
- Empty state icons: `w-12 h-12`

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

Global toast component in `base.html`, using DaisyUI `toast` + `alert`:

```html
<div class="toast toast-end toast-bottom z-50"
     x-data="{ toasts: [] }"
     @bb-toast.window="
       toasts.push({ message: $event.detail.message, type: $event.detail.type || 'info', id: Date.now() });
       setTimeout(() => toasts.shift(), 4000)
     ">
  <template x-for="t in toasts" :key="t.id">
    <div class="alert shadow-lg"
         :class="{
           'alert-success': t.type === 'success',
           'alert-error': t.type === 'error',
           'alert-warning': t.type === 'warning',
           'alert-info': t.type === 'info'
         }">
      <span x-text="t.message"></span>
    </div>
  </template>
</div>
```

**Dispatch from anywhere:**
```js
window.dispatchEvent(new CustomEvent('bb-toast', {
  detail: { message: 'Sync triggered', type: 'success' }
}));
```

## 10. Typography

DaisyUI inherits Tailwind's type scale. Key decisions:

- **Body text:** Default (`text-base`, 16px) — no override needed
- **Page titles:** `text-2xl font-bold` (or `text-xl` on mobile)
- **Section headers:** `text-lg font-semibold`
- **Stat values:** `stat-value` class (DaisyUI handles sizing)
- **Amounts:** `tabular-nums` via `.bb-amount` class (fixed-width digits for alignment)
- **Small text:** `text-sm` for labels, metadata, timestamps
- **Monospace:** `font-mono` for API keys, transaction IDs, code snippets

## 11. Migration Notes

### What to Remove

- Pico CSS CDN link (`@picocss/pico@2/css/pico.min.css`)
- All `--bb-color-*` CSS custom properties (replaced by DaisyUI theme colors)
- All `bb-badge`, `bb-stat`, `bb-layout`, `bb-sidebar`, `bb-main`, `bb-nav`, `bb-page-header`, `bb-empty` CSS class definitions
- Flash message `[data-flash]` CSS (replaced by DaisyUI `alert`)
- The `@media (max-width: 768px)` responsive block (DaisyUI drawer handles this)
- All `data-theme="light"` hardcoding (if any remains)

### What to Keep

- Alpine.js v3 CDN script
- `[x-cloak]` CSS rule
- `--bb-gap-*` spacing tokens (until fully replaced by Tailwind utilities)
- Go template functions (`statusBadge`, `syncBadge`, `errorMessage`) — update their HTML output to use DaisyUI classes
- `relativeTime`, `formatUUID`, `formatNumeric` template functions — unchanged

### Migration Order

1. Set up build tooling (16A.1)
2. Swap CSS framework in layouts (16A.2)
3. Migrate base layout to drawer (16A.3) — this is the biggest single change
4. Add icons (16A.4)
5. Extract component classes (16A.5)
6. Unify toast (16A.6)
7. Standardize tables (16A.7)
8. Brand identity (16A.8)
9. Then page-by-page redesigns (16B.1-16B.8)

### Template Conversion Pattern

For each page template:
1. Replace inline `style` attributes with Tailwind utility classes or `bb-*` component classes
2. Replace `<article>` (Pico card) with `<div class="card bg-base-100 shadow-sm"><div class="card-body">`
3. Replace custom badge HTML with `{{statusBadge .Status}}` (which now emits DaisyUI classes)
4. Replace `<div style="overflow-x: auto;">` + `<table>` with `<div class="overflow-x-auto"><table class="table table-zebra table-sm">`
5. Replace flash message divs with `<div class="alert alert-success">` etc.
6. Add Lucide icon `<i data-lucide="...">` elements where appropriate
