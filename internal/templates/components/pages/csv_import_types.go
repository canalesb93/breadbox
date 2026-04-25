package pages

import "breadbox/internal/templates/components"

// CSVImportProps mirrors the data map the old csv_import.html read off the
// layout's data map. The handler pre-resolves the user list into a flat
// {ID, Name} slice so the templ side stays free of pgtype/pgconv plumbing.
//
// When ConnectionID is non-empty the page is in re-import mode: the family
// member + account-name fields are hidden and replaced by an info row that
// shows the existing connection's name + owner.
type CSVImportProps struct {
	Breadcrumbs            []components.Breadcrumb
	Users                  []CSVImportUser
	CSRFToken              string
	ConnectionID           string
	ExistingConnectionName string
	ExistingUserID         string
	ExistingUserName       string
}

// CSVImportUser is the flat view of a User the family-member <select> needs.
// ID is the formatted UUID string (already run through pgconv.FormatUUID).
type CSVImportUser struct {
	ID   string
	Name string
}
