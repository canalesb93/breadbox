package admin

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path"
	"strings"
	"sync"

	"breadbox/internal/service"
	"breadbox/internal/templates"
	"breadbox/internal/templates/components"
	"breadbox/internal/templates/components/pages"
	"breadbox/internal/version"

	"github.com/a-h/templ"
	"github.com/alexedwards/scs/v2"
)

// componentAdapter converts untyped bridge data to a typed templ.Component.
// Each adapter is responsible for type-asserting data and returning an error
// if the type is wrong — the bridge logs these and emits an HTML comment.
type componentAdapter func(data any) (templ.Component, error)

// componentRegistry maps the bridge name used in html/template partials
// (e.g. {{renderComponent "TxRow" .}}) to its typed adapter. Add an entry
// here when porting a new partial to templ.
var componentRegistry = map[string]componentAdapter{
	"TxRow": func(data any) (templ.Component, error) {
		tx, err := assertAdminTxRow(data)
		if err != nil {
			return nil, err
		}
		return components.TxRow(tx), nil
	},
	"Flash": func(data any) (templ.Component, error) {
		f, ok := data.(*Flash)
		if !ok {
			if v, ok2 := data.(Flash); ok2 {
				f = &v
			} else {
				return nil, fmt.Errorf("want *admin.Flash, got %T", data)
			}
		}
		if f == nil {
			return templ.NopComponent, nil
		}
		return components.Flash(f.Type, f.Message), nil
	},
	"Breadcrumb": func(data any) (templ.Component, error) {
		crumbs, ok := data.([]Breadcrumb)
		if !ok {
			return nil, fmt.Errorf("want []admin.Breadcrumb, got %T", data)
		}
		if len(crumbs) == 0 {
			return templ.NopComponent, nil
		}
		items := make([]components.Breadcrumb, len(crumbs))
		for i, b := range crumbs {
			items[i] = components.Breadcrumb{Label: b.Label, Href: b.Href}
		}
		return components.BreadcrumbNav(items), nil
	},
	"CategoryPickerScript": func(_ any) (templ.Component, error) {
		return components.CategoryPickerScript(), nil
	},
	"CategoryPickerStyles": func(_ any) (templ.Component, error) {
		return components.CategoryPickerStyles(), nil
	},
	"Nav": func(data any) (templ.Component, error) {
		m, ok := data.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("want map[string]any, got %T", data)
		}
		return components.Nav(navPropsFromData(m)), nil
	},
	"Kbd": func(data any) (templ.Component, error) {
		key, ok := data.(string)
		if !ok {
			return nil, fmt.Errorf("Kbd: want string, got %T", data)
		}
		return components.Kbd(key), nil
	},
	"KbdCombo": func(data any) (templ.Component, error) {
		keys, err := toStringSlice(data)
		if err != nil {
			return nil, fmt.Errorf("KbdCombo: %w", err)
		}
		return components.KbdCombo(keys...), nil
	},
}

// assertAdminTxRow extracts a service.AdminTransactionRow from data,
// accepting both value and pointer forms.
func assertAdminTxRow(data any) (service.AdminTransactionRow, error) {
	if tx, ok := data.(service.AdminTransactionRow); ok {
		return tx, nil
	}
	if p, ok := data.(*service.AdminTransactionRow); ok && p != nil {
		return *p, nil
	}
	return service.AdminTransactionRow{}, fmt.Errorf("want service.AdminTransactionRow, got %T", data)
}

// toStringSlice coerces the value passed via renderComponent into a
// []string. Accepts []string directly (from Go call sites) and []any
// (produced by the template-side `strs` funcmap). Keeps component
// adapters tolerant of both paths without caring which one fed them.
func toStringSlice(v any) ([]string, error) {
	switch s := v.(type) {
	case []string:
		return s, nil
	case []any:
		out := make([]string, 0, len(s))
		for i, item := range s {
			str, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("element %d: want string, got %T", i, item)
			}
			out = append(out, str)
		}
		return out, nil
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("want []string or []any, got %T", v)
	}
}

// renderTemplComponent renders a named templ component to template.HTML so
// html/template partials can forward to Go-generated components during the
// incremental #462 migration. Errors are logged and emitted as HTML comments
// so the page degrades gracefully rather than panicking.
func renderTemplComponent(name string, data any) template.HTML {
	adapter, ok := componentRegistry[name]
	if !ok {
		log.Printf("renderTemplComponent: unknown component %q", name)
		return template.HTML(fmt.Sprintf("<!-- renderComponent(%q): unknown -->", name))
	}
	c, err := adapter(data)
	if err != nil {
		log.Printf("renderTemplComponent: %q adapter error: %v", name, err)
		return template.HTML(fmt.Sprintf("<!-- renderComponent(%q): %s -->", name, template.HTMLEscapeString(err.Error())))
	}
	var buf bytes.Buffer
	if err := c.Render(context.Background(), &buf); err != nil {
		log.Printf("renderTemplComponent: %q render error: %v", name, err)
		return template.HTML(fmt.Sprintf("<!-- renderComponent(%q) render error: %s -->", name, template.HTMLEscapeString(err.Error())))
	}
	return template.HTML(buf.String())
}

// buildNavProps assembles a NavProps from the already-injected layout data
// map. Called once in Render() so navPropsFromData doesn't have to re-extract
// each key from scratch.
func buildNavProps(m map[string]any) components.NavProps {
	return navPropsFromData(m)
}

// navPropsFromData copies the sidebar fields out of the render-time data
// map into a typed struct the nav templ component consumes. Centralising
// the mapping here keeps the component decoupled from html/template
// conventions (string keys, untyped values).
//
// At runtime this always hits the fast path: Render() caches a _NavProps
// entry before the template executes, so the type assertion succeeds and
// the string-key extraction below is never reached.
func navPropsFromData(m map[string]any) components.NavProps {
	if p, ok := m["_NavProps"].(components.NavProps); ok {
		return p
	}
	// Fallback: extract fields from the map (used when called before
	// _NavProps is cached, e.g. during initial Render injection).
	str := func(key string) string {
		s, _ := m[key].(string)
		return s
	}
	boolv := func(key string) bool {
		b, _ := m[key].(bool)
		return b
	}
	p := components.NavProps{
		CurrentPage:          str("CurrentPage"),
		IsAdmin:              boolv("IsAdmin"),
		IsEditor:             boolv("IsEditor"),
		HasLinkedUser:        boolv("HasLinkedUser"),
		SessionUserID:        str("SessionUserID"),
		SessionAvatarVersion: str("SessionAvatarVersion"),
		AdminUsername:        str("AdminUsername"),
		UserName:             str("SessionUserName"),
		RoleDisplay:          str("RoleDisplay"),
		CSRFToken:            str("CSRFToken"),
		AppVersion:           str("AppVersion"),
		NavUpdateAvailable:   boolv("NavUpdateAvailable"),
		NavLatestVersion:     str("NavLatestVersion"),
		NavLatestURL:         str("NavLatestURL"),
	}
	if badges, ok := m["NavBadges"].(NavBadges); ok {
		p.ShowGettingStarted = badges.ShowGettingStarted
		p.UnreadReports = badges.UnreadReports
		p.ConnectionsAttention = badges.ConnectionsAttention
		p.PendingReviews = badges.PendingReviews
	}
	return p
}

// Flash represents a one-time message shown to the user after a redirect.
type Flash struct {
	Type    string // "success", "error", "info"
	Message string
}

// BaseData contains fields available to every template.
type BaseData struct {
	PageTitle   string
	CurrentPage string // "dashboard", "connections", etc. — for nav active state
	Flash       *Flash
	CSRFToken   string
}

// WizardData extends BaseData for setup wizard pages.
type WizardData struct {
	BaseData
	StepNumber int
}

// Breadcrumb represents one item in a navigation breadcrumb trail.
// If Href is empty, it's rendered as the current page (no link).
type Breadcrumb struct {
	Label string
	Href  string
}

// TemplateRenderer parses and renders HTML templates.
type TemplateRenderer struct {
	mu             sync.RWMutex
	templates      map[string]*template.Template
	specs          map[string][]string // name → file list, used to re-parse in dev mode
	funcMap        template.FuncMap
	sm             *scs.SessionManager
	version        string
	versionChecker *version.Checker
}

// NewTemplateRenderer parses all embedded templates and returns a renderer.
// sm is used to auto-inject the admin username into template data.
func NewTemplateRenderer(sm *scs.SessionManager) (*TemplateRenderer, error) {
	tr := &TemplateRenderer{
		templates: make(map[string]*template.Template),
		specs:     make(map[string][]string),
		sm:        sm,
		// The funcMap is intentionally tiny: the html/template surface has
		// shrunk to layout/base.html plus a handful of partials that bridge
		// to templ via renderComponent. Helpers used only inside templ
		// pages live in internal/templates/components — not here.
		// See #462 for the migration history.
		funcMap: template.FuncMap{
			// strs collects variadic string args into a []string so
			// templates can pass slices through `renderComponent`
			// (e.g. `{{renderComponent "KbdCombo" (strs "cmd" "k")}}`).
			"strs":            func(vals ...string) []string { return vals },
			"renderComponent": renderTemplComponent,
		},
	}
	if err := tr.parseTemplates(); err != nil {
		return nil, err
	}
	return tr, nil
}

var templatePartials = []string{
	"partials/flash.html",
	"partials/nav.html",
	"partials/category_picker.html",
	"partials/breadcrumb.html",
	"partials/tx_row.html",
}

func (tr *TemplateRenderer) parseTemplates() error {
	// Pages using the base layout (authenticated dashboard pages).
	basePages := []string{
		"pages/_templ_shell.html",
		// dashboard.html, settings.html, 404.html, and 500.html removed —
		// those pages render via RenderWithTempl which uses the
		// _templ_shell template key.
		// pages/connections.html removed — renders via RenderWithTempl
		// using the _templ_shell template key (see pages.Connections).
		// pages/connection_new.html removed — renders via RenderWithTempl
		// using the _templ_shell template key (see pages.ConnectionNew).
		// pages/connection_detail.html removed — renders via RenderWithTempl
		// using the _templ_shell template key (see pages.ConnectionDetail).
		// pages/connection_reauth.html removed — renders via RenderWithTempl
		// using the _templ_shell template key (see pages.ConnectionReauth).
		// pages/users.html removed — renders via RenderWithTempl using
		// the _templ_shell template key (see pages.Users).
		// pages/user_form.html removed — renders via RenderWithTempl using
		// the _templ_shell template key (see pages.UserForm).
		// pages/access.html removed — renders via RenderWithTempl using
		// the _templ_shell template key (see pages.Access).
		// pages/api_keys.html removed — was dead (no handler rendered it after
		// the API keys list moved into pages/access.html in PR #808).
		// pages/api_key_new.html and pages/api_key_created.html removed —
		// both render via RenderWithTempl using the _templ_shell template
		// key (see pages.APIKeyNew and pages.APIKeyCreated).
		// pages/providers.html removed — renders via RenderWithTempl using
		// the _templ_shell template key (see pages.Providers).
		// pages/csv_import.html removed — renders via RenderWithTempl using
		// the _templ_shell template key (see pages.CSVImport).
		// pages/transactions.html removed — renders via RenderWithTempl
		// using the _templ_shell template key (see pages.Transactions).
		// pages/account_detail.html removed — renders via RenderWithTempl
		// using the _templ_shell template key (see pages.AccountDetail).
		// pages/categories.html and pages/category_form.html removed —
		// both render via RenderWithTempl using the _templ_shell template
		// key (see pages.Categories and pages.CategoryForm).
		// pages/transaction_detail.html removed — renders via RenderWithTempl
		// using the _templ_shell template key (see pages.TransactionDetail).
		// pages/rules.html removed — renders via RenderWithTempl using the
		// _templ_shell template key (see pages.Rules).
		// pages/tags.html and pages/tag_form.html removed — both render
		// via RenderWithTempl using the _templ_shell template key
		// (see pages.Tags and pages.TagForm).
		// pages/rule_form.html removed — renders via RenderWithTempl using
		// the _templ_shell template key (see pages.RuleForm).
		// pages/rule_detail.html removed — renders via RenderWithTempl using
		// the _templ_shell template key (see pages.RuleDetail).
		// pages/reports.html removed — renders via RenderWithTempl using
		// the _templ_shell template key (see pages.Reports).
		// pages/logs.html removed — renders via RenderWithTempl using the
		// _templ_shell template key (see pages.Logs).
		// pages/oauth_clients.html removed — was dead (consolidated into pages/access.html).
		// pages/oauth_client_new.html and pages/oauth_client_created.html removed —
		// both render via RenderWithTempl using the _templ_shell template
		// key (see pages.OAuthClientNew and pages.OAuthClientCreated).
		// pages/agents.html, pages/mcp_guide.html, pages/agent_wizard.html, and
		// pages/mcp_settings.html removed — agent prompts now render via
		// RenderWithTempl using pages.AgentWizard. MCP Settings moved to the
		// unified Settings shell at /settings/mcp.
		// pages/prompt_builder.html removed — renders via RenderWithTempl using
		// the _templ_shell template key (see pages.PromptBuilder).
		// pages/session_detail.html removed — renders via RenderWithTempl using
		// the _templ_shell template key (see pages.SessionDetail).
		// pages/getting_started.html removed — renders via RenderWithTempl
		// using the _templ_shell template key (see pages.GettingStarted).
	}

	for _, page := range basePages {
		if err := tr.parseBasePage(page); err != nil {
			return err
		}
	}

	return nil
}

func (tr *TemplateRenderer) parseBasePage(pagePath string) error {
	files := []string{"layout/base.html"}
	files = append(files, templatePartials...)
	files = append(files, pagePath)

	t, err := template.New("").Funcs(tr.funcMap).ParseFS(templates.FS, files...)
	if err != nil {
		return fmt.Errorf("parse base page %s: %w", pagePath, err)
	}
	name := path.Base(pagePath)
	tr.templates[name] = t
	tr.specs[name] = files
	return nil
}

// Render writes the named template to the response writer with the given data.
// name is the base filename (e.g. "login.html", "dashboard.html").
// The request parameter is available for future middleware integration but
// is not currently used by the renderer itself.
func (tr *TemplateRenderer) Render(w http.ResponseWriter, r *http.Request, name string, data interface{}) {
	tr.mu.RLock()
	t, ok := tr.templates[name]
	files := tr.specs[name]
	tr.mu.RUnlock()
	if !ok {
		http.Error(w, "template not found: "+name, http.StatusInternalServerError)
		return
	}

	// Dev-reload: re-parse the template from disk on every render so template
	// edits apply without rebuilding the binary. Requires BREADBOX_DEV_RELOAD=1
	// and templates.FS pointing at the source directory (see internal/templates/embed.go).
	if templates.DevReload && len(files) > 0 {
		fresh, err := template.New("").Funcs(tr.funcMap).ParseFS(templates.FS, files...)
		if err != nil {
			http.Error(w, "dev-reload parse failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		t = fresh
	}

	// Auto-inject common fields into map data if not already present.
	if m, ok := data.(map[string]any); ok {
		if _, exists := m["AdminUsername"]; !exists && tr.sm != nil {
			m["AdminUsername"] = tr.sm.GetString(r.Context(), sessionKeyAccountUsername)
		}
		if _, exists := m["AppVersion"]; !exists && tr.version != "" {
			m["AppVersion"] = tr.version
		}
		// Auto-inject version update status for nav footer.
		if _, exists := m["NavUpdateAvailable"]; !exists && tr.versionChecker != nil {
			updateAvailable, latest, err := tr.versionChecker.CheckForUpdate(r.Context())
			if err == nil && updateAvailable != nil && *updateAvailable && latest != nil {
				m["NavUpdateAvailable"] = true
				m["NavLatestVersion"] = latest.Version
				m["NavLatestURL"] = latest.URL
			}
		}
		// Auto-inject sidebar notification badges from middleware context.
		if _, exists := m["NavBadges"]; !exists {
			m["NavBadges"] = getNavBadges(r.Context())
		}
		// Auto-inject all nav-required session fields.
		// This is the single source of truth — handlers don't need to set these.
		if tr.sm != nil {
			if _, exists := m["SessionRole"]; !exists {
				role := tr.sm.GetString(r.Context(), sessionKeyAccountRole)
				if role == "" {
					role = RoleAdmin
				}
				m["SessionRole"] = role
				m["IsAdmin"] = role == RoleAdmin
				m["IsEditor"] = role == RoleAdmin || role == RoleEditor
				m["RoleDisplay"] = RoleDisplayName(role)
			}
			if _, exists := m["SessionUserID"]; !exists {
				uid := tr.sm.GetString(r.Context(), sessionKeyUserID)
				m["HasLinkedUser"] = uid != ""
				if uid == "" {
					uid = tr.sm.GetString(r.Context(), sessionKeyAccountID)
				}
				m["SessionUserID"] = uid
			}
			if _, exists := m["SessionAvatarVersion"]; !exists {
				m["SessionAvatarVersion"] = tr.sm.GetString(r.Context(), sessionKeyAvatarVersion)
			}
			if _, exists := m["SessionUserName"]; !exists {
				m["SessionUserName"] = tr.sm.GetString(r.Context(), sessionKeyUserName)
			}
		}
		// Cache assembled NavProps so navPropsFromData can type-assert it
		// directly rather than re-extracting each string key.
		m["_NavProps"] = buildNavProps(m)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, "template render error: "+err.Error(), http.StatusInternalServerError)
	}
}

// RenderWithTempl hosts a templ-rendered page body inside the existing
// html/template base layout (nav, drawer, cmd-K palette, progress bar,
// CSRF shim, Alpine/chart.js scripts). The component is rendered to a
// buffer and passed to the layout via the TemplContent slot added in
// layout/base.html. Pages migrated to templ call this instead of
// Render — no base.html rewrite needed. See issue #462.
//
// The template key is always _templ_shell.html — migrated pages don't need
// their own registered html/template; the body flows through TemplContent.
func (tr *TemplateRenderer) RenderWithTempl(w http.ResponseWriter, r *http.Request, data map[string]any, body templ.Component) {
	var buf bytes.Buffer
	if err := body.Render(r.Context(), &buf); err != nil {
		http.Error(w, "templ render error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	data["TemplContent"] = template.HTML(buf.String())
	tr.Render(w, r, "_templ_shell.html", data)
}

// BaseTemplateData returns the common fields needed by every template as a map.
// Handlers can add page-specific fields to the returned map before rendering.
func BaseTemplateData(r *http.Request, sm *scs.SessionManager, currentPage, pageTitle string) map[string]any {
	role := sm.GetString(r.Context(), sessionKeyAccountRole)
	if role == "" {
		role = RoleAdmin
	}
	return map[string]any{
		"PageTitle":     pageTitle,
		"CurrentPage":   currentPage,
		"Flash":         GetFlash(r.Context(), sm),
		"CSRFToken":     GetCSRFToken(r),
		"AdminUsername": sm.GetString(r.Context(), sessionKeyAccountUsername),
		"SessionRole":   role,
		"IsAdmin":       role == RoleAdmin,
		"IsEditor":      role == RoleAdmin || role == RoleEditor,
		"RoleDisplay":   RoleDisplayName(role),
	}
}

// RenderNotFound renders the styled 404 page. For authenticated sessions the
// page lives inside the admin shell (so the user can navigate away); for
// anonymous visitors it renders standalone in the wizard layout so the admin
// sidebar and 'Administrator' footer don't leak to logged-out users.
func (tr *TemplateRenderer) RenderNotFound(w http.ResponseWriter, r *http.Request) {
	if !IsViewer(tr.sm, r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		if err := pages.NotFoundPublic().Render(r.Context(), w); err != nil {
			log.Printf("templ render error (NotFoundPublic): %v", err)
		}
		return
	}
	w.WriteHeader(http.StatusNotFound)
	data := BaseTemplateData(r, tr.sm, "", "Page Not Found")
	tr.RenderWithTempl(w, r, data, pages.NotFound())
}

// RenderError renders the styled 500 page. Matches RenderNotFound's branching:
// anonymous visitors get a standalone wizard-layout page; authenticated users
// get the full admin shell.
func (tr *TemplateRenderer) RenderError(w http.ResponseWriter, r *http.Request) {
	if !IsViewer(tr.sm, r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		if err := pages.InternalErrorPublic().Render(r.Context(), w); err != nil {
			log.Printf("templ render error (InternalErrorPublic): %v", err)
		}
		return
	}
	w.WriteHeader(http.StatusInternalServerError)
	data := BaseTemplateData(r, tr.sm, "", "Error")
	tr.RenderWithTempl(w, r, data, pages.InternalError())
}

// SetVersion sets the application version for auto-injection into template data.
func (tr *TemplateRenderer) SetVersion(v string) {
	tr.version = v
}

// SetVersionChecker sets the version checker for auto-injecting update status into template data.
func (tr *TemplateRenderer) SetVersionChecker(vc *version.Checker) {
	tr.versionChecker = vc
}

// ruleFieldLabel maps a rule-condition field name to its human-readable
// label. Falls back to title-casing the raw identifier so unknown fields
// still read reasonably.
func ruleFieldLabel(field string) string {
	switch field {
	case "provider_name":
		return "Name"
	case "provider_merchant_name":
		return "Merchant"
	case "amount":
		return "Amount"
	case "pending":
		return "Pending"
	case "category":
		return "Category"
	case "provider_category_primary":
		return "Category (primary)"
	case "provider_category_detailed":
		return "Category (detail)"
	case "tags":
		return "Tag"
	case "account_name":
		return "Account"
	case "user_name":
		return "Family member"
	case "provider":
		return "Provider"
	default:
		if field == "" {
			return "—"
		}
		return components.TitleCase(strings.ReplaceAll(field, "_", " "))
	}
}

// ruleOpLabel maps an operator code to a short display symbol/phrase. The
// field argument lets us pick type-appropriate wording (e.g. "contains" for
// strings vs "has" for tag-list fields, "=" for numerics vs "is" for bools).
func ruleOpLabel(op, field string) string {
	numericFields := map[string]bool{"amount": true}
	boolFields := map[string]bool{"pending": true}
	tagField := field == "tags"
	switch op {
	case "contains":
		if tagField {
			return "has"
		}
		return "contains"
	case "not_contains":
		if tagField {
			return "does not have"
		}
		return "does not contain"
	case "in":
		if tagField {
			return "has any of"
		}
		return "in"
	case "matches":
		return "matches /regex/"
	case "eq":
		if numericFields[field] {
			return "="
		}
		if boolFields[field] {
			return "is"
		}
		return "is"
	case "neq":
		if numericFields[field] {
			return "≠"
		}
		if boolFields[field] {
			return "is not"
		}
		return "is not"
	case "gt":
		return ">"
	case "gte":
		return "≥"
	case "lt":
		return "<"
	case "lte":
		return "≤"
	default:
		if op == "" {
			return "—"
		}
		return op
	}
}

// ruleValueFormat renders a condition's value for display. Arrays come back
// comma-separated; booleans as "true"/"false"; everything else via fmt.Sprint.
func ruleValueFormat(v any) string {
	if v == nil {
		return ""
	}
	switch vv := v.(type) {
	case []any:
		parts := make([]string, 0, len(vv))
		for _, x := range vv {
			parts = append(parts, fmt.Sprint(x))
		}
		return strings.Join(parts, ", ")
	case string:
		return vv
	case bool:
		if vv {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprint(v)
	}
}

