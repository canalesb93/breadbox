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

func TestValidate(t *testing.T) {
	if err := Validate("0 6,18 * * *"); err != nil {
		t.Errorf("valid cron rejected: %v", err)
	}
	if Validate("") == nil || Validate("nonsense") == nil {
		t.Error("expected errors for empty/invalid cron")
	}
}
