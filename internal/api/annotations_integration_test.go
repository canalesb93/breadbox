//go:build integration && !lite

// Integration tests for GET /api/v1/transactions/{id}/annotations.
// Run with:
//   DATABASE_URL="postgres://breadbox:breadbox@localhost:5432/breadbox_test?sslmode=disable" \
//   go test -tags integration -count=1 -p 1 -v ./internal/api/... -run TestAPI_ListAnnotations
package api

import (
	"net/http"
	"testing"
	"time"

	"breadbox/internal/pgconv"
	"breadbox/internal/service"
)

// annotationRow mirrors the per-row JSON shape the handler emits, with the
// fields each test checks. Unrecognized fields are tolerated.
type annotationRow struct {
	ID            string         `json:"id"`
	ShortID       string         `json:"short_id"`
	TransactionID string         `json:"transaction_id"`
	Kind          string         `json:"kind"`
	ActorType     string         `json:"actor_type"`
	ActorName     string         `json:"actor_name"`
	Content       string         `json:"content,omitempty"`
	Summary       string         `json:"summary,omitempty"`
	Action        string         `json:"action,omitempty"`
	CreatedAt     string         `json:"created_at"`
	Payload       map[string]any `json:"payload,omitempty"`
}

// listAnnotations issues GET /api/v1/transactions/{txnID}/annotations[?query]
// and returns the parsed row list. Fails the test on unexpected status.
func listAnnotations(t *testing.T, env *testEnv, txnID, query string) []annotationRow {
	t.Helper()
	path := "/api/v1/transactions/" + txnID + "/annotations"
	if query != "" {
		path += "?" + query
	}
	resp := env.doGet(t, path)
	assertStatus(t, resp, http.StatusOK)
	var body struct {
		Annotations []annotationRow `json:"annotations"`
	}
	parseJSON(t, resp, &body)
	return body.Annotations
}

// userActor builds a service.Actor for a human user. Tests seed annotations
// directly via service methods (which bypass HTTP middleware) and so pass the
// actor explicitly.
func userActor(name string) service.Actor {
	return service.Actor{Type: "user", ID: "", Name: name}
}

// TestAPI_ListAnnotations_Empty: a transaction with no activity returns an
// empty annotations array (and 200 OK).
func TestAPI_ListAnnotations_Empty(t *testing.T) {
	env := setupTestEnv(t)
	_, _, txn := seedFixture(t, env.Queries)

	rows := listAnnotations(t, env, pgconv.FormatUUID(txn.ID), "")
	if len(rows) != 0 {
		t.Fatalf("want empty annotations, got %d rows: %+v", len(rows), rows)
	}
}

// TestAPI_ListAnnotations_AfterComment: a single comment surfaces with the
// expected fields populated.
func TestAPI_ListAnnotations_AfterComment(t *testing.T) {
	env := setupTestEnv(t)
	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	if _, err := env.Service.CreateComment(t.Context(), service.CreateCommentParams{
		TransactionID: txnID,
		Content:       "needs receipt",
		Actor:         userActor("Alice"),
	}); err != nil {
		t.Fatalf("seed comment: %v", err)
	}

	rows := listAnnotations(t, env, txnID, "")
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d: %+v", len(rows), rows)
	}
	row := rows[0]
	if row.Kind != "comment" {
		t.Errorf("want kind=comment, got %q", row.Kind)
	}
	if row.ActorType != "user" {
		t.Errorf("want actor_type=user, got %q", row.ActorType)
	}
	if row.ActorName != "Alice" {
		t.Errorf("want actor_name=Alice, got %q", row.ActorName)
	}
	if row.Content != "needs receipt" {
		t.Errorf("want content='needs receipt', got %q", row.Content)
	}
	if row.ID == "" || row.ShortID == "" {
		t.Errorf("want id and short_id populated, got id=%q short_id=%q", row.ID, row.ShortID)
	}
	if row.TransactionID == "" {
		t.Error("want transaction_id populated")
	}
}

// TestAPI_ListAnnotations_AfterMultipleEvents: category set + tag added +
// comment all surface, ordered ASC by created_at.
func TestAPI_ListAnnotations_AfterMultipleEvents(t *testing.T) {
	env := setupTestEnv(t)
	_, _, txn := seedFixture(t, env.Queries)
	cat := seedUncategorized(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	if err := env.Service.SetTransactionCategory(t.Context(), txnID, pgconv.FormatUUID(cat.ID), userActor("Alice")); err != nil {
		t.Fatalf("set category: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if _, _, err := env.Service.AddTransactionTag(t.Context(), txnID, "needs-review", userActor("Alice")); err != nil {
		t.Fatalf("add tag: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if _, err := env.Service.CreateComment(t.Context(), service.CreateCommentParams{
		TransactionID: txnID,
		Content:       "follow up",
		Actor:         userActor("Alice"),
	}); err != nil {
		t.Fatalf("create comment: %v", err)
	}

	rows := listAnnotations(t, env, txnID, "")
	if len(rows) < 3 {
		t.Fatalf("want at least 3 rows, got %d: %+v", len(rows), rows)
	}

	// Verify ASC ordering by parsing created_at as RFC3339.
	for i := 1; i < len(rows); i++ {
		prev, err1 := time.Parse(time.RFC3339, rows[i-1].CreatedAt)
		curr, err2 := time.Parse(time.RFC3339, rows[i].CreatedAt)
		if err1 != nil || err2 != nil {
			t.Fatalf("parse created_at: %v / %v", err1, err2)
		}
		if curr.Before(prev) {
			t.Errorf("rows not ASC at index %d: %s before %s", i, rows[i].CreatedAt, rows[i-1].CreatedAt)
		}
	}

	// Verify the kinds we produced are present (enrichment may dedup
	// adjacent comment-vs-tag-note rows, but our comment is not a tag note
	// so all three rows should survive).
	kinds := make(map[string]int)
	for _, r := range rows {
		kinds[r.Kind]++
	}
	if kinds["category_set"] == 0 {
		t.Errorf("want a category_set row, got kinds=%+v", kinds)
	}
	if kinds["tag_added"] == 0 {
		t.Errorf("want a tag_added row, got kinds=%+v", kinds)
	}
	if kinds["comment"] == 0 {
		t.Errorf("want a comment row, got kinds=%+v", kinds)
	}
}

// TestAPI_ListAnnotations_FilterByKind: ?kind=comment returns only the
// comment row.
func TestAPI_ListAnnotations_FilterByKind(t *testing.T) {
	env := setupTestEnv(t)
	_, _, txn := seedFixture(t, env.Queries)
	cat := seedUncategorized(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	if err := env.Service.SetTransactionCategory(t.Context(), txnID, pgconv.FormatUUID(cat.ID), userActor("Alice")); err != nil {
		t.Fatalf("set category: %v", err)
	}
	if _, err := env.Service.CreateComment(t.Context(), service.CreateCommentParams{
		TransactionID: txnID,
		Content:       "comment-only",
		Actor:         userActor("Alice"),
	}); err != nil {
		t.Fatalf("create comment: %v", err)
	}

	rows := listAnnotations(t, env, txnID, "kind=comment")
	if len(rows) == 0 {
		t.Fatalf("want at least 1 comment row, got 0")
	}
	for _, r := range rows {
		if r.Kind != "comment" {
			t.Errorf("kind=comment filter leaked %q row", r.Kind)
		}
	}
}

// TestAPI_ListAnnotations_FilterByMultipleKinds: ?kind=comment&kind=tag_added
// returns the union of both kinds.
func TestAPI_ListAnnotations_FilterByMultipleKinds(t *testing.T) {
	env := setupTestEnv(t)
	_, _, txn := seedFixture(t, env.Queries)
	cat := seedUncategorized(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	if err := env.Service.SetTransactionCategory(t.Context(), txnID, pgconv.FormatUUID(cat.ID), userActor("Alice")); err != nil {
		t.Fatalf("set category: %v", err)
	}
	if _, _, err := env.Service.AddTransactionTag(t.Context(), txnID, "needs-review", userActor("Alice")); err != nil {
		t.Fatalf("add tag: %v", err)
	}
	if _, err := env.Service.CreateComment(t.Context(), service.CreateCommentParams{
		TransactionID: txnID,
		Content:       "hello",
		Actor:         userActor("Alice"),
	}); err != nil {
		t.Fatalf("create comment: %v", err)
	}

	// Repeatable form: ?kind=comment&kind=tag_added
	rows := listAnnotations(t, env, txnID, "kind=comment&kind=tag_added")
	for _, r := range rows {
		if r.Kind != "comment" && r.Kind != "tag_added" {
			t.Errorf("multi-kind filter leaked %q row", r.Kind)
		}
	}
	saw := map[string]bool{}
	for _, r := range rows {
		saw[r.Kind] = true
	}
	if !saw["comment"] || !saw["tag_added"] {
		t.Errorf("want both kinds present, got %+v", saw)
	}

	// Comma-separated form: ?kind=comment,tag_added
	csvRows := listAnnotations(t, env, txnID, "kind=comment,tag_added")
	if len(csvRows) != len(rows) {
		t.Errorf("comma-form returned %d rows, repeatable returned %d", len(csvRows), len(rows))
	}
}

// TestAPI_ListAnnotations_FilterByActorType: ?actor_type=user excludes
// non-user rows.
func TestAPI_ListAnnotations_FilterByActorType(t *testing.T) {
	env := setupTestEnv(t)
	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	// User comment.
	if _, err := env.Service.CreateComment(t.Context(), service.CreateCommentParams{
		TransactionID: txnID,
		Content:       "human",
		Actor:         userActor("Alice"),
	}); err != nil {
		t.Fatalf("user comment: %v", err)
	}
	// Agent comment.
	if _, err := env.Service.CreateComment(t.Context(), service.CreateCommentParams{
		TransactionID: txnID,
		Content:       "robot",
		Actor:         service.Actor{Type: "agent", ID: "key-1", Name: "TestAgent"},
	}); err != nil {
		t.Fatalf("agent comment: %v", err)
	}

	rows := listAnnotations(t, env, txnID, "actor_type=user")
	if len(rows) == 0 {
		t.Fatalf("want at least 1 user-actor row, got 0")
	}
	for _, r := range rows {
		if r.ActorType != "user" {
			t.Errorf("actor_type=user filter leaked %q row", r.ActorType)
		}
	}
}

// TestAPI_ListAnnotations_Since: with a cursor between two annotations, only
// the second is returned.
func TestAPI_ListAnnotations_Since(t *testing.T) {
	env := setupTestEnv(t)
	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	first, err := env.Service.CreateComment(t.Context(), service.CreateCommentParams{
		TransactionID: txnID,
		Content:       "first",
		Actor:         userActor("Alice"),
	})
	if err != nil {
		t.Fatalf("first comment: %v", err)
	}

	// Sleep beyond second precision to make the RFC3339 cursor unambiguous.
	time.Sleep(1100 * time.Millisecond)
	cursor := time.Now().UTC()
	time.Sleep(1100 * time.Millisecond)

	if _, err := env.Service.CreateComment(t.Context(), service.CreateCommentParams{
		TransactionID: txnID,
		Content:       "second",
		Actor:         userActor("Alice"),
	}); err != nil {
		t.Fatalf("second comment: %v", err)
	}

	rows := listAnnotations(t, env, txnID, "since="+cursor.Format(time.RFC3339))
	if len(rows) != 1 {
		t.Fatalf("want 1 row after since cursor, got %d: %+v (first=%q)", len(rows), rows, first.ID)
	}
	if rows[0].Content != "second" {
		t.Errorf("want content='second', got %q", rows[0].Content)
	}
}

// TestAPI_ListAnnotations_Limit: with N annotations, ?limit=2 returns the
// most recent 2.
func TestAPI_ListAnnotations_Limit(t *testing.T) {
	env := setupTestEnv(t)
	_, _, txn := seedFixture(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	for i, content := range []string{"one", "two", "three", "four"} {
		if _, err := env.Service.CreateComment(t.Context(), service.CreateCommentParams{
			TransactionID: txnID,
			Content:       content,
			Actor:         userActor("Alice"),
		}); err != nil {
			t.Fatalf("comment %d: %v", i, err)
		}
		// Spread events so ASC ordering is deterministic.
		time.Sleep(15 * time.Millisecond)
	}

	rows := listAnnotations(t, env, txnID, "limit=2")
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d: %+v", len(rows), rows)
	}
	// limit returns the tail, still ASC. So the last two created rows.
	if rows[0].Content != "three" || rows[1].Content != "four" {
		t.Errorf("want tail [three, four], got [%q, %q]", rows[0].Content, rows[1].Content)
	}
}

// TestAPI_ListAnnotations_Raw: ?raw=true bypasses enrichment, so the
// derived Summary field is empty (enriched runs populate it for non-comment
// kinds; for comments raw vs enriched both leave Summary empty, but the
// rule_applied dedup is also disabled — for this test we just assert that
// raw mode does not blow up and returns the rows.
func TestAPI_ListAnnotations_Raw(t *testing.T) {
	env := setupTestEnv(t)
	_, _, txn := seedFixture(t, env.Queries)
	cat := seedUncategorized(t, env.Queries)
	txnID := pgconv.FormatUUID(txn.ID)

	if err := env.Service.SetTransactionCategory(t.Context(), txnID, pgconv.FormatUUID(cat.ID), userActor("Alice")); err != nil {
		t.Fatalf("set category: %v", err)
	}

	enriched := listAnnotations(t, env, txnID, "")
	raw := listAnnotations(t, env, txnID, "raw=true")

	if len(raw) == 0 {
		t.Fatalf("raw mode returned 0 rows")
	}

	// In enriched mode the category_set row carries a populated Summary
	// (e.g. "Alice set category to Uncategorized"). In raw mode the Summary
	// is empty because enrichment is the thing that builds it.
	var enrichedCatRow, rawCatRow *annotationRow
	for i := range enriched {
		if enriched[i].Kind == "category_set" {
			enrichedCatRow = &enriched[i]
			break
		}
	}
	for i := range raw {
		if raw[i].Kind == "category_set" {
			rawCatRow = &raw[i]
			break
		}
	}
	if enrichedCatRow == nil || rawCatRow == nil {
		t.Fatalf("missing category_set row: enriched=%v raw=%v", enrichedCatRow, rawCatRow)
	}
	if enrichedCatRow.Summary == "" {
		t.Errorf("enriched category_set row should carry Summary, got empty")
	}
	if rawCatRow.Summary != "" {
		t.Errorf("raw category_set row should not carry Summary, got %q", rawCatRow.Summary)
	}
}

// TestAPI_ListAnnotations_NotFound: unknown short_id → 404 NOT_FOUND.
//
// Mirrors the existing service behavior shared with the MCP list_annotations
// tool: short_id resolution misses cleanly to ErrNotFound (the DB lookup
// returns no rows, the resolver maps that to NOT_FOUND). A syntactically
// valid UUID that doesn't match any row is treated as resolved and returns
// an empty list — the resolver doesn't probe the transactions table for raw
// UUIDs. Both behaviors are intentional and shared with the MCP tool.
func TestAPI_ListAnnotations_NotFound(t *testing.T) {
	env := setupTestEnv(t)

	resp := env.doGet(t, "/api/v1/transactions/00000000/annotations")
	readErrorCode(t, resp, http.StatusNotFound, "NOT_FOUND")
}

// TestAPI_ListAnnotations_AcceptsShortID: lookup by short_id resolves to the
// same row set as lookup by UUID.
func TestAPI_ListAnnotations_AcceptsShortID(t *testing.T) {
	env := setupTestEnv(t)
	_, _, txn := seedFixture(t, env.Queries)
	if _, err := env.Service.CreateComment(t.Context(), service.CreateCommentParams{
		TransactionID: pgconv.FormatUUID(txn.ID),
		Content:       "via short id",
		Actor:         userActor("Alice"),
	}); err != nil {
		t.Fatalf("create comment: %v", err)
	}

	rows := listAnnotations(t, env, txn.ShortID, "")
	if len(rows) != 1 {
		t.Fatalf("want 1 row via short_id, got %d", len(rows))
	}
	if rows[0].Content != "via short id" {
		t.Errorf("want content via short id, got %q", rows[0].Content)
	}
}

// TestAPI_ListAnnotations_BadSince: malformed timestamp → 400 INVALID_PARAMETER.
func TestAPI_ListAnnotations_BadSince(t *testing.T) {
	env := setupTestEnv(t)
	_, _, txn := seedFixture(t, env.Queries)

	resp := env.doGet(t, "/api/v1/transactions/"+pgconv.FormatUUID(txn.ID)+"/annotations?since=not-a-timestamp")
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

// TestAPI_ListAnnotations_BadLimit: negative limit → 400 INVALID_PARAMETER.
func TestAPI_ListAnnotations_BadLimit(t *testing.T) {
	env := setupTestEnv(t)
	_, _, txn := seedFixture(t, env.Queries)

	resp := env.doGet(t, "/api/v1/transactions/"+pgconv.FormatUUID(txn.ID)+"/annotations?limit=-1")
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")

	resp = env.doGet(t, "/api/v1/transactions/"+pgconv.FormatUUID(txn.ID)+"/annotations?limit=notanumber")
	readErrorCode(t, resp, http.StatusBadRequest, "INVALID_PARAMETER")
}

// TestAPI_ListAnnotations_AllowsReadOnlyKey: read-only API key can list
// annotations (read scope).
func TestAPI_ListAnnotations_AllowsReadOnlyKey(t *testing.T) {
	env := setupReadOnlyEnv(t)
	_, _, txn := seedFixture(t, env.Queries)

	resp := env.doGet(t, "/api/v1/transactions/"+pgconv.FormatUUID(txn.ID)+"/annotations")
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}
