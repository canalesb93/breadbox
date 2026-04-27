package pages

import "breadbox/internal/templates/components"

// OAuthClientCreatedProps mirrors the data map the old
// oauth_client_created.html read off the layout's data map. The handler
// pops the client ID, secret, and name from the session before calling
// the templ component (the secret can only be displayed once).
type OAuthClientCreatedProps struct {
	Breadcrumbs  []components.Breadcrumb
	ClientName   string
	ClientID     string
	ClientSecret string
	MCPServerURL string
}
