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
- **Scoreboard / stat card** (iter 2): `StatCard` lives inline in
  `features/home/home-stats.tsx` — tiny uppercase label + icon, big
  tabular-nums value, hint line, optional accent (positive / negative /
  warning). If Reports or Accounts wants the same scoreboard pattern,
  extract to `components/stat-card.tsx` then. Don't pre-extract.
- **Card with list rows** (iter 2): Home uses `<Card className="gap-0
  py-0">` with a `border-b` `CardHeader` and `<ul className="divide-y">`
  rows inside `<CardContent className="px-0 py-0">`. Reused for both the
  recent-activity feed and the connections panel — if a third surface
  needs the pattern (e.g. Reports recent insights), wrap into a
  `ListCard` primitive.

## Backlog (ordered roughly by impact)

Pages:

- [x] App shell + sidebar (`app-sidebar.tsx`, `__root.tsx`, `settings-shell.tsx`) — #1113
- [x] Home / dashboard (`home.tsx`) — #1115
- [x] Transactions list (`transactions.tsx`) — #1116
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
- [~] `data-table.tsx` — density + hover tightened in #1116 (new
  `stickyHeader` + `refinedHeader` opt-ins; `Table` primitive picks
  up softer borders and `px-3 py-2.5` cell padding). Sort header
  affordances still TODO when we wire interactive sorting.
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

- **Iter 2 — Home / dashboard** ([#1115](https://github.com/canalesb93/breadbox/pull/1115))
  - Replaced the three placeholder cards with a real dashboard: KPI
    strip (net cash, cash, credit & loans, connections health),
    recent-activity feed (6 rows, reusing `TransactionPrimary` +
    `TransactionAmount` so vocabulary matches the Transactions list),
    and a connections side panel sorted attention-first.
  - PageHeader gets a `Synced X ago` eyebrow (max `last_synced_at`
    across connections) and primary/secondary CTAs (Connect bank,
    Transactions).
  - Skeletons + empty states wired for every section. Empty connections
    panel CTA goes straight to `/connections?action=connect`.
  - Reuses `ConnectionStatusBadge` + `relativeTime` from
    `features/connections` — no forked rendering.

- **Iter 3 — Transactions list** ([#1116](https://github.com/canalesb93/breadbox/pull/1116))
  - PageHeader now carries a description + "Showing N of M" eyebrow
    + primary "Connect bank" CTA, so the densest page in the app
    lands with intent instead of a naked H1. The redundant
    "Showing 50 of 879 · Press J / K to navigate" sub-line is gone
    — count moved up, j/k discoverability handed off to the global
    shortcut sheet.
  - `DataTable` gains opt-in `stickyHeader` + `refinedHeader` props.
    Transactions opts into both: column band stays pinned on scroll
    with a tinted backdrop blur and reads as small uppercase tracked
    labels (Transaction / Category / Amount).
  - `Table` primitive picks up softer `border-border/60` row
    separators, calmer `hover:bg-muted/40`, and a uniform
    `px-3 py-2.5` cell — denser scan path without feeling cramped.
    Container is `rounded-lg` with a quieter border.
  - No-filters empty state offers a "Connect a bank" CTA instead of
    a dead end.

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
- **Worktree creation under sandbox** (iter 2): `EnterWorktree` rejects
  the call from a subagent; falling back to `git worktree add
  -b <branch> /tmp/claude/<dir> origin/design/v2-shadcn` works even when
  sandbox blocks `.git/config` writes (the worktree still gets created;
  the config errors are warnings, not fatals). Branch rename via
  `git branch -m` also succeeds despite the config write error.
- **`go build ./...` from a fresh worktree** needs `sqlc generate`
  first (committed `*.sql.go` are required by `internal/testutil`) and
  a stub `static/css/styles.css` (gitignored Tailwind output;
  `mkdir -p static/css && touch static/css/styles.css` is enough for
  the embed pattern to resolve). After that, no Go regen needed for
  v2 SPA work.
- **Templ also needs `templ generate`** in a fresh worktree (iter 3):
  `internal/templates/components/helpers.go` references the
  lowercase generated `txAvatarColorStyle`, which only appears in the
  `*_templ.go` siblings — without regen, `go build ./...` fails with
  "undefined: txAvatarColorStyle". `/Users/canales/go/bin/templ` is
  on PATH; running it once at worktree setup is enough.
- **Vite restart with `CHOKIDAR_USEPOLLING=1` is the fix** when the
  worktree lives under `/tmp/claude-501/...` and fsevents silently
  drops edits (iter 3). Symptom: edits are saved to disk and `curl`
  hits Vite's transform showing OLD content. Polling watcher catches
  every change reliably, at the cost of more CPU — fine for an
  iteration. `bun dev --port <N> --force` on a fresh port also flushes
  Vite's dep cache.
