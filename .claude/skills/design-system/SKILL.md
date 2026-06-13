---
name: design-system
description: >
  Breadbox's surface design language — the conventions for building and
  redesigning admin UI surfaces so every page reads as one calm, legible
  product. Codifies the principles, the canonical page anatomy, the component
  vocabulary, and the redesign method (IA-first, not a re-skin). Invoke when
  building a new admin surface or bringing an existing one up to standard.
  Triggers: "redesign this page", "apply our design system", "apply the surface
  language", "make this follow our conventions", "polish this surface", "bring
  /accounts (etc.) up to the design system", "what's our pattern for X".
  Pair with the `daisyui` skill (primitive classes) and `validate-ui` (browser
  evidence). NOT for low-level CSS questions — those are `.claude/rules/daisyui.md`.
metadata:
  version: 0.1.0
  status: living
---

# Breadbox Design System

This is the **surface design language** for the admin app: the small set of
decisions that make `/workflows`, `/workflows/runs`, and their drawers the
cleanest surfaces we have, generalized so every page can read the same way —
simple, easy on the eyes, space used well on desktop *and* mobile, with a flow
that makes sense.

It is **living**. When we refine a convention, update this skill in the same PR
so it never drifts from what we actually ship. Treat a contradiction between
this skill and the running app as a bug in one of them — reconcile, don't ignore.

> **Scope.** This skill is about *composition* — how primitives assemble into a
> coherent surface, and how to approach a redesign. For the primitive classes
> themselves (which daisy class, which `bb-*` extension) defer to
> [`.claude/rules/daisyui.md`](../../rules/daisyui.md) and `docs/design-system.md`.
> For Settings tabs specifically, [`.claude/rules/settings.md`](../../rules/settings.md)
> is the local dialect of this same language.

---

## The eight principles

The DNA. Each is a small, reusable decision; together they're why our best
surfaces feel calm.

1. **One consistent chrome.** Every surface opens with the same `PageHeader`
   (title + subtitle) and, when it has sections, a `TabBar`. Switching tabs
   should feel like moving through one surface — not landing on
   differently-shaped pages.

2. **Status lives in one tile.** A single color-coded icon tile on the left
   carries a row's state (success / error / running / reauth). No parallel
   "Success" text badge next to it. Less ink, faster scan. Loading/running
   states are *never* a daisy badge — badges are terminal/categorical only.

3. **One body line, by priority.** A row says at most one thing. Pick the
   highest-priority line (error → headline → note → silence) and let the title
   carry the quiet rows. Don't stack three muted sublines.

4. **Minimal icon clusters.** Secondary actions are bare icons with tooltips —
   run toggle, gear, overflow kebab — not text buttons crowding the row. The
   primary action can be a real button; everything else recedes.

5. **Edit in a slide-over.** Create / configure / reconfigure happen in a
   right-side `Drawer` (header · scrollable body · sticky footer). Context stays
   put — no full-page detour, no cramped centered modal. Use `$store.drawers`.

6. **Generous, honest spacing.** The daisy `list-row` grid inside a `bb-card`,
   hairline dividers, real breathing room. One layout that stacks cleanly on
   mobile — not a desktop table plus a separate mobile card list.

7. **Identity via avatar.** People and workflows get a stable DiceBear avatar
   (`UserAvatar`). Any feed of "who did this" reads the same on every surface.

8. **Every interactive element has interaction states.** Anything clickable — a
   button, a row that toggles, a custom control, a card that acts as a link —
   must show a visible **hover** state *and* a **focus-visible** state for
   keyboard users. Hover is a subtle background/opacity shift, never a jarring
   one; pair it with `transition-colors` so it eases, and `cursor-pointer` so
   the affordance reads. Keyboard focus gets a ring — the project idiom is
   `focus:outline-none focus-visible:ring-2 focus-visible:ring-primary/40` (use
   `focus-visible:`, not `:focus`, so the ring only shows for keyboard nav). The
   mechanics: `hover:bg-base-200/60` (or `hover:bg-base-content/5` on darker
   rows), `transition-colors`, `cursor-pointer`, the focus-visible ring. **Don't
   re-roll press feedback for `.btn`** — `input.css` already ships the Nova
   `:active` translate; a real daisy button is covered. This rule is app-wide:
   a control with no hover and no focus state is a bug, not a style choice.

---

## Canonical page anatomy

A standard surface, top to bottom:

```
PageHeader{ Title, Subtitle, Right? }          ← chrome (principle 1)
  TabBar{ Variant:"border", Items }            ← only if the surface has sections
[ filter row ]                                  ← TabBar Variant:"box" w/ Counts + a quiet <select>
[ summary strip ]                               ← StatTileRow, only when numbers lead the page
group label line  ───────────  subtotal        ← quiet text, not a boxed card-header
  <ul class="list ..."> in a bb-card            ← list-row grid (principle 6)
    list-row: [status tile] [name + 1 body line] [value] [overflow]
  ...
EmptyState{…}                                   ← when the group / page is empty
```

Rules of thumb:

- **Group, don't tab, when the axis is data.** `/accounts` groups rows by
  *connection* with a quiet label line (institution · count … subtotal) over a
  `bb-card` of list-rows — the `/transactions` day-group idiom. A boxed
  card-header per group is too heavy; a label line is enough.
- **A `box` `TabBar` is a filter, not navigation.** Give each item a `Count`.
  Never let it stretch full-width — it scrolls horizontally on mobile
  (`tabs` already does this on main as of #1803). Pair it with a `<select>` for
  the secondary axis instead of a second row of tabs.
- **Summary tiles only when numbers are the headline** (Net Worth / Assets /
  Liabilities on `/accounts`, Backups stats). Don't tile a page whose job is a
  list.
- **Money is private by default.** `Amount` marks values `data-private` unless
  `Public: true`; institution/account names get `data-private="institution|account"`.
  See `internal/templates/components/private.templ` + the privacy engine.

---

## Component vocabulary

Reach for these before authoring anything new. Full prop catalog:
[`components.md`](components.md).

| Intent | Component | File |
| --- | --- | --- |
| Page title + actions | `PageHeader` | `page_header.templ` |
| Section / filter tabs | `TabBar` (`border` nav, `box` filter) | `tab_bar.templ` |
| Section title + count + action | `SectionHeader` | `section_header.templ` |
| A row's status | color-coded `IconTile` | `icon_tile.templ` |
| Slide-over edit/create | `Drawer` + `DrawerHeader` + `DrawerFooter` | `drawer.templ` |
| Choice cards in a drawer | `RadioCard` | `radio_card.templ` |
| Per-row actions | `OverflowMenu` (Size `"sm"`) | `overflow_menu.templ` |
| Identity | `UserAvatar` (`XS`/`SM`) | `user_avatar.templ` |
| Money | `Amount` | `amount.templ` |
| Summary numbers | `StatTile` / `StatTileRow` | `stat_tile.templ` |
| Empty list / zero state | `EmptyState` | `empty_state.templ` |
| Loading | daisy `skeleton` mirroring the real shape | `*_skeleton.templ` |
| Settings tab body | `SettingsSection` + `SettingsRow` | `settings_*.templ` |

A run/feed row reference implementation lives in `agent_run_row.templ`
(`AgentRunRowList` + `AgentRunRow`) — the cleanest example of principles 2–7 in
one component.

---

## Format palette — the list-row is the default, not the only format

The list-row earns its place as the default for *entity lists*, but a great
surface uses the format that fits its content. **Be bold:** a **table**, a
**card grid**, or a **timeline** is the right call when the content calls for it.
What makes them feel like one product is the shared **family treatment**, not a
single layout — don't force everything into list-rows.

Pick by content:

- **list-row** — entity lists (accounts, rules, reports, members, tags…). The
  default. Status tile · title + one body line · value · overflow.
- **table** — dense, sortable, **multi-numeric-column matrices** where columns
  must align across rows (Backups files, CSV preview, a sample-matches grid).
  Don't list-row these.
- **card / card grid** — heterogeneous or **visual** items that need a richer
  preview (the Workflows gallery, onboarding step cards, the recurring
  candidate cards with detection evidence).
- **stat tiles** — summary numbers that lead a page (`StatTileRow`).
- **timeline** — chronological activity (the activity timeline, run transcript).

The **family treatment** is what unifies every format — whatever the layout, it
wears the same skin so it reads as one product:

- the `bb-card` surface (flat border + dark-mode lift) and **hairline dividers** —
  never heavy shadows or boxed sub-headers;
- the color-coded **status tile / `IconTile`** for state — not a parallel text badge;
- **vivid-only badges** (`info`/`success`/`warning`/`error`; `ghost` for a quiet neutral);
- quiet **uppercase `text-xs text-base-content/50`** section + column labels;
- **`tabular-nums`** for money/metrics, `Amount` for currency, privacy marking;
- generous, honest spacing, and principle #8 **hover + focus-visible** on every
  interactive element.

A table that wears these reads as the same family as a list-row. The "family
table" + "family card" specifics live in [`components.md`](components.md).

---

## The redesign method — IA first, not a re-skin

When asked to bring a surface up to the design system, **do not** just swap
classes. Improve the information architecture:

1. **Name the surface's one job.** What is the user here to do? (`/accounts`:
   see balances grouped by where the money lives, and manage a connection.)
   Everything that doesn't serve that job is a candidate for removal.

2. **Audit what's there.** List every control, column, badge, and filter.
   For each: does it earn its pixels? Filters nobody uses, columns that repeat
   the row title, sort headers on a 12-item list — cut them. (We removed the
   type tabs + search box from `/accounts` because the list is short.)

3. **Find the right grouping.** A flat list of N things usually wants a
   grouping axis — connection, day, category, status. Group with a quiet label
   line, order groups by something meaningful (subtotal desc, recency), and sink
   the orphan/empty bucket last.

4. **Collapse to the anatomy above.** Map the content onto PageHeader → optional
   tabs → optional summary → grouped list-rows → drawer for edits. Move
   multi-input edit flows into a `Drawer`.

5. **Make the grouping testable.** Pure grouping/ordering logic goes in a
   `*_types.go` helper with a unit test (see `GroupAccountsByConnection` +
   `accounts_list_test.go`), so the IA decision is pinned, not vibes.

6. **Keep scope honest.** Server-side filtering must re-scope subtotals; respect
   soft-deletes, attribution (`COALESCE(attributed_user_id, …)`), currency
   (never sum across `iso_currency_code`), and liability sign/color.

It is expected — and wanted — that you make a real leap on what a surface's
redesign *is*. Propose the IA change, show it in the sandbox or a screenshot,
then implement on the live page.

---

## Build & validate loop

1. Edit `.templ` → `templ generate` (or rely on `make dev-watch`). Commit both
   the `.templ` and its generated `_templ.go` sibling.
2. Prototype risky IA in the **`/design` sandbox** first — add a section in
   `internal/templates/components/pages/design_*.templ` + register it in
   `design_types.go`. Share `/design/c/{slug}?embed=1` for review.
3. Run the server with `make dev-bg`; screenshot with `make dev-shot` or the
   `validate-ui` skill. Attach the screenshot to the PR (upload via
   `github-image-hosting` → bb-artifacts.exe.xyz).
4. `go build ./... && go vet ./... && go test ./...` green before pushing.

---

## When NOT to use this

- Low-level "which daisy class" → `.claude/rules/daisyui.md`.
- Settings tab structure → `.claude/rules/settings.md` (a dialect of this).
- The marketing site / onboarding wizard — different patterns (`bb-wizard-*`,
  the Astro site repo).
- The settings modal shell, sidebar, topbar — global chrome, not a surface.
