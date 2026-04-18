package admin

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

// writeError writes the canonical {"error": {"code", "message"}} envelope.
// Thin wrapper around middleware.WriteError so admin handlers share the same
// shape as the REST API without pulling the middleware import into every file.
func writeError(w http.ResponseWriter, status int, code, message string) {
	mw.WriteError(w, status, code, message)
}

// decodeJSON decodes the JSON request body into v. On failure it writes a
// standard 400 INVALID_REQUEST error envelope and returns false; callers should
// return immediately when this returns false.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return false
	}
	return true
}
