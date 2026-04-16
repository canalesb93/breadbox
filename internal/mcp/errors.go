package mcp

import (
	"encoding/json"
	"errors"

	"breadbox/internal/service"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Error codes returned by MCP tool error envelopes.
//
// These mirror the UPPER_SNAKE_CASE codes used by the REST API in
// internal/api so that agents can programmatically distinguish error
// conditions without parsing the human-readable message.
const (
	CodeNotFound            = "NOT_FOUND"
	CodeForbidden           = "FORBIDDEN"
	CodeInvalidParameter    = "INVALID_PARAMETER"
	CodeInvalidCategory     = "INVALID_CATEGORY"
	CodeInvalidCursor       = "INVALID_CURSOR"
	CodeSyncInProgress      = "SYNC_IN_PROGRESS"
	CodeSlugConflict        = "SLUG_CONFLICT"
	CodeCategoryUndeletable = "CATEGORY_UNDELETABLE"
	CodeInvalidAPIKey       = "INVALID_API_KEY"
	CodeRevokedAPIKey       = "REVOKED_API_KEY"
	CodeInternalError       = "INTERNAL_ERROR"
)

// ErrorCode maps a Go error to a stable UPPER_SNAKE_CASE code string.
//
// It uses errors.Is so wrapped errors (fmt.Errorf("...: %w", err)) still
// resolve to the correct code. Unknown errors fall through to
// CodeInternalError, matching the REST API convention where unmapped
// service errors surface as 500/INTERNAL_ERROR.
func ErrorCode(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, service.ErrNotFound),
		errors.Is(err, service.ErrCategoryNotFound):
		return CodeNotFound
	case errors.Is(err, service.ErrForbidden):
		return CodeForbidden
	case errors.Is(err, service.ErrInvalidCursor):
		return CodeInvalidCursor
	case errors.Is(err, service.ErrInvalidParameter):
		return CodeInvalidParameter
	case errors.Is(err, service.ErrSyncInProgress):
		return CodeSyncInProgress
	case errors.Is(err, service.ErrSlugConflict):
		return CodeSlugConflict
	case errors.Is(err, service.ErrCategoryUndeletable):
		return CodeCategoryUndeletable
	case errors.Is(err, service.ErrInvalidAPIKey):
		return CodeInvalidAPIKey
	case errors.Is(err, service.ErrRevokedAPIKey):
		return CodeRevokedAPIKey
	}
	return CodeInternalError
}

// errorResult builds an MCP tool error result with an UPPER_SNAKE_CASE
// code derived from the error via ErrorCode.
//
// Shape: {"code": "REVIEW_ALREADY_RESOLVED", "error": "review has already been resolved"}
func errorResult(err error) *mcpsdk.CallToolResult {
	return errorResultWithCode(ErrorCode(err), err.Error())
}

// errorResultWithCode is the low-level builder that lets callers
// specify both the code and human-readable message directly. Useful
// for validation errors that don't originate from a service sentinel
// (e.g., missing required input fields).
func errorResultWithCode(code, message string) *mcpsdk.CallToolResult {
	payload := map[string]string{
		"code":  code,
		"error": message,
	}
	errJSON, _ := json.Marshal(payload)
	return &mcpsdk.CallToolResult{
		IsError: true,
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: string(errJSON)},
		},
	}
}
