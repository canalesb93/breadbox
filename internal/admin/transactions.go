package admin

import (
	"encoding/json"
	"errors"
	"html/template"
	"math"
	"net/http"
	"net/url"
	"sort"
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

// DateGroup holds transactions grouped by a single date.
type DateGroup struct {
	Date         string // raw date string, e.g. "2026-03-24"
	Label        string // human-friendly: "Today", "Yesterday", "Mon, Mar 22"
	Transactions []service.AdminTransactionRow
	DayTotal     float64 // net spending for the day (positive = outflow)
	DayIncome    float64 // total income (credits) for the day
	DaySpending  float64 // total spending (debits) for the day
}

// groupTransactionsByDate groups a flat list of transactions into date groups
// with smart labels (Today, Yesterday, or weekday + date).
func groupTransactionsByDate(txns []service.AdminTransactionRow) []DateGroup {
	if len(txns) == 0 {
		return nil
	}

	now := time.Now()
	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")

	// Preserve order: transactions come sorted by date desc.
	var groups []DateGroup
	groupIdx := map[string]int{}

	for i := range txns {
		tx := txns[i]
		date := tx.Date

		idx, exists := groupIdx[date]
		if !exists {
			label := smartDateLabel(date, today, yesterday)
			groups = append(groups, DateGroup{
				Date:  date,
				Label: label,
			})
			idx = len(groups) - 1
			groupIdx[date] = idx
		}

		groups[idx].Transactions = append(groups[idx].Transactions, tx)

		// Amount > 0 means outflow (debit), < 0 means income (credit) in Breadbox convention
		if tx.Amount > 0 {
			groups[idx].DaySpending += tx.Amount
		} else {
			groups[idx].DayIncome += math.Abs(tx.Amount)
		}
		groups[idx].DayTotal += tx.Amount
	}

	return groups
}

// smartDateLabel returns "Today", "Yesterday", or "Mon, Mar 22" / "Mon, Mar 22, 2025" for older.
func smartDateLabel(dateStr, today, yesterday string) string {
	if dateStr == today {
		return "Today"
	}
	if dateStr == yesterday {
		return "Yesterday"
	}
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return dateStr
	}
	now := time.Now()
	if t.Year() == now.Year() {
		return t.Format("Mon, Jan 2")
	}
	return t.Format("Mon, Jan 2, 2006")
}

// TransactionListHandler serves GET /admin/transactions.
func TransactionListHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}

		pageSize := 50
		if v, err := strconv.Atoi(r.URL.Query().Get("per_page")); err == nil {
			switch v {
			case 25, 50, 100:
				pageSize = v
			}
		}

		params := service.AdminTransactionListParams{
			Page:     page,
			PageSize: pageSize,
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
		if v := r.URL.Query().Get("search_mode"); v != "" && service.ValidateSearchMode(v) {
			params.SearchMode = &v
		}
		if v := r.URL.Query().Get("search_field"); v != "" && service.ValidateSearchField(v) {
			params.SearchField = &v
		}
		if v := r.URL.Query().Get("sort"); v == "asc" {
			params.SortOrder = "asc"
		}

		result, err := svc.ListTransactionsAdmin(ctx, params)
		if err != nil {
			a.Logger.Error("list admin transactions", "error", err)
			tr.RenderError(w, r)
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

		// Build pagination base URL (all params except page).
		paginationBase := buildPaginationBase(r)

		// Group transactions by date for the modern list view.
		dateGroups := groupTransactionsByDate(result.Transactions)

		data := map[string]any{
			"PageTitle":         "Transactions",
			"CurrentPage":      "transactions",
			"CSRFToken":        GetCSRFToken(r),
			"Flash":            GetFlash(ctx, sm),
			"Transactions":     result.Transactions,
			"DateGroups":       dateGroups,
			"Accounts":         accounts,
			"Users":            users,
			"Categories":       categoryTree,
			"Connections":      connections,
			"Page":             result.Page,
			"PageSize":         result.PageSize,
			"TotalPages":       result.TotalPages,
			"Total":            result.Total,
			"ExportURL":         exportURL,
			"PaginationBase":    paginationBase,
			"ShowingStart":      (result.Page-1)*result.PageSize + 1,
			"ShowingEnd":        min(int64(result.Page*result.PageSize), result.Total),
			"FilterStartDate":  stringOrEmpty(dateParamPtr(r, "start_date")),
			"FilterEndDate":    stringOrEmpty(dateParamPtr(r, "end_date")),
			"FilterAccountID":  stringOrEmpty(params.AccountID),
			"FilterUserID":     stringOrEmpty(params.UserID),
			"FilterConnID":     stringOrEmpty(params.ConnectionID),
			"FilterCategory":   stringOrEmpty(params.CategorySlug),
			"FilterMinAmount":  stringOrEmpty(floatParamPtr(r, "min_amount")),
			"FilterMaxAmount":  stringOrEmpty(floatParamPtr(r, "max_amount")),
			"FilterPending":    r.URL.Query().Get("pending"),
			"FilterSearch":      r.URL.Query().Get("search"),
			"FilterSearchMode":  r.URL.Query().Get("search_mode"),
			"FilterSearchField": r.URL.Query().Get("search_field"),
			"FilterSort":        r.URL.Query().Get("sort"),
		}
		tr.Render(w, r, "transactions.html", data)
	}
}

// TransactionSearchHandler serves GET /admin/transactions/search.
// Returns an HTML fragment (tx-results-partial) for AJAX swap by quickSearch().
func TransactionSearchHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}

		pageSize := 50
		if v, err := strconv.Atoi(r.URL.Query().Get("per_page")); err == nil {
			switch v {
			case 25, 50, 100:
				pageSize = v
			}
		}

		params := service.AdminTransactionListParams{
			Page:     page,
			PageSize: pageSize,
		}

		if v := r.URL.Query().Get("start_date"); v != "" {
			if t, err := time.Parse("2006-01-02", v); err == nil {
				params.StartDate = &t
			}
		}
		if v := r.URL.Query().Get("end_date"); v != "" {
			if t, err := time.Parse("2006-01-02", v); err == nil {
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
		if v := r.URL.Query().Get("search_mode"); v != "" && service.ValidateSearchMode(v) {
			params.SearchMode = &v
		}
		if v := r.URL.Query().Get("search_field"); v != "" && service.ValidateSearchField(v) {
			params.SearchField = &v
		}
		if v := r.URL.Query().Get("sort"); v == "asc" {
			params.SortOrder = "asc"
		}

		result, err := svc.ListTransactionsAdmin(ctx, params)
		if err != nil {
			a.Logger.Error("search admin transactions", "error", err)
			http.Error(w, "search error", http.StatusInternalServerError)
			return
		}

		// Load category tree for category pickers in the partial.
		categoryTree, err := svc.ListCategoryTree(ctx)
		if err != nil {
			a.Logger.Error("list categories for transaction search", "error", err)
		}

		paginationBase := buildPaginationBase(r)
		dateGroups := groupTransactionsByDate(result.Transactions)

		data := map[string]any{
			"Transactions":    result.Transactions,
			"DateGroups":      dateGroups,
			"Categories":      categoryTree,
			"Page":            result.Page,
			"PageSize":        result.PageSize,
			"TotalPages":      result.TotalPages,
			"Total":           result.Total,
			"PaginationBase":  paginationBase,
			"ShowingStart":    (result.Page-1)*result.PageSize + 1,
			"ShowingEnd":      min(int64(result.Page*result.PageSize), result.Total),
			"CSRFToken":       GetCSRFToken(r),
			"FilterSearch":    r.URL.Query().Get("search"),
			"FilterSearchMode": r.URL.Query().Get("search_mode"),
		}

		tr.RenderPartial(w, r, "transactions.html", "tx-results-partial", data)
	}
}

// AccountDetailHandler serves GET /admin/accounts/{id}.
func AccountDetailHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		idStr := chi.URLParam(r, "id")

		var accountID pgtype.UUID
		if err := accountID.Scan(idStr); err != nil {
			tr.RenderNotFound(w, r)
			return
		}

		detail, err := svc.GetAccountDetail(ctx, idStr)
		if err != nil {
			a.Logger.Error("get account detail", "error", err)
			tr.RenderNotFound(w, r)
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

		// --- Spending analytics for this account ---
		now := time.Now()
		thirtyDaysAgo := now.AddDate(0, 0, -30)
		sixtyDaysAgo := now.AddDate(0, 0, -60)

		// Category breakdown (last 30 days)
		type AccountCategory struct {
			Name  string
			Color string
			Icon  string
			Amount float64
		}
		var topCategories []AccountCategory
		var categoryLabelsJSON, categoryAmountsJSON, categoryColorsJSON template.JS
		var totalSpending float64

		catSummary, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
			GroupBy:      "category",
			AccountID:    &idStr,
			StartDate:    &thirtyDaysAgo,
			SpendingOnly: true,
		})
		if err != nil {
			a.Logger.Error("account category summary", "error", err)
		}
		if catSummary != nil {
			if catSummary.Totals.TotalAmount != nil {
				totalSpending = *catSummary.Totals.TotalAmount
			}
			catLabels := make([]string, 0, len(catSummary.Summary))
			catAmounts := make([]float64, 0, len(catSummary.Summary))
			catColors := make([]string, 0, len(catSummary.Summary))
			defaultColors := []string{
				"oklch(0.55 0.15 250)", "oklch(0.55 0.15 150)", "oklch(0.55 0.15 25)",
				"oklch(0.55 0.15 310)", "oklch(0.55 0.15 75)", "oklch(0.55 0.15 200)",
				"oklch(0.55 0.10 50)", "oklch(0.55 0.10 120)",
			}
			for i, row := range catSummary.Summary {
				name := "Uncategorized"
				if row.Category != nil {
					name = *row.Category
				}
				color := defaultColors[i%len(defaultColors)]
				if row.CategoryColor != nil && *row.CategoryColor != "" {
					color = *row.CategoryColor
				}
				icon := ""
				if row.CategoryIcon != nil {
					icon = *row.CategoryIcon
				}
				topCategories = append(topCategories, AccountCategory{
					Name:   name,
					Color:  color,
					Icon:   icon,
					Amount: row.TotalAmount,
				})
				catLabels = append(catLabels, name)
				catAmounts = append(catAmounts, row.TotalAmount)
				catColors = append(catColors, color)
			}
			if lb, err := json.Marshal(catLabels); err == nil {
				categoryLabelsJSON = template.JS(lb)
			}
			if ab, err := json.Marshal(catAmounts); err == nil {
				categoryAmountsJSON = template.JS(ab)
			}
			if cb, err := json.Marshal(catColors); err == nil {
				categoryColorsJSON = template.JS(cb)
			}
		}

		// Daily spending trend (last 30 days)
		var dailyLabelsJSON, dailyAmountsJSON template.JS
		dailySummary, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
			GroupBy:      "day",
			AccountID:    &idStr,
			StartDate:    &thirtyDaysAgo,
			SpendingOnly: true,
		})
		if err != nil {
			a.Logger.Error("account daily summary", "error", err)
		}
		if dailySummary != nil && len(dailySummary.Summary) > 0 {
			// Build a map of date->amount, then fill in all 30 days.
			dayMap := make(map[string]float64, len(dailySummary.Summary))
			for _, row := range dailySummary.Summary {
				if row.Period != nil {
					dayMap[*row.Period] = row.TotalAmount
				}
			}
			labels := make([]string, 0, 30)
			amounts := make([]float64, 0, 30)
			for i := 29; i >= 0; i-- {
				d := now.AddDate(0, 0, -i).Format("2006-01-02")
				labels = append(labels, d)
				amounts = append(amounts, dayMap[d])
			}
			if lb, err := json.Marshal(labels); err == nil {
				dailyLabelsJSON = template.JS(lb)
			}
			if ab, err := json.Marshal(amounts); err == nil {
				dailyAmountsJSON = template.JS(ab)
			}
		}

		// Previous 30-day spending for comparison
		var prevTotalSpending float64
		var spendingChangePercent float64
		var hasSpendingChange bool
		prevSummary, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
			GroupBy:      "category",
			AccountID:    &idStr,
			StartDate:    &sixtyDaysAgo,
			EndDate:      &thirtyDaysAgo,
			SpendingOnly: true,
		})
		if err != nil {
			a.Logger.Error("account prev period summary", "error", err)
		}
		if prevSummary != nil && prevSummary.Totals.TotalAmount != nil {
			prevTotalSpending = *prevSummary.Totals.TotalAmount
		}
		if prevTotalSpending > 0 {
			hasSpendingChange = true
			spendingChangePercent = ((totalSpending - prevTotalSpending) / prevTotalSpending) * 100
		}

		// Total income for this account (negative amounts = credits/income)
		var totalIncome float64
		allSummary, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
			GroupBy:   "day",
			AccountID: &idStr,
			StartDate: &thirtyDaysAgo,
		})
		if err != nil {
			a.Logger.Error("account income summary", "error", err)
		}
		if allSummary != nil {
			for _, row := range allSummary.Summary {
				if row.TotalAmount < 0 {
					totalIncome += -row.TotalAmount
				}
			}
		}

		// Transaction count for this account (30 days)
		var txCount30d int64
		if catSummary != nil {
			txCount30d = catSummary.Totals.TransactionCount
		}

		// Is this a liability (credit/loan)?
		isLiability := detail.Type == "credit" || detail.Type == "loan"

		// Credit utilization for credit cards
		var creditUtilization float64
		var hasCreditUtil bool
		if isLiability && detail.BalanceLimit != nil && detail.BalanceCurrent != nil {
			limit := *detail.BalanceLimit
			current := math.Abs(*detail.BalanceCurrent)
			if limit > 0 {
				hasCreditUtil = true
				creditUtilization = (current / limit) * 100
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
			// Spending analytics
			"TotalSpending":         totalSpending,
			"TotalIncome":           totalIncome,
			"TxCount30d":            txCount30d,
			"TopCategories":         topCategories,
			"CategoryLabels":        categoryLabelsJSON,
			"CategoryAmounts":       categoryAmountsJSON,
			"CategoryColors":        categoryColorsJSON,
			"DailyLabels":           dailyLabelsJSON,
			"DailyAmounts":          dailyAmountsJSON,
			"SpendingChangePercent": spendingChangePercent,
			"HasSpendingChange":     hasSpendingChange,
			"IsLiability":           isLiability,
			"CreditUtilization":     creditUtilization,
			"HasCreditUtil":         hasCreditUtil,
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
				tr.RenderNotFound(w, r)
				return
			}
			a.Logger.Error("get transaction detail", "error", err)
			tr.RenderError(w, r)
			return
		}

		comments, err := svc.ListComments(ctx, idStr)
		if err != nil && !errors.Is(err, service.ErrNotFound) {
			a.Logger.Error("list transaction comments", "error", err)
		}

		// Fetch review history for this transaction.
		reviews, err := svc.ListReviewsByTransactionID(ctx, idStr)
		if err != nil && !errors.Is(err, service.ErrNotFound) {
			a.Logger.Error("list transaction reviews", "error", err)
		}

		// Fetch rule applications for this transaction.
		ruleApps, err := svc.ListRuleApplicationsByTransactionID(ctx, idStr)
		if err != nil {
			a.Logger.Error("list transaction rule applications", "error", err)
		}

		// Build unified activity timeline.
		activity := buildActivityTimeline(reviews, comments, ruleApps)

		// Check if there's a pending review (to disable re-enqueue)
		hasPendingReview := false
		for _, r := range reviews {
			if r.Status == "pending" {
				hasPendingReview = true
				break
			}
		}

		// Fetch account context for richer detail display.
		var accountID, accountName, userName string
		var institutionName, accountMask, accountType, connectionID string
		var account *service.AccountResponse

		if txn.AccountID != nil {
			accountID = *txn.AccountID
			acct, acctErr := svc.GetAccount(ctx, accountID)
			if acctErr == nil {
				account = acct
				accountName = acct.Name
				if acct.InstitutionName != nil {
					institutionName = *acct.InstitutionName
				}
				if acct.Mask != nil {
					accountMask = *acct.Mask
				}
				accountType = acct.Type
				if acct.ConnectionID != nil {
					connectionID = *acct.ConnectionID
				}
				// Fetch user name from the account's user_id.
				if acct.UserID != nil {
					var uid pgtype.UUID
					if scanErr := uid.Scan(*acct.UserID); scanErr == nil {
						u, uErr := a.Queries.GetUser(ctx, uid)
						if uErr == nil {
							userName = u.Name
						}
					}
				}
			} else {
				a.Logger.Error("get account for transaction detail", "error", acctErr)
			}
		}
		// Fall back to denormalized names if account lookup didn't populate them.
		if accountName == "" && txn.AccountName != nil {
			accountName = *txn.AccountName
		}
		if userName == "" && txn.UserName != nil {
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
			"PageTitle":       txn.Name,
			"CurrentPage":     "transactions",
			"CSRFToken":       GetCSRFToken(r),
			"Flash":           GetFlash(ctx, sm),
			"Transaction":     txn,
			"TransactionID":   idStr,
			"AccountID":       accountID,
			"AccountName":     accountName,
			"UserName":        userName,
			"InstitutionName": institutionName,
			"AccountMask":     accountMask,
			"AccountType":     accountType,
			"ConnectionID":    connectionID,
			"Account":         account,
			"Activity":          activity,
			"HasPendingReview":  hasPendingReview,
			"Categories":      categoryTree,
			"Breadcrumbs":     breadcrumbs,
		}
		tr.Render(w, r, "transaction_detail.html", data)
	}
}

// buildActivityTimeline merges reviews, comments, and rule applications into a sorted timeline.
func buildActivityTimeline(reviews []service.ReviewResponse, comments []service.CommentResponse, ruleApps []service.TransactionRuleApplicationDetail) []service.ActivityEntry {
	var entries []service.ActivityEntry

	// Convert reviews — for resolved reviews, emit both the enqueue and resolution events
	for _, r := range reviews {
		// Always emit the "enqueued" event (using CreatedAt)
		var enqueueReason string
		switch r.ReviewType {
		case "uncategorized":
			enqueueReason = "Uncategorized transaction"
		case "low_confidence":
			enqueueReason = "Low confidence categorization"
		case "new_transaction":
			enqueueReason = "New transaction"
		case "re_review":
			enqueueReason = "Sent back for review"
		case "manual":
			enqueueReason = "Manually flagged"
		default:
			enqueueReason = "Flagged for review"
		}
		entries = append(entries, service.ActivityEntry{
			Type:         "review",
			Timestamp:    r.CreatedAt,
			ActorName:    "System",
			ActorType:    "system",
			Summary:      enqueueReason,
			Detail:       "Added to review queue",
			ReviewStatus: "pending",
		})

		// For resolved reviews, also emit the resolution event
		if r.Status != "pending" && r.ReviewedAt != nil {
			e := service.ActivityEntry{
				Type:      "review",
				Timestamp: *r.ReviewedAt,
			}
			if r.ReviewerName != nil {
				e.ActorName = *r.ReviewerName
			}
			if r.ReviewerType != nil {
				e.ActorType = *r.ReviewerType
			}
			switch r.Status {
			case "approved":
				e.ReviewStatus = "approved"
				e.Summary = "Approved"
				if r.ResolvedCategoryDisplayName != nil {
					e.Summary = "Approved as " + *r.ResolvedCategoryDisplayName
					e.CategoryName = *r.ResolvedCategoryDisplayName
				}
			case "rejected":
				e.ReviewStatus = "rejected"
				e.Summary = "Rejected"
			case "skipped":
				e.ReviewStatus = "skipped"
				e.Summary = "Skipped"
			}
			if r.ReviewNote != nil && *r.ReviewNote != "" {
				e.Detail = *r.ReviewNote
			}
			entries = append(entries, e)
		}
	}

	// Convert comments (filter out [Review: ...] duplicates from legacy data)
	for _, c := range comments {
		if strings.HasPrefix(c.Content, "[Review: ") {
			continue
		}
		entries = append(entries, service.ActivityEntry{
			Type:      "comment",
			Timestamp: c.CreatedAt,
			ActorName: c.AuthorName,
			ActorType: c.AuthorType,
			Detail:    c.Content,
			CommentID: c.ID,
		})
	}

	// Convert rule applications
	for _, ra := range ruleApps {
		summary := "Rule \"" + ra.RuleName + "\" set category"
		if ra.CategoryDisplayName != "" {
			summary = "Rule \"" + ra.RuleName + "\" set category to " + ra.CategoryDisplayName
		}
		how := "during sync"
		if ra.AppliedBy == "retroactive" {
			how = "retroactively"
		}
		entries = append(entries, service.ActivityEntry{
			Type:      "rule",
			Timestamp: ra.AppliedAt,
			ActorName: how,
			ActorType: "system",
			Summary:   summary,
			RuleName:  ra.RuleName,
			RuleID:    ra.RuleID,
			CategoryName: ra.CategoryDisplayName,
		})
	}

	// Sort by timestamp descending (newest first)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp > entries[j].Timestamp
	})

	return entries
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

// buildPaginationBase returns the query string for pagination links (all params except page).
func buildPaginationBase(r *http.Request) string {
	paginationParams := []string{
		"start_date", "end_date", "account_id", "user_id",
		"connection_id", "category", "min_amount", "max_amount",
		"pending", "search", "search_mode", "search_field", "sort", "per_page",
	}
	q := r.URL.Query()
	qs := make([]string, 0, len(paginationParams))
	for _, key := range paginationParams {
		if v := q.Get(key); v != "" {
			qs = append(qs, key+"="+url.QueryEscape(v))
		}
	}
	base := "/transactions?page="
	if len(qs) > 0 {
		base = "/transactions?" + strings.Join(qs, "&") + "&page="
	}
	return base
}

// buildExportURL returns the full CSV export URL with the current filter params.
func buildExportURL(r *http.Request) string {
	exportParams := []string{
		"start_date", "end_date", "account_id", "user_id",
		"connection_id", "category", "min_amount", "max_amount",
		"pending", "search", "search_mode", "search_field", "sort",
	}
	q := r.URL.Query()
	qs := make([]string, 0, len(exportParams))
	for _, key := range exportParams {
		if v := q.Get(key); v != "" {
			qs = append(qs, key+"="+url.QueryEscape(v))
		}
	}
	exportURL := "/-/transactions/export-csv"
	if len(qs) > 0 {
		exportURL += "?" + strings.Join(qs, "&")
	}
	return exportURL
}

// QuickSearchTransactionsHandler serves GET /-/search/transactions.
// Returns a lightweight JSON array for the command palette.
func QuickSearchTransactionsHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if len(q) < 2 {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
			return
		}

		searchMode := "words"
		params := service.AdminTransactionListParams{
			Page:       1,
			PageSize:   8,
			Search:     &q,
			SearchMode: &searchMode,
			SortOrder:  "desc",
		}

		result, err := svc.ListTransactionsAdmin(r.Context(), params)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
			return
		}

		type txResult struct {
			ID            string  `json:"id"`
			Name          string  `json:"name"`
			Amount        float64 `json:"amount"`
			Date          string  `json:"date"`
			Account       string  `json:"account"`
			Merchant      *string `json:"merchant,omitempty"`
			Pending       bool    `json:"pending,omitempty"`
			CategoryIcon  *string `json:"category_icon,omitempty"`
			CategoryColor *string `json:"category_color,omitempty"`
		}

		items := make([]txResult, 0, len(result.Transactions))
		for _, tx := range result.Transactions {
			items = append(items, txResult{
				ID:            tx.ID,
				Name:          tx.Name,
				Amount:        tx.Amount,
				Date:          tx.Date,
				Account:       tx.AccountName,
				Merchant:      tx.MerchantName,
				Pending:       tx.Pending,
				CategoryIcon:  tx.CategoryIcon,
				CategoryColor: tx.CategoryColor,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(items)
	}
}
