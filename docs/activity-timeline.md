# Activity Timeline

The activity timeline is the GitHub-style vertical-rail event log on the
transaction detail page. It is the canonical visualisation of a transaction's
audit trail — every comment, tag change, category set, rule application, and
sync event for a single transaction renders as a row threaded onto a continuous
left rail, with a composer at the bottom for typing the next comment.

This document captures the rendering contract, CSS invariants, dedup rules,
soft-delete behaviour, optimistic-update strategy, and the recipe for adding a
new system-event kind. It exists so future timeline-shaped surfaces (sync-log
detail, agent-run logs, etc.) can reuse the same component without
rediscovering the gotchas baked into the transaction detail implementation.

Cross-references to the canonical specs:

- Schema and enums: `docs/data-model.md` (annotation kinds live in the
  `annotations.kind` CHECK constraint).
- Service-layer conventions for the `service.Annotation` shape:
  `.claude/rules/service.md`.
- UI invariants for daisyUI / Tailwind / Alpine (rail-agnostic):
  `docs/design-system.md` and `.claude/rules/ui.md`.

## Where it's used

- `/transactions/{id}` — the activity card under the main transaction body.
  This is the canonical implementation and the only consumer today.
- Future: sync-log detail and agent-run logs are the obvious reuse targets.
  They share the same row-on-rail shape (system-attributed events, occasional
  human comments, day grouping) and should reuse the same `txdTimeline*`
  primitives rather than fork them. See "Future extensions" below.

## Rendering contract

### `service.Annotation` -> `service.ActivityEntry`

`service.Annotation` (in `internal/service/annotations.go`) is the canonical DB
projection of a single timeline event:

- Structural columns: `Kind`, `ActorType`, `ActorID`, `ActorName`, `CreatedAt`,
  `Payload`, `TagID`, `RuleID`, `IsDeleted`.
- Derived fields populated by `EnrichAnnotations`: `Action`, `Summary`,
  `Subject`, `Origin`, `Source`, `Note`, `Content`, `TagSlug`,
  `CategorySlug`, `RuleName`.
- `ActorAvatarVersion` — unix timestamp of `users.updated_at` for the actor,
  used as a `?v=<ts>` cache-buster on avatar URLs.

`service.ActivityEntry` (defined alongside `Annotation`) is the UI projection
the templ component reads. The admin handler converts each enriched
`Annotation` to an `ActivityEntry` in `activityEntryFromAnnotation`
(`internal/admin/transactions.go`). The conversion adds presentation-only
fields the service layer doesn't know about: `TagColor`, `CategoryColor`,
`CategoryIcon`, `ReviewStatus`, and a normalised `Type` value used for the
`if e.Type == "..."` branches in the templ.

Today's `Type` values:

| Type       | Source kind(s)                   | Renderer                |
|------------|----------------------------------|-------------------------|
| `comment`  | `comment`                        | `txdTimelineComment`    |
| `tag`      | `tag_added`, `tag_removed`       | `txdTimelineSystem`     |
| `category` | `category_set`                   | `txdTimelineSystem`     |
| `rule`     | `rule_applied`                   | `txdTimelineSystem`     |
| `sync`     | `sync_started`, `sync_updated`   | `txdTimelineSystem`     |
| `review`   | (legacy, retained for fallback)  | `txdTimelineSystem`     |

`syncEntryType` collapses both DB-level `sync_*` kinds onto a single `sync`
type because they share an icon (`refresh-cw`) and the differentiated verb
already lives on the `Summary` string.

`IsDeleted` is preserved on the comment entry — it gates the tombstone branch
in `txdTimelineComment` (see "Comment soft-delete" below).

Unknown / unrecognised kinds are dropped at this layer:
`activityEntryFromAnnotation` returns `(zero, false)` for kinds it doesn't
recognise. The caller skips them. This keeps the timeline forward-compatible
with kinds added by other workers as long as the renderer is updated in the
same release.

### Day grouping

`groupActivityByDay` (in `internal/admin/transactions.go`) groups the sorted
entry list into per-day buckets in the server's local timezone. It expects the
input slice to already be sorted **ascending** (oldest first) — that is what
`buildActivityTimeline` produces:

```go
sort.Slice(entries, func(i, j int) bool {
    return entries[i].Timestamp < entries[j].Timestamp
})
```

The composer sits at the **bottom** of the timeline, so newer events appear
where the user typed them. This is the inverse of the convention many activity
feeds use; the choice is deliberate — chat-style "new at the bottom" reads
better when the primary interaction is composing a comment.

Each bucket emits an `ActivityDayGroup`:

```go
type ActivityDayGroup struct {
    Date   string                  // ISO date, e.g. "2026-04-16"
    Label  string                  // "Today", "Yesterday", "Thursday, April 16"
    Events []service.ActivityEntry // newest first within the day, ASC overall
}
```

Day labels (`activityDayLabel`):

- Same calendar day as `now`        -> `Today`
- Previous calendar day             -> `Yesterday`
- Same year, older                  -> `Monday, April 14`
- Different year                    -> `Monday, April 14, 2025`

Entries with unparseable timestamps are silently dropped rather than
mis-bucketed. The few that hit this path are bugs in the writer; surfacing
them on the timeline as "no timestamp" rows would be worse than dropping.

The composer renders after all day groups via `txdTimelineComposer`. When
there are no entries at all, `txdTimelineEmptyComposer` takes over and
removes the `<ol>` wrapper entirely so we don't render an empty rail.

### Shared `now` anchor

`TransactionDetailProps.Now` is the single `time.Time` captured at the top of
`TransactionDetailHandler`:

```go
now := time.Now()
activity := buildActivityTimeline(annotations, ...)
activityDays := groupActivityByDay(activity, now)
// ... props.Now = now
```

Both day-bucket labels (`Today` / `Yesterday`) **and** per-row relative
timestamps read from the same anchor. This matters whenever a render begins
just before midnight and the bottom of the page paints just after — without a
shared anchor, the day group says `Yesterday` while a row inside it says
`5 minutes ago`, or vice versa.

The cross-cutting helper is `timefmt.RelativeAt(t, now)` in
`internal/timefmt/timefmt.go` — a pure function with the same `now` anchor
threaded in. Buckets:

- `< 1 minute` -> `just now`
- `< 1 hour`   -> `N minute(s) ago`
- `< 1 day`    -> `N hour(s) ago`
- `>= 1 day`   -> `N day(s) ago`

The templ component calls a thin wrapper, `relativeTimeStrAt(s, now)`, which
parses the RFC3339 string off the entry and delegates to `RelativeAt`. Future
timeline surfaces should follow this exact pattern: capture one `now` in the
handler, thread it through both grouping and per-row formatting, and never
call `time.Now()` from inside the templ.

## CSS / Tailwind invariants

The rail and rows are tuned together; changing one usually breaks the other.
The invariants below are what makes every icon centre sit dead-on the rail.

### The rail

```html
<span class="absolute left-[28px] sm:left-[32px] top-2 bottom-12 w-px bg-base-200" aria-hidden="true"></span>
```

- `left-[28px]` (mobile) / `sm:left-[32px]` (>=640px) is **container padding +
  tile half-width**: the `<ol>` has `pl-4 sm:pl-5` (16px / 20px) and every
  row's icon tile is 24px wide (half = 12px). 16 + 12 = 28; 20 + 12 = 32. If
  you change the container padding or the tile size, the rail's `left`
  offsets must change in lockstep.
- `top-2 bottom-12` anchors the rail so it threads through every row tile and
  through the composer below — but stops short of the bottom edge so it
  doesn't visually escape the card.
- `w-px bg-base-200` keeps the rail at 1px regardless of zoom, in a colour
  that adapts to dark mode automatically.

### Row geometry

Every row's first child is a 24px opaque tile:

```html
<div class="relative z-10 shrink-0 w-6 h-6 rounded-full bg-base-100">
  ...icon...
</div>
```

`bg-base-100` is critical: the tile is **opaque** by design so it visually
masks the rail behind it, producing the "rail enters the tile, doesn't
exit" silhouette that gives the timeline its rhythm. Inside the tile the
icon span itself adds `ring-4 ring-base-100` to draw a soft halo around the
coloured pill — that ring is what hides the rail seam without an extra
masking element.

The text container next to the tile uses `leading-6` (24px line-height):

```html
<div class="flex-1 min-w-0 text-xs leading-6 text-base-content/75 break-words">
  ...sentence...
</div>
```

A 24px line-box gives the first line of text the same vertical centre as the
24px icon tile next to it, so the actor name reads horizontally aligned with
the rail circle without per-element margin tweaks. Comment bubbles preserve
this on the meta line for the same reason.

### Day separators

```html
<li class="relative flex items-center gap-3 select-none ..." role="separator" aria-label="...">
  <div class="relative z-10 shrink-0 w-6 h-6 flex items-center justify-center" aria-hidden="true">
    <span class="w-3 h-3 rounded-full bg-base-300 ring-4 ring-base-100"></span>
  </div>
  <h3 class="text-xs font-semibold uppercase tracking-wider text-base-content/40">{ label }</h3>
  <span class="flex-1 h-px bg-base-200" aria-hidden="true"></span>
</li>
```

Note the dot lives inside the same 24px tile geometry as every other row — it
sits on the rail by construction, not by a manual `top: ...px` tweak. The
first day's separator gets `pt-1 pb-3`; later days get `pt-5 pb-3` so groups
breathe.

### Tooltips & relative timestamps

Relative timestamps wrap a daisyUI tooltip with the absolute time as the tip:

```html
<span class="tooltip tooltip-top" data-tip={ formatDateTimeStr(e.Timestamp) }>
  <time
    datetime={ e.Timestamp }
    class="tabular-nums cursor-default hover:underline underline-offset-2 decoration-base-content/30"
    title={ formatDateTimeStr(e.Timestamp) }>
    { relativeTimeStrAt(e.Timestamp, now) }
  </time>
</span>
```

The hover affordances — `hover:underline underline-offset-2
decoration-base-content/30 cursor-default` — exist to communicate "this is
hover-y" without pointer-fingering it as a link. `tabular-nums` keeps numeric
widths stable so neighbouring rows don't jitter when their relative-time
string changes length.

## Dedup contract (`service.EnrichAnnotations`)

Enrichment is a pure transformation in `internal/service/annotations_enrich.go`
that runs on every list of annotations before they reach the UI (or MCP). It
does three things:

1. **Drop rule-source structural rows.** When a rule fires during sync, the
   sync engine writes a `rule_applied` annotation **and** a structural
   side-effect row (`tag_added` / `category_set`) carrying
   `payload.source = "rule"`. The `rule_applied` row is the canonical audit
   record; its rule-source siblings are noise. `isRuleSourceDuplicate`
   filters them out. Comments and `rule_applied` itself are never deduped
   here — only structural side-effects flagged with `source: "rule"`.

2. **Drop adjacent same-actor comment-vs-tag-note duplicates.** The MCP
   `update_transactions` tool can write a `tag_added` with `payload.note`
   alongside a standalone `comment` with the same body, both in one call.
   The note already inlines the rationale on the tag row; the parallel
   comment is redundant. `isCommentDuplicateOfTagNote` collapses them within
   a 2-second window when actor identity matches (preferring `ActorID`,
   falling back to `ActorName` for system actors).

3. **Compute derived fields.** Action, Summary, Subject, Origin, Source,
   Note, Content, TagSlug, CategorySlug, RuleName are all derived per kind
   in `enrichOne`. Unknown kinds round-trip with empty derived fields rather
   than being dropped — keeps the timeline forward-compatible with new kinds
   that haven't shipped a UI branch yet.

### Tombstones never fold

This is the PR 4 invariant: **a soft-deleted comment is never deduped away**.
Even if the same actor wrote a `tag_added.note` adjacent to a now-tombstoned
`comment`, the tombstone survives because it carries audit value of its own
("Alice deleted a comment at 14:32"). The check lives at the top of step 2:

```go
if src.Kind == "comment" && !src.IsDeleted && isCommentDuplicateOfTagNote(in, src) {
    continue
}
```

If you find yourself extending dedup logic, preserve this invariant.
Tombstones are forensic; they exist precisely because the body is gone.

## Comment soft-delete (tombstones)

PR 4 of activity-log v2 introduced soft-delete for transaction comments.
Salient points:

- The DB row is **not** removed. `annotations.deleted_at` is set; the rest of
  the row stays put.
- `annotationFromActorRow` reads `DeletedAt.Valid` and sets `IsDeleted = true`
  on the projected `Annotation`.
- Enrichment overrides `Summary` to the tombstone phrase
  (`formatDeletedCommentSummary`: `"<Actor> deleted a comment"` or
  `"Comment deleted"` for anonymous actors), and clears the original body
  (`a.Detail = ""` in the admin mapper) so the retired content never
  re-renders.
- The `CommentID` short_id stays populated even on tombstones — the optimistic
  update path uses it to identify which bubble to swap. The bubble's trash
  button is gated on `CommentID != "" && !IsDeleted`, so retaining the ID
  here doesn't accidentally re-surface the delete affordance.
- The templ entry point `txdTimelineComment` branches at the top:

  ```go
  templ txdTimelineComment(e service.ActivityEntry, now time.Time) {
      if e.IsDeleted {
          @txdTimelineDeletedComment(e, now)
      } else {
          @txdTimelineCommentBubble(e, now)
      }
  }
  ```

  The deleted variant mirrors the system-row layout (24px tile, single
  muted line) so a tombstoned comment reads like a system event preserving
  who removed what and when, without re-displaying retired content.

## Optimistic in-place updates

PR 5 of activity-log v2 replaced the post-mutation `location.reload()` with
in-place row inserts. The strategy is documented at the top of
`static/js/admin/components/transaction_detail.js`; the contract:

### Strategy A — server-rendered partials

- The server is the **single source of truth for row markup**. There is no
  client-side row template in JS.
- After every mutation (POST/PATCH/DELETE), the JS `GET`s
  `/-/transactions/{id}/timeline/rows?since=<lastTs>` to fetch the rendered
  HTML for the rows that were just written.
- The handler reuses `buildActivityTimeline` and the same `txdTimelineDay`
  / `txdTimelineSystem` / `txdTimelineComment` templ helpers as the main
  page render, so partial rows are byte-equivalent to the full-page render.
  No drift, no parallel renderer to keep in sync.

### `GET /-/transactions/{id}/timeline/rows`

`TimelineRowsHandler` (`internal/admin/transactions.go`) accepts:

- `since` (RFC3339) — return entries with `Timestamp > since`. An empty or
  missing `since` returns no entries (sentinel for "first load — page
  already has every row").
- `comment_ids` — comma-separated list of comment short_ids. Comments in this
  set are returned **even when their `Timestamp` is older than `since`**.
  This is the soft-delete tombstone path: `is_deleted` flips on an existing
  row, the row's `CreatedAt` stays in the past, and the JS asks the endpoint
  to render the tombstone variant for that specific ID.

The response is `text/html` with `<li>` rows (and an optional preceding
`<li>` day separator when the new rows fall on a different calendar day from
the most recent prior entry — or when the page had no prior entries). The
JS unwraps the fragment and inserts each `<li>` immediately before the
composer.

The `data-last-activity-ts` attribute on the `#activity` section seeds the
cursor; `txdLastActivityTimestamp` reads it from the last entry of
`p.Activity` so the JS has a starting point on first paint.

### `restorePageState()` rollback

The base layout's SPA progress bar auto-starts on internal link clicks and
fades the main content (opacity / blur / pointer-events). Any async error
path **must** clear that state — otherwise the page stays blurred and the
trickling progress bar never finishes.

The convention (see `.claude/rules/ui.md` "SPA progress bar"): every Alpine
component defines or shares a `restorePageState()` helper at module scope and
calls it on every error / non-2xx branch. The transaction-detail JS shares a
single module-level implementation across all three factories
(category, tags, comments).

In addition to clearing the SPA fade, each call site rolls back the
optimistic local chip state at the call site so the UI reflects the prior
state (category chip reverts, tag chips re-add, etc.) and surfaces a toast
via `window.showToast`.

## Adding a new system-event kind

Worked example: `sync_started` and `sync_updated` from PR 6 of the
activity-log v2 stack. To add a new kind, work top-to-bottom through these
six steps.

### 1. Migration: extend the `annotations.kind` CHECK constraint

`internal/db/migrations/<timestamp>_<name>.sql` — drop and re-add the
constraint with the new kind. Goose wraps each migration in a transaction by
default, so the table is never without a constraint mid-run.

```sql
-- +goose Up
ALTER TABLE annotations DROP CONSTRAINT IF EXISTS annotations_kind_check;
ALTER TABLE annotations ADD CONSTRAINT annotations_kind_check
  CHECK (kind IN (
    'comment',
    'rule_applied',
    'tag_added',
    'tag_removed',
    'category_set',
    'sync_started',
    'sync_updated',
    'your_new_kind'
  ));

-- +goose Down
-- (mirror the up direction without your new kind)
```

This is an **additive** migration in the shared-DB sense (see
`.claude/rules/migrations.md`) — adding a new accepted value doesn't break
older `breadbox serve` processes; they'll just never write the new kind. Run
`sqlc generate` afterward; the generated code rarely changes for CHECK-only
edits.

### 2. Emit: write the row inside the originating transaction

Mirror the helper-shape patterns:

- `internal/ruleapply/annotations.go` for rule-attributed writes
  (`WriteRuleApplied`, `WriteCategorySet`).
- `internal/sync/annotations.go` for sync-attributed writes
  (`writeSyncStartedAnnotation`, `writeSyncUpdatedAnnotation`).

Each helper takes a `pgx.Tx` so the annotation insert commits atomically with
the originating action. Don't write annotations in a separate transaction —
the sync engine relies on "either everything commits or nothing does"
(`.claude/rules/sync.md`).

The canonical payload pattern: serialise actor metadata into the JSON payload
so the consumer can cross-link without parsing the untyped map. Example from
`writeSyncUpdatedAnnotation`:

```go
payload := map[string]any{
    "provider":      providerType,
    "connection_id": connShortID,
    "sync_log_id":   syncLogShortID,
    "status_change": map[string]any{
        "from": pendingLabel(fromPending),
        "to":   pendingLabel(toPending),
    },
}
```

`actor_type = "system"` for engine-driven kinds; `actor_id` should hold the
**short_id** of the closest entity (a connection short_id for sync events; a
rule short_id for rule events). `actor_name` should be the human-readable
display string the timeline will surface (`"Plaid"`, `"Teller"`, `"CSV
import"` for sync; the rule name for rule events).

### 3. Enrich: add a Summary branch in `EnrichAnnotations`

In `internal/service/annotations_enrich.go`, extend the `enrichOne` switch
with a case for the new kind and a `format<Kind>Summary` helper:

```go
case "your_new_kind":
    a.Action = "your-verb"
    a.Subject = a.ActorName
    a.Summary = formatYourNewKindSummary(a.ActorName, ...)
```

The Summary is the canonical sentence both MCP consumers and the templ
fallback branch read. Keep it self-contained — agents that look at MCP
output don't read `Origin` separately, so trailing qualifiers belong in the
sentence ("during sync", " · pending → posted", etc.) when relevant.

### 4. Render: add a `txdSystemIcon` branch and confirm the sentence

In `internal/templates/components/pages/transaction_detail.templ`, extend
`txdSystemIcon` with the new event type (Lucide icon name, tile background,
ring colour):

```go
} else if e.Type == "your_new_kind" {
    <span class="flex items-center justify-center w-6 h-6 rounded-full bg-base-200 ring-4 ring-base-100" aria-hidden="true">
        <i data-lucide="your-icon" class="w-3 h-3 text-base-content/60"></i>
    </span>
}
```

Pick a Lucide name that already appears in the codebase when possible —
`refresh-cw` for sync, `zap` for rules, etc. New icons are fine but check
`docs/design-system.md` -> "Icons" first.

For the sentence, decide whether your kind should:

- Render its prebuilt `Summary` directly (the `txdSystemSentence` fallback
  branch — same path used by `rule` and `sync`); or
- Compose its own actor-verb-object phrase like `tag` and `category` do.

For service-emitted system events with a static phrase, prefer the Summary
path — keeps the sentence consistent across MCP and the UI.

Then map the DB kind to the `Type` value the templ branches on. For sync
events both `sync_started` and `sync_updated` collapse to `"sync"` via
`syncEntryType` because they share an icon. If your two kinds need
**different** icons, give them distinct `Type` values.

Finally, regenerate templ:

```bash
templ generate
```

and commit both `*.templ` and the generated `*_templ.go` siblings.

### 5. MCP: add the kind to `mcpAnnotationKinds`

`internal/mcp/tools_tags.go`:

```go
var mcpAnnotationKinds = map[string][]string{
    "comment":  {"comment"},
    "rule":     {"rule_applied"},
    "tag":      {"tag_added", "tag_removed"},
    "category": {"category_set"},
    "sync":     {"sync_started", "sync_updated"},
    "your":     {"your_new_kind"},
}
```

The map's keys are the **agent-facing** names (one normalised name plus an
`action` field on each row); the values are the raw DB kinds the service
filters by. Keep the boundary narrow — agents shouldn't have to know about
`tag_added` vs `tag_removed`, just `tag` plus a verb.

### 6. Test

Pair each new kind with at least one integration test that proves the row
appears end-to-end. The existing reference is
`internal/sync/sync_annotations_integration_test.go` — drives a real sync,
asserts that `sync_started` and `sync_updated` rows are written, and that
`ListAnnotations` returns them with the expected Summary.

Also pin a unit test on the enrichment branch
(`internal/service/annotations_enrich_test.go`) so future refactors can't
silently break the Summary string.

## Future extensions

- **Sync-log detail page.** Each connection sync produces a `sync_logs` row
  with a structured outcome. A sync-log detail view could reuse this
  component to thread the connection's lifecycle (started, succeeded, errored,
  reauth-required, etc.) onto the same rail.
- **Agent-run logs.** When an MCP agent runs a workflow (review queue,
  bulk recategorize), the sequence of writes is already captured as
  annotations — surfacing them as a per-run timeline is mostly a routing /
  filtering problem, not a rendering one.
- **Cross-transaction activity feed.** A household-wide "what happened
  recently" page would need scrolling and pagination, but the per-day
  grouping and row markup are reusable.

When promoting `txdTimeline*` to a shared component, extract them into
`internal/templates/components/timeline/` with a context-agnostic prop set
(no `TransactionID` baked in) and have the transaction detail page wrap
them. Don't refactor speculatively — wait for the second concrete consumer.

## See also

- `internal/admin/transactions.go` — `buildActivityTimeline`,
  `groupActivityByDay`, `activityDayLabel`, `activityEntryFromAnnotation`,
  `txdLastActivityTimestamp`, `TimelineRowsHandler`.
- `internal/templates/components/pages/transaction_detail.templ` — `txdActivity`,
  `txdTimelineDay`, `txdTimelineSystem`, `txdSystemSentence`, `txdSystemIcon`,
  `txdTimelineComment`, `txdTimelineCommentBubble`, `txdTimelineDeletedComment`,
  `txdTimelineComposer`, `TimelineRows`.
- `internal/service/annotations.go` — `Annotation` shape, `ListAnnotations`,
  `IsDeleted` plumbing.
- `internal/service/annotations_enrich.go` — `EnrichAnnotations`, dedup
  rules, per-kind Summary helpers.
- `internal/ruleapply/annotations.go` — rule-attributed write helpers.
- `internal/sync/annotations.go` — sync-attributed write helpers.
- `internal/timefmt/timefmt.go` — `RelativeAt` (the page-shared `now`-anchor
  relative-time formatter).
- `static/js/admin/components/transaction_detail.js` — Strategy A optimistic
  updates, `restorePageState` rollback.
- `internal/mcp/tools_tags.go` — `mcpAnnotationKinds` mapping.
- `.claude/rules/ui.md` — admin UI / Tailwind / Alpine invariants.
- `.claude/rules/sync.md` — atomicity rules for sync-driven writes.
- `.claude/rules/migrations.md` — additive-only migration safety in shared
  dev DB.
