package pages

import "breadbox/internal/templates/components"

// APIKeyCreatedProps mirrors the data map the old api_key_created.html
// read off the layout's data map. The handler pops the plaintext key
// from the session before calling the templ component (it can only
// be displayed once).
type APIKeyCreatedProps struct {
	Breadcrumbs  []components.Breadcrumb
	KeyName      string
	PlaintextKey string
}
