package admin

import (
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"sort"
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

		// ── Spending by category for the selected date range ──
		categorySummary, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
			GroupBy:      "category",
			StartDate:    &chartStart,
			SpendingOnly: true,
		})
		if err != nil {
			a.Logger.Error("category summary", "error", err)
		}

		var categoryLabelsJSON, categoryAmountsJSON, categoryColorsJSON template.JS

		type CategorySpend struct {
			Name    string
			Color   string
			Amount  float64
			Percent float64
		}

		categoryPalette := []string{
			"oklch(0.55 0.12 250)", // blue
			"oklch(0.55 0.14 160)", // teal
			"oklch(0.58 0.12 35)",  // warm amber
			"oklch(0.52 0.14 300)", // purple
			"oklch(0.58 0.10 80)",  // olive
			"oklch(0.50 0.12 200)", // slate blue
			"oklch(0.55 0.12 120)", // green
			"oklch(0.48 0.10 350)", // rose
			"oklch(0.45 0 0)",      // gray (for "Other")
		}

		var topCategories []CategorySpend
		var maxCategorySpend float64
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
				if row.TotalAmount > maxCategorySpend {
					maxCategorySpend = row.TotalAmount
				}
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

		// ── Category Drill-Down Data ──
		// Query top merchants per category for interactive donut drill-down.
		type CategoryMerchant struct {
			Name   string  `json:"name"`
			Amount float64 `json:"amount"`
			Count  int     `json:"count"`
		}
		categoryDrilldown := make(map[string][]CategoryMerchant)
		var categoryDrilldownJSON template.JS

		drilldownRows, drilldownErr := a.DB.Query(ctx, `
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
		if drilldownErr != nil {
			a.Logger.Error("category drilldown query", "error", drilldownErr)
		} else {
			for drilldownRows.Next() {
				var cat, merchant string
				var total float64
				var count int
				if err := drilldownRows.Scan(&cat, &merchant, &total, &count); err != nil {
					continue
				}
				categoryDrilldown[cat] = append(categoryDrilldown[cat], CategoryMerchant{
					Name:   merchant,
					Amount: total,
					Count:  count,
				})
			}
			drilldownRows.Close()
		}
		if len(categoryDrilldown) > 0 {
			if db, err := json.Marshal(categoryDrilldown); err == nil {
				categoryDrilldownJSON = template.JS(db)
			}
		}

		// ── Daily spending trend ──
		chartGroupBy := "day"
		if chartDays == 365 {
			chartGroupBy = "month"
		}
		dailySummary, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
			GroupBy:      chartGroupBy,
			StartDate:    &chartStart,
			SpendingOnly: true,
		})
		if err != nil {
			a.Logger.Error("daily summary", "error", err)
		}

		// Daily income for the same period.
		dailyIncomeSummary, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
			GroupBy:   chartGroupBy,
			StartDate: &chartStart,
		})
		if err != nil {
			a.Logger.Error("daily income summary", "error", err)
		}
		incomeByPeriod := make(map[string]float64)
		if dailyIncomeSummary != nil {
			for _, row := range dailyIncomeSummary.Summary {
				if row.TotalAmount < 0 && row.Period != nil {
					incomeByPeriod[*row.Period] = -row.TotalAmount
				}
			}
		}

		// Previous period spending for comparison.
		prevStart := time.Now().AddDate(0, 0, -chartDays*2)
		prevEnd := time.Now().AddDate(0, 0, -chartDays)
		prevSummary, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
			GroupBy:      "category",
			StartDate:    &prevStart,
			EndDate:      &prevEnd,
			SpendingOnly: true,
		})
		if err != nil {
			a.Logger.Error("previous period summary", "error", err)
		}
		var prevTotalSpending float64
		if prevSummary != nil {
			for _, row := range prevSummary.Summary {
				prevTotalSpending += row.TotalAmount
			}
		}

		var spendingChangePercent float64
		var hasSpendingChange bool

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

		// Total spending for the selected period.
		var totalSpending float64
		if categorySummary != nil && categorySummary.Totals.TotalAmount != nil {
			totalSpending = *categorySummary.Totals.TotalAmount
		}

		// Compute spending change vs previous period.
		if prevTotalSpending > 0 {
			hasSpendingChange = true
			spendingChangePercent = ((totalSpending - prevTotalSpending) / prevTotalSpending) * 100
		}

		// Total income for the selected date range.
		var totalIncome float64
		incomeSummary, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
			GroupBy:   "day",
			StartDate: &chartStart,
		})
		if err != nil {
			a.Logger.Error("income summary", "error", err)
		}
		if incomeSummary != nil {
			for _, row := range incomeSummary.Summary {
				if row.TotalAmount < 0 {
					totalIncome += -row.TotalAmount
				}
			}
		}

		// Cash flow: net = income - spending, savings rate = net/income * 100.
		var cashFlowNet float64
		var savingsRate float64
		var hasCashFlow bool
		if totalIncome > 0 || totalSpending > 0 {
			hasCashFlow = true
			cashFlowNet = totalIncome - totalSpending
			if totalIncome > 0 {
				savingsRate = (cashFlowNet / totalIncome) * 100
				// Clamp for display — extreme values are not meaningful.
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

		// ── Spending Pace ──
		today := time.Now()
		monthStart := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, today.Location())
		daysElapsed := today.Day()
		daysInMonth := time.Date(today.Year(), today.Month()+1, 0, 0, 0, 0, 0, today.Location()).Day()
		daysRemaining := daysInMonth - daysElapsed

		var currentMonthSpending float64
		currentMonthSummary, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
			GroupBy:      "day",
			StartDate:    &monthStart,
			SpendingOnly: true,
		})
		if err != nil {
			a.Logger.Error("current month spending", "error", err)
		}
		if currentMonthSummary != nil {
			for _, row := range currentMonthSummary.Summary {
				currentMonthSpending += row.TotalAmount
			}
		}

		var currentMonthIncome float64
		currentMonthIncomeSummary, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
			GroupBy:   "day",
			StartDate: &monthStart,
		})
		if err != nil {
			a.Logger.Error("current month income", "error", err)
		}
		if currentMonthIncomeSummary != nil {
			for _, row := range currentMonthIncomeSummary.Summary {
				if row.TotalAmount < 0 {
					currentMonthIncome += -row.TotalAmount
				}
			}
		}

		lastMonthStart := time.Date(today.Year(), today.Month()-1, 1, 0, 0, 0, 0, today.Location())
		lastMonthEnd := monthStart
		var lastMonthSpending float64
		lastMonthSummary, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
			GroupBy:      "day",
			StartDate:    &lastMonthStart,
			EndDate:      &lastMonthEnd,
			SpendingOnly: true,
		})
		if err != nil {
			a.Logger.Error("last month spending", "error", err)
		}
		if lastMonthSummary != nil {
			for _, row := range lastMonthSummary.Summary {
				lastMonthSpending += row.TotalAmount
			}
		}

		// Last month spending at the same point.
		lastMonthSameDay := time.Date(today.Year(), today.Month()-1, 1, 0, 0, 0, 0, today.Location())
		lastMonthDaysInMonth := time.Date(lastMonthSameDay.Year(), lastMonthSameDay.Month()+1, 0, 0, 0, 0, 0, today.Location()).Day()
		sameDayOfLastMonth := daysElapsed
		if sameDayOfLastMonth > lastMonthDaysInMonth {
			sameDayOfLastMonth = lastMonthDaysInMonth
		}
		lastMonthSameDayEnd := time.Date(lastMonthSameDay.Year(), lastMonthSameDay.Month(), sameDayOfLastMonth+1, 0, 0, 0, 0, today.Location())
		var lastMonthPaceSpending float64
		lastMonthPaceSummary, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
			GroupBy:      "day",
			StartDate:    &lastMonthSameDay,
			EndDate:      &lastMonthSameDayEnd,
			SpendingOnly: true,
		})
		if err != nil {
			a.Logger.Error("last month pace spending", "error", err)
		}
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

		// ── Smart Spending Insights ──
		type SpendingInsight struct {
			Icon      string
			Title     string
			Detail    string
			Amount    float64
			Change    float64
			Sentiment string
			Category  string
		}
		var spendingInsights []SpendingInsight

		currentCatMap := make(map[string]float64)
		if categorySummary != nil {
			for _, row := range categorySummary.Summary {
				label := "Uncategorized"
				if row.Category != nil && *row.Category != "" {
					label = *row.Category
				}
				currentCatMap[label] = row.TotalAmount
			}
		}
		prevCatMap := make(map[string]float64)
		if prevSummary != nil {
			for _, row := range prevSummary.Summary {
				label := "Uncategorized"
				if row.Category != nil && *row.Category != "" {
					label = *row.Category
				}
				prevCatMap[label] = row.TotalAmount
			}
		}

		var biggestIncreaseCat string
		var biggestIncreaseAmt, biggestIncreasePct float64
		var biggestDecreaseCat string
		var biggestDecreaseAmt, biggestDecreasePct float64

		for cat, curAmt := range currentCatMap {
			prevAmt, existed := prevCatMap[cat]
			if !existed || prevAmt < 10 {
				continue
			}
			changePct := ((curAmt - prevAmt) / prevAmt) * 100
			if changePct > biggestIncreasePct && changePct > 15 {
				biggestIncreaseCat = cat
				biggestIncreaseAmt = curAmt
				biggestIncreasePct = changePct
			}
			if changePct < biggestDecreasePct && changePct < -15 {
				biggestDecreaseCat = cat
				biggestDecreaseAmt = curAmt
				biggestDecreasePct = changePct
			}
		}

		if biggestIncreaseCat != "" {
			spendingInsights = append(spendingInsights, SpendingInsight{
				Icon:      "trending-up",
				Title:     fmt.Sprintf("%s up %.0f%%", biggestIncreaseCat, biggestIncreasePct),
				Detail:    fmt.Sprintf("$%.0f spent vs $%.0f last period", biggestIncreaseAmt, prevCatMap[biggestIncreaseCat]),
				Amount:    biggestIncreaseAmt,
				Change:    biggestIncreasePct,
				Sentiment: "negative",
				Category:  biggestIncreaseCat,
			})
		}

		if biggestDecreaseCat != "" {
			spendingInsights = append(spendingInsights, SpendingInsight{
				Icon:      "trending-down",
				Title:     fmt.Sprintf("%s down %.0f%%", biggestDecreaseCat, math.Abs(biggestDecreasePct)),
				Detail:    fmt.Sprintf("$%.0f spent vs $%.0f last period", biggestDecreaseAmt, prevCatMap[biggestDecreaseCat]),
				Amount:    biggestDecreaseAmt,
				Change:    biggestDecreasePct,
				Sentiment: "positive",
				Category:  biggestDecreaseCat,
			})
		}

		// Largest single transaction this period.
		var largestTxName string
		var largestTxAmount float64
		var largestTxDate string
		err = a.DB.QueryRow(ctx, `
			SELECT COALESCE(merchant_name, name), amount, TO_CHAR(date, 'Mon DD')
			FROM transactions
			WHERE deleted_at IS NULL AND date >= $1 AND amount > 0 AND pending = false
			ORDER BY amount DESC
			LIMIT 1
		`, chartStart).Scan(&largestTxName, &largestTxAmount, &largestTxDate)
		if err == nil && largestTxAmount >= 50 {
			spendingInsights = append(spendingInsights, SpendingInsight{
				Icon:      "receipt",
				Title:     fmt.Sprintf("Largest: $%.0f", largestTxAmount),
				Detail:    fmt.Sprintf("%s on %s", largestTxName, largestTxDate),
				Amount:    largestTxAmount,
				Sentiment: "neutral",
			})
		}

		// Recurring spending detection.
		var recurringMerchant string
		var recurringCount int64
		var recurringTotal float64
		err = a.DB.QueryRow(ctx, `
			SELECT COALESCE(merchant_name, name), COUNT(*), SUM(amount)
			FROM transactions
			WHERE deleted_at IS NULL AND date >= $1 AND amount > 0 AND pending = false
			GROUP BY COALESCE(merchant_name, name)
			HAVING COUNT(*) >= 3
			ORDER BY SUM(amount) DESC
			LIMIT 1
		`, chartStart).Scan(&recurringMerchant, &recurringCount, &recurringTotal)
		if err == nil && recurringCount >= 3 {
			spendingInsights = append(spendingInsights, SpendingInsight{
				Icon:      "repeat",
				Title:     fmt.Sprintf("%s: %dx", recurringMerchant, recurringCount),
				Detail:    fmt.Sprintf("$%.0f total across %d transactions", recurringTotal, recurringCount),
				Amount:    recurringTotal,
				Sentiment: "info",
			})
		}

		// New spending categories.
		newCatCount := 0
		for cat, amt := range currentCatMap {
			if cat == "Uncategorized" {
				continue
			}
			if _, existed := prevCatMap[cat]; !existed && amt >= 20 {
				spendingInsights = append(spendingInsights, SpendingInsight{
					Icon:      "sparkles",
					Title:     fmt.Sprintf("New: %s", cat),
					Detail:    fmt.Sprintf("$%.0f in first-time spending", amt),
					Amount:    amt,
					Sentiment: "info",
					Category:  cat,
				})
				newCatCount++
				if newCatCount >= 1 {
					break
				}
			}
		}

		if len(spendingInsights) > 5 {
			spendingInsights = spendingInsights[:5]
		}

		// ── Top Merchants Analysis ──
		type MerchantSpend struct {
			Name      string
			Category  string
			Total     float64
			Count     int
			AvgAmount float64
			Percent   float64
		}
		var topMerchants []MerchantSpend
		var maxMerchantSpend float64

		merchantRows, err := a.DB.Query(ctx, `
			SELECT
				COALESCE(NULLIF(merchant_name, ''), name) AS merchant,
				COUNT(*)::int AS tx_count,
				SUM(amount) AS total,
				AVG(amount) AS avg_amount,
				MODE() WITHIN GROUP (ORDER BY COALESCE(category_primary, '')) AS top_category
			FROM transactions
			WHERE deleted_at IS NULL AND date >= $1 AND amount > 0 AND pending = false
			GROUP BY COALESCE(NULLIF(merchant_name, ''), name)
			ORDER BY SUM(amount) DESC
			LIMIT 10
		`, chartStart)
		if err != nil {
			a.Logger.Error("top merchants query", "error", err)
		} else {
			defer merchantRows.Close()
			for merchantRows.Next() {
				var m MerchantSpend
				var cat *string
				if err := merchantRows.Scan(&m.Name, &m.Count, &m.Total, &m.AvgAmount, &cat); err != nil {
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
			merchantRows.Close()
		}

		// Compute merchant bar percentages relative to max.
		if len(topMerchants) > 0 && maxMerchantSpend > 0 {
			for i := range topMerchants {
				topMerchants[i].Percent = (topMerchants[i].Total / maxMerchantSpend) * 100
			}
		}

		// ── Day-of-Week Spending Pattern ──
		type DayOfWeekSpend struct {
			DayShort  string  // "Mon", "Tue", etc.
			DayFull   string  // "Monday", "Tuesday", etc.
			Total     float64
			Count     int
			Intensity float64 // 0-1 for heatmap coloring
		}
		dowSpending := make([]DayOfWeekSpend, 7)
		dayNames := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
		dayFullNames := []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}
		for i := range dowSpending {
			dowSpending[i].DayShort = dayNames[i]
			dowSpending[i].DayFull = dayFullNames[i]
		}

		var maxDaySpend float64
		dowRows, err := a.DB.Query(ctx, `
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
		} else {
			defer dowRows.Close()
			for dowRows.Next() {
				var dow int
				var total float64
				var count int
				if err := dowRows.Scan(&dow, &total, &count); err != nil {
					continue
				}
				// ISODOW: 1=Mon, 7=Sun
				idx := dow - 1
				if idx >= 0 && idx < 7 {
					dowSpending[idx].Total = total
					dowSpending[idx].Count = count
					if total > maxDaySpend {
						maxDaySpend = total
					}
				}
			}
			dowRows.Close()
		}
		hasDOWData := maxDaySpend > 0
		if hasDOWData {
			for i := range dowSpending {
				if dowSpending[i].Total > 0 {
					dowSpending[i].Intensity = dowSpending[i].Total / maxDaySpend
				}
			}
		}

		// Find highest and lowest spending days.
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

		// ── Recurring / Subscription Detection ──
		// Find merchants with 2+ transactions at consistent amounts over 90 days.
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
		var recurringCharges []RecurringCharge
		var totalRecurringMonthly float64
		var hasRecurringData bool

		recurringLookback := time.Now().AddDate(0, 0, -90)
		recurringRows, err := a.DB.Query(ctx, `
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
		`, recurringLookback)
		if err != nil {
			a.Logger.Error("recurring charges query", "error", err)
		} else {
			defer recurringRows.Close()
			for recurringRows.Next() {
				var rc RecurringCharge
				var cat *string
				var avgDaysBetween *float64
				var cv float64
				if err := recurringRows.Scan(&rc.Name, &rc.Amount, &cat, &rc.TxCount, &rc.TotalSpent, &rc.LastChargeDate, &avgDaysBetween, &cv); err != nil {
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
			recurringRows.Close()
			hasRecurringData = len(recurringCharges) > 0
		}

		// ── Monthly Category Comparison Table ──
		// Fetch category_month data for last 4 months to build a comparison grid.
		type MonthlyCompRow struct {
			Category      string
			CategoryColor string
			Amounts       []float64 // one per month column, aligned with MonthHeaders
			Total         float64
			Change        float64 // % change newest vs prior month
			HasChange     bool
		}
		var monthHeaders []string // e.g. ["Dec", "Jan", "Feb", "Mar"]
		var monthlyCompRows []MonthlyCompRow
		var monthlyTotals []float64
		var hasMonthlyComp bool

		compStart := time.Date(today.Year(), today.Month()-3, 1, 0, 0, 0, 0, today.Location())
		compEnd := time.Now().AddDate(0, 0, 1)
		compSummary, compErr := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
			GroupBy:      "category_month",
			StartDate:    &compStart,
			EndDate:      &compEnd,
			SpendingOnly: true,
		})
		if compErr != nil {
			a.Logger.Error("monthly comparison summary", "error", compErr)
		}
		if compSummary != nil && len(compSummary.Summary) > 0 {
			// Collect unique months (sorted) and categories.
			monthSet := make(map[string]bool)
			catColorMap := make(map[string]string)
			// Map: category -> month -> amount
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

			// Sort months chronologically.
			sortedMonths := make([]string, 0, len(monthSet))
			for m := range monthSet {
				sortedMonths = append(sortedMonths, m)
			}
			sort.Strings(sortedMonths)
			// Limit to last 4 months.
			if len(sortedMonths) > 4 {
				sortedMonths = sortedMonths[len(sortedMonths)-4:]
			}

			// Build month headers as short names (e.g. "Jan", "Feb").
			for _, m := range sortedMonths {
				t, parseErr := time.Parse("2006-01", m)
				if parseErr == nil {
					monthHeaders = append(monthHeaders, t.Format("Jan"))
				} else {
					monthHeaders = append(monthHeaders, m)
				}
			}

			// Build rows: collect total per category across all months, sort by total desc.
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

			// Limit to top 10 categories.
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
				// Compute % change between the two most recent months.
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

		// ── Spending Velocity: cumulative daily spending for last 3 months ──
		type VelocityMonth struct {
			Label      string    // "March 2026"
			ShortLabel string    // "Mar"
			IsCurrent  bool
			Days       []float64 // cumulative amount per day (index 0 = day 1)
			FinalTotal float64
		}
		var velocityMonths []VelocityMonth
		var hasVelocityData bool
		var velocityMaxDays int

		// Build 3 months: current, previous, two months ago.
		for mi := 2; mi >= 0; mi-- {
			mStart := time.Date(today.Year(), today.Month()-time.Month(mi), 1, 0, 0, 0, 0, today.Location())
			mEnd := time.Date(mStart.Year(), mStart.Month()+1, 1, 0, 0, 0, 0, today.Location())
			mDaysInMonth := time.Date(mStart.Year(), mStart.Month()+1, 0, 0, 0, 0, 0, today.Location()).Day()

			// For current month, only up to today.
			maxDay := mDaysInMonth
			if mi == 0 && daysElapsed < mDaysInMonth {
				maxDay = daysElapsed
			}

			vm := VelocityMonth{
				Label:      mStart.Format("January 2006"),
				ShortLabel: mStart.Format("Jan"),
				IsCurrent:  mi == 0,
				Days:       make([]float64, mDaysInMonth),
			}

			velocityRows, vErr := a.DB.Query(ctx, `
				SELECT EXTRACT(DAY FROM date)::int AS day_num, SUM(amount)
				FROM transactions
				WHERE deleted_at IS NULL AND date >= $1 AND date < $2 AND amount > 0 AND pending = false
				GROUP BY EXTRACT(DAY FROM date)::int
				ORDER BY day_num
			`, mStart, mEnd)
			if vErr != nil {
				a.Logger.Error("velocity query", "error", vErr)
			} else {
				for velocityRows.Next() {
					var dayNum int
					var dayTotal float64
					if err := velocityRows.Scan(&dayNum, &dayTotal); err != nil {
						continue
					}
					if dayNum >= 1 && dayNum <= mDaysInMonth {
						vm.Days[dayNum-1] = dayTotal
					}
				}
				velocityRows.Close()
			}

			// Convert to cumulative.
			var cumulative float64
			for d := 0; d < mDaysInMonth; d++ {
				cumulative += vm.Days[d]
				vm.Days[d] = cumulative
				// For current month, zero out future days.
				if mi == 0 && d >= maxDay {
					vm.Days[d] = 0
				}
			}
			vm.FinalTotal = cumulative

			if mDaysInMonth > velocityMaxDays {
				velocityMaxDays = mDaysInMonth
			}
			if cumulative > 0 {
				hasVelocityData = true
			}
			velocityMonths = append(velocityMonths, vm)
		}

		// JSON-encode velocity data for Chart.js.
		type velocityDS struct {
			Label string    `json:"label"`
			Data  []float64 `json:"data"`
		}
		type velocityChartData struct {
			Labels   []int        `json:"labels"`
			Datasets []velocityDS `json:"datasets"`
		}
		var velocityJSON template.JS
		if hasVelocityData {
			vLabels := make([]int, velocityMaxDays)
			for i := range vLabels {
				vLabels[i] = i + 1
			}
			vData := velocityChartData{Labels: vLabels}
			for _, vm := range velocityMonths {
				ds := velocityDS{Label: vm.ShortLabel}
				// Pad to max days.
				padded := make([]float64, velocityMaxDays)
				for i := 0; i < len(vm.Days) && i < velocityMaxDays; i++ {
					padded[i] = vm.Days[i]
				}
				ds.Data = padded
				vData.Datasets = append(vData.Datasets, ds)
			}
			if vb, err := json.Marshal(vData); err == nil {
				velocityJSON = template.JS(vb)
			}
		}

		// ── Family Spending Breakdown ──
		type FamilyMemberSpend struct {
			UserID   string
			UserName string
			Total    float64
			Percent  float64
			TxCount  int
			TopCat   string
			Color    string
		}
		var familySpending []FamilyMemberSpend
		var hasFamilyData bool
		var maxFamilySpend float64

		familyColors := []string{
			"oklch(0.60 0.15 250)", // blue
			"oklch(0.58 0.14 160)", // teal
			"oklch(0.60 0.14 35)",  // amber
			"oklch(0.55 0.14 300)", // purple
			"oklch(0.58 0.12 80)",  // olive
		}

		familyRows, fErr := a.DB.Query(ctx, `
			SELECT
				COALESCE(bc.user_id::text, 'unknown') AS uid,
				COALESCE(u.display_name, u.username, 'Unknown') AS user_name,
				SUM(t.amount) AS total,
				COUNT(*)::int AS tx_count,
				MODE() WITHIN GROUP (ORDER BY COALESCE(t.category_primary, '')) AS top_cat
			FROM transactions t
			JOIN accounts a ON t.account_id = a.id
			LEFT JOIN bank_connections bc ON a.connection_id = bc.id
			LEFT JOIN users u ON bc.user_id = u.id
			WHERE t.deleted_at IS NULL AND t.date >= $1 AND t.amount > 0 AND t.pending = false
				AND COALESCE(a.is_dependent_linked, false) = false
			GROUP BY bc.user_id, u.display_name, u.username
			ORDER BY SUM(t.amount) DESC
		`, chartStart)
		if fErr != nil {
			a.Logger.Error("family spending query", "error", fErr)
		} else {
			for familyRows.Next() {
				var fs FamilyMemberSpend
				var topCat *string
				if err := familyRows.Scan(&fs.UserID, &fs.UserName, &fs.Total, &fs.TxCount, &topCat); err != nil {
					a.Logger.Error("family spending scan", "error", err)
					continue
				}
				if topCat != nil && *topCat != "" {
					fs.TopCat = *topCat
				}
				fs.Color = familyColors[len(familySpending)%len(familyColors)]
				familySpending = append(familySpending, fs)
				if fs.Total > maxFamilySpend {
					maxFamilySpend = fs.Total
				}
			}
			familyRows.Close()
			hasFamilyData = len(familySpending) > 1 // Only show if there are multiple users
		}
		// Compute percentages for family spending.
		if len(familySpending) > 0 {
			var familyTotal float64
			for _, fs := range familySpending {
				familyTotal += fs.Total
			}
			if familyTotal > 0 {
				for i := range familySpending {
					familySpending[i].Percent = (familySpending[i].Total / familyTotal) * 100
				}
			}
		}

		// ── Savings Rate Trend (last 6 months) ──
		type SavingsRatePoint struct {
			Month       string  // "Jan", "Feb", ...
			MonthFull   string  // "January 2026"
			Income      float64
			Spending    float64
			Net         float64
			Rate        float64 // savings rate %
		}
		var savingsRateTrend []SavingsRatePoint
		var hasSavingsRateTrend bool
		var savingsRateTrendJSON template.JS

		for mi := 5; mi >= 0; mi-- {
			mStart := time.Date(today.Year(), today.Month()-time.Month(mi), 1, 0, 0, 0, 0, today.Location())
			mEnd := time.Date(mStart.Year(), mStart.Month()+1, 1, 0, 0, 0, 0, today.Location())

			// Get all transactions for this month (both income and spending).
			var mIncome, mSpending float64
			srtRows, srtErr := a.DB.Query(ctx, `
				SELECT amount
				FROM transactions
				WHERE deleted_at IS NULL AND date >= $1 AND date < $2 AND pending = false
			`, mStart, mEnd)
			if srtErr != nil {
				a.Logger.Error("savings rate trend query", "error", srtErr)
				continue
			}
			for srtRows.Next() {
				var amt float64
				if err := srtRows.Scan(&amt); err != nil {
					continue
				}
				if amt > 0 {
					mSpending += amt
				} else {
					mIncome += -amt
				}
			}
			srtRows.Close()

			net := mIncome - mSpending
			rate := 0.0
			if mIncome > 0 {
				rate = (net / mIncome) * 100
			}
			// Clamp rate for display purposes.
			if rate < -200 {
				rate = -200
			}
			if rate > 100 {
				rate = 100
			}

			savingsRateTrend = append(savingsRateTrend, SavingsRatePoint{
				Month:     mStart.Format("Jan"),
				MonthFull: mStart.Format("January 2006"),
				Income:    mIncome,
				Spending:  mSpending,
				Net:       net,
				Rate:      rate,
			})
			if mIncome > 0 || mSpending > 0 {
				hasSavingsRateTrend = true
			}
		}

		if hasSavingsRateTrend {
			type srtJSON struct {
				Labels   []string  `json:"labels"`
				Rates    []float64 `json:"rates"`
				Income   []float64 `json:"income"`
				Spending []float64 `json:"spending"`
				Net      []float64 `json:"net"`
			}
			srtData := srtJSON{}
			for _, pt := range savingsRateTrend {
				srtData.Labels = append(srtData.Labels, pt.Month)
				srtData.Rates = append(srtData.Rates, math.Round(pt.Rate*10)/10)
				srtData.Income = append(srtData.Income, math.Round(pt.Income*100)/100)
				srtData.Spending = append(srtData.Spending, math.Round(pt.Spending*100)/100)
				srtData.Net = append(srtData.Net, math.Round(pt.Net*100)/100)
			}
			if sb, err := json.Marshal(srtData); err == nil {
				savingsRateTrendJSON = template.JS(sb)
			}
		}

		// ── Cash Flow Forecast ──
		// Build a day-by-day chart: actual income-spending for past days,
		// projected for remaining days of the month based on daily averages.
		type ForecastPoint struct {
			Day       int     `json:"day"`
			DateLabel string  `json:"dateLabel"` // "Mar 1", "Mar 2", ...
			Actual    float64 `json:"actual"`    // cumulative net (income - spending), 0 for future
			Forecast  float64 `json:"forecast"`  // projected cumulative net, 0 for past-only days
			IsActual  bool    `json:"isActual"`
		}
		var forecastPoints []ForecastPoint
		var hasForecastData bool
		var forecastJSON template.JS

		// We need daily income and spending for the current month.
		type dailyFlow struct {
			Income   float64
			Spending float64
		}
		dailyFlows := make(map[int]dailyFlow)

		flowRows, flowErr := a.DB.Query(ctx, `
			SELECT EXTRACT(DAY FROM date)::int AS day_num, amount
			FROM transactions
			WHERE deleted_at IS NULL
				AND date >= $1
				AND date < $2
				AND pending = false
		`, monthStart, time.Date(today.Year(), today.Month()+1, 1, 0, 0, 0, 0, today.Location()))
		if flowErr != nil {
			a.Logger.Error("forecast flow query", "error", flowErr)
		} else {
			for flowRows.Next() {
				var dayNum int
				var amt float64
				if err := flowRows.Scan(&dayNum, &amt); err != nil {
					continue
				}
				df := dailyFlows[dayNum]
				if amt > 0 {
					df.Spending += amt
				} else {
					df.Income += -amt
				}
				dailyFlows[dayNum] = df
			}
			flowRows.Close()
		}

		if len(dailyFlows) > 0 && daysElapsed > 0 {
			hasForecastData = true

			// Calculate average daily income and spending from actual data.
			var totalDailyIncome, totalDailySpending float64
			for d := 1; d <= daysElapsed; d++ {
				df := dailyFlows[d]
				totalDailyIncome += df.Income
				totalDailySpending += df.Spending
			}
			avgDailyIncome := totalDailyIncome / float64(daysElapsed)
			avgDailySpending := totalDailySpending / float64(daysElapsed)

			// Build the forecast points.
			var cumulativeNet float64
			for d := 1; d <= daysInMonth; d++ {
				pt := ForecastPoint{
					Day:       d,
					DateLabel: time.Date(today.Year(), today.Month(), d, 0, 0, 0, 0, today.Location()).Format("Jan 2"),
				}

				if d <= daysElapsed {
					// Actual data.
					df := dailyFlows[d]
					dayNet := df.Income - df.Spending
					cumulativeNet += dayNet
					pt.Actual = math.Round(cumulativeNet*100) / 100
					pt.Forecast = math.Round(cumulativeNet*100) / 100 // forecast matches actual for past days
					pt.IsActual = true
				} else {
					// Projected: use daily averages.
					projectedDayNet := avgDailyIncome - avgDailySpending
					cumulativeNet += projectedDayNet
					pt.Forecast = math.Round(cumulativeNet*100) / 100
					pt.IsActual = false
				}
				forecastPoints = append(forecastPoints, pt)
			}

			// JSON-encode.
			type forecastChartData struct {
				Labels        []string  `json:"labels"`
				Actual        []any     `json:"actual"`   // float64 or null
				Forecast      []float64 `json:"forecast"`
				DaysElapsed   int       `json:"daysElapsed"`
				AvgDailyNet   float64   `json:"avgDailyNet"`
				ProjectedEOM  float64   `json:"projectedEOM"`
			}
			fcd := forecastChartData{
				DaysElapsed: daysElapsed,
				AvgDailyNet: math.Round((avgDailyIncome-avgDailySpending)*100) / 100,
			}
			for _, pt := range forecastPoints {
				fcd.Labels = append(fcd.Labels, pt.DateLabel)
				if pt.IsActual {
					fcd.Actual = append(fcd.Actual, pt.Actual)
				} else {
					fcd.Actual = append(fcd.Actual, nil)
				}
				fcd.Forecast = append(fcd.Forecast, pt.Forecast)
			}
			if len(forecastPoints) > 0 {
				fcd.ProjectedEOM = forecastPoints[len(forecastPoints)-1].Forecast
			}

			if fb, err := json.Marshal(fcd); err == nil {
				forecastJSON = template.JS(fb)
			}
		}

		// ── Anomaly Detection ──
		// Find categories where current period spending is significantly above
		// the historical per-period average, and individual large transactions
		// that are outliers within their category.
		type CategoryAnomaly struct {
			Category    string
			Color       string
			Current     float64
			Historical  float64 // average per-period
			Multiplier  float64 // current / historical
			Percentile  float64 // 0-100, how extreme this is
			TopTxName   string
			TopTxAmount float64
			TopTxDate   string
		}
		var categoryAnomalies []CategoryAnomaly
		var hasCategoryAnomalies bool

		type TransactionAnomaly struct {
			Name           string
			Amount         float64
			Date           string
			Category       string
			CategoryColor  string
			CategoryAvg    float64
			Multiplier     float64
			AccountName    string
		}
		var transactionAnomalies []TransactionAnomaly
		var hasTransactionAnomalies bool

		// We need historical category averages. Use 90 days of history,
		// broken into periods matching the current chartDays window.
		historyDays := 90
		if chartDays >= 90 {
			historyDays = 365
		}
		histStart := time.Now().AddDate(0, 0, -historyDays)
		numPeriods := float64(historyDays) / float64(chartDays)
		if numPeriods < 1 {
			numPeriods = 1
		}

		// Get historical category spending totals.
		histCatSummary, histErr := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
			GroupBy:      "category",
			StartDate:    &histStart,
			SpendingOnly: true,
		})
		if histErr != nil {
			a.Logger.Error("anomaly historical summary", "error", histErr)
		}

		histCatAvg := make(map[string]float64) // category -> avg per period
		histCatColor := make(map[string]string)
		if histCatSummary != nil {
			for i, row := range histCatSummary.Summary {
				label := "Uncategorized"
				if row.Category != nil && *row.Category != "" {
					label = *row.Category
				}
				avgPerPeriod := row.TotalAmount / numPeriods
				histCatAvg[label] = avgPerPeriod
				color := ""
				if row.CategoryColor != nil && *row.CategoryColor != "" {
					color = *row.CategoryColor
				} else {
					color = categoryPalette[i%len(categoryPalette)]
				}
				histCatColor[label] = color
			}
		}

		// Compare current period vs historical averages.
		for cat, curAmt := range currentCatMap {
			avgAmt, exists := histCatAvg[cat]
			if !exists || avgAmt < 20 {
				continue
			}
			multiplier := curAmt / avgAmt
			// Flag if spending is 1.8x or more of the historical average.
			if multiplier >= 1.8 && curAmt >= 50 {
				pctl := math.Min((multiplier-1)*50, 99)
				ca := CategoryAnomaly{
					Category:   cat,
					Color:      histCatColor[cat],
					Current:    curAmt,
					Historical: avgAmt,
					Multiplier: multiplier,
					Percentile: pctl,
				}

				// Find the largest transaction in this category for the period.
				var txName *string
				var txAmt *float64
				var txDate *string
				_ = a.DB.QueryRow(ctx, `
					SELECT COALESCE(NULLIF(merchant_name, ''), name), amount, TO_CHAR(date, 'Mon DD')
					FROM transactions
					WHERE deleted_at IS NULL AND date >= $1 AND amount > 0
						AND pending = false AND COALESCE(category_primary, '') = $2
					ORDER BY amount DESC LIMIT 1
				`, chartStart, cat).Scan(&txName, &txAmt, &txDate)
				if txName != nil {
					ca.TopTxName = *txName
				}
				if txAmt != nil {
					ca.TopTxAmount = *txAmt
				}
				if txDate != nil {
					ca.TopTxDate = *txDate
				}

				categoryAnomalies = append(categoryAnomalies, ca)
			}
		}

		// Sort by multiplier descending (most extreme first).
		sort.Slice(categoryAnomalies, func(i, j int) bool {
			return categoryAnomalies[i].Multiplier > categoryAnomalies[j].Multiplier
		})
		if len(categoryAnomalies) > 6 {
			categoryAnomalies = categoryAnomalies[:6]
		}
		hasCategoryAnomalies = len(categoryAnomalies) > 0

		// ── Individual transaction anomalies ──
		// Find transactions that are significantly larger than the average for
		// their category (3x or more).
		anomalyTxRows, atxErr := a.DB.Query(ctx, `
			WITH cat_stats AS (
				SELECT
					COALESCE(category_primary, '') AS cat,
					AVG(amount) AS avg_amount,
					STDDEV(amount) AS std_amount,
					COUNT(*) AS tx_count
				FROM transactions
				WHERE deleted_at IS NULL AND date >= $1 AND amount > 0 AND pending = false
				GROUP BY COALESCE(category_primary, '')
				HAVING COUNT(*) >= 3 AND AVG(amount) > 5
			)
			SELECT
				COALESCE(NULLIF(t.merchant_name, ''), t.name) AS tx_name,
				t.amount,
				TO_CHAR(t.date, 'Mon DD') AS tx_date,
				COALESCE(t.category_primary, 'Uncategorized') AS category,
				cs.avg_amount AS cat_avg,
				t.amount / cs.avg_amount AS multiplier,
				COALESCE(a.display_name, a.name, '') AS account_name
			FROM transactions t
			JOIN cat_stats cs ON COALESCE(t.category_primary, '') = cs.cat
			LEFT JOIN accounts a ON t.account_id = a.id
			WHERE t.deleted_at IS NULL AND t.date >= $2 AND t.amount > 0 AND t.pending = false
				AND t.amount >= cs.avg_amount * 1.5
				AND t.amount >= 50
			ORDER BY t.amount / cs.avg_amount DESC
			LIMIT 8
		`, histStart, chartStart)
		if atxErr != nil {
			a.Logger.Error("transaction anomaly query", "error", atxErr)
		} else {
			defer anomalyTxRows.Close()
			for anomalyTxRows.Next() {
				var ta TransactionAnomaly
				if err := anomalyTxRows.Scan(&ta.Name, &ta.Amount, &ta.Date, &ta.Category, &ta.CategoryAvg, &ta.Multiplier, &ta.AccountName); err != nil {
					a.Logger.Error("transaction anomaly scan", "error", err)
					continue
				}
				ta.CategoryColor = histCatColor[ta.Category]
				if ta.CategoryColor == "" {
					ta.CategoryColor = categoryPalette[len(transactionAnomalies)%len(categoryPalette)]
				}
				transactionAnomalies = append(transactionAnomalies, ta)
			}
			anomalyTxRows.Close()
			hasTransactionAnomalies = len(transactionAnomalies) > 0
		}

		data := map[string]any{
			"PageTitle":              "Insights",
			"CurrentPage":            "insights",
			"CSRFToken":              GetCSRFToken(r),
			"ChartDays":              chartDays,
			// Spending chart data.
			"CategoryLabels":         categoryLabelsJSON,
			"CategoryAmounts":        categoryAmountsJSON,
			"CategoryColors":         categoryColorsJSON,
			"DailyLabels":            dailyLabelsJSON,
			"DailyAmounts":           dailyAmountsJSON,
			"DailyIncomeAmounts":     dailyIncomeAmountsJSON,
			"TopCategories":          topCategories,
			"MaxCategorySpend":       maxCategorySpend,
			"CategoryDrilldownJSON":  categoryDrilldownJSON,
			// Totals.
			"TotalSpending":          totalSpending,
			"TotalIncome":            totalIncome,
			"PrevTotalSpending":      prevTotalSpending,
			"SpendingChangePercent":  spendingChangePercent,
			"HasSpendingChange":      hasSpendingChange,
			// Cash flow.
			"CashFlowNet":            cashFlowNet,
			"SavingsRate":            savingsRate,
			"HasCashFlow":            hasCashFlow,
			"SpendingRatio":          spendingRatio,
			// Spending pace.
			"HasPaceData":            hasPaceData,
			"CurrentMonthSpending":   currentMonthSpending,
			"CurrentMonthIncome":     currentMonthIncome,
			"LastMonthSpending":      lastMonthSpending,
			"LastMonthPaceSpending":  lastMonthPaceSpending,
			"DailyAvgSpending":       dailyAvgSpending,
			"ProjectedMonthly":       projectedMonthly,
			"PacePercent":            pacePercent,
			"PaceVsLastMonth":        paceVsLastMonth,
			"DaysElapsed":            daysElapsed,
			"DaysInMonth":            daysInMonth,
			"DaysRemaining":          daysRemaining,
			"MonthProgress":          monthProgress,
			"CurrentMonthName":       today.Format("January"),
			"LastMonthName":          lastMonthStart.Format("January"),
			// Smart spending insights.
			"SpendingInsights":       spendingInsights,
			// Merchant analysis.
			"TopMerchants":           topMerchants,
			// Day-of-week spending.
			"DOWSpending":            dowSpending,
			"HasDOWData":             hasDOWData,
			"HighSpendDay":           highSpendDay,
			"LowSpendDay":            lowSpendDay,
			"HighSpendDayAmt":        highSpendDayAmt,
			"LowSpendDayAmt":         lowSpendDayAmt,
			// Recurring charges.
			"RecurringCharges":       recurringCharges,
			"HasRecurringData":       hasRecurringData,
			"TotalRecurringMonthly":  totalRecurringMonthly,
			// Monthly category comparison.
			"HasMonthlyComp":         hasMonthlyComp,
			"MonthHeaders":           monthHeaders,
			"MonthlyCompRows":        monthlyCompRows,
			"MonthlyTotals":          monthlyTotals,
			"MaxMonthlyTotal":        maxMonthlyTotal,
			// Spending velocity.
			"HasVelocityData":        hasVelocityData,
			"VelocityJSON":           velocityJSON,
			"VelocityMonths":         velocityMonths,
			"VelocityMaxDays":        velocityMaxDays,
			// Family spending breakdown.
			"HasFamilyData":          hasFamilyData,
			"FamilySpending":         familySpending,
			// Savings rate trend.
			"HasSavingsRateTrend":       hasSavingsRateTrend,
			"SavingsRateTrend":          savingsRateTrend,
			"SavingsRateTrendJSON":      savingsRateTrendJSON,
			// Cash flow forecast.
			"HasForecastData":           hasForecastData,
			"ForecastJSON":              forecastJSON,
			// Anomaly detection.
			"HasCategoryAnomalies":      hasCategoryAnomalies,
			"CategoryAnomalies":         categoryAnomalies,
			"HasTransactionAnomalies":   hasTransactionAnomalies,
			"TransactionAnomalies":      transactionAnomalies,
		}
		tr.Render(w, r, "insights.html", data)
	}
}
