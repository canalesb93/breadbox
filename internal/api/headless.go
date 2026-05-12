package api

import (
	"net/http"

	"breadbox/internal/app"
	"breadbox/internal/db"
	mw "breadbox/internal/middleware"
	"breadbox/internal/service"
)

// HeadlessBootstrapHandler returns a one-shot readiness report for the CLI's
// `doctor` command (and any external orchestrator). Mounted under
// /api/v1/headless/bootstrap; any API key (read or write) is sufficient.
//
// The response shape is HeadlessBootstrapResponse in internal/service; see
// also docs/api-endpoints.md and the OpenAPI spec.
func HeadlessBootstrapHandler(svc *service.Service, a *app.App, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		in := service.HeadlessBootstrapInput{
			Version:          version,
			EncryptionKeySet: a.Config.EncryptionKey != nil,
			SchedulerRunning: a.Scheduler != nil && a.Scheduler.IsRunning(),
			Providers:        collectProviderStatus(a),
		}
		// Latest embedded migration version — used to compute
		// "migrations_current". Best-effort: if the embed read fails the
		// report still goes out, just with version=0 and current=false.
		if latest, err := db.LatestEmbeddedMigration(); err == nil {
			in.LatestMigrationVersion = latest
		}

		resp, err := svc.HeadlessBootstrap(r.Context(), in)
		if err != nil {
			// Service only surfaces an error when the DB is unreachable.
			// Return 503 so a healthcheck can flag the box as not-ready;
			// the body still carries whatever partial info we have.
			mw.WriteError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", err.Error())
			return
		}
		writeData(w, resp)
	}
}

// collectProviderStatus walks the configured providers and reports which are
// wired. Order is stable (plaid, teller) so the report doesn't churn between
// calls. Provider env (sandbox / production / etc.) is only emitted when the
// provider is configured — keeps the JSON tidy for `breadbox doctor` output.
func collectProviderStatus(a *app.App) []service.HeadlessProvider {
	out := []service.HeadlessProvider{
		{Name: "plaid", Configured: a.Config.PlaidClientID != ""},
		{Name: "teller", Configured: a.Config.TellerAppID != ""},
	}
	if out[0].Configured {
		out[0].Env = a.Config.PlaidEnv
	}
	if out[1].Configured {
		out[1].Env = a.Config.TellerEnv
	}
	return out
}
