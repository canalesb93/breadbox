package admin

import (
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"net/http"
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
		}
		tr.Render(w, r, "insights.html", data)
	}
}
