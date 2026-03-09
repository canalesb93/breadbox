package middleware

import (
	"context"

	"breadbox/internal/db"
)

type contextKey string

const apiKeyContextKey contextKey = "api_key"

// SetAPIKey stores the full API key record in the request context.
func SetAPIKey(ctx context.Context, key *db.ApiKey) context.Context {
	return context.WithValue(ctx, apiKeyContextKey, key)
}

// GetAPIKey retrieves the API key record from the request context.
func GetAPIKey(ctx context.Context) *db.ApiKey {
	key, _ := ctx.Value(apiKeyContextKey).(*db.ApiKey)
	return key
}
