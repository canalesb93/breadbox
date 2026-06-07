//go:build !headless && !lite

package admin

import (
	"fmt"
	"net/http"
	"strings"

	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// connectorsSettingsPath is where every connector mutation flashes back to.
const connectorsSettingsPath = "/settings/connectors"

// parseConnectorForm builds a ConnectorLibraryInput from the add/edit modal's
// fields. Custom headers post as parallel arrays header_name[] / header_value[];
// rows with a blank name are dropped. A blank value on an existing connector is
// left empty so the service carries the stored value forward.
func parseConnectorForm(r *http.Request) service.ConnectorLibraryInput {
	names := r.Form["header_name"]
	values := r.Form["header_value"]
	headers := make([]service.ConnectorHeaderInput, 0, len(names))
	for i, name := range names {
		if strings.TrimSpace(name) == "" {
			continue
		}
		val := ""
		if i < len(values) {
			val = values[i]
		}
		headers = append(headers, service.ConnectorHeaderInput{
			Name:  strings.TrimSpace(name),
			Value: strings.TrimSpace(val),
		})
	}
	return service.ConnectorLibraryInput{
		Name:      strings.TrimSpace(r.FormValue("name")),
		URL:       strings.TrimSpace(r.FormValue("url")),
		Transport: strings.TrimSpace(r.FormValue("transport")),
		Note:      strings.TrimSpace(r.FormValue("note")),
		Headers:   headers,
	}
}

// CreateConnectorAdminHandler handles POST /-/connectors — add a connector to
// the global library from the Connectors settings page.
func CreateConnectorAdminHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			FlashRedirect(w, r, sm, "error", "Invalid form data.", connectorsSettingsPath)
			return
		}
		in := parseConnectorForm(r)
		if _, err := svc.CreateConnector(r.Context(), in); err != nil {
			FlashRedirect(w, r, sm, "error", "Failed to add connector: "+err.Error(), connectorsSettingsPath)
			return
		}
		FlashRedirect(w, r, sm, "success", fmt.Sprintf("Added connector %q.", in.Name), connectorsSettingsPath)
	}
}

// UpdateConnectorAdminHandler handles POST /-/connectors/{id}/update. A header
// with a blank value keeps its stored value.
func UpdateConnectorAdminHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := r.ParseForm(); err != nil {
			FlashRedirect(w, r, sm, "error", "Invalid form data.", connectorsSettingsPath)
			return
		}
		in := parseConnectorForm(r)
		if _, err := svc.UpdateConnector(r.Context(), id, in); err != nil {
			FlashRedirect(w, r, sm, "error", "Failed to save connector: "+err.Error(), connectorsSettingsPath)
			return
		}
		FlashRedirect(w, r, sm, "success", fmt.Sprintf("Saved connector %q.", in.Name), connectorsSettingsPath)
	}
}

// DeleteConnectorAdminHandler handles POST /-/connectors/{id}/delete.
func DeleteConnectorAdminHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.DeleteConnector(r.Context(), id); err != nil {
			FlashRedirect(w, r, sm, "error", "Failed to delete connector: "+err.Error(), connectorsSettingsPath)
			return
		}
		FlashRedirect(w, r, sm, "success", "Connector deleted.", connectorsSettingsPath)
	}
}

// ImportConnectorsAdminHandler handles POST /-/connectors/import — create
// connectors from a pasted Claude/Manus-style mcpServers JSON.
func ImportConnectorsAdminHandler(svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			FlashRedirect(w, r, sm, "error", "Invalid form data.", connectorsSettingsPath)
			return
		}
		created, skipped, err := svc.ImportConnectorsJSON(r.Context(), r.FormValue("config_json"))
		if err != nil {
			FlashRedirect(w, r, sm, "error", "Import failed: "+err.Error(), connectorsSettingsPath)
			return
		}
		msg := fmt.Sprintf("Imported %d connector(s).", len(created))
		if len(skipped) > 0 {
			msg += fmt.Sprintf(" Skipped %d non-HTTP entr(ies): %s.", len(skipped), strings.Join(skipped, ", "))
		}
		FlashRedirect(w, r, sm, "success", msg, connectorsSettingsPath)
	}
}
