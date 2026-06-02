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

// DeveloperSettingsHandler serves GET /settings/developer — the Developer
// Mode configuration tab. It owns the enable toggle plus the GitHub repo /
// token / label the floating reporter files issues against, and lists the
// reports filed from this instance.
//
// The GitHub token never leaves the server in plaintext — only a masked
// placeholder surfaces. Submitting an empty token field keeps the current
// value; the "Remove the saved token" checkbox clears it.
func DeveloperSettingsHandler(a *app.App, svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		settings, err := svc.GetDevModeSettings(ctx, a.Config.EncryptionKey)
		if err != nil {
			http.Error(w, "Failed to load developer settings: "+err.Error(), http.StatusInternalServerError)
			return
		}

		reports, err := svc.ListDevReports(ctx, 15)
		if err != nil {
			// History is display-only — degrade to an empty list rather than
			// failing the whole tab.
			reports = nil
		}

		form := pages.DeveloperSettingsFormFields{
			Enabled:    settings.Enabled,
			GithubRepo: settings.GithubRepo,
			IssueLabel: settings.IssueLabel,
		}
		if settings.TokenMask != nil {
			form.GithubTokenDisplay = *settings.TokenMask
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

		props := pages.DeveloperSettingsProps{
			Form:            form,
			FieldErrors:     map[string]string{},
			FormError:       formError,
			FormSuccess:     formSuccess,
			CSRFToken:       GetCSRFToken(r),
			Reports:         toDevReportRows(reports),
			RepoConfigured:  settings.GithubRepo != "",
			TokenConfigured: settings.HasToken,
		}

		data := BaseTemplateData(r, sm, "developer-settings", "Developer")
		data["CSRFToken"] = props.CSRFToken
		data["Flash"] = nil
		renderSettingsTab(tr, w, r, data, pages.SettingsTabDeveloper, pages.DeveloperSettings(props))
	}
}

// DeveloperSettingsPostHandler handles POST /settings/developer — the
// multi-input save. The enable toggle, repo, token, and label all write
// together; an empty token keeps the current value, and the clear_token
// checkbox removes it.
func DeveloperSettingsPostHandler(a *app.App, svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			FlashRedirect(w, r, sm, "error", "Could not read the submitted form.", "/settings/developer")
			return
		}

		enabled := r.FormValue("enabled") == "true"
		repo := strings.TrimSpace(r.FormValue("github_repo"))
		label := strings.TrimSpace(r.FormValue("issue_label"))
		token := strings.TrimSpace(r.FormValue("github_token"))
		clearToken := r.FormValue("clear_token") == "true"

		params := service.UpdateDevModeSettingsParams{
			Enabled:          &enabled,
			GithubRepo:       &repo,
			IssueLabel:       &label,
			ClearGithubToken: clearToken,
		}
		if !clearToken && token != "" {
			params.GithubToken = &token
		}

		if _, err := svc.UpdateDevModeSettings(r.Context(), params, a.Config.EncryptionKey); err != nil {
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

// toDevReportRows maps service summaries to the page row shape, formatting the
// timestamp for display.
func toDevReportRows(reports []service.DevReportSummary) []pages.DevReportRow {
	rows := make([]pages.DevReportRow, 0, len(reports))
	for _, rep := range reports {
		created := ""
		if !rep.CreatedAt.IsZero() {
			created = rep.CreatedAt.Local().Format("Jan 2, 15:04")
		}
		rows = append(rows, pages.DevReportRow{
			ShortID:     rep.ShortID,
			Type:        rep.Type,
			Title:       rep.Title,
			PagePath:    rep.PagePath,
			Status:      rep.Status,
			IssueNumber: rep.GithubIssueNumber,
			IssueURL:    rep.GithubIssueURL,
			CreatedBy:   rep.CreatedBy,
			CreatedAt:   created,
			Error:       rep.Error,
		})
	}
	return rows
}
