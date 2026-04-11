package admin

import (
	"net/http"

	"breadbox/internal/app"
	"breadbox/internal/db"

	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5/pgtype"
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
		dismissed := GetConfigBool(ctx, a.Queries, "onboarding_dismissed")

		data := BaseTemplateData(r, sm, "getting-started", "Getting Started")
		data["HasMember"] = hasMember
		data["HasProvider"] = hasProvider
		data["HasConnection"] = hasConnection
		data["HasSync"] = hasSync
		data["HasAPIKey"] = hasAPIKey
		data["CompletedSteps"] = completedSteps
		data["TotalSteps"] = 5
		data["ConnectionCount"] = connCount
		data["AccountCount"] = accountCount
		data["TransactionCount"] = txCount
		data["SuccessfulSyncs"] = successfulSyncs
		data["OnboardingDismissed"] = dismissed

		tr.Render(w, r, "getting_started.html", data)
	}
}

// DismissGettingStartedHandler handles POST /getting-started/dismiss.
// Sets onboarding_dismissed=true and redirects to the dashboard.
func DismissGettingStartedHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := a.Queries.SetAppConfig(r.Context(), db.SetAppConfigParams{
			Key:   "onboarding_dismissed",
			Value: pgtype.Text{String: "true", Valid: true},
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
			Value: pgtype.Text{String: "false", Valid: true},
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
