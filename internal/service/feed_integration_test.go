//go:build integration

// Integration tests that pin down `Service.ListFeedEvents` grouping behaviour.
//
// The feed page intentionally collapses noisy clusters of annotations into a
// handful of high-signal cards. The cases below seed real annotations + sync
// logs + mcp_sessions + transactions and assert each grouping rule the
// previous iterations of /feed established. They serve as a regression net so
// future iterations can't quietly drop the dedup, bulk-bucket, or
// session-collapse logic.
package service_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	"breadbox/internal/testutil"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// feedSeed bundles the IDs needed across the per-subtest seeders. Every
// subtest gets a fresh, truncated DB via newService → testutil.Pool, so
// helpers populate this struct top-down.
type feedSeed struct {
	UserID  pgtype.UUID
	ConnID  pgtype.UUID
	AcctID  pgtype.UUID
	TxnIDs  []pgtype.UUID
}

// seedFeedFixture creates one user/connection/account plus `txnCount`
// transactions so tests have transaction IDs to attach annotations to.
func seedFeedFixture(t *testing.T, queries *db.Queries, suffix string, txnCount int) feedSeed {
	t.Helper()
	user := testutil.MustCreateUser(t, queries, "Alice "+suffix)
	conn := testutil.MustCreateConnection(t, queries, user.ID, "feed_conn_"+suffix)
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "feed_acct_"+suffix, "Checking "+suffix)

	out := feedSeed{UserID: user.ID, ConnID: conn.ID, AcctID: acct.ID}
	for i := 0; i < txnCount; i++ {
		txn := testutil.MustCreateTransaction(
			t, queries, acct.ID,
			fmt.Sprintf("feed_txn_%s_%d", suffix, i),
			fmt.Sprintf("Merchant %s %d", suffix, i),
			int64(1000+i*250),
			"2026-04-15",
		)
		out.TxnIDs = append(out.TxnIDs, txn.ID)
	}
	return out
}

// insertAnnotation inserts an annotation and forces its `created_at` to the
// supplied time. The sqlc-generated InsertAnnotation does not accept a
// created_at parameter (the column defaults to NOW()), so we patch it after
// the fact — mirrors what real history would look like once enough wall-time
// has elapsed.
func insertAnnotation(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	queries *db.Queries,
	txnID pgtype.UUID,
	kind, actorType, actorID, actorName string,
	sessionID pgtype.UUID,
	payload []byte,
	createdAt time.Time,
) pgtype.UUID {
	t.Helper()
	params := db.InsertAnnotationParams{
		TransactionID: txnID,
		Kind:          kind,
		ActorType:     actorType,
		ActorName:     actorName,
		Payload:       payload,
	}
	if actorID != "" {
		params.ActorID = pgtype.Text{String: actorID, Valid: true}
	}
	if sessionID.Valid {
		params.SessionID = sessionID
	}
	ann, err := queries.InsertAnnotation(ctx, params)
	if err != nil {
		t.Fatalf("InsertAnnotation(%s): %v", kind, err)
	}
	if _, err := pool.Exec(ctx,
		"UPDATE annotations SET created_at = $1 WHERE id = $2",
		createdAt, ann.ID,
	); err != nil {
		t.Fatalf("backdate annotation: %v", err)
	}
	return ann.ID
}

// findEventByType returns the first FeedEvent of the requested type, or nil.
func findEventByType(events []service.FeedEvent, eventType string) *service.FeedEvent {
	for i := range events {
		if events[i].Type == eventType {
			return &events[i]
		}
	}
	return nil
}

// countEventsByType counts events whose Type matches.
func countEventsByType(events []service.FeedEvent, eventType string) int {
	n := 0
	for _, e := range events {
		if e.Type == eventType {
			n++
		}
	}
	return n
}

func TestListFeedEvents_GroupingBehaviors(t *testing.T) {
	t.Run("SyncErrorDedup", func(t *testing.T) {
		svc, queries, _ := newService(t)
		ctx := context.Background()
		seed := seedFeedFixture(t, queries, "syncerr", 0)

		now := time.Now().UTC()
		errMsg := pgtype.Text{String: "ITEM_LOGIN_REQUIRED", Valid: true}
		// Five sync_logs, same connection + same error message, oldest →
		// newest. The dedup key is (connection_id, error_message); these
		// should collapse into one event with RetryCount == 4 and
		// FirstFailureAt == oldest.
		oldest := now.Add(-90 * time.Minute)
		for i := 0; i < 5; i++ {
			startedAt := now.Add(time.Duration(-90+i*15) * time.Minute)
			if _, err := queries.CreateSyncLog(ctx, db.CreateSyncLogParams{
				ConnectionID: seed.ConnID,
				Trigger:      db.SyncTriggerCron,
				Status:       db.SyncStatusError,
				StartedAt:    pgconv.Timestamptz(startedAt),
				CompletedAt:  pgconv.Timestamptz(startedAt.Add(2 * time.Second)),
				ErrorMessage: errMsg,
			}); err != nil {
				t.Fatalf("CreateSyncLog %d: %v", i, err)
			}
		}

		events, err := svc.ListFeedEvents(ctx, service.FeedEventsParams{})
		if err != nil {
			t.Fatalf("ListFeedEvents: %v", err)
		}
		if got := countEventsByType(events, "sync"); got != 1 {
			t.Fatalf("expected 1 sync event after dedup, got %d", got)
		}
		ev := findEventByType(events, "sync").Sync
		if ev.RetryCount != 4 {
			t.Errorf("RetryCount = %d, want 4", ev.RetryCount)
		}
		if ev.Status != "error" {
			t.Errorf("Status = %q, want %q", ev.Status, "error")
		}
		// FirstFailureAt is the oldest start. Compare at second precision —
		// pgtype rounds to micro and time.Now() roundtrips through pg.
		if !ev.FirstFailureAt.Truncate(time.Second).Equal(oldest.Truncate(time.Second)) {
			t.Errorf("FirstFailureAt = %v, want %v", ev.FirstFailureAt, oldest)
		}
	})

	t.Run("DifferentErrorsDoNotDedupe", func(t *testing.T) {
		svc, queries, _ := newService(t)
		ctx := context.Background()
		seed := seedFeedFixture(t, queries, "differr", 0)

		now := time.Now().UTC()
		mkLog := func(offsetMin int, msg string) {
			startedAt := now.Add(time.Duration(offsetMin) * time.Minute)
			if _, err := queries.CreateSyncLog(ctx, db.CreateSyncLogParams{
				ConnectionID: seed.ConnID,
				Trigger:      db.SyncTriggerCron,
				Status:       db.SyncStatusError,
				StartedAt:    pgconv.Timestamptz(startedAt),
				CompletedAt:  pgconv.Timestamptz(startedAt.Add(time.Second)),
				ErrorMessage: pgtype.Text{String: msg, Valid: true},
			}); err != nil {
				t.Fatalf("CreateSyncLog: %v", err)
			}
		}
		// Three with err A, two with err B.
		for i := 0; i < 3; i++ {
			mkLog(-90+i*5, "ITEM_LOGIN_REQUIRED")
		}
		for i := 0; i < 2; i++ {
			mkLog(-30+i*5, "RATE_LIMIT_EXCEEDED")
		}

		events, err := svc.ListFeedEvents(ctx, service.FeedEventsParams{})
		if err != nil {
			t.Fatalf("ListFeedEvents: %v", err)
		}
		if got := countEventsByType(events, "sync"); got != 2 {
			t.Fatalf("expected 2 sync events (one per distinct error), got %d", got)
		}
	})

	t.Run("HardGroupBySessionID", func(t *testing.T) {
		svc, queries, pool := newService(t)
		ctx := context.Background()
		seed := seedFeedFixture(t, queries, "session", 5)

		// Create one MCP session + 12 annotations across 5 transactions
		// (4 category_set, 4 tag_added, 4 comment), all with session_id.
		session, err := queries.CreateMCPSession(ctx, db.CreateMCPSessionParams{
			ApiKeyID:   "00000000-0000-0000-0000-000000000abc",
			ApiKeyName: "review-bot",
			Purpose:    "categorize",
		})
		if err != nil {
			t.Fatalf("CreateMCPSession: %v", err)
		}

		// Need a tag for tag_added rows.
		tag := testutil.MustCreateTag(t, queries, "needs-review", "Needs Review")

		now := time.Now().UTC()
		actorID := pgconv.FormatUUID(session.ID)
		mk := func(kind string, txnIdx int, idx int) {
			ts := now.Add(time.Duration(-30+idx) * time.Second)
			params := db.InsertAnnotationParams{
				TransactionID: seed.TxnIDs[txnIdx],
				Kind:          kind,
				ActorType:     "agent",
				ActorID:       pgtype.Text{String: actorID, Valid: true},
				ActorName:     "review-bot",
				SessionID:     session.ID,
				Payload:       []byte(`{}`),
			}
			if kind == "tag_added" {
				params.TagID = tag.ID
				params.Payload = []byte(`{"tag_slug":"needs-review"}`)
			}
			if kind == "category_set" {
				params.Payload = []byte(`{"category_slug":"food_and_drink"}`)
			}
			ann, err := queries.InsertAnnotation(ctx, params)
			if err != nil {
				t.Fatalf("InsertAnnotation(%s): %v", kind, err)
			}
			if _, err := pool.Exec(ctx,
				"UPDATE annotations SET created_at = $1 WHERE id = $2", ts, ann.ID,
			); err != nil {
				t.Fatalf("backdate: %v", err)
			}
		}
		// 4 of each kind, distributed over 5 transactions.
		for i := 0; i < 4; i++ {
			mk("category_set", i%5, i)
		}
		for i := 0; i < 4; i++ {
			mk("tag_added", (i+1)%5, 4+i)
		}
		for i := 0; i < 4; i++ {
			mk("comment", (i+2)%5, 8+i)
		}

		events, err := svc.ListFeedEvents(ctx, service.FeedEventsParams{})
		if err != nil {
			t.Fatalf("ListFeedEvents: %v", err)
		}
		if got := countEventsByType(events, "agent_session"); got != 1 {
			t.Fatalf("expected 1 agent_session event, got %d (events=%+v)", got, events)
		}
		if got := countEventsByType(events, "bulk_action"); got != 0 {
			t.Errorf("expected 0 bulk_action events (sessioned annotations should not bucket), got %d", got)
		}
		if got := countEventsByType(events, "comment"); got != 0 {
			t.Errorf("expected 0 comment events (sessioned comments should fold into the session), got %d", got)
		}
		ev := findEventByType(events, "agent_session").AgentSession
		if ev.AnnotationCount != 12 {
			t.Errorf("AnnotationCount = %d, want 12", ev.AnnotationCount)
		}
		if ev.UniqueTransactions != 5 {
			t.Errorf("UniqueTransactions = %d, want 5", ev.UniqueTransactions)
		}
		for _, kind := range []string{"category_set", "tag_added", "comment"} {
			if ev.KindCounts[kind] != 4 {
				t.Errorf("KindCounts[%s] = %d, want 4", kind, ev.KindCounts[kind])
			}
		}
	})

	t.Run("SoftBucketBulkAction", func(t *testing.T) {
		svc, queries, pool := newService(t)
		ctx := context.Background()
		seed := seedFeedFixture(t, queries, "bulk", 4)

		// Anchor 2 minutes past a 15-min bucket boundary an hour ago so all
		// seeded annotations land in one bucket AND safely in the past
		// relative to the service's `time.Now()`. The earlier `Add(-1h)`
		// guards against the test firing in the first ~2 minutes of a
		// bucket — without it, `Truncate(15m).Add(2m)` lands in the
		// FUTURE and the SQL window's `created_at < now` upper bound
		// drops every seeded row. Mirrors the safer anchor pattern used
		// by every other Truncate-based test below in this file.
		now := time.Now().UTC().Add(-1 * time.Hour).Truncate(15 * time.Minute).Add(2 * time.Minute)
		actorID := "user-actor-bulk"
		// 4 category_set annotations, same actor, 4 different transactions,
		// all within ~30 seconds (well inside the 15-minute soft bucket).
		for i := 0; i < 4; i++ {
			insertAnnotation(t, ctx, pool, queries, seed.TxnIDs[i],
				"category_set", "user", actorID, "Alice",
				pgtype.UUID{},
				[]byte(`{"category_slug":"food_and_drink"}`),
				now.Add(time.Duration(i*5)*time.Second),
			)
		}

		events, err := svc.ListFeedEvents(ctx, service.FeedEventsParams{})
		if err != nil {
			t.Fatalf("ListFeedEvents: %v", err)
		}
		if got := countEventsByType(events, "bulk_action"); got != 1 {
			t.Fatalf("expected 1 bulk_action event, got %d", got)
		}
		ev := findEventByType(events, "bulk_action").BulkAction
		if ev.Kind != "category_set" {
			t.Errorf("Kind = %q, want %q", ev.Kind, "category_set")
		}
		if ev.Count != 4 {
			t.Errorf("Count = %d, want 4", ev.Count)
		}
	})

	t.Run("BelowThresholdDropsStandaloneCategorySet", func(t *testing.T) {
		svc, queries, pool := newService(t)
		ctx := context.Background()
		seed := seedFeedFixture(t, queries, "below", 2)

		now := time.Now().UTC()
		// Two category_set annotations from the same actor in the same
		// minute. Default BulkThreshold is 3 → bucket count 2 falls through,
		// and only `comment` rows survive sub-threshold, so these category
		// rows should disappear from the feed.
		for i := 0; i < 2; i++ {
			insertAnnotation(t, ctx, pool, queries, seed.TxnIDs[i],
				"category_set", "user", "actor-below", "Alice",
				pgtype.UUID{},
				[]byte(`{"category_slug":"food_and_drink"}`),
				now.Add(time.Duration(-10+i*5)*time.Second),
			)
		}

		events, err := svc.ListFeedEvents(ctx, service.FeedEventsParams{})
		if err != nil {
			t.Fatalf("ListFeedEvents: %v", err)
		}
		if got := countEventsByType(events, "bulk_action"); got != 0 {
			t.Errorf("expected 0 bulk_action events (under threshold), got %d", got)
		}
		if got := countEventsByType(events, "comment"); got != 0 {
			t.Errorf("expected 0 comment events, got %d", got)
		}
		// The whole feed should be empty for this fixture (no sync logs were
		// seeded either).
		if len(events) != 0 {
			t.Errorf("expected 0 events total, got %d (%+v)", len(events), events)
		}
	})

	t.Run("StandaloneCommentSurvives", func(t *testing.T) {
		svc, queries, pool := newService(t)
		ctx := context.Background()
		seed := seedFeedFixture(t, queries, "comment", 1)

		now := time.Now().UTC()
		insertAnnotation(t, ctx, pool, queries, seed.TxnIDs[0],
			"comment", "user", "actor-comment", "Alice",
			pgtype.UUID{},
			[]byte(`{"content":"Looks like a duplicate"}`),
			now.Add(-5*time.Minute),
		)

		events, err := svc.ListFeedEvents(ctx, service.FeedEventsParams{})
		if err != nil {
			t.Fatalf("ListFeedEvents: %v", err)
		}
		if got := countEventsByType(events, "comment"); got != 1 {
			t.Fatalf("expected 1 comment event, got %d", got)
		}
	})

	t.Run("SyncStartedAndUpdatedExcluded", func(t *testing.T) {
		svc, queries, pool := newService(t)
		ctx := context.Background()
		seed := seedFeedFixture(t, queries, "syncann", 1)

		now := time.Now().UTC()
		insertAnnotation(t, ctx, pool, queries, seed.TxnIDs[0],
			"sync_started", "system", "", "Plaid Sync",
			pgtype.UUID{},
			[]byte(`{}`),
			now.Add(-30*time.Second),
		)
		insertAnnotation(t, ctx, pool, queries, seed.TxnIDs[0],
			"sync_updated", "system", "", "Plaid Sync",
			pgtype.UUID{},
			[]byte(`{}`),
			now.Add(-20*time.Second),
		)

		events, err := svc.ListFeedEvents(ctx, service.FeedEventsParams{})
		if err != nil {
			t.Fatalf("ListFeedEvents: %v", err)
		}
		if len(events) != 0 {
			t.Errorf("expected sync_started/sync_updated annotations to be excluded; got %d events: %+v", len(events), events)
		}
	})

	t.Run("RuleAppliedDuringSyncExcluded", func(t *testing.T) {
		svc, queries, pool := newService(t)
		ctx := context.Background()
		seed := seedFeedFixture(t, queries, "rulesync", 1)

		now := time.Now().UTC()
		insertAnnotation(t, ctx, pool, queries, seed.TxnIDs[0],
			"rule_applied", "system", "", "Plaid Sync",
			pgtype.UUID{},
			[]byte(`{"applied_by":"sync"}`),
			now.Add(-30*time.Second),
		)

		events, err := svc.ListFeedEvents(ctx, service.FeedEventsParams{})
		if err != nil {
			t.Fatalf("ListFeedEvents: %v", err)
		}
		if len(events) != 0 {
			t.Errorf("expected rule_applied(applied_by=sync) to be excluded; got %d events", len(events))
		}
	})

	t.Run("RuleAppliedRetroactiveIncluded", func(t *testing.T) {
		svc, queries, pool := newService(t)
		ctx := context.Background()
		seed := seedFeedFixture(t, queries, "ruleretro", 5)

		// Need a real rule so rule_applied annotations have an FK target.
		rule := testutil.MustCreateTransactionRule(t, queries, "Coffee Rule", []byte(`[]`), []byte(`[]`), "on_create")
		ruleShortID := rule.ShortID

		// Anchor an hour back, snapped to a 15-min bucket + 2 min, so the
		// seeds bucket together AND land in the past — mirrors the
		// SoftBucketBulkAction fix above. See that comment for why the
		// straight `Truncate(15m).Add(2m)` form was flaky.
		now := time.Now().UTC().Add(-1 * time.Hour).Truncate(15 * time.Minute).Add(2 * time.Minute)
		actorID := "actor-retro"
		// 5 rule_applied annotations, same actor, no `applied_by=sync`, all
		// within ~25 seconds → should produce a single bulk_action event.
		for i := 0; i < 5; i++ {
			params := db.InsertAnnotationParams{
				TransactionID: seed.TxnIDs[i],
				Kind:          "rule_applied",
				ActorType:     "user",
				ActorID:       pgtype.Text{String: actorID, Valid: true},
				ActorName:     "Alice",
				RuleID:        rule.ID,
				Payload:       []byte(fmt.Sprintf(`{"rule_short_id":%q}`, ruleShortID)),
			}
			ann, err := queries.InsertAnnotation(ctx, params)
			if err != nil {
				t.Fatalf("InsertAnnotation %d: %v", i, err)
			}
			if _, err := pool.Exec(ctx,
				"UPDATE annotations SET created_at = $1 WHERE id = $2",
				now.Add(time.Duration(i*5)*time.Second), ann.ID,
			); err != nil {
				t.Fatalf("backdate: %v", err)
			}
		}

		events, err := svc.ListFeedEvents(ctx, service.FeedEventsParams{})
		if err != nil {
			t.Fatalf("ListFeedEvents: %v", err)
		}
		if got := countEventsByType(events, "bulk_action"); got != 1 {
			t.Fatalf("expected 1 bulk_action event, got %d (%+v)", got, events)
		}
		ev := findEventByType(events, "bulk_action").BulkAction
		if ev.Kind != "rule_applied" {
			t.Errorf("Kind = %q, want %q", ev.Kind, "rule_applied")
		}
		if ev.Count != 5 {
			t.Errorf("Count = %d, want 5", ev.Count)
		}
	})

	t.Run("BeforeAnchorsUpperBound", func(t *testing.T) {
		// `Before` rolls the 3-day window backward — events newer than the
		// upper bound must drop out of the result, while events inside
		// `[Before-3d, Before)` come through. Validates the pagination
		// affordance the /feed footer exposes.
		svc, queries, pool := newService(t)
		ctx := context.Background()
		seed := seedFeedFixture(t, queries, "before-window", 2)

		now := time.Now().UTC()
		// Recent comment (1 hour ago) — should be excluded by Before=now-2d.
		insertAnnotation(t, ctx, pool, queries, seed.TxnIDs[0],
			"comment", "user", "actor-recent", "Alice",
			pgtype.UUID{},
			[]byte(`{"content":"recent"}`),
			now.Add(-1*time.Hour),
		)
		// Older comment (2.5 days ago) — should land inside the chunk
		// `[Before-3d, Before)` where Before = now-2d.
		insertAnnotation(t, ctx, pool, queries, seed.TxnIDs[1],
			"comment", "user", "actor-older", "Alice",
			pgtype.UUID{},
			[]byte(`{"content":"older"}`),
			now.Add(-60*time.Hour),
		)

		before := now.Add(-2 * 24 * time.Hour)
		events, err := svc.ListFeedEvents(ctx, service.FeedEventsParams{
			Window: 3 * 24 * time.Hour,
			Before: before,
		})
		if err != nil {
			t.Fatalf("ListFeedEvents: %v", err)
		}
		if got := countEventsByType(events, "comment"); got != 1 {
			t.Fatalf("expected 1 comment in `[before-3d, before)` chunk, got %d (%+v)", got, events)
		}
		// The single event must be the older one (the recent comment is
		// past the upper bound).
		c := findEventByType(events, "comment").Comment
		if c.Content != "older" {
			t.Errorf("Comment.Content = %q, want %q", c.Content, "older")
		}
	})

	t.Run("BeforeIsCappedAt30Days", func(t *testing.T) {
		// Asking for a `Before` deeper than 30 days clamps the upper bound
		// to (now - FeedMaxLookback). With Window=3d the resulting chunk is
		// `[now-33d, now-30d)`, so an annotation aged ~31 days ago lands
		// inside it while a 90-day-old annotation does not.
		svc, queries, pool := newService(t)
		ctx := context.Background()
		seed := seedFeedFixture(t, queries, "before-cap", 2)

		now := time.Now().UTC()
		insertAnnotation(t, ctx, pool, queries, seed.TxnIDs[0],
			"comment", "user", "actor-cap-in", "Alice",
			pgtype.UUID{},
			[]byte(`{"content":"in-clamped-chunk"}`),
			now.Add(-31*24*time.Hour),
		)
		insertAnnotation(t, ctx, pool, queries, seed.TxnIDs[1],
			"comment", "user", "actor-cap-out", "Alice",
			pgtype.UUID{},
			[]byte(`{"content":"way-too-old"}`),
			now.Add(-90*24*time.Hour),
		)

		// Caller asks for a `Before` 60 days back — service must clamp it
		// to (now - FeedMaxLookback) so the 31-day-old annotation lands
		// inside the resulting chunk while the 90-day-old one stays out.
		events, err := svc.ListFeedEvents(ctx, service.FeedEventsParams{
			Window: 3 * 24 * time.Hour,
			Before: now.Add(-60 * 24 * time.Hour),
		})
		if err != nil {
			t.Fatalf("ListFeedEvents: %v", err)
		}
		if got := countEventsByType(events, "comment"); got != 1 {
			t.Fatalf("expected 1 comment after clamping Before to -30d, got %d (%+v)", got, events)
		}
		c := findEventByType(events, "comment").Comment
		if c.Content != "in-clamped-chunk" {
			t.Errorf("Comment.Content = %q, want %q", c.Content, "in-clamped-chunk")
		}
	})

	t.Run("BulkActionFoldsAcrossKinds", func(t *testing.T) {
		// Iteration-13 widening: a single actor that touches multiple kinds
		// in one 15-minute window collapses into ONE bulk_action card whose
		// KindCounts surfaces the breakdown. Previously each kind got its
		// own bucket and the rail showed 3 adjacent cards for one agent run.
		svc, queries, pool := newService(t)
		ctx := context.Background()
		seed := seedFeedFixture(t, queries, "mixedkind", 21)

		// Anchor inserts well inside a single 15-min bucket window so
		// the floor(unix/15m) key is identical for every annotation. We
		// step back 1 hour from real wall-time and snap to the bucket
		// centre — the resulting timestamp is comfortably inside the 3-day
		// fetch window and clear of any 15-min boundary.
		anchor := time.Now().UTC().Add(-1 * time.Hour).Truncate(15 * time.Minute).Add(7 * time.Minute)
		actorID := "actor-mixed-kind"
		var i int
		// 5 category_set
		for j := 0; j < 5; j++ {
			insertAnnotation(t, ctx, pool, queries, seed.TxnIDs[i],
				"category_set", "user", actorID, "Alice",
				pgtype.UUID{},
				[]byte(`{"category_slug":"food_and_drink"}`),
				anchor.Add(time.Duration(j)*time.Second),
			)
			i++
		}
		// 8 tag_removed
		for j := 0; j < 8; j++ {
			insertAnnotation(t, ctx, pool, queries, seed.TxnIDs[i],
				"tag_removed", "user", actorID, "Alice",
				pgtype.UUID{},
				[]byte(`{"tag_slug":"needs-review"}`),
				anchor.Add(time.Duration(10+j)*time.Second),
			)
			i++
		}
		// 8 comments from the same actor in the same window. Together with
		// the bursts above they prove the bucket is kind-agnostic — without
		// this iteration's widening, each kind would produce its own card.
		for j := 0; j < 8; j++ {
			insertAnnotation(t, ctx, pool, queries, seed.TxnIDs[i],
				"comment", "user", actorID, "Alice",
				pgtype.UUID{},
				[]byte(`{"content":"thinking"}`),
				anchor.Add(time.Duration(20+j)*time.Second),
			)
			i++
		}

		events, err := svc.ListFeedEvents(ctx, service.FeedEventsParams{})
		if err != nil {
			t.Fatalf("ListFeedEvents: %v", err)
		}
		if got := countEventsByType(events, "bulk_action"); got != 1 {
			t.Fatalf("expected 1 bulk_action event (kind-agnostic bucket), got %d", got)
		}
		if got := countEventsByType(events, "comment"); got != 0 {
			t.Errorf("expected 0 standalone comment events (folded into bulk), got %d", got)
		}
		ev := findEventByType(events, "bulk_action").BulkAction
		if ev.Count != 21 {
			t.Errorf("Count = %d, want 21", ev.Count)
		}
		if ev.Kind != "mixed" {
			t.Errorf("Kind = %q, want %q", ev.Kind, "mixed")
		}
		if ev.KindCounts["category_set"] != 5 {
			t.Errorf("KindCounts[category_set] = %d, want 5", ev.KindCounts["category_set"])
		}
		if ev.KindCounts["tag_removed"] != 8 {
			t.Errorf("KindCounts[tag_removed] = %d, want 8", ev.KindCounts["tag_removed"])
		}
		if ev.KindCounts["comment"] != 8 {
			t.Errorf("KindCounts[comment] = %d, want 8", ev.KindCounts["comment"])
		}
	})

	t.Run("CommentsBucketIfThree", func(t *testing.T) {
		// Three same-actor comments in a 15-min window collapse into a
		// bulk_action with Kind="comment" Count=3. Two comments in the same
		// window stay as individual comment cards (sub-threshold).
		t.Run("ThreeFold", func(t *testing.T) {
			svc, queries, pool := newService(t)
			ctx := context.Background()
			seed := seedFeedFixture(t, queries, "comments3", 3)

			anchor := time.Now().UTC().Add(-1 * time.Hour).Truncate(15 * time.Minute).Add(7 * time.Minute)
			actorID := "actor-3-comments"
			for j := 0; j < 3; j++ {
				insertAnnotation(t, ctx, pool, queries, seed.TxnIDs[j],
					"comment", "user", actorID, "Alice",
					pgtype.UUID{},
					[]byte(`{"content":"hi"}`),
					anchor.Add(time.Duration(j)*time.Second),
				)
			}

			events, err := svc.ListFeedEvents(ctx, service.FeedEventsParams{})
			if err != nil {
				t.Fatalf("ListFeedEvents: %v", err)
			}
			if got := countEventsByType(events, "bulk_action"); got != 1 {
				t.Fatalf("expected 1 bulk_action (≥3 comments), got %d", got)
			}
			if got := countEventsByType(events, "comment"); got != 0 {
				t.Errorf("expected 0 standalone comments, got %d", got)
			}
			ev := findEventByType(events, "bulk_action").BulkAction
			if ev.Kind != "comment" {
				t.Errorf("Kind = %q, want %q", ev.Kind, "comment")
			}
			if ev.Count != 3 {
				t.Errorf("Count = %d, want 3", ev.Count)
			}
		})
		t.Run("TwoStandalone", func(t *testing.T) {
			svc, queries, pool := newService(t)
			ctx := context.Background()
			seed := seedFeedFixture(t, queries, "comments2", 2)

			anchor := time.Now().UTC().Add(-1 * time.Hour).Truncate(15 * time.Minute).Add(7 * time.Minute)
			actorID := "actor-2-comments"
			for j := 0; j < 2; j++ {
				insertAnnotation(t, ctx, pool, queries, seed.TxnIDs[j],
					"comment", "user", actorID, "Alice",
					pgtype.UUID{},
					[]byte(`{"content":"hi"}`),
					anchor.Add(time.Duration(j)*time.Second),
				)
			}

			events, err := svc.ListFeedEvents(ctx, service.FeedEventsParams{})
			if err != nil {
				t.Fatalf("ListFeedEvents: %v", err)
			}
			if got := countEventsByType(events, "bulk_action"); got != 0 {
				t.Errorf("expected 0 bulk_action (sub-threshold), got %d", got)
			}
			if got := countEventsByType(events, "comment"); got != 2 {
				t.Errorf("expected 2 standalone comments, got %d", got)
			}
		})
	})

	t.Run("ReportFoldsIntoBulkAction", func(t *testing.T) {
		// A reporting agent run: 8 same-actor annotations in a 5-minute
		// span plus an agent report at the bucket center (same actor).
		// The report folds into the bulk_action card; only ONE event
		// surfaces, with the report title in the headline.
		svc, queries, pool := newService(t)
		ctx := context.Background()
		seed := seedFeedFixture(t, queries, "reportfold", 8)

		anchor := time.Now().UTC().Add(-1 * time.Hour).Truncate(15 * time.Minute).Add(7 * time.Minute)
		actorID := "agent-report-fold"
		for j := 0; j < 8; j++ {
			insertAnnotation(t, ctx, pool, queries, seed.TxnIDs[j],
				"category_set", "agent", actorID, "ReportingAgent",
				pgtype.UUID{},
				[]byte(`{"category_slug":"food_and_drink"}`),
				anchor.Add(time.Duration(j)*time.Second),
			)
		}

		// Report from the same agent created in the middle of the bucket.
		actor := service.Actor{Type: "agent", ID: actorID, Name: "ReportingAgent"}
		report, err := svc.CreateAgentReport(ctx, "Reviewed and cleared 8 transactions",
			"All 8 looked clean.",
			actor, "info", []string{"review"}, "", "")
		if err != nil {
			t.Fatalf("CreateAgentReport: %v", err)
		}
		// Backdate the report into the middle of the bulk_action's
		// time window so the actor+window fold check passes.
		if _, err := pool.Exec(ctx,
			"UPDATE agent_reports SET created_at = $1 WHERE id::text = $2",
			anchor.Add(10*time.Second), report.ID,
		); err != nil {
			t.Fatalf("backdate report: %v", err)
		}
		// Re-fetch the report so the SessionID/CreatedAt round-trip.
		fetched, err := svc.GetAgentReport(ctx, report.ID)
		if err != nil {
			t.Fatalf("GetAgentReport: %v", err)
		}

		events, leftover, err := svc.ListFeedEventsWithReports(ctx,
			service.FeedEventsParams{},
			[]service.AgentReportResponse{fetched},
		)
		if err != nil {
			t.Fatalf("ListFeedEventsWithReports: %v", err)
		}
		if got := countEventsByType(events, "bulk_action"); got != 1 {
			t.Fatalf("expected 1 bulk_action event, got %d", got)
		}
		if len(leftover) != 0 {
			t.Errorf("expected 0 leftover reports (folded into bulk), got %d (%+v)", len(leftover), leftover)
		}
		ev := findEventByType(events, "bulk_action").BulkAction
		if ev.Report == nil {
			t.Fatalf("BulkAction.Report = nil, want non-nil with title %q", "Reviewed and cleared 8 transactions")
		}
		if ev.Report.Title != "Reviewed and cleared 8 transactions" {
			t.Errorf("BulkAction.Report.Title = %q, want %q", ev.Report.Title, "Reviewed and cleared 8 transactions")
		}
	})

	t.Run("WindowIsBounded", func(t *testing.T) {
		svc, queries, pool := newService(t)
		ctx := context.Background()
		seed := seedFeedFixture(t, queries, "window", 1)

		// Annotation aged 4 days ago, with a 3-day window: must be excluded.
		old := time.Now().UTC().Add(-4 * 24 * time.Hour)
		insertAnnotation(t, ctx, pool, queries, seed.TxnIDs[0],
			"comment", "user", "actor-window", "Alice",
			pgtype.UUID{},
			[]byte(`{"content":"old"}`),
			old,
		)

		events, err := svc.ListFeedEvents(ctx, service.FeedEventsParams{
			Window: 3 * 24 * time.Hour,
		})
		if err != nil {
			t.Fatalf("ListFeedEvents: %v", err)
		}
		if len(events) != 0 {
			t.Errorf("expected 0 events with 3d window vs 4d-old annotation; got %d", len(events))
		}
	})
}
