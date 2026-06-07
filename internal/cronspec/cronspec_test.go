package cronspec

import (
	"testing"
	"time"
)

func TestResolveCron(t *testing.T) {
	// Known preset ignores the custom value.
	expr, preset, err := ResolveCron("nightly", "0 0 * * *")
	if err != nil {
		t.Fatalf("preset resolve: %v", err)
	}
	if expr != "0 3 * * *" || preset != "nightly" {
		t.Errorf("preset resolve = (%q, %q)", expr, preset)
	}

	// Custom uses the supplied expression.
	expr, preset, err = ResolveCron("custom", "0 6,18 * * *")
	if err != nil {
		t.Fatalf("custom resolve: %v", err)
	}
	if expr != "0 6,18 * * *" || preset != CustomKey {
		t.Errorf("custom resolve = (%q, %q)", expr, preset)
	}

	// Invalid custom cron errors.
	if _, _, err := ResolveCron("custom", "nope"); err == nil {
		t.Error("expected error for invalid custom cron")
	}
}

func TestHumanize(t *testing.T) {
	if got := Humanize("0 3 * * *", "nightly"); got != "Nightly (3 AM)" {
		t.Errorf("preset humanize = %q", got)
	}
	// Unknown preset falls back to matching the raw cron.
	if got := Humanize("0 3 * * *", ""); got != "Nightly (3 AM)" {
		t.Errorf("cron-match humanize = %q", got)
	}
	// Truly custom cron echoes literally.
	if got := Humanize("7 4 * * 1", "custom"); got != "Custom (7 4 * * 1)" {
		t.Errorf("custom humanize = %q", got)
	}
}

func TestNextRun(t *testing.T) {
	now := time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC)
	// Earliest across two exprs: 12:00 beats 18:00.
	next, ok := NextRun([]string{"0 18 * * *", "0 12 * * *"}, "", now)
	if !ok {
		t.Fatal("expected a next run")
	}
	if want := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC); !next.Equal(want) {
		t.Errorf("NextRun = %v, want %v", next, want)
	}
	// All invalid → not found.
	if _, ok := NextRun([]string{"bad"}, "", now); ok {
		t.Error("expected no next run for invalid expr")
	}
	if _, ok := NextRun(nil, "", now); ok {
		t.Error("expected no next run for empty exprs")
	}
}

func TestDuePassed(t *testing.T) {
	now := time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC)
	// Nightly 03:00 fired between 02:00 and 09:00 → due.
	if !DuePassed([]string{"0 3 * * *"}, "", time.Date(2026, 6, 4, 2, 0, 0, 0, time.UTC), now) {
		t.Error("expected due (03:00 fire passed since 02:00)")
	}
	// Synced 04:00, nightly's next fire is tomorrow 03:00 > now → not due.
	if DuePassed([]string{"0 3 * * *"}, "", time.Date(2026, 6, 4, 4, 0, 0, 0, time.UTC), now) {
		t.Error("expected not due (next fire is tomorrow)")
	}
}

func TestNextRuns(t *testing.T) {
	from := time.Date(2026, 6, 4, 9, 0, 0, 0, time.UTC)
	runs := NextRuns("0 12 * * *", "", from, 3)
	if len(runs) != 3 {
		t.Fatalf("want 3 runs, got %d", len(runs))
	}
	if want := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC); !runs[0].Equal(want) {
		t.Errorf("first run = %v, want %v", runs[0], want)
	}
	if len(NextRuns("nonsense", "", from, 3)) != 0 {
		t.Error("invalid expr should give no runs")
	}
	if NextRuns("0 12 * * *", "", from, 0) != nil {
		t.Error("n=0 should give nil")
	}
}

func TestParseTimezoneAnchoring(t *testing.T) {
	// "0 6 * * *" in America/Los_Angeles fires at 06:00 PDT = 13:00 UTC (summer,
	// UTC-7). Anchoring via the tzName must produce that instant regardless of
	// the reference time's location.
	from := time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC)
	runs := NextRuns("0 6 * * *", "America/Los_Angeles", from, 1)
	if len(runs) != 1 {
		t.Fatalf("want 1 run, got %d", len(runs))
	}
	if h := runs[0].UTC().Hour(); h != 13 {
		t.Errorf("expected fire at 13:00 UTC (06:00 PDT), got %02d:00 UTC", h)
	}
	// Same expression without a tz anchors to UTC → fires at 06:00 UTC.
	utcRuns := NextRuns("0 6 * * *", "", from, 1)
	if len(utcRuns) != 1 || utcRuns[0].UTC().Hour() != 6 {
		t.Errorf("expected UTC fire at 06:00, got %v", utcRuns)
	}
}

func TestValidate(t *testing.T) {
	if err := Validate("0 6,18 * * *"); err != nil {
		t.Errorf("valid cron rejected: %v", err)
	}
	if Validate("") == nil || Validate("nonsense") == nil {
		t.Error("expected errors for empty/invalid cron")
	}
}
