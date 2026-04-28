# Feed

The home Feed (`/feed`) is the chronological, GitHub-style activity stream
across the whole household — sync runs, agent reports, MCP agent sessions,
bulk-action bursts, and standalone comments rendered as mixed-weight cards on
a single rail. It is bounded to the last three days by default and paginates
backward in three-day chunks up to a 30-day ceiling.

`/feed` is gated behind backlog issue #24 and may eventually replace the
existing dashboard at `/-` for households that opt in. Until then it ships as
an additional page, not a replacement.

This document captures the data shape, aggregation pipeline, filtering and
pagination contracts, hero band, empty states, and the recipe for adding a
new event type. It is the canonical engineering reference for `/feed`; future
agents picking up the page should not need to re-read the implementation.

Cross-references to canonical specs:

- Activity-timeline primitives the Feed builds on: `docs/activity-timeline.md`
  ("Shared primitives"). The per-transaction timeline is the row-shaped
  consumer; the Feed is the global, grouped consumer.
- Schema and enum definitions: `docs/data-model.md`
  (`annotations.kind`, `mcp_sessions`, `sync_logs`).
- Service-layer conventions for read helpers: `.claude/rules/service.md`.

## Overview

The audience is the household admin or a power member sitting down for a
quick check-in: "what did Breadbox do this weekend, and is anything wrong?"
The page answers it inline without forcing a click into per-transaction
detail. It deliberately collapses bulk activity (a 200-annotation MCP review
is one row, not 200) so the rail stays readable on a sparse weekend or after
a re-categorisation burst.

The Feed is read-only. Every interactive affordance on the page (Sync now,
Connect bank, Fix connection, Load older activity, filter chips) is a link or
a small Alpine.js island — there is no compose-on-feed surface. Comments are
rendered through the same markdown scanner as the per-transaction timeline so
a comment posted from `/transactions/{id}` looks identical when it surfaces
here.

## Data model

The renderable unit is `service.FeedEvent` — one row on the rail, possibly
representing many underlying annotations folded together by grouping. Exactly
one of the typed payload pointers is set; the rest are nil.

```go
type FeedEvent struct {
    Type      string    // "sync" | "agent_session" | "bulk_action" | "comment"
    Timestamp time.Time

    Sync         *FeedSyncEvent
    AgentSession *FeedAgentSessionEvent
    BulkAction   *FeedBulkActionEvent
    Comment      *FeedCommentEvent
}
```

Defined in [`internal/service/feed.go`](../internal/service/feed.go).

The handler layer adds one more renderable type — `report` — by pulling
agent reports separately (`svc.ListAgentReports`) and mapping them into the
view model. Reports are not part of `service.ListFeedEvents` because they
have their own table (`agent_reports`) and read/unread state, but they sit
on the rail with the same chrome.

| `FeedItem.Type` | Service source                          | Card                                                                       |
|-----------------|------------------------------------------|----------------------------------------------------------------------------|
| `sync`          | `FeedSyncEvent` (one per `sync_logs` row, retry-folded) | Headline (`12 new from Chase`), inline tx samples, rule-outcome lines, error pill |
| `report`        | `service.AgentReportResponse` (handler-side, not via `ListFeedEvents`) | Title, body excerpt, priority tile, unread dot, tag chips                  |
| `comment`       | `FeedCommentEvent` (un-bucketed standalone comment) | Avatar tile, markdown body bubble, anchored transaction-ref               |
| `agent_session` | `FeedAgentSessionEvent` (one per `mcp_sessions` row, all annotations folded) | API key name + purpose, kind breakdown line, sample tx refs               |
| `bulk_action`   | `FeedBulkActionEvent` (one per `(actor, kind, 5-min)` bucket above threshold) | Verb sentence + subject chips, sample tx refs                              |

The view-model projection lives in
[`internal/templates/components/pages/feed_types.go`](../internal/templates/components/pages/feed_types.go).
`projectFeedEvent` ([`internal/admin/feed.go`](../internal/admin/feed.go))
maps each `FeedEvent` to a `pages.FeedItem`, and resolves display metadata
(tag colours, category icons) for bulk-action subjects via
`tagDisplayLookup` and `categoryDetailLookup` so the rail never renders raw
slugs.

## Aggregation pipeline

`service.ListFeedEvents` is the single entry point and runs in three stages.
The pipeline is described in the function-level comment on
[`ListFeedEvents`](../internal/service/feed.go); the canonical version is
this doc.

### Stage 1 — windowed annotation pull

```sql
SELECT a.*, t.*, ac.name, bc.institution_name, ...
FROM annotations a
JOIN transactions t ON a.transaction_id = t.id
LEFT JOIN accounts ac ON t.account_id = ac.id
LEFT JOIN bank_connections bc ON ac.connection_id = bc.id
LEFT JOIN auth_accounts aa
    ON a.actor_type = 'user' AND aa.id::text = a.actor_id
LEFT JOIN users u_via_account ON aa.user_id = u_via_account.id
LEFT JOIN users u_direct
    ON a.actor_type = 'user'
   AND aa.id IS NULL
   AND u_direct.id::text = a.actor_id
WHERE t.deleted_at IS NULL
  AND a.created_at >= $1   -- cutoff = upper - window
  AND a.created_at <  $2   -- upper  = now or ?before=
  AND a.kind NOT IN ('sync_started', 'sync_updated')
  AND COALESCE(a.payload->>'applied_by', '') <> 'sync'
ORDER BY a.created_at DESC
```

Two filtering rules drop annotations that would double-count an event:

- `sync_started` / `sync_updated` per-transaction rows are excluded — every
  one of them is already represented by the parent sync card built in Stage 3.
- Rule-applied annotations whose `payload.applied_by == "sync"` are excluded
  for the same reason: the sync card surfaces rule outcomes inline via
  `sync_logs.rule_hits`. Rule applications written outside a sync (manual
  `apply_rule`, retroactive backfill) keep `applied_by != "sync"` and survive.

The user join mirrors `ListAnnotationsWithActorByTransaction` from the
per-transaction timeline so `actor_name` resolves identically across both
surfaces (preferring the live `users.name` over the frozen-at-write-time
`annotations.actor_name`, plus the `users.updated_at` epoch as the avatar
cache-buster).

### Stage 2 — hard group by `session_id`

`groupAnnotationsIntoEvents` ([`internal/service/feed.go`](../internal/service/feed.go))
splits the rows into two buckets:

1. Annotations with `session_id IS NOT NULL` → grouped per session into one
   `FeedAgentSessionEvent`. Every MCP session collapses into a single row no
   matter how many annotations it produced — `KindCounts` carries the
   per-kind breakdown ("23 categorised, 12 tagged, 8 commented"), and
   `SampleTransactions` picks up to `SampleLimit` unique transaction
   refs sorted by absolute amount.
2. Annotations without a session → handed to Stage 3.

Session metadata (api-key name, purpose, started-at) is fetched in one
follow-up query against `mcp_sessions`.

### Stage 3 — soft bucket by `(actor_id, kind, 5-min)`

Un-sessioned annotations are bucketed by:

```go
key := bucketKey{
    actorID: actorID, // actor UUID, fallback to "type:name"
    kind:    annotation.Kind,
    bucket:  annotation.CreatedAtTime.Unix() / int64(5*60),
}
```

Buckets at-or-above `BulkThreshold` (default 3) collapse into a
`FeedBulkActionEvent`. Below threshold, only `comment` rows survive — every
other singleton (a one-off retroactive `category_set`, a stray
`tag_added`) is dropped because it is too low-signal for the home rail. The
per-transaction timeline still shows them; the home Feed deliberately does
not.

Tombstoned comments (`IsDeleted == true` after `EnrichAnnotations`) are
skipped at this stage — the home Feed does not render comment tombstones,
only the per-transaction timeline does.

### Stage 4 — sync events

`fetchFeedSyncEvents` runs a separate query against `sync_logs` for the same
window:

```sql
SELECT sl.id, sl.connection_id, bc.institution_name, bc.provider,
       sl.trigger, sl.status, sl.added_count, sl.modified_count,
       sl.removed_count, sl.error_message, sl.started_at,
       sl.completed_at, sl.rule_hits
FROM sync_logs sl
JOIN bank_connections bc ON sl.connection_id = bc.id
WHERE sl.started_at >= $1 AND sl.started_at < $2
ORDER BY sl.started_at DESC
```

A sync row only becomes a card if `added + modified + removed > 0` **or**
`status = 'error'`. Successful no-op syncs are dropped — they're the
overwhelming majority of cron fires and they would otherwise drown out every
signal event.

Sample transactions for each sync are fetched in **one** batched query over
all connections in the window, then partitioned in Go by
`(connection_id, started_at..completed_at)` window with a 30-second slack on
each side. Samples are sorted by absolute amount descending so the biggest
movements surface first.

#### Sync-error dedup

A connection in error state hits the cron-retry cadence (every 15 min), and
left alone produces 50+ identical "still failing" cards a day. The dedup
key is:

```go
errKey{ connectionID, errorMessage }
```

The first error attempt seeds the cluster; subsequent attempts increment
`RetryCount` and rebase `FirstFailureAt` to the earliest start. The card
renders "Failing for 18h · 49 attempts" instead of dozens of identical
rows. Only the most recent attempt survives in the rendered slice.

### Stage 5 — sort and return

The two slices (sync events + grouped events) are concatenated, sorted
newest-first by `Timestamp`, then passed through `filterFeedEvents` for the
chip filter. Reports are not added here — the handler appends them in its
own pass after calling `ListFeedEvents`.

## Window and pagination

The default window is **3 days** (`feedWindowDays = 3` in
[`internal/admin/feed.go`](../internal/admin/feed.go)). The number is tuned
with the product owner — three days lines up with "what's happened this
weekend" and is short enough that the day-bucket separators stay
interpretable.

`?before=<rfc3339>` rolls the window backward in three-day chunks; the
"Load older activity" footer button generates the next URL using
`OldestVisible` from the current page. The handler clamps `before` into
`[now - FeedMaxLookback, now]`, and the service layer enforces the same
ceiling — defence in depth, since the service layer owns the unbounded-query
risk.

`FeedMaxLookback = 30 * 24 * time.Hour` is the hard ceiling. Once
`OldestVisible` is older than `now - FeedMaxLookback`, the templ renders
"End of feed" instead of the load-older button so users have a clear stop
signal. `AtMaxLookback` is computed in the handler and threaded through to
the templ; the service layer never returns events older than the cap.

## Filters

Six chip values are recognised; the handler reads `?filter=` and the service
layer narrows the post-grouping slice via `filterFeedEvents`. The chip strip
is rendered by `feedFilters`; an active chip also shows a banner above the
rail (`feedActiveFilterBanner`) with a "Clear filter" link.

| Chip       | `?filter=` value | Effect                                                    |
|------------|------------------|-----------------------------------------------------------|
| All        | `""`             | No narrowing                                              |
| Syncs      | `syncs`          | Only `sync` events                                        |
| Reports    | `reports`        | Only reports — `ListFeedEvents` returns nil; handler still loads reports |
| Comments   | `comments`       | Only `comment` events                                     |
| Sessions   | `sessions`       | Only `agent_session` events                               |
| From me    | `me`             | Events whose actor matches the session's resolved id      |

The `me` chip's actor resolution path is the gotcha. The handler does:

```go
sessionActorID = SessionUserID(tr.sm, r)         // linked household user_id
if sessionActorID == "" {
    sessionActorID = SessionAccountID(tr.sm, r)   // fallback for initial admin
}
if sessionActorID == "" {
    filter = ""                                    // silent downgrade to All
}
```

Both forms are accepted because `annotations.actor_id` historically stores
either depending on whether the `auth_accounts` row is linked to a household
user. The initial admin (which is created before any household user exists)
writes annotations against its `auth_accounts.id`; later household members
are linked and write against `users.id`. The downgrade-to-All when neither
resolves is intentional — better to render a useful page than blank
silence.

Reports and connection alerts hide under any active chip so the filtered
view is exclusively the chip's scope; both reappear on the unfiltered
"All" page.

## Hero band

The `FeedHero` view-model carries five tile values:

| Tile                  | Source                                                                |
|-----------------------|-----------------------------------------------------------------------|
| Events today          | Sum of `FeedItem`s with `Timestamp >= startOfDay`                     |
| New transactions      | `Σ FeedSyncEvent.AddedCount` for syncs that started today             |
| Last sync             | Newest `FeedSyncEvent.StartedAt` across the visible window            |
| Unread reports        | `Σ AgentReportResponse where ReadAt == nil` (no window cap)          |
| (sub-line) Next sync  | `formatNextSync(a.Scheduler.NextRun())`                               |

`LastSyncAt` is collected in the same loop that projects FeedItems — only
events inside the rendered window contribute. `lastSyncStatus` and
`lastSyncInstitution` follow the newest sync so the tile sub-line renders
"success · Chase" or "error · Wells Fargo".

`NextSyncRel` drives the "Next sync in ~6h" sub-line under the Last Sync
tile so the page answers "why no new transactions yet?" inline. The
scheduler is nil in test environments (no cron); the templ hides the
sub-line when `NextSyncRel == ""`. The sub-line is also suppressed when
the last sync status is `error` — the next tick won't help until reauth
(`feedShowNextSync` in feed.templ).

## Empty states

The Feed has three distinct empty-state variants. `feedEmptyState` in
[`feed.templ`](../internal/templates/components/pages/feed.templ) dispatches
on `(HasConnections, Filter, LastSyncAt)`. Order matters:

1. **First-run** — `!HasConnections`. Shown when zero bank connections exist.
   Copy: "Welcome to Breadbox · Connect your first bank to start filling
   your feed." CTA: primary `Connect a bank` button to `/connections/new`.
2. **Filtered** — chip is active but produced zero rows. Copy reflects the
   chip ("No syncs in the last 3 days", etc.). CTA: ghost `Clear filter`
   button to `/feed`.
3. **Quiet around here** — connections exist, no chip, no events in window.
   Admins get a `Sync now` button (Alpine `feedSyncNow` POSTs to
   `/-/connections/sync-all`); members see the link to `/transactions`
   only because sync-all is admin-only. Copy mentions `LastSyncAt` when
   present so operators can tell "broken sync" from "actually quiet".

The order is non-negotiable: a filter active on a household with zero
connections still shows the first-run card, because installing onboarding
is the only way forward. Filtered before quiet because a filter that yields
zero is a different problem than a sleepy weekend.

## Rendering

Every row threads through the shared timeline primitives in
`internal/templates/components/timeline.templ`:

- `TimelineSystemRowCustomTile` for `sync`, `report`, `agent_session`,
  `bulk_action` — the caller renders the entire 24px tile (avatar for
  actor-driven rows, status-tinted icon for sync, priority-tinted icon for
  report).
- `TimelineCommentRowRaw` for `comment` — the caller renders the full
  meta line and markdown body bubble. The same markdown scanner
  (`/static/js/admin/markdown.js` + marked + DOMPurify) used on
  `/transactions/{id}` is loaded at the top of `feed.templ` so a comment
  posted from the per-transaction composer renders identically here.

The row-chrome contract (rail centring, 24px tile geometry, day-separator
dot, relative-time pill) lives in `docs/activity-timeline.md` — the Feed
deliberately does not redefine any of it. `feedRow` dispatches on
`FeedItem.Type` and never falls back to a bespoke `<li>`.

`Timeline` is invoked with `Variant: "prominent"` so the home Feed gets a
slightly heavier section heading than the per-transaction page; everything
else is shared.

## Connection alerts

`buildFeedConnectionMeta` does one `ListBankConnections` pass and projects
out three things:

- **Pinned alert cards** for connections in `error` or `pending_reauth`
  status. Rendered above the rail in `feedAlerts`. Each alert links to
  `/connections/{id}` for the fix flow.
- `hasConnections` — drives the first-run empty-state branch.
- `globalLastSyncAt` — most recent successful sync across the household,
  used by the "quiet around here · last sync was {rel}" empty state.

Co-locating the projections means the empty-state branch can pick the
right copy without a second query.

Alerts hide under any active chip filter (`if filter != "" { alerts = nil }`)
so the filtered view is exclusively the chip's scope. They reappear on the
unfiltered "All" page. This is deliberate — a user filtering for
`Comments` does not want a connection-error alert pinned on top.

## Adding a new event type

Concrete recipe for surfacing a new kind of event on the rail. Touches five
layers; do them in this order so each commit compiles.

1. **SQL — new source rows.** If the events live in an existing table whose
   rows you can read in the windowed annotation query, no SQL change is
   needed. If they live in a new table (e.g. `agent_reports`), add a
   parallel `fetchFeedXxxEvents(ctx, cutoff, upper, ...)` helper in
   [`internal/service/feed.go`](../internal/service/feed.go) following the
   shape of `fetchFeedSyncEvents`. Keep the windowing parameters identical
   so all event sources share the same cutoff.
2. **Service grouping branch.** Add a new `FeedXxxEvent` struct alongside
   `FeedSyncEvent` / `FeedAgentSessionEvent` and a new `Type` constant on
   `FeedEvent`. Either fold it into `groupAnnotationsIntoEvents` (if it
   derives from annotations) or merge it into `ListFeedEvents` between the
   sync-events and grouped-events slice (if it has its own table). Update
   `filterFeedEvents` if it needs its own chip.
3. **Handler projection.** Add a `case "xxx"` branch to `projectFeedEvent`
   in [`internal/admin/feed.go`](../internal/admin/feed.go) that copies the
   service shape onto a templ-side `pages.FeedXxx` struct. Add the
   corresponding view-model type to
   [`feed_types.go`](../internal/templates/components/pages/feed_types.go).
4. **Templ renderer.** Add a `case "xxx"` arm to `feedRow` in
   [`feed.templ`](../internal/templates/components/pages/feed.templ).
   Always use `TimelineSystemRowCustomTile` (or `TimelineCommentRowRaw` for
   comment-shaped events) — never reach for a bespoke `<li>`. Author the
   tile and body sub-templates next to the existing
   `feedSyncTile` / `feedSyncBody` helpers. Run `templ generate` and
   commit the generated `_templ.go` siblings.
5. **Tests.** Add a unit test in
   [`internal/service/feed_test.go`](../internal/service/feed_test.go) that
   pins the grouping behaviour for the new type (in/out of window,
   threshold, dedup) and a templ-snapshot test in
   `internal/templates/components/pages/feed_xxx_test.go` that pins the
   rendered headline. The integration test in
   [`feed_integration_test.go`](../internal/service/feed_integration_test.go)
   should grow a fixture row for the new type so the end-to-end DB-backed
   path is covered.

If the new type also needs a hero-band tile, extend `FeedHero` and the
collection loop in `FeedHandler` — keep the loop single-pass so the page
stays one O(n) sweep over the projected items.

## Known caveats

- **Stacked-PR CI gap pre-#957.** Until #957 (`ci: run on stacked-PR base
  branches`) lands, only PRs targeting `main` get the full check matrix.
  Mid-stack PRs in the `stack/feed-polish/*` chain may show "no checks"
  even though their tip is green — verify locally with
  `go build ./... && go vet ./... && go test ./...` before requesting
  review on a mid-stack PR. This is a CI infrastructure caveat, not a
  Feed-specific bug.
- **Dark-mode lift is class-scoped.** PR #961 lifts the bb-card surface
  one tone in dark mode for WCAG contrast, scoped to `.bb-card` rather
  than the page wrapper. Other surfaces on `/feed` (the alert banner,
  filter chips, empty-state cards) inherit theme-default contrast and
  do not get the lift. If you add a new surface, decide explicitly
  whether it should follow `.bb-card` or stay at the default tone — do
  not assume the page-level lift will cover it.
- **`?before=` does not preserve hero stats.** "New transactions today"
  and "Events today" are computed from the rendered window, so paginating
  backward will show the totals for that older window, not for today.
  This is by design — the hero answers "what's in this view" — but it
  surprises first-time users.
- **`me` filter blanks for unlinked initial admin.** The initial admin
  with no linked household user has `SessionUserID == ""` and
  `SessionAccountID == ""` (the auth-account is not registered as the
  actor on its own annotations until a write happens). The handler
  silently downgrades to All in that case rather than rendering blank.
- **Reports are not part of `ListFeedEvents`.** The chip filter
  `reports` returns nil from the service layer; the handler loads
  reports separately via `ListAgentReports(ctx, 50)`. Tests that mock
  the service slice will not exercise the report path — use the
  handler-level integration test instead.
- **Sync samples can lag.** Sample transactions are fetched in a
  separate query and partitioned by `(connection_id, started_at +
  slack)`. A sync that completes after `FeedHandler` has already read
  `sync_logs` will show its own card with `AddedCount > 0` but an
  empty `SampleTransactions` slice. The next page render fills in.

## References

Source files (relative paths from repo root):

- [`internal/admin/feed.go`](../internal/admin/feed.go) — handler,
  projection, hero collection, day bucketing.
- [`internal/service/feed.go`](../internal/service/feed.go) — windowed
  reads, grouping pipeline, sync-error dedup, filter narrowing.
- [`internal/templates/components/pages/feed.templ`](../internal/templates/components/pages/feed.templ)
  — view rendering, hero band, alerts, filter chips, empty states,
  per-row dispatch, load-older affordance.
- [`internal/templates/components/pages/feed_types.go`](../internal/templates/components/pages/feed_types.go)
  — view-model types (`FeedProps`, `FeedHero`, `FeedItem`, etc.).
- [`internal/templates/components/timeline.templ`](../internal/templates/components/timeline.templ)
  — shared row chrome (`TimelineSystemRowCustomTile`,
  `TimelineCommentRowRaw`).
- [`internal/service/feed_test.go`](../internal/service/feed_test.go) —
  unit tests for grouping / dedup / window behaviour.
- [`internal/service/feed_integration_test.go`](../internal/service/feed_integration_test.go)
  — DB-backed end-to-end tests.
- `internal/templates/components/pages/feed_*_test.go` — templ-snapshot
  tests for hero, empty states, load-older, pending pill.

Per-iteration PRs (`stack/feed-polish/*`):

- #954 — `feat(feed): wire up filter chips on /feed page`
- #955 — `ui(feed): polish empty states with first-run / quiet / filtered variants`
- #956 — `test(feed): pin ListFeedEvents grouping/dedup/window behavior`
- #958 — `ui(feed): next-sync ETA sub-line under Last Sync hero tile`
- #959 — `ui(feed): add Load older activity pagination`
- #960 — `ui(feed): show pending pill on inline transaction-ref rows`
- #961 — `ui(feed): lift card surfaces in dark mode for WCAG contrast`

CI infrastructure:

- #957 — `ci: run on stacked-PR base branches` (gates full CI on stacked
  PRs; merge before relying on green checks for mid-stack tips).
