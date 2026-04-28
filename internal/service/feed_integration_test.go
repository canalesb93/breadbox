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

		now := time.Now().UTC()
		actorID := "user-actor-bulk"
		// 4 category_set annotations, same actor, 4 different transactions,
		// all within ~30 seconds (well inside the 5-minute soft bucket and
		// the 2-minute window the spec asks us to assert).
		for i := 0; i < 4; i++ {
			insertAnnotation(t, ctx, pool, queries, seed.TxnIDs[i],
				"category_set", "user", actorID, "Alice",
				pgtype.UUID{},
				[]byte(`{"category_slug":"food_and_drink"}`),
				now.Add(time.Duration(-30+i*10)*time.Second),
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

		now := time.Now().UTC()
		actorID := "actor-retro"
		// 5 rule_applied annotations, same actor, no `applied_by=sync`, all
		// within ~30 seconds → should produce a single bulk_action event.
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
				now.Add(time.Duration(-30+i*5)*time.Second), ann.ID,
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
