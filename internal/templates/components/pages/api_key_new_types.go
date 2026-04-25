package pages

import "breadbox/internal/templates/components"

// APIKeyNewProps mirrors the data map the old api_key_new.html read off
// the layout's data map. The handler resolves the breadcrumb trail and
// CSRF token before calling the templ component.
type APIKeyNewProps struct {
	Breadcrumbs []components.Breadcrumb
	CSRFToken   string
}
