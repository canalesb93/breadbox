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
