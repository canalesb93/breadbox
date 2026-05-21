//go:build !headless && !lite

package webapp

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"breadbox/internal/service"
	"breadbox/internal/webapp/pages"
)

// home is the authenticated overview landing. Full overview content lands in Phase 2;
// for now it confirms the shell + auth + nav loop works end to end.
func (h *Handler) home(w http.ResponseWriter, r *http.Request) {
	render(w, r, http.StatusOK, pages.Home(h.shellData(r, "Home")))
}

// accountsList renders all accounts (the first ported read surface).
func (h *Handler) accountsList(w http.ResponseWriter, r *http.Request) {
	accounts, err := h.app.Service.ListAccounts(r.Context(), nil)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	render(w, r, http.StatusOK, pages.AccountsList(h.shellData(r, "Accounts"), accounts))
}

// accountDetail renders one account with recent transactions.
func (h *Handler) accountDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	detail, err := h.app.Service.GetAccountDetailResponse(r.Context(), id, 25)
	if errors.Is(err, service.ErrNotFound) {
		h.notFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	title := detail.Name
	if detail.DisplayName != nil && *detail.DisplayName != "" {
		title = *detail.DisplayName
	}
	render(w, r, http.StatusOK, pages.AccountDetail(h.shellData(r, title), detail))
}

// notFound renders a 404 inside the app shell so nav stays usable.
func (h *Handler) notFound(w http.ResponseWriter, r *http.Request) {
	render(w, r, http.StatusNotFound, pages.ErrorPage(h.shellData(r, "Not found"), "404", "We couldn't find that page."))
}

// serverError logs and renders a 500 inside the app shell.
func (h *Handler) serverError(w http.ResponseWriter, r *http.Request, err error) {
	h.app.Logger.Error("webapp: server error", "path", r.URL.Path, "error", err)
	render(w, r, http.StatusInternalServerError, pages.ErrorPage(h.shellData(r, "Error"), "500", "Something went wrong on our end."))
}
