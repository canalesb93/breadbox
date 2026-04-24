# Keyboard Shortcuts

Breadbox's admin UI is keyboard-first. Every page registers a **scope** on mount; the global dispatcher, the `?` help modal, and the `Cmd+K` command palette all read from a single Alpine store and stay in sync automatically.

- Press `?` anywhere for the live list of shortcuts — the modal groups them by **Global** and **This page**, and only shows bindings the current page actually registered.
- Press `Cmd+K` (`/` also works) for the command palette. Rows that map to a registered shortcut render their binding inline.
- Shortcuts are suppressed on touch devices (`$store.device.isTouch`), so hint `kbd` glyphs stay out of the way on mobile.

For the authoritative runtime list on any given page, open the `?` modal. The tables below capture the shortcuts that shipped during the keyboard-nav sprint (2026-04).

## Global shortcuts

Always available — wired once in `internal/templates/layout/base.html`.

| Keys | Action |
| --- | --- |
| `?` | Open the shortcuts help modal |
| `Cmd+K` | Open the command palette |
| `/` | Open the command palette (alt) — pages may reassign `/` locally (e.g. Transactions uses it to focus search) |
| `Esc` | Close the open dialog / cancel the current overlay |
| `g` then `d` | Go to Dashboard |
| `g` then `t` | Go to Transactions |
| `g` then `c` | Go to Connections |
| `g` then `r` | Go to Reviews |
| `g` then `u` | Go to Rules |
| `g` then `a` | Go to Categories |
| `g` then `l` | Go to Sync Logs |
| `g` then `b` | Go to Backups |
| `g` then `s` | Go to Settings |
| `g` then `p` | Go to Providers |
| `g` then `m` | Go to MCP Settings |
| `g` then `k` | Go to Access (API keys) |
| `g` then `f` | Go to Users |
| `n` then `c` | New Connection |
| `n` then `i` | Import CSV |
| `n` then `k` | New API Key |
| `n` then `u` | New User |

Chord keys (`g+X`, `n+X`) use a 500 ms prefix window. The help modal lists only the bindings that have a registered action; any mapping you see in the modal will fire — the list above will rot if the source of truth drifts, so treat the modal as canonical.

## Per-page shortcuts

Each page calls `$store.shortcuts.setScope('<page>')` in its root `x-init` and resets to `'global'` in `x-destroy`. Shortcuts fire only within their own scope.

### Transactions list — scope `transactions`

| Keys | Action |
| --- | --- |
| `j` / `k` | Move focus down / up |
| `Enter` | Open focused transaction |
| `c` | Categorize focused row |
| `t` | Tag focused row |
| `e` | Expand / collapse row |
| `x` | Toggle selection on focused row |
| `/` | Focus the quick-search input |
| `Esc` | Clear selection, then exit select mode, then clear focus |

### Transaction detail — scope `transaction-detail`

| Keys | Action |
| --- | --- |
| `c` | Categorize transaction (opens inline picker) |
| `t` | Edit tags (opens tag picker) |
| `e` | Toggle system details |

### Reviews queue — scope `reviews`

The review queue reuses the transactions list UI, so `j/k/Enter/c/t/Esc` all work there too. The reviews scope layers on the workflow actions:

| Keys | Action |
| --- | --- |
| `a` | Approve focused review |
| `r` | Reject focused review |
| `s` | Skip focused review |

### Connections — scope `connections`

| Keys | Action |
| --- | --- |
| `j` / `k` | Move focus down / up (skips hidden rows) |
| `Enter` | Open connection detail |
| `s` | Sync focused connection (respects 5 s cooldown) |

### Rules list — scope `rules`

| Keys | Action |
| --- | --- |
| `j` / `k` | Move focus down / up |
| `Enter` | Edit focused rule |
| `n` | Create a new rule (shadows the global `n+_` chord while this scope is active) |
| `Space` | Toggle focused rule's enabled state |
| `d` | Delete focused rule (uses the confirm dialog; system rules are skipped) |

### Rule detail — scope `rule-detail`

| Keys | Action |
| --- | --- |
| `Cmd+Enter` / `Ctrl+Enter` | Save (toggles rule enabled on the detail form) |
| `a` | Apply rule retroactively (opens the confirm modal) |
| `Esc` | Cancel / close the apply modal |

### Categories tree — scope `categories`

Expand/collapse is handled inline via Alpine `@keydown.enter` / `@keydown.space` on each parent row. The scope adds tree navigation:

| Keys | Action |
| --- | --- |
| `j` / `k` | Move focus down / up through visible rows |
| `n` | Add a new category (clicks the Add link; shadows global `n+_`) |

### Backups — scope `backups`

| Keys | Action |
| --- | --- |
| `n` | Create Backup (clicks the submit button; respects preflight gating) |

### Users — scope `users`

| Keys | Action |
| --- | --- |
| `n` | Add member (clicks the Add Member link) |

### Dashboard, Settings — scope-only

These pages register their scope so the help modal reflects the truth, but add no page-scoped letter bindings. Dashboard leans on the `g+_` navigation chords; Settings is a form-heavy surface where single-key bindings would collide with inputs.

## Architecture

The registry lives in `internal/templates/layout/base.html` as an Alpine store:

```js
Alpine.store('shortcuts', {
  items: [...],                 // { id, keys, description, group, scope, when?, action, visible? }
  register(spec) { ... },
  unregister(id) { ... },
  forScope(scope) { ... },
  currentScope: 'global',
  setScope(s) { ... },
})
```

A single global `keydown` listener in `base.html` matches events against `forScope(currentScope)` plus `'global'`. Input fields short-circuit (unless a spec opts in), overlays with `data-dialog-open` are respected, and touch devices are skipped for page-level shortcuts (globals like `?`, `Cmd+K`, and `Esc` still fire). Chord state (`g`, `n`) is held for 500 ms; a page-scoped single key can shadow a global chord prefix for that page.

- `base.html` seeds `goRoutes` (chord `g+X`) and `newRoutes` (chord `n+X`) into the registry on `alpine:init`.
- `Alpine.store('device')` (`isTouch`, `isMobile`) is populated from `matchMedia('(hover: none) and (pointer: coarse)')` once per page load.
- The `Kbd` / `KbdChord` templ components (`internal/templates/components/kbd.templ`) wrap glyphs with `x-show="!$store.device.isTouch"` so hints disappear on touch devices without killing the underlying handler.
- The help modal (`base.html` around line 271) and the command palette (`base.html` around line 1650) both render off the registry. Palette rows render their binding via a `shortcutId` lookup — if no registration matches, the row renders without a kbd hint.

The source of truth is `base.html`. Per-page shortcuts live alongside the page template (e.g. `internal/templates/pages/transaction_detail.html`, `internal/templates/components/pages/assets/transactions_scripts.js.html`) and register via `Alpine.store('shortcuts').register(...)` on `alpine:init`.

## Touch / mobile

- `Alpine.store('device').isTouch` is set via `matchMedia('(hover: none) and (pointer: coarse)')`.
- The dispatcher early-exits for page-scoped shortcuts when `isTouch` is true; only `global` entries still fire.
- All `Kbd` / `KbdChord` components hide on touch, so no shortcut glyphs leak into the mobile UI.

## Adding a new shortcut

1. Pick the right scope. For a new page, call `setScope` in `x-init`/`x-destroy`:

    ```html
    <div x-data
         x-init="$store.shortcuts.setScope('my-page')"
         x-destroy="$store.shortcuts.setScope('global')"></div>
    ```

2. Register every binding through the store. Never add a raw `addEventListener('keydown', ...)`.

    ```js
    document.addEventListener('alpine:init', function () {
      var reg = Alpine.store('shortcuts');
      if (!reg) return;
      reg.register({
        id: 'my-page.do-thing',
        keys: 'd',
        description: 'Do the thing',
        group: 'Actions',       // shows up as a header in the ? modal
        scope: 'my-page',
        // when: function() { return somethingFocused(); },  // optional predicate
        action: function () { doTheThing(); },
      });
    });
    ```

3. If you want a visible hint in the UI, wrap the glyph in the `Kbd` templ component (or `KbdChord` for chord bindings) — it will hide itself on touch devices.

4. Verify: reload the page, press `?`, and confirm your binding appears under **This page**.

`id` is unique per binding. Re-registering the same `id` replaces the previous spec, so each page's registration block is safe to re-run (Alpine does this in some dev flows).
