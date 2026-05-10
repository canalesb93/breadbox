//go:build integration

package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/testutil"

	"github.com/jackc/pgx/v5/pgtype"
)

// Helper: insert a sync_log row directly via Queries with explicit
// status / trigger / started_at, returning the created row.
func mustCreateSyncLog(
	t *testing.T,
	q *db.Queries,
	connID pgtype.UUID,
	trigger db.SyncTrigger,
	status db.SyncStatus,
	startedAt time.Time,
) db.SyncLog {
	t.Helper()
	log, err := q.CreateSyncLog(context.Background(), db.CreateSyncLogParams{
		ConnectionID: connID,
		Trigger:      trigger,
		Status:       status,
		StartedAt:    pgtype.Timestamptz{Time: startedAt, Valid: true},
		// completed_at is optional; for "in_progress" we leave it null.
		CompletedAt: pgtype.Timestamptz{
			Time:  startedAt.Add(2 * time.Second),
			Valid: status != db.SyncStatusInProgress,
		},
	})
	if err != nil {
		t.Fatalf("CreateSyncLog: %v", err)
	}
	return log
}

// ============================================================
// GET /sync/logs
// ============================================================

func TestListSyncLogs_Empty(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doGet(t, "/api/v1/sync/logs")
	assertStatus(t, resp, http.StatusOK)

	var body struct {
		SyncLogs []map[string]any `json:"sync_logs"`
		HasMore  bool             `json:"has_more"`
		Limit    int              `json:"limit"`
		Total    int64            `json:"total"`
	}
	parseJSON(t, resp, &body)
	if len(body.SyncLogs) != 0 {
		t.Fatalf("want empty sync_logs, got %d", len(body.SyncLogs))
	}
	if body.HasMore {
		t.Fatalf("want has_more=false, got true")
	}
	if body.Total != 0 {
		t.Fatalf("want total=0, got %d", body.Total)
	}
	if body.Limit != 50 {
		t.Fatalf("want default limit=50, got %d", body.Limit)
	}
}

func TestListSyncLogs_AfterTriggers(t *testing.T) {
	env := setupTestEnv(t)
	waitForSyncDrain(t, env)

	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_after_1")
	testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_after_2")

	// Trigger a sync. The engine has no providers wired, so the goroutine
	// will fail fast — but the in-progress sync_log row is written
	// synchronously, which is enough to populate the list.
	r1 := env.doPost(t, "/api/v1/sync", nil)
	assertStatus(t, r1, http.StatusAccepted)
	r1.Body.Close()
	r2 := env.doPost(t, "/api/v1/sync", nil)
	assertStatus(t, r2, http.StatusAccepted)
	r2.Body.Close()

	// Give the background sync goroutines a beat to insert their rows.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var n int64
		if err := env.Pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM sync_logs").Scan(&n); err == nil && n >= 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	resp := env.doGet(t, "/api/v1/sync/logs")
	assertStatus(t, resp, http.StatusOK)

	var body struct {
		SyncLogs []map[string]any `json:"sync_logs"`
		Total    int64            `json:"total"`
	}
	parseJSON(t, resp, &body)
	if len(body.SyncLogs) == 0 {
		t.Fatalf("want at least one sync log row, got 0 (total=%d)", body.Total)
	}
}

func TestListSyncLogs_FilterByConnection(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	connA := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_filter_a")
	connB := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_filter_b")

	now := time.Now().UTC()
	mustCreateSyncLog(t, env.Queries, connA.ID, db.SyncTriggerManual, db.SyncStatusSuccess, now.Add(-3*time.Minute))
	mustCreateSyncLog(t, env.Queries, connA.ID, db.SyncTriggerManual, db.SyncStatusSuccess, now.Add(-2*time.Minute))
	mustCreateSyncLog(t, env.Queries, connB.ID, db.SyncTriggerManual, db.SyncStatusSuccess, now.Add(-1*time.Minute))

	resp := env.doGet(t, "/api/v1/sync/logs?connection_id="+pgconv.FormatUUID(connA.ID))
	assertStatus(t, resp, http.StatusOK)

	var body struct {
		SyncLogs []map[string]any `json:"sync_logs"`
		Total    int64            `json:"total"`
	}
	parseJSON(t, resp, &body)
	if body.Total != 2 {
		t.Fatalf("want total=2, got %d", body.Total)
	}
	for _, row := range body.SyncLogs {
		if row["connection_id"] != pgconv.FormatUUID(connA.ID) {
			t.Fatalf("want only connA rows, saw connection_id=%v", row["connection_id"])
		}
	}
}

func TestListSyncLogs_FilterByStatus(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_status")

	now := time.Now().UTC()
	mustCreateSyncLog(t, env.Queries, conn.ID, db.SyncTriggerManual, db.SyncStatusSuccess, now.Add(-3*time.Minute))
	mustCreateSyncLog(t, env.Queries, conn.ID, db.SyncTriggerManual, db.SyncStatusInProgress, now.Add(-2*time.Minute))
	mustCreateSyncLog(t, env.Queries, conn.ID, db.SyncTriggerManual, db.SyncStatusError, now.Add(-1*time.Minute))

	resp := env.doGet(t, "/api/v1/sync/logs?status=in_progress")
	assertStatus(t, resp, http.StatusOK)

	var body struct {
		SyncLogs []map[string]any `json:"sync_logs"`
		Total    int64            `json:"total"`
	}
	parseJSON(t, resp, &body)
	if body.Total != 1 {
		t.Fatalf("want total=1 in_progress, got %d", body.Total)
	}
	if len(body.SyncLogs) != 1 || body.SyncLogs[0]["status"] != "in_progress" {
		t.Fatalf("want one in_progress row, got %+v", body.SyncLogs)
	}
}

func TestListSyncLogs_FilterByTrigger(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_trig")

	now := time.Now().UTC()
	mustCreateSyncLog(t, env.Queries, conn.ID, db.SyncTriggerCron, db.SyncStatusSuccess, now.Add(-3*time.Minute))
	mustCreateSyncLog(t, env.Queries, conn.ID, db.SyncTriggerManual, db.SyncStatusSuccess, now.Add(-2*time.Minute))
	mustCreateSyncLog(t, env.Queries, conn.ID, db.SyncTriggerWebhook, db.SyncStatusSuccess, now.Add(-1*time.Minute))

	resp := env.doGet(t, "/api/v1/sync/logs?trigger=manual")
	assertStatus(t, resp, http.StatusOK)

	var body struct {
		SyncLogs []map[string]any `json:"sync_logs"`
		Total    int64            `json:"total"`
	}
	parseJSON(t, resp, &body)
	if body.Total != 1 {
		t.Fatalf("want total=1 manual, got %d", body.Total)
	}
	if len(body.SyncLogs) != 1 || body.SyncLogs[0]["trigger"] != "manual" {
		t.Fatalf("want one manual row, got %+v", body.SyncLogs)
	}
}

func TestListSyncLogs_DateRange(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_daterange")

	// Three logs spanning three days.
	t1 := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC)
	t3 := time.Date(2025, 1, 3, 12, 0, 0, 0, time.UTC)
	mustCreateSyncLog(t, env.Queries, conn.ID, db.SyncTriggerManual, db.SyncStatusSuccess, t1)
	mustCreateSyncLog(t, env.Queries, conn.ID, db.SyncTriggerManual, db.SyncStatusSuccess, t2)
	mustCreateSyncLog(t, env.Queries, conn.ID, db.SyncTriggerManual, db.SyncStatusSuccess, t3)

	from := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	to := time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	q := url.Values{}
	q.Set("from", from)
	q.Set("to", to)

	resp := env.doGet(t, "/api/v1/sync/logs?"+q.Encode())
	assertStatus(t, resp, http.StatusOK)

	var body struct {
		SyncLogs []map[string]any `json:"sync_logs"`
		Total    int64            `json:"total"`
	}
	parseJSON(t, resp, &body)
	if body.Total != 1 {
		t.Fatalf("want total=1 (only Jan 2 log), got %d", body.Total)
	}
}

func TestListSyncLogs_Cursor(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_cursor")

	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		mustCreateSyncLog(t, env.Queries, conn.ID, db.SyncTriggerManual, db.SyncStatusSuccess, now.Add(-time.Duration(i)*time.Minute))
	}

	// First page: limit=2 returns rows 1+2 of 5; expect has_more + cursor.
	resp1 := env.doGet(t, "/api/v1/sync/logs?limit=2")
	assertStatus(t, resp1, http.StatusOK)
	var p1 struct {
		SyncLogs   []map[string]any `json:"sync_logs"`
		NextCursor *string          `json:"next_cursor"`
		HasMore    bool             `json:"has_more"`
		Total      int64            `json:"total"`
	}
	parseJSON(t, resp1, &p1)
	if !p1.HasMore || p1.NextCursor == nil || *p1.NextCursor == "" {
		t.Fatalf("want has_more + cursor on first page, got has_more=%v cursor=%v", p1.HasMore, p1.NextCursor)
	}
	if p1.Total != 5 {
		t.Fatalf("want total=5, got %d", p1.Total)
	}
	if len(p1.SyncLogs) != 2 {
		t.Fatalf("want 2 rows on page 1, got %d", len(p1.SyncLogs))
	}

	// Second page using cursor.
	resp2 := env.doGet(t, "/api/v1/sync/logs?limit=2&cursor="+*p1.NextCursor)
	assertStatus(t, resp2, http.StatusOK)
	var p2 struct {
		SyncLogs   []map[string]any `json:"sync_logs"`
		NextCursor *string          `json:"next_cursor"`
		HasMore    bool             `json:"has_more"`
	}
	parseJSON(t, resp2, &p2)
	if len(p2.SyncLogs) != 2 {
		t.Fatalf("want 2 rows on page 2, got %d", len(p2.SyncLogs))
	}
	if !p2.HasMore || p2.NextCursor == nil {
		t.Fatalf("want has_more on page 2 (3 left, limit 2), got has_more=%v", p2.HasMore)
	}

	// Page 1 and page 2 should have disjoint IDs.
	seen := make(map[string]bool)
	for _, r := range p1.SyncLogs {
		seen[r["id"].(string)] = true
	}
	for _, r := range p2.SyncLogs {
		if seen[r["id"].(string)] {
			t.Fatalf("page 2 returned a duplicate id %v from page 1", r["id"])
		}
	}

	// Third page exhausts the result.
	resp3 := env.doGet(t, "/api/v1/sync/logs?limit=2&cursor="+*p2.NextCursor)
	assertStatus(t, resp3, http.StatusOK)
	var p3 struct {
		SyncLogs []map[string]any `json:"sync_logs"`
		HasMore  bool             `json:"has_more"`
	}
	parseJSON(t, resp3, &p3)
	if p3.HasMore {
		t.Fatalf("want has_more=false on final page, got true")
	}
	if len(p3.SyncLogs) != 1 {
		t.Fatalf("want 1 row on final page (5 rows / limit 2), got %d", len(p3.SyncLogs))
	}
}

func TestListSyncLogs_BadFilter(t *testing.T) {
	env := setupTestEnv(t)

	cases := []struct {
		name string
		path string
	}{
		{"bad_status", "/api/v1/sync/logs?status=garbage"},
		{"bad_trigger", "/api/v1/sync/logs?trigger=garbage"},
		{"bad_from", "/api/v1/sync/logs?from=not-a-date"},
		{"bad_to", "/api/v1/sync/logs?to=2025-13-99"},
		{"negative_limit", "/api/v1/sync/logs?limit=-1"},
		{"too_large_limit", "/api/v1/sync/logs?limit=10000"},
		{"from_after_to", "/api/v1/sync/logs?from=2025-01-02T00:00:00Z&to=2025-01-01T00:00:00Z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := env.doGet(t, tc.path)
			readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
		})
	}

	// And bad cursor → INVALID_CURSOR.
	resp := env.doGet(t, "/api/v1/sync/logs?cursor=not-base64-json")
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_CURSOR")
}

// ============================================================
// GET /sync/logs/{id}
// ============================================================

func TestGetSyncLog_Success(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_detail")
	acct := testutil.MustCreateAccount(t, env.Queries, conn.ID, "ext_detail_acct", "Checking")

	now := time.Now().UTC()
	log := mustCreateSyncLog(t, env.Queries, conn.ID, db.SyncTriggerManual, db.SyncStatusSuccess, now.Add(-1*time.Minute))

	// Seed two per-account rows so the embedded `accounts` array is populated.
	if err := env.Queries.InsertSyncLogAccount(context.Background(), db.InsertSyncLogAccountParams{
		SyncLogID:      log.ID,
		AccountID:      pgtype.UUID{Bytes: acct.ID.Bytes, Valid: true},
		AccountName:    "Checking",
		AddedCount:     3,
		ModifiedCount:  1,
		RemovedCount:   0,
		UnchangedCount: 5,
	}); err != nil {
		t.Fatalf("InsertSyncLogAccount: %v", err)
	}

	resp := env.doGet(t, "/api/v1/sync/logs/"+pgconv.FormatUUID(log.ID))
	assertStatus(t, resp, http.StatusOK)

	var body struct {
		ID       string                   `json:"id"`
		Status   string                   `json:"status"`
		Accounts []map[string]any         `json:"accounts"`
		RuleHits []map[string]any         `json:"rule_hits"`
	}
	parseJSON(t, resp, &body)
	if body.ID != pgconv.FormatUUID(log.ID) {
		t.Fatalf("want id %s, got %s", pgconv.FormatUUID(log.ID), body.ID)
	}
	if body.Status != "success" {
		t.Fatalf("want status success, got %s", body.Status)
	}
	if len(body.Accounts) != 1 {
		t.Fatalf("want 1 account row, got %d", len(body.Accounts))
	}
	if body.Accounts[0]["account_name"] != "Checking" {
		t.Fatalf("want account_name=Checking, got %v", body.Accounts[0]["account_name"])
	}
}

func TestGetSyncLog_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doGet(t, "/api/v1/sync/logs/00000000-0000-0000-0000-000000000000")
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

// ============================================================
// GET /sync/health
// ============================================================

func TestSyncHealth_Aggregate(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_health")

	now := time.Now().UTC()
	mustCreateSyncLog(t, env.Queries, conn.ID, db.SyncTriggerManual, db.SyncStatusSuccess, now.Add(-1*time.Hour))
	mustCreateSyncLog(t, env.Queries, conn.ID, db.SyncTriggerManual, db.SyncStatusSuccess, now.Add(-30*time.Minute))
	mustCreateSyncLog(t, env.Queries, conn.ID, db.SyncTriggerManual, db.SyncStatusError, now.Add(-15*time.Minute))

	resp := env.doGet(t, "/api/v1/sync/health")
	assertStatus(t, resp, http.StatusOK)

	var body struct {
		OverallHealth     string  `json:"overall_health"`
		LastSyncStatus    string  `json:"last_sync_status"`
		RecentSyncCount   int64   `json:"recent_sync_count"`
		RecentErrorCount  int64   `json:"recent_error_count"`
		RecentSuccessRate float64 `json:"recent_success_rate"`
	}
	parseJSON(t, resp, &body)
	if body.RecentSyncCount != 3 {
		t.Fatalf("want recent_sync_count=3, got %d", body.RecentSyncCount)
	}
	if body.RecentErrorCount != 1 {
		t.Fatalf("want recent_error_count=1, got %d", body.RecentErrorCount)
	}
	// Last sync (most recent started_at) is the error one.
	if body.LastSyncStatus != "error" {
		t.Fatalf("want last_sync_status=error, got %s", body.LastSyncStatus)
	}
	if body.OverallHealth == "" {
		t.Fatalf("want overall_health populated, got empty")
	}
}

// ============================================================
// GET /sync/health/providers
// ============================================================

func TestSyncHealthProviders(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	plaidConn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_p")
	tellerConn := testutil.MustCreateTellerConnection(t, env.Queries, user.ID, "ext_t")

	now := time.Now().UTC()
	mustCreateSyncLog(t, env.Queries, plaidConn.ID, db.SyncTriggerManual, db.SyncStatusSuccess, now.Add(-1*time.Hour))
	mustCreateSyncLog(t, env.Queries, tellerConn.ID, db.SyncTriggerManual, db.SyncStatusError, now.Add(-30*time.Minute))

	resp := env.doGet(t, "/api/v1/sync/health/providers")
	assertStatus(t, resp, http.StatusOK)

	var body struct {
		Providers map[string]map[string]any `json:"providers"`
	}
	parseJSON(t, resp, &body)
	if _, ok := body.Providers["plaid"]; !ok {
		t.Fatalf("want plaid entry, got providers=%+v", body.Providers)
	}
	if _, ok := body.Providers["teller"]; !ok {
		t.Fatalf("want teller entry, got providers=%+v", body.Providers)
	}
	if body.Providers["teller"]["last_sync_status"] != "error" {
		t.Fatalf("want teller last_sync_status=error, got %v", body.Providers["teller"]["last_sync_status"])
	}
}

// ============================================================
// GET /sync/stats
// ============================================================

func TestSyncStats_MatchesFilter(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_stats")

	now := time.Now().UTC()
	mustCreateSyncLog(t, env.Queries, conn.ID, db.SyncTriggerManual, db.SyncStatusSuccess, now.Add(-3*time.Minute))
	mustCreateSyncLog(t, env.Queries, conn.ID, db.SyncTriggerManual, db.SyncStatusSuccess, now.Add(-2*time.Minute))
	mustCreateSyncLog(t, env.Queries, conn.ID, db.SyncTriggerCron, db.SyncStatusError, now.Add(-1*time.Minute))

	// Unfiltered: 3 total, 2 success, 1 error.
	resp := env.doGet(t, "/api/v1/sync/stats")
	assertStatus(t, resp, http.StatusOK)
	var body struct {
		TotalSyncs   int64   `json:"total_syncs"`
		SuccessCount int64   `json:"success_count"`
		ErrorCount   int64   `json:"error_count"`
		SuccessRate  float64 `json:"success_rate"`
	}
	parseJSON(t, resp, &body)
	if body.TotalSyncs != 3 || body.SuccessCount != 2 || body.ErrorCount != 1 {
		t.Fatalf("unfiltered stats: total=%d success=%d error=%d", body.TotalSyncs, body.SuccessCount, body.ErrorCount)
	}

	// Filter to manual trigger only: 2 total, 2 success, 0 error.
	resp2 := env.doGet(t, "/api/v1/sync/stats?trigger=manual")
	assertStatus(t, resp2, http.StatusOK)
	var body2 struct {
		TotalSyncs   int64 `json:"total_syncs"`
		SuccessCount int64 `json:"success_count"`
		ErrorCount   int64 `json:"error_count"`
	}
	parseJSON(t, resp2, &body2)
	if body2.TotalSyncs != 2 || body2.SuccessCount != 2 || body2.ErrorCount != 0 {
		t.Fatalf("manual-filtered stats: total=%d success=%d error=%d", body2.TotalSyncs, body2.SuccessCount, body2.ErrorCount)
	}
}

// ============================================================
// Scope check — read-only API key works on all 5 GETs
// ============================================================

func TestSyncEndpoints_AllowReadOnlyKey(t *testing.T) {
	env := setupReadOnlyEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_ro")
	now := time.Now().UTC()
	log := mustCreateSyncLog(t, env.Queries, conn.ID, db.SyncTriggerManual, db.SyncStatusSuccess, now.Add(-1*time.Minute))

	paths := []string{
		"/api/v1/sync/logs",
		"/api/v1/sync/logs/" + pgconv.FormatUUID(log.ID),
		"/api/v1/sync/health",
		"/api/v1/sync/health/providers",
		"/api/v1/sync/stats",
	}
	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			resp := env.doGet(t, p)
			assertStatus(t, resp, http.StatusOK)
			resp.Body.Close()
		})
	}
}

// Quick sanity: the cursor we hand out is base64-RawURL of JSON {"p":N}. The
// public API contract treats it as opaque, but the tests want to be sure it
// doesn't accidentally leak something else (e.g. an integer, or PII).
func TestListSyncLogs_CursorShape(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_cshape")
	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		mustCreateSyncLog(t, env.Queries, conn.ID, db.SyncTriggerManual, db.SyncStatusSuccess, now.Add(-time.Duration(i)*time.Minute))
	}

	resp := env.doGet(t, "/api/v1/sync/logs?limit=1")
	assertStatus(t, resp, http.StatusOK)
	var body struct {
		NextCursor *string `json:"next_cursor"`
	}
	parseJSON(t, resp, &body)
	if body.NextCursor == nil {
		t.Fatalf("want next_cursor populated")
	}
	raw, err := base64.RawURLEncoding.DecodeString(*body.NextCursor)
	if err != nil {
		t.Fatalf("cursor not base64-RawURL: %v", err)
	}
	var payload map[string]int
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("cursor payload not JSON: %v", err)
	}
	if payload["p"] != 2 {
		t.Fatalf("want page=2 in cursor, got %v", payload)
	}
}
