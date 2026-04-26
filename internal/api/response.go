package api

import (
	"encoding/json"
	"net/http"

	mw "breadbox/internal/middleware"
)

// writeJSON writes v as JSON with the given HTTP status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeData writes v as JSON with HTTP 200 OK.
func writeData(w http.ResponseWriter, v any) {
	writeJSON(w, http.StatusOK, v)
}

// decodeJSON decodes the JSON request body into v. On failure it writes a
// standard 400 INVALID_BODY error envelope and returns false; callers should
// return immediately when this returns false.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		mw.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON body")
		return false
	}
	return true
}

// writeServiceError maps a service-layer sentinel error to the canonical
// {error: {code, message}} envelope. Unmapped errors fall through to a
// 500 INTERNAL_ERROR with internalMessage.
//
// notFoundMessage overrides the response message when err resolves to
// service.ErrNotFound — pass empty to use the wrapped error's own text.
// Handlers that need to override other codes (e.g. FORBIDDEN with a
// resource-specific message) should call mw.MapServiceError directly.
func writeServiceError(w http.ResponseWriter, err error, notFoundMessage, internalMessage string) {
	if resp, ok := mw.MapServiceError(err); ok {
		if notFoundMessage != "" && resp.Code == "NOT_FOUND" {
			resp.Message = notFoundMessage
		}
		mw.WriteError(w, resp.Status, resp.Code, resp.Message)
		return
	}
	mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", internalMessage)
}
