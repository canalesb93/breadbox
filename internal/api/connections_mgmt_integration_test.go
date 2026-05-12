//go:build integration && !lite

package api

import (
	"net/http"
	"testing"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/testutil"
)

// connectionDetail mirrors the JSON shape of service.ConnectionDetailResponse
// for assertions in these tests. Only the fields tests inspect are listed.
type connectionDetail struct {
	ID                          string  `json:"id"`
	ShortID                     string  `json:"short_id"`
	Provider                    string  `json:"provider"`
	Status                      string  `json:"status"`
	Paused                      bool    `json:"paused"`
	SyncIntervalOverrideMinutes *int32  `json:"sync_interval_override_minutes"`
	AccountCount                int     `json:"account_count"`
	UserID                      *string `json:"user_id"`
}

func TestGetConnection_Success(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_get_ok")
	testutil.MustCreateAccount(t, env.Queries, conn.ID, "ext_acct_get_ok", "Checking")

	resp := env.doGet(t, "/api/v1/connections/"+pgconv.FormatUUID(conn.ID))
	assertStatus(t, resp, http.StatusOK)

	var got connectionDetail
	parseJSON(t, resp, &got)
	if got.ShortID != conn.ShortID {
		t.Errorf("want short_id %q, got %q", conn.ShortID, got.ShortID)
	}
	if got.Status != "active" {
		t.Errorf("want status active, got %q", got.Status)
	}
	if got.AccountCount != 1 {
		t.Errorf("want account_count 1, got %d", got.AccountCount)
	}
	if got.Paused {
		t.Error("want paused=false on fresh connection")
	}
	if got.SyncIntervalOverrideMinutes != nil {
		t.Errorf("want nil sync_interval_override_minutes, got %v", *got.SyncIntervalOverrideMinutes)
	}
}

func TestGetConnection_NotFound(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doGet(t, "/api/v1/connections/00000000-0000-0000-0000-000000000000")
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestGetConnection_AcceptsShortID(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_get_short")

	resp := env.doGet(t, "/api/v1/connections/"+conn.ShortID)
	assertStatus(t, resp, http.StatusOK)

	var got connectionDetail
	parseJSON(t, resp, &got)
	if got.ShortID != conn.ShortID {
		t.Errorf("want short_id %q, got %q", conn.ShortID, got.ShortID)
	}
}

func TestGetConnection_AllowsReadScope(t *testing.T) {
	env := setupReadOnlyEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_get_ro")

	resp := env.doGet(t, "/api/v1/connections/"+conn.ShortID)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestDeleteConnection_DisconnectsActive(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_del_ok")

	resp := env.doDelete(t, "/api/v1/connections/"+conn.ShortID)
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()

	// Match the existing read-filter behavior: GetConnectionForAPI does
	// not filter by status, so the GET still resolves but reports
	// status="disconnected".
	resp = env.doGet(t, "/api/v1/connections/"+conn.ShortID)
	assertStatus(t, resp, http.StatusOK)
	var got connectionDetail
	parseJSON(t, resp, &got)
	if got.Status != "disconnected" {
		t.Errorf("want status disconnected, got %q", got.Status)
	}

	// And it's hidden from the list (which DOES filter disconnected).
	resp = env.doGet(t, "/api/v1/connections")
	assertStatus(t, resp, http.StatusOK)
	var list []connectionDetail
	parseJSON(t, resp, &list)
	for _, c := range list {
		if c.ShortID == conn.ShortID {
			t.Errorf("disconnected connection %q still in list", conn.ShortID)
		}
	}
}

func TestDeleteConnection_AlreadyDisconnected(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_del_twice")

	if err := env.Queries.UpdateBankConnectionStatus(t.Context(), db.UpdateBankConnectionStatusParams{
		ID:     conn.ID,
		Status: db.ConnectionStatusDisconnected,
	}); err != nil {
		t.Fatalf("set disconnected: %v", err)
	}

	resp := env.doDelete(t, "/api/v1/connections/"+conn.ShortID)
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestPauseConnection_Toggle(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_pause_toggle")

	// Pause → true.
	resp := env.doPost(t, "/api/v1/connections/"+conn.ShortID+"/paused", map[string]any{
		"paused": true,
	})
	assertStatus(t, resp, http.StatusOK)
	var got connectionDetail
	parseJSON(t, resp, &got)
	if !got.Paused {
		t.Error("want paused=true after first toggle")
	}

	// Toggle back → false.
	resp = env.doPost(t, "/api/v1/connections/"+conn.ShortID+"/paused", map[string]any{
		"paused": false,
	})
	assertStatus(t, resp, http.StatusOK)
	parseJSON(t, resp, &got)
	if got.Paused {
		t.Error("want paused=false after second toggle")
	}
}

func TestPauseConnection_NotFound(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doPost(t, "/api/v1/connections/00000000-0000-0000-0000-000000000000/paused", map[string]any{
		"paused": true,
	})
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestUpdateSyncInterval_Set(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_interval_set")

	resp := env.doPost(t, "/api/v1/connections/"+conn.ShortID+"/sync-interval", map[string]any{
		"interval_minutes": 30,
	})
	assertStatus(t, resp, http.StatusOK)
	var got connectionDetail
	parseJSON(t, resp, &got)
	if got.SyncIntervalOverrideMinutes == nil || *got.SyncIntervalOverrideMinutes != 30 {
		t.Errorf("want sync_interval_override_minutes=30, got %v", got.SyncIntervalOverrideMinutes)
	}
}

func TestUpdateSyncInterval_Clear(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_interval_clear")

	// First set a non-default value.
	resp := env.doPost(t, "/api/v1/connections/"+conn.ShortID+"/sync-interval", map[string]any{
		"interval_minutes": 45,
	})
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// Now clear via null.
	resp = env.doPost(t, "/api/v1/connections/"+conn.ShortID+"/sync-interval", map[string]any{
		"interval_minutes": nil,
	})
	assertStatus(t, resp, http.StatusOK)
	var got connectionDetail
	parseJSON(t, resp, &got)
	if got.SyncIntervalOverrideMinutes != nil {
		t.Errorf("want nil after clear, got %v", *got.SyncIntervalOverrideMinutes)
	}
}

func TestTriggerConnectionSync(t *testing.T) {
	env := setupTestEnv(t)
	waitForSyncDrain(t, env)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_per_conn_sync")

	resp := env.doPost(t, "/api/v1/connections/"+conn.ShortID+"/sync", nil)
	assertSyncTriggered(t, resp)
}

func TestTriggerConnectionSync_Disconnected(t *testing.T) {
	env := setupTestEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_per_conn_sync_disc")

	if err := env.Queries.UpdateBankConnectionStatus(t.Context(), db.UpdateBankConnectionStatusParams{
		ID:     conn.ID,
		Status: db.ConnectionStatusDisconnected,
	}); err != nil {
		t.Fatalf("set disconnected: %v", err)
	}

	resp := env.doPost(t, "/api/v1/connections/"+conn.ShortID+"/sync", nil)
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestConnectionMgmt_RequiresWriteScope(t *testing.T) {
	env := setupReadOnlyEnv(t)
	user := testutil.MustCreateUser(t, env.Queries, "Alice")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "ext_scope_check")

	cases := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"delete", "DELETE", "/api/v1/connections/" + conn.ShortID, nil},
		{"per-conn sync", "POST", "/api/v1/connections/" + conn.ShortID + "/sync", nil},
		{"paused", "POST", "/api/v1/connections/" + conn.ShortID + "/paused", map[string]any{"paused": true}},
		{"sync-interval", "POST", "/api/v1/connections/" + conn.ShortID + "/sync-interval", map[string]any{"interval_minutes": 30}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var resp *http.Response
			if tc.method == "DELETE" {
				resp = env.doDelete(t, tc.path)
			} else {
				resp = env.doPost(t, tc.path, tc.body)
			}
			readErrorCode(t, resp, http.StatusForbidden, "INSUFFICIENT_SCOPE")
		})
	}
}
