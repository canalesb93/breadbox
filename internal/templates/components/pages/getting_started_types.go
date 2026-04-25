package pages

// GettingStartedProps mirrors the data map the old getting_started.html
// read. The handler computes step completion against the live database
// and assembles flat primitives so the templ side stays pure-presentation.
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

	// Counters for the summary strip — only shown when TransactionCount > 0.
	ConnectionCount  int64
	AccountCount     int64
	TransactionCount int64
	SuccessfulSyncs  int64

	// Whether the onboarding banner has been dismissed (controls the
	// dismiss form vs. the always-available info note).
	OnboardingDismissed bool

	// CSRF token for the dismiss form.
	CSRFToken string
}
