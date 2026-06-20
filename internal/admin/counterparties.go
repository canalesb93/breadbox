//go:build !headless && !lite

package admin

import (
	"context"
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

// Counterparties admin UI (rules-as-substrate, P4). A counterparty is the
// canonical, cross-provider "other side" of a charge — merchants AND
// non-merchants. It is a THIN, rule-maintained entity: an identity + name +
// optional enrichment (website/logo/category/mcc). Membership comes from
// `assign_counterparty` rules: the detail page shows the linked charges beside
// the GOVERNING RULES that define them, plus a manual enrichment form. The list
// is a flat directory; the create form needs only a name.

// CounterpartiesListPageHandler serves GET /counterparties — every live
// counterparty as one row (name · logo · linked-charge count · governing-rule
// count). No candidate/review split: membership comes from rules.
func CounterpartiesListPageHandler(a *app.App, svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		all, err := svc.ListCounterparties(ctx)
		if err != nil {
			a.Logger.Error("list counterparties", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		counts, err := svc.ListCounterpartyMemberCounts(ctx)
		if err != nil {
			a.Logger.Error("counterparty member counts", "error", err)
			counts = map[string]int{} // degrade gracefully — rows still render
		}
		ruleByShortID, ruleByName, err := svc.ListCounterpartyGoverningRuleCounts(ctx)
		if err != nil {
			a.Logger.Error("counterparty governing rule counts", "error", err)
			ruleByShortID, ruleByName = map[string]int{}, map[string]int{}
		}

		rows := make([]pages.CounterpartyRow, 0, len(all))
		for _, c := range all {
			row := counterpartyRow(c)
			row.MemberCount = counts[c.ID]
			row.GoverningRuleCount = ruleByShortID[c.ShortID] + ruleByName[c.Name]
			rows = append(rows, row)
		}

		props := pages.CounterpartiesListProps{
			CSRFToken: GetCSRFToken(r),
			Rows:      rows,
		}

		data := map[string]any{
			"PageTitle":   "Counterparties",
			"CurrentPage": "counterparties",
			"CSRFToken":   GetCSRFToken(r),
			"Flash":       GetFlash(ctx, sm),
		}
		tr.RenderWithTempl(w, r, data, pages.CounterpartiesList(props))
	}
}

// CounterpartyDetailHandler serves GET /counterparties/{id} — the counterparty's
// enrichment form, its linked charges, and the governing rules that define its
// membership.
func CounterpartyDetailHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer, svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		idStr := chi.URLParam(r, "id")

		c, err := svc.GetCounterparty(ctx, idStr)
		if err != nil {
			tr.RenderNotFound(w, r)
			return
		}
		row := counterpartyRow(*c)

		memberIDs, err := svc.CounterpartyMembers(ctx, idStr)
		if err != nil {
			a.Logger.Error("counterparty members", "error", err)
		}
		row.MemberCount = len(memberIDs)
		memberRows, mrErr := svc.GetAdminTransactionRowsByIDs(ctx, memberIDs)
		if mrErr != nil {
			a.Logger.Error("counterparty member rows", "error", mrErr)
		}

		// Governing rules — the assign_counterparty rules that define membership.
		var governing []components.GoverningRule
		if rules, grErr := svc.ListCounterpartyGoverningRules(ctx, idStr); grErr != nil {
			a.Logger.Error("counterparty governing rules", "error", grErr)
		} else {
			governing = make([]components.GoverningRule, 0, len(rules))
			for _, rule := range rules {
				governing = append(governing, pages.BuildGoverningRule(rule))
			}
		}
		row.GoverningRuleCount = len(governing)

		props := pages.CounterpartyDetailProps{
			CSRFToken:       GetCSRFToken(r),
			Counterparty:    row,
			CreatedAt:       c.CreatedAt,
			WebsiteURL:      derefStr(c.WebsiteURL),
			LogoURL:         derefStr(c.LogoURL),
			MCC:             derefStr(c.MCC),
			MemberRows:      memberRows,
			GoverningRules:  governing,
			CategoryOptions: counterpartyCategoryOptions(ctx, svc, c.CategoryID),
		}
		if c.CategoryID != nil {
			for _, o := range props.CategoryOptions {
				if o.Selected {
					props.CategoryID = o.Value
					break
				}
			}
		}

		data := map[string]any{
			"PageTitle":   c.Name + " — Counterparties",
			"CurrentPage": "counterparties",
			"CSRFToken":   GetCSRFToken(r),
			"Flash":       GetFlash(ctx, sm),
			"Breadcrumbs": []components.Breadcrumb{
				{Label: "Counterparties", Href: "/counterparties"},
				{Label: c.Name},
			},
		}
		tr.RenderWithTempl(w, r, data, pages.CounterpartyDetail(props))
	}
}

// UpdateCounterpartyPageHandler handles POST /counterparties/{id} — the manual
// enrichment form. Applies name/category/mcc/website/logo via UpdateCounterparty
// and redirects back to the detail page with a flash.
func UpdateCounterpartyPageHandler(a *app.App, svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		idStr := chi.URLParam(r, "id")
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		name := strings.TrimSpace(r.FormValue("name"))
		edit := service.EditCounterpartyInput{
			WebsiteURL: formPtr(r, "website_url"),
			LogoURL:    formPtr(r, "logo_url"),
			MCC:        formPtr(r, "mcc"),
		}
		if name != "" {
			edit.Name = &name
		}
		if cat := strings.TrimSpace(r.FormValue("category_id")); cat != "" {
			edit.CategoryID = &cat
		}

		actor := ActorFromSession(sm, r)
		if _, err := svc.UpdateCounterparty(ctx, idStr, edit, actor); err != nil {
			SetFlash(ctx, sm, "error", "Could not save: "+err.Error())
		} else {
			SetFlash(ctx, sm, "success", "Counterparty updated.")
		}
		http.Redirect(w, r, "/counterparties/"+idStr, http.StatusSeeOther)
	}
}

// NewCounterpartyPageHandler renders the create-from-scratch form at
// GET /counterparties/new.
func NewCounterpartyPageHandler(a *app.App, svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := map[string]any{
			"PageTitle":   "New counterparty",
			"CurrentPage": "counterparties",
			"CSRFToken":   GetCSRFToken(r),
			"Breadcrumbs": counterpartyFormBreadcrumbs(),
		}
		tr.RenderWithTempl(w, r, data, pages.CounterpartyForm(pages.CounterpartyFormProps{
			CSRFToken: GetCSRFToken(r),
		}))
	}
}

// CreateCounterpartyPageHandler handles POST /counterparties/new — creates a
// counterparty by name (strict create) and redirects to its detail page.
func CreateCounterpartyPageHandler(a *app.App, svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(r.FormValue("name"))

		rerender := func(msg string) {
			tr.RenderWithTempl(w, r, map[string]any{
				"PageTitle":   "New counterparty",
				"CurrentPage": "counterparties",
				"CSRFToken":   GetCSRFToken(r),
				"Breadcrumbs": counterpartyFormBreadcrumbs(),
			}, pages.CounterpartyForm(pages.CounterpartyFormProps{
				CSRFToken: GetCSRFToken(r), Error: msg, Name: name,
			}))
		}

		if name == "" {
			rerender("Name is required.")
			return
		}

		actor := ActorFromSession(sm, r)
		resp, err := svc.AssignCounterparty(ctx, service.AssignCounterpartyInput{
			Name:            name,
			CreateIfMissing: true,
			FailIfExists:    true, // a deliberate create must not silently resolve an existing one
		}, actor)
		if err != nil {
			if errors.Is(err, service.ErrConflict) {
				rerender(err.Error())
				return
			}
			rerender("Could not create the counterparty: " + err.Error())
			return
		}

		http.Redirect(w, r, "/counterparties/"+resp.ShortID, http.StatusSeeOther)
	}
}

// counterpartyFormBreadcrumbs is the topbar trail for the new-counterparty form.
func counterpartyFormBreadcrumbs() []components.Breadcrumb {
	return []components.Breadcrumb{
		{Label: "Counterparties", Href: "/counterparties"},
		{Label: "New counterparty"},
	}
}

// counterpartyRow maps a service.CounterpartyResponse to the templ row shape.
// MemberCount / GoverningRuleCount are filled by the caller.
func counterpartyRow(c service.CounterpartyResponse) pages.CounterpartyRow {
	return pages.CounterpartyRow{
		ShortID: c.ShortID,
		Name:    c.Name,
		LogoURL: derefStr(c.LogoURL),
		Search:  strings.ToLower(c.Name),
	}
}

// counterpartyCategoryOptions flattens the category tree into "Parent / Child"
// options, each carrying the category short_id (which resolveCategoryID accepts)
// and a Selected flag matching the counterparty's current category (by UUID).
func counterpartyCategoryOptions(ctx context.Context, svc *service.Service, currentCategoryUUID *string) []pages.CounterpartyCategoryOption {
	cats, err := svc.ListCategories(ctx)
	if err != nil {
		return nil
	}
	var out []pages.CounterpartyCategoryOption
	add := func(c service.CategoryResponse, label string) {
		out = append(out, pages.CounterpartyCategoryOption{
			Value:    c.ShortID,
			Label:    label,
			Selected: currentCategoryUUID != nil && c.ID == *currentCategoryUUID,
		})
	}
	for _, parent := range cats {
		add(parent, parent.DisplayName)
		for _, child := range parent.Children {
			add(child, parent.DisplayName+" / "+child.DisplayName)
		}
	}
	return out
}

// derefStr returns the pointed-to string or "".
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// formPtr returns a pointer to the trimmed form value, or nil when the field is
// absent. An explicitly-cleared field (present but empty) returns a pointer to ""
// so the enrichment form can blank a value.
func formPtr(r *http.Request, key string) *string {
	if _, ok := r.Form[key]; !ok {
		return nil
	}
	v := strings.TrimSpace(r.FormValue(key))
	return &v
}
