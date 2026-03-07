package api

import (
	"encoding/json"
	"net/http"

	"breadbox/internal/version"
)

type versionResponse struct {
	Version        string  `json:"version"`
	Latest         string  `json:"latest,omitempty"`
	UpdateAvail    *bool   `json:"update_available"`
	LatestURL      string  `json:"latest_url,omitempty"`
}

// VersionHandler returns a handler for GET /api/v1/version that checks for
// available updates via the GitHub releases API.
func VersionHandler(checker *version.Checker, currentVersion string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := versionResponse{
			Version: currentVersion,
		}

		if currentVersion == "dev" {
			f := false
			resp.UpdateAvail = &f
		} else {
			updateAvailable, latest, err := checker.CheckForUpdate(r.Context())
			if err != nil {
				// GitHub unreachable — update_available is null.
				resp.UpdateAvail = nil
			} else {
				resp.UpdateAvail = updateAvailable
				if latest != nil {
					resp.Latest = latest.Version
					resp.LatestURL = latest.URL
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}
