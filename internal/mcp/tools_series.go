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
	Fields string `json:"fields,omitempty" jsonschema:"Comma-separated fields to include, to cut response size. Aliases: minimal (name,status,type,cadence), overview (identity + renewal prediction; the default — omits the verbose detection_signals blob). Default when omitted: overview. Pass fields=all for every field including detection_signals. Use get_series for a single series' full detail. id is always included."`
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
	Type            string   `json:"type,omitempty" jsonschema:"Optional recurring-charge type for a minted series: subscription, bill, loan, or other. Omit to infer from the linked charges' category."`
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
	// Lean-by-default: list_series returns the overview projection (identity +
	// renewal prediction) unless the caller asks for more. The verbose
	// detection_signals blob is excluded by default — fetch a single series'
	// full detail via get_series, or pass fields=all here.
	fieldsRaw := input.Fields
	switch fieldsRaw {
	case "":
		fieldsRaw = service.DefaultSeriesFields
	case "all":
		fieldsRaw = "" // ParseSeriesFields("") → nil → full struct
	}
	fieldSet, err := service.ParseSeriesFields(fieldsRaw)
	if err != nil {
		return errorResult(err), nil, nil
	}

	series, err := s.svc.ListSeries(ctx, status)
	if err != nil {
		return errorResult(err), nil, nil
	}

	if fieldSet != nil {
		projected := make([]map[string]any, len(series))
		for i, sr := range series {
			projected[i] = service.FilterSeriesFields(sr, fieldSet)
		}
		return jsonResult(map[string]any{"series": projected})
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

type updateSeriesInput struct {
	ID              string   `json:"id" jsonschema:"required,Series short ID or UUID to edit."`
	Name            string   `json:"name,omitempty" jsonschema:"New display name. Omit to leave unchanged."`
	ExpectedAmount  *float64 `json:"expected_amount,omitempty" jsonschema:"New expected charge amount in dollars (pair with currency). Omit to leave unchanged."`
	AmountTolerance *float64 `json:"amount_tolerance,omitempty" jsonschema:"New ± match tolerance in dollars. Omit to leave unchanged."`
	Currency        string   `json:"currency,omitempty" jsonschema:"New ISO currency code for the amount (e.g. USD). Part of the dedup signature — a change is collision-guarded. Omit to leave unchanged."`
	Cadence         string   `json:"cadence,omitempty" jsonschema:"New cadence: weekly, biweekly, monthly, quarterly, semiannual, annual, irregular, unknown. Re-derives the next expected date. Omit to leave unchanged."`
	ExpectedDay     *int32   `json:"expected_day,omitempty" jsonschema:"New day-of-month / day-of-week anchor (1–31). Send 0 to clear the anchor; omit to leave unchanged."`
	CategoryID      string   `json:"category_id,omitempty" jsonschema:"New suggested category short ID or UUID. Omit to leave unchanged."`
	UserID          string   `json:"user_id,omitempty" jsonschema:"New household-member owner short ID or UUID. Part of the dedup signature — a change is collision-guarded. Omit to leave unchanged."`
}

func (s *MCPServer) handleUpdateSeries(ctx context.Context, _ *mcpsdk.CallToolRequest, input updateSeriesInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.ID == "" {
		return errorResult(fmt.Errorf("id is required")), nil, nil
	}
	edit := service.EditSeriesInput{
		Name:            optStr(input.Name),
		ExpectedAmount:  input.ExpectedAmount,
		AmountTolerance: input.AmountTolerance,
		Currency:        optStr(input.Currency),
		Cadence:         optStr(input.Cadence),
		ExpectedDay:     input.ExpectedDay,
		CategoryID:      optStr(input.CategoryID),
		UserID:          optStr(input.UserID),
	}
	if edit.Name == nil && edit.ExpectedAmount == nil && edit.AmountTolerance == nil &&
		edit.Currency == nil && edit.Cadence == nil && edit.ExpectedDay == nil &&
		edit.CategoryID == nil && edit.UserID == nil {
		return errorResult(fmt.Errorf("provide at least one field to update")), nil, nil
	}
	actor := service.ActorFromContext(ctx)
	series, err := s.svc.UpdateSeries(context.Background(), input.ID, edit, actor)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			return errorResult(fmt.Errorf("series not found")), nil, nil
		}
		return errorResult(err), nil, nil
	}
	return jsonResult(series)
}

type unlinkSeriesInput struct {
	ID             string   `json:"id" jsonschema:"required,Series short ID or UUID."`
	TransactionIDs []string `json:"transaction_ids" jsonschema:"required,Transactions (short ID or UUID) to detach from the series. Each must currently belong to it. Max 50."`
}

func (s *MCPServer) handleUnlinkSeriesTransactions(ctx context.Context, _ *mcpsdk.CallToolRequest, input unlinkSeriesInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.ID == "" || len(input.TransactionIDs) == 0 {
		return errorResult(fmt.Errorf("id and transaction_ids are required")), nil, nil
	}
	actor := service.ActorFromContext(ctx)
	series, err := s.svc.UnlinkSeriesTransactions(context.Background(), input.ID, input.TransactionIDs, actor)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			return errorResult(fmt.Errorf("series not found")), nil, nil
		}
		return errorResult(err), nil, nil
	}
	return jsonResult(series)
}

type setSeriesTypeInput struct {
	ID   string `json:"id" jsonschema:"required,Series short ID or UUID."`
	Type string `json:"type" jsonschema:"required,One of: subscription, bill, loan, other."`
}

func (s *MCPServer) handleSetSeriesType(ctx context.Context, _ *mcpsdk.CallToolRequest, input setSeriesTypeInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.ID == "" || input.Type == "" {
		return errorResult(fmt.Errorf("id and type are required")), nil, nil
	}
	actor := service.ActorFromContext(ctx)
	series, err := s.svc.SetSeriesType(context.Background(), input.ID, input.Type, actor)
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

type rekeySeriesInput struct {
	ID     string `json:"id" jsonschema:"required,Series short ID or UUID to re-key."`
	NewKey string `json:"new_merchant_key" jsonschema:"required,The corrected normalized merchant key (e.g. 'spotify' rather than a fallback like 'payment'). Members' merchant_key is repointed to match."`
}

func (s *MCPServer) handleRekeySeries(ctx context.Context, _ *mcpsdk.CallToolRequest, input rekeySeriesInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.ID == "" || input.NewKey == "" {
		return errorResult(fmt.Errorf("id and new_merchant_key are required")), nil, nil
	}
	actor := service.ActorFromContext(ctx)
	series, err := s.svc.RekeySeries(context.Background(), input.ID, input.NewKey, actor)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			return errorResult(fmt.Errorf("series not found")), nil, nil
		}
		return errorResult(err), nil, nil
	}
	return jsonResult(series)
}

type splitSeriesInput struct {
	ID             string   `json:"id" jsonschema:"required,Source series short ID or UUID to split."`
	NewKey         string   `json:"new_merchant_key" jsonschema:"required,Merchant key for the new series the split-out charges move into. Must not already have a series and must differ from the source key."`
	Name           string   `json:"name,omitempty" jsonschema:"Optional display name for the new series; defaults to a title-cased new_merchant_key."`
	TransactionIDs []string `json:"transaction_ids" jsonschema:"required,Transactions (short ID or UUID) to move out of the source series into the new one. Each must currently belong to the source series. Max 50."`
}

func (s *MCPServer) handleSplitSeries(ctx context.Context, _ *mcpsdk.CallToolRequest, input splitSeriesInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.ID == "" || input.NewKey == "" || len(input.TransactionIDs) == 0 {
		return errorResult(fmt.Errorf("id, new_merchant_key, and transaction_ids are required")), nil, nil
	}
	actor := service.ActorFromContext(ctx)
	series, err := s.svc.SplitSeries(context.Background(), input.ID, input.TransactionIDs, input.NewKey, input.Name, actor)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			return errorResult(fmt.Errorf("series not found")), nil, nil
		}
		return errorResult(err), nil, nil
	}
	return jsonResult(series)
}

func (s *MCPServer) handleAssignSeries(ctx context.Context, _ *mcpsdk.CallToolRequest, input assignSeriesInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	actor := service.ActorFromContext(ctx)
	series, err := s.svc.AssignSeries(context.Background(), service.AssignSeriesInput{
		SeriesID:        optStr(input.SeriesID),
		MerchantKey:     input.MerchantKey,
		CreateIfMissing: input.CreateIfMissing,
		Name:            input.Name,
		Cadence:         input.Cadence,
		Type:            input.Type,
		ExpectedAmount:  input.ExpectedAmount,
		Currency:        optStr(input.Currency),
		CategoryID:      optStr(input.CategoryID),
		UserID:          optStr(input.UserID),
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
