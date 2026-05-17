//go:build integration && !lite

package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"breadbox/internal/appconfig"
	"breadbox/internal/service"
)

// devEncKey is the 32-byte AES-256 key used by these tests.
// Tests should not share keys with production; this one is fine for the
// integration DB.
var devEncKey = []byte("0123456789abcdef0123456789abcdef")

func mustCreateAgentDefinition(t *testing.T, svc *service.Service, slug string, enabled bool) *service.AgentDefinitionResponse {
	t.Helper()
	def, err := svc.CreateAgentDefinition(context.Background(), service.CreateAgentDefinitionParams{
		Name:         "Test " + slug,
		Slug:         slug,
		Prompt:       "Review uncategorized transactions and categorize them.",
		ToolScope:    "read_write",
		AllowedTools: []string{"mcp__breadbox__*"},
		Model:        "claude-opus-4-7",
		MaxTurns:     10,
		Enabled:      enabled,
	})
	if err != nil {
		t.Fatalf("create agent definition %q: %v", slug, err)
	}
	return def
}

func TestCreateAgentDefinition_Success(t *testing.T) {
	svc, _, _ := newService(t)
	def := mustCreateAgentDefinition(t, svc, "svc-create-success", false)
	if def.ShortID == "" || len(def.ShortID) != 8 {
		t.Errorf("expected 8-char short_id, got %q", def.ShortID)
	}
	if def.ToolScope != "read_write" {
		t.Errorf("ToolScope = %q, want read_write", def.ToolScope)
	}
	if def.Model != "claude-opus-4-7" {
		t.Errorf("Model = %q, want claude-opus-4-7", def.Model)
	}
	if len(def.AllowedTools) != 1 || def.AllowedTools[0] != "mcp__breadbox__*" {
		t.Errorf("AllowedTools = %v, want [mcp__breadbox__*]", def.AllowedTools)
	}
}

func TestCreateAgentDefinition_DefaultsAppliedWhenOmitted(t *testing.T) {
	svc, _, _ := newService(t)
	def, err := svc.CreateAgentDefinition(context.Background(), service.CreateAgentDefinitionParams{
		Name:   "defaults",
		Slug:   "svc-defaults",
		Prompt: "p",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if def.ToolScope != "read_write" {
		t.Errorf("default ToolScope = %q, want read_write", def.ToolScope)
	}
	if def.Model != service.DefaultAgentModel {
		t.Errorf("default Model = %q, want %q", def.Model, service.DefaultAgentModel)
	}
	if def.MaxTurns != service.DefaultAgentMaxTurns {
		t.Errorf("default MaxTurns = %d, want %d", def.MaxTurns, service.DefaultAgentMaxTurns)
	}
}

func TestCreateAgentDefinition_InvalidSlug(t *testing.T) {
	svc, _, _ := newService(t)
	_, err := svc.CreateAgentDefinition(context.Background(), service.CreateAgentDefinitionParams{
		Name: "bad", Slug: "Has Spaces", Prompt: "p",
	})
	if err == nil {
		t.Fatal("expected validation error for slug with spaces")
	}
	if !errors.Is(err, service.ErrInvalidParameter) {
		t.Errorf("err = %v, want ErrInvalidParameter", err)
	}
}

func TestCreateAgentDefinition_InvalidScheduleCron(t *testing.T) {
	svc, _, _ := newService(t)
	bad := "not a cron expression"
	_, err := svc.CreateAgentDefinition(context.Background(), service.CreateAgentDefinitionParams{
		Name: "bad cron", Slug: "svc-bad-cron", Prompt: "p", ScheduleCron: &bad,
	})
	if err == nil {
		t.Fatal("expected validation error for bad cron")
	}
	if !errors.Is(err, service.ErrInvalidParameter) {
		t.Errorf("err = %v, want ErrInvalidParameter", err)
	}
}

func TestGetAgentDefinition_BySlugAndShortID(t *testing.T) {
	svc, _, _ := newService(t)
	def := mustCreateAgentDefinition(t, svc, "svc-get-slug-short", true)

	bySlug, err := svc.GetAgentDefinition(context.Background(), def.Slug)
	if err != nil {
		t.Fatalf("get by slug: %v", err)
	}
	if bySlug.ID != def.ID {
		t.Errorf("by slug returned different ID: %s vs %s", bySlug.ID, def.ID)
	}

	byShort, err := svc.GetAgentDefinition(context.Background(), def.ShortID)
	if err != nil {
		t.Fatalf("get by short_id: %v", err)
	}
	if byShort.ID != def.ID {
		t.Errorf("by short_id returned different ID: %s vs %s", byShort.ID, def.ID)
	}
}

func TestGetAgentDefinition_NotFound(t *testing.T) {
	svc, _, _ := newService(t)
	_, err := svc.GetAgentDefinition(context.Background(), "no-such-agent")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestUpdateAgentDefinition_PatchSemantics(t *testing.T) {
	svc, _, _ := newService(t)
	def := mustCreateAgentDefinition(t, svc, "svc-patch", false)

	newName := "Renamed"
	updated, err := svc.UpdateAgentDefinition(context.Background(), def.Slug, service.UpdateAgentDefinitionParams{
		Name: &newName,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != newName {
		t.Errorf("Name = %q, want %q", updated.Name, newName)
	}
	if updated.Prompt != def.Prompt {
		t.Errorf("Prompt drifted: %q vs original %q", updated.Prompt, def.Prompt)
	}
	if updated.ToolScope != def.ToolScope {
		t.Errorf("ToolScope drifted: %q vs original %q", updated.ToolScope, def.ToolScope)
	}
}

func TestSetAgentDefinitionEnabled_Toggle(t *testing.T) {
	svc, _, _ := newService(t)
	def := mustCreateAgentDefinition(t, svc, "svc-toggle", false)

	on, err := svc.SetAgentDefinitionEnabled(context.Background(), def.Slug, true)
	if err != nil {
		t.Fatalf("enable: %v", err)
	}
	if !on.Enabled {
		t.Errorf("expected enabled=true after toggle")
	}

	off, err := svc.SetAgentDefinitionEnabled(context.Background(), def.Slug, false)
	if err != nil {
		t.Fatalf("disable: %v", err)
	}
	if off.Enabled {
		t.Errorf("expected enabled=false after toggle")
	}
}

func TestDeleteAgentDefinition_RemovesRow(t *testing.T) {
	svc, _, _ := newService(t)
	def := mustCreateAgentDefinition(t, svc, "svc-delete", false)
	if err := svc.DeleteAgentDefinition(context.Background(), def.Slug); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.GetAgentDefinition(context.Background(), def.Slug); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestMintRunAPIKey_ScopeFromToolScope(t *testing.T) {
	svc, _, _ := newService(t)
	read := mustCreateAgentDefinition(t, svc, "svc-mint-read", false)
	// Manually flip ToolScope to read_only via an update for this test.
	scope := "read_only"
	read, err := svc.UpdateAgentDefinition(context.Background(), read.Slug, service.UpdateAgentDefinitionParams{ToolScope: &scope})
	if err != nil {
		t.Fatalf("update read: %v", err)
	}
	keyR, err := svc.MintRunAPIKey(context.Background(), read, "abcd1234")
	if err != nil {
		t.Fatalf("mint read: %v", err)
	}
	if keyR.Scope != "read_only" {
		t.Errorf("read key scope = %q, want read_only", keyR.Scope)
	}
	if !strings.HasPrefix(keyR.Name, "agent:") {
		t.Errorf("key Name = %q, want agent:* prefix", keyR.Name)
	}

	write := mustCreateAgentDefinition(t, svc, "svc-mint-write", false)
	keyW, err := svc.MintRunAPIKey(context.Background(), write, "wxyz9876")
	if err != nil {
		t.Fatalf("mint write: %v", err)
	}
	if keyW.Scope != "full_access" {
		t.Errorf("write key scope = %q, want full_access", keyW.Scope)
	}
}

// --- Settings ---

func TestGetAgentSettings_DefaultsWhenUnset(t *testing.T) {
	svc, _, _ := newService(t)
	s, err := svc.GetAgentSettings(context.Background(), devEncKey)
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	if s.AuthMode != appconfig.AuthModeSubscription {
		t.Errorf("AuthMode default = %q, want subscription", s.AuthMode)
	}
	if s.SubscriptionToken != nil {
		t.Errorf("SubscriptionToken should be nil when unset, got %v", *s.SubscriptionToken)
	}
	if s.MaxConcurrent != 1 {
		t.Errorf("MaxConcurrent default = %d, want 1", s.MaxConcurrent)
	}
}

func TestUpdateAgentSettings_TokenMaskedNeverReturnedPlaintext(t *testing.T) {
	svc, _, _ := newService(t)
	plain := "sk-ant-oat01-ABCDEFGHIJKLMNOPQRSTUVWXYZ-test"
	token := plain
	s, err := svc.UpdateAgentSettings(context.Background(), service.UpdateAgentSettingsParams{
		SubscriptionToken: &token,
	}, devEncKey)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if s.SubscriptionToken == nil {
		t.Fatal("SubscriptionToken should be populated (masked) after set")
	}
	if *s.SubscriptionToken == plain {
		t.Errorf("masked value should not equal plaintext")
	}
	if !strings.Contains(*s.SubscriptionToken, "••••") {
		t.Errorf("masked value should contain bullets, got %q", *s.SubscriptionToken)
	}

	// Confirm GET returns the same masked form (and not plaintext).
	got, err := svc.GetAgentSettings(context.Background(), devEncKey)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.SubscriptionToken == nil || *got.SubscriptionToken != *s.SubscriptionToken {
		t.Errorf("GET masked value mismatched PUT response")
	}
}

func TestUpdateAgentSettings_ClearTokenWithEmptyString(t *testing.T) {
	svc, _, _ := newService(t)
	set := "sk-ant-oat01-some-value-12345"
	if _, err := svc.UpdateAgentSettings(context.Background(), service.UpdateAgentSettingsParams{
		SubscriptionToken: &set,
	}, devEncKey); err != nil {
		t.Fatalf("set: %v", err)
	}
	empty := ""
	cleared, err := svc.UpdateAgentSettings(context.Background(), service.UpdateAgentSettingsParams{
		SubscriptionToken: &empty,
	}, devEncKey)
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if cleared.SubscriptionToken != nil {
		t.Errorf("expected SubscriptionToken nil after clearing, got %v", *cleared.SubscriptionToken)
	}
}

func TestUpdateAgentSettings_RejectsInvalidAuthMode(t *testing.T) {
	svc, _, _ := newService(t)
	bad := "banana"
	_, err := svc.UpdateAgentSettings(context.Background(), service.UpdateAgentSettingsParams{
		AuthMode: &bad,
	}, devEncKey)
	if !errors.Is(err, service.ErrInvalidParameter) {
		t.Errorf("err = %v, want ErrInvalidParameter", err)
	}
}
