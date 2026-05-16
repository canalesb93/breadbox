//go:build !lite

package api

// Generic provider registry + dispatch for the public REST API.
//
// This file collapses the per-provider endpoint sprawl
// (POST /connections/plaid/link-token, /plaid/exchange, /teller,
// /csv/import, ...) into a self-describing surface:
//
//   GET  /api/v1/providers                         — list with capabilities + credential schema
//   GET  /api/v1/providers/{name}                  — single provider entry
//   POST /api/v1/providers/{name}/link-session     — generic link-token start (204 for providers without one)
//   POST /api/v1/connections                       — provider-discriminated create
//
// Adding a new provider (MoneyKit, Salt Edge, Tink, ...) becomes a pure
// backend change: register a new entry in providerRegistry and the four
// generic endpoints surface it automatically. The old per-provider routes
// remain wired as deprecated shims (see connections_plaid_link.go,
// connections_teller_setup.go, csv_import.go) so existing clients —
// including the admin UI's wizard — keep working.

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"breadbox/internal/app"
	mw "breadbox/internal/middleware"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// CredentialField is the hand-authored JSON-schema-ish description of one
// field in a provider's credentials blob. It mirrors the slimmest useful
// subset of JSON Schema: type + required + a free-form description. Keeping
// it tiny avoids pulling in a full schema library and keeps the response
// payload small enough for SDKs to consume directly.
type CredentialField struct {
	Type        string `json:"type"`                  // "string", "array", "object", "file"
	Required    any    `json:"required"`              // bool, or "alt" for "one of N alternatives"
	Description string `json:"description,omitempty"` // human-readable
}

// providerInfo is the wire shape of one entry in the GET /providers list.
type providerInfo struct {
	Name              string                     `json:"name"`
	Configured        bool                       `json:"configured"`
	NeedsLinkSession  bool                       `json:"needs_link_session"`
	Capabilities      []string                   `json:"capabilities"`
	CredentialsSchema map[string]CredentialField `json:"credentials_schema"`
}

// providerEntry is the dispatch-table row for one provider. It carries the
// static descriptor (name, capabilities, schema) plus the two function
// pointers the generic POST /connections handler needs:
//
//   - extractFromJSON parses an application/json body's `credentials` field
//     into the shape this provider's exchange step expects.
//   - extractFromMultipart parses a multipart/form-data body. Optional —
//     leave nil for providers that don't accept multipart (Plaid, Teller).
//   - exchange runs the actual provider call + service.RegisterNewConnection
//     (or service.ImportCSV for CSV). It returns the canonical envelope.
type providerEntry struct {
	name              string
	needsLinkSession  bool
	capabilities      []string
	credentialsSchema map[string]CredentialField
	// extractFromJSON parses the `credentials` object out of a JSON request.
	// On validation failure it writes the error envelope and returns nil.
	extractFromJSON func(w http.ResponseWriter, raw json.RawMessage) any
	// extractFromMultipart parses a multipart form. Optional. Returns nil
	// (and writes the error) when the body is missing required fields.
	extractFromMultipart func(w http.ResponseWriter, r *http.Request) any
	// exchange takes the parsed credentials + user UUID and persists the
	// connection. Writes the response on success or error.
	exchange func(a *app.App, w http.ResponseWriter, r *http.Request, uid pgtype.UUID, creds any)
}

// connectionEnvelope is the shared 201-Created shape across all providers.
// All three current handlers (Plaid, Teller, CSV) already converged on
// this shape; the generic endpoint cements it.
type connectionEnvelope struct {
	ConnectionID    string `json:"connection_id"`
	InstitutionName string `json:"institution_name"`
	Status          string `json:"status"`
}

// providerRegistry is keyed by provider name. The list order is preserved
// for GET /providers via providerOrder so the response is deterministic
// (helpful for clients that cache or diff the list).
var (
	providerOrder    = []string{"plaid", "teller", "csv"}
	providerRegistry = map[string]providerEntry{
		"plaid":  plaidEntry,
		"teller": tellerEntry,
		"csv":    csvEntry,
	}
)

// ---- GET /api/v1/providers ----

// ListProvidersHandler serves GET /api/v1/providers.
//
// Returns a bare JSON array (bounded resource per the API conventions in
// .claude/rules/api.md). Unconfigured providers still appear with
// `configured: false` so clients can render a "set me up" CTA without
// guessing what providers exist.
func ListProvidersHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out := make([]providerInfo, 0, len(providerOrder))
		for _, name := range providerOrder {
			entry, ok := providerRegistry[name]
			if !ok {
				continue
			}
			out = append(out, providerInfoFor(a, entry))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// GetProviderHandler serves GET /api/v1/providers/{name}.
func GetProviderHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.ToLower(chi.URLParam(r, "name"))
		entry, ok := providerRegistry[name]
		if !ok {
			mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Unknown provider")
			return
		}
		writeJSON(w, http.StatusOK, providerInfoFor(a, entry))
	}
}

func providerInfoFor(a *app.App, entry providerEntry) providerInfo {
	return providerInfo{
		Name:              entry.name,
		Configured:        isProviderConfigured(a, entry.name),
		NeedsLinkSession:  entry.needsLinkSession,
		Capabilities:      entry.capabilities,
		CredentialsSchema: entry.credentialsSchema,
	}
}

// isProviderConfigured mirrors the live-provider check used elsewhere
// (a.Providers[name] != nil) for Plaid/Teller, and is always-true for CSV
// because the CSV "provider" is just an import code path — no external
// credentials needed.
func isProviderConfigured(a *app.App, name string) bool {
	if name == "csv" {
		return true
	}
	_, ok := a.Providers[name]
	return ok
}

// ---- POST /api/v1/providers/{name}/link-session ----

type linkSessionRequest struct {
	UserID string `json:"user_id"`
}

type linkSessionResponse struct {
	LinkToken  string `json:"link_token"`
	Expiration string `json:"expiration"`
}

// LinkSessionHandler serves POST /api/v1/providers/{name}/link-session.
//
// Returns:
//   - 200 OK with {link_token, expiration} for providers that issue a token
//     (Plaid; Teller returns the server-configured application id here so the
//     SPA can bootstrap Teller Connect without a window global).
//   - 204 No Content for providers where the link flow is fully client-side
//     and no init token is needed (CSV).
//   - 404 NOT_FOUND for an unknown provider name.
//   - 400 INVALID_PARAMETER when the provider isn't configured.
func LinkSessionHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		name := strings.ToLower(chi.URLParam(r, "name"))
		entry, ok := providerRegistry[name]
		if !ok {
			mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Unknown provider")
			return
		}

		var req linkSessionRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.UserID == "" {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "user_id is required")
			return
		}

		uid, err := a.Service.ResolveUserUUID(ctx, req.UserID)
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "User not found")
				return
			}
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "Invalid user_id")
			return
		}

		// Providers that don't need an init token return 204. The client
		// proceeds straight to POST /connections with the credentials it
		// gathered from the provider's hosted UI.
		if !entry.needsLinkSession {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		prov, ok := a.Providers[name]
		if !ok {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER",
				"Provider is not configured on this server")
			return
		}

		session, err := prov.CreateLinkSession(ctx, pgconv.FormatUUID(uid))
		if err != nil {
			a.Logger.Error("create link session", "provider", name, "error", err)
			mw.WriteError(w, http.StatusBadGateway, "PROVIDER_ERROR", "Failed to create link token")
			return
		}

		writeJSON(w, http.StatusOK, linkSessionResponse{
			LinkToken:  session.Token,
			Expiration: session.Expiry.Format("2006-01-02T15:04:05Z"),
		})
	}
}

// ---- POST /api/v1/connections ----

// createConnectionRequest is the JSON shape for the generic create endpoint.
// `credentials` is left as raw JSON so we can hand it to the provider-specific
// extractor that knows the shape. Multipart bodies bypass this struct entirely.
type createConnectionRequest struct {
	Provider    string          `json:"provider"`
	UserID      string          `json:"user_id"`
	Credentials json.RawMessage `json:"credentials"`
}

// CreateConnectionHandler serves POST /api/v1/connections.
//
// Dispatches on `provider`. JSON bodies decode the `credentials` field via
// the provider's extractor; multipart bodies (CSV only today) read the raw
// form fields the existing csv_import handler already supports.
func CreateConnectionHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		contentType := r.Header.Get("Content-Type")
		mediaType := strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))

		switch mediaType {
		case "multipart/form-data":
			// Multipart bodies don't have a `provider` field as JSON — but
			// the client must still tell us which provider this body
			// targets. Accept it from a regular form field of the same name.
			if err := r.ParseMultipartForm(maxCSVRESTUploadSize); err != nil {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "failed to parse multipart form: "+err.Error())
				return
			}
			providerName := strings.ToLower(strings.TrimSpace(r.FormValue("provider")))
			if providerName == "" {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "provider is required")
				return
			}
			entry, ok := providerRegistry[providerName]
			if !ok {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Unknown provider")
				return
			}
			if entry.extractFromMultipart == nil {
				mw.WriteError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE",
					"provider does not accept multipart/form-data; use application/json")
				return
			}
			userIDRaw := r.FormValue("user_id")
			// Multipart-only providers may resolve the user themselves
			// (CSV has the single-user-fallback rule); pass an empty UUID
			// when no user_id is supplied and let extract handle it.
			var uid pgtype.UUID
			if userIDRaw != "" {
				var err error
				uid, err = a.Service.ResolveUserUUID(ctx, userIDRaw)
				if err != nil {
					if errors.Is(err, service.ErrNotFound) {
						mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "User not found")
						return
					}
					mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "Invalid user_id")
					return
				}
			}
			creds := entry.extractFromMultipart(w, r)
			if creds == nil {
				return
			}
			entry.exchange(a, w, r, uid, creds)
			return

		case "application/json", "":
			var req createConnectionRequest
			if !decodeJSON(w, r, &req) {
				return
			}
			providerName := strings.ToLower(strings.TrimSpace(req.Provider))
			if providerName == "" {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "provider is required")
				return
			}
			entry, ok := providerRegistry[providerName]
			if !ok {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Unknown provider")
				return
			}

			// CSV is the one provider that may resolve the user implicitly
			// (single-user household, or existing connection_id branch).
			// Skip user_id validation here so the CSV extractor can fall
			// through to its own resolver. Plaid/Teller require user_id.
			var uid pgtype.UUID
			if providerName != "csv" {
				if req.UserID == "" {
					mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "user_id is required")
					return
				}
				var err error
				uid, err = a.Service.ResolveUserUUID(ctx, req.UserID)
				if err != nil {
					if errors.Is(err, service.ErrNotFound) {
						mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "User not found")
						return
					}
					mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "Invalid user_id")
					return
				}
			} else if req.UserID != "" {
				// CSV with an explicit user_id still validates the UUID
				// up-front so callers get an early 404 on a missing user.
				var err error
				uid, err = a.Service.ResolveUserUUID(ctx, req.UserID)
				if err != nil {
					if errors.Is(err, service.ErrNotFound) {
						mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "User not found")
						return
					}
					mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "Invalid user_id")
					return
				}
			}

			creds := entry.extractFromJSON(w, req.Credentials)
			if creds == nil {
				return
			}
			entry.exchange(a, w, r, uid, creds)
			return

		default:
			mw.WriteError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE",
				"Content-Type must be application/json or multipart/form-data")
			return
		}
	}
}
