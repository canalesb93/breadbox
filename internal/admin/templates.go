package admin

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"math"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"breadbox/internal/service"
	bsync "breadbox/internal/sync"
	"breadbox/internal/templates"

	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5/pgtype"
)

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
	mu        sync.RWMutex
	templates map[string]*template.Template
	funcMap   template.FuncMap
	sm        *scs.SessionManager
	version   string
}

// NewTemplateRenderer parses all embedded templates and returns a renderer.
// sm is used to auto-inject the admin username into template data.
func NewTemplateRenderer(sm *scs.SessionManager) (*TemplateRenderer, error) {
	tr := &TemplateRenderer{
		templates: make(map[string]*template.Template),
		sm:        sm,
		funcMap: template.FuncMap{
			"sub": func(a, b int) int { return a - b },
			"add": func(a, b int) int { return a + b },
			"mulFloat": func(a *float64, b float64) float64 {
				if a == nil {
					return 0
				}
				return *a * b
			},
			"divFloat": func(a, b float64) float64 {
				if b == 0 {
					return 0
				}
				return a / b
			},
			"mulFloatRaw": func(a, b float64) float64 {
				return a * b
			},
			"intToFloat": func(a int) float64 {
				return float64(a)
			},
			"subf": func(a, b float64) float64 { return a - b },
			"mulf": func(a, b float64) float64 { return a * b },
			"divf": func(a, b float64) float64 {
				if b == 0 {
					return 0
				}
				return a / b
			},
			"minf": func(a, b float64) float64 {
				if a < b {
					return a
				}
				return b
			},
			"absf": func(a float64) float64 {
				if a < 0 {
					return -a
				}
				return a
			},
			"itof": func(a int) float64 {
				return float64(a)
			},
			"syncDuration": func(start, end time.Time) string {
				d := end.Sub(start)
				if d < time.Second {
					return fmt.Sprintf("%dms", d.Milliseconds())
				}
				if d < time.Minute {
					return fmt.Sprintf("%.1fs", d.Seconds())
				}
				return fmt.Sprintf("%.0fm", d.Minutes())
			},
			"formatDurationMs": func(ms int32) string {
				if ms < 1000 {
					return fmt.Sprintf("%dms", ms)
				}
				if ms < 60000 {
					return fmt.Sprintf("%.1fs", float64(ms)/1000)
				}
				mins := ms / 60000
				secs := (ms % 60000) / 1000
				if secs == 0 {
					return fmt.Sprintf("%dm", mins)
				}
				return fmt.Sprintf("%dm %ds", mins, secs)
			},
			"relativeTime": func(t interface{}) string {
				switch v := t.(type) {
				case time.Time:
					return relativeTime(v)
				case pgtype.Timestamptz:
					if v.Valid {
						return relativeTime(v.Time)
					}
					return "Never"
				case string:
					if parsed, err := time.Parse(time.RFC3339, v); err == nil {
						return relativeTime(parsed)
					}
					return v
				case *string:
					if v == nil {
						return ""
					}
					if parsed, err := time.Parse(time.RFC3339, *v); err == nil {
						return relativeTime(parsed)
					}
					return *v
				default:
					return ""
				}
			},
			"formatUUID": func(u pgtype.UUID) string {
				return formatUUID(u)
			},
			"formatIntervalMinutes": func(minutes int) string {
				// Render a sync interval in human-readable form (e.g., "12h", "4h", "30m", "1d").
				if minutes <= 0 {
					return "N/A"
				}
				if minutes >= 1440 && minutes%1440 == 0 {
					d := minutes / 1440
					if d == 1 {
						return "24h"
					}
					return fmt.Sprintf("%dd", d)
				}
				if minutes >= 60 && minutes%60 == 0 {
					return fmt.Sprintf("%dh", minutes/60)
				}
				if minutes >= 60 {
					return fmt.Sprintf("%dh %dm", minutes/60, minutes%60)
				}
				return fmt.Sprintf("%dm", minutes)
			},
			"accountLabel": func(name string, mask interface{}) string {
				// Format an account name with optional last-4 digits for disambiguation.
				// mask can be *string, string, or pgtype.Text.
				var m string
				switch v := mask.(type) {
				case string:
					m = v
				case *string:
					if v != nil {
						m = *v
					}
				case pgtype.Text:
					if v.Valid {
						m = v.String
					}
				}
				if m != "" {
					return name + " ••" + m
				}
				return name
			},
			"statusBadge": func(status string) template.HTML {
				switch status {
				case "active":
					return `<span class="badge badge-success badge-sm">Active</span>`
				case "pending_reauth":
					return `<span class="badge badge-warning badge-sm">Re-auth Needed</span>`
				case "error":
					return `<span class="badge badge-error badge-sm">Error</span>`
				default:
					return `<span class="badge badge-ghost badge-sm">Disconnected</span>`
				}
			},
			"syncBadge": func(status string) template.HTML {
				switch status {
				case "success":
					return `<span class="badge badge-success badge-sm">success</span>`
				case "error":
					return `<span class="badge badge-error badge-sm">error</span>`
				case "in_progress":
					return `<span class="badge badge-warning badge-sm">in progress</span>`
				default:
					return template.HTML(`<span class="badge badge-ghost badge-sm">` + template.HTMLEscapeString(status) + `</span>`)
				}
			},
			"errorMessage": func(code string) string {
				messages := map[string]string{
					"ITEM_LOGIN_REQUIRED":      "Your bank login has changed. Please re-authenticate.",
					"INSUFFICIENT_CREDENTIALS": "Additional credentials are needed. Please re-authenticate.",
					"INVALID_CREDENTIALS":      "Your bank credentials are incorrect. Please re-authenticate.",
					"MFA_NOT_SUPPORTED":        "This connection requires MFA which is not supported. Please reconnect.",
					"NO_ACCOUNTS":              "No accounts found for this connection.",
					"enrollment.disconnected":  "This bank connection has been disconnected.",
				}
				if msg, ok := messages[code]; ok {
					return msg
				}
				return code
			},
			"syncFriendlyError": func(rawErr string) string {
				return bsync.FriendlyError(rawErr)
			},
			"configSource": func(sources map[string]string, key string) template.HTML {
				source := sources[key]
				switch source {
				case "env":
					return `<span class="badge badge-ghost badge-sm">from env</span>`
				case "db":
					return `<span class="badge badge-ghost badge-sm">from database</span>`
				default:
					return `<span class="badge badge-ghost badge-sm">default</span>`
				}
			},
			"toJSON": func(v any) template.JS {
				b, _ := json.Marshal(v)
				return template.JS(b)
			},
			"safeCSS": func(s string) template.CSS {
				return template.CSS(s)
			},
			"conditionSummary": func(c service.Condition) string {
				return service.ConditionSummary(c)
			},
			"deref": func(s *string) string {
				if s == nil {
					return ""
				}
				return *s
			},
			"lower": strings.ToLower,
			"syncLogFilterQuery": func(status, connID, trigger, dateFrom, dateTo string) template.URL {
				params := url.Values{}
				if status != "" {
					params.Set("status", status)
				}
				if connID != "" {
					params.Set("connection_id", connID)
				}
				if trigger != "" {
					params.Set("trigger", trigger)
				}
				if dateFrom != "" {
					params.Set("date_from", dateFrom)
				}
				if dateTo != "" {
					params.Set("date_to", dateTo)
				}
				return template.URL(params.Encode())
			},
			"derefFloat": func(f *float64) float64 {
				if f == nil {
					return 0
				}
				return *f
			},
			"humanize": func(s string) string {
				return strings.ReplaceAll(s, "_", " ")
			},
			"pageRange": func(current, total int) []int {
				// Returns page numbers to display: always include first, last,
				// current, and neighbors. Use 0 as ellipsis sentinel.
				if total <= 7 {
					pages := make([]int, total)
					for i := range pages {
						pages[i] = i + 1
					}
					return pages
				}
				seen := map[int]bool{}
				add := func(p int) {
					if p >= 1 && p <= total {
						seen[p] = true
					}
				}
				add(1)
				add(total)
				for d := -1; d <= 1; d++ {
					add(current + d)
				}
				sorted := make([]int, 0, len(seen))
				for p := range seen {
					sorted = append(sorted, p)
				}
				// Sort
				for i := 0; i < len(sorted); i++ {
					for j := i + 1; j < len(sorted); j++ {
						if sorted[j] < sorted[i] {
							sorted[i], sorted[j] = sorted[j], sorted[i]
						}
					}
				}
				// Insert 0 for gaps
				result := make([]int, 0, len(sorted)*2)
				for i, p := range sorted {
					if i > 0 && p > sorted[i-1]+1 {
						result = append(result, 0) // ellipsis
					}
					result = append(result, p)
				}
				return result
			},
			"firstChar": func(s string) string {
				if s == "" {
					return "?"
				}
				for _, r := range s {
					c := strings.ToUpper(string(r))
					if c >= "A" && c <= "Z" {
						return c
					}
					if c >= "0" && c <= "9" {
						return c
					}
				}
				return strings.ToUpper(string([]rune(s)[0]))
			},
			"expired": func(s *string) bool {
				if s == nil {
					return false
				}
				t, err := time.Parse(time.RFC3339, *s)
				if err != nil {
					return false
				}
				return t.Before(time.Now())
			},
			"percent": func(value, max float64) float64 {
				if max <= 0 {
					return 0
				}
				return (value / max) * 100
			},
			"formatAmount": func(amount float64) string {
				neg := amount < 0
				abs := math.Abs(amount)
				formatted := formatCurrency(abs)
				if neg {
					return "-" + formatted
				}
				return formatted
			},
			"formatBalance": func(amount float64) string {
				abs := math.Abs(amount)
				if abs >= 1_000_000 {
					return fmt.Sprintf("$%.1fM", abs/1_000_000)
				}
				if abs >= 1_000 {
					whole := int(abs)
					cents := int((abs - float64(whole)) * 100)
					// Format with comma separators
					s := fmt.Sprintf("%d", whole)
					if len(s) > 3 {
						result := ""
						for i, c := range s {
							if i > 0 && (len(s)-i)%3 == 0 {
								result += ","
							}
							result += string(c)
						}
						s = result
					}
					return fmt.Sprintf("$%s.%02d", s, cents)
				}
				return fmt.Sprintf("$%.2f", abs)
			},
			"accountTypeIcon": func(acctType string) string {
				switch acctType {
				case "depository":
					return "landmark"
				case "credit":
					return "credit-card"
				case "loan":
					return "file-text"
				case "investment":
					return "trending-up"
				default:
					return "wallet"
				}
			},
			"accountTypeLabel": func(acctType, subtype string) string {
				if subtype != "" {
					labels := map[string]string{
						"checking":         "Checking",
						"savings":          "Savings",
						"credit card":      "Credit Card",
						"credit_card":      "Credit Card",
						"money market":     "Money Market",
						"money_market":     "Money Market",
						"cd":               "CD",
						"paypal":           "PayPal",
						"student":          "Student Loan",
						"mortgage":         "Mortgage",
						"auto":             "Auto Loan",
						"401k":             "401(k)",
						"ira":              "IRA",
						"brokerage":        "Brokerage",
						"prepaid":          "Prepaid",
						"hsa":              "HSA",
					}
					if label, ok := labels[subtype]; ok {
						return label
					}
					return subtype
				}
				labels := map[string]string{
					"depository":  "Bank Account",
					"credit":     "Credit Card",
					"loan":       "Loan",
					"investment": "Investment",
				}
				if label, ok := labels[acctType]; ok {
					return label
				}
				return acctType
			},
			"formatDateTime": func(t interface{}) string {
				format := func(tm time.Time) string {
					return tm.Local().Format("Jan 2, 2006 3:04 PM")
				}
				switch v := t.(type) {
				case time.Time:
					return format(v)
				case *time.Time:
					if v == nil {
						return ""
					}
					return format(*v)
				case string:
					if parsed, err := time.Parse(time.RFC3339, v); err == nil {
						return format(parsed)
					}
					return v
				case *string:
					if v == nil {
						return ""
					}
					if parsed, err := time.Parse(time.RFC3339, *v); err == nil {
						return format(parsed)
					}
					return *v
				default:
					return ""
				}
			},
			"formatDateShort": func(t interface{}) string {
				format := func(tm time.Time) string {
					return tm.Local().Format("Jan 2, 3:04 PM")
				}
				switch v := t.(type) {
				case time.Time:
					return format(v)
				case *time.Time:
					if v == nil {
						return ""
					}
					return format(*v)
				case string:
					if parsed, err := time.Parse(time.RFC3339, v); err == nil {
						return format(parsed)
					}
					return v
				case *string:
					if v == nil {
						return ""
					}
					if parsed, err := time.Parse(time.RFC3339, *v); err == nil {
						return format(parsed)
					}
					return *v
				default:
					return ""
				}
			},
			"formatDate": func(s string) string {
				if t, err := time.Parse("2006-01-02", s); err == nil {
					return t.Format("Jan 2, 2006")
				}
				return s
			},
			"formatNumeric": func(n pgtype.Numeric) string {
				if !n.Valid {
					return ""
				}
				f, err := n.Float64Value()
				if err != nil || !f.Valid {
					return ""
				}
				return formatCurrency(math.Abs(f.Float64))
			},
			"fmtBalance": func(v interface{}) string {
				var f float64
				switch val := v.(type) {
				case *float64:
					if val == nil {
						return ""
					}
					f = *val
				case float64:
					f = val
				default:
					return ""
				}
				neg := f < 0
				abs := math.Abs(f)
				whole := int(abs)
				cents := int(math.Round((abs - float64(whole)) * 100))
				s := fmt.Sprintf("%d", whole)
				if len(s) > 3 {
					result := ""
					for i, c := range s {
						if i > 0 && (len(s)-i)%3 == 0 {
							result += ","
						}
						result += string(c)
					}
					s = result
				}
				formatted := fmt.Sprintf("$%s.%02d", s, cents)
				if neg {
					formatted = "-" + formatted
				}
				return formatted
			},
			"fmtFloat": func(v interface{}) string {
				switch val := v.(type) {
				case *float64:
					if val == nil {
						return ""
					}
					return fmt.Sprintf("%.2f", *val)
				case float64:
					return fmt.Sprintf("%.2f", val)
				default:
					return ""
				}
			},
			"pgtext": func(v pgtype.Text) string {
				if !v.Valid {
					return ""
				}
				return v.String
			},
			"mapFloat": func(m map[string]float64, key string) float64 {
				return m[key]
			},
			"mapInt": func(m map[string]int64, key string) int64 {
				return m[key]
			},
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
	"partials/skeletons.html",
	"partials/breadcrumb.html",
	"partials/tx_row.html",
	"partials/tx_results.html",
}

func (tr *TemplateRenderer) parseTemplates() error {
	// Pages using the base layout (authenticated dashboard pages).
	basePages := []string{
		"pages/404.html",
		"pages/500.html",
		"pages/dashboard.html",
		"pages/connections.html",
		"pages/connection_new.html",
		"pages/connection_detail.html",
		"pages/connection_reauth.html",
		"pages/users.html",
		"pages/user_form.html",
		"pages/api_keys.html",
		"pages/api_key_new.html",
		"pages/api_key_created.html",
		"pages/sync_logs.html",
		"pages/settings.html",
		"pages/providers.html",
		"pages/csv_import.html",
		"pages/transactions.html",
		"pages/account_detail.html",
		"pages/categories.html",
		"pages/transaction_detail.html",
		"pages/mcp_settings.html",
		"pages/reviews.html",
		"pages/rules.html",
		"pages/review_instructions.html",
		"pages/insights.html",
		"pages/account_links.html",
		"pages/account_link_detail.html",
		"pages/reports.html",
		"pages/webhook_events.html",
		"pages/oauth_clients.html",
		"pages/oauth_client_new.html",
		"pages/oauth_client_created.html",
		"pages/agent_wizard.html",
	}

	// Pages using the wizard layout (login + first-run admin creation + OAuth consent).
	wizardPages := []string{
		"pages/login.html",
		"pages/setup_create_admin.html",
		"pages/oauth_authorize.html",
	}

	for _, page := range basePages {
		if err := tr.parseBasePage(page); err != nil {
			return err
		}
	}

	for _, page := range wizardPages {
		files := []string{"layout/wizard.html"}
		files = append(files, templatePartials...)
		files = append(files, page)

		t, err := template.New("").Funcs(tr.funcMap).ParseFS(templates.FS, files...)
		if err != nil {
			return fmt.Errorf("parse wizard page %s: %w", page, err)
		}
		// Store using just the filename (e.g., "login.html").
		tr.templates[path.Base(page)] = t
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
	tr.templates[path.Base(pagePath)] = t
	return nil
}

// RegisterBasePage registers a page template that uses the base layout.
// pagePath is relative to the templates FS root, e.g. "pages/dashboard.html".
// The template is accessible by its base filename, e.g. "dashboard.html".
func (tr *TemplateRenderer) RegisterBasePage(pagePath string) error {
	files := []string{"layout/base.html"}
	files = append(files, templatePartials...)
	files = append(files, pagePath)

	t, err := template.New("").Funcs(tr.funcMap).ParseFS(templates.FS, files...)
	if err != nil {
		return err
	}

	tr.mu.Lock()
	tr.templates[path.Base(pagePath)] = t
	tr.mu.Unlock()
	return nil
}

// Render writes the named template to the response writer with the given data.
// name is the base filename (e.g. "login.html", "dashboard.html").
// The request parameter is available for future middleware integration but
// is not currently used by the renderer itself.
func (tr *TemplateRenderer) Render(w http.ResponseWriter, r *http.Request, name string, data interface{}) {
	tr.mu.RLock()
	t, ok := tr.templates[name]
	tr.mu.RUnlock()
	if !ok {
		http.Error(w, "template not found: "+name, http.StatusInternalServerError)
		return
	}

	// Auto-inject common fields into map data if not already present.
	if m, ok := data.(map[string]any); ok {
		if _, exists := m["AdminUsername"]; !exists && tr.sm != nil {
			m["AdminUsername"] = tr.sm.GetString(r.Context(), sessionKeyAdminUsername)
		}
		if _, exists := m["AppVersion"]; !exists && tr.version != "" {
			m["AppVersion"] = tr.version
		}
		// Auto-inject sidebar notification badges from middleware context.
		if _, exists := m["NavBadges"]; !exists {
			m["NavBadges"] = getNavBadges(r.Context())
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, "template render error: "+err.Error(), http.StatusInternalServerError)
	}
}

// RenderPartial renders a named block from a template without the layout wrapper.
// name is the template key (e.g. "transactions.html"), block is the define name
// (e.g. "tx-results-partial"). Used for HTML fragment responses (AJAX swap).
func (tr *TemplateRenderer) RenderPartial(w http.ResponseWriter, r *http.Request, name, block string, data interface{}) {
	tr.mu.RLock()
	t, ok := tr.templates[name]
	tr.mu.RUnlock()
	if !ok {
		http.Error(w, "template not found: "+name, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, block, data); err != nil {
		http.Error(w, "template render error: "+err.Error(), http.StatusInternalServerError)
	}
}

// BaseTemplateData returns the common fields needed by every template as a map.
// Handlers can add page-specific fields to the returned map before rendering.
func BaseTemplateData(r *http.Request, sm *scs.SessionManager, currentPage, pageTitle string) map[string]any {
	return map[string]any{
		"PageTitle":     pageTitle,
		"CurrentPage":   currentPage,
		"Flash":         GetFlash(r.Context(), sm),
		"CSRFToken":     GetCSRFToken(r),
		"AdminUsername": sm.GetString(r.Context(), sessionKeyAdminUsername),
	}
}

// RenderNotFound renders the styled 404 page within the app layout.
func (tr *TemplateRenderer) RenderNotFound(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	data := BaseTemplateData(r, tr.sm, "", "Page Not Found")
	tr.Render(w, r, "404.html", data)
}

// RenderError renders the styled 500 page within the app layout.
func (tr *TemplateRenderer) RenderError(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusInternalServerError)
	data := BaseTemplateData(r, tr.sm, "", "Error")
	tr.Render(w, r, "500.html", data)
}

// SetVersion sets the application version for auto-injection into template data.
func (tr *TemplateRenderer) SetVersion(v string) {
	tr.version = v
}

// formatCurrency formats a non-negative float as "$X,XXX.XX".
func formatCurrency(abs float64) string {
	whole := int(abs)
	cents := int(math.Round((abs - float64(whole)) * 100))
	s := fmt.Sprintf("%d", whole)
	if len(s) > 3 {
		result := ""
		for i, c := range s {
			if i > 0 && (len(s)-i)%3 == 0 {
				result += ","
			}
			result += string(c)
		}
		s = result
	}
	return fmt.Sprintf("$%s.%02d", s, cents)
}

// AdminUsername returns the admin username from the session for use in template data maps.
func AdminUsername(r *http.Request, sm *scs.SessionManager) string {
	return sm.GetString(r.Context(), sessionKeyAdminUsername)
}

// RenderTo writes the named template to any io.Writer.
func (tr *TemplateRenderer) RenderTo(w io.Writer, name string, data interface{}) error {
	tr.mu.RLock()
	t, ok := tr.templates[name]
	tr.mu.RUnlock()
	if !ok {
		return fmt.Errorf("template not found: %s", name)
	}
	return t.ExecuteTemplate(w, "layout", data)
}
