//go:build !headless && !lite

package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"breadbox/internal/app"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	"breadbox/internal/templates/components"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// SubscriptionsListPageHandler serves GET /subscriptions — the single
// human-adjudication surface for recurring series. Candidates (auto-detected,
// awaiting a verdict) are split out from the confirmed/live ledger, and the
// stat tiles (active count, monthly-equivalent spend per currency, candidates
// awaiting review) are computed here.
func SubscriptionsListPageHandler(a *app.App, svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		all, err := svc.ListSeries(ctx, nil)
		if err != nil {
			a.Logger.Error("list series", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		catName := subscriptionCategoryNames(ctx, a)
		userName := subscriptionUserNames(ctx, a)

		var candidates, active []pages.SubscriptionRow
		monthlyByCurrency := map[string]float64{}
		var currencyOrder []string
		activeCount := 0
		usersWithSeries := map[string]bool{}
		typesPresent := map[string]bool{}
		upcomingByCurrency := map[string]float64{}
		var upcomingOrder []string
		upcomingCount := 0

		for _, s := range all {
			// A rejected verdict ("Not a subscription") is a dismissal — it
			// leaves confidence='rejected' but keeps status='candidate' as the
			// sticky suppression key (§6.3). Such rows must not reappear in the
			// review queue or the ledger.
			if s.Confidence == service.SeriesConfidenceRejected {
				continue
			}
			row := subscriptionRow(s, catName, userName)
			if row.UserID != "" {
				usersWithSeries[row.UserID] = true
			}
			if row.Type != "" {
				typesPresent[row.Type] = true
			}
			if s.Status == service.SeriesStatusCandidate {
				// Detection evidence (the grouped charges) is shown on the
				// candidate's detail page, which the Needs-review row links to —
				// so the list view no longer fetches it per candidate.
				candidates = append(candidates, row)
				continue
			}
			active = append(active, row)
			if s.Status == service.SeriesStatusActive {
				activeCount++
				if row.HasAmount {
					cur := row.Currency
					if cur == "" {
						cur = "USD"
					}
					if _, seen := monthlyByCurrency[cur]; !seen {
						currencyOrder = append(currencyOrder, cur)
					}
					monthlyByCurrency[cur] += monthlyEquivalent(s.Cadence, row.Amount)
					// Upcoming spend: the next charge lands within the next 30 days.
					if row.DaysUntilRenewal != nil && *row.DaysUntilRenewal >= 0 && *row.DaysUntilRenewal <= 30 {
						if _, seen := upcomingByCurrency[cur]; !seen {
							upcomingOrder = append(upcomingOrder, cur)
						}
						upcomingByCurrency[cur] += row.Amount
						upcomingCount++
					}
				}
			}
		}

		// Order the ledger by renewal urgency: overdue → due-soon → upcoming
		// (ascending days), then series with no projection, then likely-cancelled
		// (stale) last. Surfaces "what's renewing soon" at the top, leveraging the
		// renewal chip, without a separate section.
		sort.SliceStable(active, func(i, j int) bool {
			gi, gj := renewalSortGroup(active[i]), renewalSortGroup(active[j])
			if gi != gj {
				return gi < gj
			}
			return renewalSortDays(active[i]) < renewalSortDays(active[j])
		})

		var monthlyTotals []pages.SubscriptionMonthlyTotal
		for _, cur := range currencyOrder {
			monthlyTotals = append(monthlyTotals, pages.SubscriptionMonthlyTotal{
				Currency: cur,
				Amount:   monthlyByCurrency[cur],
			})
		}
		var upcomingTotals []pages.SubscriptionMonthlyTotal
		for _, cur := range upcomingOrder {
			upcomingTotals = append(upcomingTotals, pages.SubscriptionMonthlyTotal{
				Currency: cur,
				Amount:   upcomingByCurrency[cur],
			})
		}

		activeTab := "active"
		if r.URL.Query().Get("tab") == "review" {
			activeTab = "review"
		}

		props := pages.SubscriptionsListProps{
			CSRFToken:      GetCSRFToken(r),
			ActiveTab:      activeTab,
			ActiveCount:    activeCount,
			CandidateCount: len(candidates),
			MonthlyTotals:  monthlyTotals,
			UpcomingTotals: upcomingTotals,
			UpcomingCount:  upcomingCount,
			Candidates:     candidates,
			Active:         active,
			Users:          subscriptionUserFilters(userName, usersWithSeries),
			Types:          subscriptionTypeFilters(typesPresent),
		}

		data := map[string]any{
			"PageTitle":   "Recurring",
			"CurrentPage": "recurring",
			"CSRFToken":   GetCSRFToken(r),
			"Flash":       GetFlash(ctx, sm),
		}
		tr.RenderWithTempl(w, r, data, pages.SubscriptionsList(props))
	}
}

// SubscriptionDetailHandler serves GET /subscriptions/{id}.
func SubscriptionDetailHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		idStr := chi.URLParam(r, "id")

		s, err := svc.GetSeries(ctx, idStr)
		if err != nil {
			tr.RenderNotFound(w, r)
			return
		}
		catName := subscriptionCategoryNames(ctx, a)
		userName := subscriptionUserNames(ctx, a)
		row := subscriptionRow(*s, catName, userName)

		members, err := svc.SeriesMembers(ctx, idStr)
		if err != nil {
			a.Logger.Error("series members", "error", err)
		}
		// Canonical transaction rows for the "Charges in this series" list,
		// rendered via the shared TxRowCompact (carries its own date/account/
		// category) — same row as the /transactions list.
		memberIDs := make([]string, 0, len(members))
		for _, m := range members {
			memberIDs = append(memberIDs, m.ShortID)
		}
		memberRows, mrErr := svc.GetAdminTransactionRowsByIDs(ctx, memberIDs)
		if mrErr != nil {
			a.Logger.Error("series member rows", "error", mrErr)
		}

		// Tags: full vocabulary (seeds the shared picker), chip data for the
		// tags currently on the series, and the legacy add-select options —
		// all from one ListTags call.
		onSeries := map[string]bool{}
		for _, tg := range row.Tags {
			onSeries[tg] = true
		}
		allTags, _ := svc.ListTags(ctx)
		bySlug := make(map[string]service.TagResponse, len(allTags))
		var tagOptions []pages.SubscriptionTagOption
		var tagChips []components.TagChipData
		for _, t := range allTags {
			bySlug[t.Slug] = t
			if onSeries[t.Slug] {
				continue
			}
			name := t.DisplayName
			if name == "" {
				name = t.Slug
			}
			tagOptions = append(tagOptions, pages.SubscriptionTagOption{Slug: t.Slug, Name: name})
		}
		for _, slug := range row.Tags {
			if t, ok := bySlug[slug]; ok {
				tagChips = append(tagChips, components.TagChipDataFromResponse(t))
			} else {
				tagChips = append(tagChips, components.TagChipData{Slug: slug, DisplayName: slug})
			}
		}

		catTree, _ := svc.ListCategories(ctx)

		props := pages.SubscriptionDetailProps{
			CSRFToken:            GetCSRFToken(r),
			Series:               row,
			ExpectedAmount:       subscriptionMoney(s.ExpectedAmount, deref(s.IsoCurrencyCode)),
			AmountTolerance:      subscriptionMoney(s.AmountTolerance, deref(s.IsoCurrencyCode)),
			ExpectedDay:          subscriptionExpectedDay(s.ExpectedDay),
			NextExpected:         formatSubDate(s.NextExpectedDate, "Jan 2, 2006"),
			LastSeen:             formatSubDate(s.LastSeenDate, "Jan 2, 2006"),
			Confidence:           s.Confidence,
			HasExpectedAmount:    s.ExpectedAmount != nil,
			ExpectedAmountValue:  derefFloat(s.ExpectedAmount),
			AmountToleranceValue: derefFloat(s.AmountTolerance),
			ExpectedDayValue:     derefInt(s.ExpectedDay),
			CurrentCategoryID:    deref(s.CategoryID),
			Categories:           flattenCategoryOptions(catTree, ""),
			MemberRows:           memberRows,
			PriceChanges:         subscriptionPriceChanges(members),
			AvailableTags:        tagOptions,
			TagChips:             tagChips,
			Detection:            assembleSeriesDetection(*s, members),
			Evidence:             assembleSeriesEvidence(*s, members),
			Facts:                assembleSeriesFacts(*s, row),
			CategoryTree:         catTree,
			AllTags:              allTags,
			CurrentTagSlugs:      row.Tags,
		}

		data := map[string]any{
			"PageTitle":   s.Name + " — Recurring",
			"CurrentPage": "recurring",
			"CSRFToken":   GetCSRFToken(r),
			"Flash":       GetFlash(ctx, sm),
			"Breadcrumbs": []components.Breadcrumb{
				{Label: "Recurring", Href: "/recurring"},
				{Label: s.Name},
			},
		}
		tr.RenderWithTempl(w, r, data, pages.SubscriptionDetail(props))
	}
}

// --- Detection-forward detail assembly -------------------------------------

// seriesMatchStrength maps a series' confidence to the detection-panel badge.
func seriesMatchStrength(s service.SeriesResponse) (string, string) {
	if s.Confidence == service.SeriesConfidenceConfirmed {
		return "Strong match", "success"
	}
	band, _ := subscriptionConfidenceBand(s)
	switch band {
	case "High":
		return "Strong match", "success"
	case "Medium":
		return "Likely match", "warning"
	default:
		return "Weak match", "neutral"
	}
}

func seriesCadencePhrase(cadence string) string {
	switch cadence {
	case service.SeriesCadenceWeekly:
		return "about every week"
	case service.SeriesCadenceBiweekly:
		return "about every 2 weeks"
	case service.SeriesCadenceMonthly:
		return "about every month"
	case service.SeriesCadenceQuarterly:
		return "about every quarter"
	case service.SeriesCadenceSemiannual:
		return "about every 6 months"
	case service.SeriesCadenceAnnual:
		return "about every year"
	default:
		return "on an irregular cadence"
	}
}

func seriesDayPhrase(cadence string, day *int) string {
	if day == nil || *day < 1 {
		return ""
	}
	switch cadence {
	case service.SeriesCadenceMonthly, service.SeriesCadenceQuarterly,
		service.SeriesCadenceSemiannual, service.SeriesCadenceAnnual:
		return "around the " + ordinal(*day)
	default:
		return ""
	}
}

func ordinal(n int) string {
	suffix := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return fmt.Sprintf("%d%s", n, suffix)
}

func seriesMoney(v float64, currency string) string {
	if currency == "" || currency == "USD" {
		return fmt.Sprintf("$%.2f", v)
	}
	return fmt.Sprintf("%.2f %s", v, currency)
}

func seriesMoneyWhole(v float64, currency string) string {
	if currency == "" || currency == "USD" {
		return fmt.Sprintf("$%.0f", v)
	}
	return fmt.Sprintf("%.0f %s", v, currency)
}

func pctStr(v float64) string {
	if v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	return strconv.FormatFloat(v, 'f', 1, 64)
}

func chargesWord(n int) string {
	if n == 1 {
		return "1 charge"
	}
	return fmt.Sprintf("%d charges", n)
}

// assembleSeriesDetection builds the detection-summary panel: match-strength
// badge, plain-language sentence, and (when an amount is known) the match-window.
func assembleSeriesDetection(s service.SeriesResponse, members []service.SeriesMember) components.SeriesDetectionProps {
	cur := deref(s.IsoCurrencyCode)
	label, tone := seriesMatchStrength(s)
	d := components.SeriesDetectionProps{
		StrengthLabel: label,
		StrengthTone:  tone,
		MerchantKey:   s.MerchantKey,
		CadencePhrase: seriesCadencePhrase(s.Cadence),
		DayPhrase:     seriesDayPhrase(s.Cadence, s.ExpectedDay),
	}
	if s.ExpectedAmount != nil {
		d.HasAmount = true
		d.AmountText = seriesMoney(*s.ExpectedAmount, cur)
		d.ToleranceText = seriesMoney(derefFloat(s.AmountTolerance), cur)
		d.Window = buildSeriesMatchWindow(*s.ExpectedAmount, derefFloat(s.AmountTolerance), cur, members)
	}
	return d
}

func buildSeriesMatchWindow(expected, tol float64, cur string, members []service.SeriesMember) *components.SeriesMatchWindowProps {
	if tol <= 0 {
		tol = 1.0
	}
	lower, upper := expected-tol, expected+tol
	prior, hasPrior := seriesPriorPrice(members, lower)

	lo := lower
	if hasPrior && prior < lo {
		lo = prior
	}
	hi := upper
	span := hi - lo
	if span <= 0 {
		span = tol*2 + 1
	}
	pad := span * 0.35
	if pad < tol {
		pad = tol
	}
	axisMin := math.Floor(lo - pad)
	axisMax := math.Ceil(hi + pad)
	if axisMax <= axisMin {
		axisMax = axisMin + 1
	}
	pct := func(v float64) float64 { return (v - axisMin) / (axisMax - axisMin) * 100 }

	inWin := 0
	for _, m := range members {
		if m.Amount == nil {
			continue
		}
		a := math.Abs(*m.Amount)
		if a >= lower-0.005 && a <= upper+0.005 {
			inWin++
		}
	}

	w := &components.SeriesMatchWindowProps{
		RangeText:    seriesMoney(lower, cur) + " – " + seriesMoney(upper, cur),
		AxisMinText:  seriesMoneyWhole(axisMin, cur),
		AxisMaxText:  seriesMoneyWhole(axisMax, cur),
		ExpectedText: seriesMoney(expected, cur) + " expected",
		InWindowText: chargesWord(inWin),
		BandLeftPct:  pctStr(pct(lower)),
		BandWidthPct: pctStr(pct(upper) - pct(lower)),
		ExpectedPct:  pctStr(pct(expected)),
	}
	if hasPrior {
		w.HasPrior = true
		w.PriorPct = pctStr(pct(prior))
		w.PriorText = seriesMoney(prior, cur) + " · prior price"
	}
	return w
}

// seriesPriorPrice returns the highest member amount strictly below the match
// band's lower bound — the price tier before the most recent step-up.
func seriesPriorPrice(members []service.SeriesMember, lower float64) (float64, bool) {
	best, found := 0.0, false
	for _, m := range members {
		if m.Amount == nil {
			continue
		}
		a := math.Abs(*m.Amount)
		if a < lower-0.005 && (!found || a > best) {
			best, found = a, true
		}
	}
	return best, found
}

// assembleSeriesEvidence builds the charge timeline: a projected next charge,
// then the members newest-first with matched/prior markers and a price-change
// inset on the member where the amount first stepped to a new value.
func assembleSeriesEvidence(s service.SeriesResponse, members []service.SeriesMember) components.SeriesEvidenceProps {
	cur := deref(s.IsoCurrencyCode)
	hasExp := s.ExpectedAmount != nil
	expected := derefFloat(s.ExpectedAmount)
	tol := derefFloat(s.AmountTolerance)
	if tol <= 0 {
		tol = 1.0
	}

	// Transition points: walk oldest→newest (members are newest-first), mark a
	// member when its amount differs from the previous one.
	type transition struct{ from, to float64 }
	transitions := map[string]transition{}
	prev, havePrev := 0.0, false
	for i := len(members) - 1; i >= 0; i-- {
		m := members[i]
		if m.Amount == nil {
			continue
		}
		a := math.Abs(*m.Amount)
		if havePrev && math.Abs(a-prev) > 0.005 {
			transitions[m.ShortID] = transition{from: prev, to: a}
		}
		prev, havePrev = a, true
	}

	var rows []components.SeriesEvidenceRow
	if hasExp {
		if nd := formatSubDate(s.NextExpectedDate, "Jan 2, 2006"); nd != "" {
			rows = append(rows, components.SeriesEvidenceRow{
				Date: nd, AmountText: "~" + seriesMoney(expected, cur),
				State: "projected", Note: "projected", NoteTone: "neutral",
			})
		}
	}
	for _, m := range members {
		a := 0.0
		if m.Amount != nil {
			a = math.Abs(*m.Amount)
		}
		state := "prior"
		if !hasExp || math.Abs(a-expected) <= tol+0.005 {
			state = "matched"
		}
		row := components.SeriesEvidenceRow{
			ShortID:    m.ShortID,
			Date:       formatSubDate(m.Date, "Jan 2, 2006"),
			AmountText: seriesMoney(a, cur),
			State:      state,
		}
		if t, ok := transitions[m.ShortID]; ok {
			row.Note, row.NoteTone = "first at new price", "success"
			row.HasPrice = true
			row.PriceFrom = seriesMoney(t.from, cur)
			row.PriceTo = seriesMoney(t.to, cur)
		}
		rows = append(rows, row)
	}
	return components.SeriesEvidenceProps{Rows: rows, Unlinkable: true, SeriesShortID: s.ShortID}
}

func assembleSeriesFacts(s service.SeriesResponse, row pages.SubscriptionRow) components.SeriesFactStripProps {
	dash := func(v string) string {
		if v == "" {
			return "—"
		}
		return v
	}
	lastCharge := "—"
	if s.LastAmount != nil {
		lastCharge = subscriptionMoney(s.LastAmount, deref(s.IsoCurrencyCode))
	}
	return components.SeriesFactStripProps{Facts: []components.SeriesFact{
		{Label: "Last charge", Value: lastCharge, Private: "amount"},
		{Label: "Next renewal", Value: dash(formatSubDate(s.NextExpectedDate, "Jan 2, 2006"))},
		{Label: "Last seen", Value: dash(formatSubDate(s.LastSeenDate, "Jan 2, 2006"))},
		{Label: "Detected by", Value: dash(row.SourceLabel)},
	}}
}

// NewRecurringSeriesPageHandler renders the create-from-scratch form at
// GET /recurring/new — for a recurring charge the detector hasn't surfaced.
func NewRecurringSeriesPageHandler(a *app.App, svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		var categoryOptions []pages.SubscriptionCategoryOption
		if cats, cerr := svc.ListCategories(ctx); cerr == nil {
			categoryOptions = flattenCategoryOptions(cats, "")
		}
		props := pages.RecurringSeriesFormProps{
			CSRFToken:  GetCSRFToken(r),
			Type:       service.SeriesTypeSubscription,
			Cadence:    service.SeriesCadenceMonthly,
			Currency:   "USD",
			Categories: categoryOptions,
		}
		data := map[string]any{
			"PageTitle":   "New recurring series",
			"CurrentPage": "recurring",
			"CSRFToken":   GetCSRFToken(r),
			"Breadcrumbs": recurringFormBreadcrumbs(),
		}
		tr.RenderWithTempl(w, r, data, pages.RecurringSeriesForm(props))
	}
}

// recurringFormBreadcrumbs is the topbar trail for the new-series form,
// shared by the GET handler and its validation-error re-render.
func recurringFormBreadcrumbs() []components.Breadcrumb {
	return []components.Breadcrumb{
		{Label: "Recurring", Href: "/recurring"},
		{Label: "New series"},
	}
}

// CreateRecurringSeriesHandler handles POST /recurring/new — mints an ACTIVE,
// CONFIRMED, user-authored series (not a candidate in the review queue),
// deriving a merchant_key from the name, then redirects to its detail page.
func CreateRecurringSeriesHandler(a *app.App, svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(r.FormValue("name"))
		typ := strings.TrimSpace(r.FormValue("type"))
		cadence := strings.TrimSpace(r.FormValue("cadence"))
		currency := strings.ToUpper(strings.TrimSpace(r.FormValue("currency")))
		amountStr := strings.TrimSpace(r.FormValue("expected_amount"))
		dayStr := strings.TrimSpace(r.FormValue("expected_day"))
		categoryID := strings.TrimSpace(r.FormValue("category_id"))

		// Re-render the form with an error + the entered values on validation fail.
		rerender := func(msg string) {
			var categoryOptions []pages.SubscriptionCategoryOption
			if cats, cerr := svc.ListCategories(ctx); cerr == nil {
				categoryOptions = flattenCategoryOptions(cats, "")
			}
			tr.RenderWithTempl(w, r, map[string]any{
				"PageTitle":   "New recurring series",
				"CurrentPage": "recurring",
				"CSRFToken":   GetCSRFToken(r),
				"Breadcrumbs": recurringFormBreadcrumbs(),
			}, pages.RecurringSeriesForm(pages.RecurringSeriesFormProps{
				CSRFToken: GetCSRFToken(r), Error: msg,
				Name: name, Type: typ, Cadence: cadence, Currency: currency,
				ExpectedAmount: amountStr, ExpectedDay: dayStr, CategoryID: categoryID,
				Categories: categoryOptions,
			}))
		}

		if name == "" {
			rerender("Name is required.")
			return
		}
		var amount *float64
		if amountStr != "" {
			f, err := strconv.ParseFloat(amountStr, 64)
			if err != nil {
				rerender("Expected amount must be a number.")
				return
			}
			amount = &f
		}
		var day *int32
		if dayStr != "" {
			d, derr := strconv.Atoi(dayStr)
			if derr != nil || d < 1 || d > 31 {
				rerender("Expected day must be a number from 1 to 31.")
				return
			}
			d32 := int32(d)
			day = &d32
		}

		actor := ActorFromSession(sm, r)
		in := service.AssignSeriesInput{
			MerchantKey:     slugifyMerchantKey(name),
			CreateIfMissing: true,
			Confirm:         true, // a deliberate manual entry is active+confirmed, not a candidate
			FailIfExists:    true, // a deliberate create must not silently adopt/revive an existing series
			Name:            name,
			Cadence:         cadence,
			Type:            typ,
			ExpectedAmount:  amount,
			ExpectedDay:     day, // threaded into the insert, not a swallowed follow-up edit
			Currency:        strPtrIfNotEmpty(currency),
			CategoryID:      strPtrIfNotEmpty(categoryID),
		}
		resp, err := svc.AssignSeries(ctx, in, actor)
		if err != nil {
			if errors.Is(err, service.ErrConflict) {
				rerender(err.Error())
				return
			}
			rerender("Could not create the series: " + err.Error())
			return
		}

		http.Redirect(w, r, "/recurring/"+resp.ShortID, http.StatusSeeOther)
	}
}

// slugifyMerchantKey derives a stable merchant_key anchor from a user-supplied
// name (lowercase alphanumerics). Incoming charges still key off the provider
// name at sync time, so this only anchors the manual series' identity.
func slugifyMerchantKey(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "series"
	}
	return b.String()
}

func strPtrIfNotEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// subscriptionRow maps a service.SeriesResponse to the templ row shape,
// deriving the coarse confidence band, signal summary, and display labels.
func subscriptionRow(s service.SeriesResponse, catName, userName map[string]string) pages.SubscriptionRow {
	band, bandTone := subscriptionConfidenceBand(s)
	row := pages.SubscriptionRow{
		ShortID:         s.ShortID,
		Name:            s.Name,
		MerchantKey:     s.MerchantKey,
		Cadence:         s.Cadence,
		CadenceLabel:    cadenceLabel(s.Cadence),
		Status:          s.Status,
		StatusLabel:     statusLabel(s.Status),
		StatusTone:      statusTone(s.Status),
		Currency:        deref(s.IsoCurrencyCode),
		NextExpected:    formatSubDate(s.NextExpectedDate, "Jan 2"),
		LastSeen:        formatSubDate(s.LastSeenDate, "Jan 2"),
		OccurrenceCount: s.OccurrenceCount,
		ConfidenceBand:  band,
		BandTone:        bandTone,
		SignalSummary:   subscriptionSignalSummary(s),
		SignalsJSON:     prettySignals(s.DetectionSignals),
		SignalFacts:     subscriptionSignalFacts(s),
		Source:          s.DetectionSource,
		SourceLabel:     sourceLabel(s.DetectionSource),
	}
	row.Type = s.Type
	row.TypeLabel = recurringTypeLabel(s.Type)
	row.RenewalLabel, row.RenewalTone = subscriptionRenewal(s)
	row.DaysUntilRenewal = s.DaysUntilRenewal
	if sig, ok := decodeSignals(s.DetectionSignals); ok {
		row.PriceChanged = sig.AmountBranch == "monotonic_drift"
	}
	if s.LastAmount != nil {
		row.HasAmount = true
		row.Amount = math.Abs(*s.LastAmount)
		// Monthly-equivalent contribution — drives the ledger group's monthly
		// subtotal (single-currency only; never summed across currencies).
		row.MonthlyEquiv = monthlyEquivalent(s.Cadence, row.Amount)
	}
	if s.CategoryID != nil {
		row.CategoryName = catName[*s.CategoryID]
	}
	row.Tags = s.Tags
	if s.UserID != nil {
		row.UserID = *s.UserID
		row.OwnerName = userName[*s.UserID]
	}
	row.Search = strings.ToLower(strings.Join([]string{s.Name, s.MerchantKey, row.CadenceLabel, row.CategoryName, row.OwnerName, row.TypeLabel}, " "))
	return row
}

// recurringTypeLabel renders the structured type for display.
func recurringTypeLabel(t string) string {
	switch t {
	case service.SeriesTypeSubscription:
		return "Subscription"
	case service.SeriesTypeBill:
		return "Bill"
	case service.SeriesTypeLoan:
		return "Loan"
	case service.SeriesTypeOther:
		return "Other"
	default:
		return "Subscription"
	}
}

// subscriptionRenewal derives an attention chip (label + daisy tone) from a
// series' renewal health (shipped on SeriesResponse). Returns empty for
// comfortably-renewing / no-projection series — only due_soon / overdue /
// stale earn a chip, so the ledger highlights what needs a look. Health is
// only populated for active series, so candidates/paused/cancelled get nothing.
func subscriptionRenewal(s service.SeriesResponse) (string, string) {
	days := 0
	if s.DaysUntilRenewal != nil {
		days = *s.DaysUntilRenewal
	}
	switch s.RenewalHealth {
	case service.SeriesHealthDueSoon:
		switch {
		case days <= 0:
			return "Due today", "info"
		case days == 1:
			return "Due tomorrow", "info"
		default:
			return fmt.Sprintf("Renews in %dd", days), "info"
		}
	case service.SeriesHealthOverdue:
		return fmt.Sprintf("%dd overdue", -days), "warning"
	case service.SeriesHealthStale:
		// Type-aware copy: a bill/loan that goes silent has "lapsed" (you may
		// have missed a payment), whereas a subscription has likely been cancelled.
		switch s.Type {
		case service.SeriesTypeBill, service.SeriesTypeLoan:
			return "Lapsed?", "error"
		default:
			return "Likely cancelled", "error"
		}
	default:
		return "", ""
	}
}

// renewalSortGroup buckets an active-ledger row for the renewal-urgency sort:
// 0 = has a projection and isn't stale (overdue/due-soon/upcoming), 1 = no
// projection (paused/cancelled/unknown cadence), 2 = stale ("likely
// cancelled") — pushed to the bottom since it isn't really "upcoming".
func renewalSortGroup(r pages.SubscriptionRow) int {
	switch {
	case r.RenewalTone == "error": // stale / likely cancelled
		return 2
	case r.DaysUntilRenewal == nil:
		return 1
	default:
		return 0
	}
}

// renewalSortDays is the secondary key (ascending) within a group — most
// overdue first, then soonest upcoming. Missing projection sorts as 0.
func renewalSortDays(r pages.SubscriptionRow) int {
	if r.DaysUntilRenewal == nil {
		return 0
	}
	return *r.DaysUntilRenewal
}

// subscriptionSignalsShape is the subset of detection_signals (§6.6) the UI
// reads to derive the coarse band + one-line summary.
type subscriptionSignalsShape struct {
	OccurrenceCount   int     `json:"occurrence_count"`
	IntervalCV        float64 `json:"interval_cv"`
	Cadence           string  `json:"cadence"`
	AmountBranch      string  `json:"amount_branch"`
	AmountSpreadRatio float64 `json:"amount_spread_ratio"`
}

func decodeSignals(raw json.RawMessage) (subscriptionSignalsShape, bool) {
	var sig subscriptionSignalsShape
	if len(raw) == 0 {
		return sig, false
	}
	if err := json.Unmarshal(raw, &sig); err != nil {
		return sig, false
	}
	return sig, true
}

// subscriptionConfidenceBand derives High / Medium / Low + a daisy tone from
// the raw detection signals (falling back to occurrence count alone when an
// agent/user created the series without signals).
func subscriptionConfidenceBand(s service.SeriesResponse) (string, string) {
	occ := s.OccurrenceCount
	branch := ""
	if sig, ok := decodeSignals(s.DetectionSignals); ok {
		if sig.OccurrenceCount > 0 {
			occ = sig.OccurrenceCount
		}
		branch = sig.AmountBranch
	}
	switch {
	case occ <= 3 || branch == "monotonic_drift":
		return "Low", "neutral"
	case occ >= 6 && (branch == "" || branch == "tight"):
		return "High", "success"
	default:
		return "Medium", "warning"
	}
}

// subscriptionSignalSummary builds the one-line "11 charges · monthly · ±0%
// spread" summary surfaced under a candidate's name.
func subscriptionSignalSummary(s service.SeriesResponse) string {
	occ := s.OccurrenceCount
	cadence := strings.ToLower(cadenceLabel(s.Cadence))
	amountPhrase := ""
	if sig, ok := decodeSignals(s.DetectionSignals); ok {
		if sig.OccurrenceCount > 0 {
			occ = sig.OccurrenceCount
		}
		if sig.Cadence != "" {
			cadence = sig.Cadence
		}
		switch sig.AmountBranch {
		case "monotonic_drift":
			amountPhrase = "price rising"
		case "tight":
			// AmountSpreadRatio is max/min (1.0 = identical charges). Surface
			// the percentage range above the floor, or "stable" when flat.
			if pct := (sig.AmountSpreadRatio - 1) * 100; pct >= 1 {
				amountPhrase = fmt.Sprintf("±%.0f%% range", pct)
			} else {
				amountPhrase = "stable price"
			}
		}
	}
	parts := []string{fmt.Sprintf("%d charges", occ), cadence}
	if amountPhrase != "" {
		parts = append(parts, amountPhrase)
	}
	return strings.Join(parts, " · ")
}

// subscriptionSignalFacts parses detection_signals into labelled criteria for
// the "Why detected" surface — a readable breakdown (charges seen, cadence,
// timing regularity, amount pattern) instead of a raw JSON dump. Returns nil
// when there are no signals (manual / agent-created series) so the card falls
// back to a plain note.
func subscriptionSignalFacts(s service.SeriesResponse) []pages.SubscriptionSignalFact {
	sig, ok := decodeSignals(s.DetectionSignals)
	if !ok {
		return nil
	}
	occ := s.OccurrenceCount
	if sig.OccurrenceCount > 0 {
		occ = sig.OccurrenceCount
	}
	occTone := "neutral"
	switch {
	case occ >= 6:
		occTone = "success"
	case occ <= 3:
		occTone = "warning"
	}

	cadence := sig.Cadence
	if cadence == "" {
		cadence = strings.ToLower(cadenceLabel(s.Cadence))
	}

	facts := []pages.SubscriptionSignalFact{
		{Icon: "hash", Label: "Charges seen", Value: fmt.Sprintf("%d", occ), Tone: occTone},
		{Icon: "calendar", Label: "Cadence", Value: cadence, Tone: "neutral"},
	}

	// Timing regularity from the interval coefficient of variation (0 = perfectly
	// even spacing). Lower CV ⇒ stronger recurring signal.
	regularity, regTone := intervalRegularity(sig.IntervalCV)
	facts = append(facts, pages.SubscriptionSignalFact{
		Icon: "activity", Label: "Timing", Value: regularity, Tone: regTone,
	})

	// Amount pattern from the detector's amount branch.
	amountValue, amountTone := amountPattern(sig)
	facts = append(facts, pages.SubscriptionSignalFact{
		Icon: "dollar-sign", Label: "Amount", Value: amountValue, Tone: amountTone,
	})
	return facts
}

// intervalRegularity turns the interval coefficient of variation into a phrase +
// tone. CV is stddev/mean of the gaps between charges.
func intervalRegularity(cv float64) (string, string) {
	switch {
	case cv <= 0.0001:
		return "Perfectly even spacing", "success"
	case cv < 0.10:
		return fmt.Sprintf("Very regular (±%.0f%%)", cv*100), "success"
	case cv < 0.25:
		return fmt.Sprintf("Regular (±%.0f%%)", cv*100), "success"
	case cv < 0.50:
		return fmt.Sprintf("Somewhat irregular (±%.0f%%)", cv*100), "warning"
	default:
		return fmt.Sprintf("Irregular (±%.0f%%)", cv*100), "neutral"
	}
}

// amountPattern turns the detector's amount branch + spread into a phrase + tone.
func amountPattern(sig subscriptionSignalsShape) (string, string) {
	switch sig.AmountBranch {
	case "monotonic_drift":
		return "Rising over time", "warning"
	case "tight":
		if pct := (sig.AmountSpreadRatio - 1) * 100; pct >= 1 {
			return fmt.Sprintf("Stable (±%.0f%%)", pct), "success"
		}
		return "Identical each time", "success"
	case "":
		return "—", "neutral"
	default:
		return "Variable", "neutral"
	}
}

func prettySignals(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "No detection signals recorded (created manually or by an agent)."
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return string(raw)
	}
	return buf.String()
}

// monthlyEquivalent normalizes a cadence's amount to a monthly figure.
// Irregular/unknown cadences contribute nothing (we can't project them).
func monthlyEquivalent(cadence string, amount float64) float64 {
	switch cadence {
	case service.SeriesCadenceWeekly:
		return amount * 52.0 / 12.0
	case service.SeriesCadenceBiweekly:
		return amount * 26.0 / 12.0
	case service.SeriesCadenceMonthly:
		return amount
	case service.SeriesCadenceQuarterly:
		return amount / 3.0
	case service.SeriesCadenceSemiannual:
		return amount / 6.0
	case service.SeriesCadenceAnnual:
		return amount / 12.0
	default:
		return 0
	}
}

func cadenceLabel(c string) string {
	switch c {
	case service.SeriesCadenceWeekly:
		return "Weekly"
	case service.SeriesCadenceBiweekly:
		return "Every 2 weeks"
	case service.SeriesCadenceMonthly:
		return "Monthly"
	case service.SeriesCadenceQuarterly:
		return "Quarterly"
	case service.SeriesCadenceSemiannual:
		return "Every 6 months"
	case service.SeriesCadenceAnnual:
		return "Annual"
	case service.SeriesCadenceIrregular:
		return "Irregular"
	default:
		return "Unknown"
	}
}

func statusLabel(s string) string {
	switch s {
	case service.SeriesStatusActive:
		return "Active"
	case service.SeriesStatusPaused:
		return "Paused"
	case service.SeriesStatusCancelled:
		return "Cancelled"
	case service.SeriesStatusCandidate:
		return "Candidate"
	default:
		return s
	}
}

func statusTone(s string) string {
	switch s {
	case service.SeriesStatusActive:
		return "success"
	case service.SeriesStatusPaused:
		return "warning"
	case service.SeriesStatusCancelled:
		return "neutral"
	case service.SeriesStatusCandidate:
		return "info"
	default:
		return "neutral"
	}
}

func sourceLabel(s string) string {
	switch s {
	case service.SeriesSourceDeterministic:
		return "Auto-detected"
	case service.SeriesSourceAgent:
		return "Agent"
	case service.SeriesSourceUser:
		return "You"
	case service.SeriesSourceRule:
		return "Rule"
	default:
		return s
	}
}

// subscriptionMoney delegates to seriesMoney so every money string across the
// recurring surfaces formats identically ("$15.99" for USD/blank, "15.99 EUR"
// otherwise) — previously this and seriesMoney disagreed and the same series
// rendered "$15.99" in the detection panel but "15.99 USD" in the fact strip.
func subscriptionMoney(v *float64, currency string) string {
	if v == nil {
		return ""
	}
	return seriesMoney(math.Abs(*v), currency)
}

func subscriptionExpectedDay(d *int) string {
	if d == nil {
		return ""
	}
	return fmt.Sprintf("Day %d", *d)
}

// formatSubDate reparses a service "2006-01-02" date string and reformats it.
func formatSubDate(s *string, layout string) string {
	if s == nil || *s == "" {
		return ""
	}
	t, err := time.Parse("2006-01-02", *s)
	if err != nil {
		return *s
	}
	return t.Format(layout)
}

// subscriptionPriceChanges walks the members oldest→newest and records each
// point where the charge amount changed (beyond a cent).
func subscriptionPriceChanges(in []service.SeriesMember) []pages.SubscriptionPriceChange {
	// Members arrive newest-first; reverse to chronological order.
	chrono := make([]service.SeriesMember, 0, len(in))
	for i := len(in) - 1; i >= 0; i-- {
		if in[i].Amount != nil {
			chrono = append(chrono, in[i])
		}
	}
	var changes []pages.SubscriptionPriceChange
	for i := 1; i < len(chrono); i++ {
		prev := math.Abs(*chrono[i-1].Amount)
		cur := math.Abs(*chrono[i].Amount)
		if math.Abs(cur-prev) >= 0.01 {
			changes = append(changes, pages.SubscriptionPriceChange{
				Date:     formatSubDate(chrono[i].Date, "Jan 2, 2006"),
				From:     prev,
				To:       cur,
				Currency: deref(chrono[i].Currency),
			})
		}
	}
	return changes
}

// subscriptionCategoryNames builds a formatted-UUID → name lookup.
func subscriptionCategoryNames(ctx context.Context, a *app.App) map[string]string {
	cats, _ := a.Queries.ListCategories(ctx)
	m := make(map[string]string, len(cats))
	for _, c := range cats {
		m[pgconv.FormatUUID(c.ID)] = c.DisplayName
	}
	return m
}

func subscriptionUserNames(ctx context.Context, a *app.App) map[string]string {
	users, _ := a.Queries.ListUsers(ctx)
	m := make(map[string]string, len(users))
	for _, u := range users {
		m[pgconv.FormatUUID(u.ID)] = u.Name
	}
	return m
}

// subscriptionUserFilters builds the household-member filter chips from the
// users that actually own at least one series, sorted by name. Reuses the
// name map already fetched for row rendering — no extra ListUsers query.
func subscriptionUserFilters(userName map[string]string, owners map[string]bool) []pages.SubscriptionUserFilter {
	if len(owners) < 2 {
		return nil
	}
	var out []pages.SubscriptionUserFilter
	for id, name := range userName {
		if !owners[id] {
			continue
		}
		first := ""
		if name != "" {
			first = name[:1]
		}
		out = append(out, pages.SubscriptionUserFilter{ID: id, Name: name, First: first})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// subscriptionTypeFilters builds the "filter by type" options from the types
// actually present, in a stable order. Returns nil for a single type (no filter
// worth showing).
func subscriptionTypeFilters(present map[string]bool) []pages.SubscriptionTypeFilter {
	if len(present) < 2 {
		return nil
	}
	order := []struct{ value, label string }{
		{service.SeriesTypeSubscription, "Subscriptions"},
		{service.SeriesTypeBill, "Bills"},
		{service.SeriesTypeLoan, "Loans"},
		{service.SeriesTypeOther, "Other"},
	}
	var out []pages.SubscriptionTypeFilter
	for _, o := range order {
		if present[o.value] {
			out = append(out, pages.SubscriptionTypeFilter{Value: o.value, Label: o.label})
		}
	}
	return out
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefFloat(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}

func derefInt(i *int) int {
	if i == nil {
		return 0
	}
	return *i
}

// flattenCategoryOptions walks the category tree into a flat select list,
// indenting children under their parent so the hierarchy stays legible in a
// plain <select>. Values are category UUIDs (resolveOptionalCategoryID accepts
// them cleanly).
func flattenCategoryOptions(cats []service.CategoryResponse, prefix string) []pages.SubscriptionCategoryOption {
	var out []pages.SubscriptionCategoryOption
	for _, c := range cats {
		out = append(out, pages.SubscriptionCategoryOption{ID: c.ID, Name: prefix + c.DisplayName})
		if len(c.Children) > 0 {
			out = append(out, flattenCategoryOptions(c.Children, prefix+"— ")...)
		}
	}
	return out
}
