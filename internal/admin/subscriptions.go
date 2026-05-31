//go:build !headless && !lite

package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"breadbox/internal/app"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"
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
			if s.Status == service.SeriesStatusCandidate {
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

		props := pages.SubscriptionsListProps{
			CSRFToken:      GetCSRFToken(r),
			ActiveCount:    activeCount,
			CandidateCount: len(candidates),
			MonthlyTotals:  monthlyTotals,
			Candidates:     candidates,
			Active:         active,
			Users:          subscriptionUserFilters(ctx, a, usersWithSeries),
		}

		data := map[string]any{
			"PageTitle":   "Subscriptions",
			"CurrentPage": "subscriptions",
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

		props := pages.SubscriptionDetailProps{
			CSRFToken:       GetCSRFToken(r),
			Series:          row,
			ExpectedAmount:  subscriptionMoney(s.ExpectedAmount, deref(s.IsoCurrencyCode)),
			AmountTolerance: subscriptionMoney(s.AmountTolerance, deref(s.IsoCurrencyCode)),
			ExpectedDay:     subscriptionExpectedDay(s.ExpectedDay),
			NextExpected:    formatSubDate(s.NextExpectedDate, "Jan 2, 2006"),
			LastSeen:        formatSubDate(s.LastSeenDate, "Jan 2, 2006"),
			Confidence:      s.Confidence,
			Members:         subscriptionMembers(members),
			PriceChanges:    subscriptionPriceChanges(members),
		}

		data := map[string]any{
			"PageTitle":   s.Name + " — Subscription",
			"CurrentPage": "subscriptions",
			"CSRFToken":   GetCSRFToken(r),
			"Flash":       GetFlash(ctx, sm),
		}
		tr.RenderWithTempl(w, r, data, pages.SubscriptionDetail(props))
	}
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
		Source:          s.DetectionSource,
		SourceLabel:     sourceLabel(s.DetectionSource),
	}
	row.RenewalLabel, row.RenewalTone = subscriptionRenewal(s)
	row.DaysUntilRenewal = s.DaysUntilRenewal
	if s.LastAmount != nil {
		row.HasAmount = true
		row.Amount = math.Abs(*s.LastAmount)
	}
	if s.CategoryID != nil {
		row.CategoryName = catName[*s.CategoryID]
	}
	row.Tags = s.Tags
	if s.UserID != nil {
		row.UserID = *s.UserID
		row.OwnerName = userName[*s.UserID]
	}
	row.Search = strings.ToLower(strings.Join([]string{s.Name, s.MerchantKey, row.CadenceLabel, row.CategoryName, row.OwnerName}, " "))
	return row
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
		return "Likely cancelled", "error"
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

func subscriptionMoney(v *float64, currency string) string {
	if v == nil {
		return ""
	}
	if currency == "" {
		return fmt.Sprintf("%.2f", math.Abs(*v))
	}
	return fmt.Sprintf("%.2f %s", math.Abs(*v), currency)
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

func subscriptionMembers(in []service.SeriesMember) []pages.SubscriptionMember {
	out := make([]pages.SubscriptionMember, 0, len(in))
	for _, m := range in {
		name := m.Name
		if m.MerchantName != nil && *m.MerchantName != "" {
			name = *m.MerchantName
		}
		row := pages.SubscriptionMember{
			ShortID:  m.ShortID,
			Date:     formatSubDate(m.Date, "Jan 2, 2006"),
			Name:     name,
			Currency: deref(m.Currency),
		}
		if m.Amount != nil {
			row.HasAmount = true
			row.Amount = math.Abs(*m.Amount)
		}
		out = append(out, row)
	}
	return out
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
// users that actually own at least one series, sorted by name.
func subscriptionUserFilters(ctx context.Context, a *app.App, owners map[string]bool) []pages.SubscriptionUserFilter {
	if len(owners) < 2 {
		return nil
	}
	users, _ := a.Queries.ListUsers(ctx)
	var out []pages.SubscriptionUserFilter
	for _, u := range users {
		id := pgconv.FormatUUID(u.ID)
		if !owners[id] {
			continue
		}
		first := ""
		if u.Name != "" {
			first = u.Name[:1]
		}
		out = append(out, pages.SubscriptionUserFilter{ID: id, Name: u.Name, First: first})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
