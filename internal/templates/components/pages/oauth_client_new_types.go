package pages

import "breadbox/internal/templates/components"

// OAuthClientNewProps mirrors the data map the old oauth_client_new.html
// read off the layout's data map. The handler resolves the breadcrumb
// trail and CSRF token before calling the templ component.
type OAuthClientNewProps struct {
	Breadcrumbs []components.Breadcrumb
	CSRFToken   string
}
