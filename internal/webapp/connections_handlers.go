//go:build !headless && !lite

package webapp

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"breadbox/internal/service"
	"breadbox/internal/webapp/pages"
)

// registerConnections wires the connection read surfaces onto the authenticated
// /app subrouter. Every page is a real document — no client router. Mutating
// routes (sync-all, reconnect) are POSTs guarded by requireSameOrigin.
func (h *Handler) registerConnections(r chi.Router) {
	r.Get("/connections", h.connectionsList)
	r.Post("/connections/sync-all", h.requireSameOrigin(h.connectionsSyncAll))
	r.Get("/connections/{id}", h.connectionDetail)
	r.Post("/connections/{id}/reconnect", h.requireSameOrigin(h.connectionReconnect))
}

// connectionsList renders every bank connection as a card grid. Supports an
// optional ?user_id= filter that scopes the list to one household member; the
// page renders a tab bar of members + "All" so the filter is a real link nav.
func (h *Handler) connectionsList(w http.ResponseWriter, r *http.Request) {
	var userID *string
	if uid := r.URL.Query().Get("user_id"); uid != "" {
		userID = &uid
	}

	conns, err := h.app.Service.ListConnections(r.Context(), userID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}

	// Household members for the filter tab bar. A failure here is non-fatal —
	// the list still renders, just without member tabs.
	members, err := h.app.Service.ListUsers(r.Context())
	if err != nil {
		h.app.Logger.Warn("webapp: list users for connection filter", "error", err)
		members = nil
	}

	data := pages.ConnectionsListData{
		Shell:        h.shellData(r, "Connections"),
		Connections:  conns,
		Members:      members,
		ActiveUserID: r.URL.Query().Get("user_id"),
		Synced:       r.URL.Query().Get("synced") == "1",
	}
	render(w, r, http.StatusOK, pages.ConnectionsList(data))
}

// connectionsSyncAll kicks off a sync of every connection and redirects back to
// the list with a flash flag. TriggerSync(nil) launches SyncEngine.SyncAll on a
// background context internally, so the request returns immediately — we never
// block on the (potentially long) sync, mirroring the REST TriggerSyncHandler.
func (h *Handler) connectionsSyncAll(w http.ResponseWriter, r *http.Request) {
	if err := h.app.Service.TriggerSync(r.Context(), nil); err != nil {
		h.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/app/connections?synced=1", http.StatusSeeOther)
}

// connectionDetail renders one connection's header, detail grid, and accounts.
func (h *Handler) connectionDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	detail, err := h.app.Service.GetConnection(r.Context(), id)
	if errors.Is(err, service.ErrNotFound) {
		h.notFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	title := "Connection"
	if detail.InstitutionName != nil && *detail.InstitutionName != "" {
		title = *detail.InstitutionName
	}
	render(w, r, http.StatusOK, pages.ConnectionDetail(h.shellData(r, title), detail))
}

// connectionReconnect mints a single-use re-auth hosted-link session pinned to
// this connection and redirects the browser to the standalone /link/{token}
// page. That page runs the provider SDK (Plaid Link / Teller Connect) in the
// browser — the only place the OAuth widget can actually run — so we delegate to
// it rather than minting a bare link token the v3 app has no way to consume.
//
// CreateHostedLinkRelink derives provider + owner from the connection row and
// always sets action="relink", single_use=true. A disconnected connection
// yields ErrInvalidState (re-auth on a soft-deleted connection is meaningless);
// we bounce back to the detail page in that case rather than 500.
func (h *Handler) connectionReconnect(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	result, err := h.app.Service.CreateHostedLinkRelink(r.Context(), service.CreateHostedLinkRelinkParams{
		ConnectionID: id,
		RedirectURL:  webappReconnectRedirectURL(r, id),
		Actor:        service.ActorFromContext(r.Context()),
	})
	if errors.Is(err, service.ErrNotFound) {
		h.notFound(w, r)
		return
	}
	if errors.Is(err, service.ErrInvalidState) {
		http.Redirect(w, r, "/app/connections/"+id, http.StatusSeeOther)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}

	http.Redirect(w, r, webappHostedLinkURL(r, result.Token), http.StatusSeeOther)
}

// webappHostedLinkURL builds the absolute URL to the standalone hosted-link page
// for a token. Mirrors api.buildHostedLinkURL — honors X-Forwarded-Proto behind
// a TLS-terminating proxy, falls back to r.TLS, then plain http.
func webappHostedLinkURL(r *http.Request, token string) string {
	return webappScheme(r) + "://" + r.Host + "/link/" + token
}

// webappReconnectRedirectURL is where the hosted-link page sends the browser
// after a successful relink — straight back to the connection's detail page.
func webappReconnectRedirectURL(r *http.Request, id string) string {
	return webappScheme(r) + "://" + r.Host + "/app/connections/" + id
}

func webappScheme(r *http.Request) string {
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		return proto
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}
