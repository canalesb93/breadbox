package mcp

import (
	"context"

	"breadbox/internal/service"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- Input types ---

type listAccountLinksInput struct {
	ReadSessionContext
}

type createAccountLinkInput struct {
	WriteSessionContext
	PrimaryAccountID   string `json:"primary_account_id" jsonschema:"The primary cardholder's account ID"`
	DependentAccountID string `json:"dependent_account_id" jsonschema:"The authorized/dependent user's account ID"`
	MatchStrategy      string `json:"match_strategy,omitempty" jsonschema:"Matching strategy: date_amount_name (default)"`
	MatchToleranceDays int    `json:"match_tolerance_days,omitempty" jsonschema:"Date tolerance in days for matching (default 0 = same day)"`
}

type deleteAccountLinkInput struct {
	WriteSessionContext
	LinkID string `json:"link_id" jsonschema:"The account link ID to delete"`
}

type reconcileAccountLinkInput struct {
	WriteSessionContext
	LinkID string `json:"link_id" jsonschema:"The account link ID to reconcile"`
}

type listTransactionMatchesInput struct {
	ReadSessionContext
	LinkID string `json:"link_id" jsonschema:"The account link ID to list matches for"`
}

type confirmMatchInput struct {
	WriteSessionContext
	MatchID string `json:"match_id" jsonschema:"The transaction match ID to confirm"`
}

type rejectMatchInput struct {
	WriteSessionContext
	MatchID string `json:"match_id" jsonschema:"The transaction match ID to reject"`
}

type pendingReviewsOverviewInput struct {
	ReadSessionContext
}

// --- Handlers ---

func (s *MCPServer) handleListAccountLinks(_ context.Context, _ *mcpsdk.CallToolRequest, _ listAccountLinksInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
	links, err := s.svc.ListAccountLinks(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(links)
}

func (s *MCPServer) handleCreateAccountLink(ctx context.Context, _ *mcpsdk.CallToolRequest, input createAccountLinkInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	ctx = context.Background()
	link, err := s.svc.CreateAccountLink(ctx, service.CreateAccountLinkParams{
		PrimaryAccountID:   input.PrimaryAccountID,
		DependentAccountID: input.DependentAccountID,
		MatchStrategy:      input.MatchStrategy,
		MatchToleranceDays: input.MatchToleranceDays,
	})
	if err != nil {
		return errorResult(err), nil, nil
	}

	// Auto-run initial reconciliation after creating a link.
	result, reconcileErr := s.svc.RunMatchReconciliation(ctx, link.ID)
	if reconcileErr != nil {
		// Non-fatal — return the link anyway.
		return jsonResult(map[string]any{
			"link":                link,
			"reconciliation_note": "Link created but initial reconciliation failed: " + reconcileErr.Error(),
		})
	}

	return jsonResult(map[string]any{
		"link":           link,
		"reconciliation": result,
	})
}

func (s *MCPServer) handleDeleteAccountLink(ctx context.Context, _ *mcpsdk.CallToolRequest, input deleteAccountLinkInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	ctx = context.Background()
	if err := s.svc.DeleteAccountLink(ctx, input.LinkID); err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(map[string]string{"status": "deleted"})
}

func (s *MCPServer) handleReconcileAccountLink(ctx context.Context, _ *mcpsdk.CallToolRequest, input reconcileAccountLinkInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	ctx = context.Background()
	result, err := s.svc.RunMatchReconciliation(ctx, input.LinkID)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result)
}

func (s *MCPServer) handleListTransactionMatches(_ context.Context, _ *mcpsdk.CallToolRequest, input listTransactionMatchesInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
	matches, err := s.svc.ListTransactionMatches(ctx, input.LinkID)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(matches)
}

func (s *MCPServer) handleConfirmMatch(ctx context.Context, _ *mcpsdk.CallToolRequest, input confirmMatchInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	ctx = context.Background()
	if err := s.svc.ConfirmMatch(ctx, input.MatchID); err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(map[string]string{"status": "confirmed"})
}

func (s *MCPServer) handleRejectMatch(ctx context.Context, _ *mcpsdk.CallToolRequest, input rejectMatchInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	ctx = context.Background()
	if err := s.svc.RejectMatch(ctx, input.MatchID); err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(map[string]string{"status": "rejected"})
}

func (s *MCPServer) handlePendingReviewsOverview(_ context.Context, _ *mcpsdk.CallToolRequest, _ pendingReviewsOverviewInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()

	if !s.svc.IsReviewsEnabled(ctx) {
		return jsonResult(map[string]any{
			"total_pending":   0,
			"counts_by_type":  []any{},
			"category_groups": []any{},
			"note":            "Transaction reviews are currently disabled. Enable them in the admin dashboard at /reviews.",
		})
	}

	result, err := s.svc.GetPendingReviewsOverview(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result)
}
