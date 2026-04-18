package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestReviewsAliasHandler guards the `/reviews` alias that the dashboard
// "Attention" CTA and the `gr` keyboard shortcut both depend on. If someone
// deletes the alias, those links 404.
func TestReviewsAliasHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/reviews", nil)
	w := httptest.NewRecorder()
	ReviewsAliasHandler(w, req)

	if w.Code != http.StatusMovedPermanently {
		t.Fatalf("expected 301, got %d", w.Code)
	}
	got := w.Header().Get("Location")
	want := "/transactions?tags=needs-review"
	if got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}
