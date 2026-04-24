package admin

import (
	"errors"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"breadbox/internal/app"
	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	"breadbox/internal/templates/components"
	"breadbox/internal/templates/components/pages"

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

		q := r.URL.Query()
		params := service.AdminTransactionListParams{
			Page:         parsePage(r),
			PageSize:     parsePerPage(r, 50, 25, 50, 100),
			StartDate:    parseDateParam(r, "start_date"),
			EndDate:      parseInclusiveDateParam(r, "end_date"),
			AccountID:    optStrQuery(q, "account_id"),
			UserID:       optStrQuery(q, "user_id"),
			ConnectionID: optStrQuery(q, "connection_id"),
			CategorySlug: optStrQuery(q, "category"),
			MinAmount:    optFloatQuery(q, "min_amount"),
			MaxAmount:    optFloatQuery(q, "max_amount"),
			Search:       optStrQuery(q, "search"),
		}

		if v := q.Get("pending"); v != "" {
			b := v == "true"
			params.Pending = &b
		}
		if v := q.Get("search_mode"); v != "" && service.ValidateSearchMode(v) {
			params.SearchMode = &v
		}
		if v := q.Get("search_field"); v != "" && service.ValidateSearchField(v) {
			params.SearchField = &v
		}
		if q.Get("sort") == "asc" {
			params.SortOrder = "asc"
		}

		// Tag filters. ?tags=needs-review,foo (AND) and ?any_tag=a,b (OR).
		if v := q.Get("tags"); v != "" {
			params.Tags = splitCSV(v)
		}
		if v := q.Get("any_tag"); v != "" {
			params.AnyTag = splitCSV(v)
		}

		// Scope to viewer's own data. Editors and admins see all.
		if !IsEditor(sm, r) {
			if uid := SessionUserID(sm, r); uid != "" {
				params.UserID = &uid
			}
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

		// Pull the registered tag list for the multi-select filter UI.
		allTags, _ := svc.ListTags(ctx)

		renderTransactions(w, r, tr, transactionsRenderInput{
			sm:             sm,
			params:         params,
			result:         result,
			dateGroups:     dateGroups,
			accounts:       accounts,
			users:          users,
			categories:     categoryTree,
			connections:    connections,
			allTags:        allTags,
			exportURL:      exportURL,
			paginationBase: paginationBase,
		})
	}
}

// transactionsRenderInput gathers the handler-side inputs that feed
// renderTransactions. Kept as a struct so the call site stays readable and
// future fields can be added without a long positional argument list.
type transactionsRenderInput struct {
	sm             *scs.SessionManager
	params         service.AdminTransactionListParams
	result         *service.AdminTransactionListResult
	dateGroups     []DateGroup
	accounts       []service.AccountResponse
	users          []db.User
	categories     []service.CategoryResponse
	connections    []db.ListBankConnectionsRow
	allTags        []service.TagResponse
	exportURL      string
	paginationBase string
}

// renderTransactions builds the TransactionsProps view model and hands it to
// the templ component via RenderWithTempl. Mirrors the renderDashboard pattern
// in internal/admin/dashboard.go — the handler only has to collect raw inputs;
// the conversion to the typed props lives here so the templ component stays
// decoupled from the handler's map[string]any layout data.
func renderTransactions(w http.ResponseWriter, r *http.Request, tr *TemplateRenderer, in transactionsRenderInput) {
	q := r.URL.Query()

	connOpts := make([]pages.TransactionsConnectionOption, 0, len(in.connections))
	for _, c := range in.connections {
		connOpts = append(connOpts, pages.TransactionsConnectionOption{
			ID:              pgconv.FormatUUID(c.ID),
			InstitutionName: c.InstitutionName.String,
		})
	}

	acctOpts := make([]pages.TransactionsAccountOption, 0, len(in.accounts))
	for _, a := range in.accounts {
		mask := ""
		if a.Mask != nil {
			mask = *a.Mask
		}
		acctOpts = append(acctOpts, pages.TransactionsAccountOption{
			ID:   a.ID,
			Name: a.Name,
			Mask: mask,
		})
	}

	userOpts := make([]pages.TransactionsUserOption, 0, len(in.users))
	for _, u := range in.users {
		userOpts = append(userOpts, pages.TransactionsUserOption{
			ID:   pgconv.FormatUUID(u.ID),
			Name: u.Name,
		})
	}

	// Convert internal admin.DateGroup slice into components.TxResultsDateGroup
	// (same field set, just a different package home).
	groups := make([]components.TxResultsDateGroup, len(in.dateGroups))
	for i, g := range in.dateGroups {
		groups[i] = components.TxResultsDateGroup{
			Date:         g.Date,
			Label:        g.Label,
			Transactions: g.Transactions,
			DaySpending:  g.DaySpending,
			DayIncome:    g.DayIncome,
		}
	}

	results := components.TxResultsProps{
		DateGroups:     groups,
		Transactions:   in.result.Transactions,
		Page:           in.result.Page,
		TotalPages:     in.result.TotalPages,
		PageSize:       in.result.PageSize,
		Total:          int(in.result.Total),
		ShowingStart:   (in.result.Page-1)*in.result.PageSize + 1,
		ShowingEnd:     int(min(int64(in.result.Page*in.result.PageSize), in.result.Total)),
		PaginationBase: in.paginationBase,
	}

	props := pages.TransactionsProps{
		CSRFToken:         GetCSRFToken(r),
		Total:             in.result.Total,
		Transactions:      in.result.Transactions,
		Connections:       connOpts,
		Accounts:          acctOpts,
		Users:             userOpts,
		Categories:        in.categories,
		AllTags:           in.allTags,
		FilterStartDate:   stringOrEmpty(dateParamPtr(r, "start_date")),
		FilterEndDate:     stringOrEmpty(dateParamPtr(r, "end_date")),
		FilterAccountID:   stringOrEmpty(in.params.AccountID),
		FilterUserID:      stringOrEmpty(in.params.UserID),
		FilterConnID:      stringOrEmpty(in.params.ConnectionID),
		FilterCategory:    stringOrEmpty(in.params.CategorySlug),
		FilterMinAmount:   stringOrEmpty(floatParamPtr(r, "min_amount")),
		FilterMaxAmount:   stringOrEmpty(floatParamPtr(r, "max_amount")),
		FilterPending:     q.Get("pending"),
		FilterSearch:      q.Get("search"),
		FilterSearchMode:  q.Get("search_mode"),
		FilterSearchField: q.Get("search_field"),
		FilterSort:        q.Get("sort"),
		FilterTags:        in.params.Tags,
		FilterAnyTag:      in.params.AnyTag,
		ExportURL:         in.exportURL,
		Results:           results,
	}
	// Chip summary renders above the results when the filter panel is
	// collapsed. Built after the rest of props so all lookup lists (names
	// for connections/accounts/users/categories/tags) are already populated.
	props.FilterChips = buildTransactionFilterChips(r, props)

	data := map[string]any{
		"PageTitle":   "Transactions",
		"CurrentPage": "transactions",
		"CSRFToken":   GetCSRFToken(r),
		"Flash":       GetFlash(r.Context(), in.sm),
	}
	tr.RenderWithTempl(w, r, data, pages.Transactions(props))
}

// TransactionSearchHandler serves GET /admin/transactions/search.
// Returns an HTML fragment (tx-results-partial) for AJAX swap by quickSearch().
func TransactionSearchHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		q := r.URL.Query()
		params := service.AdminTransactionListParams{
			Page:         parsePage(r),
			PageSize:     parsePerPage(r, 50, 25, 50, 100),
			StartDate:    parseDateParam(r, "start_date"),
			EndDate:      parseInclusiveDateParam(r, "end_date"),
			AccountID:    optStrQuery(q, "account_id"),
			UserID:       optStrQuery(q, "user_id"),
			ConnectionID: optStrQuery(q, "connection_id"),
			CategorySlug: optStrQuery(q, "category"),
			MinAmount:    optFloatQuery(q, "min_amount"),
			MaxAmount:    optFloatQuery(q, "max_amount"),
			Search:       optStrQuery(q, "search"),
		}

		if v := q.Get("pending"); v != "" {
			b := v == "true"
			params.Pending = &b
		}
		if v := q.Get("search_mode"); v != "" && service.ValidateSearchMode(v) {
			params.SearchMode = &v
		}
		if v := q.Get("search_field"); v != "" && service.ValidateSearchField(v) {
			params.SearchField = &v
		}
		if q.Get("sort") == "asc" {
			params.SortOrder = "asc"
		}

		if v := q.Get("tags"); v != "" {
			params.Tags = splitCSV(v)
		}
		if v := q.Get("any_tag"); v != "" {
			params.AnyTag = splitCSV(v)
		}

		// Scope to viewer's own data. Editors and admins see all.
		if !IsEditor(sm, r) {
			if uid := SessionUserID(sm, r); uid != "" {
				params.UserID = &uid
			}
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
		_ = categoryTree // categories are no longer needed — rows render tags/labels from DOM-cached data

		// Convert to the templ component's typed props and render the
		// fragment directly (no shell). This replaces the old
		// tr.RenderPartial("transactions.html", "tx-results-partial") call
		// — the TxResults templ component is the canonical source for
		// both the inline and AJAX render paths.
		groups := make([]components.TxResultsDateGroup, len(dateGroups))
		for i, g := range dateGroups {
			groups[i] = components.TxResultsDateGroup{
				Date:         g.Date,
				Label:        g.Label,
				Transactions: g.Transactions,
				DaySpending:  g.DaySpending,
				DayIncome:    g.DayIncome,
			}
		}
		props := components.TxResultsProps{
			DateGroups:     groups,
			Transactions:   result.Transactions,
			Page:           result.Page,
			TotalPages:     result.TotalPages,
			PageSize:       result.PageSize,
			Total:          int(result.Total),
			ShowingStart:   (result.Page-1)*result.PageSize + 1,
			ShowingEnd:     int(min(int64(result.Page*result.PageSize), result.Total)),
			PaginationBase: paginationBase,
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := pages.TransactionsResults(props).Render(r.Context(), w); err != nil {
			a.Logger.Error("render transactions results partial", "error", err)
		}
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

		// IDOR check: viewers can only view their own accounts. Editors+ see all.
		memberUID := SessionUserID(sm, r)
		if !IsEditor(sm, r) {
			if detail.UserID == nil || *detail.UserID != memberUID {
				tr.RenderNotFound(w, r)
				return
			}
		}

		// Fetch transactions for this account.
		txParams := service.AdminTransactionListParams{
			Page:      parsePage(r),
			PageSize:  50,
			AccountID: &idStr,
			StartDate: parseDateParam(r, "start_date"),
			EndDate:   parseInclusiveDateParam(r, "end_date"),
		}

		// Scope transaction query to viewer's user. Editors+ see all.
		if !IsEditor(sm, r) {
			txParams.UserID = &memberUID
		}

		q := r.URL.Query()
		txParams.Search = optStrQuery(q, "search")
		txParams.CategorySlug = optStrQuery(q, "category")
		if v := q.Get("pending"); v != "" {
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
		if sd := q.Get("start_date"); sd != "" {
			acctExportURL += "&start_date=" + sd
		}
		if ed := q.Get("end_date"); ed != "" {
			acctExportURL += "&end_date=" + ed
		}
		if cat := q.Get("category"); cat != "" {
			acctExportURL += "&category=" + cat
		}
		if search := q.Get("search"); search != "" {
			acctExportURL += "&search=" + search
		}

		// --- Spending analytics for this account ---
		now := time.Now()
		thirtyDaysAgo := now.AddDate(0, 0, -30)
		sixtyDaysAgo := now.AddDate(0, 0, -60)

		// 30-day spending total
		var totalSpending float64
		var txCount30d int64
		spendingSummary, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
			GroupBy:      "category",
			AccountID:    &idStr,
			StartDate:    &thirtyDaysAgo,
			SpendingOnly: true,
		})
		if err != nil {
			a.Logger.Error("account spending summary", "error", err)
		}
		if spendingSummary != nil {
			if spendingSummary.Totals.TotalAmount != nil {
				totalSpending = *spendingSummary.Totals.TotalAmount
			}
			txCount30d = spendingSummary.Totals.TransactionCount
		}

		// Previous 30-day spending for comparison
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
			prevTotalSpending := *prevSummary.Totals.TotalAmount
			if prevTotalSpending > 0 {
				hasSpendingChange = true
				spendingChangePercent = ((totalSpending - prevTotalSpending) / prevTotalSpending) * 100
			}
		}

		// Is this a liability (credit/loan)?
		isLiability := IsLiabilityAccount(detail.Type)

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

		// Build pagination base for account detail
		acctPaginationBase := buildAccountPaginationBase(r, idStr)
		pageSize := txResult.PageSize
		if pageSize == 0 {
			pageSize = 50
		}
		showingStart := int64((txResult.Page-1)*pageSize + 1)
		showingEnd := min(int64(txResult.Page*pageSize), txResult.Total)

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
			"PageSize":       pageSize,
			"TotalPages":     txResult.TotalPages,
			"Total":          txResult.Total,
			"PaginationBase": acctPaginationBase,
			"ShowingStart":   showingStart,
			"ShowingEnd":     showingEnd,
			"ExportURL":      acctExportURL,
			"FilterStartDate": stringOrEmpty(dateParamPtr(r, "start_date")),
			"FilterEndDate":   stringOrEmpty(dateParamPtr(r, "end_date")),
			"FilterCategory":  r.URL.Query().Get("category"),
			"FilterPending":   r.URL.Query().Get("pending"),
			"FilterSearch":    r.URL.Query().Get("search"),
			// Spending analytics
			"TotalSpending":         totalSpending,
			"TxCount30d":            txCount30d,
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

		// IDOR check: viewers can only view transactions belonging to their user. Editors+ see all.
		if !IsEditor(sm, r) {
			memberUID := SessionUserID(sm, r)
			// Look up the account to determine ownership.
			ownerMatch := false
			if txn.AccountID != nil {
				acctCheck, acctErr := svc.GetAccount(ctx, *txn.AccountID)
				if acctErr == nil && acctCheck.UserID != nil && *acctCheck.UserID == memberUID {
					ownerMatch = true
				}
			}
			if !ownerMatch {
				tr.RenderNotFound(w, r)
				return
			}
		}

		// Annotations are the canonical activity-timeline source. Review
		// lifecycle events flow through tag_added / tag_removed annotations
		// for the needs-review tag.
		annotations, err := svc.ListAnnotations(ctx, idStr)
		if err != nil && !errors.Is(err, service.ErrNotFound) {
			a.Logger.Error("list transaction annotations", "error", err)
		}

		// Load category tree early so the activity timeline can humanize
		// category slugs (rule_applied + category_set rows).
		categoryTree, err := svc.ListCategoryTree(ctx)
		if err != nil {
			a.Logger.Error("list categories for transaction detail", "error", err)
		}

		// Build unified activity timeline from annotations.
		activity := buildActivityTimeline(annotations, categoryDisplayLookup(categoryTree))
		activityDays := groupActivityByDay(activity)

		// Load tags currently attached + the registered-tag list (for the inline
		// add-tag suggestion datalist). Also derive HasPendingReview from the
		// presence of the needs-review tag.
		currentTags, err := svc.ListTransactionTags(ctx, idStr)
		if err != nil {
			a.Logger.Error("list transaction tags for detail", "error", err)
			currentTags = []service.TransactionTagResponse{}
		}
		availableTags, err := svc.ListTags(ctx)
		if err != nil {
			a.Logger.Error("list tags for detail", "error", err)
			availableTags = []service.TagResponse{}
		}
		hasPendingReview := false
		for _, tag := range currentTags {
			if tag.Slug == "needs-review" {
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

		// categoryTree already loaded above for the activity timeline; reused
		// for the inline category picker.

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
			"ActivityDays":      activityDays,
			"HasPendingReview":  hasPendingReview,
			"CurrentTags":       currentTags,
			"AvailableTags":     availableTags,
			"Categories":      categoryTree,
			"Breadcrumbs":     breadcrumbs,
		}
		tr.Render(w, r, "transaction_detail.html", data)
	}
}

// categoryDisplayLookup returns a slug→"Parent › Child" formatter for the
// given category tree. Falls back to the raw slug when a match can't be
// found (e.g. the category was deleted after the annotation was written).
func categoryDisplayLookup(tree []service.CategoryResponse) func(string) string {
	names := make(map[string]string, 64)
	for _, parent := range tree {
		names[parent.Slug] = parent.DisplayName
		for _, child := range parent.Children {
			names[child.Slug] = parent.DisplayName + " › " + child.DisplayName
		}
	}
	return func(slug string) string {
		if slug == "" {
			return ""
		}
		if name, ok := names[slug]; ok {
			return name
		}
		return slug
	}
}

// buildActivityTimeline produces a sorted activity list from annotations.
// Review lifecycle events surface as tag_added/tag_removed on the
// needs-review tag. Comment annotations originally authored as review notes
// (identified by payload.review_id) render inline on their resolution event.
//
// categoryDisplay maps a category slug to a human-readable name
// ("Food & Drink › Groceries"). Pass a no-op (returning slug unchanged) in
// tests that don't need humanization.
func buildActivityTimeline(annotations []service.Annotation, categoryDisplay func(string) string) []service.ActivityEntry {
	if categoryDisplay == nil {
		categoryDisplay = func(s string) string { return s }
	}
	var entries []service.ActivityEntry

	// Annotations.
	for _, a := range annotations {
		switch a.Kind {
		case "comment":
			content, _ := a.Payload["content"].(string)
			// Filter legacy [Review: ...] prefix duplicates from pre-consolidation imports.
			if strings.HasPrefix(content, "[Review: ") {
				continue
			}
			entry := service.ActivityEntry{
				Type:      "comment",
				Timestamp: a.CreatedAt,
				ActorName: a.ActorName,
				ActorType: a.ActorType,
				Detail:    content,
				CommentID: a.ShortID,
			}
			if a.ActorID != nil && *a.ActorID != "" {
				id := *a.ActorID
				entry.ActorID = &id
			}
			entries = append(entries, entry)

		case "rule_applied":
			ruleName, _ := a.Payload["rule_name"].(string)
			field, _ := a.Payload["action_field"].(string)
			value, _ := a.Payload["action_value"].(string)
			appliedBy, _ := a.Payload["applied_by"].(string)
			// Older rows (and any future bug) can land here with rule_name
			// empty and rule_id empty — render a generic subject instead of
			// `Rule "" set category to food_and_drink_groceries`. The
			// template already skips the /rules/<id> link when RuleID is
			// empty, so the fallback text renders as plain copy.
			subject := `Rule "` + ruleName + `"`
			if ruleName == "" {
				subject = "A rule"
			}
			// Humanize the action value for category actions so we render
			// "Food & Drink › Groceries" rather than the raw slug.
			displayValue := value
			if field == "category" {
				displayValue = categoryDisplay(value)
			}
			summary := subject + " applied"
			switch field {
			case "category":
				summary = subject + " set category to " + displayValue
			case "tag":
				summary = subject + " added tag " + value
			case "comment":
				summary = subject + " added a comment"
			}
			how := "during sync"
			if appliedBy == "retroactive" {
				how = "retroactively"
			}
			entries = append(entries, service.ActivityEntry{
				Type:      "rule",
				Timestamp: a.CreatedAt,
				ActorName: how,
				ActorType: "system",
				Summary:   summary,
				RuleName:  ruleName,
				RuleID:    derefOr(a.RuleID, ""),
			})

		case "tag_added":
			source, _ := a.Payload["source"].(string)
			if source == "rule" {
				// Represented separately via the rule_applied annotation
				// written alongside tag_added during sync. Skip to avoid
				// double-rendering. Mirrors the category_set dedup below.
				continue
			}
			slug, _ := a.Payload["slug"].(string)
			note, _ := a.Payload["note"].(string)
			summary := "Added tag " + slug
			entry := service.ActivityEntry{
				Type:      "tag",
				Timestamp: a.CreatedAt,
				ActorName: a.ActorName,
				ActorType: a.ActorType,
				Summary:   summary,
				Detail:    note,
				TagSlug:   slug,
			}
			if a.ActorID != nil && *a.ActorID != "" {
				id := *a.ActorID
				entry.ActorID = &id
			}
			entries = append(entries, entry)

		case "tag_removed":
			source, _ := a.Payload["source"].(string)
			if source == "rule" {
				// Future-proof: if a rule ever emits a rule-sourced tag_removed
				// alongside a rule_applied annotation, dedup the same way as
				// tag_added / category_set so the timeline stays symmetric.
				continue
			}
			slug, _ := a.Payload["slug"].(string)
			note, _ := a.Payload["note"].(string)
			summary := "Removed tag " + slug
			entry := service.ActivityEntry{
				Type:      "tag",
				Timestamp: a.CreatedAt,
				ActorName: a.ActorName,
				ActorType: a.ActorType,
				Summary:   summary,
				Detail:    note,
				TagSlug:   slug,
			}
			if a.ActorID != nil && *a.ActorID != "" {
				id := *a.ActorID
				entry.ActorID = &id
			}
			entries = append(entries, entry)

		case "category_set":
			slug, _ := a.Payload["category_slug"].(string)
			source, _ := a.Payload["source"].(string)
			if source == "rule" {
				// Represented separately via the rule_applied annotation
				// written alongside category_set during sync. Skip to avoid
				// double-rendering.
				continue
			}
			displaySlug := categoryDisplay(slug)
			summary := "Category set to " + displaySlug
			entry := service.ActivityEntry{
				Type:         "category",
				Timestamp:    a.CreatedAt,
				ActorName:    a.ActorName,
				ActorType:    a.ActorType,
				Summary:      summary,
				CategoryName: displaySlug,
			}
			if a.ActorID != nil && *a.ActorID != "" {
				id := *a.ActorID
				entry.ActorID = &id
			}
			entries = append(entries, entry)
		}
	}

	// Sort by timestamp descending (newest first).
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp > entries[j].Timestamp
	})

	return entries
}

// ActivityDayGroup holds activity entries grouped by calendar day (in the
// server's local timezone) for rendering day separators on the transaction
// detail timeline.
type ActivityDayGroup struct {
	Date   string                  // ISO date, e.g. "2026-04-16"
	Label  string                  // Human-friendly: "Today", "Yesterday", "Thursday, April 16"
	Events []service.ActivityEntry // events for this day, newest first (order preserved)
}

// groupActivityByDay groups a sorted-desc activity list into per-day buckets.
// Timestamp is an RFC3339 string on ActivityEntry; entries with unparseable
// timestamps are skipped (they're dropped rather than mis-bucketed). Each
// returned group preserves the relative ordering of the input slice.
func groupActivityByDay(entries []service.ActivityEntry) []ActivityDayGroup {
	if len(entries) == 0 {
		return nil
	}

	now := time.Now()
	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")

	var groups []ActivityDayGroup
	groupIdx := map[string]int{}

	for _, e := range entries {
		t, err := time.Parse(time.RFC3339, e.Timestamp)
		if err != nil {
			continue
		}
		date := t.Local().Format("2006-01-02")

		idx, ok := groupIdx[date]
		if !ok {
			groups = append(groups, ActivityDayGroup{
				Date:  date,
				Label: activityDayLabel(date, today, yesterday),
			})
			idx = len(groups) - 1
			groupIdx[date] = idx
		}
		groups[idx].Events = append(groups[idx].Events, e)
	}

	return groups
}

// activityDayLabel returns "Today", "Yesterday", or "Thursday, April 16" /
// "Thursday, April 16, 2025" for older dates. The long-form weekday/month
// mirrors GitHub's timeline convention and reads well at mobile widths.
func activityDayLabel(dateStr, today, yesterday string) string {
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
	if t.Year() == time.Now().Year() {
		return t.Format("Monday, January 2")
	}
	return t.Format("Monday, January 2, 2006")
}

// derefOr returns *p if non-nil, else def.
func derefOr(p *string, def string) string {
	if p == nil {
		return def
	}
	return *p
}

// CreateTransactionCommentHandler serves POST /admin/api/transactions/{id}/comments.
func CreateTransactionCommentHandler(a *app.App, sm *scs.SessionManager, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		txnID := chi.URLParam(r, "id")
		actor := ActorFromSession(sm, r)

		var input struct {
			Content string `json:"content"`
		}
		if !decodeJSON(w, r, &input) {
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
		"tags", "any_tag",
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
		"tags", "any_tag",
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

// buildAccountPaginationBase returns the query string prefix for account detail pagination links.
func buildAccountPaginationBase(r *http.Request, accountID string) string {
	paginationParams := []string{
		"start_date", "end_date", "category", "pending", "search",
	}
	q := r.URL.Query()
	qs := make([]string, 0, len(paginationParams))
	for _, key := range paginationParams {
		if v := q.Get(key); v != "" {
			qs = append(qs, key+"="+url.QueryEscape(v))
		}
	}
	base := "/accounts/" + accountID + "?page="
	if len(qs) > 0 {
		base = "/accounts/" + accountID + "?" + strings.Join(qs, "&") + "&page="
	}
	return base
}

// BulkUpdateTransactionsAdminHandler serves POST /-/transactions/batch-update.
// Accepts an update_transactions-style body and applies it across many
// transactions. Used by the transactions list's bulk-action bar.
func BulkUpdateTransactionsAdminHandler(a *app.App, sm *scs.SessionManager, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor := ActorFromSession(sm, r)

		var body struct {
			Operations []struct {
				TransactionID string  `json:"transaction_id"`
				CategorySlug  *string `json:"category_slug,omitempty"`
				TagsToAdd     []struct {
					Slug string `json:"slug"`
					Note string `json:"note,omitempty"`
				} `json:"tags_to_add,omitempty"`
				TagsToRemove []struct {
					Slug string `json:"slug"`
					Note string `json:"note,omitempty"`
				} `json:"tags_to_remove,omitempty"`
				Comment *string `json:"comment,omitempty"`
			} `json:"operations"`
			OnError string `json:"on_error,omitempty"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		if len(body.Operations) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "operations required"})
			return
		}
		if len(body.Operations) > 50 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "maximum 50 operations per batch"})
			return
		}

		ops := make([]service.UpdateTransactionsOp, len(body.Operations))
		for i, op := range body.Operations {
			ops[i] = service.UpdateTransactionsOp{
				TransactionID: op.TransactionID,
				CategorySlug:  op.CategorySlug,
				Comment:       op.Comment,
			}
			for _, t := range op.TagsToAdd {
				ops[i].TagsToAdd = append(ops[i].TagsToAdd, service.UpdateTransactionsTagOp{Slug: t.Slug, Note: t.Note})
			}
			for _, t := range op.TagsToRemove {
				ops[i].TagsToRemove = append(ops[i].TagsToRemove, service.UpdateTransactionsTagOp{Slug: t.Slug, Note: t.Note})
			}
		}

		results, err := svc.UpdateTransactions(r.Context(), service.UpdateTransactionsParams{
			Operations: ops,
			OnError:    body.OnError,
			Actor:      actor,
		})
		succeeded := 0
		failed := 0
		for _, rr := range results {
			if rr.Status == "ok" {
				succeeded++
			} else {
				failed++
			}
		}
		payload := map[string]any{
			"results":   results,
			"succeeded": succeeded,
			"failed":    failed,
		}
		if err != nil {
			payload["aborted"] = true
			payload["error"] = err.Error()
			a.Logger.Warn("bulk update transactions aborted", "error", err)
			writeJSON(w, http.StatusUnprocessableEntity, payload)
			return
		}
		writeJSON(w, http.StatusOK, payload)
	}
}

// QuickSearchTransactionsHandler serves GET /-/search/transactions.
// Returns []service.TransactionSummary — the shared DTO used by every
// "transaction-as-a-card" preview surface (command palette, future rule
// preview modal, etc.). Formatting lives in service.ToTransactionSummary.
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
			writeJSON(w, http.StatusOK, []service.TransactionSummary{})
			return
		}

		items := make([]service.TransactionSummary, 0, len(result.Transactions))
		for _, tx := range result.Transactions {
			items = append(items, service.ToTransactionSummary(tx))
		}

		writeJSON(w, http.StatusOK, items)
	}
}
