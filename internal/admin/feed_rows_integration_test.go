//go:build integration && !headless && !lite

package admin

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"breadbox/internal/app"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	"breadbox/internal/testutil"

	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5/pgtype"
)

// TestFeedRowsHandler exercises the inline "Load older activity" endpoint
// (GET /-/feed/rows). It seeds backdated comment annotations across multiple
// 3-day windows and asserts:
//
//   - a `before` cursor with events in the older window returns rendered <li>
//     rows plus the X-Feed-Next-Before / X-Feed-Last-Day / X-Feed-At-Max
//     pagination headers;
//   - empty 3-day windows are skipped server-side so a single call still
//     lands on the next non-empty window;
//   - a missing cursor (or one past the lookback cap with nothing older)
//     returns an empty body flagged X-Feed-At-Max:1 so the client renders
//     "End of feed".
func TestFeedRowsHandler(t *testing.T) {
	pool, q := testutil.ServicePool(t)
	svc := newTestSvc(t)
	ctx := context.Background()

	a := &app.App{DB: pool, Queries: q, Logger: slog.Default()}
	tr := &TemplateRenderer{sm: &scs.SessionManager{}}
	handler := FeedRowsHandler(a, svc, tr)

	user := testutil.MustCreateUser(t, q, "Alice")
	conn := testutil.MustCreateConnection(t, q, user.ID, "feed-rows-conn")
	acct := testutil.MustCreateAccount(t, q, conn.ID, "feed-rows-acct", "Checking")
	actor := service.Actor{Type: "user", ID: pgconv.FormatUUID(user.ID), Name: "Alice"}

	now := time.Now()

	// seedComment creates one comment on a fresh transaction and backdates the
	// annotation's created_at so the feed treats it as historical activity.
	seedComment := func(extID, txName, body string, ts time.Time) pgtype.UUID {
		t.Helper()
		txn := testutil.MustCreateTransaction(t, q, acct.ID, extID, txName, 500, "2026-04-01")
		if _, err := svc.CreateComment(ctx, service.CreateCommentParams{
			TransactionID: pgconv.FormatUUID(txn.ID),
			Content:       body,
			Actor:         actor,
		}); err != nil {
			t.Fatalf("CreateComment: %v", err)
		}
		if _, err := pool.Exec(ctx,
			"UPDATE annotations SET created_at = $1 WHERE transaction_id = $2 AND kind = 'comment'",
			ts, txn.ID,
		); err != nil {
			t.Fatalf("backdate comment: %v", err)
		}
		return txn.ID
	}

	call := func(query string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/-/feed/rows?"+query, nil)
		w := httptest.NewRecorder()
		handler(w, req)
		return w
	}

	// One comment 5 days old (in the window just past the default 3-day page)
	// and one 10 days old (two empty windows further back).
	seedComment("feed-rows-tx-5d", "Coffee Bar", "five days ago", now.Add(-5*24*time.Hour))
	seedComment("feed-rows-tx-10d", "Old Diner", "ten days ago", now.Add(-10*24*time.Hour))

	t.Run("returns_older_rows_with_pagination_headers", func(t *testing.T) {
		// before = 3 days ago → window [6d, 3d) contains the 5-day-old comment.
		w := call("before=" + now.Add(-3*24*time.Hour).UTC().Format(time.RFC3339))
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		body := w.Body.String()
		if !strings.Contains(body, "<li") {
			t.Fatalf("expected <li> rows in body, got: %s", body)
		}
		if !strings.Contains(body, "Coffee Bar") {
			t.Errorf("expected the 5-day-old comment's transaction in body, got: %s", body)
		}
		if w.Header().Get("X-Feed-Next-Before") == "" {
			t.Errorf("expected X-Feed-Next-Before header to be set")
		}
		if w.Header().Get("X-Feed-Last-Day") == "" {
			t.Errorf("expected X-Feed-Last-Day header to be set")
		}
		if got := w.Header().Get("X-Feed-At-Max"); got != "0" {
			t.Errorf("expected X-Feed-At-Max=0 (more history remains), got %q", got)
		}
	})

	t.Run("skips_empty_windows_to_next_content", func(t *testing.T) {
		// before = 6 days ago → window [9d, 6d) is empty; the handler must walk
		// back to [12d, 9d) and surface the 10-day-old comment in one call.
		w := call("before=" + now.Add(-6*24*time.Hour).UTC().Format(time.RFC3339))
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		body := w.Body.String()
		if !strings.Contains(body, "Old Diner") {
			t.Fatalf("expected the 10-day-old comment after skipping an empty window, got: %s", body)
		}
	})

	t.Run("no_cursor_signals_end_of_feed", func(t *testing.T) {
		w := call("")
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if strings.TrimSpace(w.Body.String()) != "" {
			t.Errorf("expected empty body with no cursor, got: %s", w.Body.String())
		}
		if got := w.Header().Get("X-Feed-At-Max"); got != "1" {
			t.Errorf("expected X-Feed-At-Max=1 with no cursor, got %q", got)
		}
	})

	t.Run("past_cap_with_no_history_signals_end", func(t *testing.T) {
		// before = 29 days ago, no events that old → walk to the 30-day cap
		// and return an empty, end-flagged response.
		w := call("before=" + now.Add(-29*24*time.Hour).UTC().Format(time.RFC3339))
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if strings.TrimSpace(w.Body.String()) != "" {
			t.Errorf("expected empty body past the lookback cap, got: %s", w.Body.String())
		}
		if got := w.Header().Get("X-Feed-At-Max"); got != "1" {
			t.Errorf("expected X-Feed-At-Max=1 past the cap, got %q", got)
		}
	})
}
