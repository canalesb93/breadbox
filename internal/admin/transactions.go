package admin

import (
	"net/http"
	"strconv"
	"time"

	"breadbox/internal/app"
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
			params.Category = &v
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

		// Load filter dropdowns.
		accounts, err := svc.ListAccounts(ctx, nil)
		if err != nil {
			a.Logger.Error("list accounts for transaction filters", "error", err)
		}

		users, err := a.Queries.ListUsers(ctx)
		if err != nil {
			a.Logger.Error("list users for transaction filters", "error", err)
		}

		categoryPairs, err := svc.ListDistinctCategories(ctx)
		if err != nil {
			a.Logger.Error("list categories for transaction filters", "error", err)
		}
		seen := make(map[string]bool)
		var categories []string
		for _, cp := range categoryPairs {
			if !seen[cp.Primary] {
				seen[cp.Primary] = true
				categories = append(categories, cp.Primary)
			}
		}

		connections, err := a.Queries.ListBankConnections(ctx)
		if err != nil {
			a.Logger.Error("list connections for transaction filters", "error", err)
		}

		data := map[string]any{
			"PageTitle":        "Transactions",
			"CurrentPage":     "transactions",
			"CSRFToken":       GetCSRFToken(r),
			"Flash":           GetFlash(ctx, sm),
			"Transactions":    result.Transactions,
			"Accounts":        accounts,
			"Users":           users,
			"Categories":      categories,
			"Connections":     connections,
			"Page":            result.Page,
			"TotalPages":      result.TotalPages,
			"Total":           result.Total,
			"FilterStartDate": stringOrEmpty(dateParamPtr(r, "start_date")),
			"FilterEndDate":   stringOrEmpty(dateParamPtr(r, "end_date")),
			"FilterAccountID": stringOrEmpty(params.AccountID),
			"FilterUserID":    stringOrEmpty(params.UserID),
			"FilterConnID":    stringOrEmpty(params.ConnectionID),
			"FilterCategory":  stringOrEmpty(params.Category),
			"FilterMinAmount": stringOrEmpty(floatParamPtr(r, "min_amount")),
			"FilterMaxAmount": stringOrEmpty(floatParamPtr(r, "max_amount")),
			"FilterPending":   r.URL.Query().Get("pending"),
			"FilterSearch":    r.URL.Query().Get("search"),
			"FilterSort":      r.URL.Query().Get("sort"),
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
			txParams.Category = &v
		}
		if v := r.URL.Query().Get("pending"); v != "" {
			b := v == "true"
			txParams.Pending = &b
		}

		txResult, err := svc.ListTransactionsAdmin(ctx, txParams)
		if err != nil {
			a.Logger.Error("list transactions for account detail", "error", err)
		}

		categoryPairs2, err := svc.ListDistinctCategories(ctx)
		if err != nil {
			a.Logger.Error("list categories for account detail", "error", err)
		}
		seen2 := make(map[string]bool)
		var categories []string
		for _, cp := range categoryPairs2 {
			if !seen2[cp.Primary] {
				seen2[cp.Primary] = true
				categories = append(categories, cp.Primary)
			}
		}

		data := map[string]any{
			"PageTitle":       detail.InstitutionName + " — " + accountDisplayName(detail),
			"CurrentPage":    "transactions",
			"CSRFToken":      GetCSRFToken(r),
			"Flash":          GetFlash(ctx, sm),
			"Account":        detail,
			"AccountID":      idStr,
			"Transactions":   txResult.Transactions,
			"Categories":     categories,
			"Page":           txResult.Page,
			"TotalPages":     txResult.TotalPages,
			"Total":          txResult.Total,
			"FilterStartDate": stringOrEmpty(dateParamPtr(r, "start_date")),
			"FilterEndDate":   stringOrEmpty(dateParamPtr(r, "end_date")),
			"FilterCategory":  r.URL.Query().Get("category"),
			"FilterPending":   r.URL.Query().Get("pending"),
			"FilterSearch":    r.URL.Query().Get("search"),
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
