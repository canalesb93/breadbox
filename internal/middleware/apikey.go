package middleware

import (
	"errors"
	"net/http"

	"breadbox/internal/service"
)

// APIKeyAuth returns middleware that validates the X-API-Key header against
// the database using the service layer.
func APIKeyAuth(svc *service.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("X-API-Key")
			if key == "" {
				WriteError(w, http.StatusUnauthorized, "MISSING_API_KEY", "X-API-Key header is required")
				return
			}

			_, err := svc.ValidateAPIKey(r.Context(), key)
			if err != nil {
				if errors.Is(err, service.ErrInvalidAPIKey) {
					WriteError(w, http.StatusUnauthorized, "INVALID_API_KEY", "The provided API key is not valid")
				} else if errors.Is(err, service.ErrRevokedAPIKey) {
					WriteError(w, http.StatusUnauthorized, "REVOKED_API_KEY", "The provided API key has been revoked")
				} else {
					WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to validate API key")
				}
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
