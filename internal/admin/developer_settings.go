//go:build !headless && !lite

package admin

import (
	"errors"
	"net/http"
	"strings"

	"breadbox/internal/app"
	"breadbox/internal/service"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
)

// DeveloperSettingsHandler serves GET /settings/developer — the Developer Mode
// configuration tab: an enable toggle plus the GitHub repo + issue label the
// floating reporter opens issue drafts against.
func DeveloperSettingsHandler(a *app.App, svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		settings, err := svc.GetDevModeSettings(r.Context())
		if err != nil {
			http.Error(w, "Failed to load developer settings: "+err.Error(), http.StatusInternalServerError)
			return
		}

		flash := GetFlash(r.Context(), sm)
		var formError, formSuccess string
		if flash != nil {
			switch flash.Type {
			case "error":
				formError = flash.Message
			case "success":
				formSuccess = flash.Message
			}
		}

		props := pages.DeveloperSettingsProps{
			Form: pages.DeveloperSettingsFormFields{
				Enabled:    settings.Enabled,
				GithubRepo: settings.GithubRepo,
				IssueLabel: settings.IssueLabel,
			},
			FieldErrors: map[string]string{},
			FormError:   formError,
			FormSuccess: formSuccess,
			CSRFToken:   GetCSRFToken(r),
		}

		data := BaseTemplateData(r, sm, "developer-settings", "Developer")
		data["CSRFToken"] = props.CSRFToken
		data["Flash"] = nil
		renderSettingsTab(tr, w, r, data, pages.SettingsTabDeveloper, pages.DeveloperSettings(props))
	}
}

// DeveloperSettingsPostHandler handles POST /settings/developer — the enable
// toggle, repo, and label save together.
func DeveloperSettingsPostHandler(a *app.App, svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			FlashRedirect(w, r, sm, "error", "Could not read the submitted form.", "/settings/developer")
			return
		}

		enabled := r.FormValue("enabled") == "true"
		repo := strings.TrimSpace(r.FormValue("github_repo"))
		label := strings.TrimSpace(r.FormValue("issue_label"))

		params := service.UpdateDevModeSettingsParams{
			Enabled:    &enabled,
			GithubRepo: &repo,
			IssueLabel: &label,
		}

		if _, err := svc.UpdateDevModeSettings(r.Context(), params); err != nil {
			msg := "Failed to save developer settings."
			if errors.Is(err, service.ErrInvalidParameter) {
				msg = strings.TrimPrefix(err.Error(), "invalid parameter: ")
			}
			FlashRedirect(w, r, sm, "error", msg, "/settings/developer")
			return
		}
		SetFlash(r.Context(), sm, "success", "Developer settings saved.")
		http.Redirect(w, r, "/settings/developer", http.StatusSeeOther)
	}
}
