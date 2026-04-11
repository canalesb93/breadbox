package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"
)

// APIKeyAuth returns middleware that validates either the X-API-Key header or
// an Authorization: Bearer token against the database using the service layer.
// Bearer tokens are OAuth 2.1 access tokens; API keys are the existing bb_* keys.
func APIKeyAuth(svc *service.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Try Bearer token first (OAuth 2.1).
			if token := extractBearerToken(r); token != "" {
				accessToken, err := svc.ValidateBearerToken(r.Context(), token)
				if err != nil {
					if errors.Is(err, service.ErrInvalidBearerToken) {
						writeWWWAuthenticate(w, http.StatusUnauthorized, "invalid_token", "The access token is invalid")
					} else if errors.Is(err, service.ErrExpiredBearerToken) {
						writeWWWAuthenticate(w, http.StatusUnauthorized, "invalid_token", "The access token has expired")
					} else if errors.Is(err, service.ErrRevokedBearerToken) {
						writeWWWAuthenticate(w, http.StatusUnauthorized, "invalid_token", "The access token has been revoked")
					} else {
						WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to validate token")
					}
					return
				}

				// Look up OAuth client name for display purposes.
				clientName := accessToken.ClientID
				if client, err := svc.Queries.GetOAuthClientByClientID(r.Context(), accessToken.ClientID); err == nil {
					clientName = client.Name
				}

				// Create a synthetic API key record so existing scope checks work.
				syntheticKey := &db.ApiKey{
					ID:    accessToken.ID,
					Name:  clientName,
					Scope: accessToken.Scope,
				}

				keyID := pgconv.FormatUUID(accessToken.ID)
				ctx := service.ContextWithAPIKey(r.Context(), keyID, syntheticKey.Name)
				ctx = SetAPIKey(ctx, syntheticKey)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Fall back to X-API-Key header.
			key := r.Header.Get("X-API-Key")
			if key == "" {
				// Return 401 with WWW-Authenticate to trigger OAuth flow for MCP clients.
				scheme := "https"
				if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
					scheme = "http"
				}
				resourceMetadata := fmt.Sprintf("%s://%s/.well-known/oauth-protected-resource", scheme, r.Host)
				w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata="%s"`, resourceMetadata))
				WriteError(w, http.StatusUnauthorized, "MISSING_CREDENTIALS", "API key or Bearer token is required")
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

			keyID := pgconv.FormatUUID(apiKey.ID)
			ctx := service.ContextWithAPIKey(r.Context(), keyID, apiKey.Name)
			ctx = SetAPIKey(ctx, apiKey)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}

func writeWWWAuthenticate(w http.ResponseWriter, status int, errorCode, description string) {
	w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer error="%s", error_description="%s"`, errorCode, description))
	WriteError(w, status, "INVALID_TOKEN", description)
}
