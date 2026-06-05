//go:build !headless && !lite

package pages


// ConnectionNewProps mirrors the data map the old connection_new.html read
// off the layout's data map. The handler pre-resolves the user list into a
// flat {ID, Name} slice so the templ side stays free of pgtype/pgconv
// plumbing.
//
// HasPlaid / HasTeller / HasSimpleFin drive the provider radio cards on the
// select step. When none is true the page renders the "no providers
// configured" empty state instead of the wizard. TellerEnv is interpolated
// into the inline <script> so TellerConnect.setup picks up sandbox/
// development/production from the running config. SimpleFIN is token-paste
// (no SDK): selecting it reveals an inline setup-token form instead of
// launching a provider popup.
type ConnectionNewProps struct {
	Users        []ConnectionNewUser
	CSRFToken    string
	HasPlaid     bool
	HasTeller    bool
	HasSimpleFin bool
	TellerEnv    string
}

// ConnectionNewUser is the flat view of a User the family-member <select>
// needs. ID is the formatted UUID string (already run through
// pgconv.FormatUUID).
type ConnectionNewUser struct {
	ID   string
	Name string
}
