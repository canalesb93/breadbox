//go:build integration

package mcp

// Integration tests for the per-entity resource templates introduced in
// stack/mcp-overhaul/04-resource-templates. Templates resolve URIs of the form
// breadbox://<entity>/{short_id} into a {entity, related} envelope agents can
// drill into. The asserts here lock:
//
//   - the request URI is echoed onto Contents[0].URI
//   - MIMEType is application/json (jsonResourceResult contract)
//   - the envelope keys match the documented shape per template
//   - the inner row's id field carries the 8-char short_id (via compactIDsBytes)
//   - related slices are populated for the seeded scenario
//   - missing entities surface the canonical -32002 ResourceNotFound error code
//
// The recent-transactions cap on the account template is exercised by seeding
// 30 transactions and asserting the response is truncated to
// templateRecentTransactionLimit.

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"breadbox/internal/testutil"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
)

// --- Tests ---

// TestTransactionTemplate_HappyPath exercises breadbox://transaction/{short_id}.
// Asserts envelope shape, URI echo, JSON mimetype, the 8-char compact id on
// the transaction row, and that the seeded tag/rule produced annotation rows.
func TestTransactionTemplate_HappyPath(t *testing.T) {
	f := seedFixtures(t)

	// Resolve the seeded transaction's short_id via the service so the URI
	// uses the public 8-char form (the template advertises {short_id}).
	txn, err := f.svc.svc.GetTransaction(f.ctx, f.txnID)
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}
	uri := "breadbox://transaction/" + txn.ShortID

	res, err := f.svc.handleTransactionTemplate(f.ctx, &mcpsdk.ReadResourceRequest{
		Params: &mcpsdk.ReadResourceParams{URI: uri},
	})
	if err != nil {
		t.Fatalf("handleTransactionTemplate: %v", err)
	}
	if len(res.Contents) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(res.Contents))
	}
	c := res.Contents[0]
	if c.URI != uri {
		t.Errorf("URI = %q, want %q", c.URI, uri)
	}
	if c.MIMEType != "application/json" {
		t.Errorf("MIMEType = %q, want application/json", c.MIMEType)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(c.Text), &out); err != nil {
		t.Fatalf("unmarshal: %v\nraw=%s", err, c.Text)
	}
	requireKeys(t, "transaction-template", out, "transaction", "annotations")

	row := asObject(t, "transaction-template.transaction", out["transaction"])
	id, _ := row["id"].(string)
	if len(id) != 8 {
		t.Errorf("transaction.id = %q (len=%d); expected 8-char short", id, len(id))
	}
	requireAbsent(t, "transaction-template.transaction", row, "short_id")

	anns := asArray(t, "transaction-template.annotations", out["annotations"])
	if len(anns) == 0 {
		t.Fatal("expected at least one annotation (seeded tag + rule)")
	}
	for i, raw := range anns {
		ann := asObject(t, fmt.Sprintf("annotations[%d]", i), raw)
		requireKeys(t, fmt.Sprintf("annotations[%d]", i), ann, "kind", "action")
	}
}

// TestAccountTemplate_HappyPath exercises breadbox://account/{short_id}.
// Asserts the envelope, the compact id on the account row, and that the
// seeded primary transaction surfaces in recent_transactions.
func TestAccountTemplate_HappyPath(t *testing.T) {
	f := seedFixtures(t)

	// The fixture seeds the primary account through UpsertAccount. Pull its
	// short_id via the service so we exercise the public template URI shape.
	accts, err := f.svc.svc.ListAccounts(f.ctx, nil)
	if err != nil || len(accts) == 0 {
		t.Fatalf("ListAccounts: err=%v, n=%d", err, len(accts))
	}
	// Pick the primary account (the one with transactions).
	var shortID string
	for _, a := range accts {
		if a.Name == "Primary Credit Card" {
			shortID = a.ShortID
			break
		}
	}
	if shortID == "" {
		t.Fatalf("could not find primary account in %+v", accts)
	}
	uri := "breadbox://account/" + shortID

	res, err := f.svc.handleAccountTemplate(f.ctx, &mcpsdk.ReadResourceRequest{
		Params: &mcpsdk.ReadResourceParams{URI: uri},
	})
	if err != nil {
		t.Fatalf("handleAccountTemplate: %v", err)
	}
	c := res.Contents[0]
	if c.URI != uri {
		t.Errorf("URI = %q, want %q", c.URI, uri)
	}
	if c.MIMEType != "application/json" {
		t.Errorf("MIMEType = %q, want application/json", c.MIMEType)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(c.Text), &out); err != nil {
		t.Fatalf("unmarshal: %v\nraw=%s", err, c.Text)
	}
	requireKeys(t, "account-template", out, "account", "recent_transactions")

	row := asObject(t, "account-template.account", out["account"])
	id, _ := row["id"].(string)
	if len(id) != 8 {
		t.Errorf("account.id = %q (len=%d); expected 8-char short", id, len(id))
	}
	requireAbsent(t, "account-template.account", row, "short_id")

	recent := asArray(t, "account-template.recent_transactions", out["recent_transactions"])
	if len(recent) == 0 {
		t.Fatal("expected at least one recent transaction (seeded primary txn)")
	}
}

// TestUserTemplate_HappyPath exercises breadbox://user/{short_id}. Asserts the
// envelope and that the user's accounts are returned.
func TestUserTemplate_HappyPath(t *testing.T) {
	f := seedFixtures(t)

	users, err := f.svc.svc.ListUsers(f.ctx)
	if err != nil || len(users) == 0 {
		t.Fatalf("ListUsers: err=%v, n=%d", err, len(users))
	}
	shortID := users[0].ShortID
	uri := "breadbox://user/" + shortID

	res, err := f.svc.handleUserTemplate(f.ctx, &mcpsdk.ReadResourceRequest{
		Params: &mcpsdk.ReadResourceParams{URI: uri},
	})
	if err != nil {
		t.Fatalf("handleUserTemplate: %v", err)
	}
	c := res.Contents[0]
	if c.URI != uri {
		t.Errorf("URI = %q, want %q", c.URI, uri)
	}
	if c.MIMEType != "application/json" {
		t.Errorf("MIMEType = %q, want application/json", c.MIMEType)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(c.Text), &out); err != nil {
		t.Fatalf("unmarshal: %v\nraw=%s", err, c.Text)
	}
	requireKeys(t, "user-template", out, "user", "accounts")

	row := asObject(t, "user-template.user", out["user"])
	id, _ := row["id"].(string)
	if len(id) != 8 {
		t.Errorf("user.id = %q (len=%d); expected 8-char short", id, len(id))
	}
	requireAbsent(t, "user-template.user", row, "short_id")

	accts := asArray(t, "user-template.accounts", out["accounts"])
	if len(accts) == 0 {
		t.Fatal("expected at least one account for the seeded user")
	}
}

// TestTransactionTemplate_NotFound asserts that an unknown short_id surfaces
// the canonical MCP -32002 ResourceNotFound code so clients can branch on it.
func TestTransactionTemplate_NotFound(t *testing.T) {
	f := seedFixtures(t)

	// Syntactically valid 8-char base62, but no row exists.
	uri := "breadbox://transaction/zzzzzzzz"
	_, err := f.svc.handleTransactionTemplate(f.ctx, &mcpsdk.ReadResourceRequest{
		Params: &mcpsdk.ReadResourceParams{URI: uri},
	})
	if err == nil {
		t.Fatal("expected ResourceNotFound error, got nil")
	}
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) {
		t.Fatalf("expected *jsonrpc.Error, got %T: %v", err, err)
	}
	if rpcErr.Code != mcpsdk.CodeResourceNotFound {
		t.Errorf("error code = %d, want %d (CodeResourceNotFound / -32002)",
			rpcErr.Code, mcpsdk.CodeResourceNotFound)
	}
}

// TestAccountTemplate_RecentTransactionsCap pins the cap at
// templateRecentTransactionLimit. We seed (cap + 5) transactions on the
// primary account and assert the response is truncated to exactly the cap.
func TestAccountTemplate_RecentTransactionsCap(t *testing.T) {
	f := seedFixtures(t)

	// Reuse the queries handle attached to the fixture service. Calling
	// testutil.ServicePool / testutil.Pool here would re-truncate the tables
	// seeded by seedFixtures and wipe our primary account out from under us.
	q := f.svc.svc.Queries
	// Resolve the primary account back to its UUID — testutil.MustCreateTransaction
	// takes a pgtype.UUID, and the fixture didn't expose the account ID.
	accts, err := f.svc.svc.ListAccounts(f.ctx, nil)
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	var primaryShort string
	for _, a := range accts {
		if a.Name == "Primary Credit Card" {
			primaryShort = a.ShortID
			break
		}
	}
	if primaryShort == "" {
		t.Fatal("primary account not found")
	}

	// Pull the row directly so we can grab the pgtype.UUID for the helper.
	primaryUUID, err := q.GetAccountUUIDByShortID(f.ctx, primaryShort)
	if err != nil {
		t.Fatalf("GetAccountUUIDByShortID: %v", err)
	}

	// Seed templateRecentTransactionLimit + 5 extra transactions.
	extra := templateRecentTransactionLimit + 5
	for i := 0; i < extra; i++ {
		testutil.MustCreateTransaction(t, q, primaryUUID,
			fmt.Sprintf("txn_cap_%d", i),
			fmt.Sprintf("Cap Test %d", i),
			int64(100+i),
			"2026-04-15",
		)
	}

	uri := "breadbox://account/" + primaryShort
	res, err := f.svc.handleAccountTemplate(f.ctx, &mcpsdk.ReadResourceRequest{
		Params: &mcpsdk.ReadResourceParams{URI: uri},
	})
	if err != nil {
		t.Fatalf("handleAccountTemplate: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(res.Contents[0].Text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	recent := asArray(t, "recent_transactions", out["recent_transactions"])
	if len(recent) != templateRecentTransactionLimit {
		t.Errorf("recent_transactions length = %d, want %d",
			len(recent), templateRecentTransactionLimit)
	}
}
