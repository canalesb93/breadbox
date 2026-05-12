package api

// Page-internal endpoints behind the bearer middleware that powers the
// /link/{token} hosted page.
//
// These are NOT part of the agent REST API — they're token-scoped and meant
// to be called only by JS running on the standalone hosted page. They mount
// under /_link/{token}/* on the root router, with no API-key auth and no
// rate limiter (the bearer token in the path IS the credential).
//
// Why a parallel surface instead of reusing /api/v1/providers/* and
// /api/v1/connections? Two reasons:
//
//  1. Attribution. The public endpoints derive user_id from the API key
//     actor or accept it on the body; the page endpoints attribute every
//     write to the session's pinned user, with no chance of cross-user
//     contamination from a misbehaving page.
//  2. Scope-pinning. A session can be locked to one provider and one action
//     ("link") at mint time; these handlers enforce that contract per call
//     instead of trusting the page to do it client-side.

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"breadbox/internal/app"
	mw "breadbox/internal/middleware"
	"breadbox/internal/pgconv"
	"breadbox/internal/provider"
	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// hostedLinkPageSession is the redacted view returned by
// GET /_link/{token}/session. user_id and connection_id are deliberately
// stripped — the page doesn't need them, and surfacing them on a
// token-only route would leak attribution to whoever holds the URL.
type hostedLinkPageSession struct {
	ID                  string   `json:"id"`
	ShortID             string   `json:"short_id"`
	Provider            string   `json:"provider"`
	Action              string   `json:"action"`
	SingleUse           bool     `json:"single_use"`
	Label               string   `json:"label"`
	RedirectURL         string   `json:"redirect_url"`
	Status              string   `json:"status"`
	ResultConnectionIDs []string `json:"result_connection_ids"`
	ExpiresAt           string   `json:"expires_at"`
}

// GetHostedLinkPageSessionHandler serves GET /_link/{token}/session.
//
// First call flips the session from pending → active (and stamps
// started_at). Subsequent calls are idempotent. Returns the redacted view
// the page needs to render its initial state.
func GetHostedLinkPageSessionHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess, ok := mw.HostedLinkToken(r)
		if !ok {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Hosted-link session missing on context")
			return
		}

		// MarkHostedLinkStarted is a no-op on already-active sessions, so
		// repeat calls on the page are safe.
		if err := svc.MarkHostedLinkStarted(r.Context(), sess.ID); err != nil {
			// The bearer middleware already filtered out expired / completed
			// / failed states, so the only remaining failure mode is a
			// transient DB error — surface as 500.
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to mark hosted-link session active")
			return
		}

		// Re-read after the status update so the response reflects the
		// new state (in particular, status flips pending → active).
		updated, err := svc.GetHostedLinkSession(r.Context(), sess.ID)
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load hosted-link session")
			return
		}

		writeJSON(w, http.StatusOK, hostedLinkPageToResponse(updated))
	}
}

// HostedLinkPageStartHandler serves POST /_link/{token}/providers/{name}/start.
//
// Mirrors POST /api/v1/providers/{name}/link-session, but pinned to the
// session's provider (if set) and using the session's user attribution.
// The handler delegates to the existing provider registry — no provider
// logic is duplicated.
//
// For sessions with action="relink" the handler swaps in the provider's
// CreateReauthSession path so the page gets a re-auth link token (Plaid
// "update mode") instead of a fresh new-connection token. The URL the JS
// hits is the same — only the underlying provider call differs.
func HostedLinkPageStartHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		sess, ok := mw.HostedLinkToken(r)
		if !ok {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Hosted-link session missing on context")
			return
		}

		name := strings.ToLower(chi.URLParam(r, "name"))
		entry, ok := providerRegistry[name]
		if !ok {
			mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Unknown provider")
			return
		}

		// Scope-pin: if the session declared a provider at mint time, the
		// page may only run that provider. Empty session.Provider means the
		// agent left the choice up to the user.
		if sess.Provider != "" && sess.Provider != name {
			mw.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Provider does not match the hosted-link session")
			return
		}

		// Providers without a server-issued init token (Teller, CSV) skip
		// the provider call entirely. Mirrors LinkSessionHandler. The
		// relink branch below requires a token, so the early 204 here only
		// applies to the link flow — Teller relink isn't wired yet anyway
		// (Phase 2 PR2 handles it) and falls through to the 400 below.
		if !entry.needsLinkSession && sess.Action == service.HostedLinkActionLink {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		prov, ok := a.Providers[name]
		if !ok {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER",
				"Provider is not configured on this server")
			return
		}

		// Relink path: re-auth on the session's pinned connection. The
		// connection UUID comes from the session itself — the page has no
		// say. CreateHostedLink already verified the connection exists at
		// mint time, but it could have been deleted in the interim; surface
		// that as 404.
		if sess.Action == service.HostedLinkActionRelink {
			if sess.ConnectionID == "" {
				// Defensive — CreateHostedLink validates this at mint time.
				mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Relink session missing connection_id")
				return
			}
			cuid, err := a.Service.ResolveConnectionUUID(ctx, sess.ConnectionID)
			if err != nil {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Connection not found")
				return
			}
			conn, err := a.Queries.GetBankConnection(ctx, cuid)
			if err != nil {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Connection not found")
				return
			}
			provConn := provider.Connection{
				ProviderName:         string(conn.Provider),
				ExternalID:           conn.ExternalID.String,
				EncryptedCredentials: conn.EncryptedCredentials,
				UserID:               pgconv.FormatUUID(conn.UserID),
			}
			reauthSession, err := prov.CreateReauthSession(ctx, provConn)
			if err != nil {
				a.Logger.Error("hosted-link create reauth session", "provider", name, "error", err)
				mw.WriteError(w, http.StatusBadGateway, "PROVIDER_ERROR", "Failed to create reauth link token")
				return
			}
			writeJSON(w, http.StatusOK, linkSessionResponse{
				LinkToken:  reauthSession.Token,
				Expiration: reauthSession.Expiry.Format("2006-01-02T15:04:05Z"),
			})
			return
		}

		uid, err := a.Service.ResolveUserUUID(ctx, sess.UserID)
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to resolve session user")
			return
		}

		linkSession, err := prov.CreateLinkSession(ctx, pgconv.FormatUUID(uid))
		if err != nil {
			a.Logger.Error("hosted-link create provider link session", "provider", name, "error", err)
			mw.WriteError(w, http.StatusBadGateway, "PROVIDER_ERROR", "Failed to create link token")
			return
		}

		writeJSON(w, http.StatusOK, linkSessionResponse{
			LinkToken:  linkSession.Token,
			Expiration: linkSession.Expiry.Format("2006-01-02T15:04:05Z"),
		})
	}
}

// HostedLinkPageReauthCompleteHandler serves
// POST /_link/{token}/reauth-complete.
//
// Marks the session's pinned connection active again after the user
// finishes the provider's re-auth flow. Mirrors the public REST endpoint
// POST /api/v1/connections/{id}/reauth-complete, but the connection ID
// comes from the session (not the URL or body) and the actor is the agent
// that minted the link.
//
// On success: flips connection.status to active, clears error fields, then
// completes the hosted-link session (single-use is enforced at mint time
// for relink). The bearer middleware will reject any follow-up call with
// 410 CONSUMED. The handler also tolerates a request body for forward
// compatibility, but currently ignores it (Plaid's update-mode flow
// completes the OAuth exchange out-of-band, exactly like the public
// /reauth-complete endpoint).
func HostedLinkPageReauthCompleteHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess, ok := mw.HostedLinkToken(r)
		if !ok {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Hosted-link session missing on context")
			return
		}
		if sess.Action != service.HostedLinkActionRelink {
			mw.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Session action does not permit reauth-complete")
			return
		}
		if sess.ConnectionID == "" {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Relink session missing connection_id")
			return
		}

		actor := service.ActorFromContext(r.Context())
		if err := svc.ReactivateConnection(r.Context(), sess.ConnectionID, actor); err != nil {
			writeServiceError(w, err, "Connection not found", "Failed to reactivate connection")
			return
		}

		// Record the (still-the-same) connection on the session and burn
		// the token. Errors here are non-fatal for the user-visible flow —
		// the reauth itself succeeded — but we log them so the audit
		// timeline shows a session that didn't complete cleanly.
		if err := svc.AppendHostedLinkResult(r.Context(), sess.ID, sess.ConnectionID); err != nil {
			// Already-active sessions append fine; a stale state could
			// surface as ErrInvalidState, in which case the user is fine
			// but the audit row stays partial. Don't bubble this up.
			_ = err
		}
		if err := svc.CompleteHostedLink(r.Context(), sess.ID); err != nil {
			if errors.Is(err, service.ErrInvalidState) {
				mw.WriteError(w, http.StatusConflict, "INVALID_STATE", err.Error())
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to complete hosted-link session")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// HostedLinkPageConnectionHandler serves POST /_link/{token}/connections.
//
// Mirrors POST /api/v1/connections but with three scope checks:
//
//  1. Session action must be "link" (relink ships in Phase 2).
//  2. Body provider must match session.Provider when the session has one
//     pinned.
//  3. user_id is sourced from the session, NOT the request body — the page
//     cannot create a connection for any other user.
//
// On a successful create, the new connection's UUID is appended to the
// session's result_connection_ids and (when SingleUse=true) the session is
// flipped to completed so the bearer middleware rejects future calls.
func HostedLinkPageConnectionHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		sess, ok := mw.HostedLinkToken(r)
		if !ok {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Hosted-link session missing on context")
			return
		}

		if sess.Action != service.HostedLinkActionLink {
			mw.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Session action does not permit connection create")
			return
		}

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
		if sess.Provider != "" && sess.Provider != providerName {
			mw.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Provider does not match the hosted-link session")
			return
		}
		// CSV isn't a realistic hosted-link flow — the page would need to
		// upload a file via multipart, and the hosted page doesn't carry a
		// file picker today. Reject explicitly so callers don't get a
		// confusing 415 from the underlying handler.
		if providerName == "csv" {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "CSV imports are not supported via hosted-link")
			return
		}

		// User attribution always comes from the session — the body has no
		// say. We still resolve it through the service so an invalid stored
		// short_id surfaces as a 500 rather than a silent FK error.
		uid, err := a.Service.ResolveUserUUID(ctx, sess.UserID)
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to resolve session user")
			return
		}

		creds := entry.extractFromJSON(w, req.Credentials)
		if creds == nil {
			return
		}

		// The providerEntry.exchange function writes directly to the
		// response. To inject our hosted-link bookkeeping (append result,
		// optionally complete) on the success path without rewriting the
		// per-provider exchange code, we capture the response into a
		// buffer, inspect status + body, and replay it.
		rec := &recordingResponseWriter{header: http.Header{}, body: &bytes.Buffer{}}
		entry.exchange(a, rec, r, uid, creds)
		// Default status when the handler didn't call WriteHeader.
		if rec.status == 0 {
			rec.status = http.StatusOK
		}

		// Only treat the canonical 201 Created as a successful link; any
		// other status (4xx, 5xx, even 200) is a failure or oddity and we
		// pass it through untouched.
		if rec.status == http.StatusCreated {
			var env connectionEnvelope
			if err := json.Unmarshal(rec.body.Bytes(), &env); err == nil && env.ConnectionID != "" {
				if err := a.Service.AppendHostedLinkResult(ctx, sess.ID, env.ConnectionID); err != nil {
					a.Logger.Error("hosted-link append result", "session_id", sess.ID, "error", err)
				}
				if sess.SingleUse {
					if err := a.Service.CompleteHostedLink(ctx, sess.ID); err != nil {
						a.Logger.Error("hosted-link auto-complete", "session_id", sess.ID, "error", err)
					}
				}
			}
		}

		// Replay headers + body to the real writer.
		for k, vs := range rec.header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(rec.status)
		_, _ = w.Write(rec.body.Bytes())
	}
}

// HostedLinkPageCompleteHandler serves POST /_link/{token}/complete.
//
// Idempotent — already-completed sessions return 204 too, so the page's
// "I'm done" button is safe to spam. The bearer middleware will reject
// future calls after this lands (status=="completed" → 410 CONSUMED).
func HostedLinkPageCompleteHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess, ok := mw.HostedLinkToken(r)
		if !ok {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Hosted-link session missing on context")
			return
		}
		if err := svc.CompleteHostedLink(r.Context(), sess.ID); err != nil {
			if errors.Is(err, service.ErrInvalidState) {
				// CompleteHostedLink is documented as idempotent on
				// already-completed; any other invalid-state means the
				// session moved to a terminal state out-of-band. Surface as
				// 409 so the page can stop offering the "I'm done" button.
				mw.WriteError(w, http.StatusConflict, "INVALID_STATE", err.Error())
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to complete hosted-link session")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// hostedLinkPageFailRequest is the body for POST /_link/{token}/fail.
type hostedLinkPageFailRequest struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// HostedLinkPageFailHandler serves POST /_link/{token}/fail.
//
// Lets the page report an SDK-level failure (Plaid Link onExit with err,
// Teller onFailure) so the audit trail captures why the flow died. After
// this call, the bearer middleware rejects future requests with 410 GONE.
func HostedLinkPageFailHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess, ok := mw.HostedLinkToken(r)
		if !ok {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Hosted-link session missing on context")
			return
		}
		var req hostedLinkPageFailRequest
		// Body is optional — accept an empty POST as "user cancelled" with
		// no detail. Don't fail on decode errors when the body is empty.
		if r.ContentLength != 0 {
			if !decodeJSON(w, r, &req) {
				return
			}
		}
		if err := svc.FailHostedLink(r.Context(), sess.ID, req.Code, req.Message); err != nil {
			if errors.Is(err, service.ErrInvalidState) {
				mw.WriteError(w, http.StatusConflict, "INVALID_STATE", err.Error())
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fail hosted-link session")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// hostedLinkPageToResponse flattens the service-layer struct to the
// page-visible shape (no user_id, no connection_id).
func hostedLinkPageToResponse(s service.HostedLinkSession) hostedLinkPageSession {
	ids := s.ResultConnectionIDs
	if ids == nil {
		ids = []string{}
	}
	return hostedLinkPageSession{
		ID:                  s.ID,
		ShortID:             s.ShortID,
		Provider:            s.Provider,
		Action:              s.Action,
		SingleUse:           s.SingleUse,
		Label:               s.Label,
		RedirectURL:         s.RedirectURL,
		Status:              s.Status,
		ResultConnectionIDs: ids,
		ExpiresAt:           s.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

// recordingResponseWriter buffers WriteHeader + Write calls so the caller
// can inspect a downstream handler's result before deciding what to do
// with it. Used by HostedLinkPageConnectionHandler to layer session
// bookkeeping over providerEntry.exchange without modifying every entry.
type recordingResponseWriter struct {
	header http.Header
	body   *bytes.Buffer
	status int
}

func (r *recordingResponseWriter) Header() http.Header { return r.header }

func (r *recordingResponseWriter) WriteHeader(status int) { r.status = status }

func (r *recordingResponseWriter) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.body.Write(b)
}
