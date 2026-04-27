# Design System Specification

## 1. Overview

The Breadbox admin dashboard uses **DaisyUI 5 + Tailwind CSS v4** with **Alpine.js v3** for interactivity. DaisyUI provides semantic component classes (drawer, table, badge, menu, modal) with built-in dark mode.

**Component authoring:** new UI pieces are written as [`a-h/templ`](https://templ.guide) components in `internal/templates/components/*.templ` — this is the target pattern going forward (issue #462). Existing `html/template` pages and partials continue to work during the migration and can call templ components via the `renderComponent` funcMap bridge. See §13 below for the workflow.

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
  .bb-page-header { ... }    /* Page title + primary action (see §5 Page Header) */
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

**Danger-zone cards** add `bb-danger-card` (soft error tint: `border-error/20 bg-error/[0.02]`) — use for destructive-action cards placed below the main form card.

### Icon Tiles (`bb-icon-tile--*`)

Colored 40×40 rounded squares used inside card headers to ground the page's purpose. Pair `.bb-icon-header__tile` (geometry) with a color modifier; the icon inside inherits `currentColor`, so don't set a text color on it.

| Modifier | Background | Text |
|---|---|---|
| `.bb-icon-tile--primary` | `bg-primary/8` | `text-primary` |
| `.bb-icon-tile--success` | `bg-success/10` | `text-success` |
| `.bb-icon-tile--warning` | `bg-warning/10` | `text-warning` |
| `.bb-icon-tile--error` | `bg-error/10` | `text-error` |

```html
<div class="bb-icon-header__tile bb-icon-tile--primary">
  <i data-lucide="user-plus" class="w-5 h-5"></i>
</div>
```

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

### Timeline

GitHub-style activity timelines (continuous left rail threading through 24px icon tiles, day separators, sentence rows, comment bubbles) are assembled from generic templ primitives in `internal/templates/components/timeline.templ`. Use them whenever a surface needs to show a chronological feed — transactions, account links, reviews, sync runs — regardless of the underlying data shape.

**Primitives**

| Component | Purpose |
| --- | --- |
| `Timeline(TimelineProps)` | Section wrapper: optional heading + event count, ordered list, the absolutely-positioned rail. Body via `{ children... }`. |
| `TimelineDay(label, first)` | Horizontal day separator with a small dot anchored on the rail. Pass `first=true` for the topmost separator to tighten the leading gap. |
| `TimelineSystemRow(TimelineRowProps)` | Compact one-liner row: 24px icon tile (driven by `IconTone`) + sentence body via `{ children... }` + optional `Origin` and relative `Timestamp`. |
| `TimelineSystemRowCustomTile(timestamp, origin, tile, body)` | Same row chrome but the caller supplies the entire 24px tile as a templ component. Use when the icon tile is data-driven (per-category colour, multi-shade status badge). |
| `TimelineCommentRow(actor, timestamp)` | Chat-bubble row with the default "`Name` commented · `time`" meta line; bubble body via `{ children... }`. |
| `TimelineCommentRowRaw(avatar)` | Escape hatch for callers that need a custom avatar or to inject controls (e.g. an inline delete button) inside the meta line. Both the avatar and the right-hand body are component slots. |
| `TimelineEmpty(message)` | Compact "no activity yet" treatment: clock glyph + muted message. Caller wraps any composer or CTA above it. |

**`IconTone` palette** — drives the small (24px) tile background + icon colour on `TimelineSystemRow`:

| Tone | Background | Icon colour | Typical use |
| --- | --- | --- | --- |
| `neutral` (default) | `bg-base-200` | `text-base-content/40` | comments, generic system events |
| `info` | `bg-info/15` | `text-info` | rule applications, automation hits |
| `success` | `bg-success/15` | `text-success` | additive events (tag added, review approved) |
| `warning` | `bg-warning/15` | `text-warning` | review skipped, soft warnings |
| `error` | `bg-error/15` | `text-error` | tag removed, review rejected, failures |
| `custom` | — | — | sentinel: caller renders the tile itself |

**Reference call site** — the transaction-detail timeline lives in `internal/templates/components/pages/transaction_detail.templ` and uses `TimelineSystemRowCustomTile` (because individual rows render category-specific colours and 10/15% alpha review tints) plus `TimelineCommentRowRaw` (because comment rows host an inline trash button on hover):

```go
@components.Timeline(components.TimelineProps{
    Heading: "Activity", HeadingIcon: "activity",
    EventCount: len(p.Activity),
    AriaLabel: "Transaction activity timeline",
}) {
    for di, day := range p.ActivityDays {
        @components.TimelineDay(day.Label, di == 0)
        for _, event := range day.Events {
            if event.Type == "comment" {
                @txdTimelineComment(event)        // wraps TimelineCommentRowRaw
            } else {
                @txdTimelineSystem(event)         // wraps TimelineSystemRowCustomTile
            }
        }
    }
    @txdTimelineComposer(p)                       // page-specific composer
}
```

The composer (textarea + submit) is intentionally **not** a primitive — comment-creation UX is too tied to its caller (Alpine bindings, max-length, "also flag for review" toggle, keyboard shortcuts). Keep composer markup in the page template and slot it into the timeline children list at the tail.

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

### Page Header (`.bb-page-header`)

Every top-level admin page opens with a `.bb-page-header` row: the `<h1 class="bb-page-title">` (plus optional subtitle) on the left, and an optional **primary action** on the right (Save, Create, Sync All, Reconcile, Add Member, etc.).

**Single-back-affordance rule.** The shared breadcrumb (`{{template "breadcrumb" .}}` / the `Breadcrumb` templ component, rendered at the top of the `content` block **above** `.bb-page-header`) is the **sole** back affordance on a page. Do **not** place a `← Back to <Parent>` link inside `.bb-page-header`, above the title, or anywhere else on the page — it duplicates the breadcrumb and on mobile the extra full-width link stretches the header awkwardly (see #579, #582, #605).

**What lives where:**

| Slot                              | Contents                                                                                      |
| --------------------------------- | --------------------------------------------------------------------------------------------- |
| Above `.bb-page-header`           | `{{template "breadcrumb" .}}` — the single back affordance.                                    |
| `.bb-page-header` left            | `<h1 class="bb-page-title">` + optional subtitle / summary counts.                             |
| `.bb-page-header` trailing slot   | **Primary action only** (Save, Create, Sync All, Reconcile, Add Member, etc.). No back links. |

**Separate patterns that are fine** (not governed by this rule):

- **Wizard / stepper step-back buttons** inside a multi-step flow (e.g. `connection_new.html`, `csv_import.html`) — these live inside the step card/footer, not in `.bb-page-header`.
- **Post-success CTAs** inside a success card (e.g. "Back to Users" after creating a login in `create_login.html`, `user_form.html`) — these live inside a card body, not in `.bb-page-header`.
- **404 / error pages** whose entire content is a "Back to Dashboard" card.

Pages that need to pass breadcrumbs should set `data["Breadcrumbs"] = []Breadcrumb{…}` in the handler (see `internal/admin/categories.go` for an example).

### Form Card Pattern

The canonical pattern for create/edit/settings forms. A sectioned card with a colored icon header on top, form fields in the middle, and a subtle action row (Cancel + Save/Create) at the bottom. Use `max-w-lg` for the container.

**Reference implementations:** `internal/templates/pages/create_login.html` (both create and manage modes) and `internal/templates/pages/user_form.html` (edit mode).

```html
<div class="bb-page-header">
  <div>
    <h1 class="bb-page-title">Create Login</h1>
    <p class="text-sm text-base-content/50 mt-1">Create a sign-in account for {{.User.Name}}</p>
  </div>
</div>

<div class="max-w-lg">
  <form @submit.prevent="submit()" class="bb-card p-0 overflow-hidden">
    <!-- Top section: icon header + fields -->
    <div class="p-5 sm:p-6">
      <div class="bb-icon-header">
        <div class="bb-icon-header__tile bb-icon-tile--primary">
          <i data-lucide="user-plus" class="w-5 h-5"></i>
        </div>
        <div class="bb-icon-header__text">
          <h2 class="text-sm font-semibold">New login account</h2>
          <p class="text-xs text-base-content/45">They'll receive a link to set their password</p>
        </div>
      </div>

      <div class="space-y-4">
        <div>
          <label class="block text-xs font-medium text-base-content/50 mb-1.5" for="email">Email</label>
          <input type="email" id="email" class="bb-form-input" required placeholder="e.g., alex@example.com">
        </div>
        <div>
          <label class="block text-xs font-medium text-base-content/50 mb-1.5" for="role">Role</label>
          <select id="role" class="bb-form-select">…</select>
        </div>
        <template x-if="error">
          <div role="alert" class="bb-form-error">
            <i data-lucide="alert-circle" class="w-4 h-4 shrink-0"></i>
            <span x-text="error"></span>
          </div>
        </template>
      </div>
    </div>

    <!-- Bottom action row -->
    <div class="bb-action-row">
      <a href="/users" class="btn btn-sm btn-ghost rounded-xl">Cancel</a>
      <button type="submit" class="btn btn-sm btn-primary rounded-xl gap-1.5 min-w-32" :disabled="submitting">
        <span x-show="!submitting" class="inline-flex items-center gap-1.5">
          <i data-lucide="save" class="w-3.5 h-3.5"></i>Save Changes
        </span>
        <span x-show="submitting" class="loading loading-spinner loading-xs"></span>
      </button>
    </div>
  </form>
</div>
```

**Conventions:**
- Top section uses `p-5 sm:p-6` (mobile/desktop). Do **not** use `p-8` — that was the old centered-card style.
- Field groups use `space-y-4`. Labels use `text-xs font-medium text-base-content/50 mb-1.5 block`.
- Primary submit button gets `min-w-32` so it doesn't jitter between its label and the loading spinner.
- Icons inside the primary button are `w-3.5 h-3.5` (not `w-4 h-4`) to match the compact `btn-sm` weight.
- **No back link inside `.bb-page-header`.** The breadcrumb above the header is the sole back affordance — see the [Page Header](#page-header-bb-page-header) section above. The Cancel button in the bottom action row is the only secondary navigation the form needs.
- For destructive operations, add a **separate** `bb-card bb-danger-card` below — never mix the save action with the delete action in one card.

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

### Dirty-State Form Tracking (Alpine)

For settings-style edit screens where Save is explicit (not auto-save on change), the canonical pattern is an Alpine-local `initialX` / `x` / computed `dirty` / `save()` model. The Save button stays disabled until something changes, and a transient "Saved" indicator confirms success for 2 seconds.

**Reference:** manage-login mode in `create_login.html`.

```html
<div class="bb-card p-0 overflow-hidden" x-data="{
  initialRole: '{{.LoginAccount.Role}}',
  role: '{{.LoginAccount.Role}}',
  saving: false,
  saved: false,
  error: '',
  get dirty() { return this.role !== this.initialRole; },
  save() {
    this.error = '';
    this.saving = true;
    var self = this;
    fetch('…', { method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({role: this.role}) })
      .then(function(r) {
        if (!r.ok) return r.json().then(function(d) { throw d; });
        self.initialRole = self.role;
        self.saving = false;
        self.saved = true;
        setTimeout(function() { self.saved = false; }, 2000);
      })
      .catch(function(e) {
        self.saving = false;
        self.error = (e && e.error && e.error.message) || 'Failed to save changes';
      });
  }
}">
  <!-- top section: fields bound with x-model -->
  <div class="bb-action-row">
    <span x-show="saved" x-transition.opacity class="text-xs text-success inline-flex items-center gap-1 mr-2">
      <i data-lucide="check" class="w-3.5 h-3.5"></i> Saved
    </span>
    <a href="/users" class="btn btn-sm btn-ghost rounded-xl">Cancel</a>
    <button @click="save()" :disabled="!dirty || saving" class="btn btn-sm btn-primary rounded-xl gap-1.5 min-w-32">
      <span x-show="!saving" class="inline-flex items-center gap-1.5"><i data-lucide="save" class="w-3.5 h-3.5"></i>Save Changes</span>
      <span x-show="saving" class="loading loading-spinner loading-xs"></span>
    </button>
  </div>
</div>
```

### Danger Zone Card

Destructive operations (delete, revoke, disconnect) go in a **separate** `bb-card bb-danger-card` placed below the main form card — never inside the primary Save row. Horizontal layout on desktop, stacked on mobile.

```html
<div class="bb-card bb-danger-card p-5 sm:p-6 mt-4">
  <div class="flex items-start justify-between gap-4 flex-col sm:flex-row sm:items-center">
    <div>
      <h3 class="text-sm font-semibold text-error/80">Delete login account</h3>
      <p class="text-xs text-base-content/50 mt-0.5">{{.Name}} will no longer be able to sign in. This cannot be undone.</p>
    </div>
    <button class="btn btn-error btn-soft btn-sm rounded-xl shrink-0" @click="…">
      <i data-lucide="user-x" class="w-4 h-4"></i>
      Delete Login
    </button>
  </div>
</div>
```

### Overflow Action Menu

When a card has ≥2 secondary actions (Edit, Manage, Transactions, Delete, …) prefer a single `ellipsis-vertical` dropdown over a cluster of icon buttons. Used on member cards in `/users`, also in `categories.html` and `rules.html`.

```html
<div class="dropdown dropdown-end">
  <div tabindex="0" role="button" class="btn btn-ghost btn-sm btn-square rounded-lg opacity-40 hover:opacity-100 transition-opacity">
    <i data-lucide="ellipsis-vertical" class="w-5 h-5"></i>
  </div>
  <ul tabindex="0" class="dropdown-content menu bg-base-100 rounded-xl shadow-lg border border-base-300 z-50 w-44 p-1">
    <li><a href="…"><i data-lucide="pencil" class="w-3.5 h-3.5"></i> Edit profile</a></li>
    <li><a href="…"><i data-lucide="settings" class="w-3.5 h-3.5"></i> Manage login</a></li>
  </ul>
</div>
```

Keep icons at `w-3.5 h-3.5` in dropdown rows to match the `btn-xs` baseline weight of menu items.

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

**Page-level alerts** use: `<div role="alert" class="alert alert-{type} rounded-xl mb-6">`.

Flash messages (`partials/flash.html`) and inline alerts follow the same pattern. Use `alert-soft` for less prominent inline warnings.

**Form-level errors** inside a `.bb-card` use the `.bb-form-error` inline variant — softer, tighter, and sits flush with the form fields rather than the page chrome:

```html
<div role="alert" class="bb-form-error">
  <i data-lucide="alert-circle" class="w-4 h-4 shrink-0"></i>
  <span x-text="error"></span>
</div>
```

Always pair with an `alert-circle` icon so the error reads at a glance. This is the canonical pattern for inline Alpine-driven form validation errors.

## 13. Templ Components

Admin UI is migrating from `html/template` to [`a-h/templ`](https://templ.guide) (issue #462). Both coexist during the migration — don't rewrite a whole page to templ unless the migration plan calls for it.

### Where components live

`internal/templates/components/*.templ` — one component per file, named in `PascalCase`. Each `.templ` file produces a generated `*_templ.go` sibling; commit both.

### Generating Go from `.templ`

```
templ generate      # regenerates all *_templ.go in the repo
```

`make generate` and `make dev-watch` invoke `templ generate` automatically (once the #462 infrastructure lands). If you edit a `.templ` file by hand, re-run generation before `go build` — the Go compiler only sees the generated files.

Install the CLI with `go install github.com/a-h/templ/cmd/templ@latest` if it's missing from `$PATH`.

### Dev-reload interaction

`make dev-watch` rebuilds the Go binary on `.templ` changes via **air** — the generated `*_templ.go` files are part of the Go build graph, so a save triggers a ~1–2s restart, not the HTML-from-disk fast path used for `.html` edits. `BREADBOX_DEV_RELOAD=1` does **not** re-read templ components from disk; they're compiled in.

If you're iterating rapidly on a templ component, expect a Go restart per save. Prefer `.html` for pure markup tweaks during the migration if restart latency matters.

### Calling templ components from `.html` pages

`.html` templates bridge to templ via the `renderComponent` funcMap helper. Register a component once in the bridge (see `internal/templates/components/bridge.go`) and call it from any `.html` block:

```html
{{renderComponent "StatusBadge" .Connection.Status}}
```

The helper invokes the component's `Render(ctx, w)` and returns `template.HTML` so the output isn't re-escaped. Pass simple Go values — the bridge signature is fixed per component.

### Adding a new component

1. Copy an existing component (e.g. `status_badge.templ`) — mirrors package, imports, and signature conventions.
2. Edit the markup and params, then run `templ generate`.
3. If the component will be called from a `.html` page, add an entry to the `renderComponent` bridge. Pure templ-to-templ calls don't need a bridge entry.
4. `go build ./...` to confirm the generated code compiles.

### `templ fmt`

Run `templ fmt .` before committing — it normalizes whitespace and attribute order in `.templ` files. CI may enforce this. Many editors have a templ LSP that formats on save.

## 14. Alpine page components

Page-level Alpine factories — the things rendered as `x-data="..."` at the root of an admin page — live in `static/js/admin/components/<pageSlug>.js` and load via a synchronous `<script src="...">` placed at the top of the templ component. This keeps JS out of templ files and out of `_scripts.go` Go-string sidecars, so editors give you syntax highlighting, the formatter works, and refactors are mechanical.

History: the migration from inline factories to static modules was tracked in #827 (foundation: #828; reference port: `prompt_builder`).

### File layout

```
static/js/admin/components/
  prompt_builder.js          # one factory per page
  ...
```

Each file is plain JS (no bundler, no transform). The whole `static/js/` tree is embedded into the binary by `static/embed.go` and served at `/static/*` by `internal/api/router.go`. Dev-watch picks edits up immediately when `BREADBOX_DEV_RELOAD=1` is set (see Section 2).

### Factory shape

Use Alpine's documented `Alpine.data()` form, registered inside an `alpine:init` listener:

```js
document.addEventListener('alpine:init', function () {
  Alpine.data('promptBuilder', function () {
    return {
      blocks: [],

      init: function () {
        var dataEl = document.getElementById('prompt-builder-data');
        if (dataEl) {
          this.blocks = JSON.parse(dataEl.textContent);
        }
      },

      // ...rest of the component (getters, methods, computed values)
    };
  });
});
```

Two stylistic constraints:

- **The factory takes no arguments.** Alpine officially supports `x-data="foo(arg)"`, but in this codebase the factory is always argument-free. Initial state flows in through `data-*` attributes or a sibling JSON `<script>`, never through Go-string interpolation. This keeps the JS file editor-friendly and forces a clean separation of concerns.
- **`init()` is where you parse data and wire one-time setup.** `destroy()` is for cleanup of global listeners (e.g. shortcut store registrations). Don't put data parsing inline in expressions — do it once.

### Templ wiring

In the `.templ` page:

```templ
templ MyPage(p MyPageProps) {
  <script src="/static/js/admin/components/my_page.js"></script>
  // ... markup ...
  @templ.JSONScript("my-page-data", p.Items)
  <div x-data="myPage" data-mode={ p.Mode }>
    // ... interactive markup using methods + state from the factory ...
  </div>
}
```

Three things to notice:

- The component `<script src="...">` is **at the top of the templ component** and **NOT marked `defer`**. Alpine itself loads via `<script defer>` in `<head>`, so its `alpine:init` event would fire before a deferred component script could register the factory. Loading synchronously here means the parser runs your `Alpine.data('myPage', ...)` registration BEFORE Alpine wakes up. The script is small and same-origin, so the synchronous fetch is a non-issue. (Alternatively a `<script defer>` placed in `<head>` would also work because of document-order defer execution; keeping it in the component is more co-located.)
- `x-data="myPage"` is a **string literal** — not `x-data={ "myPage(...)" }`. That Go-expression form is the regression #827 is undoing and is enforced against by the lint in `internal/templates/components/pages/scripts_lint_test.go`.
- `@templ.JSONScript("my-page-data", p.Items)` renders `<script id="my-page-data" type="application/json">[...]</script>` with proper escaping. The factory parses it once in `init()`. This is the documented templ helper for passing complex server data to client JS.

### Two data-passing patterns — pick one

| Use case | Pattern |
|---|---|
| Scalars, IDs, small flags | `data-foo="..."` on the `x-data` root, read via `this.$el.dataset.foo` in `init()` (or directly inside expressions). |
| Complex initial state — slices, maps, nested objects | `@templ.JSONScript("<page>-data", props.X)` + `JSON.parse(document.getElementById('<page>-data').textContent)` in `init()`. |

Don't mix: pick the one that fits the data shape. `data-*` for primitives, JSON script for structured payloads.

### What NOT to do

- **`x-data={ "myPage(" + p.JSON + ")" }`** — Go-expression `x-data` calling a factory with interpolated args. The lint rejects this. Move the data into a `data-*` attribute or `@templ.JSONScript`.
- **`@templ.Raw("<script>" + factoryBody + "</script>")`** — embedding a multi-line factory as a Go string. No syntax highlighting, no editor IntelliSense, fragile escaping. Move the body into `static/js/admin/components/`.
- **A new `<page>_scripts.go` next to `<page>.templ`** — the sidecar pattern was removed by #827. New code uses `static/js/admin/components/`.
- **A trivial one-liner inline `<script>` for `lucide.createIcons()`** — fine to leave as-is; the lint caps inline blocks at 5 content lines. Anything larger belongs in a JS file.

### Sharing logic across pages

Most pages have their own factory file. For factories that genuinely need to be shared (the canonical example is `categoryPicker`, used by category form, transactions filter bar, and per-row category assignment), put the module in `static/js/admin/components/<name>.js` and have each consumer load the script and pass differing state via `data-*` attributes. Prefer copying first and consolidating only when a third page wants the same logic — premature shared modules are hard to undo.

For shared CDN scripts (e.g. `marked`, `dompurify`) that need to load exactly once across multiple pages, the templ-idiomatic pattern is `templ.OnceHandle`. Use it when a page first needs it; until then, each page pulls in its own `<script src="...">` — duplicate loads are harmless on the admin tier.

### Lint

`internal/templates/components/pages/scripts_lint_test.go` runs as part of `go test ./...` and enforces two rules:

1. **No `x-data={ "factory("` Go-expression form.** Hard fail.
2. **No literal `<script>...</script>` block in `internal/templates/components/pages/*.templ` exceeds 5 content lines.** Anything larger belongs in `static/js/admin/components/<page>.js`. Never raise the ceiling.

Failure messages include `path:line[-end]` so the offender can be extracted directly.

### Reference implementation

`static/js/admin/components/prompt_builder.js` + `internal/templates/components/pages/prompt_builder.templ` (the `/agent-wizard/{type}` page). Copy this shape for any new Alpine page component.
