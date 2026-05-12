//go:build integration && !lite

// Integration tests for the user CRUD endpoints (GET /users/{id}, POST
// /users, PATCH /users/{id}, DELETE /users/{id}, POST /users/{id}/wipe-data).
// The {id} URL param accepts UUID or short_id — see service.resolveUserID.
//
// Run with: DATABASE_URL=... go test -tags integration -count=1 -p 1 -v -run User ./internal/api/...

package api

import (
	"net/http"
	"testing"

	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	"breadbox/internal/testutil"
)

// --- helpers ---------------------------------------------------------------

func createUserVia(t *testing.T, env *testEnv, body map[string]any) service.UserResponse {
	t.Helper()
	resp := env.doPost(t, "/api/v1/users", body)
	assertStatus(t, resp, http.StatusCreated)
	var user service.UserResponse
	parseJSON(t, resp, &user)
	return user
}

// --- GET /users/{id} -------------------------------------------------------

func TestGetUser_Success(t *testing.T) {
	env := setupTestEnv(t)
	u := testutil.MustCreateUser(t, env.Queries, "Alice")

	resp := env.doGet(t, "/api/v1/users/"+pgconv.FormatUUID(u.ID))
	assertStatus(t, resp, http.StatusOK)

	var got service.UserResponse
	parseJSON(t, resp, &got)
	if got.Name != "Alice" {
		t.Errorf("name: got %q, want Alice", got.Name)
	}
	if got.ShortID == "" {
		t.Errorf("short_id should be set")
	}
	if got.ID != pgconv.FormatUUID(u.ID) {
		t.Errorf("id: got %q, want %q", got.ID, pgconv.FormatUUID(u.ID))
	}
}

func TestGetUser_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doGet(t, "/api/v1/users/00000000-0000-0000-0000-000000000000")
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestGetUser_AcceptsShortID(t *testing.T) {
	env := setupTestEnv(t)
	u := testutil.MustCreateUser(t, env.Queries, "Bob")

	resp := env.doGet(t, "/api/v1/users/"+u.ShortID)
	assertStatus(t, resp, http.StatusOK)

	var got service.UserResponse
	parseJSON(t, resp, &got)
	if got.ShortID != u.ShortID {
		t.Errorf("short_id: got %q, want %q", got.ShortID, u.ShortID)
	}
}

// --- POST /users -----------------------------------------------------------

func TestCreateUser_Success(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doPost(t, "/api/v1/users", map[string]any{
		"name":  "Charlie",
		"email": "charlie@example.com",
	})
	assertStatus(t, resp, http.StatusCreated)

	var got service.UserResponse
	parseJSON(t, resp, &got)
	if got.Name != "Charlie" {
		t.Errorf("name: got %q, want Charlie", got.Name)
	}
	if got.Email == nil || *got.Email != "charlie@example.com" {
		t.Errorf("email: got %v, want charlie@example.com", got.Email)
	}
	if got.ID == "" || got.ShortID == "" {
		t.Errorf("id and short_id must be set")
	}
}

func TestCreateUser_EmptyName(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doPost(t, "/api/v1/users", map[string]any{
		"name": "   ",
	})
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

func TestCreateUser_DuplicateEmail(t *testing.T) {
	env := setupTestEnv(t)
	_ = createUserVia(t, env, map[string]any{
		"name":  "First",
		"email": "dup@example.com",
	})
	resp := env.doPost(t, "/api/v1/users", map[string]any{
		"name":  "Second",
		"email": "dup@example.com",
	})
	readErrorCode(t, resp, http.StatusConflict, "EMAIL_CONFLICT")
}

// --- PATCH /users/{id} -----------------------------------------------------

func TestUpdateUser_PartialPatch(t *testing.T) {
	env := setupTestEnv(t)
	u := createUserVia(t, env, map[string]any{
		"name":  "Original",
		"email": "orig@example.com",
	})

	newName := "Renamed"
	resp := env.doPatch(t, "/api/v1/users/"+u.ID, map[string]any{
		"name": newName,
	})
	assertStatus(t, resp, http.StatusOK)

	var got service.UserResponse
	parseJSON(t, resp, &got)
	if got.Name != newName {
		t.Errorf("name: got %q, want %q", got.Name, newName)
	}
	if got.Email == nil || *got.Email != "orig@example.com" {
		t.Errorf("email should remain unchanged, got %v", got.Email)
	}
}

func TestUpdateUser_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doPatch(t, "/api/v1/users/00000000-0000-0000-0000-000000000000", map[string]any{
		"name": "Anyone",
	})
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

// --- DELETE /users/{id} ----------------------------------------------------

func TestDeleteUser_NoDependents(t *testing.T) {
	env := setupTestEnv(t)
	u := testutil.MustCreateUser(t, env.Queries, "Detachable")

	resp := env.doDelete(t, "/api/v1/users/"+pgconv.FormatUUID(u.ID))
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()

	get := env.doGet(t, "/api/v1/users/"+pgconv.FormatUUID(u.ID))
	readErrorCode(t, get, http.StatusNotFound, "NOT_FOUND")
}

func TestDeleteUser_BlockedByDependents(t *testing.T) {
	env := setupTestEnv(t)
	u := testutil.MustCreateUser(t, env.Queries, "WithConnection")
	_ = testutil.MustCreateConnection(t, env.Queries, u.ID, "item_block_delete")

	resp := env.doDelete(t, "/api/v1/users/"+pgconv.FormatUUID(u.ID))
	readErrorCode(t, resp, http.StatusConflict, "USER_HAS_DEPENDENTS")
}

func TestDeleteUser_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doDelete(t, "/api/v1/users/00000000-0000-0000-0000-000000000000")
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

// --- POST /users/{id}/wipe-data --------------------------------------------

func TestWipeUserData_RemovesAttributedRows(t *testing.T) {
	env := setupTestEnv(t)
	u := testutil.MustCreateUser(t, env.Queries, "Wipey")
	conn := testutil.MustCreateConnection(t, env.Queries, u.ID, "item_wipe")
	acct := testutil.MustCreateAccount(t, env.Queries, conn.ID, "ext_acct_wipe", "Checking")
	for i, ext := range []string{"a", "b", "c"} {
		_ = testutil.MustCreateTransaction(t, env.Queries, acct.ID, "ext_txn_wipe_"+ext, "Coffee", int64(100+i), "2025-01-15")
	}

	resp := env.doPost(t, "/api/v1/users/"+pgconv.FormatUUID(u.ID)+"/wipe-data", map[string]any{})
	assertStatus(t, resp, http.StatusOK)

	var body struct {
		Status              string `json:"status"`
		DeletedTransactions int64  `json:"deleted_transactions"`
	}
	parseJSON(t, resp, &body)
	if body.Status != "ok" {
		t.Errorf("status: got %q, want ok", body.Status)
	}
	if body.DeletedTransactions != 3 {
		t.Errorf("deleted_transactions: got %d, want 3", body.DeletedTransactions)
	}

	count, err := env.Queries.CountConnectionsByUser(t.Context(), u.ID)
	if err != nil {
		t.Fatalf("count connections: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 connections after wipe, got %d", count)
	}
}

func TestWipeUserData_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	resp := env.doPost(t, "/api/v1/users/00000000-0000-0000-0000-000000000000/wipe-data", map[string]any{})
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

// --- Scope checks ----------------------------------------------------------

func TestUserCrud_RequiresWriteScope(t *testing.T) {
	env := setupReadOnlyEnv(t)
	u := testutil.MustCreateUser(t, env.Queries, "ScopedReadOnly")
	uid := pgconv.FormatUUID(u.ID)

	cases := []struct {
		name string
		do   func() *http.Response
	}{
		{"POST /users", func() *http.Response {
			return env.doPost(t, "/api/v1/users", map[string]any{"name": "Nope"})
		}},
		{"PATCH /users/{id}", func() *http.Response {
			return env.doPatch(t, "/api/v1/users/"+uid, map[string]any{"name": "Renamed"})
		}},
		{"DELETE /users/{id}", func() *http.Response {
			return env.doDelete(t, "/api/v1/users/"+uid)
		}},
		{"POST /users/{id}/wipe-data", func() *http.Response {
			return env.doPost(t, "/api/v1/users/"+uid+"/wipe-data", map[string]any{})
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			readErrorCode(t, c.do(), http.StatusForbidden, "INSUFFICIENT_SCOPE")
		})
	}
}

func TestGetUser_AllowsReadScope(t *testing.T) {
	env := setupReadOnlyEnv(t)
	u := testutil.MustCreateUser(t, env.Queries, "ReadableUser")

	resp := env.doGet(t, "/api/v1/users/"+pgconv.FormatUUID(u.ID))
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}
