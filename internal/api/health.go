package api

import (
	"encoding/json"
	"net/http"

	"breadbox/internal/app"
)

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// HealthLiveHandler returns a handler that responds with a basic liveness status.
func HealthLiveHandler(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(healthResponse{
			Status:  "ok",
			Version: version,
		})
	}
}

type readyResponse struct {
	Status    string `json:"status"`
	DB        string `json:"db"`
	DBError   string `json:"db_error,omitempty"`
	Scheduler string `json:"scheduler"`
	Version   string `json:"version"`
}

// HealthReadyHandler returns a handler that checks DB connectivity and scheduler status.
func HealthReadyHandler(a *app.App, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := readyResponse{
			Status:  "ok",
			Version: version,
		}

		// DB ping
		if err := a.DB.Ping(r.Context()); err != nil {
			resp.Status = "degraded"
			resp.DB = "error"
			resp.DBError = err.Error()
		} else {
			resp.DB = "ok"
		}

		// Scheduler status
		if a.Scheduler != nil && a.Scheduler.IsRunning() {
			resp.Scheduler = "running"
		} else {
			resp.Scheduler = "stopped"
		}

		w.Header().Set("Content-Type", "application/json")
		if resp.Status != "ok" {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		json.NewEncoder(w).Encode(resp)
	}
}
