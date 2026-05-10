//go:build integration

package api

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	"breadbox/internal/testutil"
)

// ============================================================
// Account-links matches — cursor pagination
// ============================================================

// seedMatches creates a link between two accounts and N matched
// transaction pairs across distinct dates so cursor pagination is stable.
func seedMatches(t *testing.T, env *testEnv, n int) (linkID string) {
	t.Helper()

	// Use the test name as a unique suffix on every external ID so concurrent
	// test runs against the shared dev DB don't trip unique-constraint /
	// truncation races on the bank_connections.external_id column.
	suffix := fmt.Sprintf("%d_%p", time.Now().UnixNano(), t)
	user1 := testutil.MustCreateUser(t, env.Queries, "Alice_"+suffix)
	conn1 := testutil.MustCreateConnection(t, env.Queries, user1.ID, "item_p_"+suffix)
	acct1 := testutil.MustCreateAccount(t, env.Queries, conn1.ID, "ext_p_"+suffix, "Primary")

	user2 := testutil.MustCreateUser(t, env.Queries, "Bob_"+suffix)
	conn2 := testutil.MustCreateConnection(t, env.Queries, user2.ID, "item_d_"+suffix)
	acct2 := testutil.MustCreateAccount(t, env.Queries, conn2.ID, "ext_d_"+suffix, "Dependent")

	link, err := env.Service.CreateAccountLink(context.Background(), service.CreateAccountLinkParams{
		PrimaryAccountID:   pgconv.FormatUUID(acct1.ID),
		DependentAccountID: pgconv.FormatUUID(acct2.ID),
	})
	if err != nil {
		t.Fatalf("CreateAccountLink: %v", err)
	}

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		// distinct date per pair so the date-desc ordering is deterministic
		date := base.AddDate(0, 0, i).Format("2006-01-02")
		amount := int64(1000 + i)
		txnP := testutil.MustCreateTransaction(t, env.Queries, acct1.ID, fmt.Sprintf("txn_p_%s_%d", suffix, i), "Store", amount, date)
		txnD := testutil.MustCreateTransaction(t, env.Queries, acct2.ID, fmt.Sprintf("txn_d_%s_%d", suffix, i), "Store", amount, date)

		if _, err := env.Service.ManualMatch(context.Background(), link.ID, pgconv.FormatUUID(txnP.ID), pgconv.FormatUUID(txnD.ID)); err != nil {
			t.Fatalf("ManualMatch %d: %v", i, err)
		}
	}
	return link.ID
}

// emptyLink returns a link with no matched transactions.
func emptyLink(t *testing.T, env *testEnv) string {
	t.Helper()

	suffix := fmt.Sprintf("%d_%p", time.Now().UnixNano(), t)
	user1 := testutil.MustCreateUser(t, env.Queries, "Alice_"+suffix)
	conn1 := testutil.MustCreateConnection(t, env.Queries, user1.ID, "item_e1_"+suffix)
	acct1 := testutil.MustCreateAccount(t, env.Queries, conn1.ID, "ext_e1_"+suffix, "PrimaryE")

	user2 := testutil.MustCreateUser(t, env.Queries, "Bob_"+suffix)
	conn2 := testutil.MustCreateConnection(t, env.Queries, user2.ID, "item_e2_"+suffix)
	acct2 := testutil.MustCreateAccount(t, env.Queries, conn2.ID, "ext_e2_"+suffix, "DependentE")

	link, err := env.Service.CreateAccountLink(context.Background(), service.CreateAccountLinkParams{
		PrimaryAccountID:   pgconv.FormatUUID(acct1.ID),
		DependentAccountID: pgconv.FormatUUID(acct2.ID),
	})
	if err != nil {
		t.Fatalf("CreateAccountLink: %v", err)
	}
	return link.ID
}

type matchesResponse struct {
	Matches    []map[string]any `json:"matches"`
	NextCursor string           `json:"next_cursor"`
	HasMore    bool             `json:"has_more"`
	Limit      int              `json:"limit"`
}

func TestListMatches_DefaultLimit(t *testing.T) {
	env := setupTestEnv(t)
	// Seed a small number; the response should still echo the default limit
	// of 50 even when there are fewer rows.
	link := seedMatches(t, env, 3)

	resp := env.doGet(t, "/api/v1/account-links/"+link+"/matches")
	assertStatus(t, resp, http.StatusOK)

	var got matchesResponse
	parseJSON(t, resp, &got)
	if got.Limit != 50 {
		t.Errorf("expected default limit 50, got %d", got.Limit)
	}
	if len(got.Matches) != 3 {
		t.Errorf("expected 3 matches, got %d", len(got.Matches))
	}
	if got.HasMore {
		t.Errorf("expected has_more=false when fewer than limit are seeded, got true")
	}
}

func TestListMatches_LimitParam(t *testing.T) {
	env := setupTestEnv(t)
	link := seedMatches(t, env, 10)

	resp := env.doGet(t, "/api/v1/account-links/"+link+"/matches?limit=3")
	assertStatus(t, resp, http.StatusOK)

	var got matchesResponse
	parseJSON(t, resp, &got)
	if got.Limit != 3 {
		t.Errorf("expected limit=3 echoed, got %d", got.Limit)
	}
	if len(got.Matches) != 3 {
		t.Errorf("expected 3 matches, got %d", len(got.Matches))
	}
	if !got.HasMore {
		t.Errorf("expected has_more=true with 10 seeded and limit=3")
	}
}

func TestListMatches_Cursor(t *testing.T) {
	env := setupTestEnv(t)
	link := seedMatches(t, env, 6)

	// First page of 3
	resp := env.doGet(t, "/api/v1/account-links/"+link+"/matches?limit=3")
	assertStatus(t, resp, http.StatusOK)
	var page1 matchesResponse
	parseJSON(t, resp, &page1)
	if len(page1.Matches) != 3 || !page1.HasMore || page1.NextCursor == "" {
		t.Fatalf("page1 unexpected: matches=%d has_more=%v cursor=%q", len(page1.Matches), page1.HasMore, page1.NextCursor)
	}

	// Second page
	resp = env.doGet(t, "/api/v1/account-links/"+link+"/matches?limit=3&cursor="+page1.NextCursor)
	assertStatus(t, resp, http.StatusOK)
	var page2 matchesResponse
	parseJSON(t, resp, &page2)
	if len(page2.Matches) != 3 {
		t.Errorf("expected 3 matches on page2, got %d", len(page2.Matches))
	}
	if page2.HasMore {
		t.Errorf("expected has_more=false on final page")
	}

	// No overlap between page1 and page2 IDs
	seen := map[string]bool{}
	for _, m := range page1.Matches {
		seen[m["id"].(string)] = true
	}
	for _, m := range page2.Matches {
		if seen[m["id"].(string)] {
			t.Errorf("match %v appeared in both pages", m["id"])
		}
	}
}

func TestListMatches_EmptyAccountLink(t *testing.T) {
	env := setupTestEnv(t)
	link := emptyLink(t, env)

	resp := env.doGet(t, "/api/v1/account-links/"+link+"/matches")
	assertStatus(t, resp, http.StatusOK)

	var got matchesResponse
	parseJSON(t, resp, &got)
	if len(got.Matches) != 0 {
		t.Errorf("expected 0 matches, got %d", len(got.Matches))
	}
	if got.HasMore {
		t.Errorf("expected has_more=false on empty link")
	}
	if got.NextCursor != "" {
		t.Errorf("expected empty next_cursor on empty link, got %q", got.NextCursor)
	}
}

func TestListMatches_BadCursor(t *testing.T) {
	env := setupTestEnv(t)
	link := emptyLink(t, env)

	resp := env.doGet(t, "/api/v1/account-links/"+link+"/matches?cursor=NOT_A_CURSOR")
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_CURSOR")
}

