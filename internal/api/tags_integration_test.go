//go:build integration

// Integration tests for REST tag endpoints and the tags filter on
// /api/v1/transactions. Run with:
//   DATABASE_URL="postgres://breadbox:breadbox@localhost:5432/breadbox_test?sslmode=disable" \
//   go test -tags integration -count=1 -p 1 -v ./internal/api/... -run TestAPI_Tag
package api

import (
	"net/http"
	"testing"

	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	"breadbox/internal/testutil"
)

// TestAPI_ListTags_Empty returns an empty list when no tags are registered.
func TestAPI_ListTags_Empty(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doGet(t, "/api/v1/tags")
	assertStatus(t, resp, http.StatusOK)

	var body struct {
		Tags []map[string]any `json:"tags"`
	}
	parseJSON(t, resp, &body)
	if len(body.Tags) != 0 {
		t.Fatalf("want empty tags list, got %d entries", len(body.Tags))
	}
}

// TestAPI_ListTags_WithData returns registered tags.
func TestAPI_ListTags_WithData(t *testing.T) {
	env := setupTestEnv(t)

	if _, err := env.Service.CreateTag(t.Context(), service.CreateTagParams{
		Slug:        "needs-review",
		DisplayName: "Needs Review",
	}); err != nil {
		t.Fatalf("create tag: %v", err)
	}

	resp := env.doGet(t, "/api/v1/tags")
	assertStatus(t, resp, http.StatusOK)

	var body struct {
		Tags []struct {
			Slug        string `json:"slug"`
			DisplayName string `json:"display_name"`
		} `json:"tags"`
	}
	parseJSON(t, resp, &body)
	if len(body.Tags) != 1 {
		t.Fatalf("want 1 tag, got %d", len(body.Tags))
	}
	if body.Tags[0].Slug != "needs-review" {
		t.Fatalf("want slug needs-review, got %q", body.Tags[0].Slug)
	}
	if body.Tags[0].DisplayName != "Needs Review" {
		t.Fatalf("want display_name %q, got %q", "Needs Review", body.Tags[0].DisplayName)
	}
}

// TestAPI_AddTransactionTag_Success attaches a tag to a transaction and
// confirms the response envelope. The tag is auto-created by the service.
func TestAPI_AddTransactionTag_Success(t *testing.T) {
	env := setupTestEnv(t)

	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	resp := env.doPost(t, "/api/v1/transactions/"+txnID+"/tags", map[string]string{
		"slug": "needs-review",
	})
	assertStatus(t, resp, http.StatusOK)

	var body struct {
		Added          bool   `json:"added"`
		AlreadyPresent bool   `json:"already_present"`
		Slug           string `json:"slug"`
	}
	parseJSON(t, resp, &body)
	if !body.Added || body.AlreadyPresent {
		t.Fatalf("want added=true already_present=false, got %+v", body)
	}
	if body.Slug != "needs-review" {
		t.Fatalf("want slug needs-review, got %q", body.Slug)
	}
}

// TestAPI_AddTransactionTag_Idempotent returns already_present=true on the
// second identical call, matching the MCP tool's semantics.
func TestAPI_AddTransactionTag_Idempotent(t *testing.T) {
	env := setupTestEnv(t)

	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	first := env.doPost(t, "/api/v1/transactions/"+txnID+"/tags", map[string]string{"slug": "x-flag"})
	assertStatus(t, first, http.StatusOK)
	first.Body.Close()

	second := env.doPost(t, "/api/v1/transactions/"+txnID+"/tags", map[string]string{"slug": "x-flag"})
	assertStatus(t, second, http.StatusOK)

	var body struct {
		Added          bool `json:"added"`
		AlreadyPresent bool `json:"already_present"`
	}
	parseJSON(t, second, &body)
	if body.Added || !body.AlreadyPresent {
		t.Fatalf("want added=false already_present=true, got %+v", body)
	}
}

// TestAPI_AddTransactionTag_MissingSlug rejects an empty slug.
func TestAPI_AddTransactionTag_MissingSlug(t *testing.T) {
	env := setupTestEnv(t)

	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	resp := env.doPost(t, "/api/v1/transactions/"+txnID+"/tags", map[string]string{})
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

// TestAPI_AddTransactionTag_TransactionNotFound returns 404 for unknown
// short IDs. A bogus short ID routes through resolveTransactionID and yields
// ErrNotFound (a UUID that is merely well-formed but absent would bypass
// resolution and surface an FK error from the DB insert instead — a separate
// concern tracked in the service layer).
func TestAPI_AddTransactionTag_TransactionNotFound(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doPost(t, "/api/v1/transactions/zzzzzzzz/tags", map[string]string{
		"slug": "needs-review",
	})
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

// TestAPI_AddTransactionTag_ReadOnlyBlocked confirms write-scope enforcement.
func TestAPI_AddTransactionTag_ReadOnlyBlocked(t *testing.T) {
	env := setupReadOnlyEnv(t)

	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	resp := env.doPost(t, "/api/v1/transactions/"+txnID+"/tags", map[string]string{
		"slug": "needs-review",
	})
	readErrorCode(t, resp, http.StatusForbidden, "INSUFFICIENT_SCOPE")
}

// TestAPI_RemoveTransactionTag_Success detaches a previously attached tag.
func TestAPI_RemoveTransactionTag_Success(t *testing.T) {
	env := setupTestEnv(t)

	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	add := env.doPost(t, "/api/v1/transactions/"+txnID+"/tags", map[string]string{"slug": "needs-review"})
	assertStatus(t, add, http.StatusOK)
	add.Body.Close()

	del := env.doDelete(t, "/api/v1/transactions/"+txnID+"/tags/needs-review")
	assertStatus(t, del, http.StatusOK)

	var body struct {
		Removed       bool `json:"removed"`
		AlreadyAbsent bool `json:"already_absent"`
	}
	parseJSON(t, del, &body)
	if !body.Removed || body.AlreadyAbsent {
		t.Fatalf("want removed=true already_absent=false, got %+v", body)
	}
}

// TestAPI_RemoveTransactionTag_AlreadyAbsent is a no-op when the tag isn't
// attached — mirrors the MCP tool's already_absent contract.
func TestAPI_RemoveTransactionTag_AlreadyAbsent(t *testing.T) {
	env := setupTestEnv(t)

	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	resp := env.doDelete(t, "/api/v1/transactions/"+txnID+"/tags/never-attached")
	assertStatus(t, resp, http.StatusOK)

	var body struct {
		Removed       bool `json:"removed"`
		AlreadyAbsent bool `json:"already_absent"`
	}
	parseJSON(t, resp, &body)
	if body.Removed || !body.AlreadyAbsent {
		t.Fatalf("want removed=false already_absent=true, got %+v", body)
	}
}

// TestAPI_ListTransactions_TagsFilter_AND ensures the tags filter applies
// AND semantics (transaction must have every listed tag).
func TestAPI_ListTransactions_TagsFilter_AND(t *testing.T) {
	env := setupTestEnv(t)

	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "item_1")
	acct := testutil.MustCreateAccount(t, env.Queries, conn.ID, "ext_acct_1", "Checking")
	txnA := testutil.MustCreateTransaction(t, env.Queries, acct.ID, "ext_txn_A", "Coffee", 450, "2025-03-15")
	txnB := testutil.MustCreateTransaction(t, env.Queries, acct.ID, "ext_txn_B", "Grocery", 1200, "2025-03-14")

	txnAID := pgconv.FormatUUID(txnA.ID)
	txnBID := pgconv.FormatUUID(txnB.ID)

	// txnA has both tags, txnB has only "needs-review".
	mustAddTag(t, env, txnAID, "needs-review")
	mustAddTag(t, env, txnAID, "urgent")
	mustAddTag(t, env, txnBID, "needs-review")

	// tags=needs-review,urgent must return txnA only (AND).
	resp := env.doGet(t, "/api/v1/transactions?tags=needs-review,urgent")
	assertStatus(t, resp, http.StatusOK)
	var body struct {
		Transactions []struct {
			ID string `json:"id"`
		} `json:"transactions"`
	}
	parseJSON(t, resp, &body)
	if len(body.Transactions) != 1 {
		t.Fatalf("want 1 transaction, got %d (body=%+v)", len(body.Transactions), body)
	}
	if body.Transactions[0].ID != txnAID {
		t.Fatalf("want txn %s, got %s", txnAID, body.Transactions[0].ID)
	}
}

// TestAPI_ListTransactions_AnyTagFilter_OR ensures any_tag applies OR
// semantics (at least one listed tag).
func TestAPI_ListTransactions_AnyTagFilter_OR(t *testing.T) {
	env := setupTestEnv(t)

	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "item_1")
	acct := testutil.MustCreateAccount(t, env.Queries, conn.ID, "ext_acct_1", "Checking")
	txnA := testutil.MustCreateTransaction(t, env.Queries, acct.ID, "ext_txn_A", "Coffee", 450, "2025-03-15")
	txnB := testutil.MustCreateTransaction(t, env.Queries, acct.ID, "ext_txn_B", "Grocery", 1200, "2025-03-14")
	// txnC has no tags — must be excluded.
	_ = testutil.MustCreateTransaction(t, env.Queries, acct.ID, "ext_txn_C", "Rent", 90000, "2025-03-13")

	txnAID := pgconv.FormatUUID(txnA.ID)
	txnBID := pgconv.FormatUUID(txnB.ID)

	mustAddTag(t, env, txnAID, "needs-review")
	mustAddTag(t, env, txnBID, "urgent")

	resp := env.doGet(t, "/api/v1/transactions?any_tag=needs-review,urgent")
	assertStatus(t, resp, http.StatusOK)

	var body struct {
		Transactions []struct {
			ID string `json:"id"`
		} `json:"transactions"`
	}
	parseJSON(t, resp, &body)
	if len(body.Transactions) != 2 {
		t.Fatalf("want 2 transactions, got %d (body=%+v)", len(body.Transactions), body)
	}
	ids := map[string]bool{}
	for _, tx := range body.Transactions {
		ids[tx.ID] = true
	}
	if !ids[txnAID] || !ids[txnBID] {
		t.Fatalf("expected txnA and txnB in results, got %+v", ids)
	}
}

// mustAddTag attaches a tag via the REST endpoint or fails the test. Keeps
// individual tests focused on the assertion under test.
func mustAddTag(t *testing.T, env *testEnv, txnID, slug string) {
	t.Helper()
	resp := env.doPost(t, "/api/v1/transactions/"+txnID+"/tags", map[string]string{"slug": slug})
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}
