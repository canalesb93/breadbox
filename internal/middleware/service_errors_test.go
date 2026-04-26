package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"breadbox/internal/service"
)

func TestMapServiceError(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantOK     bool
		wantStatus int
		wantCode   string
	}{
		{"nil", nil, false, 0, ""},
		{"unknown", errors.New("boom"), false, 0, ""},
		{"not_found", service.ErrNotFound, true, http.StatusNotFound, "NOT_FOUND"},
		{"invalid_parameter", service.ErrInvalidParameter, true, http.StatusBadRequest, "INVALID_PARAMETER"},
		{"invalid_cursor", service.ErrInvalidCursor, true, http.StatusBadRequest, "INVALID_CURSOR"},
		{"forbidden", service.ErrForbidden, true, http.StatusForbidden, "FORBIDDEN"},
		{"sync_in_progress", service.ErrSyncInProgress, true, http.StatusConflict, "SYNC_IN_PROGRESS"},
		{"invalid_api_key", service.ErrInvalidAPIKey, true, http.StatusUnauthorized, "INVALID_API_KEY"},
		{"revoked_api_key", service.ErrRevokedAPIKey, true, http.StatusUnauthorized, "REVOKED_API_KEY"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := MapServiceError(c.err)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v", ok, c.wantOK)
			}
			if !c.wantOK {
				return
			}
			if got.Status != c.wantStatus {
				t.Errorf("Status = %d, want %d", got.Status, c.wantStatus)
			}
			if got.Code != c.wantCode {
				t.Errorf("Code = %q, want %q", got.Code, c.wantCode)
			}
			if got.Message == "" {
				t.Errorf("Message empty, want service error text")
			}
		})
	}
}

func TestMapServiceError_WrappedError(t *testing.T) {
	// Wrapped sentinels should still resolve via errors.Is.
	wrapped := fmt.Errorf("primary account: %w", service.ErrNotFound)
	got, ok := MapServiceError(wrapped)
	if !ok {
		t.Fatal("ok = false, want true for wrapped ErrNotFound")
	}
	if got.Code != "NOT_FOUND" {
		t.Errorf("Code = %q, want NOT_FOUND", got.Code)
	}
	if got.Message != wrapped.Error() {
		t.Errorf("Message = %q, want %q (wrapped err text preserved)", got.Message, wrapped.Error())
	}
}
