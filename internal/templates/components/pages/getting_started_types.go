//go:build !headless && !lite

package pages

// GettingStartedProps drives the redesigned onboarding walkthrough. The
// handler computes step completion against the live database and assembles
// flat primitives so the templ side stays pure-presentation.
//
// The page composes the shared onboarding components (OnboardingHero +
// progress ring, OnboardingStats, SetupStep ×5, OnboardingAltPath,
// OnboardingNextSteps, OnboardingFooter). State per step is derived from the
// completion flags + ActiveStep:
//
//   - a step whose flag is set renders "complete"
//   - the first incomplete step (ActiveStep) renders "active" — except the
//     sync step, which renders "in_progress" while a connection exists but no
//     sync has landed yet
//   - every later incomplete step renders "pending"
type GettingStartedProps struct {
	// Step completion flags.
	HasMember     bool
	HasProvider   bool
	HasConnection bool
	HasSync       bool
	HasAPIKey     bool

	// Aggregate progress.
	CompletedSteps int
	TotalSteps     int

	// AllComplete is true once every step is done — flips the page from the
	// checklist to the celebratory "what's next" grid and the success-tinted
	// hero.
	AllComplete bool

	// ActiveStep is the 1-based index of the first incomplete step (0 when
	// AllComplete). Drives which step renders elevated/active.
	ActiveStep int

	// TimeRemaining is a rough estimate of time left to finish setup, e.g.
	// "~5 min left". Empty hides the hero's time row (also empty when done).
	TimeRemaining string

	// Counters for the always-on stats strip + per-step inline stats.
	MemberCount      int64
	ConnectionCount  int64
	AccountCount     int64
	TransactionCount int64
	SuccessfulSyncs  int64
	ApiKeyCount      int64

	// Whether the onboarding guide has been dismissed (controls the footer's
	// dismiss form vs. the always-available note).
	OnboardingDismissed bool

	// CSRF token for the dismiss form.
	CSRFToken string
}
