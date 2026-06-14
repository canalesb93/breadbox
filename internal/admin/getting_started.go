//go:build !headless && !lite

package admin

import (
	"net/http"

	"breadbox/internal/app"
	"breadbox/internal/db"
	"breadbox/internal/pgconv"

	"github.com/alexedwards/scs/v2"
)

// The dedicated /getting-started page is retired — the home-feed onboarding
// banner (components.OnboardingBanner) is the onboarding surface now. The
// GET route redirects to `/`; only the dismiss/reopen toggles remain here,
// driving the `onboarding_dismissed` app-config flag that the banner reads.

// DismissGettingStartedHandler handles POST /getting-started/dismiss.
// Sets onboarding_dismissed=true and redirects to the dashboard.
func DismissGettingStartedHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := a.Queries.SetAppConfig(r.Context(), db.SetAppConfigParams{
			Key:   "onboarding_dismissed",
			Value: pgconv.Text("true"),
		}); err != nil {
			a.Logger.Error("dismiss getting started: set app config", "error", err)
			FlashRedirect(w, r, sm, "error", "Failed to dismiss guide. Please try again.", "/")
			return
		}
		SetFlash(r.Context(), sm, "success", "Setup guide dismissed. You can re-open it from Settings.")
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

// ReopenGettingStartedHandler handles POST /getting-started/reopen.
// Clears onboarding_dismissed and redirects to the home feed, where the
// onboarding banner re-appears.
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
		SetFlash(r.Context(), sm, "success", "Setup guide re-opened.")
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}
