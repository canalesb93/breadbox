package middleware

import (
	"errors"
	"fmt"
	"net/http"

	"breadbox/internal/service"

	"github.com/jackc/pgx/v5/pgtype"
)

func formatUUID(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

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

			apiKey, err := svc.ValidateAPIKey(r.Context(), key)
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

			// Store API key identity in context for actor resolution.
			keyID := formatUUID(apiKey.ID)
			ctx := service.ContextWithAPIKey(r.Context(), keyID, apiKey.Name)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
