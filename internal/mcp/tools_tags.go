package mcp

import (
	"context"
	"errors"
	"fmt"

	"breadbox/internal/service"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- Input types ---

type listTagsInput struct {
	ReadSessionContext
}

type listAnnotationsInput struct {
	ReadSessionContext
	TransactionID string   `json:"transaction_id" jsonschema:"required,UUID or short ID of the transaction"`
	Kinds         []string `json:"kinds,omitempty" jsonschema:"Optional kind filter: any of comment, rule, tag, category, sync. Empty = all kinds. Pass ['comment'] for the comment-only timeline (replaces list_transaction_comments). Pass ['tag'] to see both add+remove events; the response carries an 'action' field (added|removed|set|applied|started|updated) for the specific event. Pass ['sync'] to see initial-import + pending-flip rows."`
}

type addTransactionTagInput struct {
	WriteSessionContext
	TransactionID string `json:"transaction_id" jsonschema:"required,UUID or short ID of the transaction"`
	TagSlug       string `json:"tag_slug" jsonschema:"required,Tag slug to add (e.g. 'needs-review'). Auto-created as persistent if not registered."`
}

type removeTransactionTagInput struct {
	WriteSessionContext
	TransactionID string `json:"transaction_id" jsonschema:"required,UUID or short ID of the transaction"`
	TagSlug       string `json:"tag_slug" jsonschema:"required,Tag slug to remove"`
}

type createTagInput struct {
	WriteSessionContext
	Slug        string  `json:"slug" jsonschema:"required,Tag slug. Lowercase alphanumerics with hyphens/colons, e.g. 'needs-review' or 'subscription:monthly'."`
	DisplayName string  `json:"display_name" jsonschema:"required,Human-readable name (e.g. 'Needs Review')."`
	Description string  `json:"description,omitempty" jsonschema:"Optional description."`
	Color       *string `json:"color,omitempty" jsonschema:"Optional CSS color (e.g. '#4f46e5') used for chip rendering."`
	Icon        *string `json:"icon,omitempty" jsonschema:"Optional Lucide icon name (e.g. 'inbox')."`
}

type updateTagInput struct {
	WriteSessionContext
	ID          string  `json:"id" jsonschema:"required,Tag UUID, short ID, or slug."`
	DisplayName *string `json:"display_name,omitempty" jsonschema:"New display name."`
	Description *string `json:"description,omitempty" jsonschema:"New description."`
	Color       *string `json:"color,omitempty" jsonschema:"New color (pass empty string to clear)."`
	Icon        *string `json:"icon,omitempty" jsonschema:"New icon (pass empty string to clear)."`
}

type deleteTagInput struct {
	WriteSessionContext
	ID string `json:"id" jsonschema:"required,Tag UUID, short ID, or slug."`
}

// --- Handlers ---

func (s *MCPServer) handleListTags(_ context.Context, _ *mcpsdk.CallToolRequest, _ listTagsInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
	tags, err := s.svc.ListTags(ctx)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(tags)
}

func (s *MCPServer) handleListAnnotations(_ context.Context, _ *mcpsdk.CallToolRequest, input listAnnotationsInput) (*mcpsdk.CallToolResult, any, error) {
	ctx := context.Background()
	if input.TransactionID == "" {
		return errorResult(fmt.Errorf("transaction_id is required")), nil, nil
	}
	dbKinds, err := mapAnnotationKinds(input.Kinds)
	if err != nil {
		return errorResult(err), nil, nil
	}
	annotations, err := s.svc.ListAnnotations(ctx, input.TransactionID, service.ListAnnotationsParams{
		Kinds: dbKinds,
	})
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			return errorResult(fmt.Errorf("transaction not found")), nil, nil
		}
		return errorResult(err), nil, nil
	}
	return jsonResult(toMCPAnnotations(annotations))
}

// mcpAnnotationKinds enumerates the generic kinds exposed at the MCP boundary.
// The DB CHECK constraint stores finer-grained values (tag_added vs tag_removed,
// rule_applied, category_set) — agents see one normalized name plus an `action`
// field on each row.
var mcpAnnotationKinds = map[string][]string{
	"comment":  {"comment"},
	"rule":     {"rule_applied"},
	"tag":      {"tag_added", "tag_removed"},
	"category": {"category_set"},
	"sync":     {"sync_started", "sync_updated"},
}

// mapAnnotationKinds translates the agent-facing generic kinds into the raw DB
// kinds the service layer filters by. Returns nil for an empty input (no
// filter). Rejects unknown kinds at the boundary so agents get a clear error
// instead of a silent empty slice.
func mapAnnotationKinds(kinds []string) ([]string, error) {
	if len(kinds) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(kinds))
	seen := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		raw, ok := mcpAnnotationKinds[k]
		if !ok {
			return nil, fmt.Errorf("invalid kind %q: expected one of comment, rule, tag, category, sync", k)
		}
		for _, r := range raw {
			if seen[r] {
				continue
			}
			seen[r] = true
			out = append(out, r)
		}
	}
	return out, nil
}

// mcpAnnotation is the agent-facing annotation shape. `kind` is the generic
// name (comment | rule | tag | category); `action` carries the specific
// event verb (added | removed | set | applied | commented) so agents can
// branch without parsing the kind string.
//
// `summary` is a human-readable one-liner ("Alice added the food tag") so
// agents can read activity directly without composing sentences from
// payload keys; the underlying `payload` is preserved for raw access.
// `subject` is the canonical object of the event (tag display name,
// category display name, rule name, or comment body preview), and the
// top-level resource refs (`tag_slug`, `category_slug`, `rule_name`) make
// cross-linking cheap.
type mcpAnnotation struct {
	ID            string                 `json:"id"`
	ShortID       string                 `json:"short_id"`
	TransactionID string                 `json:"transaction_id"`
	Kind          string                 `json:"kind"`
	Action        string                 `json:"action,omitempty"`
	Summary       string                 `json:"summary,omitempty"`
	Subject       string                 `json:"subject,omitempty"`
	Origin        string                 `json:"origin,omitempty"`
	Source        string                 `json:"source,omitempty"`
	Content       string                 `json:"content,omitempty"`
	Note          string                 `json:"note,omitempty"`
	TagSlug       string                 `json:"tag_slug,omitempty"`
	CategorySlug  string                 `json:"category_slug,omitempty"`
	RuleName      string                 `json:"rule_name,omitempty"`
	ActorType     string                 `json:"actor_type"`
	ActorID       *string                `json:"actor_id,omitempty"`
	ActorName     string                 `json:"actor_name"`
	SessionID     *string                `json:"session_id,omitempty"`
	Payload       map[string]interface{} `json:"payload,omitempty"`
	TagID         *string                `json:"tag_id,omitempty"`
	RuleID        *string                `json:"rule_id,omitempty"`
	CreatedAt     string                 `json:"created_at"`
}

// dbKindToMCP translates a stored DB kind into the (kind, action) pair surfaced
// to MCP clients. Unknown kinds round-trip unchanged so future kinds don't
// silently disappear from the timeline.
func dbKindToMCP(dbKind string) (kind, action string) {
	switch dbKind {
	case "comment":
		return "comment", ""
	case "rule_applied":
		return "rule", "applied"
	case "tag_added":
		return "tag", "added"
	case "tag_removed":
		return "tag", "removed"
	case "category_set":
		return "category", "set"
	case "sync_started":
		return "sync", "started"
	case "sync_updated":
		return "sync", "updated"
	}
	return dbKind, ""
}

func toMCPAnnotation(a service.Annotation) mcpAnnotation {
	kind, fallbackAction := dbKindToMCP(a.Kind)
	// Action prefers the enriched value populated by
	// service.EnrichAnnotations (which leaves it empty for kind=comment
	// by design); fall back to the legacy kind→action mapping for raw
	// rows returned via ListAnnotations(Raw=true).
	action := a.Action
	if action == "" && a.Kind != "comment" {
		action = fallbackAction
	}
	return mcpAnnotation{
		ID:            a.ID,
		ShortID:       a.ShortID,
		TransactionID: a.TransactionID,
		Kind:          kind,
		Action:        action,
		Summary:       a.Summary,
		Subject:       a.Subject,
		Origin:        a.Origin,
		Source:        a.Source,
		Content:       a.Content,
		Note:          a.Note,
		TagSlug:       a.TagSlug,
		CategorySlug:  a.CategorySlug,
		RuleName:      a.RuleName,
		ActorType:     a.ActorType,
		ActorID:       a.ActorID,
		ActorName:     a.ActorName,
		SessionID:     a.SessionID,
		Payload:       a.Payload,
		TagID:         a.TagID,
		RuleID:        a.RuleID,
		CreatedAt:     a.CreatedAt,
	}
}

func toMCPAnnotations(in []service.Annotation) []mcpAnnotation {
	out := make([]mcpAnnotation, len(in))
	for i, a := range in {
		out[i] = toMCPAnnotation(a)
	}
	return out
}

func (s *MCPServer) handleAddTransactionTag(ctx context.Context, _ *mcpsdk.CallToolRequest, input addTransactionTagInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.TransactionID == "" || input.TagSlug == "" {
		return errorResult(fmt.Errorf("transaction_id and tag_slug are required")), nil, nil
	}
	actor := service.ActorFromContext(ctx)
	added, alreadyPresent, err := s.svc.AddTransactionTag(context.Background(), input.TransactionID, input.TagSlug, actor)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(map[string]any{
		"added":           added,
		"already_present": alreadyPresent,
		"tag_slug":        input.TagSlug,
		"transaction_id":  input.TransactionID,
	})
}

func (s *MCPServer) handleRemoveTransactionTag(ctx context.Context, _ *mcpsdk.CallToolRequest, input removeTransactionTagInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.TransactionID == "" || input.TagSlug == "" {
		return errorResult(fmt.Errorf("transaction_id and tag_slug are required")), nil, nil
	}
	actor := service.ActorFromContext(ctx)
	removed, alreadyAbsent, err := s.svc.RemoveTransactionTag(context.Background(), input.TransactionID, input.TagSlug, actor)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(map[string]any{
		"removed":        removed,
		"already_absent": alreadyAbsent,
		"tag_slug":       input.TagSlug,
		"transaction_id": input.TransactionID,
	})
}

func (s *MCPServer) handleCreateTag(ctx context.Context, _ *mcpsdk.CallToolRequest, input createTagInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.Slug == "" || input.DisplayName == "" {
		return errorResult(fmt.Errorf("slug and display_name are required")), nil, nil
	}
	params := service.CreateTagParams{
		Slug:        input.Slug,
		DisplayName: input.DisplayName,
		Description: input.Description,
		Color:       input.Color,
		Icon:        input.Icon,
	}
	tag, err := s.svc.CreateTag(context.Background(), params)
	if err != nil {
		return errorResult(err), nil, nil
	}
	return jsonResult(tag)
}

func (s *MCPServer) handleUpdateTag(ctx context.Context, _ *mcpsdk.CallToolRequest, input updateTagInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.ID == "" {
		return errorResult(fmt.Errorf("id is required")), nil, nil
	}
	params := service.UpdateTagParams{
		DisplayName: input.DisplayName,
		Description: input.Description,
		Color:       input.Color,
		Icon:        input.Icon,
	}
	tag, err := s.svc.UpdateTag(context.Background(), input.ID, params)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			return errorResult(fmt.Errorf("tag not found")), nil, nil
		}
		return errorResult(err), nil, nil
	}
	return jsonResult(tag)
}

func (s *MCPServer) handleDeleteTag(ctx context.Context, _ *mcpsdk.CallToolRequest, input deleteTagInput) (*mcpsdk.CallToolResult, any, error) {
	if err := s.checkWritePermission(ctx); err != nil {
		return errorResult(err), nil, nil
	}
	if input.ID == "" {
		return errorResult(fmt.Errorf("id is required")), nil, nil
	}
	if err := s.svc.DeleteTag(context.Background(), input.ID); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			return errorResult(fmt.Errorf("tag not found")), nil, nil
		}
		return errorResult(err), nil, nil
	}
	return jsonResult(map[string]any{
		"deleted": true,
		"id":      input.ID,
	})
}
