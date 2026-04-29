package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5/pgtype"
)

// FeedActivityRow is a global activity entry — an annotation joined with
// transaction context (merchant, amount, currency, account, institution) so
// the home Feed page can render rich cards for events that happened on any
// transaction without re-fetching transaction details per row.
//
// FeedActivityRow is the low-level read helper. The feed page itself reads
// `FeedEvent`s from `ListFeedEvents`, which applies grouping on top.
type FeedActivityRow struct {
	Annotation Annotation

	TransactionShortID string
	TransactionName    string
	MerchantName       string
	Amount             float64
	IsoCurrencyCode    string
	TransactionDate    string
	Pending            bool

	AccountName     string
	InstitutionName string

	// Category presentation, threaded through so feed sample rows can
	// render the same coloured-circle avatar as /transactions. Nil when
	// uncategorised. Mirrors the LEFT JOIN on `categories` performed by
	// every feed query.
	CategoryDisplayName *string
	CategoryColor       *string
	CategoryIcon        *string
	CategorySlug        *string
}

// ListFeedActivity returns the most recent annotations across every
// transaction, joined with merchant/amount/account context. Pre-existing
// helper kept for any caller that wants the raw annotation feed; the home
// Feed page itself goes through `ListFeedEvents`.
func (s *Service) ListFeedActivity(ctx context.Context, limit int) ([]FeedActivityRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}

	const q = `
SELECT
    a.id, a.short_id, a.transaction_id, a.kind, a.actor_type, a.actor_id, a.actor_name,
    a.payload, a.tag_id, a.rule_id, a.session_id, a.created_at,
    COALESCE(u_via_account.name, u_direct.name, '')::text AS actor_user_name,
    COALESCE(u_via_account.updated_at, u_direct.updated_at) AS actor_updated_at,
    t.short_id, t.provider_name, t.provider_merchant_name, t.amount, t.iso_currency_code, t.date, t.pending,
    ac.name, bc.institution_name,
    cat.display_name, cat.color, cat.icon, cat.slug
FROM annotations a
JOIN transactions t ON a.transaction_id = t.id
LEFT JOIN accounts ac ON t.account_id = ac.id
LEFT JOIN bank_connections bc ON ac.connection_id = bc.id
LEFT JOIN categories cat ON t.category_id = cat.id
LEFT JOIN auth_accounts aa
    ON a.actor_type = 'user'
   AND aa.id::text = a.actor_id
LEFT JOIN users u_via_account
    ON aa.user_id = u_via_account.id
LEFT JOIN users u_direct
    ON a.actor_type = 'user'
   AND aa.id IS NULL
   AND u_direct.id::text = a.actor_id
WHERE t.deleted_at IS NULL
  AND a.deleted_at IS NULL
ORDER BY a.created_at DESC
LIMIT $1
`

	rows, err := s.Pool.Query(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("list feed activity: %w", err)
	}
	defer rows.Close()

	out := make([]FeedActivityRow, 0, limit)
	for rows.Next() {
		row, err := scanFeedActivityRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate feed activity rows: %w", err)
	}

	out = enrichFeedActivityRows(out)
	return out, nil
}

// FeedEvent is the unit the Feed page renders — one row on the rail,
// possibly representing many underlying annotations that have been grouped
// together. Exactly one of the typed payload pointers is set; the rest are
// nil.
type FeedEvent struct {
	Type      string    // "sync" | "agent_session" | "bulk_action" | "comment"
	Timestamp time.Time // sortable wall-clock anchor for newest-first ordering

	Sync         *FeedSyncEvent
	AgentSession *FeedAgentSessionEvent
	BulkAction   *FeedBulkActionEvent
	Comment      *FeedCommentEvent
}

// FeedSyncEvent represents one sync run. The card shows the headline
// (`12 new transactions from Chase`), inline transaction samples (top
// `SampleLimit` by absolute amount), and any rule outcomes that fired
// during the sync (lifted from `sync_logs.rule_hits`).
//
// Failed syncs that repeat because of cron retries (same connection +
// same error message) collapse into a single FeedSyncEvent with the
// retry counters populated. The card displays "Has been failing for
// 18h · 49 attempts" instead of 49 identical rows.
type FeedSyncEvent struct {
	SyncLogID       string
	InstitutionName string
	Provider        string
	Trigger         string
	Status          string
	ErrorMessage    string

	AddedCount    int
	ModifiedCount int
	RemovedCount  int

	StartedAt   time.Time
	CompletedAt time.Time

	// RetryCount counts the additional same-error sync attempts that
	// were folded into this card. 0 = single attempt; N = total attempts
	// is RetryCount + 1. Only populated for failed syncs.
	RetryCount      int
	FirstFailureAt  time.Time

	SampleTransactions []FeedSampleTx
	AdditionalCount    int

	RuleOutcomes []FeedRuleOutcome
}

// FeedAgentSessionEvent represents one MCP agent session whose annotations
// are folded into a single card. An MCP review session that touches 50
// transactions and writes 200 annotations is one row here, not 200.
type FeedAgentSessionEvent struct {
	SessionID      string
	SessionShortID string
	APIKeyName     string
	Purpose        string

	ActorName          string
	ActorType          string
	ActorID            string
	ActorAvatarVersion string

	StartedAt time.Time
	EndedAt   time.Time

	AnnotationCount    int
	UniqueTransactions int

	// KindCounts breaks the session's annotations down by kind so the card
	// can render "categorised 23 · tagged 12 · commented 8" without the
	// caller iterating annotations a second time.
	KindCounts map[string]int

	SampleTransactions []FeedSampleTx

	// Report, when non-nil, is an agent_report whose session_id matches
	// this session. Populated by ListFeedEvents so a reporting agent run
	// surfaces as a single feed row headlined by the report's title — the
	// alternative (a separate report card adjacent to the session card) is
	// noisy on the timeline.
	Report *FeedReportRef
}

// FeedBulkActionEvent represents ≥ N annotations from the same actor
// within a soft time-bucket — typically a human running a bulk
// recategorisation or an agent making changes outside an MCP session.
//
// As of iteration 13 the soft bucket is keyed by (actor, time-window) only
// — the actor's `kind` is *not* part of the bucket key. A single agent run
// that categorises 5 rows, removes a tag from 8, and updates 8 more in the
// same 15-minute window collapses into one bulk_action card whose
// `KindCounts` map exposes the per-kind breakdown for inline rendering
// ("5 categorised · 8 tag-removed · 8 updated"). `Kind` is kept for the
// homogeneous-bucket code path: when every annotation in the bucket
// shares one kind the templ renders the dedicated verb/subject phrasing,
// otherwise it falls back to the generic breakdown line.
type FeedBulkActionEvent struct {
	ActorName          string
	ActorType          string
	ActorID            string
	ActorAvatarVersion string

	// Kind is the homogeneous kind of every annotation in this bucket, or
	// "mixed" when the bucket folds annotations across kinds.
	Kind  string
	Count int

	// KindCounts breaks the bucket's annotations down by kind so the card
	// can render the breakdown line ("5 categorised · 8 tag-removed · 8
	// updated") when the bucket spans multiple kinds.
	KindCounts map[string]int

	// Subjects is the deduped list of subject names (tag names, category
	// names, rule names) that were applied. When all rows share one
	// subject, the card renders "added the Groceries tag to 12
	// transactions"; with several subjects it renders the count per
	// subject ("Groceries: 8 · Transport: 4").
	Subjects []FeedBulkSubject

	StartedAt time.Time
	EndedAt   time.Time

	SampleTransactions []FeedSampleTx

	// Report, when non-nil, is an agent_report whose actor + window
	// matches this bucket. Folding the report in lets a reporting agent
	// run render as a single card headlined by the report title rather
	// than the generic "X categorised, Y tagged" line.
	Report *FeedReportRef
}

// FeedReportRef is the slim projection of an agent report folded into a
// bulk_action / agent_session card. The handler still owns the canonical
// report card (rendered when the report wasn't folded into anything) — this
// shape only carries enough to display the title + priority + tags inline
// and a link to the full report.
type FeedReportRef struct {
	ID       string
	ShortID  string
	Title    string
	Priority string
	Tags     []string
	IsUnread bool
}

// FeedBulkSubject is one (subject, count) pair inside a bulk-action card.
type FeedBulkSubject struct {
	Name  string
	Slug  string
	Color string
	Icon  string
	Count int
}

// FeedCommentEvent is one standalone comment — surfaced as its own row when
// it isn't part of an MCP session and isn't part of a bulk-action group.
type FeedCommentEvent struct {
	// CommentShortID is the annotation's short_id. Used by /feed to link
	// the verb to the matching comment row on /transactions/{id} via a
	// `#comment-<id>` fragment so a tap navigates straight to the body.
	CommentShortID     string
	ActorName          string
	ActorType          string
	ActorID            string
	ActorAvatarVersion string
	Content            string

	Transaction FeedSampleTx
}

// FeedSampleTx is the small projection used inside every event card to
// reference a transaction. Carries enough to render the merchant + amount
// pair without an extra fetch per card.
type FeedSampleTx struct {
	ShortID      string
	Name         string
	MerchantName string
	Amount       float64
	Currency     string
	Date         string
	AccountName  string
	Institution  string
	// Pending mirrors `transactions.pending` so the feed card can render
	// the same clock-icon pending mark used on `/transactions/{id}` and
	// the transactions list — preliminary rows often re-show as posted
	// later, and surfacing the lifecycle state inline gives users a
	// "this is provisional" read.
	Pending bool

	// Category presentation, matched to `tx_row_compact.templ`. Populated
	// when the underlying transaction is categorised so the inline tx-ref
	// row can render the same coloured circle avatar (icon when set; first-
	// letter fallback otherwise) as the /transactions list. Nil when
	// uncategorised — the templ falls back to a neutral letter avatar.
	CategoryDisplayName *string
	CategoryColor       *string
	CategoryIcon        *string
	CategorySlug        *string
}

// FeedRuleOutcome is one (rule, count) pair shown inline on a sync card.
type FeedRuleOutcome struct {
	RuleName    string
	RuleShortID string
	Count       int
}

// FeedEventsParams configures the window + grouping thresholds applied by
// `ListFeedEvents`. Zero values resolve to sensible defaults: 3-day window,
// soft-group threshold of 3, sample limit of 5.
type FeedEventsParams struct {
	Window        time.Duration
	BulkThreshold int
	SampleLimit   int

	// Before, when non-zero, anchors the window's *upper* bound at this
	// timestamp instead of "now". Used by the "Load older activity"
	// pagination affordance — the page rolls backward in `Window`-sized
	// chunks. The cutoff becomes `Before.Add(-Window)`. Default-window
	// behaviour (no `Before`) is preserved verbatim: events newer than
	// `now - Window` and up to `now`.
	Before time.Time

	// Filter scopes the resulting event slice to a single chip on the feed
	// rail. Empty string means "no filter" — every grouped event is
	// returned. See `filterFeedEvents` for the recognised values.
	Filter string

	// ActorID is consulted only when Filter == "me". The handler resolves
	// the session's user_id (falling back to the auth_account id for the
	// initial admin) before calling. Both forms are matched because
	// `annotations.actor_id` historically stores either depending on
	// whether the auth_account is linked to a household user.
	ActorID string
}

// FeedMaxLookback is the hard ceiling for `Before`-driven pagination on the
// home feed. A user can scroll backward in 3-day chunks but cannot ask for
// activity older than this — the unbounded read would walk the entire
// annotations table for households with years of history.
const FeedMaxLookback = 30 * 24 * time.Hour

// feedSoftBucketWindow is the wall-clock window used by the unsessioned
// soft-bucket grouping in `groupUnsessionedAnnotations`. An actor's
// annotations within one window collapse into a single bulk_action card.
//
// 15 minutes is wide enough to capture an agent run end-to-end (categorise
// → tag → comment → write report → leave) while still being narrow enough
// that a human editing throughout the day stays as distinct buckets per
// hour. Iteration 12 used 5 minutes which fragmented agent runs into 3-4
// adjacent cards on the rail; widening collapsed those into one card per
// run.
const feedSoftBucketWindow = 15 * time.Minute

// ListFeedEvents returns grouped FeedEvents for the home Feed page,
// already ordered newest-first.
//
// The grouping pipeline is:
//
//  1. Pull annotations from the window, dropping rule-applied rows that
//     fired during sync (`payload.applied_by == "sync"`) and the
//     per-transaction `sync_started` / `sync_updated` rows — both are
//     already represented by their parent sync card.
//  2. Hard-group annotations by `session_id`. Every session collapses
//     into a single AgentSession event regardless of how many annotations
//     it produced.
//  3. Soft-bucket the remaining (un-sessioned) annotations by
//     `(actor_id, floor(time / feedSoftBucketWindow))` — note that `kind`
//     is intentionally absent from the key so a single agent run that
//     categorises some rows and removes a tag from others collapses into
//     ONE bulk_action card with a per-kind breakdown. Buckets with
//     ≥ BulkThreshold events become BulkAction events; sub-threshold
//     buckets pass through standalone comments individually.
//  4. Sync logs from the window become Sync events with inline transaction
//     samples and `RuleOutcomes` lifted from `sync_logs.rule_hits`.
//  5. Agent reports inside the same window are folded into matching
//     bulk_action / agent_session events when the actor or session_id
//     lines up — a reporting agent run renders as one row headed by the
//     report's title instead of two adjacent cards. Reports that don't
//     match anything are returned in the consumed/unconsumed split for
//     the handler to render as standalone report cards.
//
// Connection alerts are returned by a separate handler-level query, not
// by this function.
func (s *Service) ListFeedEvents(ctx context.Context, params FeedEventsParams) ([]FeedEvent, error) {
	out, _, err := s.listFeedEventsWithReports(ctx, params, nil)
	return out, err
}

// ListFeedEventsWithReports is the report-aware variant the home Feed
// handler uses. It folds matching reports into bulk_action / agent_session
// events and returns the leftover (un-folded) reports separately so the
// handler can render them as standalone report cards.
//
// Splitting this from `ListFeedEvents` keeps the report-free entry-point
// available for callers that don't have a windowed report list handy
// (none today; preserved for symmetry with `ListFeedActivity`).
func (s *Service) ListFeedEventsWithReports(ctx context.Context, params FeedEventsParams, reports []AgentReportResponse) ([]FeedEvent, []AgentReportResponse, error) {
	return s.listFeedEventsWithReports(ctx, params, reports)
}

func (s *Service) listFeedEventsWithReports(ctx context.Context, params FeedEventsParams, reports []AgentReportResponse) ([]FeedEvent, []AgentReportResponse, error) {
	if params.Window <= 0 {
		params.Window = 3 * 24 * time.Hour
	}
	if params.BulkThreshold <= 0 {
		params.BulkThreshold = 3
	}
	if params.SampleLimit <= 0 {
		params.SampleLimit = 5
	}

	now := time.Now()
	// Clamp the upper bound to the lookback ceiling so callers asking for a
	// timestamp deeper than `FeedMaxLookback` silently get the oldest still-
	// addressable window. Keeps the handler's `?before=` clamp honoured at
	// the service layer too — defence in depth, since this is the layer
	// that actually owns the unbounded-query risk.
	upper := now
	if !params.Before.IsZero() {
		upper = params.Before
		minUpper := now.Add(-FeedMaxLookback)
		if upper.Before(minUpper) {
			upper = minUpper
		}
	}
	cutoff := upper.Add(-params.Window)

	annotationRows, err := s.fetchFeedAnnotations(ctx, cutoff, upper)
	if err != nil {
		return nil, nil, err
	}
	annotationRows = enrichFeedActivityRows(annotationRows)

	syncEvents, err := s.fetchFeedSyncEvents(ctx, cutoff, upper, params.SampleLimit)
	if err != nil {
		return nil, nil, err
	}

	sessionMeta, err := s.fetchFeedSessionMeta(ctx, annotationRows)
	if err != nil {
		s.Logger.Debug("fetch feed session meta", "error", err)
	}

	groupedEvents := groupAnnotationsIntoEvents(annotationRows, sessionMeta, params)

	// Fold matching reports into bulk_action / agent_session events. The
	// remainder is the standalone-card list returned to the handler.
	leftoverReports := foldReportsIntoEvents(groupedEvents, reports)

	out := make([]FeedEvent, 0, len(syncEvents)+len(groupedEvents))
	out = append(out, syncEvents...)
	out = append(out, groupedEvents...)

	sort.Slice(out, func(i, j int) bool {
		return out[i].Timestamp.After(out[j].Timestamp)
	})

	out = filterFeedEvents(out, params.Filter, params.ActorID)
	return out, leftoverReports, nil
}

// filterFeedEvents narrows a feed-event slice to the subset matching the
// chip-filter string. Applied after grouping so the windowed pull stays
// shared across filters and the slice is small enough that an in-memory
// sweep beats re-running the SQL.
//
// Filter values:
//   - ""           → no filter, returns input unchanged
//   - "syncs"      → only sync events
//   - "comments"   → only comment events
//   - "sessions"   → only agent_session events
//   - "reports"    → reports come from the handler, so ListFeedEvents
//                    contributes nothing in this mode
//   - "me"         → events authored by the supplied actorID; bulk-action
//                    rows match on the bucket actor as well. If actorID is
//                    empty (e.g. the initial admin without a linked user)
//                    the filter is treated as "no filter" so the page
//                    keeps rendering instead of going blank.
func filterFeedEvents(events []FeedEvent, filter, actorID string) []FeedEvent {
	switch filter {
	case "":
		return events
	case "reports":
		return nil
	case "syncs":
		return filterEventsByType(events, "sync")
	case "comments":
		return filterEventsByType(events, "comment")
	case "sessions":
		return filterEventsByType(events, "agent_session")
	case "me":
		if actorID == "" {
			return events
		}
		return filterEventsByActor(events, actorID)
	}
	return events
}

func filterEventsByType(events []FeedEvent, t string) []FeedEvent {
	out := make([]FeedEvent, 0, len(events))
	for _, ev := range events {
		if ev.Type == t {
			out = append(out, ev)
		}
	}
	return out
}

func filterEventsByActor(events []FeedEvent, actorID string) []FeedEvent {
	out := make([]FeedEvent, 0, len(events))
	for _, ev := range events {
		switch ev.Type {
		case "comment":
			if ev.Comment != nil && ev.Comment.ActorID == actorID {
				out = append(out, ev)
			}
		case "agent_session":
			if ev.AgentSession != nil && ev.AgentSession.ActorID == actorID {
				out = append(out, ev)
			}
		case "bulk_action":
			if ev.BulkAction != nil && ev.BulkAction.ActorID == actorID {
				out = append(out, ev)
			}
		}
	}
	return out
}

// fetchFeedAnnotations runs the windowed annotation query used by
// `ListFeedEvents`. Excludes annotations that are byproducts of a sync
// (`sync_started` / `sync_updated` rows and rule applications fired
// during sync — both are represented by the parent sync card).
//
// The user join mirrors `ListAnnotationsWithActorByTransaction`: it
// resolves user.name through either an auth_accounts row whose user_id
// links to users, or a direct users.id match against actor_id, and
// surfaces both the live profile name (preferred over the frozen-at-
// write-time `annotations.actor_name`) and the user's `updated_at` for
// avatar cache-busting.
func (s *Service) fetchFeedAnnotations(ctx context.Context, cutoff, upper time.Time) ([]FeedActivityRow, error) {
	const q = `
SELECT
    a.id, a.short_id, a.transaction_id, a.kind, a.actor_type, a.actor_id, a.actor_name,
    a.payload, a.tag_id, a.rule_id, a.session_id, a.created_at,
    COALESCE(u_via_account.name, u_direct.name, '')::text AS actor_user_name,
    COALESCE(u_via_account.updated_at, u_direct.updated_at) AS actor_updated_at,
    t.short_id, t.provider_name, t.provider_merchant_name, t.amount, t.iso_currency_code, t.date, t.pending,
    ac.name, bc.institution_name,
    cat.display_name, cat.color, cat.icon, cat.slug
FROM annotations a
JOIN transactions t ON a.transaction_id = t.id
LEFT JOIN accounts ac ON t.account_id = ac.id
LEFT JOIN bank_connections bc ON ac.connection_id = bc.id
LEFT JOIN categories cat ON t.category_id = cat.id
LEFT JOIN auth_accounts aa
    ON a.actor_type = 'user'
   AND aa.id::text = a.actor_id
LEFT JOIN users u_via_account
    ON aa.user_id = u_via_account.id
LEFT JOIN users u_direct
    ON a.actor_type = 'user'
   AND aa.id IS NULL
   AND u_direct.id::text = a.actor_id
WHERE t.deleted_at IS NULL
  AND a.deleted_at IS NULL
  AND a.created_at >= $1
  AND a.created_at <  $2
  AND a.kind NOT IN ('sync_started', 'sync_updated')
  AND COALESCE(a.payload->>'applied_by', '') <> 'sync'
ORDER BY a.created_at DESC
`

	rows, err := s.Pool.Query(ctx, q, cutoff, upper)
	if err != nil {
		return nil, fmt.Errorf("list feed annotations: %w", err)
	}
	defer rows.Close()

	out := make([]FeedActivityRow, 0, 256)
	for rows.Next() {
		row, err := scanFeedActivityRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate feed annotation rows: %w", err)
	}
	return out, nil
}

// scanFeedActivityRow is the shared row scanner used by
// `fetchFeedAnnotations` and `ListFeedActivity`. The `Scanner` interface
// keeps the shape independent of pgx-vs-rows specifics.
type scanner interface {
	Scan(dest ...any) error
}

func scanFeedActivityRow(s scanner) (FeedActivityRow, error) {
	var (
		id, txnID, tagID, ruleID, sessionID pgtype.UUID
		actorName, kind, actorType          string
		actorIDOpt                          pgtype.Text
		shortID                             string
		payload                             []byte
		createdAt                           pgtype.Timestamptz
		actorUserName                       string
		actorUpdatedAt                      pgtype.Timestamptz
		txShort, txName                     string
		merchantName                        pgtype.Text
		amount                              pgtype.Numeric
		isoCcy                              pgtype.Text
		txDate                              pgtype.Date
		pending                             bool
		accountName, institutionName        pgtype.Text
		catDisplay, catColor                pgtype.Text
		catIcon, catSlug                    pgtype.Text
	)

	if err := s.Scan(
		&id, &shortID, &txnID, &kind, &actorType, &actorIDOpt, &actorName,
		&payload, &tagID, &ruleID, &sessionID, &createdAt,
		&actorUserName, &actorUpdatedAt,
		&txShort, &txName, &merchantName, &amount, &isoCcy, &txDate, &pending,
		&accountName, &institutionName,
		&catDisplay, &catColor, &catIcon, &catSlug,
	); err != nil {
		return FeedActivityRow{}, fmt.Errorf("scan feed activity row: %w", err)
	}

	// Prefer the live users.name over the actor_name frozen at write time
	// (which is typically the auth_accounts.username/email login). Mirrors
	// the same preference annotation_actor_row.go applies on the per-tx
	// timeline so feed and timeline render the same display name.
	displayName := actorName
	if actorType == "user" && actorUserName != "" {
		displayName = actorUserName
	}

	ann := Annotation{
		ID:            formatUUID(id),
		ShortID:       shortID,
		TransactionID: formatUUID(txnID),
		Kind:          kind,
		ActorType:     actorType,
		ActorName:     displayName,
		CreatedAt:     pgconv.TimestampStr(createdAt),
		CreatedAtTime: createdAt.Time.UTC(),
	}
	if actorUpdatedAt.Valid {
		ann.ActorAvatarVersion = strconv.FormatInt(actorUpdatedAt.Time.Unix(), 10)
	}
	if actorIDOpt.Valid && actorIDOpt.String != "" {
		s := actorIDOpt.String
		ann.ActorID = &s
	}
	if tagID.Valid {
		s := formatUUID(tagID)
		ann.TagID = &s
	}
	if ruleID.Valid {
		s := formatUUID(ruleID)
		ann.RuleID = &s
	}
	if sessionID.Valid {
		s := formatUUID(sessionID)
		ann.SessionID = &s
	}
	if len(payload) > 0 && string(payload) != "{}" {
		var p map[string]interface{}
		if err := json.Unmarshal(payload, &p); err == nil {
			ann.Payload = p
		}
	}

	row := FeedActivityRow{
		Annotation:         ann,
		TransactionShortID: txShort,
		TransactionName:    txName,
		MerchantName:       pgconv.TextOr(merchantName, ""),
		IsoCurrencyCode:    pgconv.TextOr(isoCcy, "USD"),
		Pending:            pending,
		AccountName:        pgconv.TextOr(accountName, ""),
		InstitutionName:    pgconv.TextOr(institutionName, ""),
	}
	if amount.Valid {
		if f, err := amount.Float64Value(); err == nil && f.Valid {
			row.Amount = f.Float64
		}
	}
	if txDate.Valid {
		row.TransactionDate = txDate.Time.Format("2006-01-02")
	}
	if catDisplay.Valid {
		v := catDisplay.String
		row.CategoryDisplayName = &v
	}
	if catColor.Valid {
		v := catColor.String
		row.CategoryColor = &v
	}
	if catIcon.Valid {
		v := catIcon.String
		row.CategoryIcon = &v
	}
	if catSlug.Valid {
		v := catSlug.String
		row.CategorySlug = &v
	}
	return row, nil
}

// enrichFeedActivityRows runs `EnrichAnnotations` over a feed-activity slice
// (ASC ordering required by the dedup heuristics) and returns the rows in
// their original order.
func enrichFeedActivityRows(in []FeedActivityRow) []FeedActivityRow {
	if len(in) == 0 {
		return in
	}
	asc := make([]Annotation, len(in))
	for i, r := range in {
		asc[len(in)-1-i] = r.Annotation
	}
	enriched := EnrichAnnotations(asc, EnrichOptions{})
	byID := make(map[string]Annotation, len(enriched))
	for _, a := range enriched {
		byID[a.ID] = a
	}
	out := make([]FeedActivityRow, 0, len(enriched))
	for _, r := range in {
		if a, ok := byID[r.Annotation.ID]; ok {
			r.Annotation = a
			out = append(out, r)
		}
	}
	return out
}


// ── Sync events ───────────────────────────────────────────────────────────

// fetchFeedSyncEvents returns one FeedEvent per sync run within the window
// (failed runs always included; successful no-op runs filtered out).
// Inline transaction samples are batched in a single follow-up query.
func (s *Service) fetchFeedSyncEvents(ctx context.Context, cutoff, upper time.Time, sampleLimit int) ([]FeedEvent, error) {
	const q = `
SELECT
    sl.id, sl.connection_id,
    bc.institution_name, bc.provider,
    sl.trigger, sl.status,
    sl.added_count, sl.modified_count, sl.removed_count,
    sl.error_message, sl.started_at, sl.completed_at, sl.rule_hits
FROM sync_logs sl
JOIN bank_connections bc ON sl.connection_id = bc.id
WHERE sl.started_at >= $1
  AND sl.started_at <  $2
ORDER BY sl.started_at DESC
`

	rows, err := s.Pool.Query(ctx, q, cutoff, upper)
	if err != nil {
		return nil, fmt.Errorf("list feed sync logs: %w", err)
	}
	defer rows.Close()

	var raws []syncRowRaw
	for rows.Next() {
		var r syncRowRaw
		if err := rows.Scan(
			&r.id, &r.connectionID, &r.institutionName, &r.provider,
			&r.trigger, &r.status,
			&r.addedCount, &r.modifiedCount, &r.removedCount,
			&r.errorMessage, &r.startedAt, &r.completedAt, &r.ruleHitsJSON,
		); err != nil {
			return nil, fmt.Errorf("scan feed sync row: %w", err)
		}
		raws = append(raws, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate feed sync rows: %w", err)
	}

	syncIDs := make([]string, 0, len(raws))
	for _, r := range raws {
		hasChanges := r.addedCount+r.modifiedCount+r.removedCount > 0
		if hasChanges || r.status == "error" {
			syncIDs = append(syncIDs, formatUUID(r.id))
		}
	}
	samplesBySync, err := s.fetchSyncSampleTransactions(ctx, raws, sampleLimit)
	if err != nil {
		s.Logger.Debug("fetch sync sample transactions", "error", err)
	}

	// Dedup repeated same-error sync attempts per connection. The cron
	// retry cadence (every 15 minutes for connections in error state)
	// otherwise dumps 50+ identical cards into the rail and drowns out
	// every signal event. Each (connection_id, error_message) cluster
	// folds into the most recent attempt with `RetryCount` set.
	type errKey struct {
		connectionID string
		errorMessage string
	}
	errClusters := make(map[errKey]*FeedSyncEvent)

	out := make([]FeedEvent, 0, len(raws))
	for _, r := range raws {
		hasChanges := r.addedCount+r.modifiedCount+r.removedCount > 0
		if !hasChanges && r.status != "error" {
			continue
		}
		if !r.startedAt.Valid {
			continue
		}

		ev := &FeedSyncEvent{
			SyncLogID:       formatUUID(r.id),
			InstitutionName: pgconv.TextOr(r.institutionName, ""),
			Provider:        r.provider,
			Trigger:         r.trigger,
			Status:          r.status,
			ErrorMessage:    pgconv.TextOr(r.errorMessage, ""),
			AddedCount:      int(r.addedCount),
			ModifiedCount:   int(r.modifiedCount),
			RemovedCount:    int(r.removedCount),
			StartedAt:       r.startedAt.Time.UTC(),
			RuleOutcomes:    s.parseFeedRuleHits(ctx, r.ruleHitsJSON),
		}
		if r.completedAt.Valid {
			ev.CompletedAt = r.completedAt.Time.UTC()
		}
		samples := samplesBySync[ev.SyncLogID]
		if len(samples) > sampleLimit {
			ev.AdditionalCount = len(samples) - sampleLimit
			samples = samples[:sampleLimit]
		} else if ev.AddedCount > len(samples) {
			ev.AdditionalCount = ev.AddedCount - len(samples)
		}
		ev.SampleTransactions = samples

		if r.status == "error" {
			key := errKey{
				connectionID: formatUUID(r.connectionID),
				errorMessage: ev.ErrorMessage,
			}
			if existing, ok := errClusters[key]; ok {
				existing.RetryCount++
				if ev.StartedAt.Before(existing.FirstFailureAt) || existing.FirstFailureAt.IsZero() {
					existing.FirstFailureAt = ev.StartedAt
				}
				continue
			}
			ev.FirstFailureAt = ev.StartedAt
			errClusters[key] = ev
		}

		out = append(out, FeedEvent{
			Type:      "sync",
			Timestamp: ev.StartedAt,
			Sync:      ev,
		})
	}
	return out, nil
}

// syncRowRaw is the per-row scratch struct used by `fetchFeedSyncEvents`
// to keep raw column values around long enough to drive the follow-up
// sample-transaction fetch. Hoisted out of the function so the helper can
// take a typed slice instead of a struct literal.
type syncRowRaw struct {
	id, connectionID                        pgtype.UUID
	institutionName                         pgtype.Text
	provider                                string
	trigger, status                         string
	addedCount, modifiedCount, removedCount int32
	errorMessage                            pgtype.Text
	startedAt, completedAt                  pgtype.Timestamptz
	ruleHitsJSON                            []byte
}

// fetchSyncSampleTransactions issues one SQL query to fetch all transactions
// added in the window and partitions them per-sync in Go using each
// transaction's account → connection mapping plus the sync's started_at /
// completed_at window.
func (s *Service) fetchSyncSampleTransactions(ctx context.Context, raws []syncRowRaw, sampleLimit int) (map[string][]FeedSampleTx, error) {
	if len(raws) == 0 {
		return nil, nil
	}

	connIDs := make(map[string]bool, len(raws))
	earliest := time.Now()
	for _, r := range raws {
		connIDs[formatUUID(r.connectionID)] = true
		if r.startedAt.Valid && r.startedAt.Time.Before(earliest) {
			earliest = r.startedAt.Time
		}
	}
	connList := make([]string, 0, len(connIDs))
	for id := range connIDs {
		connList = append(connList, id)
	}

	const q = `
SELECT t.short_id, t.provider_name, t.provider_merchant_name, t.amount, t.iso_currency_code, t.date,
       t.created_at, t.pending, ac.name, bc.id::text, bc.institution_name,
       cat.display_name, cat.color, cat.icon, cat.slug
FROM transactions t
JOIN accounts ac ON ac.id = t.account_id
JOIN bank_connections bc ON ac.connection_id = bc.id
LEFT JOIN categories cat ON t.category_id = cat.id
WHERE t.created_at >= $1
  AND t.deleted_at IS NULL
  AND bc.id::text = ANY($2::text[])
ORDER BY t.created_at DESC
`

	rows, err := s.Pool.Query(ctx, q, earliest.Add(-1*time.Minute), connList)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type txRow struct {
		FeedSampleTx
		ConnectionID string
		CreatedAt    time.Time
	}
	var fetched []txRow
	for rows.Next() {
		var (
			shortID, provName    string
			merchantName         pgtype.Text
			amount               pgtype.Numeric
			isoCcy               pgtype.Text
			txDate               pgtype.Date
			createdAt            pgtype.Timestamptz
			pending              bool
			accountName          pgtype.Text
			connID               string
			institutionName      pgtype.Text
			catDisplay, catColor pgtype.Text
			catIcon, catSlug     pgtype.Text
		)
		if err := rows.Scan(&shortID, &provName, &merchantName, &amount, &isoCcy, &txDate, &createdAt, &pending, &accountName, &connID, &institutionName, &catDisplay, &catColor, &catIcon, &catSlug); err != nil {
			return nil, err
		}
		t := txRow{
			FeedSampleTx: FeedSampleTx{
				ShortID:      shortID,
				Name:         provName,
				MerchantName: pgconv.TextOr(merchantName, ""),
				Currency:     pgconv.TextOr(isoCcy, "USD"),
				AccountName:  pgconv.TextOr(accountName, ""),
				Institution:  pgconv.TextOr(institutionName, ""),
				Pending:      pending,
			},
			ConnectionID: connID,
			CreatedAt:    createdAt.Time.UTC(),
		}
		if amount.Valid {
			if f, err := amount.Float64Value(); err == nil && f.Valid {
				t.Amount = f.Float64
			}
		}
		if txDate.Valid {
			t.Date = txDate.Time.Format("2006-01-02")
		}
		if catDisplay.Valid {
			v := catDisplay.String
			t.CategoryDisplayName = &v
		}
		if catColor.Valid {
			v := catColor.String
			t.CategoryColor = &v
		}
		if catIcon.Valid {
			v := catIcon.String
			t.CategoryIcon = &v
		}
		if catSlug.Valid {
			v := catSlug.String
			t.CategorySlug = &v
		}
		fetched = append(fetched, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	samples := make(map[string][]FeedSampleTx, len(raws))
	for _, r := range raws {
		if !r.startedAt.Valid {
			continue
		}
		connID := formatUUID(r.connectionID)
		windowStart := r.startedAt.Time.Add(-30 * time.Second)
		windowEnd := r.startedAt.Time.Add(2 * time.Hour)
		if r.completedAt.Valid {
			windowEnd = r.completedAt.Time.Add(30 * time.Second)
		}

		var picked []FeedSampleTx
		for _, t := range fetched {
			if t.ConnectionID != connID {
				continue
			}
			if t.CreatedAt.Before(windowStart) || t.CreatedAt.After(windowEnd) {
				continue
			}
			picked = append(picked, t.FeedSampleTx)
		}
		// Sort by absolute amount desc so the biggest transactions surface
		// first — they're the highest-signal samples for "what came in".
		sort.SliceStable(picked, func(i, j int) bool {
			return absFloat(picked[i].Amount) > absFloat(picked[j].Amount)
		})
		if len(picked) > sampleLimit {
			samples[formatUUID(r.id)] = append([]FeedSampleTx(nil), picked[:sampleLimit]...)
		} else {
			samples[formatUUID(r.id)] = picked
		}
	}
	return samples, nil
}

// parseFeedRuleHits decodes the `sync_logs.rule_hits` JSONB column into the
// feed's lightweight FeedRuleOutcome slice. The richer SyncLogRow path
// resolves rule names against the live rules table; on the feed we accept
// the frozen names captured at sync-time, which is good enough for the
// "X auto-tagged" outcome line and avoids the extra round-trip.
func (s *Service) parseFeedRuleHits(ctx context.Context, payload []byte) []FeedRuleOutcome {
	if len(payload) == 0 || string(payload) == "{}" || string(payload) == "[]" {
		return nil
	}
	type hit struct {
		RuleID      string `json:"rule_id"`
		RuleShortID string `json:"rule_short_id"`
		RuleName    string `json:"rule_name"`
		Count       int    `json:"count"`
	}
	var raw []hit
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil
	}
	out := make([]FeedRuleOutcome, 0, len(raw))
	for _, h := range raw {
		if h.Count <= 0 {
			continue
		}
		out = append(out, FeedRuleOutcome{
			RuleName:    h.RuleName,
			RuleShortID: h.RuleShortID,
			Count:       h.Count,
		})
	}
	return out
}

// fetchFeedSessionMeta fetches mcp_sessions metadata for any session_ids
// referenced by the supplied annotations. Returns a map keyed by session
// UUID string.
func (s *Service) fetchFeedSessionMeta(ctx context.Context, rows []FeedActivityRow) (map[string]feedSessionMeta, error) {
	ids := make([]string, 0)
	seen := make(map[string]bool)
	for _, r := range rows {
		if r.Annotation.SessionID == nil {
			continue
		}
		id := *r.Annotation.SessionID
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, nil
	}

	const q = `
SELECT id::text, short_id, api_key_name, purpose, created_at
FROM mcp_sessions
WHERE id::text = ANY($1::text[])
`

	pgrows, err := s.Pool.Query(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	defer pgrows.Close()
	out := make(map[string]feedSessionMeta, len(ids))
	for pgrows.Next() {
		var id, shortID, keyName, purpose string
		var createdAt pgtype.Timestamptz
		if err := pgrows.Scan(&id, &shortID, &keyName, &purpose, &createdAt); err != nil {
			return nil, err
		}
		out[id] = feedSessionMeta{
			ID:         id,
			ShortID:    shortID,
			APIKeyName: keyName,
			Purpose:    purpose,
			CreatedAt:  createdAt.Time.UTC(),
		}
	}
	return out, nil
}

type feedSessionMeta struct {
	ID         string
	ShortID    string
	APIKeyName string
	Purpose    string
	CreatedAt  time.Time
}

// ── Grouping ──────────────────────────────────────────────────────────────

// groupAnnotationsIntoEvents applies the two-stage grouping described on
// `ListFeedEvents`: hard group by session_id, then soft-bucket the
// remaining un-sessioned annotations.
func groupAnnotationsIntoEvents(rows []FeedActivityRow, sessions map[string]feedSessionMeta, params FeedEventsParams) []FeedEvent {
	bySession := make(map[string][]FeedActivityRow)
	var unsessioned []FeedActivityRow
	for _, r := range rows {
		if r.Annotation.SessionID != nil && *r.Annotation.SessionID != "" {
			id := *r.Annotation.SessionID
			bySession[id] = append(bySession[id], r)
		} else {
			unsessioned = append(unsessioned, r)
		}
	}

	out := make([]FeedEvent, 0, len(bySession)+len(unsessioned))

	for sessionID, rows := range bySession {
		ev := buildAgentSessionEvent(sessionID, rows, sessions[sessionID], params)
		out = append(out, FeedEvent{
			Type:         "agent_session",
			Timestamp:    ev.EndedAt,
			AgentSession: &ev,
		})
	}

	out = append(out, groupUnsessionedAnnotations(unsessioned, params)...)
	return out
}

// buildAgentSessionEvent collapses the annotations of one MCP session into
// a single FeedAgentSessionEvent, summarising the kind breakdown and
// picking up to SampleLimit unique transaction samples.
func buildAgentSessionEvent(sessionID string, rows []FeedActivityRow, meta feedSessionMeta, params FeedEventsParams) FeedAgentSessionEvent {
	ev := FeedAgentSessionEvent{
		SessionID:      sessionID,
		SessionShortID: meta.ShortID,
		APIKeyName:     meta.APIKeyName,
		Purpose:        meta.Purpose,
		KindCounts:     make(map[string]int),
	}
	if !meta.CreatedAt.IsZero() {
		ev.StartedAt = meta.CreatedAt
	}

	uniqueTx := make(map[string]FeedSampleTx)
	for _, r := range rows {
		ann := r.Annotation
		if ev.ActorName == "" {
			ev.ActorName = ann.ActorName
			ev.ActorType = ann.ActorType
			ev.ActorAvatarVersion = ann.ActorAvatarVersion
			if ann.ActorID != nil {
				ev.ActorID = *ann.ActorID
			}
		}
		ev.AnnotationCount++
		ev.KindCounts[ann.Kind]++
		if ev.StartedAt.IsZero() || ann.CreatedAtTime.Before(ev.StartedAt) {
			ev.StartedAt = ann.CreatedAtTime
		}
		if ann.CreatedAtTime.After(ev.EndedAt) {
			ev.EndedAt = ann.CreatedAtTime
		}
		if _, ok := uniqueTx[r.TransactionShortID]; !ok {
			uniqueTx[r.TransactionShortID] = sampleTxFromRow(r)
		}
	}
	ev.UniqueTransactions = len(uniqueTx)

	samples := make([]FeedSampleTx, 0, len(uniqueTx))
	for _, t := range uniqueTx {
		samples = append(samples, t)
	}
	sort.SliceStable(samples, func(i, j int) bool {
		return absFloat(samples[i].Amount) > absFloat(samples[j].Amount)
	})
	if len(samples) > params.SampleLimit {
		samples = samples[:params.SampleLimit]
	}
	ev.SampleTransactions = samples

	if ev.ActorName == "" && meta.APIKeyName != "" {
		ev.ActorName = meta.APIKeyName
		ev.ActorType = "agent"
	}
	if ev.EndedAt.IsZero() {
		ev.EndedAt = ev.StartedAt
	}
	return ev
}

// groupUnsessionedAnnotations buckets the remaining annotations by
// `(actor_id, floor(time / feedSoftBucketWindow))` — note that `kind` is
// NOT part of the bucket key. Buckets at-or-above BulkThreshold collapse
// into a single FeedBulkActionEvent regardless of how many distinct kinds
// the annotations carry; the per-kind breakdown is preserved on the
// event's `KindCounts` map so the templ can render the breakdown line.
//
// Sub-threshold buckets pass through standalone comments individually
// (other singleton recategorisations are too low-signal for the home
// feed). When ≥3 standalone comments share a bucket they collapse into a
// bulk_action event whose Kind == "comment" — we don't want a feed full
// of identical "Alice commented on transaction" rows when Alice was
// triaging.
func groupUnsessionedAnnotations(rows []FeedActivityRow, params FeedEventsParams) []FeedEvent {
	if len(rows) == 0 {
		return nil
	}
	bucketSeconds := int64(feedSoftBucketWindow.Seconds())

	type bucketKey struct {
		actorID string
		bucket  int64
	}

	buckets := make(map[bucketKey][]FeedActivityRow)
	keyOrder := make([]bucketKey, 0)
	for _, r := range rows {
		actorID := ""
		if r.Annotation.ActorID != nil {
			actorID = *r.Annotation.ActorID
		}
		if actorID == "" {
			actorID = r.Annotation.ActorType + ":" + r.Annotation.ActorName
		}
		k := bucketKey{
			actorID: actorID,
			bucket:  r.Annotation.CreatedAtTime.Unix() / bucketSeconds,
		}
		if _, ok := buckets[k]; !ok {
			keyOrder = append(keyOrder, k)
		}
		buckets[k] = append(buckets[k], r)
	}

	out := make([]FeedEvent, 0, len(buckets))
	for _, k := range keyOrder {
		bucket := buckets[k]
		if len(bucket) >= params.BulkThreshold {
			ev := buildBulkActionEvent(bucket, params)
			out = append(out, FeedEvent{
				Type:       "bulk_action",
				Timestamp:  ev.EndedAt,
				BulkAction: &ev,
			})
			continue
		}
		// Below threshold — only standalone comments survive.
		for _, r := range bucket {
			if r.Annotation.Kind != "comment" || r.Annotation.IsDeleted {
				continue
			}
			ev := FeedCommentEvent{
				CommentShortID:     r.Annotation.ShortID,
				ActorName:          r.Annotation.ActorName,
				ActorType:          r.Annotation.ActorType,
				ActorAvatarVersion: r.Annotation.ActorAvatarVersion,
				Content:            r.Annotation.Content,
				Transaction:        sampleTxFromRow(r),
			}
			if r.Annotation.ActorID != nil {
				ev.ActorID = *r.Annotation.ActorID
			}
			out = append(out, FeedEvent{
				Type:      "comment",
				Timestamp: r.Annotation.CreatedAtTime,
				Comment:   &ev,
			})
		}
	}
	return out
}

// buildBulkActionEvent collapses one (actor, time-bucket) cluster of
// annotations into a single FeedBulkActionEvent, summarising the subjects
// (tag names, category names, rule names) the actor applied. When every
// row in the bucket shares one kind, `Kind` is set to that homogeneous
// kind so the templ can render the dedicated verb phrasing; otherwise
// `Kind` is "mixed" and the templ renders the per-kind breakdown line.
func buildBulkActionEvent(rows []FeedActivityRow, params FeedEventsParams) FeedBulkActionEvent {
	ev := FeedBulkActionEvent{
		Count:      len(rows),
		KindCounts: make(map[string]int),
	}
	subjectCounts := make(map[string]int)
	subjectMeta := make(map[string]FeedBulkSubject)
	uniqueTx := make(map[string]FeedSampleTx)

	for _, r := range rows {
		ann := r.Annotation
		if ev.ActorName == "" {
			ev.ActorName = ann.ActorName
			ev.ActorType = ann.ActorType
			ev.ActorAvatarVersion = ann.ActorAvatarVersion
			if ann.ActorID != nil {
				ev.ActorID = *ann.ActorID
			}
		}
		ev.KindCounts[ann.Kind]++
		if ev.StartedAt.IsZero() || ann.CreatedAtTime.Before(ev.StartedAt) {
			ev.StartedAt = ann.CreatedAtTime
		}
		if ann.CreatedAtTime.After(ev.EndedAt) {
			ev.EndedAt = ann.CreatedAtTime
		}
		key := bulkSubjectKey(ann)
		if key != "" {
			subjectCounts[key]++
			if _, ok := subjectMeta[key]; !ok {
				subjectMeta[key] = FeedBulkSubject{
					Name: ann.Subject,
					Slug: bulkSubjectSlug(ann),
				}
			}
		}
		if _, ok := uniqueTx[r.TransactionShortID]; !ok {
			uniqueTx[r.TransactionShortID] = sampleTxFromRow(r)
		}
	}

	// Single-kind buckets keep the dedicated kind so the templ can render
	// the canonical verb phrasing ("Alice categorised 12 transactions").
	// Mixed-kind buckets get "mixed" so the templ falls back to the
	// generic "Alice updated N transactions" headline plus the per-kind
	// breakdown line.
	if len(ev.KindCounts) == 1 {
		for k := range ev.KindCounts {
			ev.Kind = k
		}
	} else {
		ev.Kind = "mixed"
	}

	subjects := make([]FeedBulkSubject, 0, len(subjectCounts))
	for k, count := range subjectCounts {
		s := subjectMeta[k]
		s.Count = count
		subjects = append(subjects, s)
	}
	sort.SliceStable(subjects, func(i, j int) bool {
		return subjects[i].Count > subjects[j].Count
	})
	ev.Subjects = subjects

	samples := make([]FeedSampleTx, 0, len(uniqueTx))
	for _, t := range uniqueTx {
		samples = append(samples, t)
	}
	sort.SliceStable(samples, func(i, j int) bool {
		return absFloat(samples[i].Amount) > absFloat(samples[j].Amount)
	})
	if len(samples) > params.SampleLimit {
		samples = samples[:params.SampleLimit]
	}
	ev.SampleTransactions = samples
	return ev
}

// foldReportsIntoEvents pairs each report with a matching bulk_action or
// agent_session event in `events` so the report renders as part of that
// card's headline instead of as its own row. Matching is:
//
//   - SessionID equality with an agent_session event, OR
//   - actor (id, type+name fallback) equality plus the report's timestamp
//     falling inside the bulk_action's [StartedAt-15m, EndedAt+15m] window.
//
// Mutates the matched events in place (sets `ev.BulkAction.Report` /
// `ev.AgentSession.Report`) and returns the un-folded reports for the
// handler to render as standalone report cards.
func foldReportsIntoEvents(events []FeedEvent, reports []AgentReportResponse) []AgentReportResponse {
	if len(events) == 0 || len(reports) == 0 {
		return reports
	}

	leftover := make([]AgentReportResponse, 0, len(reports))
	for _, rep := range reports {
		if folded := tryFoldReport(events, rep); !folded {
			leftover = append(leftover, rep)
		}
	}
	return leftover
}

// tryFoldReport attempts to attach `rep` to a matching event in `events`.
// Returns true if a match was found and the event was mutated.
func tryFoldReport(events []FeedEvent, rep AgentReportResponse) bool {
	ref := reportRefFromResponse(rep)

	// 1. Session match wins. If the report's session_id lines up with an
	//    agent_session event, fold there regardless of timestamps.
	if rep.SessionID != nil && *rep.SessionID != "" {
		for i := range events {
			if events[i].Type != "agent_session" {
				continue
			}
			if events[i].AgentSession == nil {
				continue
			}
			if events[i].AgentSession.SessionID == *rep.SessionID {
				if events[i].AgentSession.Report == nil {
					events[i].AgentSession.Report = &ref
				}
				return true
			}
		}
	}

	// 2. Actor + window match into a bulk_action event. The report's
	//    `created_at` must fall inside the bucket window (with a slack
	//    of feedSoftBucketWindow on either side so a report written
	//    seconds after the last annotation still anchors).
	repTime, err := time.Parse(time.RFC3339, rep.CreatedAt)
	if err != nil {
		return false
	}
	repActorID := ""
	if rep.CreatedByID != nil {
		repActorID = *rep.CreatedByID
	}
	repActorKey := repActorID
	if repActorKey == "" {
		repActorKey = rep.CreatedByType + ":" + rep.CreatedByName
	}

	for i := range events {
		if events[i].Type != "bulk_action" {
			continue
		}
		ba := events[i].BulkAction
		if ba == nil {
			continue
		}
		baKey := ba.ActorID
		if baKey == "" {
			baKey = ba.ActorType + ":" + ba.ActorName
		}
		if baKey != repActorKey {
			continue
		}
		windowStart := ba.StartedAt.Add(-feedSoftBucketWindow)
		windowEnd := ba.EndedAt.Add(feedSoftBucketWindow)
		if repTime.Before(windowStart) || repTime.After(windowEnd) {
			continue
		}
		if ba.Report == nil {
			ba.Report = &ref
		}
		// Take the report's timestamp as the event timestamp so the row
		// sorts at the report's wall-clock position (typically the run's
		// last action). Prevents the row drifting earlier than the
		// report itself.
		if repTime.After(events[i].Timestamp) {
			events[i].Timestamp = repTime
		}
		return true
	}
	return false
}

func reportRefFromResponse(r AgentReportResponse) FeedReportRef {
	return FeedReportRef{
		ID:       r.ID,
		ShortID:  r.ShortID,
		Title:    r.Title,
		Priority: r.Priority,
		Tags:     r.Tags,
		IsUnread: r.ReadAt == nil,
	}
}

func bulkSubjectKey(a Annotation) string {
	switch a.Kind {
	case "tag_added", "tag_removed":
		return "tag:" + a.TagSlug
	case "category_set":
		return "category:" + a.CategorySlug
	case "rule_applied":
		return "rule:" + a.RuleShortID
	}
	return ""
}

func bulkSubjectSlug(a Annotation) string {
	switch a.Kind {
	case "tag_added", "tag_removed":
		return a.TagSlug
	case "category_set":
		return a.CategorySlug
	case "rule_applied":
		return a.RuleShortID
	}
	return ""
}

// sampleTxFromRow projects a feed-activity row into the small FeedSampleTx
// shape used by every event card.
func sampleTxFromRow(r FeedActivityRow) FeedSampleTx {
	merchant := r.MerchantName
	if merchant == "" {
		merchant = r.TransactionName
	}
	return FeedSampleTx{
		ShortID:             r.TransactionShortID,
		Name:                r.TransactionName,
		MerchantName:        merchant,
		Amount:              r.Amount,
		Currency:            r.IsoCurrencyCode,
		Date:                r.TransactionDate,
		AccountName:         r.AccountName,
		Institution:         r.InstitutionName,
		Pending:             r.Pending,
		CategoryDisplayName: r.CategoryDisplayName,
		CategoryColor:       r.CategoryColor,
		CategoryIcon:        r.CategoryIcon,
		CategorySlug:        r.CategorySlug,
	}
}

func absFloat(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
