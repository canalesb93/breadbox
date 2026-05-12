//go:build integration && !lite

// Integration tests for POST /api/v1/transactions/update — the REST sibling
// of the MCP `update_transactions` tool. Run with:
//
//	DATABASE_URL="postgres://breadbox:breadbox@localhost:5432/breadbox_test?sslmode=disable" \
//	  go test -tags integration -count=1 -p 1 -v ./internal/api/... -run TestUpdateTransactions_
package api

import (
	"net/http"
	"testing"

	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	"breadbox/internal/testutil"
)

// TestUpdateTransactions_AtomicMultiField applies category + tag-add + comment
// in a single op and confirms all three writes landed.
func TestUpdateTransactions_AtomicMultiField(t *testing.T) {
	env := setupTestEnv(t)
	_ = seedUncategorized(t, env.Queries)
	cat := testutil.MustCreateCategory(t, env.Queries, "food_and_drink_groceries", "Groceries")

	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	body := map[string]any{
		"operations": []map[string]any{{
			"transaction_id": txnID,
			"category_slug":  cat.Slug,
			"tags_to_add":    []map[string]string{{"slug": "needs-review"}},
			"comment":        "clearly groceries",
		}},
	}
	resp := env.doPost(t, "/api/v1/transactions/update", body)
	assertStatus(t, resp, http.StatusOK)

	var out struct {
		Results []struct {
			TransactionID string `json:"transaction_id"`
			Status        string `json:"status"`
		} `json:"results"`
		Succeeded int `json:"succeeded"`
		Failed    int `json:"failed"`
	}
	parseJSON(t, resp, &out)
	if out.Succeeded != 1 || out.Failed != 0 {
		t.Fatalf("want succeeded=1 failed=0, got succeeded=%d failed=%d (results=%+v)", out.Succeeded, out.Failed, out.Results)
	}
	if len(out.Results) != 1 || out.Results[0].Status != "ok" {
		t.Fatalf("want one ok result, got %+v", out.Results)
	}

	// Verify the transaction is recategorized (override set to cat).
	got, err := env.Service.GetTransaction(t.Context(), txnID)
	if err != nil {
		t.Fatalf("get transaction: %v", err)
	}
	if got.Category == nil || got.Category.Slug == nil || *got.Category.Slug != cat.Slug {
		t.Fatalf("want category %q, got %+v", cat.Slug, got.Category)
	}
	if !got.CategoryOverride {
		t.Fatalf("want category_override=true after manual set")
	}

	// Verify the tag is attached.
	tagsAttached, err := env.Queries.ListTagsByTransaction(t.Context(), txn.ID)
	if err != nil {
		t.Fatalf("list tags for txn: %v", err)
	}
	foundTag := false
	for _, tg := range tagsAttached {
		if tg.Slug == "needs-review" {
			foundTag = true
		}
	}
	if !foundTag {
		t.Fatalf("expected tag needs-review attached, got %+v", tagsAttached)
	}

	// Verify a comment annotation row exists.
	annots, err := env.Service.ListAnnotations(t.Context(), txnID, service.ListAnnotationsParams{
		Kinds: []string{"comment"},
		Raw:   true,
	})
	if err != nil {
		t.Fatalf("list annotations: %v", err)
	}
	if len(annots) == 0 {
		t.Fatalf("expected at least one comment annotation, got 0")
	}
}

// TestUpdateTransactions_BatchContinueMode runs two ops; the second references
// a non-existent transaction. Continue mode commits the first op and reports
// the second op's per-row error inside results[].
func TestUpdateTransactions_BatchContinueMode(t *testing.T) {
	env := setupTestEnv(t)
	_ = seedUncategorized(t, env.Queries)
	cat := testutil.MustCreateCategory(t, env.Queries, "groceries", "Groceries")

	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	body := map[string]any{
		"on_error": "continue",
		"operations": []map[string]any{
			{
				"transaction_id": txnID,
				"category_slug":  cat.Slug,
			},
			{
				"transaction_id": "zzzzzzzz", // non-existent short_id
				"category_slug":  cat.Slug,
			},
		},
	}
	resp := env.doPost(t, "/api/v1/transactions/update", body)
	assertStatus(t, resp, http.StatusOK)

	var out struct {
		Results []struct {
			TransactionID string `json:"transaction_id"`
			Status        string `json:"status"`
			Error         *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error,omitempty"`
		} `json:"results"`
		Succeeded int `json:"succeeded"`
		Failed    int `json:"failed"`
	}
	parseJSON(t, resp, &out)
	if out.Succeeded != 1 || out.Failed != 1 {
		t.Fatalf("want succeeded=1 failed=1, got succeeded=%d failed=%d (results=%+v)", out.Succeeded, out.Failed, out.Results)
	}
	if out.Results[0].Status != "ok" {
		t.Fatalf("want first op ok, got %+v", out.Results[0])
	}
	if out.Results[1].Status != "error" || out.Results[1].Error == nil || out.Results[1].Error.Code != "NOT_FOUND" {
		t.Fatalf("want second op error NOT_FOUND, got %+v", out.Results[1])
	}

	// First op committed.
	got, err := env.Service.GetTransaction(t.Context(), txnID)
	if err != nil {
		t.Fatalf("get transaction: %v", err)
	}
	if got.Category == nil || got.Category.Slug == nil || *got.Category.Slug != cat.Slug {
		t.Fatalf("first op should have committed category, got %+v", got.Category)
	}
}

// TestUpdateTransactions_BatchAbortMode is the same shape as continue mode
// but with on_error=abort. The whole batch rolls back when the second op
// fails — the first op's category change must NOT persist.
func TestUpdateTransactions_BatchAbortMode(t *testing.T) {
	env := setupTestEnv(t)
	uncat := seedUncategorized(t, env.Queries)
	cat := testutil.MustCreateCategory(t, env.Queries, "groceries", "Groceries")

	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	body := map[string]any{
		"on_error": "abort",
		"operations": []map[string]any{
			{
				"transaction_id": txnID,
				"category_slug":  cat.Slug,
			},
			{
				"transaction_id": "zzzzzzzz",
				"category_slug":  cat.Slug,
			},
		},
	}
	resp := env.doPost(t, "/api/v1/transactions/update", body)
	assertStatus(t, resp, http.StatusOK)

	var out struct {
		Results []struct {
			TransactionID string `json:"transaction_id"`
			Status        string `json:"status"`
			Error         *struct {
				Code string `json:"code"`
			} `json:"error,omitempty"`
		} `json:"results"`
		Succeeded int  `json:"succeeded"`
		Failed    int  `json:"failed"`
		Aborted   bool `json:"aborted"`
	}
	parseJSON(t, resp, &out)
	if !out.Aborted {
		t.Fatalf("want aborted=true on abort failure, got %+v", out)
	}
	// The service contract for updateTransactionsAbort: each op result
	// reflects what happened at op-time (op[0] computed ok before op[1]
	// failed). Trailing ops AFTER the failure get an ABORTED placeholder.
	// The DB tx is rolled back regardless, which is what the
	// "first-op rollback" assertion below actually proves.
	if out.Results[1].Status != "error" || out.Results[1].Error == nil || out.Results[1].Error.Code != "NOT_FOUND" {
		t.Fatalf("want second op error NOT_FOUND, got %+v", out.Results[1])
	}

	// First op rolled back: txn must still carry the original category
	// (uncategorized seeded by MustCreateTransaction's default).
	got, err := env.Service.GetTransaction(t.Context(), txnID)
	if err != nil {
		t.Fatalf("get transaction: %v", err)
	}
	if got.Category != nil && got.Category.Slug != nil && *got.Category.Slug == cat.Slug {
		t.Fatalf("abort mode should have rolled back; category should not be %q (uncat=%s)", cat.Slug, uncat.Slug)
	}
	if got.CategoryOverride {
		t.Fatalf("abort mode should have rolled back; category_override should be false")
	}
}

// TestUpdateTransactions_ResetCategory clears a manual override.
func TestUpdateTransactions_ResetCategory(t *testing.T) {
	env := setupTestEnv(t)
	_ = seedUncategorized(t, env.Queries)
	cat := testutil.MustCreateCategory(t, env.Queries, "groceries", "Groceries")

	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	// Set a manual override first via the service. SetTransactionCategory
	// accepts UUID/short_id (not slug), so use the short_id.
	if err := env.Service.SetTransactionCategory(t.Context(), txnID, cat.ShortID, service.SystemActor()); err != nil {
		t.Fatalf("seed override: %v", err)
	}

	body := map[string]any{
		"operations": []map[string]any{{
			"transaction_id": txnID,
			"reset_category": true,
		}},
	}
	resp := env.doPost(t, "/api/v1/transactions/update", body)
	assertStatus(t, resp, http.StatusOK)

	var out struct {
		Succeeded int `json:"succeeded"`
		Failed    int `json:"failed"`
	}
	parseJSON(t, resp, &out)
	if out.Succeeded != 1 || out.Failed != 0 {
		t.Fatalf("want succeeded=1 failed=0, got %+v", out)
	}

	got, err := env.Service.GetTransaction(t.Context(), txnID)
	if err != nil {
		t.Fatalf("get transaction: %v", err)
	}
	if got.CategoryOverride {
		t.Fatalf("expected category_override cleared after reset, still true")
	}
}

// TestUpdateTransactions_RejectsEmptyOperations — empty operations array is
// a top-level INVALID_PARAMETER (400), not a per-op error.
func TestUpdateTransactions_RejectsEmptyOperations(t *testing.T) {
	env := setupTestEnv(t)

	body := map[string]any{"operations": []map[string]any{}}
	resp := env.doPost(t, "/api/v1/transactions/update", body)
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

// TestUpdateTransactions_RejectsTooMany — > 50 ops returns 400.
func TestUpdateTransactions_RejectsTooMany(t *testing.T) {
	env := setupTestEnv(t)

	ops := make([]map[string]any, 51)
	for i := range ops {
		ops[i] = map[string]any{"transaction_id": "zzzzzzzz"}
	}
	body := map[string]any{"operations": ops}
	resp := env.doPost(t, "/api/v1/transactions/update", body)
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

// TestUpdateTransactions_RequiresWriteScope — read-only API key is blocked.
func TestUpdateTransactions_RequiresWriteScope(t *testing.T) {
	env := setupReadOnlyEnv(t)

	body := map[string]any{
		"operations": []map[string]any{{"transaction_id": "zzzzzzzz"}},
	}
	resp := env.doPost(t, "/api/v1/transactions/update", body)
	readErrorCode(t, resp, http.StatusForbidden, "INSUFFICIENT_SCOPE")
}

// TestUpdateTransactions_RejectsBothCategoryAndReset — category_slug and
// reset_category are mutually exclusive. The service rejects per-op, so the
// top-level call still returns 200 but the result reports INVALID_PARAMETER.
func TestUpdateTransactions_RejectsBothCategoryAndReset(t *testing.T) {
	env := setupTestEnv(t)
	_ = seedUncategorized(t, env.Queries)
	cat := testutil.MustCreateCategory(t, env.Queries, "groceries", "Groceries")

	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	body := map[string]any{
		"operations": []map[string]any{{
			"transaction_id": txnID,
			"category_slug":  cat.Slug,
			"reset_category": true,
		}},
	}
	resp := env.doPost(t, "/api/v1/transactions/update", body)
	assertStatus(t, resp, http.StatusOK)

	var out struct {
		Results []struct {
			Status string `json:"status"`
			Error  *struct {
				Code string `json:"code"`
			} `json:"error,omitempty"`
		} `json:"results"`
		Failed int `json:"failed"`
	}
	parseJSON(t, resp, &out)
	if out.Failed != 1 {
		t.Fatalf("want failed=1, got %+v", out)
	}
	if out.Results[0].Status != "error" || out.Results[0].Error == nil || out.Results[0].Error.Code != "INVALID_PARAMETER" {
		t.Fatalf("want INVALID_PARAMETER per-op error, got %+v", out.Results[0])
	}
}

// TestUpdateTransactions_AutoCreatesTagSlug — referencing a brand-new tag
// slug in tags_to_add auto-creates it (matches MCP behavior via
// getOrCreateTagBySlug).
func TestUpdateTransactions_AutoCreatesTagSlug(t *testing.T) {
	env := setupTestEnv(t)
	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	const newSlug = "freshly-minted"

	body := map[string]any{
		"operations": []map[string]any{{
			"transaction_id": txnID,
			"tags_to_add":    []map[string]string{{"slug": newSlug}},
		}},
	}
	resp := env.doPost(t, "/api/v1/transactions/update", body)
	assertStatus(t, resp, http.StatusOK)

	var out struct {
		Succeeded int `json:"succeeded"`
		Failed    int `json:"failed"`
	}
	parseJSON(t, resp, &out)
	if out.Succeeded != 1 || out.Failed != 0 {
		t.Fatalf("want succeeded=1 failed=0, got %+v", out)
	}

	// Verify the tag was registered (auto-created).
	tags, err := env.Service.ListTags(t.Context())
	if err != nil {
		t.Fatalf("list tags: %v", err)
	}
	found := false
	for _, tg := range tags {
		if tg.Slug == newSlug {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected auto-created tag %q in registry, got %+v", newSlug, tags)
	}
}
