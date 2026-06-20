//go:build !headless && !lite

package admin

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"breadbox/internal/app"
	"breadbox/internal/service"
	"breadbox/internal/templates/components"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// NOTE (rules-as-substrate, P2-PR2): a recurring series is now a THIN entity —
// surrogate id/short_id, a name, a type, and its tags. The shipped detector and
// every derived stat (cadence, expected amount, next-date, confidence, lifecycle
// status) are gone. This file is at a MINIMAL COMPILING state: it lists series,
// shows a detail page (members + tags), and creates a series by name. The full
// detection-free UI rebuild — linked-charges + governing-rules panels — lands in
// P2-PR3, which also reshapes the templ prop structs. Detector-era prop fields
// are left zero-valued for now.

// SubscriptionsListPageHandler serves GET /subscriptions — the recurring-series
// ledger. Every live series renders as one row; there is no candidate/review
// split anymore (membership comes from rules, not a detector).
func SubscriptionsListPageHandler(a *app.App, svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		all, err := svc.ListSeries(ctx, nil)
		if err != nil {
			a.Logger.Error("list series", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		typesPresent := map[string]bool{}
		active := make([]pages.SubscriptionRow, 0, len(all))
		for _, s := range all {
			row := subscriptionRow(s)
			if row.Type != "" {
				typesPresent[row.Type] = true
			}
			active = append(active, row)
		}

		props := pages.SubscriptionsListProps{
			CSRFToken:   GetCSRFToken(r),
			ActiveTab:   "active",
			ActiveCount: len(active),
			Active:      active,
			Types:       subscriptionTypeFilters(typesPresent),
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

// SubscriptionDetailHandler serves GET /subscriptions/{id} — the series' name,
// type, tags, and linked charges.
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
		memberIDs := make([]string, 0, len(members))
		for _, m := range members {
			memberIDs = append(memberIDs, m.ShortID)
		}
		memberRows, mrErr := svc.GetAdminTransactionRowsByIDs(ctx, memberIDs)
		if mrErr != nil {
			a.Logger.Error("series member rows", "error", mrErr)
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

		catTree, _ := svc.ListCategories(ctx)

		props := pages.SubscriptionDetailProps{
			CSRFToken:       GetCSRFToken(r),
			Series:          row,
			CreatedAt:       formatSubDate(strPtr(s.CreatedAt), "Jan 2, 2006"),
			Categories:      flattenCategoryOptions(catTree, ""),
			MemberRows:      memberRows,
			AvailableTags:   tagOptions,
			TagChips:        tagChips,
			CategoryTree:    catTree,
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
		ctx := r.Context()
		var categoryOptions []pages.SubscriptionCategoryOption
		if cats, cerr := svc.ListCategories(ctx); cerr == nil {
			categoryOptions = flattenCategoryOptions(cats, "")
		}
		props := pages.RecurringSeriesFormProps{
			CSRFToken:  GetCSRFToken(r),
			Type:       service.SeriesTypeSubscription,
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
				Name: name, Type: typ,
				Categories: categoryOptions,
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
// Detector-era fields (cadence, amount, confidence, renewal, signals) are left
// at their zero values until the P2-PR3 UI rebuild reshapes the prop struct. The
// status is fixed to "active" so the ledger groups every series under one header.
func subscriptionRow(s service.SeriesResponse) pages.SubscriptionRow {
	return pages.SubscriptionRow{
		ShortID:     s.ShortID,
		Name:        s.Name,
		Status:      "active",
		StatusLabel: "Active",
		StatusTone:  "success",
		Type:        s.Type,
		TypeLabel:   recurringTypeLabel(s.Type),
		Tags:        s.Tags,
		Search:      strings.ToLower(strings.Join([]string{s.Name, recurringTypeLabel(s.Type)}, " ")),
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

// formatSubDate reparses a service date/timestamp string and reformats it.
// Accepts "2006-01-02" and RFC3339; returns the raw string on parse failure.
func formatSubDate(s *string, layout string) string {
	if s == nil || *s == "" {
		return ""
	}
	if t, err := time.Parse("2006-01-02", *s); err == nil {
		return t.Format(layout)
	}
	if t, err := time.Parse(time.RFC3339, *s); err == nil {
		return t.Format(layout)
	}
	return *s
}

func strPtr(s string) *string {
	return &s
}

// flattenCategoryOptions walks the category tree into a flat select list,
// indenting children under their parent so the hierarchy stays legible.
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
