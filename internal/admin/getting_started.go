package admin

import (
	"net/http"

	"breadbox/internal/app"
	"breadbox/internal/appconfig"
	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
)

// GettingStartedHandler serves GET /getting-started — the setup walkthrough page.
func GettingStartedHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Step completion checks.
		// Step 1: Create a family member (user).
		userCount, err := a.Queries.CountUsers(ctx)
		if err != nil {
			a.Logger.Error("getting started: count users", "error", err)
		}
		hasMember := userCount > 0

		// Step 2: Configure a bank provider (Plaid or Teller).
		hasProvider := a.Config.PlaidClientID != "" || a.Config.TellerAppID != ""

		// Step 3: Connect a bank.
		connCount, err := a.Queries.CountConnections(ctx)
		if err != nil {
			a.Logger.Error("getting started: count connections", "error", err)
		}
		hasConnection := connCount > 0

		// Step 4: First successful sync.
		successfulSyncs, err := a.Queries.CountSuccessfulSyncs(ctx)
		if err != nil {
			a.Logger.Error("getting started: count successful syncs", "error", err)
		}
		hasSync := successfulSyncs > 0

		// Step 5: Configure agents (at least one API key).
		activeAPIKeys, err := a.Queries.CountActiveApiKeys(ctx)
		if err != nil {
			a.Logger.Error("getting started: count active api keys", "error", err)
		}
		hasAPIKey := activeAPIKeys > 0

		// Progress metrics.
		accountCount, err := a.Queries.CountAccounts(ctx)
		if err != nil {
			a.Logger.Error("getting started: count accounts", "error", err)
		}

		txCount, err := a.Queries.CountTransactions(ctx)
		if err != nil {
			a.Logger.Error("getting started: count transactions", "error", err)
		}

		// Count completed steps.
		completedSteps := 0
		if hasMember {
			completedSteps++
		}
		if hasProvider {
			completedSteps++
		}
		if hasConnection {
			completedSteps++
		}
		if hasSync {
			completedSteps++
		}
		if hasAPIKey {
			completedSteps++
		}

		// Check if onboarding is dismissed (for the nav badge display).
		dismissed := appconfig.Bool(ctx, a.Queries, "onboarding_dismissed", false)

		data := BaseTemplateData(r, sm, "getting-started", "Getting Started")
		props := pages.GettingStartedProps{
			HasMember:           hasMember,
			HasProvider:         hasProvider,
			HasConnection:       hasConnection,
			HasSync:             hasSync,
			HasAPIKey:           hasAPIKey,
			CompletedSteps:      completedSteps,
			TotalSteps:          5,
			ConnectionCount:     connCount,
			AccountCount:        accountCount,
			TransactionCount:    txCount,
			SuccessfulSyncs:     successfulSyncs,
			OnboardingDismissed: dismissed,
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
			SetFlash(r.Context(), sm, "error", "Failed to dismiss guide. Please try again.")
			http.Redirect(w, r, "/getting-started", http.StatusSeeOther)
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
			SetFlash(r.Context(), sm, "error", "Failed to re-open guide. Please try again.")
			http.Redirect(w, r, "/settings", http.StatusSeeOther)
			return
		}
		SetFlash(r.Context(), sm, "success", "Getting Started guide re-opened.")
		http.Redirect(w, r, "/getting-started", http.StatusSeeOther)
	}
}
