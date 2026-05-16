# v2 Design Sprint — Sprint State

This file is the durable backlog and notes for the autonomous v2 SPA design
overhaul. The `/loop` iteration reads it at the start of each run, picks the
next target, then updates this file at the end of the run.

## How iterations work

- Branch: `design/v2-shadcn` is the long-lived design branch. Each iteration
  branches `design/v2-shadcn/<topic>` off it, opens a PR back into
  `design/v2-shadcn`, attaches before/after screenshots, then merges.
- Ricardo reviews merged PRs at his pace. He merges the final
  `design/v2-shadcn → main` when he is satisfied.
- Each PR is small (one page or one component family) so screenshots are easy
  to scan.

## Design principles (evolve as we learn)

- Looks like a real shadcn/ui application — not a generic Tailwind site.
- Heavy reliance on the existing shadcn primitives in
  `web/src/components/ui/`; pull in more via the shadcn MCP when missing.
- Consistent spacing scale, consistent typography, consistent border radii
  across pages.
- Page layouts use a clear hierarchy: page header → primary content →
  secondary panels. Avoid floating cards in a soup.
- Empty states, loading states, error states are first-class — not
  afterthoughts.
- Sidebar, command palette, and breadcrumbs feel cohesive with content.
- Dense data (tables, lists) is the dominant pattern — optimise for scanning.
- Mobile is secondary but pages should not be broken at 768px.

## Component drift to watch

(Populated by iterations as drift is discovered.)

- **Active-state vocabulary** (iter 1, established): primary-tinted 3px
  left rail at row's outer edge + tinted icon + accent bg. Used in
  `nav-main.tsx` and `settings-shell.tsx`. Any new nav/list with an
  active row should reuse this language — pulling the rail into a
  shared util in `web/src/components/` is worth doing once a third
  surface needs it.
- **Branch naming gotcha** (iter 1): the remote already holds
  `design/v2-shadcn` as a leaf ref, so a child branch named
  `design/v2-shadcn/<topic>` cannot be pushed (git refs can't be both a
  file and a directory). Use a hyphen instead: `design-v2-shadcn/<topic>`.

## Backlog (ordered roughly by impact)

Pages:

- [x] App shell + sidebar (`app-sidebar.tsx`, `__root.tsx`, `settings-shell.tsx`) — #1113
- [ ] Home / dashboard (`home.tsx`)
- [ ] Transactions list (`transactions.tsx`)
- [ ] Transaction detail (`transaction-detail.tsx`)
- [ ] Accounts list (`accounts.tsx`)
- [ ] Account detail (`account-detail.tsx`)
- [ ] Categories list (`categories.tsx`)
- [ ] Category detail (`category-detail.tsx`)
- [ ] Category new (`category-new.tsx`)
- [ ] Tags list (`tags.tsx`)
- [ ] Tag detail / new (`tag-detail.tsx`, `tag-new.tsx`)
- [ ] Connections list (`connections.tsx`)
- [ ] Connection detail (`connection-detail.tsx`)
- [ ] Providers settings (`providers.tsx`)
- [ ] API keys (`api-keys.tsx`, `api-key-new.tsx`, `api-key-created.tsx`)
- [ ] Login (`login.tsx`)
- [ ] Setup account (`setup-account.tsx`)
- [ ] Placeholder (`placeholder.tsx`)

Cross-cutting components:

- [~] `page-header.tsx` — canonical header revised in #1113 (added
  `eyebrow`, tightened spacing, sm:flex-row footer). Still needs a sweep
  to migrate the remaining pages that build their own headers.
- [ ] `empty-state.tsx` — visual language
- [ ] `data-table.tsx` — density, sort headers, hover
- [ ] `command-palette.tsx` — sections, kbd hints, recents
- [ ] `category-badge.tsx` / `tag-chip.tsx` — colour tokens, sizes
- [ ] `transaction-amount.tsx` — currency rendering
- [ ] Form patterns (used across new/edit pages) — labels, validation, footers
- [ ] Toast (`sonner.tsx`) — variants, action affordances
- [ ] Confirmation dialogs (`alert-dialog.tsx` usage) — consistency

## Completed

(Appended by iterations after merge.)

- **Iter 1 — App shell + sidebar** ([#1113](https://github.com/canalesb93/breadbox/pull/1113))
  - Sidebar: primary-tinted left rail + brighter icon for active rows;
    uppercase tracked group labels; brand header is a real lockup with
    icon tile and a "V2 Preview" mini-badge.
  - Topbar: shadcn `Breadcrumb` (with chevron) replaces flat text; ⌘K
    command-palette trigger pill on the right (search-input visual with
    kbd hint); header sticky with backdrop blur.
  - PageHeader: optional `eyebrow`, tighter sm:flex-row layout; Home
    page routes through it for consistency.
  - Settings modal section list adopts the same active-state language.

## Open observations / questions

(Populated by iterations.)

- **Backend already on 8090** in this dev environment — iter 1 reused
  it instead of starting a second `make dev`. Future iterations should
  do the same when a healthy `/v2/` is already serving; saves ~10s and
  one ENCRYPTION_KEY dance.
- **Vite file-watching is unreliable inside `/private/tmp/claude/...`
  worktrees** on macOS (fsevents doesn't catch edits in some scratch
  paths). When the browser shows stale code, kill+restart Vite —
  CHOKIDAR_USEPOLLING=1 helps but doesn't fully fix it. HMR isn't a
  hard blocker — iter 1 shipped fine with cold reloads.
- **The Vite `BREADBOX_BACKEND_PORT` env var** is the supported way to
  point the SPA at a non-default backend port; documented in
  `web/vite.config.ts`. Useful when piggybacking on an existing `make
  dev` from another worktree (see above).
