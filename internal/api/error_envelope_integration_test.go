//go:build integration && !lite

// Integration tests covering the canonical error envelope returned by REST
// handlers — specifically the sentinel-to-status mapping that flows through
// writeServiceError. These tests pin behavior so future refactors can't
// regress 404s back into 400s the way MarkReportReadHandler did before
// bundle 07.

package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"breadbox/internal/pgconv"
)

// ---------- Reports ----------

func TestReports_MarkRead_NotFound(t *testing.T) {
	env := setupTestEnv(t)

	// A syntactically-valid UUID that doesn't correspond to any report.
	resp := env.doPatch(t, "/api/v1/reports/00000000-0000-0000-0000-000000000000/read", nil)
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestReports_MarkRead_InvalidID(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doPatch(t, "/api/v1/reports/not-a-uuid/read", nil)
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

// ---------- Comments ----------

func TestComments_Update_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	// Comment ID is a valid UUID that does not exist.
	resp := env.doPut(
		t,
		fmt.Sprintf("/api/v1/transactions/%s/comments/00000000-0000-0000-0000-000000000000", txnID),
		map[string]string{"content": "updated"},
	)
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestComments_Delete_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	resp := env.doDelete(
		t,
		fmt.Sprintf("/api/v1/transactions/%s/comments/00000000-0000-0000-0000-000000000000", txnID),
	)
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestComments_Update_EmptyContent_InvalidParameter(t *testing.T) {
	env := setupTestEnv(t)
	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	// Create a real comment so we exercise the validation path, not NOT_FOUND.
	create := env.doPost(t, "/api/v1/transactions/"+txnID+"/comments", map[string]string{
		"content": "original",
	})
	assertStatus(t, create, http.StatusCreated)
	var comment map[string]any
	parseJSON(t, create, &comment)
	commentID := comment["id"].(string)

	resp := env.doPut(
		t,
		fmt.Sprintf("/api/v1/transactions/%s/comments/%s", txnID, commentID),
		map[string]string{"content": ""},
	)
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

func TestComments_Create_EmptyContent_InvalidParameter(t *testing.T) {
	env := setupTestEnv(t)
	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	resp := env.doPost(t, "/api/v1/transactions/"+txnID+"/comments", map[string]string{
		"content": "",
	})
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

func TestComments_Create_TooLong_InvalidParameter(t *testing.T) {
	env := setupTestEnv(t)
	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	long := strings.Repeat("x", 100_000)
	resp := env.doPost(t, "/api/v1/transactions/"+txnID+"/comments", map[string]string{
		"content": long,
	})
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}
