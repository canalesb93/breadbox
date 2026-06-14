//go:build !headless && !lite

package pages

import "testing"

func TestGroupConnectionsByHealth(t *testing.T) {
	rows := []ConnectionsRow{
		{ID: "ok1", Status: "active", LastSyncStatus: "success"},
		{ID: "reauth1", Status: "pending_reauth"},
		{ID: "gone1", Status: "disconnected"},
		{ID: "err1", Status: "error"},
		{ID: "syncerr1", Status: "active", LastSyncStatus: "error"},
		{ID: "ok2", Status: "active", LastSyncStatus: ""}, // never synced is still healthy/active
	}

	groups := GroupConnectionsByHealth(rows)
	if len(groups) != 3 {
		t.Fatalf("got %d groups, want 3", len(groups))
	}

	// Fixed order: needs-attention → active → disconnected.
	wantKeys := []string{"needs_attention", "active", "disconnected"}
	for i, want := range wantKeys {
		if groups[i].Key != want {
			t.Errorf("group[%d].Key = %q, want %q", i, groups[i].Key, want)
		}
	}

	// Needs-attention bucket: reauth + error + active-with-failed-sync, in
	// input order.
	na := groups[0]
	if got := rowIDs(na.Rows); !equalStrings(got, []string{"reauth1", "err1", "syncerr1"}) {
		t.Errorf("needs_attention rows = %v, want [reauth1 err1 syncerr1]", got)
	}

	// Active bucket: the two healthy connections.
	if got := rowIDs(groups[1].Rows); !equalStrings(got, []string{"ok1", "ok2"}) {
		t.Errorf("active rows = %v, want [ok1 ok2]", got)
	}

	// Disconnected bucket: just the disconnected one.
	if got := rowIDs(groups[2].Rows); !equalStrings(got, []string{"gone1"}) {
		t.Errorf("disconnected rows = %v, want [gone1]", got)
	}
}

func TestGroupConnectionsByHealthOmitsEmptyBuckets(t *testing.T) {
	// Only healthy connections → a single "active" group, no empty headers.
	rows := []ConnectionsRow{
		{ID: "a", Status: "active", LastSyncStatus: "success"},
		{ID: "b", Status: "active"},
	}
	groups := GroupConnectionsByHealth(rows)
	if len(groups) != 1 {
		t.Fatalf("got %d groups, want 1", len(groups))
	}
	if groups[0].Key != "active" || groups[0].Label != "Active" {
		t.Errorf("group = %+v, want key=active label=Active", groups[0])
	}
}

func TestConnectionStatusTone(t *testing.T) {
	cases := []struct {
		row  ConnectionsRow
		want string
	}{
		{ConnectionsRow{Status: "active", LastSyncStatus: "success"}, "success"},
		{ConnectionsRow{Status: "active"}, "success"},
		{ConnectionsRow{Status: "active", LastSyncStatus: "error"}, "warning"},
		{ConnectionsRow{Status: "pending_reauth"}, "warning"},
		{ConnectionsRow{Status: "error"}, "error"},
		{ConnectionsRow{Status: "disconnected"}, "neutral"},
	}
	for _, c := range cases {
		if got := connectionStatusTone(c.row); got != c.want {
			t.Errorf("connectionStatusTone(%+v) = %q, want %q", c.row, got, c.want)
		}
	}
}

func rowIDs(rows []ConnectionsRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.ID
	}
	return out
}

func equalStrings(a, b []string) bool {
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
