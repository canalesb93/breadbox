//go:build !headless && !lite

package pages

// LoginProps carries what the login form needs. Username is preserved on
// failure so the user doesn't have to retype it. Error is an inline message
// specific to the login attempt (distinct from session flashes).
type LoginProps struct {
	PageTitle string
	CSRFToken string
	Username  string
	Error     string
	FlashType string
	FlashMsg  string
	// ShowDevLogin renders a one-tap quick-login button that posts the
	// seeded dev credentials (admin@example.com / password). Set only when
	// the server runs with ENVIRONMENT=local — never in docker/prod.
	ShowDevLogin bool
}
