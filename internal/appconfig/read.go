// Package appconfig provides typed read helpers for the app_config DB table.
//
// The service, admin, sync, and command layers all need to read individual
// app_config rows with defaults. Each caller was repeating the same
// queries.GetAppConfig + row.Value.Valid + empty-string + parse dance. These
// helpers centralize that pattern so the rest of the codebase can ask for a
// string, bool, or int with a default in one call.
//
// For startup-time config loading (env + DB merge into a single Config
// struct), see internal/config — that package is concerned with process
// boot; this one is concerned with runtime reads of individual keys.
package appconfig

import (
	"context"
	"strconv"

	"breadbox/internal/db"
)

// Reader is the minimal query surface used by these helpers. *db.Queries
// satisfies it; tests and callers with a narrower type can implement their
// own.
type Reader interface {
	GetAppConfig(ctx context.Context, key string) (db.AppConfig, error)
}

// Read returns the raw string value stored at key and whether a usable value
// was found. A missing row, a row with a NULL value, or a query error all
// return ("", false).
func Read(ctx context.Context, r Reader, key string) (string, bool) {
	row, err := r.GetAppConfig(ctx, key)
	if err != nil || !row.Value.Valid {
		return "", false
	}
	return row.Value.String, true
}

// String returns the value at key, or def when missing or empty.
func String(ctx context.Context, r Reader, key, def string) string {
	v, ok := Read(ctx, r, key)
	if !ok || v == "" {
		return def
	}
	return v
}

// Bool returns true when the value at key is the literal string "true".
// Missing keys, NULL values, and any other string return def. The strict
// match mirrors the values the admin UI writes — widening this to accept
// "1"/"yes"/etc. would change behavior elsewhere.
func Bool(ctx context.Context, r Reader, key string, def bool) bool {
	v, ok := Read(ctx, r, key)
	if !ok {
		return def
	}
	return v == "true"
}

// Int returns the parsed integer value at key, or def when the key is
// missing, empty, or unparseable. No range clamping — callers that reject
// specific values (negative, zero) apply their own check after the call.
func Int(ctx context.Context, r Reader, key string, def int) int {
	v, ok := Read(ctx, r, key)
	if !ok || v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
