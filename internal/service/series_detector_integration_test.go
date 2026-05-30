//go:build integration && !lite

package service_test

import (
	"context"
	"fmt"
	"testing"

	"breadbox/internal/db"
	"breadbox/internal/service"
	"breadbox/internal/testutil"

	"github.com/jackc/pgx/v5/pgtype"
)

// seedConnAccount creates user→connection→account and returns (connID, acctID).
func seedConnAccount(t *testing.T, queries *db.Queries) (pgtype.UUID, pgtype.UUID) {
	t.Helper()
	user := testutil.MustCreateUser(t, queries, "Alice")
	conn := testutil.MustCreateConnection(t, queries, user.ID, "item_detect")
	acct := testutil.MustCreateAccount(t, queries, conn.ID, "ext_detect", "Checking")
	return conn.ID, acct.ID
}

func TestDetectSeriesForConnection_CreatesCandidate(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	connID, acctID := seedConnAccount(t, queries)

	// 3 monthly Spotify charges. MustCreateTransaction leaves merchant_key NULL;
	// the detector populates it via the normalizer, then groups.
	for _, d := range []string{"2026-02-15", "2026-03-15", "2026-04-15"} {
		testutil.MustCreateTransaction(t, queries, acctID, "spot_"+d, "SPOTIFY", 999, d)
	}

	n, err := svc.DetectSeriesForConnection(ctx, connID)
	if err != nil {
		t.Fatalf("DetectSeriesForConnection: %v", err)
	}
	if n != 1 {
		t.Fatalf("detected %d series, want 1", n)
	}
	cands, err := svc.ListSeriesByStatus(ctx, service.SeriesStatusCandidate)
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	if len(cands) != 1 {
		t.Fatalf("candidate count = %d, want 1", len(cands))
	}
	s := cands[0]
	if s.MerchantKey != "spotify" {
		t.Errorf("merchant_key = %q, want spotify", s.MerchantKey)
	}
	if s.Cadence != service.SeriesCadenceMonthly {
		t.Errorf("cadence = %q, want monthly", s.Cadence)
	}
	if s.OccurrenceCount != 3 {
		t.Errorf("occurrence_count = %d, want 3", s.OccurrenceCount)
	}
	if s.Confidence != service.SeriesConfidenceAuto {
		t.Errorf("confidence = %q, want auto", s.Confidence)
	}
	if len(s.DetectionSignals) == 0 {
		t.Error("expected detection_signals to be persisted")
	}
}

// The catastrophic footgun guard: charges whose descriptor normalizes to a bare
// generic word get merchant_key=NULL and must NEVER be auto-grouped into a fake
// "ach debit subscription".
func TestDetectSeriesForConnection_NullMerchantExcluded(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	connID, acctID := seedConnAccount(t, queries)

	for i, d := range []string{"2026-02-15", "2026-03-15", "2026-04-15"} {
		// Different unrelated payees, all surfacing only as a generic descriptor.
		testutil.MustCreateTransaction(t, queries, acctID, fmt.Sprintf("ach_%d", i), "ACH DEBIT", 999, d)
	}

	n, err := svc.DetectSeriesForConnection(ctx, connID)
	if err != nil {
		t.Fatalf("DetectSeriesForConnection: %v", err)
	}
	if n != 0 {
		t.Fatalf("detected %d series from generic descriptors, want 0", n)
	}
	if total, _ := queries.CountRecurringSeries(ctx); total != 0 {
		t.Fatalf("series count = %d, want 0 (NULL-merchant guard)", total)
	}
}

func TestBackfillSeriesDetection_StalenessGate(t *testing.T) {
	svc, queries, _ := newService(t)
	ctx := context.Background()
	_, acctID := seedConnAccount(t, queries)

	// Active monthly series ending recently (within the staleness window).
	for _, d := range []string{"2026-03-15", "2026-04-15", "2026-05-15"} {
		testutil.MustCreateTransaction(t, queries, acctID, "active_"+d, "NETFLIX", 1599, d)
	}
	// Long-cancelled monthly series ending 2 years ago — must be skipped.
	for _, d := range []string{"2024-01-10", "2024-02-10", "2024-03-10"} {
		testutil.MustCreateTransaction(t, queries, acctID, "old_"+d, "OLDGYM", 4500, d)
	}

	n, err := svc.BackfillSeriesDetection(ctx)
	if err != nil {
		t.Fatalf("BackfillSeriesDetection: %v", err)
	}
	if n != 1 {
		t.Fatalf("backfill emitted %d candidates, want 1 (stale OLDGYM skipped)", n)
	}
	cands, _ := svc.ListSeriesByStatus(ctx, service.SeriesStatusCandidate)
	if len(cands) != 1 || cands[0].MerchantKey != "netflix" {
		t.Fatalf("expected only the recent netflix series, got %+v", cands)
	}
}
