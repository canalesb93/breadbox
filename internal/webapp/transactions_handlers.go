//go:build !headless && !lite

package webapp

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"breadbox/internal/service"
	"breadbox/internal/webapp/pages"
)

// transactionsList renders the server-side data-table. All filters and pagination are
// query-param driven (native GET form + real <a href> page links) — no client router.
func (h *Handler) transactionsList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	const defaultLimit = 50
	limit := defaultLimit
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	offset := 0
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			offset = n
		}
	}

	// Normalize sort inputs to the values the service accepts. sort_by:
	// date (default) | amount; sort_order: desc (default) | asc. Anything else
	// falls back to the default so a hand-edited URL can't break the query.
	sortBy := q.Get("sort_by")
	if sortBy != "amount" {
		sortBy = "date"
	}
	sortOrder := q.Get("sort_order")
	if sortOrder != "asc" {
		sortOrder = "desc"
	}

	filters := pages.TxFilters{
		Search:    q.Get("search"),
		Account:   q.Get("account"),
		Category:  q.Get("category"),
		Start:     q.Get("start"),
		End:       q.Get("end"),
		MinAmount: q.Get("min_amount"),
		MaxAmount: q.Get("max_amount"),
		Pending:   q.Get("pending"),
		SortBy:    sortBy,
		SortOrder: sortOrder,
		Offset:    offset,
		Limit:     limit,
	}

	params := service.TransactionListParams{
		Offset:    offset,
		Limit:     limit,
		SortBy:    &sortBy,
		SortOrder: &sortOrder,
	}
	if filters.Search != "" {
		s := filters.Search
		params.Search = &s
	}
	if filters.Account != "" {
		a := filters.Account
		params.AccountID = &a
	}
	if filters.Category != "" {
		c := filters.Category
		params.CategorySlug = &c
	}
	if t, ok := parseTxnDate(filters.Start); ok {
		params.StartDate = &t
	}
	if t, ok := parseTxnDate(filters.End); ok {
		params.EndDate = &t
	}
	if v, err := strconv.ParseFloat(filters.MinAmount, 64); err == nil && filters.MinAmount != "" {
		params.MinAmount = &v
	}
	if v, err := strconv.ParseFloat(filters.MaxAmount, 64); err == nil && filters.MaxAmount != "" {
		params.MaxAmount = &v
	}
	switch filters.Pending {
	case "pending":
		p := true
		params.Pending = &p
	case "posted":
		p := false
		params.Pending = &p
	}

	result, err := h.app.Service.ListTransactions(r.Context(), params)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	render(w, r, http.StatusOK, pages.TransactionsList(h.shellData(r, "Transactions"), result, filters))
}

// transactionDetail renders one transaction by short_id (or UUID).
func (h *Handler) transactionDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t, err := h.app.Service.GetTransaction(r.Context(), id)
	if errors.Is(err, service.ErrNotFound) {
		h.notFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	// Activity timeline: enriched annotations (comments, rule applications,
	// tag/category changes) ordered created_at ASC. Best-effort — a timeline
	// fetch error must not 500 the whole detail page, so we render with nil.
	annotations, annErr := h.app.Service.ListAnnotations(r.Context(), id, service.ListAnnotationsParams{})
	if annErr != nil {
		annotations = nil
	}
	render(w, r, http.StatusOK, pages.TransactionDetail(h.shellData(r, txnTitle(t)), t, annotations))
}

// registerTransactions wires the transactions read surface onto the authed subrouter.
func (h *Handler) registerTransactions(r chi.Router) {
	r.Get("/transactions", h.transactionsList)
	r.Get("/transactions/{id}", h.transactionDetail)
}

// parseTxnDate parses a YYYY-MM-DD filter value; returns ok=false on empty/invalid input.
func parseTxnDate(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func txnTitle(t *service.TransactionResponse) string {
	if t.ProviderMerchantName != nil && *t.ProviderMerchantName != "" {
		return *t.ProviderMerchantName
	}
	return t.ProviderName
}
