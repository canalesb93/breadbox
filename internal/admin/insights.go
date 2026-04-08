package admin

import (
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"sort"
	"sync"
	"time"

	"breadbox/internal/app"
	"breadbox/internal/service"
)

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

		// Parse trends month range (default 6 months).
		trendMonths := 6
		if m := r.URL.Query().Get("months"); m != "" {
			switch m {
			case "3":
				trendMonths = 3
			case "6":
				trendMonths = 6
			case "12":
				trendMonths = 12
			}
		}
		trendStart := time.Date(today.Year(), today.Month()-time.Month(trendMonths), 1, 0, 0, 0, 0, today.Location())
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
		type AccountSpend struct {
			ID              string
			ShortID         string
			Name            string
			InstitutionName string
			Type            string
			Total           float64
			Percent         float64
		}
		type UserSpend struct {
			ID      string
			Name    string
			Total   float64
			Percent float64
		}
		type DayOfWeekSpend struct {
			DayShort  string
			DayFull   string
			Total     float64
			Count     int
			Intensity float64
		}
		type MonthlyIncomeSpend struct {
			Month       string
			Spending    float64
			Income      float64
			Net         float64
			SavingsRate float64
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
		var trendSummary *service.TransactionSummaryResult
		categoryDrilldown := make(map[string][]CategoryMerchant)
		var accountSpending []AccountSpend
		var userSpending []UserSpend
		dowSpending := make([]DayOfWeekSpend, 7)
		var maxDaySpend float64
		var sparklineSpending []float64
		var sparklineIncome []float64
		var netWorth, totalAssets, totalLiabilities float64
		var netWorthLabelsJSON, netWorthValuesJSON template.JS
		var allAccounts []service.AccountResponse
		// Pace comparison: daily cumulative spending for current + 2 prior months
		var paceCompJSON template.JS

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
			allAccounts = accounts
			// Net worth trend: work backwards from current balance using daily transaction totals.
			// Compute 365 days so client-side date picker can slice (30d, 90d, 6m, 1y).
			netWorthTrendStart := time.Now().AddDate(0, 0, -365)
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
			nwDays := 365
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

		// 2. Category drilldown
		wg.Add(1)
		go func() {
			defer wg.Done()
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
					GROUP BY COALESCE(cat.display_name, t.category_primary, 'Uncategorized'), COALESCE(NULLIF(t.merchant_name, ''), t.name)
				)
				SELECT cat, merchant, total, tx_count
				FROM ranked
				WHERE rn <= 8
				ORDER BY cat, total DESC
			`, chartStart)
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

		// 10b. Pace comparison: daily spending for current month + 2 prior months
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Get 3 months of daily spending data
			threeMonthsAgo := time.Date(today.Year(), today.Month()-2, 1, 0, 0, 0, 0, today.Location())
			result, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
				GroupBy:      "day",
				StartDate:    &threeMonthsAgo,
				SpendingOnly: true,
			})
			if err != nil {
				a.Logger.Error("pace comparison query", "error", err)
				return
			}
			if result == nil {
				return
			}
			// Group daily totals by month, then build cumulative arrays
			type monthData struct {
				Name string        `json:"name"`
				Days []float64     `json:"days"`
			}
			monthBuckets := make(map[string]map[int]float64) // "2026-01" -> {1: 100.0, 2: 50.0, ...}
			for _, row := range result.Summary {
				if row.Period == nil {
					continue
				}
				d, err := time.Parse("2006-01-02", *row.Period)
				if err != nil {
					continue
				}
				key := d.Format("2006-01")
				if monthBuckets[key] == nil {
					monthBuckets[key] = make(map[int]float64)
				}
				monthBuckets[key][d.Day()] += row.TotalAmount
			}
			// Build sorted month list (most recent last)
			months := make([]string, 0, len(monthBuckets))
			for m := range monthBuckets {
				months = append(months, m)
			}
			sort.Strings(months)
			// Build cumulative spending per day-of-month for each month
			var paceMonths []monthData
			for _, m := range months {
				t, err := time.Parse("2006-01", m)
				if err != nil {
					continue
				}
				daysInM := time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, t.Location()).Day()
				// For current month, only include up to today
				isCurrentMonth := t.Year() == today.Year() && t.Month() == today.Month()
				maxDay := daysInM
				if isCurrentMonth {
					maxDay = daysElapsed
				}
				cumulative := make([]float64, maxDay)
				var running float64
				for day := 1; day <= maxDay; day++ {
					running += monthBuckets[m][day]
					cumulative[day-1] = math.Round(running*100) / 100
				}
				paceMonths = append(paceMonths, monthData{
					Name: t.Format("Jan"),
					Days: cumulative,
				})
			}
			if pb, err := json.Marshal(paceMonths); err == nil {
				paceCompJSON = template.JS(pb)
			}
		}()

		// 11. Spending by account
		wg.Add(1)
		go func() {
			defer wg.Done()
			rows, err := a.DB.Query(ctx, `
				SELECT a.id, a.short_id, COALESCE(a.display_name, a.name) AS name, a.institution_name, a.type,
				       SUM(t.amount) AS total
				FROM transactions t
				JOIN accounts a ON t.account_id = a.id
				WHERE t.deleted_at IS NULL AND t.date >= $1 AND t.amount > 0 AND t.pending = false
				GROUP BY a.id, a.short_id, a.name, a.display_name, a.institution_name, a.type
				ORDER BY SUM(t.amount) DESC
			`, chartStart)
			if err != nil {
				a.Logger.Error("account spending query", "error", err)
				return
			}
			defer rows.Close()
			var grandTotal float64
			for rows.Next() {
				var as AccountSpend
				if err := rows.Scan(&as.ID, &as.ShortID, &as.Name, &as.InstitutionName, &as.Type, &as.Total); err != nil {
					a.Logger.Error("account spending scan", "error", err)
					continue
				}
				grandTotal += as.Total
				accountSpending = append(accountSpending, as)
			}
			if grandTotal > 0 {
				for i := range accountSpending {
					accountSpending[i].Percent = (accountSpending[i].Total / grandTotal) * 100
				}
			}
		}()

		// 11b. Spending by user
		wg.Add(1)
		go func() {
			defer wg.Done()
			rows, err := a.DB.Query(ctx, `
				SELECT COALESCE(t.attributed_user_id, bc.user_id)::text AS uid,
				       u.name AS user_name,
				       SUM(t.amount) AS total
				FROM transactions t
				JOIN accounts a ON t.account_id = a.id
				JOIN bank_connections bc ON a.connection_id = bc.id
				JOIN users u ON COALESCE(t.attributed_user_id, bc.user_id) = u.id
				WHERE t.deleted_at IS NULL AND t.date >= $1 AND t.amount > 0 AND t.pending = false
				GROUP BY uid, u.name
				ORDER BY SUM(t.amount) DESC
			`, chartStart)
			if err != nil {
				a.Logger.Error("user spending query", "error", err)
				return
			}
			defer rows.Close()
			var grandTotal float64
			for rows.Next() {
				var us UserSpend
				if err := rows.Scan(&us.ID, &us.Name, &us.Total); err != nil {
					a.Logger.Error("user spending scan", "error", err)
					continue
				}
				grandTotal += us.Total
				userSpending = append(userSpending, us)
			}
			if grandTotal > 0 {
				for i := range userSpending {
					userSpending[i].Percent = (userSpending[i].Total / grandTotal) * 100
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

		// 13. Monthly income vs spending (trends tab)
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
				GroupBy:   "month",
				StartDate: &trendStart,
			})
			if err != nil {
				a.Logger.Error("monthly income vs spending", "error", err)
				return
			}
			trendSummary = result
		}()

		// 14. Monthly comparison (uses trendStart to match trends tab)
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
				GroupBy:      "category_month",
				StartDate:    &trendStart,
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
			if len(sortedMonths) > trendMonths {
				sortedMonths = sortedMonths[len(sortedMonths)-trendMonths:]
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

		// ── Process monthly income vs spending (trends tab) ──
		var monthlyIncomeSpend []MonthlyIncomeSpend
		if trendSummary != nil && len(trendSummary.Summary) > 0 {
			// Build a map of period -> {spending, income}
			type monthData struct {
				spending float64
				income   float64
			}
			monthMap := make(map[string]*monthData)
			for _, row := range trendSummary.Summary {
				if row.Period == nil {
					continue
				}
				period := *row.Period
				if monthMap[period] == nil {
					monthMap[period] = &monthData{}
				}
				if row.TotalAmount > 0 {
					monthMap[period].spending += row.TotalAmount
				} else {
					monthMap[period].income += -row.TotalAmount
				}
			}
			// Sort periods chronologically
			periods := make([]string, 0, len(monthMap))
			for p := range monthMap {
				periods = append(periods, p)
			}
			sort.Strings(periods)
			for _, p := range periods {
				md := monthMap[p]
				t, parseErr := time.Parse("2006-01", p)
				label := p
				if parseErr == nil {
					label = t.Format("Jan")
				}
				net := md.income - md.spending
				var sr float64
				if md.income > 0 {
					sr = (net / md.income) * 100
				}
				monthlyIncomeSpend = append(monthlyIncomeSpend, MonthlyIncomeSpend{
					Month:       label,
					Spending:    md.spending,
					Income:      md.income,
					Net:         net,
					SavingsRate: sr,
				})
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
			// Accounts
			Accounts []struct {
				Name            string  `json:"name"`
				InstitutionName string  `json:"institutionName"`
				Type            string  `json:"type"`
				Total           float64 `json:"total"`
				Percent         float64 `json:"percent"`
			} `json:"accounts"`
			// Day of week
			DayOfWeek []struct {
				Day   string  `json:"day"`
				Total float64 `json:"total"`
				Count int     `json:"count"`
			} `json:"dayOfWeek"`
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
		for _, as := range accountSpending {
			exportData.Accounts = append(exportData.Accounts, struct {
				Name            string  `json:"name"`
				InstitutionName string  `json:"institutionName"`
				Type            string  `json:"type"`
				Total           float64 `json:"total"`
				Percent         float64 `json:"percent"`
			}{as.Name, as.InstitutionName, as.Type, as.Total, as.Percent})
		}
		for _, d := range dowSpending {
			exportData.DayOfWeek = append(exportData.DayOfWeek, struct {
				Day   string  `json:"day"`
				Total float64 `json:"total"`
				Count int     `json:"count"`
			}{d.DayFull, d.Total, d.Count})
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
			// Account balances (overview tab).
			"AllAccounts":           allAccounts,
			"HasAccounts":           len(allAccounts) > 0,
			// Pace comparison chart (overview tab).
			"PaceCompJSON":          paceCompJSON,
			// Account spending breakdown.
			"AccountSpending":       accountSpending,
			"HasAccountSpending":    len(accountSpending) > 0,
			// User spending breakdown.
			"UserSpending":          userSpending,
			"HasUserSpending":       len(userSpending) > 1,
			"UserCount":             len(userSpending),
			// Day-of-week spending.
			"DOWSpending":           dowSpending,
			"HasDOWData":            hasDOWData,
			"HighSpendDay":          highSpendDay,
			"LowSpendDay":           lowSpendDay,
			"HighSpendDayAmt":       highSpendDayAmt,
			"LowSpendDayAmt":        lowSpendDayAmt,
			// Monthly category comparison.
			"HasMonthlyComp":        hasMonthlyComp,
			"MonthHeaders":          monthHeaders,
			"MonthlyCompRows":       monthlyCompRows,
			"MonthlyTotals":         monthlyTotals,
			"MaxMonthlyTotal":       maxMonthlyTotal,
			// Trends: monthly income vs spending.
			"MonthlyIncomeSpend":    monthlyIncomeSpend,
			"HasMonthlyIncomeSpend": len(monthlyIncomeSpend) > 0,
			// Tab-aware filters.
			"TrendMonths":           trendMonths,
			// Category slugs for exclude toggle (client-side filtering).
			"NoiseCategorySlugs":    []string{"transfer", "payment"},
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
