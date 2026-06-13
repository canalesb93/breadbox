//go:build !headless && !lite

package pages

import "testing"

func TestGroupAccountsByConnection(t *testing.T) {
	rows := []AccountsListRow{
		// Chase: an asset + a liability → subtotal 11135.63.
		{ID: "a1", ConnectionShortID: "chase", InstitutionName: "Chase", ConnectionStatus: "active", HasBalance: true, BalanceFloat: 12340.18},
		{ID: "a2", ConnectionShortID: "chase", InstitutionName: "Chase", ConnectionStatus: "active", HasBalance: true, BalanceFloat: -1204.55, IsLiability: true},
		// Fidelity: a single big asset → should rank first.
		{ID: "a3", ConnectionShortID: "fidelity", InstitutionName: "Fidelity", ConnectionStatus: "active", HasBalance: true, BalanceFloat: 34120.00},
		// Amex: reauth, one liability.
		{ID: "a4", ConnectionShortID: "amex", InstitutionName: "American Express", ConnectionStatus: "pending_reauth", HasBalance: true, BalanceFloat: -842.30, IsLiability: true},
		// Orphan (connection deleted → SET NULL): no balance. Sinks last.
		{ID: "a5", ConnectionShortID: "", InstitutionName: "", HasBalance: false},
	}

	groups := GroupAccountsByConnection(rows)
	if len(groups) != 4 {
		t.Fatalf("got %d groups, want 4", len(groups))
	}

	// Order: Fidelity (34120) > Chase (11135.63) > Amex (-842.30) > orphan (last).
	wantOrder := []string{"fidelity", "chase", "amex", ""}
	for i, want := range wantOrder {
		if groups[i].ConnectionShortID != want {
			t.Errorf("group[%d] = %q, want %q", i, groups[i].ConnectionShortID, want)
		}
	}

	// Chase subtotal = 12340.18 - 1204.55 = 11135.63.
	var chase AccountsListConnGroup
	for _, g := range groups {
		if g.ConnectionShortID == "chase" {
			chase = g
		}
	}
	if !chase.HasSubtotal {
		t.Fatal("chase group should have a subtotal")
	}
	if got := chase.Subtotal; got < 11135.62 || got > 11135.64 {
		t.Errorf("chase subtotal = %.2f, want 11135.63", got)
	}
	if len(chase.Accounts) != 2 {
		t.Errorf("chase has %d accounts, want 2", len(chase.Accounts))
	}
	if chase.InstitutionName != "Chase" {
		t.Errorf("chase institution = %q, want Chase", chase.InstitutionName)
	}

	// Amex carries the connection status onto the group header.
	for _, g := range groups {
		if g.ConnectionShortID == "amex" && g.Status != "pending_reauth" {
			t.Errorf("amex status = %q, want pending_reauth", g.Status)
		}
	}

	// Orphan group: no connection, no subtotal, lands last.
	last := groups[len(groups)-1]
	if last.ConnectionShortID != "" || last.HasSubtotal {
		t.Errorf("last group = %+v, want orphan with no subtotal", last)
	}
	if got := accountsConnLabel(last); got != "Unlinked accounts" {
		t.Errorf("orphan label = %q, want \"Unlinked accounts\"", got)
	}
}

func TestAccountsConnCountLabel(t *testing.T) {
	one := AccountsListConnGroup{Accounts: []AccountsListRow{{}}}
	if got := accountsConnCountLabel(one); got != "1 account" {
		t.Errorf("got %q, want \"1 account\"", got)
	}
	three := AccountsListConnGroup{Accounts: []AccountsListRow{{}, {}, {}}}
	if got := accountsConnCountLabel(three); got != "3 accounts" {
		t.Errorf("got %q, want \"3 accounts\"", got)
	}
}
