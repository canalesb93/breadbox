package pages

import "breadbox/internal/templates/components"

// ConnectionNewProps mirrors the data map the old connection_new.html read
// off the layout's data map. The handler pre-resolves the user list into a
// flat {ID, Name} slice so the templ side stays free of pgtype/pgconv
// plumbing.
//
// HasPlaid / HasTeller drive the provider radio cards on the select step.
// When neither is true the page renders the "no providers configured"
// empty state instead of the wizard. TellerEnv is interpolated into the
// inline <script> so TellerConnect.setup picks up sandbox/development/
// production from the running config.
type ConnectionNewProps struct {
	Breadcrumbs []components.Breadcrumb
	Users       []ConnectionNewUser
	CSRFToken   string
	HasPlaid    bool
	HasTeller   bool
	TellerEnv   string
}

// ConnectionNewUser is the flat view of a User the family-member <select>
// needs. ID is the formatted UUID string (already run through
// pgconv.FormatUUID).
type ConnectionNewUser struct {
	ID   string
	Name string
}
