//go:build !headless && !lite

package webapp

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"breadbox/internal/service"
	"breadbox/internal/webapp/pages"
)

// home is the authenticated overview landing: a real dashboard composed from the
// same service methods the SPA/MCP use — net position, recent spending, a
// spending-by-category breakdown, and recent activity. Multi-currency safe
// (balances are bucketed by currency; we surface the dominant bucket and never
// sum across iso_currency_code).
func (h *Handler) home(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	svc := h.app.Service

	vm := pages.HomeView{}

	// Dataset scope + freshness (transaction count, errored connections, etc.).
	if stats, err := svc.GetOverviewStats(ctx); err == nil {
		vm.TransactionCount = stats.Scope.TransactionCount
		vm.NeedsReviewCount = stats.Backlog.NeedsReviewCount
		vm.ErroredConnections = stats.Freshness.ErroredConnectionCount
		if stats.Freshness.LastSyncAt != nil {
			vm.LastSyncAt = *stats.Freshness.LastSyncAt
		}
	}

	// Balances → net position bucketed by currency (depository minus credit/loan).
	// Dependent-linked accounts are excluded from totals, matching the SPA/MCP.
	if accounts, err := svc.ListAccounts(ctx, nil); err == nil {
		vm.FillBalances(accounts)
	}

	// Current-month spending-by-category (SpendingOnly = positive amounts = money
	// out). GroupBy=category already excludes dependent-linked rows and pending.
	monthStart := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.UTC)
	if cat, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
		StartDate:    &monthStart,
		GroupBy:      "category",
		SpendingOnly: true,
	}); err == nil {
		vm.FillSpending(cat)
	}

	// Recent activity: latest transactions (default sort = date desc).
	if list, err := svc.ListTransactions(ctx, service.TransactionListParams{Limit: 8}); err == nil {
		vm.Recent = list.Transactions
	}

	render(w, r, http.StatusOK, pages.Home(h.shellData(r, "Home"), vm))
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
