//go:build !headless && !lite

package pages

import (
	"encoding/json"
	"sort"
	"time"

	"breadbox/internal/service"
)

// metadataKV is one rendered key/value row from a transaction's metadata blob.
type metadataKV struct {
	Key   string
	Value string
}

// transactionMetadataPairs parses the transaction's free-form metadata JSONB
// into sorted display rows. Returns nil for an empty/absent/invalid object so
// the template can skip the section entirely.
func transactionMetadataPairs(raw json.RawMessage) []metadataKV {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil || len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]metadataKV, 0, len(keys))
	for _, k := range keys {
		out = append(out, metadataKV{Key: k, Value: formatMetadataValue(m[k])})
	}
	return out
}

// formatMetadataValue renders a metadata value for display: strings verbatim,
// everything else as compact JSON.
func formatMetadataValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// ActivityDayGroup mirrors admin.ActivityDayGroup. Duplicated locally so the
// templ component can consume it without importing the admin package (which
// would create an import cycle: admin -> pages -> admin).
type ActivityDayGroup struct {
	Label  string
	Events []service.ActivityEntry
}

// TransactionDetailProps is the full view model for the
// /admin/transactions/{id} detail page. Populated by the
// TransactionDetailHandler in internal/admin/transactions.go and rendered
// inside base.html via TemplateRenderer.RenderWithTempl.
type TransactionDetailProps struct {
	CSRFToken string


	Transaction   *service.TransactionResponse
	TransactionID string

	// Account context (denormalized so the template never has to nil-check
	// the optional Account pointer for the simple text labels).
	AccountID       string
	AccountName     string
	UserName        string
	InstitutionName string
	AccountMask     string
	AccountType     string
	ConnectionID    string
	Account         *service.AccountResponse

	// Activity timeline.
	Activity     []service.ActivityEntry
	ActivityDays []ActivityDayGroup

	// Now is the single time anchor captured by the handler at the top
	// of buildActivityTimeline. The per-row relative-time helpers
	// (relativeTimeStrAt) read this so they share the exact same clock
	// reference as the day-bucket labels in ActivityDays — preventing
	// the "Today" / "yesterday" mismatch that occurs when each path
	// calls time.Now() independently across midnight.
	Now time.Time

	// Has the transaction got a needs-review tag attached?
	HasPendingReview bool

	// Tags currently attached + the registered-tag list (for the picker).
	CurrentTags   []service.TransactionTagResponse
	AvailableTags []service.TagResponse

	// Two-level category tree powering the inline category picker.
	Categories []service.CategoryResponse

	// MaxCommentLength is the server-side cap (service.MaxCommentLength)
	// surfaced to the composer so the char counter mirrors it.
	MaxCommentLength int
}
