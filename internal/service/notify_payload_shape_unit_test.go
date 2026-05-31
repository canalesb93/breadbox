//go:build !lite

package service

import (
	"encoding/json"
	"testing"
	"time"
)

// T4_unmarshal parses raw JSON into a map so each test can inspect
// exactly which keys are present without relying on struct field defaults.
func T4_unmarshal(t *testing.T, p NotificationPayload) map[string]any {
	t.Helper()
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("T4: json.Marshal failed: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("T4: json.Unmarshal failed: %v", err)
	}
	return m
}

// T4_requireKey asserts a key is present in the map.
func T4_requireKey(t *testing.T, m map[string]any, key string) {
	t.Helper()
	if _, ok := m[key]; !ok {
		t.Errorf("T4: expected key %q to be present in JSON output", key)
	}
}

// T4_refuseKey asserts a key is absent from the map (omitempty omitted it).
func T4_refuseKey(t *testing.T, m map[string]any, key string) {
	t.Helper()
	if _, ok := m[key]; ok {
		t.Errorf("T4: expected key %q to be omitted from JSON output (omitempty)", key)
	}
}

// TestT4_NotificationPayload_RequiredKeysAlwaysPresent confirms that event,
// title, and sent_at are always emitted regardless of other field values.
func TestT4_NotificationPayload_RequiredKeysAlwaysPresent(t *testing.T) {
	p := NotificationPayload{
		Event:  "test",
		Title:  "hello",
		SentAt: time.Now().UTC().Format(time.RFC3339),
	}
	m := T4_unmarshal(t, p)
	T4_requireKey(t, m, "event")
	T4_requireKey(t, m, "title")
	T4_requireKey(t, m, "sent_at")
}

// TestT4_NotificationPayload_OmitemptyFieldsOmittedWhenBlank verifies that
// body, priority, workflow, and url are omitted when their values are empty
// strings.
func TestT4_NotificationPayload_OmitemptyFieldsOmittedWhenBlank(t *testing.T) {
	p := NotificationPayload{
		Event:  "report",
		Title:  "Monthly digest",
		SentAt: time.Now().UTC().Format(time.RFC3339),
		// Body, Priority, Workflow, URL intentionally left blank.
	}
	m := T4_unmarshal(t, p)
	T4_refuseKey(t, m, "body")
	T4_refuseKey(t, m, "priority")
	T4_refuseKey(t, m, "workflow")
	T4_refuseKey(t, m, "url")
}

// TestT4_NotificationPayload_OmitemptyFieldsPresentWhenSet verifies that the
// optional fields are included when non-empty.
func TestT4_NotificationPayload_OmitemptyFieldsPresentWhenSet(t *testing.T) {
	p := NotificationPayload{
		Event:    "report",
		Title:    "Weekly digest",
		Body:     "Here is your weekly summary.",
		Priority: "info",
		Workflow: "weekly-review",
		URL:      "https://breadbox.example.com/workflows/runs/abc123",
		SentAt:   time.Now().UTC().Format(time.RFC3339),
	}
	m := T4_unmarshal(t, p)
	T4_requireKey(t, m, "event")
	T4_requireKey(t, m, "title")
	T4_requireKey(t, m, "sent_at")
	T4_requireKey(t, m, "body")
	T4_requireKey(t, m, "priority")
	T4_requireKey(t, m, "workflow")
	T4_requireKey(t, m, "url")
}

// TestT4_NotificationPayload_EventValueRoundtrip verifies the event string
// value is preserved exactly through JSON serialisation.
func TestT4_NotificationPayload_EventValueRoundtrip(t *testing.T) {
	for _, event := range []string{"test", "report"} {
		p := NotificationPayload{
			Event:  event,
			Title:  "title",
			SentAt: time.Now().UTC().Format(time.RFC3339),
		}
		m := T4_unmarshal(t, p)
		if got, ok := m["event"].(string); !ok || got != event {
			t.Errorf("T4: event = %v, want %q", m["event"], event)
		}
	}
}

// TestT4_NotificationPayload_SentAtIsRFC3339 checks that SentAt round-trips
// as a valid RFC3339 timestamp.
func TestT4_NotificationPayload_SentAtIsRFC3339(t *testing.T) {
	now := time.Now().UTC()
	p := NotificationPayload{
		Event:  "test",
		Title:  "ts check",
		SentAt: now.Format(time.RFC3339),
	}
	m := T4_unmarshal(t, p)
	raw, ok := m["sent_at"].(string)
	if !ok {
		t.Fatalf("T4: sent_at is not a string: %T", m["sent_at"])
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Errorf("T4: sent_at %q is not valid RFC3339: %v", raw, err)
	}
	// Truncate to second precision for comparison (RFC3339 has second resolution).
	if !parsed.Equal(now.Truncate(time.Second)) {
		t.Errorf("T4: sent_at round-trip mismatch: got %v, want %v", parsed, now.Truncate(time.Second))
	}
}

// TestT4_NotificationPayload_TitleValueRoundtrip ensures the title is preserved
// verbatim (no JSON escaping issues for typical headline text).
func TestT4_NotificationPayload_TitleValueRoundtrip(t *testing.T) {
	const want = "Breadbox workflow notification test"
	p := NotificationPayload{
		Event:  "test",
		Title:  want,
		SentAt: time.Now().UTC().Format(time.RFC3339),
	}
	m := T4_unmarshal(t, p)
	if got, ok := m["title"].(string); !ok || got != want {
		t.Errorf("T4: title = %v, want %q", m["title"], want)
	}
}
