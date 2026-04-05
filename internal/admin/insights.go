package admin

import (
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"breadbox/internal/app"
	"breadbox/internal/service"
)

// paymentProcessorPatterns contains ILIKE patterns for merchants that are
// credit card companies, banks, or payment processors — not real merchants.
// These are filtered out of Top Merchants and related merchant rankings to
// avoid showing "American Express" or "Chase" as top spending destinations.
var paymentProcessorPatterns = []string{
	// Credit card issuers / banks
	"american express%", "amex%",
	"chase%", "jpmorgan%",
	"capital one%", "capitalone%",
	"citibank%", "citi card%", "citi %",
	"discover%",
	"bank of america%", "bofa%",
	"wells fargo%",
	"us bank%", "usbank%",
	"barclays%",
	"synchrony%",
	"td bank%",
	"pnc bank%",
	"truist%",
	"ally bank%",
	"marcus%goldman%",
	// Payment processors / transfers
	"paypal%",
	"venmo%",
	"zelle%",
	"cash app%", "square cash%",
	"apple cash%",
	// Credit card payment labels
	"%payment thank you%",
	"%autopay%",
	"%credit card payment%",
	"%card payment%",
	"%balance payment%",
	"%minimum payment%",
	// Transfers
	"%transfer to%",
	"%transfer from%",
	"%ach transfer%",
	"%wire transfer%",
	"%online transfer%",
}

// buildPaymentProcessorExclusion returns a SQL WHERE clause fragment that
// excludes payment processors from merchant queries. The merchantExpr should
// be the SQL expression for the merchant name (e.g., "COALESCE(NULLIF(merchant_name, ''), name)").
// Returns the clause fragment and the parameter values to append.
func buildPaymentProcessorExclusion(merchantExpr string, startParam int) (string, []any) {
	if len(paymentProcessorPatterns) == 0 {
		return "", nil
	}
	var conditions []string
	var params []any
	for i, pattern := range paymentProcessorPatterns {
		conditions = append(conditions, fmt.Sprintf("LOWER(%s) NOT LIKE $%d", merchantExpr, startParam+i))
		params = append(params, pattern)
	}
	return " AND " + strings.Join(conditions, " AND "), params
}

// InsightsHandler serves GET /admin/insights — the spending insights page.
func InsightsHandler(a *app.App, svc *service.Service, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Parse date range from query param (default 30 days).
		chartDays := 30
		if d := r.URL.Query().Get("days"); d != "" {
			switch d {
			case "7":
				chartDays = 7
			case "30":
				chartDays = 30
			case "90":
				chartDays = 90
			case "365":
				chartDays = 365
			}
		}
		chartStart := time.Now().AddDate(0, 0, -chartDays)

		// ── Shared palette and time constants ──
		categoryPalette := []string{
			"oklch(0.62 0.15 250)", // blue
			"oklch(0.64 0.16 160)", // teal
			"oklch(0.66 0.14 35)",  // warm amber
			"oklch(0.60 0.16 300)", // purple
			"oklch(0.66 0.12 80)",  // olive
			"oklch(0.58 0.14 200)", // slate blue
			"oklch(0.64 0.14 120)", // green
			"oklch(0.60 0.13 350)", // rose
			"oklch(0.55 0 0)",      // gray (for "Other")
		}

		today := time.Now()
		monthStart := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, today.Location())
		daysElapsed := today.Day()
		daysInMonth := time.Date(today.Year(), today.Month()+1, 0, 0, 0, 0, 0, today.Location()).Day()
		daysRemaining := daysInMonth - daysElapsed

		chartGroupBy := "day"
		if chartDays == 365 {
			chartGroupBy = "month"
		}

		prevStart := time.Now().AddDate(0, 0, -chartDays*2)
		prevEnd := time.Now().AddDate(0, 0, -chartDays)

		lastMonthStart := time.Date(today.Year(), today.Month()-1, 1, 0, 0, 0, 0, today.Location())
		lastMonthEnd := monthStart
		lastMonthSameDay := time.Date(today.Year(), today.Month()-1, 1, 0, 0, 0, 0, today.Location())
		lastMonthDaysInMonth := time.Date(lastMonthSameDay.Year(), lastMonthSameDay.Month()+1, 0, 0, 0, 0, 0, today.Location()).Day()
		sameDayOfLastMonth := daysElapsed
		if sameDayOfLastMonth > lastMonthDaysInMonth {
			sameDayOfLastMonth = lastMonthDaysInMonth
		}
		lastMonthSameDayEnd := time.Date(lastMonthSameDay.Year(), lastMonthSameDay.Month(), sameDayOfLastMonth+1, 0, 0, 0, 0, today.Location())

		recurringLookback := time.Now().AddDate(0, 0, -90)
		compStart := time.Date(today.Year(), today.Month()-3, 1, 0, 0, 0, 0, today.Location())
		compEnd := time.Now().AddDate(0, 0, 1)

		// ── Type definitions ──
		type CategorySpend struct {
			Name    string
			Color   string
			Amount  float64
			Percent float64
		}
		type CategoryMerchant struct {
			Name   string  `json:"name"`
			Amount float64 `json:"amount"`
			Count  int     `json:"count"`
		}
		type MerchantSpend struct {
			Name      string
			Category  string
			Total     float64
			Count     int
			AvgAmount float64
			Percent   float64
		}
		type DayOfWeekSpend struct {
			DayShort  string
			DayFull   string
			Total     float64
			Count     int
			Intensity float64
		}
		type RecurringCharge struct {
			Name           string
			Category       string
			Amount         float64
			Frequency      string
			TxCount        int
			TotalSpent     float64
			LastChargeDate string
			MonthlyEst     float64
		}
		type MonthlyCompRow struct {
			Category      string
			CategoryColor string
			Amounts       []float64
			Total         float64
			Change        float64
			HasChange     bool
		}

		// ══════════════════════════════════════════════════════════════
		// PARALLEL QUERY PHASE: Fire all independent DB queries at once
		// ══════════════════════════════════════════════════════════════
		var wg sync.WaitGroup

		// Results from parallel queries (each goroutine writes to its own variable).
		var categorySummary *service.TransactionSummaryResult
		var dailySummary *service.TransactionSummaryResult
		var dailyIncomeSummary *service.TransactionSummaryResult
		var prevSummary *service.TransactionSummaryResult
		var incomeSummary *service.TransactionSummaryResult
		var currentMonthSummary *service.TransactionSummaryResult
		var currentMonthIncomeSummary *service.TransactionSummaryResult
		var lastMonthSummary *service.TransactionSummaryResult
		var lastMonthPaceSummary *service.TransactionSummaryResult
		var compSummary *service.TransactionSummaryResult
		categoryDrilldown := make(map[string][]CategoryMerchant)
		var topMerchants []MerchantSpend
		var maxMerchantSpend float64
		dowSpending := make([]DayOfWeekSpend, 7)
		var maxDaySpend float64
		var recurringCharges []RecurringCharge
		var totalRecurringMonthly float64
		var hasRecurringData bool
		var sparklineSpending []float64
		var sparklineIncome []float64
		var netWorth, totalAssets, totalLiabilities float64
		var netWorthLabelsJSON, netWorthValuesJSON template.JS

		// 0. Net worth: fetch accounts and compute balances + trend
		wg.Add(1)
		go func() {
			defer wg.Done()
			accounts, err := svc.ListAccounts(ctx, nil)
			if err != nil {
				a.Logger.Error("list accounts for insights net worth", "error", err)
				return
			}
			for _, acct := range accounts {
				if acct.BalanceCurrent == nil {
					continue
				}
				bal := *acct.BalanceCurrent
				isLiability := acct.Type == "credit" || acct.Type == "loan"
				if isLiability {
					totalLiabilities += math.Abs(bal)
					netWorth -= math.Abs(bal)
				} else {
					totalAssets += bal
					netWorth += bal
				}
			}
			// Net worth trend: work backwards from current balance using daily transaction totals.
			netWorthTrendStart := time.Now().AddDate(0, 0, -chartDays)
			dailyNetSummary, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
				GroupBy:   "day",
				StartDate: &netWorthTrendStart,
			})
			if err != nil {
				a.Logger.Error("daily net summary for net worth trend", "error", err)
				return
			}
			dailyNetMap := make(map[string]float64)
			if dailyNetSummary != nil {
				for _, row := range dailyNetSummary.Summary {
					if row.Period != nil {
						dailyNetMap[*row.Period] = row.TotalAmount
					}
				}
			}
			now := time.Now()
			nwDays := chartDays
			if nwDays > 90 {
				nwDays = 90
			}
			nwLabels := make([]string, nwDays+1)
			nwValues := make([]float64, nwDays+1)
			nwValues[nwDays] = netWorth
			nwLabels[nwDays] = now.Format("2006-01-02")
			for i := nwDays - 1; i >= 0; i-- {
				day := now.AddDate(0, 0, -(nwDays - i))
				nwLabels[i] = day.Format("2006-01-02")
				nextDayStr := now.AddDate(0, 0, -(nwDays - i - 1)).Format("2006-01-02")
				nwValues[i] = nwValues[i+1] + dailyNetMap[nextDayStr]
			}
			if lb, err := json.Marshal(nwLabels); err == nil {
				netWorthLabelsJSON = template.JS(lb)
			}
			if vb, err := json.Marshal(nwValues); err == nil {
				netWorthValuesJSON = template.JS(vb)
			}
		}()

		// 1. Category summary
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
				GroupBy:      "category",
				StartDate:    &chartStart,
				SpendingOnly: true,
			})
			if err != nil {
				a.Logger.Error("category summary", "error", err)
				return
			}
			categorySummary = result
		}()

		// 2. Category drilldown (excludes payment processors)
		wg.Add(1)
		go func() {
			defer wg.Done()
			drillExcl, drillExclParams := buildPaymentProcessorExclusion("COALESCE(NULLIF(t.merchant_name, ''), t.name)", 2)
			drillQueryParams := append([]any{chartStart}, drillExclParams...)
			rows, err := a.DB.Query(ctx, `
				WITH ranked AS (
					SELECT
						COALESCE(cat.display_name, t.category_primary, 'Uncategorized') AS cat,
						COALESCE(NULLIF(t.merchant_name, ''), t.name) AS merchant,
						SUM(t.amount) AS total,
						COUNT(*)::int AS tx_count,
						ROW_NUMBER() OVER (PARTITION BY COALESCE(cat.display_name, t.category_primary, 'Uncategorized') ORDER BY SUM(t.amount) DESC) AS rn
					FROM transactions t
					LEFT JOIN categories cat ON t.category_id = cat.id
					WHERE t.deleted_at IS NULL AND t.date >= $1 AND t.amount > 0 AND t.pending = false
					`+drillExcl+`
					GROUP BY COALESCE(cat.display_name, t.category_primary, 'Uncategorized'), COALESCE(NULLIF(t.merchant_name, ''), t.name)
				)
				SELECT cat, merchant, total, tx_count
				FROM ranked
				WHERE rn <= 8
				ORDER BY cat, total DESC
			`, drillQueryParams...)
			if err != nil {
				a.Logger.Error("category drilldown query", "error", err)
				return
			}
			defer rows.Close()
			for rows.Next() {
				var cat, merchant string
				var total float64
				var count int
				if err := rows.Scan(&cat, &merchant, &total, &count); err != nil {
					continue
				}
				categoryDrilldown[cat] = append(categoryDrilldown[cat], CategoryMerchant{
					Name:   merchant,
					Amount: total,
					Count:  count,
				})
			}
		}()

		// 3. Daily spending trend
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
				GroupBy:      chartGroupBy,
				StartDate:    &chartStart,
				SpendingOnly: true,
			})
			if err != nil {
				a.Logger.Error("daily summary", "error", err)
				return
			}
			dailySummary = result
		}()

		// 4. Daily income
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
				GroupBy:   chartGroupBy,
				StartDate: &chartStart,
			})
			if err != nil {
				a.Logger.Error("daily income summary", "error", err)
				return
			}
			dailyIncomeSummary = result
		}()

		// 5. Previous period spending
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
				GroupBy:      "category",
				StartDate:    &prevStart,
				EndDate:      &prevEnd,
				SpendingOnly: true,
			})
			if err != nil {
				a.Logger.Error("previous period summary", "error", err)
				return
			}
			prevSummary = result
		}()

		// 6. Total income
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
				GroupBy:   "day",
				StartDate: &chartStart,
			})
			if err != nil {
				a.Logger.Error("income summary", "error", err)
				return
			}
			incomeSummary = result
		}()

		// 7. Current month spending
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
				GroupBy:      "day",
				StartDate:    &monthStart,
				SpendingOnly: true,
			})
			if err != nil {
				a.Logger.Error("current month spending", "error", err)
				return
			}
			currentMonthSummary = result
		}()

		// 8. Current month income
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
				GroupBy:   "day",
				StartDate: &monthStart,
			})
			if err != nil {
				a.Logger.Error("current month income", "error", err)
				return
			}
			currentMonthIncomeSummary = result
		}()

		// 9. Last month spending
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
				GroupBy:      "day",
				StartDate:    &lastMonthStart,
				EndDate:      &lastMonthEnd,
				SpendingOnly: true,
			})
			if err != nil {
				a.Logger.Error("last month spending", "error", err)
				return
			}
			lastMonthSummary = result
		}()

		// 10. Last month pace spending
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
				GroupBy:      "day",
				StartDate:    &lastMonthSameDay,
				EndDate:      &lastMonthSameDayEnd,
				SpendingOnly: true,
			})
			if err != nil {
				a.Logger.Error("last month pace spending", "error", err)
				return
			}
			lastMonthPaceSummary = result
		}()

		// 11. Top merchants (excludes payment processors & credit card companies)
		wg.Add(1)
		go func() {
			defer wg.Done()
			merchantExcl, exclParams := buildPaymentProcessorExclusion("COALESCE(NULLIF(merchant_name, ''), name)", 2)
			queryParams := append([]any{chartStart}, exclParams...)
			rows, err := a.DB.Query(ctx, `
				SELECT
					COALESCE(NULLIF(merchant_name, ''), name) AS merchant,
					COUNT(*)::int AS tx_count,
					SUM(amount) AS total,
					AVG(amount) AS avg_amount,
					MODE() WITHIN GROUP (ORDER BY COALESCE(category_primary, '')) AS top_category
				FROM transactions
				WHERE deleted_at IS NULL AND date >= $1 AND amount > 0 AND pending = false
				`+merchantExcl+`
				GROUP BY COALESCE(NULLIF(merchant_name, ''), name)
				ORDER BY SUM(amount) DESC
				LIMIT 10
			`, queryParams...)
			if err != nil {
				a.Logger.Error("top merchants query", "error", err)
				return
			}
			defer rows.Close()
			for rows.Next() {
				var m MerchantSpend
				var cat *string
				if err := rows.Scan(&m.Name, &m.Count, &m.Total, &m.AvgAmount, &cat); err != nil {
					a.Logger.Error("top merchants scan", "error", err)
					continue
				}
				if cat != nil && *cat != "" {
					m.Category = *cat
				}
				topMerchants = append(topMerchants, m)
				if m.Total > maxMerchantSpend {
					maxMerchantSpend = m.Total
				}
			}
		}()

		// 12. Day-of-week spending
		wg.Add(1)
		go func() {
			defer wg.Done()
			dayNames := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
			dayFullNames := []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}
			for i := range dowSpending {
				dowSpending[i].DayShort = dayNames[i]
				dowSpending[i].DayFull = dayFullNames[i]
			}
			rows, err := a.DB.Query(ctx, `
				SELECT
					EXTRACT(ISODOW FROM date)::int AS dow,
					SUM(amount) AS total,
					COUNT(*)::int AS tx_count
				FROM transactions
				WHERE deleted_at IS NULL AND date >= $1 AND amount > 0 AND pending = false
				GROUP BY EXTRACT(ISODOW FROM date)::int
				ORDER BY dow
			`, chartStart)
			if err != nil {
				a.Logger.Error("day-of-week query", "error", err)
				return
			}
			defer rows.Close()
			for rows.Next() {
				var dow int
				var total float64
				var count int
				if err := rows.Scan(&dow, &total, &count); err != nil {
					continue
				}
				idx := dow - 1
				if idx >= 0 && idx < 7 {
					dowSpending[idx].Total = total
					dowSpending[idx].Count = count
					if total > maxDaySpend {
						maxDaySpend = total
					}
				}
			}
		}()

		// 13. Recurring charges (excludes payment processors)
		wg.Add(1)
		go func() {
			defer wg.Done()
			recurExcl, recurExclParams := buildPaymentProcessorExclusion("COALESCE(NULLIF(merchant_name, ''), name)", 2)
			recurQueryParams := append([]any{recurringLookback}, recurExclParams...)
			rows, err := a.DB.Query(ctx, `
				WITH merchant_stats AS (
					SELECT
						COALESCE(NULLIF(merchant_name, ''), name) AS merchant,
						MODE() WITHIN GROUP (ORDER BY amount) AS typical_amount,
						MODE() WITHIN GROUP (ORDER BY COALESCE(category_primary, '')) AS top_category,
						COUNT(*) AS tx_count,
						SUM(amount) AS total_spent,
						MAX(date) AS last_date,
						MIN(date) AS first_date,
						CASE WHEN AVG(amount) > 0 THEN COALESCE(STDDEV(amount), 0) / AVG(amount) ELSE 1 END AS cv
					FROM transactions
					WHERE deleted_at IS NULL
						AND date >= $1
						AND amount > 0
						AND pending = false
						`+recurExcl+`
					GROUP BY COALESCE(NULLIF(merchant_name, ''), name)
					HAVING COUNT(*) >= 2
				)
				SELECT
					merchant,
					typical_amount,
					top_category,
					tx_count,
					total_spent,
					TO_CHAR(last_date, 'Mon DD') AS last_charge,
					(last_date - first_date)::float / NULLIF(tx_count - 1, 0) AS avg_days_between,
					cv
				FROM merchant_stats
				WHERE cv < 0.3
					AND typical_amount >= 3
				ORDER BY total_spent DESC
				LIMIT 12
			`, recurQueryParams...)
			if err != nil {
				a.Logger.Error("recurring charges query", "error", err)
				return
			}
			defer rows.Close()
			for rows.Next() {
				var rc RecurringCharge
				var cat *string
				var avgDaysBetween *float64
				var cv float64
				if err := rows.Scan(&rc.Name, &rc.Amount, &cat, &rc.TxCount, &rc.TotalSpent, &rc.LastChargeDate, &avgDaysBetween, &cv); err != nil {
					a.Logger.Error("recurring charges scan", "error", err)
					continue
				}
				if cat != nil && *cat != "" {
					rc.Category = *cat
				}
				if avgDaysBetween != nil && *avgDaysBetween > 0 {
					days := *avgDaysBetween
					switch {
					case days >= 25 && days <= 35:
						rc.Frequency = "monthly"
						rc.MonthlyEst = rc.Amount
					case days >= 12 && days <= 18:
						rc.Frequency = "biweekly"
						rc.MonthlyEst = rc.Amount * 2
					case days >= 5 && days <= 9:
						rc.Frequency = "weekly"
						rc.MonthlyEst = rc.Amount * 4.33
					case days >= 55 && days <= 65:
						rc.Frequency = "bimonthly"
						rc.MonthlyEst = rc.Amount / 2
					case days >= 80 && days <= 100:
						rc.Frequency = "quarterly"
						rc.MonthlyEst = rc.Amount / 3
					case days >= 350 && days <= 380:
						rc.Frequency = "yearly"
						rc.MonthlyEst = rc.Amount / 12
					default:
						rc.Frequency = "recurring"
						rc.MonthlyEst = rc.TotalSpent / 3
					}
				} else {
					rc.Frequency = "recurring"
					rc.MonthlyEst = rc.TotalSpent / 3
				}
				totalRecurringMonthly += rc.MonthlyEst
				recurringCharges = append(recurringCharges, rc)
			}
			hasRecurringData = len(recurringCharges) > 0
		}()

		// 14. Monthly comparison
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
				GroupBy:      "category_month",
				StartDate:    &compStart,
				EndDate:      &compEnd,
				SpendingOnly: true,
			})
			if err != nil {
				a.Logger.Error("monthly comparison summary", "error", err)
				return
			}
			compSummary = result
		}()

		// 15. Sparkline data -- last 7 days of daily spending and income for summary pills
		wg.Add(1)
		go func() {
			defer wg.Done()
			sparkStart := time.Now().AddDate(0, 0, -7)
			rows, err := a.DB.Query(ctx, `
				WITH days AS (
					SELECT generate_series($1::date, CURRENT_DATE, '1 day')::date AS d
				)
				SELECT
					days.d,
					COALESCE(SUM(CASE WHEN t.amount > 0 THEN t.amount ELSE 0 END), 0) AS spending,
					COALESCE(SUM(CASE WHEN t.amount < 0 THEN -t.amount ELSE 0 END), 0) AS income
				FROM days
				LEFT JOIN transactions t ON t.date = days.d AND t.deleted_at IS NULL AND t.pending = false
				GROUP BY days.d
				ORDER BY days.d
			`, sparkStart)
			if err != nil {
				a.Logger.Error("sparkline query", "error", err)
				return
			}
			defer rows.Close()
			for rows.Next() {
				var d time.Time
				var spending, income float64
				if err := rows.Scan(&d, &spending, &income); err != nil {
					continue
				}
				sparklineSpending = append(sparklineSpending, spending)
				sparklineIncome = append(sparklineIncome, income)
			}
		}()

		// Wait for all parallel queries to complete.
		wg.Wait()

		// ══════════════════════════════════════════════════════════
		// POST-PROCESSING: Derive computed values from query results
		// ══════════════════════════════════════════════════════════

		// ── Process category summary ──
		var categoryLabelsJSON, categoryAmountsJSON, categoryColorsJSON template.JS
		var topCategories []CategorySpend
		if categorySummary != nil && len(categorySummary.Summary) > 0 {
			labels := make([]string, 0, len(categorySummary.Summary))
			amounts := make([]float64, 0, len(categorySummary.Summary))
			colors := make([]string, 0, len(categorySummary.Summary))
			for i, row := range categorySummary.Summary {
				label := "Uncategorized"
				if row.Category != nil && *row.Category != "" {
					label = *row.Category
				}
				labels = append(labels, label)
				amounts = append(amounts, row.TotalAmount)
				color := ""
				if row.CategoryColor != nil && *row.CategoryColor != "" {
					color = *row.CategoryColor
				} else {
					color = categoryPalette[i%len(categoryPalette)]
				}
				colors = append(colors, color)
				topCategories = append(topCategories, CategorySpend{Name: label, Color: color, Amount: row.TotalAmount})
			}
			if lb, err := json.Marshal(labels); err == nil {
				categoryLabelsJSON = template.JS(lb)
			}
			if ab, err := json.Marshal(amounts); err == nil {
				categoryAmountsJSON = template.JS(ab)
			}
			if cb, err := json.Marshal(colors); err == nil {
				categoryColorsJSON = template.JS(cb)
			}
		}
		if len(topCategories) > 8 {
			topCategories = topCategories[:8]
		}
		if catTotal := func() float64 {
			var t float64
			for _, c := range topCategories {
				t += c.Amount
			}
			return t
		}(); catTotal > 0 {
			for i := range topCategories {
				topCategories[i].Percent = (topCategories[i].Amount / catTotal) * 100
			}
		}

		// ── Category drilldown JSON ──
		var categoryDrilldownJSON template.JS
		if len(categoryDrilldown) > 0 {
			if db, err := json.Marshal(categoryDrilldown); err == nil {
				categoryDrilldownJSON = template.JS(db)
			}
		}

		// ── Daily spending/income chart data ──
		incomeByPeriod := make(map[string]float64)
		if dailyIncomeSummary != nil {
			for _, row := range dailyIncomeSummary.Summary {
				if row.TotalAmount < 0 && row.Period != nil {
					incomeByPeriod[*row.Period] = -row.TotalAmount
				}
			}
		}

		var dailyLabelsJSON, dailyAmountsJSON, dailyIncomeAmountsJSON template.JS
		if dailySummary != nil && len(dailySummary.Summary) > 0 {
			rows := dailySummary.Summary
			labels := make([]string, 0, len(rows))
			amounts := make([]float64, 0, len(rows))
			incomeAmounts := make([]float64, 0, len(rows))
			for i := len(rows) - 1; i >= 0; i-- {
				row := rows[i]
				label := ""
				if row.Period != nil {
					label = *row.Period
				}
				labels = append(labels, label)
				amounts = append(amounts, row.TotalAmount)
				incomeAmounts = append(incomeAmounts, incomeByPeriod[label])
			}
			if lb, err := json.Marshal(labels); err == nil {
				dailyLabelsJSON = template.JS(lb)
			}
			if ab, err := json.Marshal(amounts); err == nil {
				dailyAmountsJSON = template.JS(ab)
			}
			if ib, err := json.Marshal(incomeAmounts); err == nil {
				dailyIncomeAmountsJSON = template.JS(ib)
			}
		}

		// ── Totals ──
		var totalSpending float64
		if categorySummary != nil && categorySummary.Totals.TotalAmount != nil {
			totalSpending = *categorySummary.Totals.TotalAmount
		}

		var prevTotalSpending float64
		if prevSummary != nil {
			for _, row := range prevSummary.Summary {
				prevTotalSpending += row.TotalAmount
			}
		}

		var spendingChangePercent float64
		var hasSpendingChange bool
		if prevTotalSpending > 0 {
			hasSpendingChange = true
			spendingChangePercent = ((totalSpending - prevTotalSpending) / prevTotalSpending) * 100
		}

		var totalIncome float64
		if incomeSummary != nil {
			for _, row := range incomeSummary.Summary {
				if row.TotalAmount < 0 {
					totalIncome += -row.TotalAmount
				}
			}
		}

		// ── Cash flow ──
		var cashFlowNet float64
		var savingsRate float64
		var hasCashFlow bool
		if totalIncome > 0 || totalSpending > 0 {
			hasCashFlow = true
			cashFlowNet = totalIncome - totalSpending
			if totalIncome > 0 {
				savingsRate = (cashFlowNet / totalIncome) * 100
				if savingsRate < -200 {
					savingsRate = -200
				}
				if savingsRate > 100 {
					savingsRate = 100
				}
			}
		}

		var spendingRatio float64
		if totalIncome > 0 {
			spendingRatio = (totalSpending / totalIncome) * 100
			if spendingRatio > 100 {
				spendingRatio = 100
			}
		}

		// ── Spending pace ──
		var currentMonthSpending float64
		if currentMonthSummary != nil {
			for _, row := range currentMonthSummary.Summary {
				currentMonthSpending += row.TotalAmount
			}
		}

		var currentMonthIncome float64
		if currentMonthIncomeSummary != nil {
			for _, row := range currentMonthIncomeSummary.Summary {
				if row.TotalAmount < 0 {
					currentMonthIncome += -row.TotalAmount
				}
			}
		}

		var lastMonthSpending float64
		if lastMonthSummary != nil {
			for _, row := range lastMonthSummary.Summary {
				lastMonthSpending += row.TotalAmount
			}
		}

		var lastMonthPaceSpending float64
		if lastMonthPaceSummary != nil {
			for _, row := range lastMonthPaceSummary.Summary {
				lastMonthPaceSpending += row.TotalAmount
			}
		}

		var dailyAvgSpending float64
		var projectedMonthly float64
		var pacePercent float64
		var hasPaceData bool
		var paceVsLastMonth string

		if daysElapsed > 0 && currentMonthSpending > 0 {
			hasPaceData = true
			dailyAvgSpending = currentMonthSpending / float64(daysElapsed)
			projectedMonthly = dailyAvgSpending * float64(daysInMonth)

			if lastMonthPaceSpending > 0 {
				pacePercent = ((currentMonthSpending - lastMonthPaceSpending) / lastMonthPaceSpending) * 100
				if pacePercent > 2.0 {
					paceVsLastMonth = "ahead"
				} else if pacePercent < -2.0 {
					paceVsLastMonth = "behind"
				} else {
					paceVsLastMonth = "same"
				}
			}
		}

		monthProgress := float64(daysElapsed) / float64(daysInMonth) * 100

		// ── Merchant bar percentages ──
		if len(topMerchants) > 0 && maxMerchantSpend > 0 {
			for i := range topMerchants {
				topMerchants[i].Percent = (topMerchants[i].Total / maxMerchantSpend) * 100
			}
		}

		// ── Day-of-week intensities ──
		hasDOWData := maxDaySpend > 0
		if hasDOWData {
			for i := range dowSpending {
				if dowSpending[i].Total > 0 {
					dowSpending[i].Intensity = dowSpending[i].Total / maxDaySpend
				}
			}
		}
		var highSpendDay, lowSpendDay string
		var highSpendDayAmt float64
		lowSpendDayAmt := math.MaxFloat64
		for _, d := range dowSpending {
			if d.Total > highSpendDayAmt {
				highSpendDayAmt = d.Total
				highSpendDay = d.DayFull
			}
			if d.Total > 0 && d.Total < lowSpendDayAmt {
				lowSpendDayAmt = d.Total
				lowSpendDay = d.DayFull
			}
		}
		if lowSpendDayAmt == math.MaxFloat64 {
			lowSpendDayAmt = 0
		}

		// ── Monthly comparison table ──
		var monthHeaders []string
		var monthlyCompRows []MonthlyCompRow
		var monthlyTotals []float64
		var hasMonthlyComp bool

		if compSummary != nil && len(compSummary.Summary) > 0 {
			monthSet := make(map[string]bool)
			catColorMap := make(map[string]string)
			catMonthMap := make(map[string]map[string]float64)

			for _, row := range compSummary.Summary {
				cat := "Uncategorized"
				if row.Category != nil && *row.Category != "" {
					cat = *row.Category
				}
				period := ""
				if row.Period != nil {
					period = *row.Period
				}
				if period == "" {
					continue
				}
				monthSet[period] = true
				if row.CategoryColor != nil && *row.CategoryColor != "" {
					catColorMap[cat] = *row.CategoryColor
				}
				if catMonthMap[cat] == nil {
					catMonthMap[cat] = make(map[string]float64)
				}
				catMonthMap[cat][period] += row.TotalAmount
			}

			sortedMonths := make([]string, 0, len(monthSet))
			for m := range monthSet {
				sortedMonths = append(sortedMonths, m)
			}
			sort.Strings(sortedMonths)
			if len(sortedMonths) > 4 {
				sortedMonths = sortedMonths[len(sortedMonths)-4:]
			}

			for _, m := range sortedMonths {
				t, parseErr := time.Parse("2006-01", m)
				if parseErr == nil {
					monthHeaders = append(monthHeaders, t.Format("Jan"))
				} else {
					monthHeaders = append(monthHeaders, m)
				}
			}

			type catEntry struct {
				Name  string
				Color string
				Total float64
			}
			var catEntries []catEntry
			for cat, months := range catMonthMap {
				var total float64
				for _, amt := range months {
					total += amt
				}
				color := catColorMap[cat]
				if color == "" {
					color = categoryPalette[len(catEntries)%len(categoryPalette)]
				}
				catEntries = append(catEntries, catEntry{Name: cat, Color: color, Total: total})
			}
			sort.Slice(catEntries, func(i, j int) bool {
				return catEntries[i].Total > catEntries[j].Total
			})
			if len(catEntries) > 10 {
				catEntries = catEntries[:10]
			}

			monthlyTotals = make([]float64, len(sortedMonths))
			for _, ce := range catEntries {
				row := MonthlyCompRow{
					Category:      ce.Name,
					CategoryColor: ce.Color,
					Amounts:       make([]float64, len(sortedMonths)),
					Total:         ce.Total,
				}
				for j, m := range sortedMonths {
					amt := catMonthMap[ce.Name][m]
					row.Amounts[j] = amt
					monthlyTotals[j] += amt
				}
				if len(sortedMonths) >= 2 {
					prev := row.Amounts[len(sortedMonths)-2]
					curr := row.Amounts[len(sortedMonths)-1]
					if prev > 10 {
						row.HasChange = true
						row.Change = ((curr - prev) / prev) * 100
					}
				}
				monthlyCompRows = append(monthlyCompRows, row)
			}
			hasMonthlyComp = len(monthlyCompRows) > 0
		}

		var maxMonthlyTotal float64
		for _, t := range monthlyTotals {
			if t > maxMonthlyTotal {
				maxMonthlyTotal = t
			}
		}

		// ── CSV Export data (JSON blob for client-side CSV generation) ──
		type csvExportData struct {
			PeriodLabel string `json:"periodLabel"`
			// Summary
			TotalSpending float64 `json:"totalSpending"`
			TotalIncome   float64 `json:"totalIncome"`
			CashFlowNet   float64 `json:"cashFlowNet"`
			SavingsRate   float64 `json:"savingsRate"`
			// Categories
			Categories []struct {
				Name    string  `json:"name"`
				Amount  float64 `json:"amount"`
				Percent float64 `json:"percent"`
			} `json:"categories"`
			// Merchants
			Merchants []struct {
				Name     string  `json:"name"`
				Category string  `json:"category"`
				Total    float64 `json:"total"`
				Count    int     `json:"count"`
				Average  float64 `json:"average"`
			} `json:"merchants"`
			// Day of week
			DayOfWeek []struct {
				Day   string  `json:"day"`
				Total float64 `json:"total"`
				Count int     `json:"count"`
			} `json:"dayOfWeek"`
			// Recurring
			Recurring []struct {
				Name      string  `json:"name"`
				Amount    float64 `json:"amount"`
				Frequency string  `json:"frequency"`
				Category  string  `json:"category"`
				Monthly   float64 `json:"monthlyEstimate"`
			} `json:"recurring"`
			// Monthly comparison
			MonthHeaders []string `json:"monthHeaders"`
			MonthlyComp  []struct {
				Category string    `json:"category"`
				Amounts  []float64 `json:"amounts"`
			} `json:"monthlyComparison"`
			MonthlyTotals []float64 `json:"monthlyTotals"`
		}

		periodLabel := fmt.Sprintf("%dd", chartDays)
		if chartDays == 365 {
			periodLabel = "12m"
		}

		exportData := csvExportData{
			PeriodLabel:   periodLabel,
			TotalSpending: totalSpending,
			TotalIncome:   totalIncome,
			CashFlowNet:   cashFlowNet,
			SavingsRate:   savingsRate,
			MonthHeaders:  monthHeaders,
			MonthlyTotals: monthlyTotals,
		}

		for _, tc := range topCategories {
			exportData.Categories = append(exportData.Categories, struct {
				Name    string  `json:"name"`
				Amount  float64 `json:"amount"`
				Percent float64 `json:"percent"`
			}{tc.Name, tc.Amount, tc.Percent})
		}
		for _, m := range topMerchants {
			exportData.Merchants = append(exportData.Merchants, struct {
				Name     string  `json:"name"`
				Category string  `json:"category"`
				Total    float64 `json:"total"`
				Count    int     `json:"count"`
				Average  float64 `json:"average"`
			}{m.Name, m.Category, m.Total, m.Count, m.AvgAmount})
		}
		for _, d := range dowSpending {
			exportData.DayOfWeek = append(exportData.DayOfWeek, struct {
				Day   string  `json:"day"`
				Total float64 `json:"total"`
				Count int     `json:"count"`
			}{d.DayFull, d.Total, d.Count})
		}
		for _, rc := range recurringCharges {
			exportData.Recurring = append(exportData.Recurring, struct {
				Name      string  `json:"name"`
				Amount    float64 `json:"amount"`
				Frequency string  `json:"frequency"`
				Category  string  `json:"category"`
				Monthly   float64 `json:"monthlyEstimate"`
			}{rc.Name, rc.Amount, rc.Frequency, rc.Category, rc.MonthlyEst})
		}
		for _, row := range monthlyCompRows {
			exportData.MonthlyComp = append(exportData.MonthlyComp, struct {
				Category string    `json:"category"`
				Amounts  []float64 `json:"amounts"`
			}{row.Category, row.Amounts})
		}

		var exportJSON template.JS
		if eb, err := json.Marshal(exportData); err == nil {
			exportJSON = template.JS(eb)
		}

		// ── Sparkline data for summary pills ──
		sparklineNet := make([]float64, len(sparklineSpending))
		sparklineSavingsRate := make([]float64, len(sparklineSpending))
		for i := range sparklineSpending {
			if i < len(sparklineIncome) {
				sparklineNet[i] = sparklineIncome[i] - sparklineSpending[i]
				if sparklineIncome[i] > 0 {
					sparklineSavingsRate[i] = ((sparklineIncome[i] - sparklineSpending[i]) / sparklineIncome[i]) * 100
				}
			}
		}

		var sparkSpendJSON, sparkIncomeJSON, sparkNetJSON, sparkSavingsJSON template.JS
		if sb, err := json.Marshal(sparklineSpending); err == nil {
			sparkSpendJSON = template.JS(sb)
		}
		if sb, err := json.Marshal(sparklineIncome); err == nil {
			sparkIncomeJSON = template.JS(sb)
		}
		if sb, err := json.Marshal(sparklineNet); err == nil {
			sparkNetJSON = template.JS(sb)
		}
		if sb, err := json.Marshal(sparklineSavingsRate); err == nil {
			sparkSavingsJSON = template.JS(sb)
		}

		data := map[string]any{
			"PageTitle":             "Insights",
			"CurrentPage":          "insights",
			"CSRFToken":            GetCSRFToken(r),
			"ChartDays":            chartDays,
			// Spending chart data.
			"CategoryLabels":        categoryLabelsJSON,
			"CategoryAmounts":       categoryAmountsJSON,
			"CategoryColors":        categoryColorsJSON,
			"DailyLabels":           dailyLabelsJSON,
			"DailyAmounts":          dailyAmountsJSON,
			"DailyIncomeAmounts":    dailyIncomeAmountsJSON,
			"TopCategories":         topCategories,
			"CategoryDrilldownJSON": categoryDrilldownJSON,
			// Totals.
			"TotalSpending":         totalSpending,
			"TotalIncome":           totalIncome,
			"SpendingChangePercent": spendingChangePercent,
			"HasSpendingChange":     hasSpendingChange,
			// Cash flow.
			"CashFlowNet":           cashFlowNet,
			"SavingsRate":           savingsRate,
			"HasCashFlow":           hasCashFlow,
			"SpendingRatio":         spendingRatio,
			// Spending pace.
			"HasPaceData":           hasPaceData,
			"CurrentMonthSpending":  currentMonthSpending,
			"CurrentMonthIncome":    currentMonthIncome,
			"LastMonthSpending":     lastMonthSpending,
			"DailyAvgSpending":      dailyAvgSpending,
			"ProjectedMonthly":      projectedMonthly,
			"PacePercent":           pacePercent,
			"PaceVsLastMonth":       paceVsLastMonth,
			"DaysElapsed":           daysElapsed,
			"DaysInMonth":           daysInMonth,
			"DaysRemaining":         daysRemaining,
			"MonthProgress":         monthProgress,
			"CurrentMonthName":      today.Format("January"),
			"LastMonthName":         lastMonthStart.Format("January"),
			// Merchant analysis.
			"TopMerchants":          topMerchants,
			// Day-of-week spending.
			"DOWSpending":           dowSpending,
			"HasDOWData":            hasDOWData,
			"HighSpendDay":          highSpendDay,
			"LowSpendDay":           lowSpendDay,
			"HighSpendDayAmt":       highSpendDayAmt,
			"LowSpendDayAmt":        lowSpendDayAmt,
			// Recurring charges.
			"RecurringCharges":      recurringCharges,
			"HasRecurringData":      hasRecurringData,
			"TotalRecurringMonthly": totalRecurringMonthly,
			// Monthly category comparison.
			"HasMonthlyComp":        hasMonthlyComp,
			"MonthHeaders":          monthHeaders,
			"MonthlyCompRows":       monthlyCompRows,
			"MonthlyTotals":         monthlyTotals,
			"MaxMonthlyTotal":       maxMonthlyTotal,
			// CSV Export.
			"ExportJSON":            exportJSON,
			// Sparkline data for summary pills.
			"SparkSpending":         sparkSpendJSON,
			"SparkIncome":           sparkIncomeJSON,
			"SparkNet":              sparkNetJSON,
			"SparkSavings":          sparkSavingsJSON,
			// Net worth.
			"NetWorth":              netWorth,
			"TotalAssets":           totalAssets,
			"TotalLiabilities":      totalLiabilities,
			"NetWorthLabels":        netWorthLabelsJSON,
			"NetWorthValues":        netWorthValuesJSON,
		}
		tr.Render(w, r, "insights.html", data)
	}
}
