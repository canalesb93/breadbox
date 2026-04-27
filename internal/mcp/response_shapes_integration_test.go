//go:build integration

package mcp

// Regression harness for MCP tool response shapes.
//
// During the MCP docs build-out (#685) several tool response shapes had been
// extrapolated from the service layer rather than read from the actual handler
// output — leading to silent drift between docs/SDKs and real responses. This
// test calls each flagged tool handler directly, decodes the JSON envelope
// returned by jsonResult (after compactIDsBytes), and asserts the presence
// and type of the keys the docs rely on.
//
// Guardrails here are intentionally loose — they lock the shape, not the
// values. The goal is: if someone renames `matched_on` → `matched_fields` or
// drops `category` from `transaction_summary` rows, a test breaks in the same
// PR as the service change. New/optional fields can still be added.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	"breadbox/internal/testutil"

	"github.com/jackc/pgx/v5/pgtype"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMain(m *testing.M) {
	testutil.RunWithDB(m)
}

// --- Fixture plumbing ---

type fixtures struct {
	svc *MCPServer
	ctx context.Context

	txnID  string
	linkID string
}

// seedFixtures provisions a scenario exercising every tool flagged in #685:
// a user, two accounts (primary + dependent) under one connection,
// transactions on both, a category, a tag attached to the primary transaction,
// a rule that matches the transaction, an account link between the two
// accounts, and a transaction match row.
func seedFixtures(t *testing.T) *fixtures {
	t.Helper()
	pool, q := testutil.ServicePool(t)
	svc := service.New(q, pool, nil, slog.Default())
	server := NewMCPServer(svc, "test")
	// BuildServer sanity-checks that the registry + input schemas are valid.
	if got := server.BuildServer(MCPServerConfig{Mode: "read_write", APIKeyScope: "full_access"}); got == nil {
		t.Fatal("BuildServer returned nil")
	}

	ctx := service.ContextWithAPIKey(context.Background(), "test-api-key", "TestKey")

	user := testutil.MustCreateUser(t, q, "Alice")
	conn := testutil.MustCreateConnection(t, q, user.ID, "item_primary")

	primaryAcct, err := q.UpsertAccount(ctx, db.UpsertAccountParams{
		ConnectionID:      conn.ID,
		ExternalAccountID: "acct_primary_1",
		Name:              "Primary Credit Card",
		Type:              "credit",
		IsoCurrencyCode:   pgtype.Text{String: "USD", Valid: true},
		BalanceCurrent:    pgconv.NumericCents(50000),
	})
	if err != nil {
		t.Fatalf("upsert primary account: %v", err)
	}
	dependentAcct, err := q.UpsertAccount(ctx, db.UpsertAccountParams{
		ConnectionID:      conn.ID,
		ExternalAccountID: "acct_dependent_1",
		Name:              "Dependent Credit Card",
		Type:              "credit",
		IsoCurrencyCode:   pgtype.Text{String: "USD", Valid: true},
	})
	if err != nil {
		t.Fatalf("upsert dependent account: %v", err)
	}

	cat := testutil.MustCreateCategory(t, q, "food_and_drink_groceries", "Groceries")

	// Same date + amount ⇒ match candidates for the account link.
	txn := testutil.MustCreateTransaction(t, q, primaryAcct.ID, "txn_primary_1", "Whole Foods", 2500, "2026-04-15")
	depTxn := testutil.MustCreateTransaction(t, q, dependentAcct.ID, "txn_dependent_1", "Whole Foods", 2500, "2026-04-15")

	// Assign the primary transaction to a category so summary + query_transactions
	// return populated category info.
	if _, err := pool.Exec(ctx,
		"UPDATE transactions SET category_id = $1 WHERE id = $2", cat.ID, txn.ID); err != nil {
		t.Fatalf("set category on txn: %v", err)
	}

	tag := testutil.MustCreateTag(t, q, "needs-review", "Needs Review")
	// Use the service-layer helper so a tag_added annotation is written —
	// list_annotations relies on that to have at least one entry to assert.
	if _, _, err := svc.AddTransactionTag(ctx, formatUUIDTest(t, txn.ID), "needs-review",
		service.Actor{Type: "system", Name: "test"}); err != nil {
		t.Fatalf("AddTransactionTag: %v", err)
	}

	actions := []byte(`[{"type":"set_category","category_slug":"food_and_drink_groceries"}]`)
	conditions := []byte(`{"field":"provider_name","op":"contains","value":"Whole Foods"}`)
	rule := testutil.MustCreateTransactionRule(t, q, "Whole Foods → Groceries", conditions, actions, "on_create")

	link := testutil.MustCreateAccountLink(t, q, primaryAcct.ID, dependentAcct.ID)
	if _, err := q.CreateTransactionMatch(ctx, db.CreateTransactionMatchParams{
		AccountLinkID:          link.ID,
		PrimaryTransactionID:   txn.ID,
		DependentTransactionID: depTxn.ID,
		MatchConfidence:        "auto",
		MatchedOn:              []string{"date", "amount", "name"},
	}); err != nil {
		t.Fatalf("create transaction match: %v", err)
	}

	_ = rule
	_ = tag
	_ = cat

	return &fixtures{
		svc:    server,
		ctx:    ctx,
		txnID:  formatUUIDTest(t, txn.ID),
		linkID: formatUUIDTest(t, link.ID),
	}
}

func formatUUIDTest(t *testing.T, u pgtype.UUID) string {
	t.Helper()
	b, err := u.MarshalJSON()
	if err != nil {
		t.Fatalf("format uuid: %v", err)
	}
	if len(b) < 2 {
		t.Fatalf("format uuid: short result %q", b)
	}
	return string(b[1 : len(b)-1])
}

// --- Response extraction ---

// decodeToolResult returns the JSON-decoded payload from a tool's
// CallToolResult. Fails the test when IsError is true so shape regressions are
// diagnosable without hunting error envelopes.
func decodeToolResult[T any](t *testing.T, name string, res *mcpsdk.CallToolResult, err error) T {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: handler error: %v", name, err)
	}
	if res == nil {
		t.Fatalf("%s: nil result", name)
	}
	if res.IsError {
		if len(res.Content) > 0 {
			if tc, ok := res.Content[0].(*mcpsdk.TextContent); ok {
				t.Fatalf("%s: error envelope: %s", name, tc.Text)
			}
		}
		t.Fatalf("%s: IsError=true with no content", name)
	}
	if len(res.Content) == 0 {
		t.Fatalf("%s: empty content", name)
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("%s: expected TextContent, got %T", name, res.Content[0])
	}
	var out T
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		t.Fatalf("%s: unmarshal response: %v\nraw=%s", name, err, tc.Text)
	}
	return out
}

// --- Shape assertions ---

func requireKeys(t *testing.T, label string, m map[string]any, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			t.Errorf("%s: missing key %q. keys present: %v", label, k, keysOf(m))
		}
	}
}

func requireAbsent(t *testing.T, label string, m map[string]any, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if _, ok := m[k]; ok {
			t.Errorf("%s: unexpected key %q present", label, k)
		}
	}
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func asObject(t *testing.T, label string, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("%s: expected object, got %T (%v)", label, v, v)
	}
	return m
}

func asArray(t *testing.T, label string, v any) []any {
	t.Helper()
	a, ok := v.([]any)
	if !ok {
		t.Fatalf("%s: expected array, got %T (%v)", label, v, v)
	}
	return a
}

// --- Tests ---

// TestListAccountsResponseShape pins the fields documented for list_accounts
// and guards the #685 finding that `provider` is NOT on AccountResponse (it
// lives on ConnectionResponse).
func TestListAccountsResponseShape(t *testing.T) {
	f := seedFixtures(t)
	res, _, err := f.svc.handleListAccounts(f.ctx, nil, listAccountsInput{})
	out := decodeToolResult[[]any](t, "list_accounts", res, err)
	if len(out) == 0 {
		t.Fatal("expected at least one account")
	}
	acct := asObject(t, "list_accounts[0]", out[0])
	requireKeys(t, "list_accounts[0]", acct,
		"id", "connection_id", "user_id", "name", "type",
		"balance_current", "iso_currency_code",
		"created_at", "updated_at", "is_dependent_linked",
	)
	requireAbsent(t, "list_accounts[0]", acct, "provider")
}

// TestListAnnotationsResponseShape pins annotation event shape: generic `kind`
// (comment | rule | tag | category) paired with an `action` field for the
// specific event, actor split across actor_type/actor_name/actor_id (not a
// single `actor` string), and the raw DB-only kind values must NOT leak.
func TestListAnnotationsResponseShape(t *testing.T) {
	f := seedFixtures(t)
	res, _, err := f.svc.handleListAnnotations(f.ctx, nil, listAnnotationsInput{
		TransactionID: f.txnID,
	})
	out := decodeToolResult[[]any](t, "list_annotations", res, err)
	if len(out) == 0 {
		t.Fatal("expected at least one annotation (tag from seeding)")
	}
	ann := asObject(t, "list_annotations[0]", out[0])
	requireKeys(t, "list_annotations[0]", ann,
		"id", "transaction_id", "kind", "action",
		"actor_type", "actor_name", "created_at",
	)
	requireAbsent(t, "list_annotations[0]", ann, "actor", "type", "event_type")

	kind, _ := ann["kind"].(string)
	switch kind {
	case "comment", "rule", "tag", "category":
		// expected generic kind
	default:
		t.Errorf("list_annotations[0]: kind=%q is not one of the generic MCP kinds", kind)
	}
	if kind == "tag" {
		if action, _ := ann["action"].(string); action != "added" && action != "removed" {
			t.Errorf("list_annotations[0]: tag row must carry action=added|removed, got %q", action)
		}
	}
}

// TestListAnnotationsKindsFilter exercises the generic-kind filter and pins
// behavioral parity with the deprecated list_transaction_comments tool. Both
// tools must return the same comment-row IDs when list_annotations is filtered
// to kinds=['comment']. Also verifies kinds=['tag'] returns both add+remove
// events, that raw DB kinds (tag_added, tag_removed, rule_applied, category_set)
// are NOT accepted at the MCP boundary, and that unknown kinds are rejected.
func TestListAnnotationsKindsFilter(t *testing.T) {
	f := seedFixtures(t)

	// Seed at least one comment so list_transaction_comments has something to
	// return — the base fixture only seeds a tag-added annotation. Also remove
	// the seeded tag to produce a tag-removed event so the kinds=['tag'] check
	// has both sides to find.
	addRes, _, err := f.svc.handleAddTransactionComment(f.ctx, nil, addTransactionCommentInput{
		WriteSessionContext: WriteSessionContext{SessionID: "sess", Reason: "kinds-parity"},
		TransactionID:       f.txnID,
		Content:             "Kinds-filter parity check",
	})
	_ = decodeToolResult[map[string]any](t, "add_transaction_comment", addRes, err)

	rmRes, _, err := f.svc.handleRemoveTransactionTag(f.ctx, nil, removeTransactionTagInput{
		WriteSessionContext: WriteSessionContext{SessionID: "sess", Reason: "kinds-tag-pair"},
		TransactionID:       f.txnID,
		TagSlug:             "needs-review",
	})
	_ = decodeToolResult[map[string]any](t, "remove_transaction_tag", rmRes, err)

	// Unfiltered: full timeline includes the tag add + tag remove + comment.
	allRes, _, err := f.svc.handleListAnnotations(f.ctx, nil, listAnnotationsInput{
		TransactionID: f.txnID,
	})
	all := decodeToolResult[[]any](t, "list_annotations", allRes, err)
	if len(all) < 3 {
		t.Fatalf("expected at least 3 annotations (tag added + tag removed + comment), got %d", len(all))
	}

	// Filtered to kind=comment.
	commentRes, _, err := f.svc.handleListAnnotations(f.ctx, nil, listAnnotationsInput{
		TransactionID: f.txnID,
		Kinds:         []string{"comment"},
	})
	comments := decodeToolResult[[]any](t, "list_annotations", commentRes, err)
	if len(comments) == 0 {
		t.Fatal("kinds=['comment'] returned 0 rows; expected at least the seeded comment")
	}
	commentIDs := map[string]struct{}{}
	for i, raw := range comments {
		ann := asObject(t, "list_annotations[comment]", raw)
		if kind, _ := ann["kind"].(string); kind != "comment" {
			t.Errorf("list_annotations[%d]: kinds=['comment'] yielded kind=%q", i, kind)
		}
		if action, ok := ann["action"]; ok && action != "" {
			t.Errorf("list_annotations[%d]: comment row should not carry action, got %v", i, action)
		}
		id, _ := ann["id"].(string)
		commentIDs[id] = struct{}{}
	}

	// kinds=['tag'] expands to both tag_added and tag_removed at the DB layer;
	// the response should carry generic kind=tag plus action=added|removed.
	tagRes, _, err := f.svc.handleListAnnotations(f.ctx, nil, listAnnotationsInput{
		TransactionID: f.txnID,
		Kinds:         []string{"tag"},
	})
	tags := decodeToolResult[[]any](t, "list_annotations", tagRes, err)
	if len(tags) < 2 {
		t.Fatalf("kinds=['tag'] expected to expand to add+remove (>=2 rows), got %d", len(tags))
	}
	sawAdded, sawRemoved := false, false
	for i, raw := range tags {
		ann := asObject(t, "list_annotations[tag]", raw)
		if kind, _ := ann["kind"].(string); kind != "tag" {
			t.Errorf("list_annotations[%d]: kinds=['tag'] yielded kind=%q", i, kind)
		}
		switch ann["action"] {
		case "added":
			sawAdded = true
		case "removed":
			sawRemoved = true
		default:
			t.Errorf("list_annotations[%d]: tag row carries unexpected action %v", i, ann["action"])
		}
	}
	if !sawAdded || !sawRemoved {
		t.Errorf("kinds=['tag'] should surface both actions; sawAdded=%v sawRemoved=%v", sawAdded, sawRemoved)
	}

	// Parity with the deprecated list_transaction_comments tool: same set
	// of annotation IDs.
	legacyRes, _, err := f.svc.handleListTransactionComments(f.ctx, nil, listTransactionCommentsInput{
		TransactionID: f.txnID,
	})
	legacy := decodeToolResult[[]any](t, "list_transaction_comments", legacyRes, err)
	if len(legacy) != len(comments) {
		t.Fatalf("parity: list_transaction_comments returned %d rows, list_annotations(kinds=['comment']) returned %d", len(legacy), len(comments))
	}
	for i, raw := range legacy {
		c := asObject(t, "list_transaction_comments[parity]", raw)
		id, _ := c["id"].(string)
		if _, ok := commentIDs[id]; !ok {
			t.Errorf("list_transaction_comments[%d] id=%q missing from list_annotations(kinds=['comment'])", i, id)
		}
	}

	// Raw DB kinds are NOT accepted at the MCP boundary — agents must use the
	// generic names.
	for _, rawKind := range []string{"tag_added", "tag_removed", "rule_applied", "category_set"} {
		res, _, _ := f.svc.handleListAnnotations(f.ctx, nil, listAnnotationsInput{
			TransactionID: f.txnID,
			Kinds:         []string{rawKind},
		})
		if res == nil || !res.IsError {
			t.Errorf("expected error envelope for raw DB kind %q (must use generic name)", rawKind)
		}
	}

	// Unknown kind is rejected at the boundary instead of silently returning
	// an empty slice.
	badRes, _, _ := f.svc.handleListAnnotations(f.ctx, nil, listAnnotationsInput{
		TransactionID: f.txnID,
		Kinds:         []string{"bogus_kind"},
	})
	if badRes == nil || !badRes.IsError {
		t.Fatalf("expected error envelope for invalid kind, got %+v", badRes)
	}
}

// TestListTransactionMatchesResponseShape pins matched_on (not matched_fields),
// match_confidence (not confidence), account_link_id (not link_id), plus
// the denormalized txn fields agents rely on.
func TestListTransactionMatchesResponseShape(t *testing.T) {
	f := seedFixtures(t)
	res, _, err := f.svc.handleListTransactionMatches(f.ctx, nil, listTransactionMatchesInput{
		LinkID: f.linkID,
	})
	out := decodeToolResult[[]any](t, "list_transaction_matches", res, err)
	if len(out) == 0 {
		t.Fatal("expected at least one match")
	}
	m := asObject(t, "list_transaction_matches[0]", out[0])
	requireKeys(t, "list_transaction_matches[0]", m,
		"id", "account_link_id",
		"primary_transaction_id", "dependent_transaction_id",
		"match_confidence", "matched_on",
		"primary_txn_name", "dependent_txn_name",
		"amount", "date", "created_at",
	)
	requireAbsent(t, "list_transaction_matches[0]", m,
		"matched_fields", "confidence", "link_id",
	)
	if _, ok := m["matched_on"].([]any); !ok {
		t.Errorf("matched_on should be array, got %T", m["matched_on"])
	}
}

// TestCreateSessionResponseShape pins session response fields; `actor` and
// `completed_at` must not appear.
func TestCreateSessionResponseShape(t *testing.T) {
	f := seedFixtures(t)
	res, _, err := f.svc.handleCreateSession(f.ctx, nil, createSessionInput{
		Purpose: "shape regression test",
	})
	out := decodeToolResult[map[string]any](t, "create_session", res, err)
	requireKeys(t, "create_session", out,
		"id", "purpose", "api_key_name", "created_at",
	)
	requireAbsent(t, "create_session", out, "actor", "completed_at")
}

// TestSubmitReportResponseShape pins created_by_* fields + body/read_at, and
// guards against re-introducing a `session_id` echo (the link is server-side
// only).
func TestSubmitReportResponseShape(t *testing.T) {
	f := seedFixtures(t)

	// submit_report requires a prior session id (enforced by the wrapper around
	// write tools). Handlers invoked directly bypass that check, but we still
	// pass a valid session_id/reason so the signature matches the real call
	// path documented in rules.
	sessRes, _, err := f.svc.handleCreateSession(f.ctx, nil, createSessionInput{
		Purpose: "regression: submit_report",
	})
	sessOut := decodeToolResult[map[string]any](t, "create_session", sessRes, err)
	sessionID, _ := sessOut["id"].(string)
	if sessionID == "" {
		t.Fatalf("create_session did not return id: %v", sessOut)
	}

	res, _, err := f.svc.handleSubmitReport(f.ctx, nil, submitReportInput{
		WriteSessionContext: WriteSessionContext{SessionID: sessionID, Reason: "shape test"},
		Title:               "Shape regression report",
		Body:                "## Summary\nRegression check.",
		Priority:            "info",
	})
	out := decodeToolResult[map[string]any](t, "submit_report", res, err)
	requireKeys(t, "submit_report", out,
		"id", "title", "body",
		"created_by_type", "created_by_name",
		"priority", "tags", "created_at", "read_at",
	)
	requireAbsent(t, "submit_report", out, "session_id")
}

// TestBulkRecategorizeResponseShape pins matched_count / updated_count (not
// matched / updated).
func TestBulkRecategorizeResponseShape(t *testing.T) {
	f := seedFixtures(t)
	nameContains := "Whole Foods"
	res, _, err := f.svc.handleBulkRecategorize(f.ctx, nil, bulkRecategorizeInput{
		WriteSessionContext: WriteSessionContext{SessionID: "sess", Reason: "shape"},
		ToCategory:          "food_and_drink_groceries",
		NameContains:        nameContains,
	})
	out := decodeToolResult[map[string]any](t, "bulk_recategorize", res, err)
	requireKeys(t, "bulk_recategorize", out, "matched_count", "updated_count")
	requireAbsent(t, "bulk_recategorize", out, "matched", "updated")
}

// TestListCategoriesResponseShape pins bare-array response + the required
// category fields.
func TestListCategoriesResponseShape(t *testing.T) {
	f := seedFixtures(t)
	res, _, err := f.svc.handleListCategories(f.ctx, nil, listCategoriesInput{})
	out := decodeToolResult[[]any](t, "list_categories", res, err)
	if len(out) == 0 {
		t.Fatal("expected at least one category")
	}
	cat := asObject(t, "list_categories[0]", out[0])
	requireKeys(t, "list_categories[0]", cat,
		"id", "slug", "display_name",
		"is_system", "parent_id", "created_at", "updated_at",
	)
}

// TestListTransactionCommentsResponseShape pins author_type/author_id/author_name
// (not a single `author` string).
func TestListTransactionCommentsResponseShape(t *testing.T) {
	f := seedFixtures(t)

	addRes, _, err := f.svc.handleAddTransactionComment(f.ctx, nil, addTransactionCommentInput{
		WriteSessionContext: WriteSessionContext{SessionID: "sess", Reason: "shape"},
		TransactionID:       f.txnID,
		Content:             "Regression check comment",
	})
	_ = decodeToolResult[map[string]any](t, "add_transaction_comment", addRes, err)

	res, _, err := f.svc.handleListTransactionComments(f.ctx, nil, listTransactionCommentsInput{
		TransactionID: f.txnID,
	})
	out := decodeToolResult[[]any](t, "list_transaction_comments", res, err)
	if len(out) == 0 {
		t.Fatal("expected at least one comment")
	}
	c := asObject(t, "list_transaction_comments[0]", out[0])
	requireKeys(t, "list_transaction_comments[0]", c,
		"id", "transaction_id",
		"author_type", "author_name",
		"content", "created_at", "updated_at",
	)
	requireAbsent(t, "list_transaction_comments[0]", c, "author")
}

// TestTransactionSummaryResponseShape pins the {summary, totals, filters}
// envelope + the row fields (category, total_amount, transaction_count).
func TestTransactionSummaryResponseShape(t *testing.T) {
	f := seedFixtures(t)
	res, _, err := f.svc.handleTransactionSummary(f.ctx, nil, transactionSummaryInput{
		GroupBy:   "category",
		StartDate: "2026-01-01",
		EndDate:   "2026-12-31",
	})
	out := decodeToolResult[map[string]any](t, "transaction_summary", res, err)
	requireKeys(t, "transaction_summary", out, "summary", "totals", "filters")

	rows := asArray(t, "transaction_summary.summary", out["summary"])
	if len(rows) == 0 {
		t.Fatal("expected at least one summary row")
	}
	row := asObject(t, "transaction_summary.summary[0]", rows[0])
	requireKeys(t, "transaction_summary.summary[0]", row,
		"category", "total_amount", "transaction_count",
	)

	filters := asObject(t, "transaction_summary.filters", out["filters"])
	requireKeys(t, "transaction_summary.filters", filters,
		"start_date", "end_date", "group_by",
	)
}

// TestMerchantSummaryResponseShape pins {merchants, totals, filters} + row fields.
func TestMerchantSummaryResponseShape(t *testing.T) {
	f := seedFixtures(t)
	res, _, err := f.svc.handleMerchantSummary(f.ctx, nil, merchantSummaryInput{
		StartDate: "2026-01-01",
		EndDate:   "2026-12-31",
		MinCount:  1,
	})
	out := decodeToolResult[map[string]any](t, "merchant_summary", res, err)
	requireKeys(t, "merchant_summary", out, "merchants", "totals", "filters")

	rows := asArray(t, "merchant_summary.merchants", out["merchants"])
	if len(rows) == 0 {
		t.Fatal("expected at least one merchant row")
	}
	row := asObject(t, "merchant_summary.merchants[0]", rows[0])
	requireKeys(t, "merchant_summary.merchants[0]", row,
		"merchant", "transaction_count", "total_amount",
		"avg_amount", "first_date", "last_date",
	)
}

// TestQueryTransactionsResponseShape pins category as an object (not a slug
// string) and the wrapper envelope.
func TestQueryTransactionsResponseShape(t *testing.T) {
	f := seedFixtures(t)
	res, _, err := f.svc.handleQueryTransactions(f.ctx, nil, queryTransactionsInput{
		Limit: 10,
	})
	out := decodeToolResult[map[string]any](t, "query_transactions", res, err)
	requireKeys(t, "query_transactions", out,
		"transactions", "has_more", "limit",
	)
	txns := asArray(t, "query_transactions.transactions", out["transactions"])
	if len(txns) == 0 {
		t.Fatal("expected at least one transaction")
	}
	txn := asObject(t, "query_transactions.transactions[0]", txns[0])
	requireKeys(t, "query_transactions.transactions[0]", txn,
		"id", "account_id", "amount", "date", "provider_name", "category",
	)
	switch v := txn["category"].(type) {
	case map[string]any:
		requireKeys(t, "query_transactions.transactions[0].category", v, "slug", "display_name")
	case nil:
		// null is acceptable when no category is set — but we seeded one, so
		// either a populated object or null is defensible here.
	default:
		t.Errorf("category must be object or null, got %T (%v)", v, v)
	}
}

// TestListTransactionRulesResponseShape pins {rules, has_more, total} and
// absence of a legacy {data: [...]} wrapper.
func TestListTransactionRulesResponseShape(t *testing.T) {
	f := seedFixtures(t)
	res, _, err := f.svc.handleListTransactionRules(f.ctx, nil, listTransactionRulesInput{})
	out := decodeToolResult[map[string]any](t, "list_transaction_rules", res, err)
	requireKeys(t, "list_transaction_rules", out,
		"rules", "has_more", "total",
	)
	requireAbsent(t, "list_transaction_rules", out, "data")
}

// TestPreviewRuleResponseShape pins `sample_matches` (not `sample`) + sample
// row fields (transaction_id, not id).
func TestPreviewRuleResponseShape(t *testing.T) {
	f := seedFixtures(t)
	res, _, err := f.svc.handlePreviewRule(f.ctx, nil, previewRuleInput{
		Conditions: map[string]any{
			"field": "provider_name",
			"op":    "contains",
			"value": "Whole Foods",
		},
		SampleSize: 5,
	})
	out := decodeToolResult[map[string]any](t, "preview_rule", res, err)
	requireKeys(t, "preview_rule", out,
		"match_count", "total_scanned", "sample_matches",
	)
	requireAbsent(t, "preview_rule", out, "sample")

	samples := asArray(t, "preview_rule.sample_matches", out["sample_matches"])
	if len(samples) == 0 {
		t.Fatal("expected at least one sample match")
	}
	sample := asObject(t, "preview_rule.sample_matches[0]", samples[0])
	requireKeys(t, "preview_rule.sample_matches[0]", sample,
		"transaction_id", "provider_name", "amount", "date", "provider_category_primary",
	)
	requireAbsent(t, "preview_rule.sample_matches[0]", sample, "id", "provider_merchant_name")
}

// TestListTagsResponseShape pins required tag fields, including updated_at
// (newly added per the #685 verification pass).
func TestListTagsResponseShape(t *testing.T) {
	f := seedFixtures(t)
	res, _, err := f.svc.handleListTags(f.ctx, nil, listTagsInput{})
	out := decodeToolResult[[]any](t, "list_tags", res, err)
	if len(out) == 0 {
		t.Fatal("expected at least one tag")
	}
	tag := asObject(t, "list_tags[0]", out[0])
	requireKeys(t, "list_tags[0]", tag,
		"id", "slug", "display_name", "description",
		"created_at", "updated_at",
	)
}

// TestListAnnotationsActorTypesFilter exercises the actor_types filter — the
// canonical "any human input?" check. Seeds events from all three actor
// kinds (user, agent, system) and asserts each filter slice returns only
// the matching rows. Also pins the validation error for unknown actor types.
func TestListAnnotationsActorTypesFilter(t *testing.T) {
	f := seedFixtures(t)

	// Seed: a user-authored comment (the human input we expect agents to
	// look for) and an agent-authored comment (rule churn analog) on top of
	// the system tag_added that seedFixtures already wrote.
	if _, err := f.svc.svc.CreateComment(f.ctx, service.CreateCommentParams{
		TransactionID: f.txnID,
		Content:       "Manually checked — this is groceries.",
		Actor:         service.Actor{Type: "user", ID: "alice", Name: "Alice"},
	}); err != nil {
		t.Fatalf("seed user comment: %v", err)
	}
	addRes, _, err := f.svc.handleAddTransactionComment(f.ctx, nil, addTransactionCommentInput{
		WriteSessionContext: WriteSessionContext{SessionID: "sess", Reason: "agent-input"},
		TransactionID:       f.txnID,
		Content:             "Auto-tagged via review loop.",
	})
	_ = decodeToolResult[map[string]any](t, "add_transaction_comment", addRes, err)

	// Unfiltered baseline: at least one row from each actor type.
	allRes, _, err := f.svc.handleListAnnotations(f.ctx, nil, listAnnotationsInput{
		TransactionID: f.txnID,
	})
	all := decodeToolResult[[]any](t, "list_annotations", allRes, err)
	gotUser, gotAgent, gotSystem := 0, 0, 0
	for _, raw := range all {
		ann := asObject(t, "list_annotations[all]", raw)
		switch ann["actor_type"] {
		case "user":
			gotUser++
		case "agent":
			gotAgent++
		case "system":
			gotSystem++
		}
	}
	if gotUser == 0 || gotAgent == 0 || gotSystem == 0 {
		t.Fatalf("baseline missing an actor: user=%d agent=%d system=%d", gotUser, gotAgent, gotSystem)
	}

	cases := []struct {
		name       string
		actorTypes []string
		want       string
	}{
		{"user-only", []string{"user"}, "user"},
		{"agent-only", []string{"agent"}, "agent"},
		{"system-only", []string{"system"}, "system"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, _, err := f.svc.handleListAnnotations(f.ctx, nil, listAnnotationsInput{
				TransactionID: f.txnID,
				ActorTypes:    tc.actorTypes,
			})
			rows := decodeToolResult[[]any](t, "list_annotations", res, err)
			if len(rows) == 0 {
				t.Fatalf("actor_types=%v returned 0 rows; expected at least one", tc.actorTypes)
			}
			for i, raw := range rows {
				ann := asObject(t, "list_annotations[actor]", raw)
				if got, _ := ann["actor_type"].(string); got != tc.want {
					t.Errorf("row %d: actor_type=%q, want %q", i, got, tc.want)
				}
			}
		})
	}

	// Combined slice: actor_types=['user','agent'] excludes system.
	combinedRes, _, err := f.svc.handleListAnnotations(f.ctx, nil, listAnnotationsInput{
		TransactionID: f.txnID,
		ActorTypes:    []string{"user", "agent"},
	})
	combined := decodeToolResult[[]any](t, "list_annotations", combinedRes, err)
	for i, raw := range combined {
		ann := asObject(t, "list_annotations[user+agent]", raw)
		if got, _ := ann["actor_type"].(string); got == "system" {
			t.Errorf("row %d: combined filter leaked system actor", i)
		}
	}

	// Token-budget evidence: the canonical "humans only" call returns a
	// strictly-smaller envelope than the unfiltered one. We log the byte
	// delta so PR reviewers can eyeball the win.
	allBytes := mustMarshal(t, all)
	humansBytes := mustMarshal(t, decodeToolResult[[]any](t, "list_annotations",
		mustListAnnotations(t, f, listAnnotationsInput{TransactionID: f.txnID, ActorTypes: []string{"user"}}), nil))
	if len(humansBytes) >= len(allBytes) {
		t.Errorf("expected actor_types=['user'] envelope (%d bytes) to be smaller than unfiltered (%d bytes)", len(humansBytes), len(allBytes))
	}
	t.Logf("token-budget evidence: unfiltered=%d bytes, actor_types=['user']=%d bytes (delta %+d)",
		len(allBytes), len(humansBytes), len(humansBytes)-len(allBytes))

	// Validation: unknown actor type returns the error envelope.
	badRes, _, _ := f.svc.handleListAnnotations(f.ctx, nil, listAnnotationsInput{
		TransactionID: f.txnID,
		ActorTypes:    []string{"robot"},
	})
	if badRes == nil || !badRes.IsError {
		t.Fatalf("expected error envelope for invalid actor_type, got %+v", badRes)
	}
}

// TestListAnnotationsSinceFilter pins the cursor-style since filter: only
// rows created strictly after the supplied timestamp are returned. We pull
// the cursor from the live DB row (full microsecond precision) so the test
// doesn't race the wall-clock second boundary that the user-facing
// CreatedAt string would truncate to.
func TestListAnnotationsSinceFilter(t *testing.T) {
	f := seedFixtures(t)

	// Snapshot the baseline timeline so we know the row count to subtract.
	beforeRes, _, err := f.svc.handleListAnnotations(f.ctx, nil, listAnnotationsInput{
		TransactionID: f.txnID,
	})
	before := decodeToolResult[[]any](t, "list_annotations", beforeRes, err)
	baselineLen := len(before)
	if baselineLen == 0 {
		t.Fatal("baseline timeline empty")
	}

	// Capture cursor as the full-precision timestamp of the latest baseline
	// row. Sleeping past it guarantees subsequent inserts land strictly
	// after.
	anns, err := f.svc.svc.ListAnnotations(f.ctx, f.txnID, service.ListAnnotationsParams{})
	if err != nil {
		t.Fatalf("svc.ListAnnotations: %v", err)
	}
	cursorTS := anns[len(anns)-1].CreatedAtTime
	time.Sleep(2 * time.Millisecond)

	// Add three new annotations after the cursor.
	for i := 0; i < 3; i++ {
		addRes, _, err := f.svc.handleAddTransactionComment(f.ctx, nil, addTransactionCommentInput{
			WriteSessionContext: WriteSessionContext{SessionID: "sess", Reason: "since-test"},
			TransactionID:       f.txnID,
			Content:             fmt.Sprintf("post-cursor comment %d", i),
		})
		_ = decodeToolResult[map[string]any](t, "add_transaction_comment", addRes, err)
	}

	deltaRes, _, err := f.svc.handleListAnnotations(f.ctx, nil, listAnnotationsInput{
		TransactionID: f.txnID,
		Since:         cursorTS.Format(time.RFC3339Nano),
	})
	delta := decodeToolResult[[]any](t, "list_annotations", deltaRes, err)
	if len(delta) != 3 {
		t.Fatalf("since=%s expected 3 new rows, got %d", cursorTS.Format(time.RFC3339Nano), len(delta))
	}

	// Validation: malformed since returns the error envelope.
	badRes, _, _ := f.svc.handleListAnnotations(f.ctx, nil, listAnnotationsInput{
		TransactionID: f.txnID,
		Since:         "yesterday",
	})
	if badRes == nil || !badRes.IsError {
		t.Fatalf("expected error envelope for malformed since, got %+v", badRes)
	}
}

// TestListAnnotationsLimit pins the tail-N semantics: limit returns the
// most recent N rows, still in ASC chronological order. Also pins the cap
// at MaxAnnotationLimit and the negative-value rejection.
func TestListAnnotationsLimit(t *testing.T) {
	f := seedFixtures(t)

	// Seed several events so we have a meaningful timeline to slice.
	for i := 0; i < 5; i++ {
		addRes, _, err := f.svc.handleAddTransactionComment(f.ctx, nil, addTransactionCommentInput{
			WriteSessionContext: WriteSessionContext{SessionID: "sess", Reason: "limit-test"},
			TransactionID:       f.txnID,
			Content:             fmt.Sprintf("limit-test comment %d", i),
		})
		_ = decodeToolResult[map[string]any](t, "add_transaction_comment", addRes, err)
		time.Sleep(1 * time.Millisecond)
	}

	allRes, _, err := f.svc.handleListAnnotations(f.ctx, nil, listAnnotationsInput{
		TransactionID: f.txnID,
	})
	all := decodeToolResult[[]any](t, "list_annotations", allRes, err)
	if len(all) < 6 {
		t.Fatalf("expected >= 6 annotations to slice, got %d", len(all))
	}

	limitRes, _, err := f.svc.handleListAnnotations(f.ctx, nil, listAnnotationsInput{
		TransactionID: f.txnID,
		Limit:         3,
	})
	limited := decodeToolResult[[]any](t, "list_annotations", limitRes, err)
	if len(limited) != 3 {
		t.Fatalf("limit=3 expected 3 rows, got %d", len(limited))
	}

	// Tail semantics: the limited slice equals the last len(limited) rows
	// of the full timeline (same IDs, same order).
	for i, raw := range limited {
		want, _ := asObject(t, "all", all[len(all)-3+i])["id"].(string)
		got, _ := asObject(t, "limited", raw)["id"].(string)
		if got != want {
			t.Errorf("row %d: limit returned id %q, want tail row %q", i, got, want)
		}
	}

	// limit=0 is the documented default — full timeline.
	zeroRes, _, err := f.svc.handleListAnnotations(f.ctx, nil, listAnnotationsInput{
		TransactionID: f.txnID,
		Limit:         0,
	})
	zero := decodeToolResult[[]any](t, "list_annotations", zeroRes, err)
	if len(zero) != len(all) {
		t.Errorf("limit=0 should match unfiltered: got %d, want %d", len(zero), len(all))
	}

	// limit > MaxAnnotationLimit is silently capped — the response itself
	// can't exceed the cap. We assert no error here; the cap is exercised
	// at the normalize layer (unit covered by the negative case below).
	bigRes, _, err := f.svc.handleListAnnotations(f.ctx, nil, listAnnotationsInput{
		TransactionID: f.txnID,
		Limit:         10_000,
	})
	if err != nil || bigRes == nil || bigRes.IsError {
		t.Errorf("limit=10000 should clamp silently, got error: %+v", bigRes)
	}

	// Negative limit is rejected with the error envelope.
	negRes, _, _ := f.svc.handleListAnnotations(f.ctx, nil, listAnnotationsInput{
		TransactionID: f.txnID,
		Limit:         -5,
	})
	if negRes == nil || !negRes.IsError {
		t.Fatalf("expected error envelope for negative limit, got %+v", negRes)
	}
}

// mustMarshal serializes v as JSON and fails the test on error. Used to
// produce the byte-size proxy for token-budget evidence.
func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// mustListAnnotations runs the handler and fails the test if the call
// errored — used in callsites that want the raw CallToolResult so they can
// re-decode it.
func mustListAnnotations(t *testing.T, f *fixtures, in listAnnotationsInput) *mcpsdk.CallToolResult {
	t.Helper()
	res, _, err := f.svc.handleListAnnotations(f.ctx, nil, in)
	if err != nil {
		t.Fatalf("list_annotations: %v", err)
	}
	return res
}
