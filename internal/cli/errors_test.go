package cli

import (
	"errors"
	"net/http"
	"testing"

	"breadbox/internal/cli/config"
	"breadbox/internal/client"
)

func TestMapExitCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, ExitOK},
		{"runtime", errors.New("boom"), ExitRuntime},
		{"usage", UsageErrorf("missing arg %s", "name"), ExitUsage},
		{"no hosts", config.ErrNoHosts, ExitAuth},
		{"host not found", config.ErrHostNotFound, ExitAuth},
		{"api 401", &client.APIError{Status: http.StatusUnauthorized}, ExitAuth},
		{"api 403", &client.APIError{Status: http.StatusForbidden}, ExitAuth},
		{"api 400", &client.APIError{Status: http.StatusBadRequest}, ExitValidation},
		{"api 404", &client.APIError{Status: http.StatusNotFound}, ExitValidation},
		{"api 500", &client.APIError{Status: http.StatusInternalServerError}, ExitUpstream},
		{"api 503", &client.APIError{Status: http.StatusServiceUnavailable}, ExitUpstream},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MapExitCode(tt.err); got != tt.want {
				t.Fatalf("MapExitCode(%v) = %d want %d", tt.err, got, tt.want)
			}
		})
	}
}
