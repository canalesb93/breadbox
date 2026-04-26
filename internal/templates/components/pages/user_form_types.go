package pages

import (
	"breadbox/internal/db"
	"breadbox/internal/templates/components"
)

// UserFormProps mirrors the data map the old user_form.html read: the
// edit/create mode flag, the user record (in edit mode), the user's UUID
// (for avatar endpoints), and the breadcrumb trail.
type UserFormProps struct {
	IsEdit      bool
	User        *db.User
	UserID      string
	Breadcrumbs []components.Breadcrumb
}

// userFormName returns the user's name or empty string.
func userFormName(u *db.User) string {
	if u == nil {
		return ""
	}
	return u.Name
}

// userFormEmail returns the user's email (empty when absent or invalid).
func userFormEmail(u *db.User) string {
	if u == nil || !u.Email.Valid {
		return ""
	}
	return u.Email.String
}

// userFormHasAvatar reports whether the user has a custom avatar uploaded.
func userFormHasAvatar(u *db.User) bool {
	return u != nil && len(u.AvatarData) > 0
}

// userFormHasAvatarStr renders userFormHasAvatar as the literal "true" or
// "false" string for hand-off to the Alpine `avatarEditor` factory via a
// data-* attribute.
func userFormHasAvatarStr(u *db.User) string {
	if userFormHasAvatar(u) {
		return "true"
	}
	return "false"
}
