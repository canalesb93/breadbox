//go:build !headless && !lite

package webapp

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"breadbox/internal/admin"
	"breadbox/internal/service"
	"breadbox/internal/webapp/components"
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
	// Category options power both the bulk "set category" <select> and the
	// per-row inline category editor. Best-effort — a category fetch error must
	// not 500 the list; the pickers simply render empty (and the island degrades).
	cats, _ := h.txnCategoryOptions(r)
	render(w, r, http.StatusOK, pages.TransactionsList(h.shellData(r, "Transactions"), result, filters, cats))
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

// transactionsBulk applies a single mutation (set category and/or add a tag) to a
// set of selected transactions. It's the no-JS fallback for the bulk action bar:
// a native multi-checkbox <form> POSTs the selected IDs plus the chosen category /
// tag here. The island enhances the same form (toggle bar, count) but submits it
// the same way, so behavior is identical with or without JS.
//
// category_override is sacred: setting a category here writes the override via the
// service's per-op SetTransactionCategoryOverride. Rows the user did NOT select are
// never touched, so no other row's override is cleared or ignored.
func (h *Handler) transactionsBulk(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	ids := r.Form["tx_id"]
	categorySlug := strings.TrimSpace(r.FormValue("category_slug"))
	tag := strings.TrimSpace(r.FormValue("tag"))

	// Build one compound op per selected transaction. An op with neither a
	// category nor a tag would be a no-op; skip the whole batch in that case.
	ops := make([]service.UpdateTransactionsOp, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		op := service.UpdateTransactionsOp{TransactionID: id}
		if categorySlug != "" {
			s := categorySlug
			op.CategorySlug = &s
		}
		if tag != "" {
			op.TagsToAdd = []service.UpdateTransactionsTagOp{{Slug: tag}}
		}
		ops = append(ops, op)
	}

	if len(ops) > 0 && (categorySlug != "" || tag != "") {
		_, err := h.app.Service.UpdateTransactions(r.Context(), service.UpdateTransactionsParams{
			Operations: ops,
			OnError:    "continue",
			Actor:      admin.ActorFromSession(h.sm, r),
		})
		if err != nil {
			h.serverError(w, r, err)
			return
		}
	}

	// 303 back to the list preserving the filter state the form carried in the
	// "return" hidden field (the island sets it to the current querystring).
	http.Redirect(w, r, txnReturnURL(r), http.StatusSeeOther)
}

// transactionCategory sets the category override on a single transaction. It backs
// the inline category editor: the island POSTs {category_slug} here as fetch and
// optimistically updates the cell; without JS the same <form> submits and 303s back.
func (h *Handler) transactionCategory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	_ = r.ParseForm()
	categorySlug := strings.TrimSpace(r.FormValue("category_slug"))
	if categorySlug == "" {
		http.Error(w, "category_slug is required", http.StatusBadRequest)
		return
	}

	_, err := h.app.Service.UpdateTransactions(r.Context(), service.UpdateTransactionsParams{
		Operations: []service.UpdateTransactionsOp{{
			TransactionID: id,
			CategorySlug:  &categorySlug,
		}},
		OnError: "abort",
		Actor:   admin.ActorFromSession(h.sm, r),
	})
	if errors.Is(err, service.ErrNotFound) {
		h.notFound(w, r)
		return
	}
	if errors.Is(err, service.ErrCategoryNotFound) || errors.Is(err, service.ErrInvalidParameter) {
		http.Error(w, "invalid category", http.StatusBadRequest)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, txnReturnURL(r), http.StatusSeeOther)
}

// registerTransactions wires the transactions read surface onto the authed subrouter.
func (h *Handler) registerTransactions(r chi.Router) {
	r.Get("/transactions", h.transactionsList)
	r.Post("/transactions/bulk", h.requireSameOrigin(h.transactionsBulk))
	r.Get("/transactions/{id}", h.transactionDetail)
	r.Post("/transactions/{id}/category", h.requireSameOrigin(h.transactionCategory))
}

// txnReturnURL extracts the same-site /app return path the form carried so a POST
// 303s back to the list with its filters intact. Falls back to the bare list.
func txnReturnURL(r *http.Request) string {
	dest := strings.TrimSpace(r.FormValue("return"))
	if dest == "" || !strings.HasPrefix(dest, "/app") || strings.HasPrefix(dest, "//") {
		return "/app/transactions"
	}
	return dest
}

// txnCategoryOptions lists categories as picker options (slug → display name) for
// the bulk and inline category controls.
func (h *Handler) txnCategoryOptions(r *http.Request) ([]components.Option, error) {
	cats, err := h.app.Service.ListCategories(r.Context())
	if err != nil {
		return nil, err
	}
	opts := make([]components.Option, 0, len(cats))
	for _, c := range cats {
		opts = append(opts, components.Option{Value: c.Slug, Label: c.DisplayName})
	}
	return opts, nil
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
