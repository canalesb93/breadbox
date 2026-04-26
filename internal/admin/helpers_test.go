package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"breadbox/internal/pgconv"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// withChiParam wraps r with a chi.RouteContext that has key=value, so
// chi.URLParam(r, key) returns value when the request bypasses the router.
func withChiParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

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
		return pgconv.Int4(minutes)
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

func TestParseURLUUIDOrInvalid(t *testing.T) {
	t.Run("valid UUID returns ok", func(t *testing.T) {
		r := withChiParam(httptest.NewRequest("GET", "/", nil), "id", "11111111-1111-1111-1111-111111111111")
		w := httptest.NewRecorder()
		got, ok := parseURLUUIDOrInvalid(w, r, "id", "Invalid ID")
		if !ok {
			t.Fatalf("ok = false, want true; body=%s", w.Body.String())
		}
		if !got.Valid {
			t.Error("returned UUID is not valid")
		}
		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want default 200 (helper should not write on success)", w.Code)
		}
	})

	t.Run("invalid UUID writes 400 with label", func(t *testing.T) {
		r := withChiParam(httptest.NewRequest("GET", "/", nil), "id", "not-a-uuid")
		w := httptest.NewRecorder()
		_, ok := parseURLUUIDOrInvalid(w, r, "id", "Invalid connection ID")
		if ok {
			t.Fatal("ok = true, want false")
		}
		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
		}
		var body map[string]string
		if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["error"] != "Invalid connection ID" {
			t.Errorf("error = %q, want %q", body["error"], "Invalid connection ID")
		}
	})
}

func TestParseURLUUIDOrNotFound(t *testing.T) {
	sm := scs.New()
	tr, err := NewTemplateRenderer(sm)
	if err != nil {
		t.Fatalf("NewTemplateRenderer: %v", err)
	}

	// Wrap the request in scs's session middleware so RenderNotFound's
	// IsViewer call doesn't panic on a missing session context.
	loadSession := func(r *http.Request) *http.Request {
		ctx, err := sm.Load(r.Context(), "")
		if err != nil {
			t.Fatalf("session load: %v", err)
		}
		return r.WithContext(ctx)
	}

	t.Run("valid UUID returns ok", func(t *testing.T) {
		r := loadSession(withChiParam(httptest.NewRequest("GET", "/", nil), "id", "22222222-2222-2222-2222-222222222222"))
		w := httptest.NewRecorder()
		got, ok := parseURLUUIDOrNotFound(w, r, tr, "id")
		if !ok {
			t.Fatalf("ok = false, want true; body=%s", w.Body.String())
		}
		if !got.Valid {
			t.Error("returned UUID is not valid")
		}
	})

	t.Run("invalid UUID renders 404", func(t *testing.T) {
		r := loadSession(withChiParam(httptest.NewRequest("GET", "/", nil), "id", "garbage"))
		w := httptest.NewRecorder()
		_, ok := parseURLUUIDOrNotFound(w, r, tr, "id")
		if ok {
			t.Fatal("ok = true, want false")
		}
		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
		}
	})
}
