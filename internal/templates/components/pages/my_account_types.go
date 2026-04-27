package pages

// MyAccountProps mirrors the data map the old my_account.html read. The
// handler builds these in admin/members.go and renders via
// TemplateRenderer.RenderWithTempl.
type MyAccountProps struct {
	UserID     string
	IsUnlinked bool
	CSRFToken  string

	// Linked household-member profile fields, surfaced inside the
	// "Profile" card that mirrors /settings/household/{id}/edit visually
	// but POSTs to /settings/account/profile so non-admins can self-edit.
	UserName        string
	UserEmail       string
	HasCustomAvatar bool

	// Identity surfaced in the page header alongside the sign-out button.
	AdminUsername        string
	RoleDisplay          string
	SessionUserID        string
	SessionAvatarVersion string
}

