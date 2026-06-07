//go:build integration && !lite

package service_test

import (
	"context"
	"testing"

	"breadbox/internal/pgconv"
	csvpkg "breadbox/internal/provider/csv"
	"breadbox/internal/service"
	"breadbox/internal/testutil"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

// classifyCfg is the column mapping used across these tests: date|amount|desc.
var classifyCfg = service.CSVParseConfig{
	ColumnMapping:   map[string]int{"date": 0, "amount": 1, "description": 2},
	DateFormat:      "2006-01-02",
	PositiveIsDebit: true, // positive already = debit (Breadbox convention)
}

func classify(t *testing.T, svc *service.Service, acct pgtype.UUID, rows [][]string) []service.CSVClassifiedRow {
	t.Helper()
	parsed := service.ParseCSVRows(classifyCfg, rows)
	out, err := svc.ClassifyCSVRows(context.Background(), acct, pgconv.FormatUUID(acct), parsed, service.DefaultDedupToleranceDays)
	if err != nil {
		t.Fatalf("ClassifyCSVRows: %v", err)
	}
	return out
}

func TestClassify_AllNewIntoEmptyAccount(t *testing.T) {
	svc, q, _ := newService(t)
	acct := seedTxnFixture(t, q)

	rows := [][]string{
		{"2026-01-10", "12.50", "STARBUCKS"},
		{"2026-01-11", "40.00", "SHELL GAS"},
	}
	got := classify(t, svc, acct, rows)
	for _, r := range got {
		if r.Classification != service.CSVRowNew || !r.Include {
			t.Errorf("row %d: got %s include=%v, want new/included", r.RowIndex, r.Classification, r.Include)
		}
	}
}

func TestClassify_ExactDupByProviderID(t *testing.T) {
	svc, q, _ := newService(t)
	acct := seedTxnFixture(t, q)
	acctStr := pgconv.FormatUUID(acct)

	// Pre-seed a transaction whose provider id is exactly what re-importing the
	// same row would generate.
	date := testutil.MustParseDate("2026-01-15")
	extID := csvpkg.GenerateExternalID(acctStr, date, decimal.RequireFromString("5.00"), "STARBUCKS")
	testutil.MustCreateTransaction(t, q, acct, extID, "STARBUCKS", 500, "2026-01-15")

	got := classify(t, svc, acct, [][]string{{"2026-01-15", "5.00", "STARBUCKS"}})
	if got[0].Classification != service.CSVRowExactDup || got[0].Include {
		t.Fatalf("got %s include=%v, want exact_dup/excluded", got[0].Classification, got[0].Include)
	}
}

func TestClassify_ProbableDupCrossProvider(t *testing.T) {
	svc, q, _ := newService(t)
	// A Teller-sourced txn (no content_hash) with same date+amount and similar
	// name. Re-importing a CSV that overlaps must flag it as a probable dup.
	user := testutil.MustCreateUser(t, q, "Bob")
	conn := testutil.MustCreateTellerConnection(t, q, user.ID, "teller_1")
	acct := testutil.MustCreateAccount(t, q, conn.ID, "tlr_acct", "Teller Checking")
	testutil.MustCreateTransaction(t, q, acct.ID, "teller_txn_1", "STARBUCKS STORE 123", 500, "2026-02-01")

	// Same amount/date, name is a superset — should fuzzy-match.
	got := classify(t, svc, acct.ID, [][]string{{"2026-02-01", "5.00", "STARBUCKS"}})
	if got[0].Classification != service.CSVRowProbableDup || got[0].Include {
		t.Fatalf("got %s include=%v reason=%q, want probable_dup/excluded",
			got[0].Classification, got[0].Include, got[0].MatchReason)
	}
	if got[0].MatchTxnID == "" {
		t.Error("probable dup should record the matched txn id")
	}
}

func TestClassify_NearbyDateNoNameStaysNew(t *testing.T) {
	svc, q, _ := newService(t)
	acct := seedTxnFixture(t, q)
	// Existing txn: same amount, 5 days away (outside tolerance), different name.
	testutil.MustCreateTransaction(t, q, acct, "x1", "WALMART", 500, "2026-03-01")

	got := classify(t, svc, acct, [][]string{{"2026-03-06", "5.00", "TARGET"}})
	if got[0].Classification != service.CSVRowNew {
		t.Fatalf("got %s, want new (amount matches but date far + name differs)", got[0].Classification)
	}
}

func TestClassify_ParseErrorRow(t *testing.T) {
	svc, q, _ := newService(t)
	acct := seedTxnFixture(t, q)
	got := classify(t, svc, acct, [][]string{{"not-a-date", "5.00", "X"}})
	if got[0].Classification != service.CSVRowError || got[0].Include {
		t.Fatalf("got %s include=%v, want error/excluded", got[0].Classification, got[0].Include)
	}
}

