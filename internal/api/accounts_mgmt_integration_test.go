//go:build integration && !lite

package api

import (
	"net/http"
	"testing"
	"time"

	"breadbox/internal/db"
	"breadbox/internal/testutil"
)

// seedAccountForMgmt creates a user, connection, account, and the requested
// number of transactions (dated descending from 2025-03-15). Returns the
// account row for assertions.
func seedAccountForMgmt(t *testing.T, env *testEnv, txnCount int) db.Account {
	t.Helper()
	user := testutil.MustCreateUser(t, env.Queries, "Account Owner")
	conn := testutil.MustCreateConnection(t, env.Queries, user.ID, "item_acct_mgmt")
	acct := testutil.MustCreateAccount(t, env.Queries, conn.ID, "ext_acct_mgmt", "Joint Checking")

	base := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)
	for i := 0; i < txnCount; i++ {
		date := base.AddDate(0, 0, -i).Format("2006-01-02")
		ext := "ext_txn_mgmt_" + date + "_" + itoa(i)
		testutil.MustCreateTransaction(t, env.Queries, acct.ID, ext, "Coffee Shop", int64(100+i), date)
	}
	return acct
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	negative := i < 0
	if negative {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if negative {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// ============================================================
// GET /accounts/{id}/detail
// ============================================================

func TestGetAccountDetail_Success(t *testing.T) {
	env := setupTestEnv(t)
	acct := seedAccountForMgmt(t, env, 5)

	resp := env.doGet(t, "/api/v1/accounts/"+acct.ShortID+"/detail")
	assertStatus(t, resp, http.StatusOK)

	var detail struct {
		ShortID            string `json:"short_id"`
		Name               string `json:"name"`
		Excluded           bool   `json:"excluded"`
		Balances           []struct {
			IsoCurrencyCode *string  `json:"iso_currency_code"`
			BalanceCurrent  *float64 `json:"balance_current"`
		} `json:"balances"`
		RecentTransactions []struct {
			ShortID string `json:"short_id"`
			Date    string `json:"date"`
		} `json:"recent_transactions"`
	}
	parseJSON(t, resp, &detail)

	if detail.ShortID != acct.ShortID {
		t.Errorf("short_id: want %q got %q", acct.ShortID, detail.ShortID)
	}
	if detail.Name != "Joint Checking" {
		t.Errorf("name: want %q got %q", "Joint Checking", detail.Name)
	}
	if len(detail.Balances) != 1 {
		t.Fatalf("expected exactly 1 balance entry, got %d", len(detail.Balances))
	}
	if len(detail.RecentTransactions) != 5 {
		t.Errorf("expected 5 recent transactions, got %d", len(detail.RecentTransactions))
	}
	// Recent txns should be date-DESC.
	if len(detail.RecentTransactions) >= 2 && detail.RecentTransactions[0].Date < detail.RecentTransactions[1].Date {
		t.Errorf("recent_transactions not date-DESC: %q before %q",
			detail.RecentTransactions[0].Date, detail.RecentTransactions[1].Date)
	}
}

func TestGetAccountDetail_NotFound(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doGet(t, "/api/v1/accounts/00000000-0000-0000-0000-000000000000/detail")
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestGetAccountDetail_AcceptsShortID(t *testing.T) {
	env := setupTestEnv(t)
	acct := seedAccountForMgmt(t, env, 1)

	// Same short_id flow as Success, but explicitly assert short_id resolution path.
	resp := env.doGet(t, "/api/v1/accounts/"+acct.ShortID+"/detail")
	assertStatus(t, resp, http.StatusOK)
	var detail struct {
		ShortID string `json:"short_id"`
	}
	parseJSON(t, resp, &detail)
	if detail.ShortID != acct.ShortID {
		t.Errorf("short_id: want %q got %q", acct.ShortID, detail.ShortID)
	}
}

func TestGetAccountDetail_LastNCap(t *testing.T) {
	env := setupTestEnv(t)
	acct := seedAccountForMgmt(t, env, 30)

	resp := env.doGet(t, "/api/v1/accounts/"+acct.ShortID+"/detail")
	assertStatus(t, resp, http.StatusOK)
	var detail struct {
		RecentTransactions []map[string]any `json:"recent_transactions"`
	}
	parseJSON(t, resp, &detail)
	if got := len(detail.RecentTransactions); got != 25 {
		t.Errorf("expected last-25 cap, got %d", got)
	}
}

func TestGetAccountDetail_AllowsReadScope(t *testing.T) {
	env := setupReadOnlyEnv(t)
	acct := seedAccountForMgmt(t, env, 2)

	resp := env.doGet(t, "/api/v1/accounts/"+acct.ShortID+"/detail")
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

// ============================================================
// PATCH /accounts/{id}
// ============================================================

func TestUpdateAccount_DisplayName(t *testing.T) {
	env := setupTestEnv(t)
	acct := seedAccountForMgmt(t, env, 0)

	resp := env.doPatch(t, "/api/v1/accounts/"+acct.ShortID, map[string]any{
		"display_name": "Renamed Checking",
	})
	assertStatus(t, resp, http.StatusOK)

	var got map[string]any
	parseJSON(t, resp, &got)
	// The base AccountResponse exposes Name (not DisplayName); DisplayName is
	// only on the detail payload. Verify by re-reading detail.
	resp2 := env.doGet(t, "/api/v1/accounts/"+acct.ShortID+"/detail")
	assertStatus(t, resp2, http.StatusOK)
	var detail struct {
		DisplayName *string `json:"display_name"`
	}
	parseJSON(t, resp2, &detail)
	if detail.DisplayName == nil || *detail.DisplayName != "Renamed Checking" {
		t.Errorf("display_name: want %q got %v", "Renamed Checking", detail.DisplayName)
	}
}

func TestUpdateAccount_DisplayNameClear(t *testing.T) {
	env := setupTestEnv(t)
	acct := seedAccountForMgmt(t, env, 0)

	// Set then clear via empty string.
	resp1 := env.doPatch(t, "/api/v1/accounts/"+acct.ShortID, map[string]any{
		"display_name": "Temporary",
	})
	assertStatus(t, resp1, http.StatusOK)
	resp1.Body.Close()

	resp2 := env.doPatch(t, "/api/v1/accounts/"+acct.ShortID, map[string]any{
		"display_name": "",
	})
	assertStatus(t, resp2, http.StatusOK)
	resp2.Body.Close()

	resp3 := env.doGet(t, "/api/v1/accounts/"+acct.ShortID+"/detail")
	var detail struct {
		DisplayName *string `json:"display_name"`
	}
	parseJSON(t, resp3, &detail)
	if detail.DisplayName != nil {
		t.Errorf("expected display_name cleared (nil), got %q", *detail.DisplayName)
	}
}

func TestUpdateAccount_IsExcludedToggle(t *testing.T) {
	env := setupTestEnv(t)
	acct := seedAccountForMgmt(t, env, 0)

	excluded := true
	resp := env.doPatch(t, "/api/v1/accounts/"+acct.ShortID, map[string]any{
		"is_excluded": excluded,
	})
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// Verify the column was actually updated.
	row, err := env.Queries.GetAccount(t.Context(), acct.ID)
	if err != nil {
		t.Fatalf("get account: %v", err)
	}
	if !row.Excluded {
		t.Error("expected excluded=true after PATCH")
	}

	// Toggle back off.
	resp2 := env.doPatch(t, "/api/v1/accounts/"+acct.ShortID, map[string]any{
		"is_excluded": false,
	})
	assertStatus(t, resp2, http.StatusOK)
	resp2.Body.Close()
	row2, err := env.Queries.GetAccount(t.Context(), acct.ID)
	if err != nil {
		t.Fatalf("get account: %v", err)
	}
	if row2.Excluded {
		t.Error("expected excluded=false after second PATCH")
	}
}

func TestUpdateAccount_IsDependentLinkedToggle(t *testing.T) {
	env := setupTestEnv(t)
	acct := seedAccountForMgmt(t, env, 0)

	resp := env.doPatch(t, "/api/v1/accounts/"+acct.ShortID, map[string]any{
		"is_dependent_linked": true,
	})
	assertStatus(t, resp, http.StatusOK)

	var body map[string]any
	parseJSON(t, resp, &body)
	if v, _ := body["is_dependent_linked"].(bool); !v {
		t.Errorf("expected is_dependent_linked=true in response, got %v", body["is_dependent_linked"])
	}

	row, err := env.Queries.GetAccount(t.Context(), acct.ID)
	if err != nil {
		t.Fatalf("get account: %v", err)
	}
	if !row.IsDependentLinked {
		t.Error("expected is_dependent_linked=true in DB")
	}
}

func TestUpdateAccount_NotFound_Account(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doPatch(t, "/api/v1/accounts/00000000-0000-0000-0000-000000000000", map[string]any{
		"display_name": "Nope",
	})
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

func TestUpdateAccount_PartialPreserves(t *testing.T) {
	env := setupTestEnv(t)
	acct := seedAccountForMgmt(t, env, 0)

	// Set both display_name and excluded.
	resp1 := env.doPatch(t, "/api/v1/accounts/"+acct.ShortID, map[string]any{
		"display_name": "Pinned",
		"is_excluded":  true,
	})
	assertStatus(t, resp1, http.StatusOK)
	resp1.Body.Close()

	// PATCH only is_dependent_linked — display_name and excluded must persist.
	resp2 := env.doPatch(t, "/api/v1/accounts/"+acct.ShortID, map[string]any{
		"is_dependent_linked": true,
	})
	assertStatus(t, resp2, http.StatusOK)
	resp2.Body.Close()

	row, err := env.Queries.GetAccount(t.Context(), acct.ID)
	if err != nil {
		t.Fatalf("get account: %v", err)
	}
	if !row.Excluded {
		t.Error("expected excluded preserved as true after partial PATCH")
	}
	if !row.IsDependentLinked {
		t.Error("expected is_dependent_linked set to true")
	}
	if !row.DisplayName.Valid || row.DisplayName.String != "Pinned" {
		t.Errorf("expected display_name preserved as %q, got %+v", "Pinned", row.DisplayName)
	}
}

func TestUpdateAccount_NoFields(t *testing.T) {
	env := setupTestEnv(t)
	acct := seedAccountForMgmt(t, env, 0)

	// PATCH with empty body — should be a no-op (200 + current state).
	resp := env.doPatch(t, "/api/v1/accounts/"+acct.ShortID, map[string]any{})
	assertStatus(t, resp, http.StatusOK)
	var got map[string]any
	parseJSON(t, resp, &got)
	if got["short_id"] != acct.ShortID {
		t.Errorf("short_id: want %q got %v", acct.ShortID, got["short_id"])
	}
}

func TestUpdateAccount_RequiresWriteScope(t *testing.T) {
	env := setupReadOnlyEnv(t)
	acct := seedAccountForMgmt(t, env, 0)

	resp := env.doPatch(t, "/api/v1/accounts/"+acct.ShortID, map[string]any{
		"display_name": "Blocked",
	})
	readErrorCode(t, resp, http.StatusForbidden, "INSUFFICIENT_SCOPE")
}
