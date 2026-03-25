package mcp

import (
	"context"

	"breadbox/internal/service"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- Input types ---

type listAccountLinksInput struct{}

type createAccountLinkInput struct {
	PrimaryAccountID   string `json:"primary_account_id" jsonschema:"The primary cardholder's account ID"`
	DependentAccountID string `json:"dependent_account_id" jsonschema:"The authorized/dependent user's account ID"`
	MatchStrategy      string `json:"match_strategy,omitempty" jsonschema:"Matching strategy: date_amount_name (default)"`
	MatchToleranceDays int    `json:"match_tolerance_days,omitempty" jsonschema:"Date tolerance in days for matching (default 0 = same day)"`
}

type deleteAccountLinkInput struct {
	LinkID string `json:"link_id" jsonschema:"The account link ID to delete"`
}

type reconcileAccountLinkInput struct {
	LinkID string `json:"link_id" jsonschema:"The account link ID to reconcile"`
}

type listTransactionMatchesInput struct {
	LinkID string `json:"link_id" jsonschema:"The account link ID to list matches for"`
}

type confirmMatchInput struct {
	MatchID string `json:"match_id" jsonschema:"The transaction match ID to confirm"`
}

type rejectMatchInput struct {
	MatchID string `json:"match_id" jsonschema:"The transaction match ID to reject"`
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

func (s *MCPServer) handleCreateAccountLink(_ context.Context, _ *mcpsdk.CallToolRequest, input createAccountLinkInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
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

func (s *MCPServer) handleDeleteAccountLink(_ context.Context, _ *mcpsdk.CallToolRequest, input deleteAccountLinkInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
	if err := s.svc.DeleteAccountLink(ctx, input.LinkID); err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(map[string]string{"status": "deleted"})
}

func (s *MCPServer) handleReconcileAccountLink(_ context.Context, _ *mcpsdk.CallToolRequest, input reconcileAccountLinkInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
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

func (s *MCPServer) handleConfirmMatch(_ context.Context, _ *mcpsdk.CallToolRequest, input confirmMatchInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
	if err := s.svc.ConfirmMatch(ctx, input.MatchID); err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(map[string]string{"status": "confirmed"})
}

func (s *MCPServer) handleRejectMatch(_ context.Context, _ *mcpsdk.CallToolRequest, input rejectMatchInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
	if err := s.svc.RejectMatch(ctx, input.MatchID); err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(map[string]string{"status": "rejected"})
}
