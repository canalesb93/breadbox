package admin

import (
	"errors"
	"math"
	"net/http"
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

// parseAdminTxFilters reads the standard transaction-list filter query params
// from r and returns an AdminTransactionListParams populated with everything
// except Page/PageSize. Callers control pagination semantics — list/search use
// the user's per_page choice; per-account or single-shot views set their own.
func parseAdminTxFilters(r *http.Request) service.AdminTransactionListParams {
	q := r.URL.Query()
	params := service.AdminTransactionListParams{
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
	return params
}

// TransactionListHandler serves GET /admin/transactions.
func TransactionListHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		params := parseAdminTxFilters(r)
		params.Page = parsePage(r)
		params.PageSize = parsePerPage(r, 50, 25, 50, 100)

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
		FilterStartDate:   q.Get("start_date"),
		FilterEndDate:     q.Get("end_date"),
		FilterAccountID:   stringOrEmpty(in.params.AccountID),
		FilterUserID:      stringOrEmpty(in.params.UserID),
		FilterConnID:      stringOrEmpty(in.params.ConnectionID),
		FilterCategory:    stringOrEmpty(in.params.CategorySlug),
		FilterMinAmount:   q.Get("min_amount"),
		FilterMaxAmount:   q.Get("max_amount"),
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

		params := parseAdminTxFilters(r)
		params.Page = parsePage(r)
		params.PageSize = parsePerPage(r, 50, 25, 50, 100)

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
		if _, ok := parseURLUUIDOrNotFound(w, r, tr, "id"); !ok {
			return
		}
		idStr := chi.URLParam(r, "id")

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

		breadcrumbs := []Breadcrumb{
			{Label: "Connections", Href: "/connections"},
			{Label: detail.InstitutionName, Href: "/connections/" + detail.ConnectionID},
			{Label: accountDisplayName(detail)},
		}

		data := map[string]any{
			"PageTitle":   detail.InstitutionName + " — " + accountDisplayName(detail),
			"CurrentPage": "transactions",
			"CSRFToken":   GetCSRFToken(r),
			"Flash":       GetFlash(ctx, sm),
		}

		// Translate admin breadcrumbs to the components shape the templ
		// component consumes. Same pattern as renderTransactionDetail.
		componentBreadcrumbs := make([]components.Breadcrumb, 0, len(breadcrumbs))
		for _, c := range breadcrumbs {
			componentBreadcrumbs = append(componentBreadcrumbs, components.Breadcrumb{Label: c.Label, Href: c.Href})
		}

		props := pages.AccountDetailProps{
			CSRFToken:             GetCSRFToken(r),
			Breadcrumbs:           componentBreadcrumbs,
			AccountID:             idStr,
			Account:               detail,
			IsLiability:           isLiability,
			HasCreditUtil:         hasCreditUtil,
			CreditUtilization:     creditUtilization,
			TotalSpending:         totalSpending,
			TxCount30d:            txCount30d,
			HasSpendingChange:     hasSpendingChange,
			SpendingChangePercent: spendingChangePercent,
			FilterStartDate:       r.URL.Query().Get("start_date"),
			FilterEndDate:         r.URL.Query().Get("end_date"),
			FilterCategory:        r.URL.Query().Get("category"),
			FilterPending:         r.URL.Query().Get("pending"),
			FilterSearch:          r.URL.Query().Get("search"),
			Transactions:          txResult.Transactions,
			Page:                  txResult.Page,
			PageSize:              pageSize,
			TotalPages:            txResult.TotalPages,
			Total:                 txResult.Total,
			ShowingStart:          showingStart,
			ShowingEnd:            showingEnd,
			PaginationBase:        acctPaginationBase,
			ExportURL:             acctExportURL,
			Categories:            categoryTree,
		}
		renderAccountDetail(tr, w, r, data, props)
	}
}

// renderAccountDetail hosts the account detail templ component inside
// base.html. Mirrors the renderTransactionDetail / renderConnections
// helpers used by the prior templ ports.
func renderAccountDetail(tr *TemplateRenderer, w http.ResponseWriter, r *http.Request, data map[string]any, props pages.AccountDetailProps) {
	tr.RenderWithTempl(w, r, data, pages.AccountDetail(props))
}

func accountDisplayName(detail *service.AdminAccountDetail) string {
	if detail.DisplayName != nil && *detail.DisplayName != "" {
		return *detail.DisplayName
	}
	return detail.Name
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
		annotations, err := svc.ListAnnotations(ctx, idStr, service.ListAnnotationsParams{})
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
		// Load registered-tag list early so tag chip rendering in the
		// timeline has display names + colors without a per-row query.
		timelineTags, err := svc.ListTags(ctx)
		if err != nil {
			a.Logger.Error("list tags for activity timeline", "error", err)
			timelineTags = nil
		}

		// Capture a single now anchor so the day-bucket labels
		// ("Today" / "Yesterday" / weekday) and the per-row relative
		// timestamps ("5 minutes ago" / "yesterday") agree across
		// midnight and timezone boundaries. Threaded into the templ via
		// props.Now → relativeTimeStrAt.
		now := time.Now()
		activity := buildActivityTimeline(annotations, categoryDetailLookup(categoryTree), tagDisplayLookup(timelineTags))
		activityDays := groupActivityByDay(activity, now)

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
		breadcrumbs = append(breadcrumbs, Breadcrumb{Label: txn.ProviderName})

		data := BaseTemplateData(r, sm, "transactions", txn.ProviderName)

		// Translate admin breadcrumbs + activity-day groups to the typed
		// shapes pages.TransactionDetail expects. The component owns its
		// view-model so the handler is the only place that bridges between
		// the admin and pages packages.
		componentBreadcrumbs := make([]components.Breadcrumb, 0, len(breadcrumbs))
		for _, c := range breadcrumbs {
			componentBreadcrumbs = append(componentBreadcrumbs, components.Breadcrumb{Label: c.Label, Href: c.Href})
		}
		componentActivityDays := make([]pages.ActivityDayGroup, 0, len(activityDays))
		for _, day := range activityDays {
			componentActivityDays = append(componentActivityDays, pages.ActivityDayGroup{
				Label:  day.Label,
				Events: day.Events,
			})
		}

		props := pages.TransactionDetailProps{
			CSRFToken:        GetCSRFToken(r),
			Breadcrumbs:      componentBreadcrumbs,
			Transaction:      txn,
			TransactionID:    idStr,
			AccountID:        accountID,
			AccountName:      accountName,
			UserName:         userName,
			InstitutionName:  institutionName,
			AccountMask:      accountMask,
			AccountType:      accountType,
			ConnectionID:     connectionID,
			Account:          account,
			Activity:         activity,
			ActivityDays:     componentActivityDays,
			Now:              now,
			HasPendingReview: hasPendingReview,
			CurrentTags:      currentTags,
			AvailableTags:    availableTags,
			Categories:       categoryTree,
			MaxCommentLength: service.MaxCommentLength,
		}
		renderTransactionDetail(tr, w, r, data, props)
	}
}

// renderTransactionDetail hosts the transaction detail templ component
// inside base.html. Mirrors the renderSettings / renderConnectionDetail
// helpers used by the prior templ ports.
func renderTransactionDetail(tr *TemplateRenderer, w http.ResponseWriter, r *http.Request, data map[string]any, props pages.TransactionDetailProps) {
	tr.RenderWithTempl(w, r, data, pages.TransactionDetail(props))
}

// categoryDisplay carries the presentation metadata needed to render a
// category_set timeline row: a "Parent › Child" name plus the registered
// color + icon used elsewhere in the app for that category.
type categoryDisplay struct {
	DisplayName string
	Color       *string
	Icon        *string
}

// categoryDetailLookup returns a slug → presentation lookup built from the
// category tree. Falls back to the raw slug with nil color/icon when a match
// can't be found (e.g. the category was deleted after the annotation was
// written).
func categoryDetailLookup(tree []service.CategoryResponse) func(string) categoryDisplay {
	by := make(map[string]categoryDisplay, 64)
	for _, parent := range tree {
		by[parent.Slug] = categoryDisplay{DisplayName: parent.DisplayName, Color: parent.Color, Icon: parent.Icon}
		for _, child := range parent.Children {
			color := child.Color
			if color == nil {
				color = parent.Color
			}
			icon := child.Icon
			if icon == nil {
				icon = parent.Icon
			}
			by[child.Slug] = categoryDisplay{
				DisplayName: parent.DisplayName + " › " + child.DisplayName,
				Color:       color,
				Icon:        icon,
			}
		}
	}
	return func(slug string) categoryDisplay {
		if slug == "" {
			return categoryDisplay{}
		}
		if d, ok := by[slug]; ok {
			return d
		}
		return categoryDisplay{DisplayName: slug}
	}
}

// tagDisplay carries presentation metadata for rendering a tag chip on a
// timeline row. See tagDisplayLookup and the tag_added / tag_removed cases
// in buildActivityTimeline.
type tagDisplay struct {
	DisplayName string
	Color       *string
}

// tagDisplayLookup returns a slug -> presentation lookup built from the
// registered-tag list. Slugs without a registered tag fall back to the
// slug itself with a nil color so the template renders a plain chip.
func tagDisplayLookup(tags []service.TagResponse) func(string) tagDisplay {
	by := make(map[string]tagDisplay, len(tags))
	for _, t := range tags {
		by[t.Slug] = tagDisplay{DisplayName: t.DisplayName, Color: t.Color}
	}
	return func(slug string) tagDisplay {
		if slug == "" {
			return tagDisplay{}
		}
		if d, ok := by[slug]; ok {
			return d
		}
		// Fall back to the slug as the display name — preserves legibility
		// for tags that have since been deleted.
		return tagDisplay{DisplayName: slug}
	}
}

// buildActivityTimeline produces a sorted activity list from annotations
// and is the admin-handler bridge between the service-layer
// service.Annotation shape (which carries derived fields like Summary, Action,
// Origin) and the UI-layer service.ActivityEntry shape (which carries
// presentation extras like TagColor and ReviewStatus).
//
// Dedup and summary derivation live in service.EnrichAnnotations; this
// function delegates to it via the supplied lookups, then maps each
// enriched annotation to its ActivityEntry.
//
// categoryDetail maps a category slug to a display name + color + icon for
// rendering category_set rows. Pass nil to use raw slugs and skip color/icon.
//
// tagDisplayFn maps a tag slug to its display name + color for rendering a
// tag chip on tag_added / tag_removed rows. Pass nil to use raw slugs.
func buildActivityTimeline(annotations []service.Annotation, categoryDetail func(string) categoryDisplay, tagDisplayFn func(string) tagDisplay) []service.ActivityEntry {
	if len(annotations) == 0 {
		return nil
	}
	// Map the admin's tagDisplay (name + color) to the service-layer tag
	// display closure (name only). Color stays here in the UI mapping pass.
	var tagNameLookup func(string) string
	if tagDisplayFn != nil {
		tagNameLookup = func(slug string) string {
			td := tagDisplayFn(slug)
			if td.DisplayName != "" {
				return td.DisplayName
			}
			return slug
		}
	}
	// Same translation for the category lookup: enrichment only needs the
	// display name; color + icon stay on the admin side.
	var categoryNameLookup func(string) string
	if categoryDetail != nil {
		categoryNameLookup = func(slug string) string {
			d := categoryDetail(slug)
			if d.DisplayName != "" {
				return d.DisplayName
			}
			return slug
		}
	}

	enriched := service.EnrichAnnotations(annotations, service.EnrichOptions{
		TagDisplay:      tagNameLookup,
		CategoryDisplay: categoryNameLookup,
	})

	entries := make([]service.ActivityEntry, 0, len(enriched))
	for _, a := range enriched {
		entry, ok := activityEntryFromAnnotation(a, tagDisplayFn, categoryDetail)
		if !ok {
			continue
		}
		entries = append(entries, entry)
	}

	// Sort by timestamp ascending (oldest first → newest last). The composer
	// sits at the bottom of the timeline so new bubbles appear right where
	// the user typed them.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp < entries[j].Timestamp
	})

	return entries
}

// activityEntryFromAnnotation maps a single enriched service.Annotation to
// the UI's ActivityEntry shape. Returns (zero, false) for unknown kinds so
// the caller can drop them. Tag rows pull color from tagDisplayFn since
// color is a presentation concern that lives in the admin layer; same for
// the category color+icon on category_set rows.
func activityEntryFromAnnotation(a service.Annotation, tagDisplayFn func(string) tagDisplay, categoryDetail func(string) categoryDisplay) (service.ActivityEntry, bool) {
	switch a.Kind {
	case "comment":
		entry := service.ActivityEntry{
			Type:               "comment",
			Timestamp:          a.CreatedAt,
			ActorName:          a.ActorName,
			ActorType:          a.ActorType,
			ActorAvatarVersion: a.ActorAvatarVersion,
			Detail:             a.Content,
			CommentID:          a.ShortID,
			IsDeleted:          a.IsDeleted,
		}
		// Tombstoned comments don't echo their original body and don't
		// expose a CommentID — there's nothing left to delete a second
		// time, and the trash button on the row would be misleading.
		if a.IsDeleted {
			entry.Detail = ""
			entry.CommentID = ""
			entry.Summary = a.Summary
		}
		if a.ActorID != nil && *a.ActorID != "" {
			id := *a.ActorID
			entry.ActorID = &id
		}
		return entry, true

	case "rule_applied":
		// Subject carries the bare rule name in enrichment; the UI prefers
		// the full pre-formatted Summary phrase but with the trailing
		// "during sync" / "retroactively" stripped off (Origin renders
		// it separately as a meta pill on the timeline row).
		summary := a.Summary
		if a.Origin != "" {
			summary = strings.TrimSuffix(summary, " "+a.Origin)
		}
		return service.ActivityEntry{
			Type:      "rule",
			Timestamp: a.CreatedAt,
			ActorName: "",
			ActorType: "system",
			Summary:   summary,
			RuleName:  a.RuleName,
			RuleID:    derefOr(a.RuleID, ""),
			Origin:    a.Origin,
		}, true

	case "tag_added", "tag_removed":
		// Look up color separately — service-layer enrichment doesn't
		// carry presentation metadata.
		var color *string
		display := a.Subject
		if tagDisplayFn != nil {
			td := tagDisplayFn(a.TagSlug)
			color = td.Color
			if td.DisplayName != "" {
				display = td.DisplayName
			}
		}
		summary := "Added tag"
		action := "added"
		if a.Kind == "tag_removed" {
			summary = "Removed tag"
			action = "removed"
		}
		entry := service.ActivityEntry{
			Type:               "tag",
			Timestamp:          a.CreatedAt,
			ActorName:          a.ActorName,
			ActorType:          a.ActorType,
			ActorAvatarVersion: a.ActorAvatarVersion,
			Summary:            summary,
			Detail:             a.Note,
			TagSlug:            a.TagSlug,
			TagDisplayName:     display,
			TagColor:           color,
			TagAction:          action,
		}
		if a.ActorID != nil && *a.ActorID != "" {
			id := *a.ActorID
			entry.ActorID = &id
		}
		return entry, true

	case "category_set":
		display := a.Subject
		var color, icon *string
		if categoryDetail != nil {
			d := categoryDetail(a.CategorySlug)
			if d.DisplayName != "" {
				display = d.DisplayName
			}
			color = d.Color
			icon = d.Icon
		}
		entry := service.ActivityEntry{
			Type:               "category",
			Timestamp:          a.CreatedAt,
			ActorName:          a.ActorName,
			ActorType:          a.ActorType,
			ActorAvatarVersion: a.ActorAvatarVersion,
			Summary:            "Category set to " + display,
			CategoryName:       display,
			CategoryColor:      color,
			CategoryIcon:       icon,
		}
		if a.ActorID != nil && *a.ActorID != "" {
			id := *a.ActorID
			entry.ActorID = &id
		}
		return entry, true
	}
	return service.ActivityEntry{}, false
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
// returned group preserves the relative ordering of the input slice. The
// now anchor is passed in (rather than read via time.Now()) so the bucket
// labels share the same reference clock as the per-row relative timestamps.
func groupActivityByDay(entries []service.ActivityEntry, now time.Time) []ActivityDayGroup {
	if len(entries) == 0 {
		return nil
	}

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
				Label: activityDayLabel(date, today, yesterday, now),
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
// now provides the reference year for the same-year shortening so the
// label cannot disagree with the day-grouping anchor across New Year.
func activityDayLabel(dateStr, today, yesterday string, now time.Time) string {
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
	if t.Year() == now.Year() {
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

// txFilterParams lists the filter keys preserved across pagination and CSV
// export links on the main transactions list. per_page is appended for
// pagination but omitted from the export URL.
var txFilterParams = []string{
	"start_date", "end_date", "account_id", "user_id",
	"connection_id", "category", "min_amount", "max_amount",
	"pending", "search", "search_mode", "search_field", "sort",
	"tags", "any_tag",
}

// buildPaginationBase returns the query string for pagination links (all params except page).
func buildPaginationBase(r *http.Request) string {
	keys := append(append([]string{}, txFilterParams...), "per_page")
	return paginationBase("/transactions", pickValues(r, keys), "page")
}

// buildExportURL returns the full CSV export URL with the current filter params.
func buildExportURL(r *http.Request) string {
	encoded := pickValues(r, txFilterParams).Encode()
	if encoded == "" {
		return "/-/transactions/export-csv"
	}
	return "/-/transactions/export-csv?" + encoded
}

// buildAccountPaginationBase returns the query string prefix for account detail pagination links.
func buildAccountPaginationBase(r *http.Request, accountID string) string {
	return paginationBase("/accounts/"+accountID, pickValues(r, []string{
		"start_date", "end_date", "category", "pending", "search",
	}), "page")
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
				} `json:"tags_to_add,omitempty"`
				TagsToRemove []struct {
					Slug string `json:"slug"`
				} `json:"tags_to_remove,omitempty"`
				Comment *string `json:"comment,omitempty"`
			} `json:"operations"`
			OnError string `json:"on_error,omitempty"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		if len(body.Operations) == 0 {
			writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "operations required")
			return
		}
		if len(body.Operations) > 50 {
			writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "maximum 50 operations per batch")
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
				ops[i].TagsToAdd = append(ops[i].TagsToAdd, service.UpdateTransactionsTagOp{Slug: t.Slug})
			}
			for _, t := range op.TagsToRemove {
				ops[i].TagsToRemove = append(ops[i].TagsToRemove, service.UpdateTransactionsTagOp{Slug: t.Slug})
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
