package admin

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestSplitCSV(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", []string{}},
		{"single", "alpha", []string{"alpha"}},
		{"multiple", "a,b,c", []string{"a", "b", "c"}},
		{"trims whitespace", " a , b ,c ", []string{"a", "b", "c"}},
		{"drops empty entries", "a,,b,", []string{"a", "b"}},
		{"only separators", ",,,", []string{}},
		{"only whitespace", "  ,  ,  ", []string{}},
		{"preserves inner spaces", "foo bar,baz", []string{"foo bar", "baz"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := splitCSV(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d (got %q)", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestIsLiabilityAccount(t *testing.T) {
	tests := []struct {
		accountType string
		want        bool
	}{
		{"credit", true},
		{"loan", true},
		{"depository", false},
		{"investment", false},
		{"", false},
		{"CREDIT", false}, // case-sensitive by design
		{"other", false},
	}
	for _, tc := range tests {
		t.Run(tc.accountType, func(t *testing.T) {
			if got := IsLiabilityAccount(tc.accountType); got != tc.want {
				t.Errorf("IsLiabilityAccount(%q) = %v, want %v", tc.accountType, got, tc.want)
			}
		})
	}
}

func TestConnectionStaleness(t *testing.T) {
	now := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)

	syncedAt := func(d time.Duration) pgtype.Timestamptz {
		return pgtype.Timestamptz{Time: now.Add(-d), Valid: true}
	}
	neverSynced := pgtype.Timestamptz{Valid: false}
	override := func(minutes int32) pgtype.Int4 {
		return pgtype.Int4{Int32: minutes, Valid: true}
	}
	noOverride := pgtype.Int4{Valid: false}

	tests := []struct {
		name         string
		globalMin    int
		override     pgtype.Int4
		lastSyncedAt pgtype.Timestamptz
		want         bool
	}{
		{
			name:         "never synced is stale",
			globalMin:    60,
			override:     noOverride,
			lastSyncedAt: neverSynced,
			want:         true,
		},
		{
			name:         "global default floors at 24h — fresh sync not stale",
			globalMin:    60, // 2x = 2h, floored to 24h
			override:     noOverride,
			lastSyncedAt: syncedAt(23 * time.Hour),
			want:         false,
		},
		{
			name:         "global default floors at 24h — sync older than 24h is stale",
			globalMin:    60,
			override:     noOverride,
			lastSyncedAt: syncedAt(25 * time.Hour),
			want:         true,
		},
		{
			name:         "global interval exceeds 24h floor — 2x applies",
			globalMin:    24 * 60, // 2x = 48h, above the 24h floor
			override:     noOverride,
			lastSyncedAt: syncedAt(47 * time.Hour),
			want:         false,
		},
		{
			name:         "global interval exceeds 24h floor — stale past 2x",
			globalMin:    24 * 60, // 2x = 48h threshold
			override:     noOverride,
			lastSyncedAt: syncedAt(49 * time.Hour),
			want:         true,
		},
		{
			name:         "override floors at 1h — fresh within 1h not stale",
			globalMin:    60,
			override:     override(15), // 2x = 30m, floored to 1h
			lastSyncedAt: syncedAt(50 * time.Minute),
			want:         false,
		},
		{
			name:         "override floors at 1h — stale past 1h",
			globalMin:    60,
			override:     override(15),
			lastSyncedAt: syncedAt(61 * time.Minute),
			want:         true,
		},
		{
			name:         "override above 1h floor — 2x applies",
			globalMin:    60,
			override:     override(120), // 2x = 4h
			lastSyncedAt: syncedAt(3 * time.Hour),
			want:         false,
		},
		{
			name:         "override above 1h floor — stale past 2x",
			globalMin:    60,
			override:     override(120),
			lastSyncedAt: syncedAt(5 * time.Hour),
			want:         true,
		},
		{
			name:         "override wins over global default",
			globalMin:    24 * 60, // global would give 48h threshold
			override:     override(30),
			lastSyncedAt: syncedAt(2 * time.Hour), // past override's 1h floor
			want:         true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ConnectionStaleness(tc.globalMin, tc.override, tc.lastSyncedAt, now)
			if got != tc.want {
				t.Errorf("ConnectionStaleness = %v, want %v", got, tc.want)
			}
		})
	}
}
