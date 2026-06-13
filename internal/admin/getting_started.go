//go:build !headless && !lite

package admin

import (
	"net/http"

	"breadbox/internal/app"
	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
)

// GettingStartedHandler serves GET /getting-started — the setup walkthrough page.
func GettingStartedHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Step completion + progress is computed by the shared helper so the
		// /getting-started walkthrough and the home-feed "Finish setting up"
		// banner never disagree about which step is next.
		p := computeOnboardingProgress(r.Context(), a)

		data := BaseTemplateData(r, sm, "getting-started", "Getting Started")
		props := pages.GettingStartedProps{
			HasMember:           p.HasMember,
			HasProvider:         p.HasProvider,
			HasConnection:       p.HasConnection,
			HasSync:             p.HasSync,
			HasAPIKey:           p.HasAPIKey,
			CompletedSteps:      p.CompletedSteps,
			TotalSteps:          p.TotalSteps,
			AllComplete:         p.AllComplete,
			ActiveStep:          p.ActiveStep,
			TimeRemaining:       p.TimeRemaining,
			MemberCount:         p.MemberCount,
			ConnectionCount:     p.ConnectionCount,
			AccountCount:        p.AccountCount,
			TransactionCount:    p.TransactionCount,
			SuccessfulSyncs:     p.SuccessfulSyncs,
			ApiKeyCount:         p.ApiKeyCount,
			OnboardingDismissed: p.Dismissed,
			CSRFToken:           GetCSRFToken(r),
		}
		renderGettingStarted(w, r, tr, data, props)
	}
}

// renderGettingStarted mirrors the renderLogs / renderPromptBuilder pattern:
// it hands the typed GettingStartedProps to the templ component and uses
// RenderWithTempl to host it inside base.html.
func renderGettingStarted(w http.ResponseWriter, r *http.Request, tr *TemplateRenderer, data map[string]any, props pages.GettingStartedProps) {
	tr.RenderWithTempl(w, r, data, pages.GettingStarted(props))
}

// DismissGettingStartedHandler handles POST /getting-started/dismiss.
// Sets onboarding_dismissed=true and redirects to the dashboard.
func DismissGettingStartedHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := a.Queries.SetAppConfig(r.Context(), db.SetAppConfigParams{
			Key:   "onboarding_dismissed",
			Value: pgconv.Text("true"),
		}); err != nil {
			a.Logger.Error("dismiss getting started: set app config", "error", err)
			FlashRedirect(w, r, sm, "error", "Failed to dismiss guide. Please try again.", "/getting-started")
			return
		}
		SetFlash(r.Context(), sm, "success", "Getting Started guide dismissed. You can re-open it from Settings.")
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

// ReopenGettingStartedHandler handles POST /getting-started/reopen.
// Clears onboarding_dismissed and redirects to /getting-started.
func ReopenGettingStartedHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := a.Queries.SetAppConfig(r.Context(), db.SetAppConfigParams{
			Key:   "onboarding_dismissed",
			Value: pgconv.Text("false"),
		}); err != nil {
			a.Logger.Error("reopen getting started: set app config", "error", err)
			FlashRedirect(w, r, sm, "error", "Failed to re-open guide. Please try again.", "/settings")
			return
		}
		SetFlash(r.Context(), sm, "success", "Getting Started guide re-opened.")
		http.Redirect(w, r, "/getting-started", http.StatusSeeOther)
	}
}
