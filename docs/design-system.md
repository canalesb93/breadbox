# Design System Architecture

> **Component shapes & visual conventions live in the `/design` sandbox**
> (`internal/templates/components/pages/design_sections.templ` + the
> live gallery at `http://localhost:<port>/design`). The sandbox is the
> source of truth — read it before adding a `bb-*` class or hand-rolling
> something daisy ships. This file documents the architectural pieces
> that don't fit a component specimen: build setup, layout patterns,
> form patterns, async button state, Alpine page-component convention,
> and the mobile baseline.
>
> Visual tuning (radii, palette, density) is configured via the
> [daisyUI theme builder](https://daisyui.com/theme-generator/) — do
> not reintroduce per-size CSS overrides here.

## 1. Build setup

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


## 2. Layout patterns

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

### Drawer / slide-over (`components.Drawer`)

The canonical **right-side slide-over** for focused create/edit flows
that shouldn't take over the whole page or fight for room inside a
modal. Backdrop + sliding panel + `DrawerHeader` (icon · title ·
subtitle · close) + scrollable body + `DrawerFooter` (sticky actions).
Reach for it for **simple inline edits** (rename, reconfigure, set up)
where a full page is too heavy and a centered modal is too cramped for
a vertical form.

Why a templ component and not daisy: DaisyUI's `drawer` is the
**sidebar layout** primitive (the one in `base.html`) — it's not a
right-anchored sheet. This is the justified extension; don't hand-roll
a fifth copy of the chrome.

Open/close is driven by the global `$store.drawers` Alpine store
(defined in `layout/base.html`) keyed by the `ID` you pass. Only one
drawer is open at a time, so many `Drawer` instances can sit on one
page and only the matching one + its backdrop show. Escape and a
backdrop click both close; body scroll is locked while open.

```go
// Trigger — from markup or JS:
//   <button @click="$store.drawers.open('edit-note')">Edit</button>
//   Alpine.store('drawers').open('edit-note')

@components.Drawer(components.DrawerProps{ID: "edit-note", Title: "Edit note"}) {
    <form class="flex flex-col h-full" @submit.prevent="save($event.target)">
        @components.DrawerHeader(components.DrawerHeaderProps{
            Icon:     "pencil",
            Title:    "Edit note",
            Subtitle: "A focused edit surface.",
        })
        <div class="flex-1 overflow-y-auto p-5 space-y-5">
            <textarea name="note" class="textarea w-full" rows="4"></textarea>
        </div>
        @components.DrawerFooter() {
            <button type="button" class="btn btn-ghost btn-sm" @click="$store.drawers.close()">Cancel</button>
            <button type="submit" class="btn btn-primary btn-sm">Save</button>
        }
    </form>
}
```

`DrawerProps.Size` maps to panel max-width: `sm`→`max-w-sm`,
`md`→`max-w-md` (default), `lg`→`max-w-lg`, `xl`→`max-w-2xl`. For a
header set at runtime (a drawer hydrated by JS), pass
`DrawerHeaderProps.TitleExpr` / `SubtitleExpr` (Alpine `x-text`
expressions) instead of the static strings. Live variants:
`/design/c/drawers`. Reference call sites: the Workflows gallery's
**Set up** / **Configure** / **Reconfigure** flows
(`workflows_gallery.templ`).

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
      <a href="/users" class="btn btn-sm btn-ghost">Cancel</a>
      <button type="submit" class="btn btn-sm btn-primary gap-1.5 min-w-32" :disabled="submitting">
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

### Error surfacing — banner vs badge

Errors and warnings should appear in **exactly one place** per page. When the same error is communicated as both a badge in the entity header *and* an alert banner below, the banner always wins — drop the badge.

**Pick one surface per condition:**

| Condition | Use |
|---|---|
| Error has a **message + action** the user must take (reauth, fix-it button, "view details") | **Banner / alert card** — full-width, prominent, contains the action |
| Error is a **transient state on a list row** with no per-row action | **Badge** — compact, scannable across many rows |
| Entity is in a **neutral state** (Active, Paused, Stale, Manual trigger) | **Badge** — these are state indicators, not duplicates of an error message |

**Decision rule (canonical):**

> If a banner or alert card on this page surfaces an error or required action, the entity header (or row header) must **not** also render a chip for that same condition. The banner carries the message; the chip would be a second copy of the same signal.

Neutral metadata badges (`Paused`, `Stale`, trigger label, `Manual`, etc.) are **orthogonal** to the error and stay regardless of whether a banner is present — they communicate different facts.

**Implementation pattern.** Compute a `hasErrorBanner` predicate from the same conditions the banner uses, then gate the status chip on `!hasErrorBanner`. Example from `connection_detail.templ`:

```go
// connDetailHasErrorBanner mirrors the two banner conditions above the
// EntityHeader (reauth-required + 3+ consecutive failures). The status
// chips inside connDetailBadges read this so they don't duplicate the
// same signal as an inline pill.
func connDetailHasErrorBanner(p ConnectionDetailProps) bool {
    if p.Status == "error" || p.Status == "pending_reauth" {
        return true
    }
    if p.ConsecutiveFailures >= 3 {
        return true
    }
    return false
}
```

```templ
templ connDetailBadges(p ConnectionDetailProps) {
    if !connDetailHasErrorBanner(p) {
        // status / sync-error chip — suppressed when a banner is canonical
        @templ.Raw(components.StatusBadge(p.Status))
    }
    if p.Paused {
        <span class="badge badge-ghost badge-sm">Paused</span>  <!-- orthogonal: always shown -->
    }
}
```

**What this fixes.** Before the consolidation, `/connections/{id}` rendered a full red banner ("This connection needs re-authentication" + Re-authenticate button) AND a `Reauth` chip beside the title, and `/sync-logs/{id}` rendered a "warning" chip beside the title AND a Warning detail card below. Both were two visual carriers of the same fact. After: banner wins, chip disappears.

**Anti-patterns to avoid:**

- A status chip + alert banner saying the same thing on the same page.
- A "Sync Error" hover-tooltip chip when the row already has a banner below explaining the same error.
- Stacking a header chip on top of a row-level error banner inside the same card.

When in doubt: **delete the chip, keep the banner**. The banner has the message + the action; the chip is just a colored label of the same fact.


## 3. Form patterns

### Filter Bar

Used on transactions, sync logs, account detail:
```html
<div class="bb-filter-bar">
  <label>
    Start Date
    <input type="date" name="start_date" class="input input-sm" />
  </label>
  <label>
    Status
    <select name="status" class="select select-sm">
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
  <input type="text" class="input input-sm w-48" ... />
  <span x-show="saving" class="loading loading-spinner loading-xs"></span>
</div>
```

### Form Validation

Use DaisyUI form control patterns:
```html
<label class="label">
  <span class="label-text">Password</span>
</label>
<input type="password" class="input" required />
<!-- Error state -->
<input type="password" class="input input-error" />
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
    <a href="/users" class="btn btn-sm btn-ghost">Cancel</a>
    <button @click="save()" :disabled="!dirty || saving" class="btn btn-sm btn-primary gap-1.5 min-w-32">
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
    <button class="btn btn-error btn-soft btn-sm shrink-0" @click="…">
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
  <div tabindex="0" role="button" class="btn btn-ghost btn-sm btn-square opacity-40 hover:opacity-100 transition-opacity">
    <i data-lucide="ellipsis-vertical" class="w-5 h-5"></i>
  </div>
  <ul tabindex="0" class="dropdown-content menu bg-base-100 rounded-xl shadow-lg border border-base-300 z-50 w-44 p-1">
    <li><a href="…"><i data-lucide="pencil" class="w-3.5 h-3.5"></i> Edit profile</a></li>
    <li><a href="…"><i data-lucide="settings" class="w-3.5 h-3.5"></i> Manage login</a></li>
  </ul>
</div>
```

Keep icons at `w-3.5 h-3.5` in dropdown rows to match the `btn-xs` baseline weight of menu items.


## 4. Async button loading

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


## 5. Alpine page components

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

## 6. Mobile baseline

The admin UI targets **iOS 26.2 / iPadOS 26.2 and the latest desktop Safari**. Anything older than the baseline still loads, but renders a soft-warn banner at the top of the shell prompting the user to update.

**Why 26.2.** Safari 26.2 closed several long-standing iOS bugs we used to work around at the component level — sticky-positioned elements decoupling during rubber-band overscroll, the keyboard not revealing focused elements that needed it, and `scrollRectToVisible` mis-targeting when the on-screen keyboard was open. It also ships the platform APIs the admin UI leans on:

- **`field-sizing: content`** — inputs and textareas size to their content without JS measurement.
- **`scrollend` event** — settle-aware infinite-scroll and analytics, no more `setTimeout` after `scroll` ticks.
- **`scrollbar-color`** — themable scrollbars that match the shadcn palette without WebKit-specific CSS.
- **Event Timing API + LCP** — Core Web Vitals reporting from Safari now matches Chromium, so perf budgets compare apples-to-apples.
- **View Transitions API improvements** — same-document transitions work consistently on iOS, used in the route-change crossfade.

**Patterns.** Apply mobile / iOS Safari patterns — dynamic viewport units, safe-area insets, 44pt tap targets via `pointer-coarse:`, `scroll-shadow-x`, mobile reflow, iOS keyboard hints, bfcache lifecycle, reduced motion, web-app metadata, accessibility — when authoring templ pages or styles in `input.css`. Manual QA on a real device (or Safari's responsive design mode) covers the rest until automated coverage returns.
