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
	Status string `json:"status,omitempty" jsonschema:"Deprecated and ignored — a recurring series no longer has a lifecycle status. Accepted for compatibility."`
	Fields string `json:"fields,omitempty" jsonschema:"Comma-separated fields to include, to cut response size. Aliases: minimal (name,type), overview (name,type,tags; the default). Default when omitted: overview. Pass fields=all for every field. id is always included."`
}

type getSeriesInput struct {
	ID string `json:"id" jsonschema:"required,Series short ID or UUID."`
}

type assignSeriesInput struct {
	SeriesID        string   `json:"series_id,omitempty" jsonschema:"Existing series short ID or UUID to assign transactions to. Provide this OR (series_name + create_if_missing)."`
	SeriesName      string   `json:"series_name,omitempty" jsonschema:"Name to mint a new series under (requires create_if_missing). Surrogate-first: the same name always resolves the same series. Use a clean, canonical label — e.g. 'Netflix'."`
	CreateIfMissing bool     `json:"create_if_missing,omitempty" jsonschema:"Mint (or resolve) a series by series_name when no series_id is given."`
	Type            string   `json:"type,omitempty" jsonschema:"Optional recurring-charge type for a minted series: subscription (default), bill, loan, or other."`
	TransactionIDs  []string `json:"transaction_ids,omitempty" jsonschema:"Transactions (short ID or UUID) to link to the series. Max 50 per call."`
}

type updateSeriesInput struct {
	ID   string `json:"id" jsonschema:"required,Series short ID or UUID to edit."`
	Name string `json:"name,omitempty" jsonschema:"New display name. Renaming onto an existing live series name is rejected. Omit to leave unchanged."`
	Type string `json:"type,omitempty" jsonschema:"New recurring-charge type: subscription, bill, loan, or other. Omit to leave unchanged."`
}

type unlinkSeriesInput struct {
	ID             string   `json:"id" jsonschema:"required,Series short ID or UUID."`
	TransactionIDs []string `json:"transaction_ids" jsonschema:"required,Transactions (short ID or UUID) to detach from the series. Each must currently belong to it. Max 50."`
}

type seriesTagInput struct {
	ID      string `json:"id" jsonschema:"required,Series short ID or UUID."`
	TagSlug string `json:"tag_slug" jsonschema:"required,Tag slug (must already exist)."`
}

func (s *MCPServer) handleListSeries(_ context.Context, _ *mcpsdk.CallToolRequest, input listSeriesInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
	// Lean-by-default: list_series returns the overview projection (identity +
	// type + tags) unless the caller asks for more.
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

	series, err := s.svc.ListSeries(ctx, nil)
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

func (s *MCPServer) handleAssignSeries(ctx context.Context, _ *mcpsdk.CallToolRequest, input assignSeriesInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	actor := service.ActorFromContext(ctx)
	series, err := s.svc.AssignSeries(context.Background(), service.AssignSeriesInput{
		SeriesID:        optStr(input.SeriesID),
		Name:            input.SeriesName,
		CreateIfMissing: input.CreateIfMissing,
		Type:            input.Type,
		TransactionIDs:  input.TransactionIDs,
	}, actor)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			return errorResult(fmt.Errorf("series not found")), nil, nil
		}
		return errorResult(err), nil, nil
	}
	return jsonResult(series)
}

func (s *MCPServer) handleUpdateSeries(ctx context.Context, _ *mcpsdk.CallToolRequest, input updateSeriesInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.ID == "" {
		return errorResult(fmt.Errorf("id is required")), nil, nil
	}
	edit := service.EditSeriesInput{
		Name: optStr(input.Name),
		Type: optStr(input.Type),
	}
	if edit.Name == nil && edit.Type == nil {
		return errorResult(fmt.Errorf("provide name and/or type to update")), nil, nil
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
