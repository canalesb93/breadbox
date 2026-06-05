//go:build !headless && !lite

package admin

import (
	"net/http"

	"breadbox/internal/app"
	"breadbox/internal/service"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
)

// ConnectorsSettingsPageHandler serves GET /settings/connectors — the global
// custom-MCP connector library. A Providers-style directory of connectors;
// adding/editing happens in a modal. Header values never leave the server.
func ConnectorsSettingsPageHandler(a *app.App, svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		connectors, err := svc.ListConnectors(ctx)
		if err != nil {
			http.Error(w, "Failed to load connectors: "+err.Error(), http.StatusInternalServerError)
			return
		}
		views := make([]pages.ConnectorView, 0, len(connectors))
		for _, c := range connectors {
			headerNames := make([]string, 0, len(c.Headers))
			for _, h := range c.Headers {
				headerNames = append(headerNames, h.Name)
			}
			views = append(views, pages.ConnectorView{
				ShortID:     c.ShortID,
				Name:        c.Name,
				URL:         c.URL,
				Transport:   c.Transport,
				Note:        c.Note,
				HeaderNames: headerNames,
				HasSecret:   c.HasSecret,
			})
		}

		flash := GetFlash(ctx, sm)
		var formError, formSuccess string
		if flash != nil {
			switch flash.Type {
			case "error":
				formError = flash.Message
			case "success":
				formSuccess = flash.Message
			}
		}

		props := pages.ConnectorsSettingsProps{
			Connectors:  views,
			CSRFToken:   GetCSRFToken(r),
			FormError:   formError,
			FormSuccess: formSuccess,
		}

		data := BaseTemplateData(r, sm, "connectors-settings", "Connectors")
		data["CSRFToken"] = props.CSRFToken
		data["Flash"] = nil
		renderSettingsTab(tr, w, r, data, pages.SettingsTabConnectors, pages.ConnectorsSettings(props))
	}
}
