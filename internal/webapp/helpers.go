//go:build !headless && !lite

package webapp

import (
	"net/http"

	"github.com/a-h/templ"

	"breadbox/internal/admin"
	"breadbox/internal/webapp/layout"
)

// render writes a templ component as an HTML document with the given status.
func render(w http.ResponseWriter, r *http.Request, status int, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = c.Render(r.Context(), w)
}

// themeClass reads the bb_theme cookie and returns the <html> class for first paint.
// "system" can't be resolved server-side, so it renders light and app.js corrects it.
func themeClass(r *http.Request) string {
	c, err := r.Cookie("bb_theme")
	if err == nil && c.Value == "dark" {
		return "dark"
	}
	return ""
}

// shellData assembles the data the authenticated app shell needs for a request.
func (h *Handler) shellData(r *http.Request, title string) layout.ShellData {
	return layout.ShellData{
		Title:       title,
		CurrentPath: r.URL.Path,
		UserName:    admin.SessionUsername(h.sm, r),
		ThemeClass:  themeClass(r),
	}
}
