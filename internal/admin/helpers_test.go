package admin

import (
	"net/http/httptest"
	"testing"
	"time"

	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestParsePage(t *testing.T) {
	tests := []struct {
		query string
		want  int
	}{
		{"", 1},
		{"page=", 1},
		{"page=abc", 1},
		{"page=0", 1},
		{"page=-5", 1},
		{"page=1", 1},
		{"page=7", 7},
	}
	for _, tc := range tests {
		t.Run(tc.query, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/?"+tc.query, nil)
			if got := parsePage(r); got != tc.want {
				t.Errorf("parsePage(%q) = %d, want %d", tc.query, got, tc.want)
			}
		})
	}
}

func TestParsePageKey(t *testing.T) {
	r := httptest.NewRequest("GET", "/?page=2&wh_page=9", nil)
	if got := parsePageKey(r, "wh_page"); got != 9 {
		t.Errorf("parsePageKey(wh_page) = %d, want 9", got)
	}
	if got := parsePageKey(r, "missing"); got != 1 {
		t.Errorf("parsePageKey(missing) = %d, want 1", got)
	}
}

func TestParsePerPage(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		def     int
		allowed []int
		want    int
	}{
		{"missing returns default", "", 25, []int{25, 50}, 25},
		{"non-numeric returns default", "per_page=abc", 25, []int{25, 50}, 25},
		{"in allowed", "per_page=50", 25, []int{25, 50, 100}, 50},
		{"not in allowed returns default", "per_page=33", 25, []int{25, 50}, 25},
		{"no allowlist accepts positive", "per_page=200", 25, nil, 200},
		{"no allowlist rejects zero", "per_page=0", 25, nil, 25},
		{"no allowlist rejects negative", "per_page=-1", 25, nil, 25},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/?"+tc.query, nil)
			if got := parsePerPage(r, tc.def, tc.allowed...); got != tc.want {
				t.Errorf("parsePerPage = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestParseDateParam(t *testing.T) {
	r := httptest.NewRequest("GET", "/?d=2026-04-19&bad=2026/04/19", nil)

	got := parseDateParam(r, "d")
	if got == nil {
		t.Fatal("parseDateParam(d) = nil, want 2026-04-19")
	}
	want := time.Date(2026, 4, 19, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("parseDateParam(d) = %v, want %v", got, want)
	}

	if parseDateParam(r, "bad") != nil {
		t.Error("parseDateParam(bad) should return nil for malformed input")
	}
	if parseDateParam(r, "missing") != nil {
		t.Error("parseDateParam(missing) should return nil")
	}
}

func TestParseInclusiveDateParam(t *testing.T) {
	r := httptest.NewRequest("GET", "/?d=2026-04-19", nil)

	got := parseInclusiveDateParam(r, "d")
	if got == nil {
		t.Fatal("parseInclusiveDateParam = nil")
	}
	want := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("parseInclusiveDateParam = %v, want %v (end-date shifted by +1 day)", got, want)
	}

	// Verify the original param isn't mutated: re-parsing returns the
	// un-shifted date.
	orig := parseDateParam(r, "d")
	if orig == nil || !orig.Equal(time.Date(2026, 4, 19, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("parseDateParam(d) after inclusive parse = %v, want 2026-04-19", orig)
	}

	if parseInclusiveDateParam(r, "missing") != nil {
		t.Error("parseInclusiveDateParam(missing) should return nil")
	}
}

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

func TestParseAdminTxFilters(t *testing.T) {
	r := httptest.NewRequest("GET",
		"/?start_date=2026-04-01&end_date=2026-04-30"+
			"&account_id=acc1&user_id=u1&connection_id=c1&category=food"+
			"&min_amount=10.5&max_amount=99&search=coffee"+
			"&pending=true&search_mode=words&search_field=name"+
			"&sort=asc&tags=a,b&any_tag=c,d", nil)

	p := parseAdminTxFilters(r)

	if p.StartDate == nil || !p.StartDate.Equal(time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("StartDate = %v, want 2026-04-01", p.StartDate)
	}
	if p.EndDate == nil || !p.EndDate.Equal(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("EndDate = %v, want 2026-05-01 (inclusive +1d)", p.EndDate)
	}
	if p.AccountID == nil || *p.AccountID != "acc1" {
		t.Errorf("AccountID = %v, want acc1", p.AccountID)
	}
	if p.UserID == nil || *p.UserID != "u1" {
		t.Errorf("UserID = %v, want u1", p.UserID)
	}
	if p.ConnectionID == nil || *p.ConnectionID != "c1" {
		t.Errorf("ConnectionID = %v, want c1", p.ConnectionID)
	}
	if p.CategorySlug == nil || *p.CategorySlug != "food" {
		t.Errorf("CategorySlug = %v, want food", p.CategorySlug)
	}
	if p.MinAmount == nil || *p.MinAmount != 10.5 {
		t.Errorf("MinAmount = %v, want 10.5", p.MinAmount)
	}
	if p.MaxAmount == nil || *p.MaxAmount != 99 {
		t.Errorf("MaxAmount = %v, want 99", p.MaxAmount)
	}
	if p.Search == nil || *p.Search != "coffee" {
		t.Errorf("Search = %v, want coffee", p.Search)
	}
	if p.Pending == nil || *p.Pending != true {
		t.Errorf("Pending = %v, want true", p.Pending)
	}
	if p.SearchMode == nil || *p.SearchMode != "words" {
		t.Errorf("SearchMode = %v, want words", p.SearchMode)
	}
	if p.SearchField == nil || *p.SearchField != "name" {
		t.Errorf("SearchField = %v, want name", p.SearchField)
	}
	if p.SortOrder != "asc" {
		t.Errorf("SortOrder = %q, want asc", p.SortOrder)
	}
	if got, want := p.Tags, []string{"a", "b"}; !equalStrings(got, want) {
		t.Errorf("Tags = %v, want %v", got, want)
	}
	if got, want := p.AnyTag, []string{"c", "d"}; !equalStrings(got, want) {
		t.Errorf("AnyTag = %v, want %v", got, want)
	}
	if p.Page != 0 || p.PageSize != 0 {
		t.Errorf("Page/PageSize = %d/%d, want 0/0 (caller-controlled)", p.Page, p.PageSize)
	}
}

func TestParseAdminTxFiltersEmpty(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	p := parseAdminTxFilters(r)

	if p.StartDate != nil || p.EndDate != nil ||
		p.AccountID != nil || p.UserID != nil || p.ConnectionID != nil ||
		p.CategorySlug != nil || p.MinAmount != nil || p.MaxAmount != nil ||
		p.Search != nil || p.Pending != nil ||
		p.SearchMode != nil || p.SearchField != nil {
		t.Errorf("expected all-nil filter pointers on empty request, got %+v", p)
	}
	if p.SortOrder != "" {
		t.Errorf("SortOrder = %q, want empty (defaults to desc downstream)", p.SortOrder)
	}
	if len(p.Tags) != 0 || len(p.AnyTag) != 0 {
		t.Errorf("Tags/AnyTag should be empty, got %v/%v", p.Tags, p.AnyTag)
	}
}

func TestParseAdminTxFiltersIgnoresInvalid(t *testing.T) {
	// Invalid search_mode and search_field should be silently dropped, not
	// passed through to the service layer. Matches admin "silent failure"
	// semantics for stale URLs.
	r := httptest.NewRequest("GET", "/?search_mode=bogus&search_field=bogus&min_amount=abc&start_date=not-a-date", nil)
	p := parseAdminTxFilters(r)
	if p.SearchMode != nil {
		t.Errorf("SearchMode should be nil for invalid value, got %v", *p.SearchMode)
	}
	if p.SearchField != nil {
		t.Errorf("SearchField should be nil for invalid value, got %v", *p.SearchField)
	}
	if p.MinAmount != nil {
		t.Errorf("MinAmount should be nil for non-numeric, got %v", *p.MinAmount)
	}
	if p.StartDate != nil {
		t.Errorf("StartDate should be nil for malformed date, got %v", *p.StartDate)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
		return pgconv.Timestamptz(now.Add(-d))
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

func TestCoerceTime(t *testing.T) {
	ref := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
	rfc3339 := ref.Format(time.RFC3339)
	rfc3339Nano := ref.Format(time.RFC3339Nano)
	emptyStr := ""
	junkStr := "not-a-timestamp"
	rfcStr := rfc3339
	tests := []struct {
		name    string
		in      any
		wantOK  bool
		wantRaw string
		wantEq  bool // expect returned time == ref
	}{
		{"time.Time", ref, true, "", true},
		{"*time.Time non-nil", &ref, true, "", true},
		{"*time.Time nil", (*time.Time)(nil), false, "", false},
		{"string RFC3339", rfc3339, true, "", true},
		{"string RFC3339Nano", rfc3339Nano, true, "", true},
		{"string empty", "", false, "", false},
		{"string junk echoes raw", "not-a-timestamp", false, "not-a-timestamp", false},
		{"*string non-nil RFC3339", &rfcStr, true, "", true},
		{"*string nil", (*string)(nil), false, "", false},
		{"*string empty", &emptyStr, false, "", false},
		{"*string junk echoes raw", &junkStr, false, "not-a-timestamp", false},
		{"pgtype.Timestamptz valid", pgtype.Timestamptz{Time: ref, Valid: true}, true, "", true},
		{"pgtype.Timestamptz invalid", pgtype.Timestamptz{}, false, "", false},
		{"unsupported type", 42, false, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, raw, ok := coerceTime(tc.in)
			if ok != tc.wantOK {
				t.Errorf("ok = %v, want %v", ok, tc.wantOK)
			}
			if raw != tc.wantRaw {
				t.Errorf("raw = %q, want %q", raw, tc.wantRaw)
			}
			if tc.wantEq && !got.Equal(ref) {
				t.Errorf("time = %v, want %v", got, ref)
			}
		})
	}
}

func TestFormatCoercedTime(t *testing.T) {
	ref := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
	stamp := func(tm time.Time) string { return tm.UTC().Format("2006-01-02") }
	tests := []struct {
		name string
		in   any
		want string
	}{
		{"valid time", ref, "2024-05-01"},
		{"empty input", "", ""},
		{"nil pointer", (*string)(nil), ""},
		{"junk string echoes raw", "not-a-timestamp", "not-a-timestamp"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatCoercedTime(tc.in, stamp); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
