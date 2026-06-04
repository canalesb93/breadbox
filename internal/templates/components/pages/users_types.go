//go:build !headless && !lite

package pages

import "fmt"

// UsersProps mirrors the data map the old users.html read off the layout.
// Kept flat so admin/users.go can build it via a small mapper.
type UsersProps struct {
	CSRFToken     string
	Created       bool
	IsUnlinked    bool
	UnlinkedUsers []UsersUnlinkedRow
	EnrichedUsers []UsersEnrichedRow
}

// UsersUnlinkedRow is one option in the "Link Existing" select on the
// admin-link prompt panel.
type UsersUnlinkedRow struct {
	ID   string // formatted UUID
	Name string
}

// UsersEnrichedRow is one household-member card on the page. Mirrors the
// admin.EnrichedUser fields needed for display, flattened to Go primitives
// so the templ side stays decoupled from pgtype.
type UsersEnrichedRow struct {
	ID                string // formatted UUID
	Name              string
	Email             string
	HasEmail          bool
	AvatarVersion     string // updated_at unix timestamp; threaded through UserAvatar's Version prop
	AccountCount      int
	ConnectionCount   int64
	HasCreatedAt      bool
	CreatedAtLabel    string // "Jan 2006"
	HasLogin          bool
	LoginRole         string // "admin", "editor", "viewer" — empty when no login
	LoginUsername     string // auth_accounts.username — typically the email-style admin login
	LoginSetupPending bool
}

// usersLoginRoleBadgeClass returns the DaisyUI badge color modifier for a
// login-account role. Mirrors the {{if eq $la.Role "admin"}}badge-primary
// {{else if eq $la.Role "editor"}}badge-secondary{{else}}badge-ghost
// chain in the prior html/template version.
func usersLoginRoleBadgeClass(role string) string {
	switch role {
	case "admin":
		return "badge-primary"
	case "editor":
		return "badge-secondary"
	default:
		return "badge-ghost"
	}
}

// usersAccountsSummary renders the "<N> account(s) across <M> connection(s)"
// caption at the top of an account list. Pluralization mirrors the old
// `{{if gt .AccountCount 1}}s{{end}}` inline template logic.
func usersAccountsSummary(accountCount int, connectionCount int64) string {
	acctsWord := "account"
	if accountCount > 1 {
		acctsWord = "accounts"
	}
	connsWord := "connection"
	if connectionCount > 1 {
		connsWord = "connections"
	}
	return fmt.Sprintf("%d %s across %d %s", accountCount, acctsWord, connectionCount, connsWord)
}

