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
		// Category palette: oklch colors balanced for both light and dark mode.
		// Lightness ~0.62-0.68 gives good contrast on white and dark backgrounds.
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

		historyDays := 90
		if chartDays >= 90 {
			historyDays = 365
		}
		histStart := time.Now().AddDate(0, 0, -historyDays)
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
		type SpendingInsight struct {
			Icon      string
			Title     string
			Detail    string
			Amount    float64
			Change    float64
			Sentiment string
			Category  string
		}
		type MonthlyCompRow struct {
			Category      string
			CategoryColor string
			Amounts       []float64
			Total         float64
			Change        float64
			HasChange     bool
		}
		type VelocityMonth struct {
			Label      string
			ShortLabel string
			IsCurrent  bool
			Days       []float64
			FinalTotal float64
		}
		type FamilyMemberSpend struct {
			UserID   string
			UserName string
			Total    float64
			Percent  float64
			TxCount  int
			TopCat   string
			Color    string
		}
		type SavingsRatePoint struct {
			Month     string
			MonthFull string
			Income    float64
			Spending  float64
			Net       float64
			Rate      float64
		}
		type ForecastPoint struct {
			Day       int     `json:"day"`
			DateLabel string  `json:"dateLabel"`
			Actual    float64 `json:"actual"`
			Forecast  float64 `json:"forecast"`
			IsActual  bool    `json:"isActual"`
		}
		type CategoryAnomaly struct {
			Category   string
			Color      string
			Current    float64
			Historical float64
			Multiplier float64
			Percentile float64
			TopTxName  string
			TopTxAmount float64
			TopTxDate  string
		}
		type TransactionAnomaly struct {
			Name          string
			Amount        float64
			Date          string
			Category      string
			CategoryColor string
			CategoryAvg   float64
			Multiplier    float64
			AccountName   string
		}
		type BudgetTarget struct {
			Category    string
			Color       string
			Current     float64
			Target      float64
			Percent     float64
			OverBudget  bool
			Difference  float64
			DiffPercent float64
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
		var histCatSummary *service.TransactionSummaryResult
		categoryDrilldown := make(map[string][]CategoryMerchant)
		var topMerchants []MerchantSpend
		var maxMerchantSpend float64
		dowSpending := make([]DayOfWeekSpend, 7)
		var maxDaySpend float64
		var recurringCharges []RecurringCharge
		var totalRecurringMonthly float64
		var hasRecurringData bool
		var velocityMonths []VelocityMonth
		var hasVelocityData bool
		var velocityMaxDays int
		var savingsRateTrend []SavingsRatePoint
		var hasSavingsRateTrend bool
		var familySpending []FamilyMemberSpend
		var hasFamilyData bool
		var maxFamilySpend float64
		var forecastPoints []ForecastPoint
		var hasForecastData bool
		var transactionAnomalies []TransactionAnomaly
		var hasTransactionAnomalies bool
		var largestTxName string
		var largestTxAmount float64
		var largestTxDate string
		var recurringMerchant string
		var recurringCount int64
		var recurringTotal float64
		var sparklineSpending []float64
		var sparklineIncome []float64

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

		// 11. Top merchants
		wg.Add(1)
		go func() {
			defer wg.Done()
			rows, err := a.DB.Query(ctx, `
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

		// 13. Recurring charges
		wg.Add(1)
		go func() {
			defer wg.Done()
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

		// 15. Spending velocity (3 months)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for mi := 2; mi >= 0; mi-- {
				mStart := time.Date(today.Year(), today.Month()-time.Month(mi), 1, 0, 0, 0, 0, today.Location())
				mEnd := time.Date(mStart.Year(), mStart.Month()+1, 1, 0, 0, 0, 0, today.Location())
				mDaysInMonth := time.Date(mStart.Year(), mStart.Month()+1, 0, 0, 0, 0, 0, today.Location()).Day()

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

				vRows, vErr := a.DB.Query(ctx, `
					SELECT EXTRACT(DAY FROM date)::int AS day_num, SUM(amount)
					FROM transactions
					WHERE deleted_at IS NULL AND date >= $1 AND date < $2 AND amount > 0 AND pending = false
					GROUP BY EXTRACT(DAY FROM date)::int
					ORDER BY day_num
				`, mStart, mEnd)
				if vErr != nil {
					a.Logger.Error("velocity query", "error", vErr)
				} else {
					for vRows.Next() {
						var dayNum int
						var dayTotal float64
						if err := vRows.Scan(&dayNum, &dayTotal); err != nil {
							continue
						}
						if dayNum >= 1 && dayNum <= mDaysInMonth {
							vm.Days[dayNum-1] = dayTotal
						}
					}
					vRows.Close()
				}

				var cumulative float64
				for d := 0; d < mDaysInMonth; d++ {
					cumulative += vm.Days[d]
					vm.Days[d] = cumulative
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
		}()

		// 16. Savings rate trend (6 months)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for mi := 5; mi >= 0; mi-- {
				mStart := time.Date(today.Year(), today.Month()-time.Month(mi), 1, 0, 0, 0, 0, today.Location())
				mEnd := time.Date(mStart.Year(), mStart.Month()+1, 1, 0, 0, 0, 0, today.Location())

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
		}()

		// 17. Family spending
		wg.Add(1)
		go func() {
			defer wg.Done()
			rows, fErr := a.DB.Query(ctx, `
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
				return
			}
			defer rows.Close()
			familyColors := []string{
				"oklch(0.60 0.15 250)",
				"oklch(0.58 0.14 160)",
				"oklch(0.60 0.14 35)",
				"oklch(0.55 0.14 300)",
				"oklch(0.58 0.12 80)",
			}
			for rows.Next() {
				var fs FamilyMemberSpend
				var topCat *string
				if err := rows.Scan(&fs.UserID, &fs.UserName, &fs.Total, &fs.TxCount, &topCat); err != nil {
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
			hasFamilyData = len(familySpending) > 1
		}()

		// 18. Cash flow forecast
		wg.Add(1)
		go func() {
			defer wg.Done()
			type dailyFlow struct {
				Income   float64
				Spending float64
			}
			dailyFlows := make(map[int]dailyFlow)
			rows, err := a.DB.Query(ctx, `
				SELECT EXTRACT(DAY FROM date)::int AS day_num, amount
				FROM transactions
				WHERE deleted_at IS NULL
					AND date >= $1
					AND date < $2
					AND pending = false
			`, monthStart, time.Date(today.Year(), today.Month()+1, 1, 0, 0, 0, 0, today.Location()))
			if err != nil {
				a.Logger.Error("forecast flow query", "error", err)
				return
			}
			defer rows.Close()
			for rows.Next() {
				var dayNum int
				var amt float64
				if err := rows.Scan(&dayNum, &amt); err != nil {
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

			if len(dailyFlows) > 0 && daysElapsed > 0 {
				hasForecastData = true
				var totalDailyIncome, totalDailySpending float64
				for d := 1; d <= daysElapsed; d++ {
					df := dailyFlows[d]
					totalDailyIncome += df.Income
					totalDailySpending += df.Spending
				}
				avgDailyIncome := totalDailyIncome / float64(daysElapsed)
				avgDailySpending := totalDailySpending / float64(daysElapsed)

				var cumulativeNet float64
				for d := 1; d <= daysInMonth; d++ {
					pt := ForecastPoint{
						Day:       d,
						DateLabel: time.Date(today.Year(), today.Month(), d, 0, 0, 0, 0, today.Location()).Format("Jan 2"),
					}
					if d <= daysElapsed {
						df := dailyFlows[d]
						dayNet := df.Income - df.Spending
						cumulativeNet += dayNet
						pt.Actual = math.Round(cumulativeNet*100) / 100
						pt.Forecast = math.Round(cumulativeNet*100) / 100
						pt.IsActual = true
					} else {
						projectedDayNet := avgDailyIncome - avgDailySpending
						cumulativeNet += projectedDayNet
						pt.Forecast = math.Round(cumulativeNet*100) / 100
						pt.IsActual = false
					}
					forecastPoints = append(forecastPoints, pt)
				}
			}
		}()

		// 19. Historical category summary (for anomalies + budgets)
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := svc.GetTransactionSummary(ctx, service.TransactionSummaryParams{
				GroupBy:      "category",
				StartDate:    &histStart,
				SpendingOnly: true,
			})
			if err != nil {
				a.Logger.Error("anomaly historical summary", "error", err)
				return
			}
			histCatSummary = result
		}()

		// 20. Transaction anomalies
		wg.Add(1)
		go func() {
			defer wg.Done()
			rows, err := a.DB.Query(ctx, `
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
			if err != nil {
				a.Logger.Error("transaction anomaly query", "error", err)
				return
			}
			defer rows.Close()
			for rows.Next() {
				var ta TransactionAnomaly
				if err := rows.Scan(&ta.Name, &ta.Amount, &ta.Date, &ta.Category, &ta.CategoryAvg, &ta.Multiplier, &ta.AccountName); err != nil {
					a.Logger.Error("transaction anomaly scan", "error", err)
					continue
				}
				transactionAnomalies = append(transactionAnomalies, ta)
			}
			hasTransactionAnomalies = len(transactionAnomalies) > 0
		}()

		// 21. Largest transaction
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = a.DB.QueryRow(ctx, `
				SELECT COALESCE(merchant_name, name), amount, TO_CHAR(date, 'Mon DD')
				FROM transactions
				WHERE deleted_at IS NULL AND date >= $1 AND amount > 0 AND pending = false
				ORDER BY amount DESC
				LIMIT 1
			`, chartStart).Scan(&largestTxName, &largestTxAmount, &largestTxDate)
		}()

		// 22. Recurring merchant insight
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = a.DB.QueryRow(ctx, `
				SELECT COALESCE(merchant_name, name), COUNT(*), SUM(amount)
				FROM transactions
				WHERE deleted_at IS NULL AND date >= $1 AND amount > 0 AND pending = false
				GROUP BY COALESCE(merchant_name, name)
				HAVING COUNT(*) >= 3
				ORDER BY SUM(amount) DESC
				LIMIT 1
			`, chartStart).Scan(&recurringMerchant, &recurringCount, &recurringTotal)
		}()

		// 23. Sparkline data — last 7 days of daily spending and income for summary pills
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

		// ── Family spending percentages ──
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

		// ── Smart spending insights ──
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

		if largestTxAmount >= 50 {
			spendingInsights = append(spendingInsights, SpendingInsight{
				Icon:      "receipt",
				Title:     fmt.Sprintf("Largest: $%.0f", largestTxAmount),
				Detail:    fmt.Sprintf("%s on %s", largestTxName, largestTxDate),
				Amount:    largestTxAmount,
				Sentiment: "neutral",
			})
		}

		if recurringCount >= 3 {
			spendingInsights = append(spendingInsights, SpendingInsight{
				Icon:      "repeat",
				Title:     fmt.Sprintf("%s: %dx", recurringMerchant, recurringCount),
				Detail:    fmt.Sprintf("$%.0f total across %d transactions", recurringTotal, recurringCount),
				Amount:    recurringTotal,
				Sentiment: "info",
			})
		}

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

		// ── Velocity JSON ──
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

		// ── Savings rate trend JSON ──
		var savingsRateTrendJSON template.JS
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

		// ── Forecast JSON ──
		var forecastJSON template.JS
		if hasForecastData && len(forecastPoints) > 0 {
			type forecastChartData struct {
				Labels       []string  `json:"labels"`
				Actual       []any     `json:"actual"`
				Forecast     []float64 `json:"forecast"`
				DaysElapsed  int       `json:"daysElapsed"`
				AvgDailyNet  float64   `json:"avgDailyNet"`
				ProjectedEOM float64   `json:"projectedEOM"`
			}
			fcd := forecastChartData{
				DaysElapsed: daysElapsed,
			}
			// Recompute avg daily net from forecast points.
			if daysElapsed > 0 {
				var lastActual float64
				for _, pt := range forecastPoints {
					if pt.IsActual {
						lastActual = pt.Actual
					}
				}
				fcd.AvgDailyNet = math.Round((lastActual/float64(daysElapsed))*100) / 100
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
			fcd.ProjectedEOM = forecastPoints[len(forecastPoints)-1].Forecast
			if fb, err := json.Marshal(fcd); err == nil {
				forecastJSON = template.JS(fb)
			}
		}

		// ── Anomaly detection (category anomalies) ──
		numPeriods := float64(historyDays) / float64(chartDays)
		if numPeriods < 1 {
			numPeriods = 1
		}

		histCatAvg := make(map[string]float64)
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

		// Assign colors to transaction anomalies.
		for i := range transactionAnomalies {
			ta := &transactionAnomalies[i]
			ta.CategoryColor = histCatColor[ta.Category]
			if ta.CategoryColor == "" {
				ta.CategoryColor = categoryPalette[i%len(categoryPalette)]
			}
		}

		var categoryAnomalies []CategoryAnomaly
		var hasCategoryAnomalies bool
		for cat, curAmt := range currentCatMap {
			avgAmt, exists := histCatAvg[cat]
			if !exists || avgAmt < 20 {
				continue
			}
			multiplier := curAmt / avgAmt
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
		sort.Slice(categoryAnomalies, func(i, j int) bool {
			return categoryAnomalies[i].Multiplier > categoryAnomalies[j].Multiplier
		})
		if len(categoryAnomalies) > 6 {
			categoryAnomalies = categoryAnomalies[:6]
		}
		hasCategoryAnomalies = len(categoryAnomalies) > 0

		// ── Budget targets ──
		var budgetTargets []BudgetTarget
		var hasBudgetTargets bool
		var budgetOnTrackCount, budgetOverCount int

		for _, tc := range topCategories {
			avgAmt, exists := histCatAvg[tc.Name]
			if !exists || avgAmt < 10 {
				continue
			}
			pct := (tc.Amount / avgAmt) * 100
			diff := tc.Amount - avgAmt
			diffPct := 0.0
			if avgAmt > 0 {
				diffPct = (diff / avgAmt) * 100
			}
			bt := BudgetTarget{
				Category:    tc.Name,
				Color:       tc.Color,
				Current:     tc.Amount,
				Target:      avgAmt,
				Percent:     pct,
				OverBudget:  pct > 105,
				Difference:  diff,
				DiffPercent: diffPct,
			}
			budgetTargets = append(budgetTargets, bt)
			if bt.OverBudget {
				budgetOverCount++
			} else {
				budgetOnTrackCount++
			}
		}
		hasBudgetTargets = len(budgetTargets) > 0

		// ── Financial Health Score (composite 0-100) ──
		// Five dimensions, each scored 0-20 points:
		//   1. Savings Rate: higher savings = higher score
		//   2. Budget Adherence: % of categories on-track
		//   3. Spending Trend: spending down vs previous period
		//   4. Cash Flow: positive net = good, negative = bad
		//   5. Spending Pace: on-track for month vs overshooting
		type HealthDimension struct {
			Name        string
			Score       int
			MaxScore    int
			Description string
			Icon        string
			Color       string
		}

		var healthScore int
		var healthDimensions []HealthDimension
		var hasHealthScore bool

		// Only compute if we have meaningful data.
		if totalSpending > 0 || totalIncome > 0 {
			hasHealthScore = true

			// Dimension 1: Savings Rate (0-20)
			savingsScore := 0
			savingsDesc := "No income data"
			if totalIncome > 0 {
				sr := savingsRate
				switch {
				case sr >= 30:
					savingsScore = 20
					savingsDesc = "Excellent savings rate"
				case sr >= 20:
					savingsScore = 17
					savingsDesc = "Strong savings rate"
				case sr >= 10:
					savingsScore = 14
					savingsDesc = "Good savings rate"
				case sr >= 0:
					savingsScore = 10
					savingsDesc = "Break-even or minimal savings"
				case sr >= -20:
					savingsScore = 6
					savingsDesc = "Spending slightly exceeds income"
				case sr >= -50:
					savingsScore = 3
					savingsDesc = "Spending significantly exceeds income"
				default:
					savingsScore = 0
					savingsDesc = "Spending far exceeds income"
				}
			}
			healthDimensions = append(healthDimensions, HealthDimension{
				Name: "Savings Rate", Score: savingsScore, MaxScore: 20,
				Description: savingsDesc, Icon: "piggy-bank", Color: "oklch(0.65 0.17 155)",
			})

			// Dimension 2: Budget Adherence (0-20)
			budgetScore := 10 // Default if no budget data
			budgetDesc := "No budget data yet"
			if len(budgetTargets) > 0 {
				total := budgetOnTrackCount + budgetOverCount
				if total > 0 {
					onTrackRatio := float64(budgetOnTrackCount) / float64(total)
					budgetScore = int(math.Round(onTrackRatio * 20))
					switch {
					case onTrackRatio >= 0.8:
						budgetDesc = fmt.Sprintf("%d of %d categories on track", budgetOnTrackCount, total)
					case onTrackRatio >= 0.5:
						budgetDesc = fmt.Sprintf("%d of %d categories over budget", budgetOverCount, total)
					default:
						budgetDesc = fmt.Sprintf("Most categories (%d/%d) over budget", budgetOverCount, total)
					}
				}
			}
			healthDimensions = append(healthDimensions, HealthDimension{
				Name: "Budget Adherence", Score: budgetScore, MaxScore: 20,
				Description: budgetDesc, Icon: "target", Color: "oklch(0.62 0.15 250)",
			})

			// Dimension 3: Spending Trend (0-20)
			trendScore := 10 // Default if no comparison data
			trendDesc := "Not enough history"
			if hasSpendingChange {
				pct := spendingChangePercent
				switch {
				case pct <= -20:
					trendScore = 20
					trendDesc = fmt.Sprintf("Spending down %.0f%% vs previous period", math.Abs(pct))
				case pct <= -10:
					trendScore = 17
					trendDesc = fmt.Sprintf("Spending down %.0f%%", math.Abs(pct))
				case pct <= -2:
					trendScore = 14
					trendDesc = fmt.Sprintf("Spending slightly down %.0f%%", math.Abs(pct))
				case pct <= 2:
					trendScore = 12
					trendDesc = "Spending roughly flat"
				case pct <= 10:
					trendScore = 8
					trendDesc = fmt.Sprintf("Spending up %.0f%%", pct)
				case pct <= 25:
					trendScore = 4
					trendDesc = fmt.Sprintf("Spending up %.0f%%", pct)
				default:
					trendScore = 0
					trendDesc = fmt.Sprintf("Spending up %.0f%% vs previous period", pct)
				}
			}
			healthDimensions = append(healthDimensions, HealthDimension{
				Name: "Spending Trend", Score: trendScore, MaxScore: 20,
				Description: trendDesc, Icon: "trending-down", Color: "oklch(0.64 0.16 160)",
			})

			// Dimension 4: Cash Flow (0-20)
			cashFlowScore := 10
			cashFlowDesc := "No cash flow data"
			if hasCashFlow && totalIncome > 0 {
				ratio := cashFlowNet / totalIncome
				switch {
				case ratio >= 0.3:
					cashFlowScore = 20
					cashFlowDesc = "Strong positive cash flow"
				case ratio >= 0.1:
					cashFlowScore = 16
					cashFlowDesc = "Positive cash flow"
				case ratio >= 0:
					cashFlowScore = 12
					cashFlowDesc = "Near break-even"
				case ratio >= -0.2:
					cashFlowScore = 6
					cashFlowDesc = "Negative cash flow"
				default:
					cashFlowScore = 0
					cashFlowDesc = "Significant negative cash flow"
				}
			}
			healthDimensions = append(healthDimensions, HealthDimension{
				Name: "Cash Flow", Score: cashFlowScore, MaxScore: 20,
				Description: cashFlowDesc, Icon: "wallet", Color: "oklch(0.66 0.14 35)",
			})

			// Dimension 5: Spending Pace (0-20)
			paceScore := 10 // Default
			paceDesc := "No pace data"
			if hasPaceData && lastMonthSpending > 0 {
				switch paceVsLastMonth {
				case "behind":
					paceScore = 18
					paceDesc = fmt.Sprintf("Spending %.0f%% less than last month's pace", math.Abs(pacePercent))
				case "same":
					paceScore = 12
					paceDesc = "On pace with last month"
				case "ahead":
					if pacePercent <= 10 {
						paceScore = 8
						paceDesc = fmt.Sprintf("Slightly ahead of last month (+%.0f%%)", pacePercent)
					} else if pacePercent <= 25 {
						paceScore = 4
						paceDesc = fmt.Sprintf("Spending faster than last month (+%.0f%%)", pacePercent)
					} else {
						paceScore = 0
						paceDesc = fmt.Sprintf("Spending much faster than last month (+%.0f%%)", pacePercent)
					}
				}
			}
			healthDimensions = append(healthDimensions, HealthDimension{
				Name: "Monthly Pace", Score: paceScore, MaxScore: 20,
				Description: paceDesc, Icon: "gauge", Color: "oklch(0.60 0.16 300)",
			})

			// Sum up total health score
			for _, d := range healthDimensions {
				healthScore += d.Score
			}
		}

		// Health score label and color
		healthLabel := "N/A"
		healthColorClass := "text-base-content/50"
		if hasHealthScore {
			switch {
			case healthScore >= 80:
				healthLabel = "Excellent"
				healthColorClass = "text-success"
			case healthScore >= 65:
				healthLabel = "Good"
				healthColorClass = "text-success/70"
			case healthScore >= 50:
				healthLabel = "Fair"
				healthColorClass = "text-warning"
			case healthScore >= 35:
				healthLabel = "Needs Attention"
				healthColorClass = "text-warning/70"
			default:
				healthLabel = "Critical"
				healthColorClass = "text-error"
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
			// Budget targets
			Budgets []struct {
				Category string  `json:"category"`
				Current  float64 `json:"current"`
				Target   float64 `json:"target"`
				Percent  float64 `json:"percent"`
			} `json:"budgets"`
			// Anomalies
			CategoryAnomalies []struct {
				Category   string  `json:"category"`
				Current    float64 `json:"current"`
				Historical float64 `json:"historical"`
				Multiplier float64 `json:"multiplier"`
			} `json:"categoryAnomalies"`
			TransactionAnomalies []struct {
				Name        string  `json:"name"`
				Amount      float64 `json:"amount"`
				Date        string  `json:"date"`
				Category    string  `json:"category"`
				CategoryAvg float64 `json:"categoryAvg"`
				Multiplier  float64 `json:"multiplier"`
			} `json:"transactionAnomalies"`
			// Savings rate trend
			SavingsRateTrend []struct {
				Month    string  `json:"month"`
				Income   float64 `json:"income"`
				Spending float64 `json:"spending"`
				Net      float64 `json:"net"`
				Rate     float64 `json:"rate"`
			} `json:"savingsRateTrend"`
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
		for _, bt := range budgetTargets {
			exportData.Budgets = append(exportData.Budgets, struct {
				Category string  `json:"category"`
				Current  float64 `json:"current"`
				Target   float64 `json:"target"`
				Percent  float64 `json:"percent"`
			}{bt.Category, bt.Current, bt.Target, bt.Percent})
		}
		for _, ca := range categoryAnomalies {
			exportData.CategoryAnomalies = append(exportData.CategoryAnomalies, struct {
				Category   string  `json:"category"`
				Current    float64 `json:"current"`
				Historical float64 `json:"historical"`
				Multiplier float64 `json:"multiplier"`
			}{ca.Category, ca.Current, ca.Historical, ca.Multiplier})
		}
		for _, ta := range transactionAnomalies {
			exportData.TransactionAnomalies = append(exportData.TransactionAnomalies, struct {
				Name        string  `json:"name"`
				Amount      float64 `json:"amount"`
				Date        string  `json:"date"`
				Category    string  `json:"category"`
				CategoryAvg float64 `json:"categoryAvg"`
				Multiplier  float64 `json:"multiplier"`
			}{ta.Name, ta.Amount, ta.Date, ta.Category, ta.CategoryAvg, ta.Multiplier})
		}
		for _, sr := range savingsRateTrend {
			exportData.SavingsRateTrend = append(exportData.SavingsRateTrend, struct {
				Month    string  `json:"month"`
				Income   float64 `json:"income"`
				Spending float64 `json:"spending"`
				Net      float64 `json:"net"`
				Rate     float64 `json:"rate"`
			}{sr.Month, sr.Income, sr.Spending, sr.Net, sr.Rate})
		}

		var exportJSON template.JS
		if eb, err := json.Marshal(exportData); err == nil {
			exportJSON = template.JS(eb)
		}

		// ── Sparkline data for summary pills ──
		// Compute net cash flow and savings rate sparklines from spending/income.
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
			// Category budget targets.
			"HasBudgetTargets":          hasBudgetTargets,
			"BudgetTargets":             budgetTargets,
			"BudgetOnTrackCount":        budgetOnTrackCount,
			"BudgetOverCount":           budgetOverCount,
			// CSV Export.
			"ExportJSON":                exportJSON,
			// Sparkline data for summary pills.
			"SparkSpending":             sparkSpendJSON,
			"SparkIncome":               sparkIncomeJSON,
			"SparkNet":                  sparkNetJSON,
			"SparkSavings":              sparkSavingsJSON,
			// Financial health score.
			"HasHealthScore":            hasHealthScore,
			"HealthScore":               healthScore,
			"HealthLabel":               healthLabel,
			"HealthColorClass":          healthColorClass,
			"HealthDimensions":          healthDimensions,
		}
		tr.Render(w, r, "insights.html", data)
	}
}
