//go:build !lite

package mcp

import (
	"context"
	"errors"
	"fmt"

	"breadbox/internal/service"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type listSeriesInput struct {
	Status string `json:"status,omitempty" jsonschema:"Optional status filter: active, candidate, paused, or cancelled."`
}

type getSeriesInput struct {
	ID string `json:"id" jsonschema:"required,Series short ID or UUID."`
}

type reviewSeriesInput struct {
	ID      string `json:"id" jsonschema:"required,Series short ID or UUID."`
	Verdict string `json:"verdict" jsonschema:"required,One of: confirm (it is a subscription), reject (not a subscription — sticky), pause, cancel."`
}

type assignSeriesInput struct {
	SeriesID        string   `json:"series_id,omitempty" jsonschema:"Existing series short ID or UUID to assign transactions to. Provide this OR (merchant_key + create_if_missing)."`
	MerchantKey     string   `json:"merchant_key,omitempty" jsonschema:"Normalized merchant key to mint a new series under (requires create_if_missing). Use the exact, specific key — e.g. 'netflix', not 'payment'."`
	CreateIfMissing bool     `json:"create_if_missing,omitempty" jsonschema:"Mint a new series keyed on merchant_key when no series_id is given."`
	Name            string   `json:"name,omitempty" jsonschema:"Optional display name for a minted series."`
	Cadence         string   `json:"cadence,omitempty" jsonschema:"Optional cadence for a minted series: weekly, biweekly, monthly, quarterly, semiannual, annual."`
	ExpectedAmount  *float64 `json:"expected_amount,omitempty" jsonschema:"Optional expected charge amount in dollars, paired with currency."`
	Currency        string   `json:"currency,omitempty" jsonschema:"ISO currency code for expected_amount (e.g. USD)."`
	CategoryID      string   `json:"category_id,omitempty" jsonschema:"Optional suggested category short ID or UUID."`
	UserID          string   `json:"user_id,omitempty" jsonschema:"Optional household member short ID or UUID; omit for a shared/household series."`
	TransactionIDs  []string `json:"transaction_ids,omitempty" jsonschema:"Transactions (short ID or UUID) to link to the series. Max 50 per call."`
	Confirm         bool     `json:"confirm,omitempty" jsonschema:"If true, immediately confirm the series (active) instead of leaving it as a reviewable candidate. Use when asserting a real subscription on the user's behalf."`
}

func (s *MCPServer) handleListSeries(_ context.Context, _ *mcpsdk.CallToolRequest, input listSeriesInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
	var status *string
	if input.Status != "" {
		status = &input.Status
	}
	series, err := s.svc.ListSeries(ctx, status)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(map[string]any{"series": series})
}

type explainSeriesInput struct{}

func (s *MCPServer) handleExplainSeriesCandidates(_ context.Context, _ *mcpsdk.CallToolRequest, _ explainSeriesInput) (*mcpsdk.CallToolResult, any, error) {
	nearMisses, err := s.svc.ExplainSeriesCandidates(context.Background())
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(map[string]any{"near_misses": nearMisses})
}

func (s *MCPServer) handleGetSeries(_ context.Context, _ *mcpsdk.CallToolRequest, input getSeriesInput) (*mcpsdk.CallToolResult, any, error) {
	if input.ID == "" {
		return errorResult(fmt.Errorf("id is required")), nil, nil
	}
	series, err := s.svc.GetSeries(context.Background(), input.ID)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			return errorResult(fmt.Errorf("series not found")), nil, nil
		}
		return errorResult(err), nil, nil
	}
	return jsonResult(series)
}

func (s *MCPServer) handleReviewSeries(ctx context.Context, _ *mcpsdk.CallToolRequest, input reviewSeriesInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.ID == "" {
		return errorResult(fmt.Errorf("id is required")), nil, nil
	}
	verdict := service.SeriesVerdict(input.Verdict)
	switch verdict {
	case service.VerdictConfirm, service.VerdictReject, service.VerdictPause, service.VerdictCancel:
	default:
		return errorResult(fmt.Errorf("verdict must be one of: confirm, reject, pause, cancel")), nil, nil
	}
	actor := service.ActorFromContext(ctx)
	series, err := s.svc.ReviewSeries(context.Background(), input.ID, verdict, actor)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			return errorResult(fmt.Errorf("series not found")), nil, nil
		}
		return errorResult(err), nil, nil
	}
	return jsonResult(series)
}

type seriesTagInput struct {
	ID      string `json:"id" jsonschema:"required,Series short ID or UUID."`
	TagSlug string `json:"tag_slug" jsonschema:"required,Tag slug (must already exist)."`
}

func (s *MCPServer) handleAddSeriesTag(ctx context.Context, _ *mcpsdk.CallToolRequest, input seriesTagInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.ID == "" || input.TagSlug == "" {
		return errorResult(fmt.Errorf("id and tag_slug are required")), nil, nil
	}
	actor := service.ActorFromContext(ctx)
	if err := s.svc.AddSeriesTag(context.Background(), input.ID, input.TagSlug, actor); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			return errorResult(fmt.Errorf("series not found")), nil, nil
		}
		return errorResult(err), nil, nil
	}
	series, err := s.svc.GetSeries(context.Background(), input.ID)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(series)
}

func (s *MCPServer) handleRemoveSeriesTag(ctx context.Context, _ *mcpsdk.CallToolRequest, input seriesTagInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.ID == "" || input.TagSlug == "" {
		return errorResult(fmt.Errorf("id and tag_slug are required")), nil, nil
	}
	if err := s.svc.RemoveSeriesTag(context.Background(), input.ID, input.TagSlug); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			return errorResult(fmt.Errorf("series not found")), nil, nil
		}
		return errorResult(err), nil, nil
	}
	series, err := s.svc.GetSeries(context.Background(), input.ID)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(series)
}

func (s *MCPServer) handleAssignSeries(ctx context.Context, _ *mcpsdk.CallToolRequest, input assignSeriesInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	actor := service.ActorFromContext(ctx)
	opt := func(v string) *string {
		if v == "" {
			return nil
		}
		return &v
	}
	series, err := s.svc.AssignSeries(context.Background(), service.AssignSeriesInput{
		SeriesID:        opt(input.SeriesID),
		MerchantKey:     input.MerchantKey,
		CreateIfMissing: input.CreateIfMissing,
		Name:            input.Name,
		Cadence:         input.Cadence,
		ExpectedAmount:  input.ExpectedAmount,
		Currency:        opt(input.Currency),
		CategoryID:      opt(input.CategoryID),
		UserID:          opt(input.UserID),
		TransactionIDs:  input.TransactionIDs,
		Confirm:         input.Confirm,
	}, actor)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			return errorResult(fmt.Errorf("series not found")), nil, nil
		}
		return errorResult(err), nil, nil
	}
	return jsonResult(series)
}
