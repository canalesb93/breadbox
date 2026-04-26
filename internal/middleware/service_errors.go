package middleware

import (
	"errors"
	"net/http"

	"breadbox/internal/service"
)

// ServiceErrorResponse is the canonical HTTP response for a known
// service-layer sentinel error: status code, UPPER_SNAKE_CASE error code,
// and human-readable message.
type ServiceErrorResponse struct {
	Status  int
	Code    string
	Message string
}

// MapServiceError returns the canonical HTTP response for a known
// service-layer sentinel error, or false if err doesn't match any sentinel.
//
// errors.Is is used so wrapped errors (fmt.Errorf("...: %w", err)) still
// resolve. The returned Message is the wrapped error's Error() string,
// preserving any context the service layer attached. Callers that want a
// fixed resource-specific message can override the Message before writing.
//
// The returned codes are stable API contracts — the same UPPER_SNAKE_CASE
// strings are surfaced in JSON error envelopes by both REST and admin
// handlers, and mirrored on the MCP side by internal/mcp/errors.go.
func MapServiceError(err error) (ServiceErrorResponse, bool) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		return ServiceErrorResponse{Status: http.StatusNotFound, Code: "NOT_FOUND", Message: err.Error()}, true
	case errors.Is(err, service.ErrInvalidParameter):
		return ServiceErrorResponse{Status: http.StatusBadRequest, Code: "INVALID_PARAMETER", Message: err.Error()}, true
	case errors.Is(err, service.ErrInvalidCursor):
		return ServiceErrorResponse{Status: http.StatusBadRequest, Code: "INVALID_CURSOR", Message: err.Error()}, true
	case errors.Is(err, service.ErrForbidden):
		return ServiceErrorResponse{Status: http.StatusForbidden, Code: "FORBIDDEN", Message: err.Error()}, true
	case errors.Is(err, service.ErrSyncInProgress):
		return ServiceErrorResponse{Status: http.StatusConflict, Code: "SYNC_IN_PROGRESS", Message: err.Error()}, true
	case errors.Is(err, service.ErrInvalidAPIKey):
		return ServiceErrorResponse{Status: http.StatusUnauthorized, Code: "INVALID_API_KEY", Message: err.Error()}, true
	case errors.Is(err, service.ErrRevokedAPIKey):
		return ServiceErrorResponse{Status: http.StatusUnauthorized, Code: "REVOKED_API_KEY", Message: err.Error()}, true
	}
	return ServiceErrorResponse{}, false
}
