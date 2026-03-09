package middleware

import "net/http"

// RequireWriteScope rejects requests from read_only API keys.
func RequireWriteScope() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := GetAPIKey(r.Context())
			if apiKey != nil && apiKey.Scope == "read_only" {
				WriteError(w, http.StatusForbidden, "INSUFFICIENT_SCOPE",
					"This API key has read-only access and cannot perform write operations")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
