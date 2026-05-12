package api

import (
	"errors"
	"net/http"
	"time"

	mw "breadbox/internal/middleware"
	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// createHostedLinkRequest is the JSON body for POST /api/v1/connections/link.
//
// `user_id` accepts either a canonical UUID or an 8-char short_id. `provider`
// is optional — empty value lets the hosted page present a picker.
// `expires_in_seconds` is clamped by the service to [0, 3600]; 0 picks the
// default (15m).
type createHostedLinkRequest struct {
	UserID           string `json:"user_id"`
	Provider         string `json:"provider"`
	SingleUse        bool   `json:"single_use"`
	RedirectURL      string `json:"redirect_url"`
	Label            string `json:"label"`
	ExpiresInSeconds *int   `json:"expires_in_seconds"`
}

// hostedLinkSessionResponse is the JSON view of a hosted_link_sessions row
// returned by GET /connections/link/{id}. It deliberately omits the plaintext
// token — that is returned exactly once by the create endpoint.
type hostedLinkSessionResponse struct {
	ID                  string     `json:"id"`
	ShortID             string     `json:"short_id"`
	UserID              string     `json:"user_id"`
	Provider            string     `json:"provider"`
	Action              string     `json:"action"`
	ConnectionID        string     `json:"connection_id,omitempty"`
	SingleUse           bool       `json:"single_use"`
	RedirectURL         string     `json:"redirect_url"`
	Label               string     `json:"label"`
	Status              string     `json:"status"`
	ErrorCode           string     `json:"error_code,omitempty"`
	ErrorMessage        string     `json:"error_message,omitempty"`
	ResultConnectionIDs []string   `json:"result_connection_ids"`
	ExpiresAt           time.Time  `json:"expires_at"`
	StartedAt           *time.Time `json:"started_at,omitempty"`
	CompletedAt         *time.Time `json:"completed_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
}

// createHostedLinkResponse mirrors hostedLinkSessionResponse with the
// one-time-only `token` and the constructed `url` field. Returned only from
// POST /connections/link — never from the poll endpoint.
type createHostedLinkResponse struct {
	hostedLinkSessionResponse
	Token string `json:"token"`
	URL   string `json:"url"`
}

// createHostedLinkRelinkRequest is the JSON body for
// POST /api/v1/connections/{id}/relink. The connection identity comes from
// the URL path, so neither `user_id` nor `provider` is accepted on the body
// — both are derived from the connection row. `single_use` is implicit and
// always true (re-auth is one-shot).
type createHostedLinkRelinkRequest struct {
	RedirectURL      string `json:"redirect_url"`
	Label            string `json:"label"`
	ExpiresInSeconds *int   `json:"expires_in_seconds"`
}

// CreateHostedLinkRelinkHandler serves POST /api/v1/connections/{id}/relink.
//
// Mints a re-auth hosted-link session pinned to one existing connection.
// The session is always `action="relink"`, `single_use=true`, and `provider`
// is sourced from the connection row — the request body has no say over any
// of those. The user attribution is the connection's owner; we never accept
// `user_id` on the body.
//
// Disconnected connections cannot be re-authenticated — re-auth on a soft-
// deleted connection makes no sense — so we surface `409 CONFLICT` in that
// case via service.ErrInvalidState. Unknown connection IDs return 404.
//
// Requires `full_access` scope.
func CreateHostedLinkRelinkHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var req createHostedLinkRelinkRequest
		// Body is optional — accept an empty POST.
		if r.ContentLength != 0 {
			if !decodeJSON(w, r, &req) {
				return
			}
		}
		if req.ExpiresInSeconds != nil {
			if *req.ExpiresInSeconds < 0 || *req.ExpiresInSeconds > 3600 {
				mw.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR",
					"expires_in_seconds must be between 0 and 3600")
				return
			}
		}

		var ttl time.Duration
		if req.ExpiresInSeconds != nil {
			ttl = time.Duration(*req.ExpiresInSeconds) * time.Second
		}

		actor := service.ActorFromContext(r.Context())
		result, err := svc.CreateHostedLinkRelink(r.Context(), service.CreateHostedLinkRelinkParams{
			ConnectionID: id,
			RedirectURL:  req.RedirectURL,
			Label:        req.Label,
			TTL:          ttl,
			Actor:        actor,
		})
		if err != nil {
			// ErrInvalidState — connection is disconnected. The generic
			// writeServiceError helper doesn't know about ErrInvalidState
			// (it's deliberately scoped to bounded sentinels), so map it
			// here to a 409 with the connection-specific code.
			if errors.Is(err, service.ErrInvalidState) {
				mw.WriteError(w, http.StatusConflict, "CONNECTION_DISCONNECTED",
					"Cannot mint a re-auth link for a disconnected connection")
				return
			}
			writeServiceError(w, err, "Connection not found", "Failed to create hosted link")
			return
		}

		writeJSON(w, http.StatusCreated, createHostedLinkResponse{
			hostedLinkSessionResponse: hostedLinkSessionToResponse(result.Session),
			Token:                     result.Token,
			URL:                       buildHostedLinkURL(r, result.Token),
		})
	}
}

// CreateHostedLinkHandler serves POST /api/v1/connections/link.
//
// Mints a new hosted-link session and returns the URL the agent should hand
// the end-user, plus the plaintext bearer token (one-time-only on this
// response — never echoed back by the poll endpoint).
//
// Action is fixed to "link" on this endpoint — the relink flow is exposed
// separately via POST /connections/{id}/relink (Phase 2).
//
// Requires `full_access` scope.
func CreateHostedLinkHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createHostedLinkRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.UserID == "" {
			mw.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "user_id is required")
			return
		}
		switch req.Provider {
		case "", "plaid", "teller":
		default:
			mw.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR",
				"provider must be \"plaid\" or \"teller\"")
			return
		}
		if req.ExpiresInSeconds != nil {
			if *req.ExpiresInSeconds < 0 || *req.ExpiresInSeconds > 3600 {
				mw.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR",
					"expires_in_seconds must be between 0 and 3600")
				return
			}
		}

		var ttl time.Duration
		if req.ExpiresInSeconds != nil {
			ttl = time.Duration(*req.ExpiresInSeconds) * time.Second
		}

		actor := service.ActorFromContext(r.Context())
		result, err := svc.CreateHostedLink(r.Context(), service.CreateHostedLinkParams{
			UserID:      req.UserID,
			Provider:    req.Provider,
			Action:      service.HostedLinkActionLink,
			SingleUse:   req.SingleUse,
			RedirectURL: req.RedirectURL,
			Label:       req.Label,
			TTL:         ttl,
			Actor:       actor,
		})
		if err != nil {
			writeServiceError(w, err, "User not found", "Failed to create hosted link")
			return
		}

		writeJSON(w, http.StatusCreated, createHostedLinkResponse{
			hostedLinkSessionResponse: hostedLinkSessionToResponse(result.Session),
			Token:                     result.Token,
			URL:                       buildHostedLinkURL(r, result.Token),
		})
	}
}

// GetHostedLinkSessionHandler serves GET /api/v1/connections/link/{id}.
//
// `{id}` is either the UUID or the 8-char short_id. The response shape
// mirrors the create response without the `token` or `url` fields — the
// plaintext token is only ever returned at creation time.
//
// Read-scope endpoint; any valid API key may poll.
func GetHostedLinkSessionHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		session, err := svc.GetHostedLinkSession(r.Context(), id)
		if err != nil {
			writeServiceError(w, err, "Hosted link session not found", "Failed to get hosted link session")
			return
		}

		writeJSON(w, http.StatusOK, hostedLinkSessionToResponse(session))
	}
}

// hostedLinkSessionToResponse flattens the service-layer struct to the JSON
// shape, normalizing nil slices to an empty array so `result_connection_ids`
// is always present.
func hostedLinkSessionToResponse(s service.HostedLinkSession) hostedLinkSessionResponse {
	ids := s.ResultConnectionIDs
	if ids == nil {
		ids = []string{}
	}
	return hostedLinkSessionResponse{
		ID:                  s.ID,
		ShortID:             s.ShortID,
		UserID:              s.UserID,
		Provider:            s.Provider,
		Action:              s.Action,
		ConnectionID:        s.ConnectionID,
		SingleUse:           s.SingleUse,
		RedirectURL:         s.RedirectURL,
		Label:               s.Label,
		Status:              s.Status,
		ErrorCode:           s.ErrorCode,
		ErrorMessage:        s.ErrorMessage,
		ResultConnectionIDs: ids,
		ExpiresAt:           s.ExpiresAt,
		StartedAt:           s.StartedAt,
		CompletedAt:         s.CompletedAt,
		CreatedAt:           s.CreatedAt,
	}
}

// buildHostedLinkURL constructs the absolute URL that the end-user should
// open to run the hosted-link flow. The scheme is derived from the request
// (X-Forwarded-Proto if a proxy added it, else r.TLS), and the host comes
// from r.Host so multi-host deployments work without server-side config.
func buildHostedLinkURL(r *http.Request, token string) string {
	scheme := "http"
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	} else if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/link/" + token
}
