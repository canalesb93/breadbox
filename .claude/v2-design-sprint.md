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

- _empty_

## Backlog (ordered roughly by impact)

Pages:

- [ ] App shell + sidebar (`app-sidebar.tsx`, `__root.tsx`, `settings-shell.tsx`)
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

- [ ] `page-header.tsx` — establish canonical header
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

- _none_

## Open observations / questions

(Populated by iterations.)

- _empty_
