package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"breadbox/internal/app"
	"breadbox/internal/db"
	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TransactionListHandler serves GET /admin/transactions.
func TransactionListHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}

		params := service.AdminTransactionListParams{
			Page:     page,
			PageSize: 50,
		}

		if v := r.URL.Query().Get("start_date"); v != "" {
			if t, err := time.Parse("2006-01-02", v); err == nil {
				params.StartDate = &t
			}
		}
		if v := r.URL.Query().Get("end_date"); v != "" {
			if t, err := time.Parse("2006-01-02", v); err == nil {
				// Add one day so the end date is inclusive.
				t = t.AddDate(0, 0, 1)
				params.EndDate = &t
			}
		}
		if v := r.URL.Query().Get("account_id"); v != "" {
			params.AccountID = &v
		}
		if v := r.URL.Query().Get("user_id"); v != "" {
			params.UserID = &v
		}
		if v := r.URL.Query().Get("connection_id"); v != "" {
			params.ConnectionID = &v
		}
		if v := r.URL.Query().Get("category"); v != "" {
			params.CategorySlug = &v
		}
		if v := r.URL.Query().Get("min_amount"); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				params.MinAmount = &f
			}
		}
		if v := r.URL.Query().Get("max_amount"); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				params.MaxAmount = &f
			}
		}
		if v := r.URL.Query().Get("pending"); v != "" {
			b := v == "true"
			params.Pending = &b
		}
		if v := r.URL.Query().Get("search"); v != "" {
			params.Search = &v
		}
		if v := r.URL.Query().Get("sort"); v == "asc" {
			params.SortOrder = "asc"
		}

		result, err := svc.ListTransactionsAdmin(ctx, params)
		if err != nil {
			a.Logger.Error("list admin transactions", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Load filter dropdowns concurrently.
		var (
			accounts     []service.AccountResponse
			users        []db.User
			categoryTree []service.CategoryResponse
			connections  []db.ListBankConnectionsRow
			wg           sync.WaitGroup
		)
		wg.Add(4)
		go func() {
			defer wg.Done()
			var err error
			accounts, err = svc.ListAccounts(ctx, nil)
			if err != nil {
				a.Logger.Error("list accounts for transaction filters", "error", err)
			}
		}()
		go func() {
			defer wg.Done()
			var err error
			users, err = a.Queries.ListUsers(ctx)
			if err != nil {
				a.Logger.Error("list users for transaction filters", "error", err)
			}
		}()
		go func() {
			defer wg.Done()
			var err error
			categoryTree, err = svc.ListCategoryTree(ctx)
			if err != nil {
				a.Logger.Error("list categories for transaction filters", "error", err)
			}
		}()
		go func() {
			defer wg.Done()
			var err error
			connections, err = a.Queries.ListBankConnections(ctx)
			if err != nil {
				a.Logger.Error("list connections for transaction filters", "error", err)
			}
		}()
		wg.Wait()

		// Build export URL from active filters (excludes page param).
		exportURL := buildExportURL(r)

		data := map[string]any{
			"PageTitle":         "Transactions",
			"CurrentPage":      "transactions",
			"CSRFToken":        GetCSRFToken(r),
			"Flash":            GetFlash(ctx, sm),
			"Transactions":     result.Transactions,
			"Accounts":         accounts,
			"Users":            users,
			"Categories":       categoryTree,
			"Connections":      connections,
			"Page":             result.Page,
			"TotalPages":       result.TotalPages,
			"Total":            result.Total,
			"ExportURL":         exportURL,
			"FilterStartDate":  stringOrEmpty(dateParamPtr(r, "start_date")),
			"FilterEndDate":    stringOrEmpty(dateParamPtr(r, "end_date")),
			"FilterAccountID":  stringOrEmpty(params.AccountID),
			"FilterUserID":     stringOrEmpty(params.UserID),
			"FilterConnID":     stringOrEmpty(params.ConnectionID),
			"FilterCategory":   stringOrEmpty(params.CategorySlug),
			"FilterMinAmount":  stringOrEmpty(floatParamPtr(r, "min_amount")),
			"FilterMaxAmount":  stringOrEmpty(floatParamPtr(r, "max_amount")),
			"FilterPending":    r.URL.Query().Get("pending"),
			"FilterSearch":     r.URL.Query().Get("search"),
			"FilterSort":       r.URL.Query().Get("sort"),
		}
		tr.Render(w, r, "transactions.html", data)
	}
}

// AccountDetailHandler serves GET /admin/accounts/{id}.
func AccountDetailHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		idStr := chi.URLParam(r, "id")

		var accountID pgtype.UUID
		if err := accountID.Scan(idStr); err != nil {
			http.Error(w, "Invalid account ID", http.StatusBadRequest)
			return
		}

		detail, err := svc.GetAccountDetail(ctx, idStr)
		if err != nil {
			a.Logger.Error("get account detail", "error", err)
			http.NotFound(w, r)
			return
		}

		// Fetch transactions for this account.
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}

		txParams := service.AdminTransactionListParams{
			Page:      page,
			PageSize:  50,
			AccountID: &idStr,
		}

		if v := r.URL.Query().Get("start_date"); v != "" {
			if t, err := time.Parse("2006-01-02", v); err == nil {
				txParams.StartDate = &t
			}
		}
		if v := r.URL.Query().Get("end_date"); v != "" {
			if t, err := time.Parse("2006-01-02", v); err == nil {
				t = t.AddDate(0, 0, 1)
				txParams.EndDate = &t
			}
		}
		if v := r.URL.Query().Get("search"); v != "" {
			txParams.Search = &v
		}
		if v := r.URL.Query().Get("category"); v != "" {
			txParams.CategorySlug = &v
		}
		if v := r.URL.Query().Get("pending"); v != "" {
			b := v == "true"
			txParams.Pending = &b
		}

		txResult, err := svc.ListTransactionsAdmin(ctx, txParams)
		if err != nil {
			a.Logger.Error("list transactions for account detail", "error", err)
		}

		categoryTree, err := svc.ListCategoryTree(ctx)
		if err != nil {
			a.Logger.Error("list categories for account detail", "error", err)
		}

		// Build export URL for this account's transactions.
		acctExportURL := "/-/transactions/export-csv?account_id=" + idStr
		if sd := r.URL.Query().Get("start_date"); sd != "" {
			acctExportURL += "&start_date=" + sd
		}
		if ed := r.URL.Query().Get("end_date"); ed != "" {
			acctExportURL += "&end_date=" + ed
		}
		if cat := r.URL.Query().Get("category"); cat != "" {
			acctExportURL += "&category=" + cat
		}
		if search := r.URL.Query().Get("search"); search != "" {
			acctExportURL += "&search=" + search
		}

		data := map[string]any{
			"PageTitle":       detail.InstitutionName + " — " + accountDisplayName(detail),
			"CurrentPage":    "transactions",
			"CSRFToken":      GetCSRFToken(r),
			"Flash":          GetFlash(ctx, sm),
			"Account":        detail,
			"AccountID":      idStr,
			"Transactions":   txResult.Transactions,
			"Categories":     categoryTree,
			"Page":           txResult.Page,
			"TotalPages":     txResult.TotalPages,
			"Total":          txResult.Total,
			"ExportURL":      acctExportURL,
			"FilterStartDate": stringOrEmpty(dateParamPtr(r, "start_date")),
			"FilterEndDate":   stringOrEmpty(dateParamPtr(r, "end_date")),
			"FilterCategory":  r.URL.Query().Get("category"),
			"FilterPending":   r.URL.Query().Get("pending"),
			"FilterSearch":    r.URL.Query().Get("search"),
			"Breadcrumbs": []Breadcrumb{
				{Label: "Connections", Href: "/connections"},
				{Label: detail.InstitutionName, Href: "/connections/" + detail.ConnectionID},
				{Label: accountDisplayName(detail)},
			},
		}
		tr.Render(w, r, "account_detail.html", data)
	}
}

func accountDisplayName(detail *service.AdminAccountDetail) string {
	if detail.DisplayName != nil && *detail.DisplayName != "" {
		return *detail.DisplayName
	}
	return detail.Name
}

func dateParamPtr(r *http.Request, key string) *string {
	v := r.URL.Query().Get(key)
	if v == "" {
		return nil
	}
	return &v
}

func floatParamPtr(r *http.Request, key string) *string {
	v := r.URL.Query().Get(key)
	if v == "" {
		return nil
	}
	return &v
}

// TransactionDetailHandler serves GET /admin/transactions/{id}.
func TransactionDetailHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		idStr := chi.URLParam(r, "id")

		txn, err := svc.GetTransaction(ctx, idStr)
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				http.NotFound(w, r)
				return
			}
			a.Logger.Error("get transaction detail", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		comments, err := svc.ListComments(ctx, idStr)
		if err != nil && !errors.Is(err, service.ErrNotFound) {
			a.Logger.Error("list transaction comments", "error", err)
		}

		// Use denormalized names from the transaction response.
		var accountName, userName, accountID string
		if txn.AccountID != nil {
			accountID = *txn.AccountID
		}
		if txn.AccountName != nil {
			accountName = *txn.AccountName
		}
		if txn.UserName != nil {
			userName = *txn.UserName
		}

		// Load category tree for inline category picker.
		categoryTree, err := svc.ListCategoryTree(ctx)
		if err != nil {
			a.Logger.Error("list categories for transaction detail", "error", err)
		}

		// Build breadcrumbs: Transactions > Account Name > Transaction Name
		breadcrumbs := []Breadcrumb{
			{Label: "Transactions", Href: "/transactions"},
		}
		if accountName != "" && accountID != "" {
			breadcrumbs = append(breadcrumbs, Breadcrumb{Label: accountName, Href: "/accounts/" + accountID})
		}
		breadcrumbs = append(breadcrumbs, Breadcrumb{Label: txn.Name})

		data := map[string]any{
			"PageTitle":     txn.Name,
			"CurrentPage":   "transactions",
			"CSRFToken":     GetCSRFToken(r),
			"Flash":         GetFlash(ctx, sm),
			"Transaction":   txn,
			"TransactionID": idStr,
			"AccountID":     accountID,
			"AccountName":   accountName,
			"UserName":      userName,
			"Comments":      comments,
			"Categories":    categoryTree,
			"Breadcrumbs":   breadcrumbs,
		}
		tr.Render(w, r, "transaction_detail.html", data)
	}
}

// CreateTransactionCommentHandler serves POST /admin/api/transactions/{id}/comments.
func CreateTransactionCommentHandler(a *app.App, sm *scs.SessionManager, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		txnID := chi.URLParam(r, "id")
		actor := ActorFromSession(sm, r)

		var input struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}

		comment, err := svc.CreateComment(r.Context(), service.CreateCommentParams{
			TransactionID: txnID,
			Content:       input.Content,
			Actor:         actor,
		})
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "Transaction not found"})
				return
			}
			// Content validation errors are safe to surface; log and genericize others.
			if strings.Contains(err.Error(), "content must be") {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			a.Logger.Error("create comment", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create comment"})
			return
		}

		writeJSON(w, http.StatusCreated, comment)
	}
}

// DeleteTransactionCommentHandler serves DELETE /admin/api/transactions/{id}/comments/{comment_id}.
func DeleteTransactionCommentHandler(a *app.App, sm *scs.SessionManager, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		commentID := chi.URLParam(r, "comment_id")
		actor := ActorFromSession(sm, r)

		if err := svc.DeleteComment(r.Context(), commentID, actor); err != nil {
			if errors.Is(err, service.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "Comment not found"})
				return
			}
			if errors.Is(err, service.ErrForbidden) {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "You can only delete your own comments"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete comment"})
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// buildExportURL returns the full CSV export URL with the current filter params.
func buildExportURL(r *http.Request) string {
	exportParams := []string{
		"start_date", "end_date", "account_id", "user_id",
		"connection_id", "category", "min_amount", "max_amount",
		"pending", "search", "sort",
	}
	q := r.URL.Query()
	qs := make([]string, 0, len(exportParams))
	for _, key := range exportParams {
		if v := q.Get(key); v != "" {
			qs = append(qs, key+"="+v)
		}
	}
	url := "/-/transactions/export-csv"
	if len(qs) > 0 {
		url += "?" + strings.Join(qs, "&")
	}
	return url
}
