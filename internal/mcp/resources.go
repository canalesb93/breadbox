package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"breadbox/internal/service"
	"breadbox/prompts"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Default static-resource bodies sourced from the embedded prompts package.
// Editable in prompts/mcp/*.md and rebuilt with the binary.
var (
	DefaultRuleDSL = prompts.MCP("rule-dsl")
)

func (s *MCPServer) handleOverviewResource(ctx context.Context, _ *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
	stats, err := s.svc.GetOverviewStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("get overview stats: %w", err)
	}
	return jsonResourceResult("breadbox://overview", stats)
}

func (s *MCPServer) handleReviewGuidelinesResource(_ context.Context, _ *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
	ctx := context.Background()
	cfg, err := s.svc.GetMCPConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("get mcp config: %w", err)
	}

	content := cfg.ReviewGuidelines
	if content == "" {
		content = DefaultReviewGuidelines
	}

	return &mcpsdk.ReadResourceResult{
		Contents: []*mcpsdk.ResourceContents{
			{
				URI:      "breadbox://review-guidelines",
				MIMEType: "text/markdown",
				Text:     content,
			},
		},
	}, nil
}

func (s *MCPServer) handleReportFormatResource(_ context.Context, _ *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
	ctx := context.Background()
	cfg, err := s.svc.GetMCPConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("get mcp config: %w", err)
	}

	content := cfg.ReportFormat
	if content == "" {
		content = DefaultReportFormat
	}

	return &mcpsdk.ReadResourceResult{
		Contents: []*mcpsdk.ResourceContents{
			{
				URI:      "breadbox://report-format",
				MIMEType: "text/markdown",
				Text:     content,
			},
		},
	}, nil
}

// staticMarkdownResource returns a handler that serves a fixed markdown body.
// Used for rule-dsl — agent-facing reference that doesn't have an app_config
// override slot today.
func staticMarkdownResource(uri, content string) func(context.Context, *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
	return func(_ context.Context, _ *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
		return &mcpsdk.ReadResourceResult{
			Contents: []*mcpsdk.ResourceContents{
				{
					URI:      uri,
					MIMEType: "text/markdown",
					Text:     content,
				},
			},
		}, nil
	}
}

// marshalResourceJSON marshals v as compact JSON and runs compactIDsBytes
// so the resource payload follows the same compact-ID convention as MCP tool
// responses.
//
// Compact (no-indent) marshalling is load-bearing: the byte-scanner in
// compactIDsBytes was designed for the output of json.Marshal and does not
// handle the whitespace json.MarshalIndent inserts between values. On
// payloads with deeply nested objects (the rule list's conditions JSONB hit
// this), the scanner could mis-step into an infinite loop. Sticking to plain
// Marshal keeps the resource and tool surfaces using identical byte layouts.
func marshalResourceJSON(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return compactIDsBytes(raw), nil
}

// jsonResourceResult is the standard wrapper for service-backed JSON resources.
// Keeps every handler's tail uniform: marshal → compact IDs → wrap.
func jsonResourceResult(uri string, v any) (*mcpsdk.ReadResourceResult, error) {
	data, err := marshalResourceJSON(v)
	if err != nil {
		return nil, fmt.Errorf("marshal %s: %w", uri, err)
	}
	return &mcpsdk.ReadResourceResult{
		Contents: []*mcpsdk.ResourceContents{
			{
				URI:      uri,
				MIMEType: "application/json",
				Text:     string(data),
			},
		},
	}, nil
}

func (s *MCPServer) handleCategoriesResource(ctx context.Context, _ *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
	cats, err := s.svc.ListCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	return jsonResourceResult("breadbox://categories", map[string]any{"categories": cats})
}

func (s *MCPServer) handleAccountsResource(ctx context.Context, _ *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
	accts, err := s.svc.ListAccounts(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	return jsonResourceResult("breadbox://accounts", map[string]any{"accounts": accts})
}

func (s *MCPServer) handleUsersResource(ctx context.Context, _ *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
	users, err := s.svc.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	return jsonResourceResult("breadbox://users", map[string]any{"users": users})
}

func (s *MCPServer) handleTagsResource(ctx context.Context, _ *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
	tags, err := s.svc.ListTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	return jsonResourceResult("breadbox://tags", map[string]any{"tags": tags})
}

func (s *MCPServer) handleSyncStatusResource(ctx context.Context, _ *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
	conns, err := s.svc.ListConnections(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("list connections: %w", err)
	}
	return jsonResourceResult("breadbox://sync-status", map[string]any{"connections": conns})
}

// rulesResourceLimit caps the rules resource payload. The rule list is small
// in practice (households tend to have tens, not thousands of rules), but the
// cap keeps the resource bounded and predictable.
const rulesResourceLimit = 200

func (s *MCPServer) handleRulesResource(ctx context.Context, _ *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
	result, err := s.svc.ListTransactionRules(ctx, service.TransactionRuleListParams{
		Limit: rulesResourceLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	return jsonResourceResult("breadbox://rules", result)
}

// resourceAnnotations builds the standard *mcpsdk.Annotations payload used on
// every Breadbox resource. lastModifiedFn returns the timestamp the client
// should treat as the resource's mtime; pass nil for resources that don't
// expose a meaningful mtime.
func resourceAnnotations(audience []mcpsdk.Role, priority float64, lastModifiedFn func() time.Time) *mcpsdk.Annotations {
	a := &mcpsdk.Annotations{
		Audience: audience,
		Priority: priority,
	}
	if lastModifiedFn != nil {
		a.LastModified = lastModifiedFn().UTC().Format(time.RFC3339)
	}
	return a
}

// staticPromptModTime returns the build time as a stable mtime for embedded
// markdown resources. Embed reads don't expose file timestamps; the binary
// build time is the closest meaningful proxy.
func staticPromptModTime() time.Time {
	return buildStartTime
}

// liveResourceModTime returns the current time so clients can observe that
// service-backed resources are recomputed on every read.
func liveResourceModTime() time.Time {
	return time.Now()
}

// buildStartTime is captured once at program start so successive reads of
// static markdown resources advertise a consistent mtime.
var buildStartTime = time.Now()

// audienceUserAndAssistant marks resources that should appear in the user's
// attachment menu (paperclip in Claude.ai). audienceAssistantOnly hides them
// from user pickers in hosts that filter by audience — appropriate for
// agent-internal references.
var (
	audienceUserAndAssistant = []mcpsdk.Role{"user", "assistant"}
	audienceAssistantOnly    = []mcpsdk.Role{"assistant"}
)
