//go:build integration

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
	"github.com/go-chi/chi/v5"
)

// TestTimelineRowsHandler_RendersNewRows verifies that GET
// /-/transactions/{id}/timeline/rows?since=<ts> returns rendered <li> rows
// for activity entries created after the given timestamp.
func TestTimelineRowsHandler_RendersNewRows(t *testing.T) {
	pool, q := testutil.ServicePool(t)
	svc := newTestSvc(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, q, "Alice")
	conn := testutil.MustCreateConnection(t, q, user.ID, "ext-conn-1")
	acct := testutil.MustCreateAccount(t, q, conn.ID, "ext-acct-1", "Checking")
	txn := testutil.MustCreateTransaction(t, q, acct.ID, "ext-txn-1", "Coffee", 500, "2026-04-01")

	// Capture a "before" timestamp, then write a comment annotation. The
	// render endpoint should return the comment row when asked for
	// "everything since <before>".
	// `before` rounds to seconds when serialized as RFC3339, and the
	// annotation's CreatedAt rounds the same way. To guarantee
	// CreatedAt > before once both have second-resolution, anchor
	// `before` two seconds in the past.
	before := time.Now().Add(-2 * time.Second)

	actor := service.Actor{Type: "user", ID: pgconv.FormatUUID(user.ID), Name: "Alice"}
	if _, err := svc.CreateComment(ctx, service.CreateCommentParams{
		TransactionID: pgconv.FormatUUID(txn.ID),
		Content:       "First note",
		Actor:         actor,
	}); err != nil {
		t.Fatalf("CreateComment: %v", err)
	}

	r := chi.NewRouter()
	a := &app.App{DB: pool, Queries: q, Logger: slog.Default()}
	r.Get("/-/transactions/{id}/timeline/rows", TimelineRowsHandler(a, &scs.SessionManager{}, svc))

	url := "/-/transactions/" + pgconv.FormatUUID(txn.ID) + "/timeline/rows?since=" + before.Format(time.RFC3339)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "<li") {
		t.Fatalf("expected <li> markup in body, got: %s", body)
	}
	// The comment bubble template renders the actor name + "commented".
	if !strings.Contains(body, "commented") {
		t.Errorf("expected 'commented' meta-line in body, got: %s", body)
	}
	if !strings.Contains(body, "First note") {
		t.Errorf("expected comment body 'First note' in markup, got: %s", body)
	}
}

// TestTimelineRowsHandler_EmptyWhenNoNewRows verifies that the render
// endpoint returns an empty 200 body when there are no entries newer than
// the given since cursor.
func TestTimelineRowsHandler_EmptyWhenNoNewRows(t *testing.T) {
	pool, q := testutil.ServicePool(t)
	svc := newTestSvc(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, q, "Bob")
	conn := testutil.MustCreateConnection(t, q, user.ID, "ext-conn-2")
	acct := testutil.MustCreateAccount(t, q, conn.ID, "ext-acct-2", "Checking")
	txn := testutil.MustCreateTransaction(t, q, acct.ID, "ext-txn-2", "Lunch", 1200, "2026-04-02")

	actor := service.Actor{Type: "user", ID: pgconv.FormatUUID(user.ID), Name: "Bob"}
	if _, err := svc.CreateComment(ctx, service.CreateCommentParams{
		TransactionID: pgconv.FormatUUID(txn.ID),
		Content:       "Old note",
		Actor:         actor,
	}); err != nil {
		t.Fatalf("CreateComment: %v", err)
	}

	// Use a since timestamp well in the future — nothing should match.
	since := time.Now().Add(1 * time.Hour).Format(time.RFC3339)

	r := chi.NewRouter()
	a := &app.App{DB: pool, Queries: q, Logger: slog.Default()}
	r.Get("/-/transactions/{id}/timeline/rows", TimelineRowsHandler(a, &scs.SessionManager{}, svc))

	url := "/-/transactions/" + pgconv.FormatUUID(txn.ID) + "/timeline/rows?since=" + since
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if strings.TrimSpace(w.Body.String()) != "" {
		t.Fatalf("expected empty body when nothing newer than since, got: %s", w.Body.String())
	}
}

// TestTimelineRowsHandler_NoSinceCursor verifies that calling the endpoint
// without a `since` cursor returns an empty 200 (the JS uses the absence
// of a cursor as a sentinel for "first load — page already has every row").
func TestTimelineRowsHandler_NoSinceCursor(t *testing.T) {
	pool, q := testutil.ServicePool(t)
	svc := newTestSvc(t)
	ctx := context.Background()

	user := testutil.MustCreateUser(t, q, "Charlie")
	conn := testutil.MustCreateConnection(t, q, user.ID, "ext-conn-3")
	acct := testutil.MustCreateAccount(t, q, conn.ID, "ext-acct-3", "Checking")
	txn := testutil.MustCreateTransaction(t, q, acct.ID, "ext-txn-3", "Coffee", 350, "2026-04-03")

	actor := service.Actor{Type: "user", ID: pgconv.FormatUUID(user.ID), Name: "Charlie"}
	if _, err := svc.CreateComment(ctx, service.CreateCommentParams{
		TransactionID: pgconv.FormatUUID(txn.ID),
		Content:       "A note",
		Actor:         actor,
	}); err != nil {
		t.Fatalf("CreateComment: %v", err)
	}

	r := chi.NewRouter()
	a := &app.App{DB: pool, Queries: q, Logger: slog.Default()}
	r.Get("/-/transactions/{id}/timeline/rows", TimelineRowsHandler(a, &scs.SessionManager{}, svc))

	req := httptest.NewRequest(http.MethodGet, "/-/transactions/"+pgconv.FormatUUID(txn.ID)+"/timeline/rows", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if strings.TrimSpace(w.Body.String()) != "" {
		t.Fatalf("expected empty body when no since cursor passed, got: %s", w.Body.String())
	}
}
