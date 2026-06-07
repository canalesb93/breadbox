//go:build integration && !lite

package service_test

import (
	"context"
	"testing"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	csvpkg "breadbox/internal/provider/csv"
	"breadbox/internal/service"
	"breadbox/internal/testutil"
)

// overlapRows builds CSV rows matching the given (date, amount) pairs.
func overlapRows(pairs [][2]string) [][]string {
	rows := make([][]string, len(pairs))
	for i, p := range pairs {
		rows[i] = []string{p[0], p[1], "TXN " + p[0]}
	}
	return rows
}

func TestAccountMatch_OverlapRanksCorrectAccount(t *testing.T) {
	svc, q, _ := newService(t)
	user := testutil.MustCreateUser(t, q, "Alice")
	conn := testutil.MustCreateConnection(t, q, user.ID, "item_1")
	accA := testutil.MustCreateAccount(t, q, conn.ID, "extA", "Checking A")
	accB := testutil.MustCreateAccount(t, q, conn.ID, "extB", "Savings B")

	// Seed account B with three transactions; A gets an unrelated one.
	testutil.MustCreateTransaction(t, q, accB.ID, "b1", "TXN 2026-04-01", 1000, "2026-04-01")
	testutil.MustCreateTransaction(t, q, accB.ID, "b2", "TXN 2026-04-02", 2000, "2026-04-02")
	testutil.MustCreateTransaction(t, q, accB.ID, "b3", "TXN 2026-04-03", 3000, "2026-04-03")
	testutil.MustCreateTransaction(t, q, accA.ID, "a1", "OTHER", 9999, "2026-04-01")

	rows := overlapRows([][2]string{{"2026-04-01", "10.00"}, {"2026-04-02", "20.00"}, {"2026-04-03", "30.00"}})
	parsed := service.ParseCSVRows(classifyCfg, rows)

	sug, err := svc.MatchCSVAccounts(context.Background(), user.ID, service.CSVDetectionSignals{}, parsed)
	if err != nil {
		t.Fatalf("MatchCSVAccounts: %v", err)
	}
	if len(sug.Matches) == 0 || sug.Matches[0].AccountID != pgconv.FormatUUID(accB.ID) {
		t.Fatalf("expected account B on top, got %+v", sug.Matches)
	}
	if sug.Matches[0].Score < 40 {
		t.Errorf("full overlap should score the overlap max, got %d", sug.Matches[0].Score)
	}
}

func TestAccountMatch_PreselectOnMaskPlusOverlap(t *testing.T) {
	svc, q, pool := newService(t)
	user := testutil.MustCreateUser(t, q, "Alice")
	conn := testutil.MustCreateConnection(t, q, user.ID, "item_1")
	acc := testutil.MustCreateAccount(t, q, conn.ID, "extA", "Chase Checking")
	// Give the account a mask.
	if _, err := pool.Exec(context.Background(), "UPDATE accounts SET mask = '4321' WHERE id = $1", acc.ID); err != nil {
		t.Fatalf("set mask: %v", err)
	}
	testutil.MustCreateTransaction(t, q, acc.ID, "t1", "TXN 2026-05-01", 1000, "2026-05-01")
	testutil.MustCreateTransaction(t, q, acc.ID, "t2", "TXN 2026-05-02", 2000, "2026-05-02")

	rows := overlapRows([][2]string{{"2026-05-01", "10.00"}, {"2026-05-02", "20.00"}})
	parsed := service.ParseCSVRows(classifyCfg, rows)

	sug, err := svc.MatchCSVAccounts(context.Background(), user.ID, service.CSVDetectionSignals{Mask: "4321"}, parsed)
	if err != nil {
		t.Fatalf("MatchCSVAccounts: %v", err)
	}
	if sug.Preselect != pgconv.FormatUUID(acc.ID) {
		t.Fatalf("expected preselect of the matched account, got %q (top score %d)", sug.Preselect, topScore(sug))
	}
}

func TestAccountMatch_ProfilePreselect(t *testing.T) {
	svc, q, _ := newService(t)
	user := testutil.MustCreateUser(t, q, "Alice")
	conn := testutil.MustCreateConnection(t, q, user.ID, "item_1")
	acc := testutil.MustCreateAccount(t, q, conn.ID, "extA", "Chase Checking")

	headers := []string{"Date", "Amount", "Description"}
	fp := csvpkg.HeaderFingerprint(headers)
	_, err := q.UpsertCSVImportProfile(context.Background(), db.UpsertCSVImportProfileParams{
		UserID:            user.ID,
		Name:              "Chase CSV",
		HeaderFingerprint: fp,
		Headers:           []byte(`["Date","Amount","Description"]`),
		ColumnMapping:     []byte(`{"date":0,"amount":1,"description":2}`),
		DateFormat:        "2006-01-02",
		Delimiter:         ",",
		IsoCurrencyCode:   "USD",
		DefaultAccountID:  acc.ID,
	})
	if err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	rows := overlapRows([][2]string{{"2026-06-01", "10.00"}})
	parsed := service.ParseCSVRows(classifyCfg, rows)

	sug, err := svc.MatchCSVAccounts(context.Background(), user.ID, service.CSVDetectionSignals{Headers: headers}, parsed)
	if err != nil {
		t.Fatalf("MatchCSVAccounts: %v", err)
	}
	if sug.ProfileID == "" {
		t.Error("expected ProfileID to be set on a fingerprint hit")
	}
	if sug.Preselect != pgconv.FormatUUID(acc.ID) {
		t.Fatalf("profile should preselect its default account, got %q", sug.Preselect)
	}
	if len(sug.Matches) == 0 || !sug.Matches[0].ProfileMatch {
		t.Error("top match should be flagged ProfileMatch")
	}
}

func topScore(s *service.CSVAccountSuggestion) int {
	if len(s.Matches) == 0 {
		return 0
	}
	return s.Matches[0].Score
}
