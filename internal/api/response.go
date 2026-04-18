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

// writeError writes the standard JSON error envelope with the given HTTP
// status code. Thin wrapper around middleware.WriteError so handlers don't
// need to import the middleware package just to emit error responses.
func writeError(w http.ResponseWriter, status int, code, message string) {
	mw.WriteError(w, status, code, message)
}
