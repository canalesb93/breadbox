//go:build !lite

package mcp

import (
	"context"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// list_workflows surfaces the household's automation layer to an agent: the
// enabled, preset-backed Workflows (name, slug, trigger, source preset, last
// run status) plus the full catalog of available presets it could enable. It
// is read-only and reshapes the same service paths the admin /workflows
// gallery uses (Service.ListWorkflowsForMCP → ListAgentDefinitions +
// ListWorkflowPresets), so an agent can answer "what runs automatically here,
// and what else is on offer?" without an HTTP round-trip.

// listWorkflowsInput takes no parameters — the result is the full picture
// (enabled workflows + every preset). An empty input struct keeps the tool's
// schema minimal, matching the other zero-arg reference reads (list_categories,
// list_users) in tools_reads.go.
type listWorkflowsInput struct {
}

// handleListWorkflows returns the enabled-workflows + available-presets
// payload. Mirrors the tool-shaped envelope of the other reference reads: a
// single jsonResult that runs through compactIDs. The views carry slug-only
// identity (no UUIDs), so compaction is a no-op here — but routing through
// jsonResult keeps the dual TextContent/StructuredContent contract consistent
// with every other tool.
func (s *MCPServer) handleListWorkflows(_ context.Context, _ *mcpsdk.CallToolRequest, _ listWorkflowsInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
	result, err := s.svc.ListWorkflowsForMCP(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(result)
}
