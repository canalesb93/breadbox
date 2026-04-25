package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"math"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	bsync "breadbox/internal/sync"
	"breadbox/internal/templates"
	"breadbox/internal/templates/components"
	"breadbox/internal/templates/components/pages"
	"breadbox/internal/version"

	"github.com/a-h/templ"
	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5/pgtype"
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
	"TxRowCompact": func(data any) (templ.Component, error) {
		tx, err := assertAdminTxRow(data)
		if err != nil {
			return nil, err
		}
		return components.TxRowCompact(tx), nil
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
	"TagChip": func(data any) (templ.Component, error) {
		td, err := assertTagChipData(data)
		if err != nil {
			return nil, err
		}
		return components.TagChip(td), nil
	},
	"TagChipSm": func(data any) (templ.Component, error) {
		td, err := assertTagChipData(data)
		if err != nil {
			return nil, err
		}
		return components.TagChipSm(td), nil
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
	"TxResults": func(data any) (templ.Component, error) {
		m, ok := data.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("want map[string]any, got %T", data)
		}
		return components.TxResults(txResultsPropsFromData(m)), nil
	},
	"Kbd": func(data any) (templ.Component, error) {
		key, ok := data.(string)
		if !ok {
			return nil, fmt.Errorf("Kbd: want string, got %T", data)
		}
		return components.Kbd(key), nil
	},
	"KbdChord": func(data any) (templ.Component, error) {
		keys, ok := data.([]string)
		if !ok {
			return nil, fmt.Errorf("KbdChord: want []string, got %T", data)
		}
		return components.KbdChord(keys...), nil
	},
	"KbdCombo": func(data any) (templ.Component, error) {
		keys, err := toStringSlice(data)
		if err != nil {
			return nil, fmt.Errorf("KbdCombo: %w", err)
		}
		return components.KbdCombo(keys...), nil
	},
	"ConditionRow": func(data any) (templ.Component, error) {
		m, ok := data.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("want map[string]any, got %T", data)
		}
		var cond service.Condition
		switch c := m["Cond"].(type) {
		case service.Condition:
			cond = c
		case *service.Condition:
			if c != nil {
				cond = *c
			}
		default:
			return nil, fmt.Errorf("ConditionRow: want service.Condition in Cond, got %T", m["Cond"])
		}
		idx := 0
		switch v := m["Idx"].(type) {
		case int:
			idx = v
		case int32:
			idx = int(v)
		case int64:
			idx = int(v)
		}
		conj, _ := m["Conj"].(string)
		return components.ConditionRow(components.ConditionRowProps{
			IsFirst:     idx == 0,
			Conj:        conj,
			FieldLabel:  ruleFieldLabel(cond.Field),
			OpLabel:     ruleOpLabel(cond.Op, cond.Field),
			ValueFormat: ruleValueFormat(cond.Value),
		}), nil
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

// assertTagChipData extracts a components.TagChipData from the several types
// that the tag-chip bridge accepts.
func assertTagChipData(data any) (components.TagChipData, error) {
	switch v := data.(type) {
	case service.TagResponse:
		return components.TagChipDataFromResponse(v), nil
	case *service.TagResponse:
		if v == nil {
			return components.TagChipData{}, nil
		}
		return components.TagChipDataFromResponse(*v), nil
	case service.AdminTransactionTag:
		return components.TagChipDataFromTx(v), nil
	case components.TagChipData:
		return v, nil
	default:
		return components.TagChipData{}, fmt.Errorf("unsupported tag chip type %T", data)
	}
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

// txResultsPropsFromData copies the AJAX transaction-list fields out of
// the render-time data map into a typed struct the TxResults templ
// component consumes. Called by the "TxResults" bridge entry and by
// the TransactionSearchHandler (which renders the fragment directly).
func txResultsPropsFromData(m map[string]any) components.TxResultsProps {
	getInt := func(key string) int {
		switch v := m[key].(type) {
		case int:
			return v
		case int32:
			return int(v)
		case int64:
			return int(v)
		}
		return 0
	}
	props := components.TxResultsProps{
		Page:           getInt("Page"),
		TotalPages:     getInt("TotalPages"),
		PageSize:       getInt("PageSize"),
		Total:          getInt("Total"),
		ShowingStart:   getInt("ShowingStart"),
		ShowingEnd:     getInt("ShowingEnd"),
		PaginationBase: fmt.Sprintf("%v", m["PaginationBase"]),
	}
	if txns, ok := m["Transactions"].([]service.AdminTransactionRow); ok {
		props.Transactions = txns
	}
	if groups, ok := m["DateGroups"].([]DateGroup); ok {
		props.DateGroups = make([]components.TxResultsDateGroup, len(groups))
		for i, g := range groups {
			props.DateGroups[i] = components.TxResultsDateGroup{
				Date:         g.Date,
				Label:        g.Label,
				Transactions: g.Transactions,
				DaySpending:  g.DaySpending,
				DayIncome:    g.DayIncome,
			}
		}
	}
	return props
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
		funcMap: template.FuncMap{
			"sub": func(a, b int) int { return a - b },
			"add": func(a, b int) int { return a + b },
			// strs collects variadic string args into a []string so
			// templates can pass slices through `renderComponent`
			// (e.g. `{{renderComponent "KbdCombo" (strs "cmd" "k")}}`).
			"strs": func(vals ...string) []string { return vals },
			"commaInt": func(n any) string {
				switch v := n.(type) {
				case int:
					return components.CommaInt(int64(v))
				case int64:
					return components.CommaInt(v)
				default:
					return fmt.Sprintf("%v", v)
				}
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
				return service.FormatDurationMs(end.Sub(start).Milliseconds())
			},
			"formatDurationMs": func(ms int32) string { return service.FormatDurationMs(int64(ms)) },
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
				return pgconv.FormatUUID(u)
			},
			"formatIntervalMinutes": components.FormatIntervalMinutes,
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
					return `<span class="badge badge-soft badge-success badge-sm">Active</span>`
				case "pending_reauth":
					return `<span class="badge badge-soft badge-warning badge-sm">Reauth Needed</span>`
				case "error":
					return `<span class="badge badge-soft badge-error badge-sm">Error</span>`
				default:
					return `<span class="badge badge-ghost badge-sm">Disconnected</span>`
				}
			},
			"syncBadge": func(status string) template.HTML {
				switch status {
				case "success":
					return `<span class="badge badge-soft badge-success badge-sm">success</span>`
				case "error":
					return `<span class="badge badge-soft badge-error badge-sm">error</span>`
				case "in_progress":
					return `<span class="badge badge-soft badge-warning badge-sm">in progress</span>`
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
			"conditionSummary": func(c any) string {
				switch v := c.(type) {
				case service.Condition:
					return service.ConditionSummary(v)
				case *service.Condition:
					if v == nil {
						return service.ConditionSummary(service.Condition{})
					}
					return service.ConditionSummary(*v)
				default:
					return service.ConditionSummary(service.Condition{})
				}
			},
			"isMatchAllCondition": func(c any) bool {
				switch v := c.(type) {
				case service.Condition:
					return v.Field == "" && len(v.And) == 0 && len(v.Or) == 0 && v.Not == nil
				case *service.Condition:
					if v == nil {
						return true
					}
					return v.Field == "" && len(v.And) == 0 && len(v.Or) == 0 && v.Not == nil
				default:
					return false
				}
			},
			"conditionCount": func(c any) int {
				count := func(v service.Condition) int {
					if len(v.And) > 0 {
						return len(v.And)
					}
					if len(v.Or) > 0 {
						return len(v.Or)
					}
					if v.Field != "" {
						return 1
					}
					return 0
				}
				switch v := c.(type) {
				case service.Condition:
					return count(v)
				case *service.Condition:
					if v == nil {
						return 0
					}
					return count(*v)
				default:
					return 0
				}
			},
			"actionsSummary": func(rule any) string {
				if r, ok := rule.(*service.TransactionRuleResponse); ok && r != nil {
					name := ""
					if r.CategoryName != nil {
						name = *r.CategoryName
					}
					return service.ActionsSummary(r.Actions, name)
				}
				if r, ok := rule.(service.TransactionRuleResponse); ok {
					name := ""
					if r.CategoryName != nil {
						name = *r.CategoryName
					}
					return service.ActionsSummary(r.Actions, name)
				}
				return ""
			},
			"triggerLabel":    service.TriggerLabel,
			"ruleFieldLabel":  ruleFieldLabel,
			"ruleOpLabel":     ruleOpLabel,
			"ruleValueFormat": ruleValueFormat,
			"ruleHasRetroactiveAction": func(actions []service.RuleAction) bool {
				// Retroactive apply materializes set_category / add_tag / remove_tag.
				// add_comment is sync-only. A rule with only comments isn't
				// usefully apply-able retroactively.
				for _, a := range actions {
					switch a.Type {
					case "set_category", "add_tag", "remove_tag":
						return true
					}
				}
				return false
			},
			"badgeCount": func(n int64) string {
				if n > 99 {
					return "99+"
				}
				return strconv.FormatInt(n, 10)
			},
			"deref": func(s *string) string {
				if s == nil {
					return ""
				}
				return *s
			},
			"derefInt32": func(p *int32) int32 {
				if p == nil {
					return 0
				}
				return *p
			},
			"prettyJSON": func(v interface{}) string {
				var raw []byte
				switch val := v.(type) {
				case *json.RawMessage:
					if val == nil {
						return ""
					}
					raw = *val
				case json.RawMessage:
					raw = val
				case []byte:
					raw = val
				default:
					return fmt.Sprintf("%v", v)
				}
				var buf bytes.Buffer
				if err := json.Indent(&buf, raw, "", "  "); err != nil {
					return string(raw)
				}
				return buf.String()
			},
			"lower":     strings.ToLower,
			"eqFold":    strings.EqualFold,
			"titleCase": components.TitleCase,
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
			// subtypeSuffix returns the humanized subtype for use after a type label
			// (e.g. "savings" in "Bank Account · savings"), or "" when the subtype
			// is an echo of the type label (e.g. "credit_card" under "Credit Card").
			// Comparison strips spaces and underscores and is case-insensitive, so
			// "Credit Card" vs "credit_card" collapses to the same token.
			"subtypeSuffix": func(typeLabel, subtype string) string {
				if subtype == "" {
					return ""
				}
				normalize := func(s string) string {
					s = strings.ToLower(s)
					s = strings.ReplaceAll(s, "_", "")
					s = strings.ReplaceAll(s, " ", "")
					return s
				}
				if normalize(typeLabel) == normalize(subtype) {
					return ""
				}
				return strings.ReplaceAll(subtype, "_", " ")
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
			"avatarURL": func(args ...any) string {
				if len(args) == 0 {
					return "/avatars/unknown"
				}
				var base string
				switch v := args[0].(type) {
				case pgtype.UUID:
					if !v.Valid {
						return "/avatars/unknown"
					}
					base = "/avatars/" + pgconv.FormatUUID(v)
				case string:
					if v == "" {
						return "/avatars/unknown"
					}
					base = "/avatars/" + v
				case *string:
					if v == nil || *v == "" {
						return "/avatars/unknown"
					}
					base = "/avatars/" + *v
				default:
					return "/avatars/unknown"
				}
				// Optional second arg: version fingerprint for cache busting.
				if len(args) > 1 {
					switch v := args[1].(type) {
					case pgtype.Timestamptz:
						if v.Valid {
							base += "?v=" + strconv.FormatInt(v.Time.Unix(), 10)
						}
					case string:
						if v != "" {
							base += "?v=" + v
						}
					}
				}
				return base
			},
			"firstChar": components.FirstChar,
			"firstWord": func(s string) string {
				if s == "" {
					return ""
				}
				parts := strings.Fields(s)
				return parts[0]
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
			"formatAmount": components.FormatAmount,
			"formatBalance": func(amount float64) string {
				return components.FormatBalance(amount)
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
						"checking":     "Checking",
						"savings":      "Savings",
						"credit card":  "Credit Card",
						"credit_card":  "Credit Card",
						"money market": "Money Market",
						"money_market": "Money Market",
						"cd":           "CD",
						"paypal":       "PayPal",
						"student":      "Student Loan",
						"mortgage":     "Mortgage",
						"auto":         "Auto Loan",
						"401k":         "401(k)",
						"ira":          "IRA",
						"brokerage":    "Brokerage",
						"prepaid":      "Prepaid",
						"hsa":          "HSA",
					}
					if label, ok := labels[subtype]; ok {
						return label
					}
					return subtype
				}
				labels := map[string]string{
					"depository": "Bank Account",
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
			// clockTime renders the local clock portion of a timestamp
			// ("2:03 AM"). Paired with a same-day day separator on the
			// activity timeline it disambiguates 10 events that would all
			// otherwise read "8 days ago" (#707).
			"clockTime": func(t interface{}) string {
				format := func(tm time.Time) string {
					return tm.Local().Format("3:04 PM")
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
					if parsed, err := time.Parse(time.RFC3339Nano, v); err == nil {
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
			"formatDate":   components.FormatDate,
			"relativeDate": components.RelativeDate,
			"formatNumeric": func(n pgtype.Numeric) string {
				f, ok := pgconv.NumericToFloat(n)
				if !ok {
					return ""
				}
				return service.FormatCurrency(math.Abs(f))
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
				formatted := "$" + components.CommaAmount(math.Abs(f))
				if f < 0 {
					return "-" + formatted
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
			"dict": func(pairs ...any) map[string]any {
				m := make(map[string]any, len(pairs)/2)
				for i := 0; i < len(pairs)-1; i += 2 {
					key, _ := pairs[i].(string)
					m[key] = pairs[i+1]
				}
				return m
			},
			"formatBytes": func(bytes int64) string {
				return service.FormatBytes(bytes)
			},
			// renderComponent bridges html/template partials to templ-generated
			// components during the incremental UI migration (issue #462).
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
	"partials/tx_row_compact.html",
	"partials/tx_results.html",
	"partials/tag_chip.html",
	"partials/condition_row.html",
}

func (tr *TemplateRenderer) parseTemplates() error {
	// Pages using the base layout (authenticated dashboard pages).
	basePages := []string{
		"pages/_templ_shell.html",
		// dashboard.html, settings.html, 404.html, and 500.html removed —
		// those pages render via RenderWithTempl which uses the
		// _templ_shell template key.
		"pages/connections.html",
		"pages/connection_new.html",
		"pages/connection_detail.html",
		"pages/connection_reauth.html",
		"pages/users.html",
		"pages/user_form.html",
		"pages/access.html",
		"pages/api_keys.html",
		"pages/api_key_new.html",
		"pages/api_key_created.html",
		"pages/sync_log_detail.html",
		"pages/providers.html",
		// pages/csv_import.html removed — renders via RenderWithTempl using
		// the _templ_shell template key (see pages.CSVImport).
		// pages/transactions.html removed — renders via RenderWithTempl
		// using the _templ_shell template key (see pages.Transactions).
		"pages/account_detail.html",
		// pages/categories.html and pages/category_form.html removed —
		// both render via RenderWithTempl using the _templ_shell template
		// key (see pages.Categories and pages.CategoryForm).
		"pages/transaction_detail.html",
		"pages/rules.html",
		// pages/tags.html and pages/tag_form.html removed — both render
		// via RenderWithTempl using the _templ_shell template key
		// (see pages.Tags and pages.TagForm).
		// pages/rule_form.html removed — renders via RenderWithTempl using
		// the _templ_shell template key (see pages.RuleForm).
		"pages/rule_detail.html",
		"pages/backups.html",
		"pages/account_link_detail.html",
		"pages/reports.html",
		"pages/report_detail.html",
		// pages/logs.html removed — renders via RenderWithTempl using the
		// _templ_shell template key (see pages.Logs).
		"pages/oauth_clients.html",
		"pages/oauth_client_new.html",
		"pages/oauth_client_created.html",
		// pages/mcp_guide.html, pages/agent_wizard.html, and pages/mcp_settings.html
		// are not registered as standalone base pages — their standalone routes
		// redirect to /agents and they're only consumed as composite extras
		// (see compositePages below).
		// pages/prompt_builder.html removed — renders via RenderWithTempl using
		// the _templ_shell template key (see pages.PromptBuilder).
		"pages/session_detail.html",
		"pages/my_account.html",
		"pages/getting_started.html",
		"pages/create_login.html",
	}

	// Pages that need multiple page files parsed together (for sub-template sharing).
	compositePages := map[string][]string{
		"pages/agents.html": {
			"pages/mcp_guide.html",
			"pages/agent_wizard.html",
			"pages/mcp_settings.html",
		},
	}

	for _, page := range basePages {
		if err := tr.parseBasePage(page); err != nil {
			return err
		}
	}

	// Composite pages: parsed with extra page files so sub-templates are available.
	for page, extras := range compositePages {
		files := []string{"layout/base.html"}
		files = append(files, templatePartials...)
		files = append(files, extras...)
		files = append(files, page)
		t, err := template.New("").Funcs(tr.funcMap).ParseFS(templates.FS, files...)
		if err != nil {
			return fmt.Errorf("parse composite page %s: %w", page, err)
		}
		name := path.Base(page)
		tr.templates[name] = t
		tr.specs[name] = files
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

