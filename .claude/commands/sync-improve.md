---
description: Autonomous sync engine improvement iteration for Breadbox
---

You are an autonomous agent improving the Breadbox sync engine. Each iteration you pick ONE focused improvement, implement it, and ship a PR for review. Schema changes, new migrations, service layer changes, and UI changes are all allowed.

## Sync Engine Architecture (read this first)

### End-to-End Flow
1. **Triggers**: Cron (15min check), manual, webhook (Plaid/Teller), startup
2. **Per-connection mutex** (`sync.Map` + `TryLock`) prevents concurrent syncs
3. **Pagination loop**: Calls `provider.SyncTransactions(ctx, conn, cursor)` until `HasMore=false`
4. **Atomic DB transaction**: Processes removed/added/modified txns in single commit
5. **Category resolution**: Rules first (priority-ordered), then category_mappings, then `uncategorized`
6. **Review queue**: Auto-enqueues uncategorized/low-confidence/new transactions
7. **Post-sync**: Flush rule hit counts, reconcile account links, update connection status
8. **Sync log**: Records status, counts, errors, timing

### Key Files
- `internal/sync/engine.go` (564 lines) — Main orchestration
- `internal/sync/scheduler.go` — Cron scheduling, staleness checks
- `internal/sync/matcher.go` — Account link transaction matching
- `internal/sync/rule_resolver.go` — Category rule evaluation
- `internal/sync/review.go` — Review queue enqueue logic
- `internal/provider/provider.go` — Provider interface
- `internal/provider/plaid/sync.go` — Plaid cursor-based sync
- `internal/provider/teller/sync.go` — Teller date-range polling
- `internal/service/sync.go` — TriggerSync, ListSyncLogs
- `internal/admin/sync_logs.go` — Dashboard sync log views
- `internal/templates/pages/sync_logs.html` — Sync log UI

### Provider Differences
- **Plaid**: Cursor-based incremental. Retry on `SYNC_MUTATION_DURING_PAGINATION`. Positive=debit.
- **Teller**: Date-range polling with 10-day overlap. Negates amounts. Stale pending cleanup. 250/page. Initial=2yr lookback.

### Current Sync Log Schema
```sql
sync_logs: id, connection_id, trigger, added_count, modified_count, removed_count,
           status (in_progress/success/error), error_message, started_at, completed_at
```

### Category Resolution Pipeline
1. Transaction rules (JSONB condition tree, priority-ordered, compiled regexes)
2. Category mappings table (`provider:category_string` -> category_id)
3. Fallback: `uncategorized`

## Known Improvement Areas

### Reliability
- TryLock silently skips if sync running — no retry queue or backoff
- Balance fetch is best-effort inside sync — failures silently logged
- Rule resolver load failure silently degrades to mappings-only
- Matcher runs outside the main DB transaction — partial reconciliation possible

### Observability / Sync Logs
- Sync logs only capture basic counts — no per-account breakdown
- No tracking of which rules fired, which categories were resolved
- No sync duration tracking (only started_at/completed_at, duration must be calculated)
- No historical failure tracking per connection (e.g., consecutive failures)
- No way to see what changed in a sync (which transactions were added/modified)
- Sync log detail page is minimal — could show much more useful information

### Performance
- Account ID resolution does per-transaction lookups (could batch/cache)
- Rule evaluation is linear search by priority (could optimize for large rule sets)
- Matcher does per-transaction candidate queries (could batch)

### Error Handling
- Per-transaction errors are logged but don't surface to users
- No retry mechanism for transient provider errors beyond Plaid's built-in retry
- No alerting when a connection fails multiple syncs in a row

### Testing
- No integration tests for Teller sync path
- No tests for concurrent sync collisions
- No chaos testing (mid-pagination failures, DB connection loss)

### Dashboard / UI
- Sync log detail page could show per-account breakdown
- No way to see sync history trends (success rate, avg duration over time)
- Connection cards could show more sync health info
- No "what changed" view for a specific sync run

## Phase 1: Discovery

```bash
gh pr list --label auto-qa --state all --limit 10 --json number,title,state,headRefName
git log main --oneline -15
```

Read the relevant source files to understand current state. Cross-reference with docs:
- `docs/architecture.md`, `docs/data-model.md`, `docs/teller-integration.md`
- `docs/category-mapping.md`, `docs/ROADMAP.md`

## Phase 2: Choose ONE Improvement

Pick the improvement that will have the most impact. Consider:
- Does it improve reliability? (preventing data loss or corruption)
- Does it improve observability? (helping users understand what happened)
- Does it improve the user experience? (sync log pages, connection health)
- Is it self-contained enough to ship in one PR?

## Phase 3: Implement

### Setup
```bash
git checkout main && git pull origin main
git checkout -b sync-improve/<short-descriptive-name>
```

### Guidelines
- Schema changes are allowed — use timestamp-based migration names: `date -u +%Y%m%d%H%M%S`
- After adding a migration, run `sqlc generate` then `go build ./...`
- Service layer changes are expected
- UI changes to sync log pages are welcome
- Run `make css` if you changed CSS
- Keep each PR focused on ONE improvement

### Build and Test
```bash
go build ./...
go test ./...
# If you wrote integration tests:
make test-integration
# Start the app:
lsof -ti:8080 | xargs kill 2>/dev/null; sleep 1
nohup make dev > /tmp/breadbox.log 2>&1 &
sleep 6
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/health/ready
```

## Phase 4: Screenshot (if UI changes)

```bash
PAGE_NAME="<descriptive-name>"
osascript <<APPLESCRIPT
tell application "Google Chrome"
    set winCount to count of windows
    repeat with i from 1 to winCount
        set w to window i
        set tabCount to count of tabs of w
        repeat with j from 1 to tabCount
            if title of tab j of w contains "Breadbox" then
                set active tab index of w to j
                set index of w to 1
                activate
                delay 2
                do shell script "screencapture -x /tmp/app-screenshot-${PAGE_NAME}.png"
                return "Captured"
            end if
        end repeat
    end repeat
end tell
APPLESCRIPT

FILE_SIZE=$(stat -f%z /tmp/app-screenshot-${PAGE_NAME}.png)
if [ "$FILE_SIZE" -gt 1000000 ]; then
    sips -Z 1400 /tmp/app-screenshot-${PAGE_NAME}.png --out /tmp/app-screenshot-${PAGE_NAME}.png
fi

IMG_URL=$(curl -s -X POST https://img402.dev/api/free -F image=@/tmp/app-screenshot-${PAGE_NAME}.png | grep -o '"url":"[^"]*"' | cut -d'"' -f4)
echo "Screenshot URL: $IMG_URL"
```

## Phase 5: Submit PR (do NOT merge)

```bash
git add -A
git commit -m "<concise description>

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"

git push -u origin sync-improve/<your-branch-name>

gh pr create \
  --title "<short title under 70 chars>" \
  --label "sync-improvement" \
  --base main \
  --body "## Summary
<what you improved and why — 2-3 bullets>

## Changes
<detailed description of what changed: schema, service layer, UI, etc.>

## Testing
<what tests you added or ran>

## Screenshots
<if applicable>

🤖 Generated by autonomous sync improvement agent"
```

**Do NOT merge the PR. Leave it open for review.**

## Rules

- ONE focused improvement per run
- Cross-reference docs before making changes
- Schema changes require migrations with `YYYYMMDDHHMMSS` prefix
- After migrations: `sqlc generate` then `go build ./...`
- The app must compile and run after changes
- Don't break existing sync functionality
- Write tests for new code paths
- Update CLAUDE.md or docs if you establish new patterns
- **Do NOT merge PRs** — leave open for manual review
