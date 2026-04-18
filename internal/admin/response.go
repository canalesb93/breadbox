package admin

import (
	"net/http"

	"breadbox/internal/middleware"
)

// writeError writes the canonical JSON error envelope documented in
// .claude/rules/api.md: { "error": { "code": "...", "message": "..." } }.
// Use this instead of hand-rolling map literals so admin responses stay
// consistent with the REST API contract.
func writeError(w http.ResponseWriter, status int, code, message string) {
	middleware.WriteError(w, status, code, message)
}
