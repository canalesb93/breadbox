//go:build integration && !lite

// Integration tests for GET /api/v1/rules/{id}/sync-history. Run with:
//
//	DATABASE_URL="postgres://breadbox:breadbox@localhost:5432/breadbox_test?sslmode=disable" \
//	  go test -tags integration -count=1 -p 1 -v ./internal/api/... -run TestGetRuleSyncHistory_
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"breadbox/internal/service"
	"breadbox/internal/testutil"

	"github.com/jackc/pgx/v5/pgtype"
)

// seedSyncLogWithRuleHits inserts a sync_log row that records a hit for the
// given rule UUID. Apply does not write to sync_logs (only the sync engine
// does), so tests for the read-side handler seed the data directly.
func seedSyncLogWithRuleHits(t *testing.T, env *testEnv, connID pgtype.UUID, ruleUUID string, hits int, startedAt time.Time) {
	t.Helper()
	hitsJSON, err := json.Marshal(map[string]int{ruleUUID: hits})
	if err != nil {
		t.Fatalf("marshal rule hits: %v", err)
	}
	_, err = env.Pool.Exec(context.Background(),
		`INSERT INTO sync_logs (connection_id, "trigger", status, started_at, completed_at,
			added_count, modified_count, removed_count, rule_hits)
		 VALUES ($1, 'manual', 'success', $2, $2, 0, 0, 0, $3)`,
		connID, startedAt, hitsJSON)
	if err != nil {
		t.Fatalf("seed sync_log row: %v", err)
	}
}

// syncHistoryFixture seeds a user, connection, account, and a rule. Returns
// the connection UUID (for sync_logs) and the rule's UUID id + short id.
func syncHistoryFixture(t *testing.T, env *testEnv) (connID pgtype.UUID, ruleID string, ruleShortID string) {
	t.Helper()
	cat := testutil.MustCreateCategory(t, env.Queries, "groceries", "Groceries")
	user := testutil.MustCreateUser(t, env.Queries, "Sync History User")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "synchist_item")
	_ = testutil.MustCreateAccount(t, env.Queries, conn.ID, "synchist_acct", "Checking")

	resp := env.doPost(t, "/api/v1/rules", map[string]any{
		"name":          "Grocery Rule",
		"category_slug": cat.Slug,
		"trigger":       "always",
		"conditions":    map[string]any{"field": "provider_name", "op": "contains", "value": "grocer"},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create rule: status %d", resp.StatusCode)
	}
	var rule struct {
		ID      string `json:"id"`
		ShortID string `json:"short_id"`
	}
	parseJSON(t, resp, &rule)
	if rule.ID == "" {
		t.Fatalf("rule has no id")
	}

	// Resolve to the canonical UUID used as the JSONB key in sync_logs.rule_hits.
	got, err := env.Service.GetTransactionRule(context.Background(), rule.ID)
	if err != nil {
		t.Fatalf("get rule: %v", err)
	}
	return conn.ID, got.ID, got.ShortID
}

// TestGetRuleSyncHistory_AfterApply — seeds one sync_log row that records
// the rule's hit, then asserts the GET returns that one entry.
func TestGetRuleSyncHistory_AfterApply(t *testing.T) {
	env := setupTestEnv(t)
	connID, ruleID, _ := syncHistoryFixture(t, env)
	seedSyncLogWithRuleHits(t, env, connID, ruleID, 7, time.Now().Add(-1*time.Hour))

	resp := env.doGet(t, "/api/v1/rules/"+ruleID+"/sync-history")
	assertStatus(t, resp, http.StatusOK)

	var out struct {
		History []map[string]any `json:"history"`
	}
	parseJSON(t, resp, &out)
	if len(out.History) != 1 {
		t.Fatalf("want 1 history entry, got %d (%+v)", len(out.History), out.History)
	}
	entry := out.History[0]
	// hit_count is decoded as float64 from JSON.
	if hc, ok := entry["hit_count"].(float64); !ok || hc != 7 {
		t.Fatalf("want hit_count=7, got %v", entry["hit_count"])
	}
	if entry["status"] != "success" {
		t.Fatalf("want status=success, got %v", entry["status"])
	}
	if _, ok := entry["sync_id"].(string); !ok {
		t.Fatalf("want sync_id string, got %v", entry["sync_id"])
	}
}

// TestGetRuleSyncHistory_LimitParam — seeds 3 sync rows, requests limit=2,
// and asserts only 2 come back (and they are the most recent).
func TestGetRuleSyncHistory_LimitParam(t *testing.T) {
	env := setupTestEnv(t)
	connID, ruleID, _ := syncHistoryFixture(t, env)
	now := time.Now()
	seedSyncLogWithRuleHits(t, env, connID, ruleID, 1, now.Add(-3*time.Hour))
	seedSyncLogWithRuleHits(t, env, connID, ruleID, 2, now.Add(-2*time.Hour))
	seedSyncLogWithRuleHits(t, env, connID, ruleID, 3, now.Add(-1*time.Hour))

	resp := env.doGet(t, "/api/v1/rules/"+ruleID+"/sync-history?limit=2")
	assertStatus(t, resp, http.StatusOK)

	var out struct {
		History []map[string]any `json:"history"`
	}
	parseJSON(t, resp, &out)
	if len(out.History) != 2 {
		t.Fatalf("want 2 entries (limit=2), got %d", len(out.History))
	}
	// Service orders DESC by started_at, so the first entry is the most recent (hit_count=3).
	if hc, _ := out.History[0]["hit_count"].(float64); hc != 3 {
		t.Fatalf("want most-recent hit_count=3 first, got %v", out.History[0]["hit_count"])
	}
	if hc, _ := out.History[1]["hit_count"].(float64); hc != 2 {
		t.Fatalf("want second hit_count=2, got %v", out.History[1]["hit_count"])
	}
}

// TestGetRuleSyncHistory_NotFound — non-existent rule returns 404.
func TestGetRuleSyncHistory_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doGet(t, "/api/v1/rules/00000000-0000-0000-0000-000000000000/sync-history")
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

// TestGetRuleSyncHistory_AcceptsShortID — short_id resolves correctly.
func TestGetRuleSyncHistory_AcceptsShortID(t *testing.T) {
	env := setupTestEnv(t)
	connID, ruleID, ruleShortID := syncHistoryFixture(t, env)
	seedSyncLogWithRuleHits(t, env, connID, ruleID, 5, time.Now().Add(-30*time.Minute))

	if ruleShortID == "" {
		t.Fatalf("rule short_id missing")
	}

	resp := env.doGet(t, "/api/v1/rules/"+ruleShortID+"/sync-history")
	assertStatus(t, resp, http.StatusOK)

	var out struct {
		History []map[string]any `json:"history"`
	}
	parseJSON(t, resp, &out)
	if len(out.History) != 1 {
		t.Fatalf("want 1 entry via short_id, got %d", len(out.History))
	}
}

// TestGetRuleSyncHistory_AllowsReadScope — read-only API key can call this
// endpoint (it's mounted under the read group).
func TestGetRuleSyncHistory_AllowsReadScope(t *testing.T) {
	env := setupReadOnlyEnv(t)
	// Create a rule via the service layer directly (read-only key can't POST /rules).
	cat := testutil.MustCreateCategory(t, env.Queries, "groceries", "Groceries")
	rule, err := env.Service.CreateTransactionRule(context.Background(), service.CreateTransactionRuleParams{
		Name:         "ReadScope Rule",
		CategorySlug: cat.Slug,
		Trigger:      "always",
		Conditions: service.Condition{
			Field: "provider_name",
			Op:    "contains",
			Value: "x",
		},
		Actor: service.SystemActor(),
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}

	resp := env.doGet(t, "/api/v1/rules/"+rule.ID+"/sync-history")
	assertStatus(t, resp, http.StatusOK)

	var out struct {
		History []map[string]any `json:"history"`
	}
	parseJSON(t, resp, &out)
	if len(out.History) != 0 {
		t.Fatalf("want empty history (no sync rows seeded), got %d", len(out.History))
	}
}
