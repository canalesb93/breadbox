//go:build integration && !headless && !lite

package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// T6_routerWithPresetSlug registers EnableWorkflowPresetAdminHandler on a
// minimal chi router with the {slug} param wired, bypassing RequireAdmin
// middleware for handler-level coverage.
func T6_routerWithPresetSlug(svc *service.Service) *chi.Mux {
	r := chi.NewRouter()
	r.Post("/-/workflow-presets/{slug}/enable", EnableWorkflowPresetAdminHandler(svc))
	return r
}

// T6_postPresetEnable fires a POST /-/workflow-presets/{slug}/enable with the
// supplied form values and returns the recorded response.
func T6_postPresetEnable(t *testing.T, svc *service.Service, slug string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	r := T6_routerWithPresetSlug(svc)
	req := httptest.NewRequest(http.MethodPost, "/-/workflow-presets/"+slug+"/enable",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// T6_mustDecodeError decodes the standard {"error":{"code","message"}} envelope.
func T6_mustDecodeError(t *testing.T, body []byte) (code, message string) {
	t.Helper()
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("T6: decode error envelope: %v (body=%s)", err, body)
	}
	return env.Error.Code, env.Error.Message
}

// T6_TestEnableWorkflowPreset_ConsentRequired verifies that the consent gate
// returns 400 CONSENT_REQUIRED when consent has not been previously given and
// the form does not supply consent=true.
func T6_TestEnableWorkflowPreset_ConsentRequired(t *testing.T) {
	svc := newTestSvc(t)

	// No consent=true and consent not yet acknowledged in DB.
	w := T6_postPresetEnable(t, svc, "routine-reviewer", url.Values{})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("T6: expected 400, got %d: %s", w.Code, w.Body.String())
	}
	code, msg := T6_mustDecodeError(t, w.Body.Bytes())
	if code != "CONSENT_REQUIRED" {
		t.Errorf("T6: error.code = %q, want CONSENT_REQUIRED", code)
	}
	if msg == "" {
		t.Error("T6: error.message should not be empty")
	}
}

// T6_TestEnableWorkflowPreset_UnknownSlug verifies that an unrecognised preset
// slug returns 404 NOT_FOUND even when consent=true is provided.
func T6_TestEnableWorkflowPreset_UnknownSlug(t *testing.T) {
	svc := newTestSvc(t)

	// Acknowledge consent first so the slug-lookup path is exercised.
	if err := svc.AcknowledgeWorkflowsConsent(t.Context()); err != nil {
		t.Fatalf("T6: AcknowledgeWorkflowsConsent: %v", err)
	}

	w := T6_postPresetEnable(t, svc, "no-such-preset", url.Values{
		"consent": {"true"},
	})

	if w.Code != http.StatusNotFound {
		t.Fatalf("T6: expected 404, got %d: %s", w.Code, w.Body.String())
	}
	code, _ := T6_mustDecodeError(t, w.Body.Bytes())
	if code != "NOT_FOUND" {
		t.Errorf("T6: error.code = %q, want NOT_FOUND", code)
	}
}

// T6_TestEnableWorkflowPreset_Success verifies that enabling a valid preset
// with consent=true returns 200 and a workflow response body containing the
// correct slug.
func T6_TestEnableWorkflowPreset_Success(t *testing.T) {
	svc := newTestSvc(t)

	w := T6_postPresetEnable(t, svc, "routine-reviewer", url.Values{
		"consent": {"true"},
		"enabled": {"false"}, // start paused so no scheduler interference
	})

	if w.Code != http.StatusOK {
		t.Fatalf("T6: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp service.AgentDefinitionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("T6: decode response: %v (body=%s)", err, w.Body.String())
	}
	if resp.Slug != "routine-reviewer" {
		t.Errorf("T6: resp.Slug = %q, want %q", resp.Slug, "routine-reviewer")
	}

	// Consent should now be persisted: the handler calls
	// AcknowledgeWorkflowsConsent after the first successful enable.
	if !svc.WorkflowsConsentAcknowledged(t.Context()) {
		t.Error("T6: consent should be acknowledged after first successful enable")
	}
}

// T6_TestEnableWorkflowPreset_AlreadyEnabled verifies that enabling a preset
// that has already been instantiated returns 200 with already_enabled:true
// (idempotent gallery behaviour — the page just refreshes the tile state).
func T6_TestEnableWorkflowPreset_AlreadyEnabled(t *testing.T) {
	svc := newTestSvc(t)

	// First enable directly via the service (bypassing the HTTP handler).
	if _, err := svc.EnableWorkflowFromPreset(t.Context(), "backlog-closer",
		service.EnableWorkflowFromPresetParams{Enabled: false}); err != nil {
		t.Fatalf("T6: first EnableWorkflowFromPreset: %v", err)
	}
	// Mark consent as acknowledged so the handler skips the consent gate.
	if err := svc.AcknowledgeWorkflowsConsent(t.Context()); err != nil {
		t.Fatalf("T6: AcknowledgeWorkflowsConsent: %v", err)
	}

	// Second enable via the HTTP handler — should be idempotent (200 +
	// already_enabled:true) rather than returning a conflict error.
	w := T6_postPresetEnable(t, svc, "backlog-closer", url.Values{})

	if w.Code != http.StatusOK {
		t.Fatalf("T6: expected 200 for double-enable, got %d: %s", w.Code, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("T6: decode body: %v", err)
	}
	alreadyEnabled, _ := body["already_enabled"].(bool)
	if !alreadyEnabled {
		t.Errorf("T6: expected already_enabled:true in response, got %v", body)
	}
	slug, _ := body["slug"].(string)
	if slug != "backlog-closer" {
		t.Errorf("T6: expected slug=backlog-closer in response, got %q", slug)
	}
}

// T6_TestEnableWorkflowPreset_ConsentAlreadyAcknowledged verifies that once
// consent has been recorded in the DB, subsequent enables do NOT require
// consent=true in the form body — the gate is a one-time check.
func T6_TestEnableWorkflowPreset_ConsentAlreadyAcknowledged(t *testing.T) {
	svc := newTestSvc(t)

	// Pre-seed consent acknowledgement.
	if err := svc.AcknowledgeWorkflowsConsent(t.Context()); err != nil {
		t.Fatalf("T6: AcknowledgeWorkflowsConsent: %v", err)
	}

	// Enable without consent=true — the gate must be bypassed.
	w := T6_postPresetEnable(t, svc, "weekly-money-digest", url.Values{
		"enabled": {"false"},
	})

	if w.Code != http.StatusOK {
		t.Fatalf("T6: expected 200 when consent already given, got %d: %s", w.Code, w.Body.String())
	}

	var resp service.AgentDefinitionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("T6: decode response: %v", err)
	}
	if resp.Slug != "weekly-money-digest" {
		t.Errorf("T6: resp.Slug = %q, want %q", resp.Slug, "weekly-money-digest")
	}
}

// T6_TestEnableWorkflowPreset_InstructionsTooLong verifies that
// additional_instructions exceeding 4000 chars returns 400 INVALID_PARAMETER.
// This exercises the ErrInvalidParameter → INVALID_PARAMETER error branch.
func T6_TestEnableWorkflowPreset_InstructionsTooLong(t *testing.T) {
	svc := newTestSvc(t)

	if err := svc.AcknowledgeWorkflowsConsent(t.Context()); err != nil {
		t.Fatalf("T6: AcknowledgeWorkflowsConsent: %v", err)
	}

	// 4001 bytes of 'x' — one byte over the maxAdditionalInstructions cap.
	oversized := strings.Repeat("x", 4001)
	w := T6_postPresetEnable(t, svc, "routine-reviewer", url.Values{
		"additional_instructions": {oversized},
		"enabled":                 {"false"},
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("T6: expected 400 for oversized instructions, got %d: %s", w.Code, w.Body.String())
	}
	code, _ := T6_mustDecodeError(t, w.Body.Bytes())
	if code != "INVALID_PARAMETER" {
		t.Errorf("T6: error.code = %q, want INVALID_PARAMETER", code)
	}
}

// TestT6WorkflowActionsHandler is the single exported entry-point that runs
// all T6 sub-tests. The admin package's TestMain (tags_integration_test.go)
// sets up and tears down the DB; sub-tests are independent — each calls
// newTestSvc(t) for a fresh service instance sharing the connection pool.
func TestT6WorkflowActionsHandler(t *testing.T) {
	t.Run("ConsentRequired", T6_TestEnableWorkflowPreset_ConsentRequired)
	t.Run("UnknownSlug", T6_TestEnableWorkflowPreset_UnknownSlug)
	t.Run("Success", T6_TestEnableWorkflowPreset_Success)
	t.Run("AlreadyEnabled", T6_TestEnableWorkflowPreset_AlreadyEnabled)
	t.Run("ConsentAlreadyAcknowledged", T6_TestEnableWorkflowPreset_ConsentAlreadyAcknowledged)
	t.Run("InstructionsTooLong", T6_TestEnableWorkflowPreset_InstructionsTooLong)
}
