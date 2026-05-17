//go:build !lite

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"

	"breadbox/internal/service"
)

// TestWriteAgentDefinitionMutationError pins the iter-35 fix for audit
// HIGH #4: before iter-35 only CreateAgentDefinitionHandler mapped the
// Postgres unique-constraint failure to 409 CONFLICT — the Update path
// fell through to a generic 500. The shared helper now serves both
// handlers, so a slug-rename collision returns 409 consistently.
//
// Tests run against the helper directly (no router setup needed), which
// also makes the validation-error and generic-error branches cheap to
// pin.
func TestWriteAgentDefinitionMutationError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "validation error → 400",
			err:        fmt.Errorf("max_turns: %w", service.ErrInvalidParameter),
			wantStatus: 400,
			wantCode:   "VALIDATION_ERROR",
		},
		{
			name:       "duplicate key → 409",
			err:        errors.New(`ERROR: duplicate key value violates unique constraint "agent_definitions_slug_key" (SQLSTATE 23505)`),
			wantStatus: 409,
			wantCode:   "CONFLICT",
		},
		{
			name:       "unique constraint wording → 409",
			err:        errors.New("postgres: row violates unique constraint"),
			wantStatus: 409,
			wantCode:   "CONFLICT",
		},
		{
			name:       "generic error → 500",
			err:        errors.New("connection refused"),
			wantStatus: 500,
			wantCode:   "INTERNAL_ERROR",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			if !writeAgentDefinitionMutationError(rr, tc.err, "fallback msg") {
				t.Fatal("helper returned false; expected true (response was written)")
			}
			if rr.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rr.Code, tc.wantStatus)
			}
			var body struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v\nbody: %s", err, rr.Body.String())
			}
			if body.Error.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", body.Error.Code, tc.wantCode)
			}
		})
	}
}
