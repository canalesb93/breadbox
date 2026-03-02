package admin

import (
	"fmt"
	"html/template"
	"io"
	"net/http"
	"path"
	"sync"
	"time"

	"breadbox/internal/templates"

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

// TemplateRenderer parses and renders HTML templates.
type TemplateRenderer struct {
	mu        sync.RWMutex
	templates map[string]*template.Template
	funcMap   template.FuncMap
}

// NewTemplateRenderer parses all embedded templates and returns a renderer.
func NewTemplateRenderer() (*TemplateRenderer, error) {
	tr := &TemplateRenderer{
		templates: make(map[string]*template.Template),
		funcMap: template.FuncMap{
			"sub": func(a, b int) int { return a - b },
			"add": func(a, b int) int { return a + b },
			"relativeTime": func(t interface{}) string {
				switch v := t.(type) {
				case time.Time:
					return relativeTime(v)
				case pgtype.Timestamptz:
					if v.Valid {
						return relativeTime(v.Time)
					}
					return "Never"
				default:
					return ""
				}
			},
			"formatUUID": func(u pgtype.UUID) string {
				return formatUUID(u)
			},
			"formatNumeric": func(n pgtype.Numeric) string {
				if !n.Valid {
					return ""
				}
				// Use the decimal representation from the numeric value.
				f, err := n.Float64Value()
				if err != nil || !f.Valid {
					return ""
				}
				return fmt.Sprintf("%.2f", f.Float64)
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
	}

	// Pages using the wizard layout (setup wizard + login).
	wizardPages := []string{
		"pages/login.html",
		"pages/setup_step1.html",
		"pages/setup_step2.html",
		"pages/setup_step3.html",
		"pages/setup_step4.html",
		"pages/setup_step5.html",
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

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, "template render error: "+err.Error(), http.StatusInternalServerError)
	}
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
