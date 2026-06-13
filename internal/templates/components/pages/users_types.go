//go:build !headless && !lite

package pages

import (
	"fmt"
	"sort"
	"strings"
)

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
// chain in the prior html/template version. Solid (not soft) so the tone
// stays visible in dark mode, where soft primary/secondary collapse to
// invisible grays.
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

// householdRoleLabel title-cases a login role for the row badge. Empty
// stays empty (the row renders no badge for a profile without a login).
func householdRoleLabel(role string) string {
	switch role {
	case "admin":
		return "Admin"
	case "editor":
		return "Editor"
	case "viewer":
		return "Viewer"
	case "":
		return ""
	default:
		return strings.ToUpper(role[:1]) + role[1:]
	}
}

// householdRoleRank orders sign-in roles from most to least privileged so
// an admin always sorts above an editor above a viewer within the access
// group. Unknown / empty roles sink last.
func householdRoleRank(role string) int {
	switch role {
	case "admin":
		return 0
	case "editor":
		return 1
	case "viewer":
		return 2
	default:
		return 3
	}
}

// HouseholdMemberGroup is one labelled bucket on the Household page: the
// members who can sign in (ordered by privilege) and the attribution-only
// profiles (no login). Pinning the IA in a typed, testable helper keeps the
// grouping decision honest rather than vibes (design-system principle #5).
type HouseholdMemberGroup struct {
	Key   string // "access" | "profiles"
	Label string
	Rows  []UsersEnrichedRow
}

// groupHouseholdMembers partitions members into the sign-in-access group
// (HasLogin) and the profiles group (no login), drops any empty bucket, and
// orders the access group by role rank then name, profiles by name. The
// access group always precedes profiles. Pure + deterministic so the page's
// information architecture is unit-pinned.
func groupHouseholdMembers(rows []UsersEnrichedRow) []HouseholdMemberGroup {
	var access, profiles []UsersEnrichedRow
	for _, r := range rows {
		if r.HasLogin {
			access = append(access, r)
		} else {
			profiles = append(profiles, r)
		}
	}

	sort.SliceStable(access, func(i, j int) bool {
		ri, rj := householdRoleRank(access[i].LoginRole), householdRoleRank(access[j].LoginRole)
		if ri != rj {
			return ri < rj
		}
		return strings.ToLower(access[i].Name) < strings.ToLower(access[j].Name)
	})
	sort.SliceStable(profiles, func(i, j int) bool {
		return strings.ToLower(profiles[i].Name) < strings.ToLower(profiles[j].Name)
	})

	var groups []HouseholdMemberGroup
	if len(access) > 0 {
		groups = append(groups, HouseholdMemberGroup{Key: "access", Label: "Sign-in access", Rows: access})
	}
	if len(profiles) > 0 {
		groups = append(groups, HouseholdMemberGroup{Key: "profiles", Label: "Profiles", Rows: profiles})
	}
	return groups
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

