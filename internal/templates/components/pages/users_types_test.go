//go:build !headless && !lite

package pages

import "testing"

func TestGroupHouseholdMembers(t *testing.T) {
	rows := []UsersEnrichedRow{
		{ID: "1", Name: "Zara", HasLogin: true, LoginRole: "viewer"},
		{ID: "2", Name: "Bob", HasLogin: false},
		{ID: "3", Name: "Alice", HasLogin: true, LoginRole: "admin"},
		{ID: "4", Name: "Cara", HasLogin: true, LoginRole: "editor"},
		{ID: "5", Name: "Amy", HasLogin: false},
		{ID: "6", Name: "Adam", HasLogin: true, LoginRole: "admin"},
	}

	groups := groupHouseholdMembers(rows)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}

	// Access group comes first, ordered admin → editor → viewer, then name.
	if groups[0].Key != "access" {
		t.Fatalf("expected first group key 'access', got %q", groups[0].Key)
	}
	wantAccess := []string{"Adam", "Alice", "Cara", "Zara"} // two admins tie-break by name
	if got := names(groups[0].Rows); !equal(got, wantAccess) {
		t.Errorf("access order = %v, want %v", got, wantAccess)
	}

	// Profiles group second, ordered by name.
	if groups[1].Key != "profiles" {
		t.Fatalf("expected second group key 'profiles', got %q", groups[1].Key)
	}
	wantProfiles := []string{"Amy", "Bob"}
	if got := names(groups[1].Rows); !equal(got, wantProfiles) {
		t.Errorf("profiles order = %v, want %v", got, wantProfiles)
	}
}

func TestGroupHouseholdMembersDropsEmptyBuckets(t *testing.T) {
	// Only members with logins → a single 'access' group, no empty profiles.
	onlyAccess := groupHouseholdMembers([]UsersEnrichedRow{
		{Name: "A", HasLogin: true, LoginRole: "admin"},
	})
	if len(onlyAccess) != 1 || onlyAccess[0].Key != "access" {
		t.Fatalf("expected single access group, got %+v", onlyAccess)
	}

	// Only profiles → a single 'profiles' group.
	onlyProfiles := groupHouseholdMembers([]UsersEnrichedRow{
		{Name: "A", HasLogin: false},
	})
	if len(onlyProfiles) != 1 || onlyProfiles[0].Key != "profiles" {
		t.Fatalf("expected single profiles group, got %+v", onlyProfiles)
	}

	if got := groupHouseholdMembers(nil); len(got) != 0 {
		t.Fatalf("expected no groups for empty input, got %+v", got)
	}
}

func TestHouseholdRoleLabel(t *testing.T) {
	cases := map[string]string{
		"admin":  "Admin",
		"editor": "Editor",
		"viewer": "Viewer",
		"":       "",
		"custom": "Custom",
	}
	for in, want := range cases {
		if got := householdRoleLabel(in); got != want {
			t.Errorf("householdRoleLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func names(rows []UsersEnrichedRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Name
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
