//go:build integration && !lite

package mcp

// MCP-layer assertion for the reviewer-authors-rule loop (P5-PR2 / T5).
//
// Companion to the service-layer rules_reviewer_e2e_integration_test.go: this
// proves the same loop survives the MCP tool boundary. An agent calls
// create_transaction_rule with an assign_series action and apply_retroactively,
// and we lock the response envelope (rule + retroactive_matches) plus the
// side-effect (a series is minted and members link). Asserts are strict on
// shape, loose on values — mirrors flag_tool_response_shape_integration_test.go.

import (
	"testing"

	"breadbox/internal/service"
	"breadbox/internal/testutil"
)

// reviewerFreshTxn creates a fresh Netflix charge on the seeded primary account
// and returns its UUID string.
func reviewerFreshTxn(t *testing.T, f *fixtures, extID, date string, cents int64) string {
	t.Helper()
	q := f.svc.svc.Queries
	accts, err := f.svc.svc.ListAccounts(f.ctx, nil)
	if err != nil || len(accts) == 0 {
		t.Fatalf("reviewerFreshTxn: ListAccounts: err=%v n=%d", err, len(accts))
	}
	var primaryShort string
	for _, a := range accts {
		if a.Name == "Primary Credit Card" {
			primaryShort = a.ShortID
			break
		}
	}
	if primaryShort == "" {
		t.Fatal("reviewerFreshTxn: could not find Primary Credit Card in accounts")
	}
	primaryUUID, err := q.GetAccountUUIDByShortID(f.ctx, primaryShort)
	if err != nil {
		t.Fatalf("reviewerFreshTxn: GetAccountUUIDByShortID: %v", err)
	}
	txn := testutil.MustCreateTransaction(t, q, primaryUUID, extID, "Netflix", cents, date)
	return formatUUIDTest(t, txn.ID)
}

// TestReviewerCreateRuleAssignSeriesResponseShape pins the create_transaction_rule
// envelope when an agent authors an assign_series rule and applies it
// retroactively in one call: required keys `rule` and `retroactive_matches`
// must be present, and the side-effect must mint a Netflix series with linked
// members.
func TestReviewerCreateRuleAssignSeriesResponseShape(t *testing.T) {
	f := seedFixtures(t)
	reviewerFreshTxn(t, f, "txn_reviewer_nflx_1", "2026-01-15", 1549)
	reviewerFreshTxn(t, f, "txn_reviewer_nflx_2", "2026-02-16", 1499)

	res, _, err := f.svc.handleCreateTransactionRule(f.ctx, nil, createTransactionRuleInput{
		Rules: []ruleSpecInput{{
			Name: "Netflix → series",
			Conditions: map[string]any{
				"field": "provider_name",
				"op":    "contains",
				"value": "netflix",
			},
			Actions: []map[string]any{{
				"type":              "assign_series",
				"series_name":       "Netflix",
				"create_if_missing": true,
			}},
			ApplyRetroactively: true,
		}},
	})
	out := decodeToolResult[map[string]any](t, "reviewer:create_transaction_rule", res, err)

	// create_transaction_rule returns a batch envelope: {created, failed, rules:[{rule, retroactive_matches}], errors}.
	requireKeys(t, "reviewer:create_transaction_rule", out, "created", "rules")
	rulesArr, ok := out["rules"].([]any)
	if !ok || len(rulesArr) == 0 {
		t.Fatalf("reviewer:create_transaction_rule: rules is %T (len %d), want non-empty array", out["rules"], len(rulesArr))
	}
	entry, ok := rulesArr[0].(map[string]any)
	if !ok {
		t.Fatalf("reviewer:create_transaction_rule: rules[0] is %T, want object", rulesArr[0])
	}
	requireKeys(t, "reviewer:create_transaction_rule.rules[0]", entry, "rule", "retroactive_matches")

	rule, ok := entry["rule"].(map[string]any)
	if !ok {
		t.Fatalf("reviewer:create_transaction_rule: rule is %T, want object", entry["rule"])
	}
	requireKeys(t, "reviewer:create_transaction_rule.rule", rule, "actions", "created_by_type")

	// Side-effect: a Netflix series exists after retroactive apply.
	all, err := f.svc.svc.ListSeries(f.ctx, nil)
	if err != nil {
		t.Fatalf("ListSeries: %v", err)
	}
	var found *service.SeriesResponse
	for i := range all {
		if all[i].Name == "Netflix" {
			found = &all[i]
			break
		}
	}
	if found == nil {
		t.Fatal("reviewer loop via MCP did not mint a Netflix series")
	}
	if matches, _ := entry["retroactive_matches"].(float64); matches < 2 {
		t.Errorf("retroactive_matches = %v, want >= 2", entry["retroactive_matches"])
	}
}
