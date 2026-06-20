//go:build !headless && !lite

package admin

import (
	"errors"
	"net/http"
	"strings"

	"breadbox/internal/app"
	"breadbox/internal/service"
	"breadbox/internal/templates/components"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// Recurring-series admin UI (rules-as-substrate, P2). A recurring series is a
// THIN entity — surrogate id/short_id, a name, a type, and its tags. There is no
// shipped detector and no derived stat (cadence, amount, next-date, confidence,
// lifecycle). Membership comes from `assign_series` rules: the detail page shows
// the linked charges beside the GOVERNING RULES that define them, making the
// relationship explicit. The list is a flat ledger of every live series; the
// create form needs only a name + type.

// SubscriptionsListPageHandler serves GET /recurring — every live series as one
// row (name · type · linked-charge count). No candidate/review split: membership
// comes from rules, not a detector.
func SubscriptionsListPageHandler(a *app.App, svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		all, err := svc.ListSeries(ctx, nil)
		if err != nil {
			a.Logger.Error("list series", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		counts, err := svc.ListSeriesMemberCounts(ctx)
		if err != nil {
			a.Logger.Error("series member counts", "error", err)
			counts = map[string]int{} // degrade gracefully — rows still render
		}
		ruleByShortID, ruleByName, err := svc.ListSeriesGoverningRuleCounts(ctx)
		if err != nil {
			a.Logger.Error("series governing rule counts", "error", err)
			ruleByShortID, ruleByName = map[string]int{}, map[string]int{}
		}

		typesPresent := map[string]bool{}
		rows := make([]pages.SubscriptionRow, 0, len(all))
		for _, s := range all {
			row := subscriptionRow(s)
			row.MemberCount = counts[s.ID]
			row.GoverningRuleCount = ruleByShortID[s.ShortID] + ruleByName[s.Name]
			if row.Type != "" {
				typesPresent[row.Type] = true
			}
			rows = append(rows, row)
		}

		props := pages.SubscriptionsListProps{
			CSRFToken: GetCSRFToken(r),
			Rows:      rows,
			Types:     subscriptionTypeFilters(typesPresent),
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

// SubscriptionDetailHandler serves GET /recurring/{id} — the series' name, type,
// tags, linked charges, and the governing rules that define its membership.
func SubscriptionDetailHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		idStr := chi.URLParam(r, "id")

		s, err := svc.GetSeries(ctx, idStr)
		if err != nil {
			tr.RenderNotFound(w, r)
			return
		}
		row := subscriptionRow(*s)

		members, err := svc.SeriesMembers(ctx, idStr)
		if err != nil {
			a.Logger.Error("series members", "error", err)
		}
		row.MemberCount = len(members)
		memberIDs := make([]string, 0, len(members))
		for _, m := range members {
			memberIDs = append(memberIDs, m.ShortID)
		}
		memberRows, mrErr := svc.GetAdminTransactionRowsByIDs(ctx, memberIDs)
		if mrErr != nil {
			a.Logger.Error("series member rows", "error", mrErr)
		}

		// Governing rules — the assign_series rules that define this series'
		// membership. This is the doctrine payoff: a series IS its rules.
		var governing []components.GoverningRule
		if rules, grErr := svc.ListGoverningRules(ctx, idStr); grErr != nil {
			a.Logger.Error("series governing rules", "error", grErr)
		} else {
			governing = make([]components.GoverningRule, 0, len(rules))
			for _, rule := range rules {
				governing = append(governing, pages.BuildGoverningRule(rule))
			}
		}

		// Tags currently on the series → chips; the rest seed the add picker.
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

		props := pages.SubscriptionDetailProps{
			CSRFToken:       GetCSRFToken(r),
			Series:          row,
			CreatedAt:       s.CreatedAt,
			MemberRows:      memberRows,
			GoverningRules:  governing,
			AvailableTags:   tagOptions,
			TagChips:        tagChips,
			AllTags:         allTags,
			CurrentTagSlugs: row.Tags,
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

// NewRecurringSeriesPageHandler renders the create-from-scratch form at
// GET /recurring/new.
func NewRecurringSeriesPageHandler(a *app.App, svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		props := pages.RecurringSeriesFormProps{
			CSRFToken: GetCSRFToken(r),
			Type:      service.SeriesTypeSubscription,
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

// recurringFormBreadcrumbs is the topbar trail for the new-series form.
func recurringFormBreadcrumbs() []components.Breadcrumb {
	return []components.Breadcrumb{
		{Label: "Recurring", Href: "/recurring"},
		{Label: "New series"},
	}
}

// CreateRecurringSeriesHandler handles POST /recurring/new — mints a series by
// name (surrogate-first) and redirects to its detail page.
func CreateRecurringSeriesHandler(a *app.App, svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(r.FormValue("name"))
		typ := strings.TrimSpace(r.FormValue("type"))

		rerender := func(msg string) {
			tr.RenderWithTempl(w, r, map[string]any{
				"PageTitle":   "New recurring series",
				"CurrentPage": "recurring",
				"CSRFToken":   GetCSRFToken(r),
				"Breadcrumbs": recurringFormBreadcrumbs(),
			}, pages.RecurringSeriesForm(pages.RecurringSeriesFormProps{
				CSRFToken: GetCSRFToken(r), Error: msg,
				Name: name, Type: typ,
			}))
		}

		if name == "" {
			rerender("Name is required.")
			return
		}

		actor := ActorFromSession(sm, r)
		resp, err := svc.AssignSeries(ctx, service.AssignSeriesInput{
			Name:            name,
			Type:            typ,
			CreateIfMissing: true,
			FailIfExists:    true, // a deliberate create must not silently resolve an existing series
		}, actor)
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

// subscriptionRow maps a thin service.SeriesResponse to the templ row shape.
// MemberCount is filled by the caller (it's a separate query).
func subscriptionRow(s service.SeriesResponse) pages.SubscriptionRow {
	label := recurringTypeLabel(s.Type)
	return pages.SubscriptionRow{
		ShortID:   s.ShortID,
		Name:      s.Name,
		Type:      s.Type,
		TypeLabel: label,
		Tags:      s.Tags,
		Search:    strings.ToLower(strings.Join([]string{s.Name, label}, " ")),
	}
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

// subscriptionTypeFilters builds the "filter by type" options from the types
// actually present, in a stable order. Returns nil for a single type.
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
