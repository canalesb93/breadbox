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
