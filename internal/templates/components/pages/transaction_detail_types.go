package pages

import (
	"time"

	"breadbox/internal/service"
	"breadbox/internal/templates/components"
)

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

	Breadcrumbs []components.Breadcrumb

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
