package api

import (
	"encoding/json"
	"net/http"
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
