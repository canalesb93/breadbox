//go:build !lite

package mcp

import (
	"context"
	"errors"
	"fmt"

	"breadbox/internal/service"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type listCounterpartiesInput struct{}

type getCounterpartyInput struct {
	ID string `json:"id" jsonschema:"required,Counterparty short ID or UUID."`
}

type createCounterpartyInput struct {
	Name       string `json:"name" jsonschema:"required,Display name for the counterparty (the canonical, cross-provider 'other side' of a charge — a merchant, person, or employer). De-duped on the live name, but not unique."`
	WebsiteURL string `json:"website_url,omitempty" jsonschema:"Optional canonical website URL."`
	LogoURL    string `json:"logo_url,omitempty" jsonschema:"Optional logo image URL."`
	CategoryID string `json:"category_id,omitempty" jsonschema:"Optional default category (slug or short ID/UUID) for this counterparty's charges."`
	MCC        string `json:"mcc,omitempty" jsonschema:"Optional 4-digit merchant category code."`
}

type updateCounterpartyInput struct {
	ID         string  `json:"id" jsonschema:"required,Counterparty short ID or UUID to enrich."`
	Name       *string `json:"name,omitempty" jsonschema:"New display name. Omit to leave unchanged; empty string is rejected."`
	WebsiteURL *string `json:"website_url,omitempty" jsonschema:"Canonical website URL. Omit to leave unchanged."`
	LogoURL    *string `json:"logo_url,omitempty" jsonschema:"Logo image URL. Omit to leave unchanged."`
	CategoryID *string `json:"category_id,omitempty" jsonschema:"Default category (slug or short ID/UUID). Omit to leave unchanged."`
	MCC        *string `json:"mcc,omitempty" jsonschema:"4-digit merchant category code. Omit to leave unchanged."`
}

type assignCounterpartyInput struct {
	CounterpartyID  string   `json:"counterparty_id,omitempty" jsonschema:"Existing counterparty short ID or UUID to bind transactions to. Provide this OR (name + create_if_missing)."`
	Name            string   `json:"name,omitempty" jsonschema:"Name to resolve-or-create a counterparty under (requires create_if_missing). Surrogate-first: pick a clean canonical label — e.g. 'Amazon'."`
	CreateIfMissing bool     `json:"create_if_missing,omitempty" jsonschema:"Resolve (or create) a counterparty by name when no counterparty_id is given."`
	TransactionIDs  []string `json:"transaction_ids,omitempty" jsonschema:"Transactions (short ID or UUID) to link to the counterparty. Max 50 per call. NULL-fill only — never steals a charge already bound elsewhere."`
}

type unlinkCounterpartyTransactionInput struct {
	ID             string   `json:"id" jsonschema:"required,Counterparty short ID or UUID."`
	TransactionIDs []string `json:"transaction_ids" jsonschema:"required,Transactions (short ID or UUID) to detach from the counterparty. Each must currently belong to it. Max 50."`
}

func (s *MCPServer) handleListCounterparties(_ context.Context, _ *mcpsdk.CallToolRequest, _ listCounterpartiesInput) (*mcpsdk.CallToolResult, any, error) {
	cps, err := s.svc.ListCounterparties(context.Background())
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(map[string]any{"counterparties": cps})
}

func (s *MCPServer) handleGetCounterparty(_ context.Context, _ *mcpsdk.CallToolRequest, input getCounterpartyInput) (*mcpsdk.CallToolResult, any, error) {
	if input.ID == "" {
		return errorResult(fmt.Errorf("id is required")), nil, nil
	}
	cp, err := s.svc.GetCounterparty(context.Background(), input.ID)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			return errorResult(fmt.Errorf("counterparty not found")), nil, nil
		}
		return errorResult(err), nil, nil
	}
	return jsonResult(cp)
}

func (s *MCPServer) handleCreateCounterparty(ctx context.Context, _ *mcpsdk.CallToolRequest, input createCounterpartyInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.Name == "" {
		return errorResult(fmt.Errorf("name is required")), nil, nil
	}
	actor := service.ActorFromContext(ctx)
	// Strict create (FailIfExists) — an explicit "make a new counterparty" intent
	// should surface a conflict rather than silently resolve an existing row.
	cp, err := s.svc.AssignCounterparty(context.Background(), service.AssignCounterpartyInput{
		Name:            input.Name,
		CreateIfMissing: true,
		FailIfExists:    true,
	}, actor)
	if err != nil {
		return errorResult(err), nil, nil
	}
	// Apply any enrichment fields supplied at create time.
	if input.WebsiteURL != "" || input.LogoURL != "" || input.CategoryID != "" || input.MCC != "" {
		edit := service.EditCounterpartyInput{}
		if input.WebsiteURL != "" {
			edit.WebsiteURL = &input.WebsiteURL
		}
		if input.LogoURL != "" {
			edit.LogoURL = &input.LogoURL
		}
		if input.CategoryID != "" {
			edit.CategoryID = &input.CategoryID
		}
		if input.MCC != "" {
			edit.MCC = &input.MCC
		}
		enriched, err := s.svc.UpdateCounterparty(context.Background(), cp.ShortID, edit, actor)
		if err != nil {
			return errorResult(err), nil, nil
		}
		cp = enriched
	}
	return jsonResult(cp)
}

func (s *MCPServer) handleUpdateCounterparty(ctx context.Context, _ *mcpsdk.CallToolRequest, input updateCounterpartyInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.ID == "" {
		return errorResult(fmt.Errorf("id is required")), nil, nil
	}
	edit := service.EditCounterpartyInput{
		Name:       input.Name,
		WebsiteURL: input.WebsiteURL,
		LogoURL:    input.LogoURL,
		CategoryID: input.CategoryID,
		MCC:        input.MCC,
	}
	actor := service.ActorFromContext(ctx)
	cp, err := s.svc.UpdateCounterparty(context.Background(), input.ID, edit, actor)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			return errorResult(fmt.Errorf("counterparty not found")), nil, nil
		}
		return errorResult(err), nil, nil
	}
	return jsonResult(cp)
}

func (s *MCPServer) handleAssignCounterparty(ctx context.Context, _ *mcpsdk.CallToolRequest, input assignCounterpartyInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	actor := service.ActorFromContext(ctx)
	cp, err := s.svc.AssignCounterparty(context.Background(), service.AssignCounterpartyInput{
		CounterpartyShortID: optStr(input.CounterpartyID),
		Name:                input.Name,
		CreateIfMissing:     input.CreateIfMissing,
		TransactionIDs:      input.TransactionIDs,
	}, actor)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			return errorResult(fmt.Errorf("counterparty not found")), nil, nil
		}
		return errorResult(err), nil, nil
	}
	return jsonResult(cp)
}

func (s *MCPServer) handleUnlinkCounterpartyTransaction(ctx context.Context, _ *mcpsdk.CallToolRequest, input unlinkCounterpartyTransactionInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.ID == "" || len(input.TransactionIDs) == 0 {
		return errorResult(fmt.Errorf("id and transaction_ids are required")), nil, nil
	}
	actor := service.ActorFromContext(ctx)
	cp, err := s.svc.UnlinkCounterpartyTransactions(context.Background(), input.ID, input.TransactionIDs, actor)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			return errorResult(fmt.Errorf("counterparty not found")), nil, nil
		}
		return errorResult(err), nil, nil
	}
	return jsonResult(cp)
}
