//go:build integration && !lite

// Integration tests for DELETE /api/v1/transactions/{id} (soft-delete) and
// POST /api/v1/transactions/{id}/restore (undo). Run with:
//
//	DATABASE_URL="postgres://breadbox:breadbox@localhost:5432/breadbox_test?sslmode=disable" \
//	  go test -tags integration -count=1 -p 1 -v ./internal/api/... -run TestDeleteTransaction_ -run TestRestoreTransaction_
package api

import (
	"net/http"
	"testing"

	"breadbox/internal/pgconv"
	"breadbox/internal/service"
)

// TestDeleteTransaction_SoftDeletes confirms DELETE returns 204, the row
// disappears from GET, and the summary excludes it.
func TestDeleteTransaction_SoftDeletes(t *testing.T) {
	env := setupTestEnv(t)
	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	resp := env.doDelete(t, "/api/v1/transactions/"+txnID)
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()

	// Subsequent GET returns 404 — row is hidden from the read path.
	getResp := env.doGet(t, "/api/v1/transactions/"+txnID)
	readErrorCode(t, getResp, http.StatusNotFound, "NOT_FOUND")

	// CountTransactions filters on deleted_at IS NULL, so the deleted row
	// must not show up. We seeded exactly one txn — count should be zero.
	count, err := env.Service.CountTransactions(t.Context())
	if err != nil {
		t.Fatalf("count transactions: %v", err)
	}
	if count != 0 {
		t.Fatalf("want count=0 after delete, got %d", count)
	}

	// Annotation row is written attributed to the API key actor.
	annots, err := env.Service.ListAnnotations(t.Context(), txnID, service.ListAnnotationsParams{
		Kinds: []string{"transaction_deleted"},
		Raw:   true,
	})
	if err != nil {
		t.Fatalf("list annotations: %v", err)
	}
	if len(annots) != 1 {
		t.Fatalf("want 1 transaction_deleted annotation, got %d", len(annots))
	}
	if annots[0].ActorType != "agent" {
		t.Fatalf("want actor_type=agent, got %q", annots[0].ActorType)
	}
}

// TestDeleteTransaction_NotFound — DELETE on a fake UUID returns 404.
func TestDeleteTransaction_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doDelete(t, "/api/v1/transactions/00000000-0000-0000-0000-000000000000")
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

// TestDeleteTransaction_AlreadyDeleted — DELETE twice; second call returns 404.
func TestDeleteTransaction_AlreadyDeleted(t *testing.T) {
	env := setupTestEnv(t)
	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	first := env.doDelete(t, "/api/v1/transactions/"+txnID)
	assertStatus(t, first, http.StatusNoContent)
	first.Body.Close()

	second := env.doDelete(t, "/api/v1/transactions/"+txnID)
	readErrorCode(t, second, http.StatusNotFound, "NOT_FOUND")
}

// TestRestoreTransaction_Restores — soft-delete, then POST /restore;
// subsequent GET returns the row again.
func TestRestoreTransaction_Restores(t *testing.T) {
	env := setupTestEnv(t)
	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	delResp := env.doDelete(t, "/api/v1/transactions/"+txnID)
	assertStatus(t, delResp, http.StatusNoContent)
	delResp.Body.Close()

	resResp := env.doPost(t, "/api/v1/transactions/"+txnID+"/restore", nil)
	assertStatus(t, resResp, http.StatusNoContent)
	resResp.Body.Close()

	getResp := env.doGet(t, "/api/v1/transactions/"+txnID)
	assertStatus(t, getResp, http.StatusOK)
	getResp.Body.Close()

	// `transaction_restored` annotation is recorded.
	annots, err := env.Service.ListAnnotations(t.Context(), txnID, service.ListAnnotationsParams{
		Kinds: []string{"transaction_restored"},
		Raw:   true,
	})
	if err != nil {
		t.Fatalf("list annotations: %v", err)
	}
	if len(annots) != 1 {
		t.Fatalf("want 1 transaction_restored annotation, got %d", len(annots))
	}
}

// TestRestoreTransaction_NotDeleted — POST /restore on a live (non-deleted)
// transaction returns 404 NOT_FOUND.
func TestRestoreTransaction_NotDeleted(t *testing.T) {
	env := setupTestEnv(t)
	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	resp := env.doPost(t, "/api/v1/transactions/"+txnID+"/restore", nil)
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

// TestRestoreTransaction_NotFound — POST /restore on a fake UUID returns 404.
func TestRestoreTransaction_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doPost(t, "/api/v1/transactions/00000000-0000-0000-0000-000000000000/restore", nil)
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

// TestDeleteTransaction_RequiresWriteScope — read-only key gets 403
// INSUFFICIENT_SCOPE before reaching the handler.
func TestDeleteTransaction_RequiresWriteScope(t *testing.T) {
	env := setupReadOnlyEnv(t)
	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	resp := env.doDelete(t, "/api/v1/transactions/"+txnID)
	readErrorCode(t, resp, http.StatusForbidden, "INSUFFICIENT_SCOPE")
}

// TestRestoreTransaction_RequiresWriteScope — read-only key gets 403
// INSUFFICIENT_SCOPE before reaching the handler.
func TestRestoreTransaction_RequiresWriteScope(t *testing.T) {
	env := setupReadOnlyEnv(t)
	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	resp := env.doPost(t, "/api/v1/transactions/"+txnID+"/restore", nil)
	readErrorCode(t, resp, http.StatusForbidden, "INSUFFICIENT_SCOPE")
}

// TestDeleteTransaction_AcceptsShortID — DELETE works with a short_id, not
// just a UUID. Confirms resolveTransactionID's short-id path is wired into
// the handler.
func TestDeleteTransaction_AcceptsShortID(t *testing.T) {
	env := setupTestEnv(t)
	_, _, txn := seedFixture(t, env.Queries)

	if txn.ShortID == "" {
		t.Fatalf("expected short_id on seeded txn, got empty")
	}

	resp := env.doDelete(t, "/api/v1/transactions/"+txn.ShortID)
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()

	// The row should now be hidden — GET by short_id returns 404 because the
	// short-id resolver itself filters on deleted_at IS NULL.
	getResp := env.doGet(t, "/api/v1/transactions/"+txn.ShortID)
	readErrorCode(t, getResp, http.StatusNotFound, "NOT_FOUND")
}
