//go:build !headless && !lite

package pages

import (
	"fmt"

	"breadbox/internal/templates/components"
)

// ConnectionsProps mirrors the data map the old connections.html read off
// the layout. Kept flat so admin/connections.go can copy fields one-to-one.
type ConnectionsProps struct {
	Tab       string // "connections" or "links"
	CSRFToken string

	// CanManage gates the destructive "Disconnect" row action — true for
	// admins (who alone can DELETE a connection). Sync + Configure are
	// available to every viewer who can see the connection.
	CanManage bool

	// Connection list-rows, grouped by health (needs-attention → active →
	// disconnected) at render time via GroupConnectionsByHealth.
	Connections []ConnectionsRow

	// Account links tab
	Links        []ConnectionsLinkRow
	LinkAccounts []ConnectionsLinkAccount

	// Connect-a-bank drawer (embedded on this page, opened from the
	// "Connect Bank" button / empty state). Mirrors ConnectionNewProps so
	// the shared connectWizard partial renders inside the drawer.
	Users        []ConnectionNewUser
	HasPlaid     bool
	HasTeller    bool
	HasSimpleFin bool
	TellerEnv    string
}

// ConnectInProps adapts a ConnectionsProps into the ConnectionNewProps the
// shared connectWizard partial expects.
func (p ConnectionsProps) ConnectInProps() ConnectionNewProps {
	return ConnectionNewProps{
		Users:        p.Users,
		CSRFToken:    p.CSRFToken,
		HasPlaid:     p.HasPlaid,
		HasTeller:    p.HasTeller,
		HasSimpleFin: p.HasSimpleFin,
		TellerEnv:    p.TellerEnv,
	}
}

// Groups buckets the flat connection list by health so the template can
// render needs-attention rows first under quiet label lines. Computed at
// render time (cheap: one pass over a bounded list) rather than threaded
// through the handler.
func (p ConnectionsProps) Groups() []ConnectionsStatusGroup {
	return GroupConnectionsByHealth(p.Connections)
}

// ConnectionsRow is one bank-connection list-row on the page.
type ConnectionsRow struct {
	ID                   string // formatted UUID
	UserID               string // formatted UUID
	Provider             string // "plaid" | "teller" | "simplefin" | "csv"
	Status               string // canonical connection status enum
	InstitutionName      string
	UserName             string
	Paused               bool
	IsStale              bool
	NewAccountsAvailable bool

	// Last-sync state (drives the body line + the sync-error health bucket).
	LastSyncStatus       string // "success" | "error" | "in_progress" | "" (none)
	LastSyncErrorMessage string // empty when no message
	LastSyncedAtValid    bool
	LastSyncedAtRelative string

	// Connection-level error (e.g. reauth)
	ErrorCodeValid    bool
	ErrorCode         string
	ErrorMessageValid bool

	// Per-connection totals
	HasBalance   bool
	TotalBalance float64
	AccountCount int64
}

// ConnectionsStatusGroup is one health bucket on the connections list — a
// quiet label line over its list-rows. Groups render in fixed priority
// order (needs-attention → active → disconnected); empty buckets are
// omitted by GroupConnectionsByHealth.
type ConnectionsStatusGroup struct {
	Key   string // "needs_attention" | "active" | "disconnected"
	Label string // human label for the group header
	Rows  []ConnectionsRow
}

// connectionHealthBucket classifies a connection into one of three buckets.
// A connection needs attention when it is errored, awaiting reauth, or
// active-but-its-last-sync-failed; disconnected is its own terminal bucket;
// everything else is healthy/active. Pure so the IA is unit-testable.
func connectionHealthBucket(c ConnectionsRow) string {
	switch {
	case c.Status == "disconnected":
		return "disconnected"
	case c.Status == "error" || c.Status == "pending_reauth":
		return "needs_attention"
	case c.Status == "active" && c.LastSyncStatus == "error":
		return "needs_attention"
	default:
		return "active"
	}
}

// connectionsGroupOrder pins the render order + labels of the buckets.
var connectionsGroupOrder = []struct{ Key, Label string }{
	{"needs_attention", "Needs attention"},
	{"active", "Active"},
	{"disconnected", "Disconnected"},
}

// GroupConnectionsByHealth buckets connections into needs-attention →
// active → disconnected, preserving input order within each bucket and
// omitting empty buckets. Pure function so the grouping IA is pinned by a
// unit test without a DB.
func GroupConnectionsByHealth(rows []ConnectionsRow) []ConnectionsStatusGroup {
	byKey := make(map[string][]ConnectionsRow, 3)
	for _, r := range rows {
		k := connectionHealthBucket(r)
		byKey[k] = append(byKey[k], r)
	}
	groups := make([]ConnectionsStatusGroup, 0, len(connectionsGroupOrder))
	for _, b := range connectionsGroupOrder {
		if rs := byKey[b.Key]; len(rs) > 0 {
			groups = append(groups, ConnectionsStatusGroup{Key: b.Key, Label: b.Label, Rows: rs})
		}
	}
	return groups
}

// connectionStatusTone maps a connection to one of the leading-tile tones.
// Only the vivid tones (success / warning / error) carry health; a
// disconnected connection is quiet neutral. Mirrors the badge-tone rule.
func connectionStatusTone(c ConnectionsRow) string {
	switch {
	case c.Status == "disconnected":
		return "neutral"
	case c.Status == "error":
		return "error"
	case c.Status == "pending_reauth":
		return "warning"
	case c.Status == "active" && c.LastSyncStatus == "error":
		return "warning"
	default:
		return "success"
	}
}

// connectionStatusIcon picks the leading-tile glyph for the connection's
// health. Distinct from tone so reauth (key) and a generic error (alert)
// read differently even though both are warning/error-toned.
func connectionStatusIcon(c ConnectionsRow) string {
	switch {
	case c.Status == "disconnected":
		return "unplug"
	case c.Status == "error":
		return "alert-circle"
	case c.Status == "pending_reauth":
		return "key-round"
	case c.Status == "active" && c.LastSyncStatus == "error":
		return "alert-triangle"
	default:
		return "circle-check"
	}
}

// connectionsShowStatusPill is true for any connection that warrants a
// status pill — i.e. it is not a healthy active connection. Healthy rows
// stay pill-free so the calm majority reads quietly (Mintlify-clean): the
// quiet provider tile + "synced Nh ago" body line carry them. Attention is
// signalled only where it is warranted.
func connectionsShowStatusPill(c ConnectionsRow) bool {
	return connectionHealthBucket(c) != "active"
}

// connectionStatusLabel is the short word shown in the row's status pill.
func connectionStatusLabel(c ConnectionsRow) string {
	switch {
	case c.Status == "disconnected":
		return "Disconnected"
	case c.Status == "error":
		return "Connection error"
	case c.Status == "pending_reauth":
		return "Reauth needed"
	case c.Status == "active" && c.LastSyncStatus == "error":
		return "Sync error"
	default:
		return "Connected"
	}
}

// connectionStatusBadgeClass maps a connection's health tone to the quiet
// daisy badge utility for its status pill. The vivid tones use badge-soft so
// the pill stays a small accent (never a fill); disconnected falls back to
// badge-ghost because a neutral-soft badge is invisible on the dark theme.
func connectionStatusBadgeClass(c ConnectionsRow) string {
	switch connectionStatusTone(c) {
	case "warning":
		return "badge-soft badge-warning"
	case "error":
		return "badge-soft badge-error"
	case "success":
		return "badge-soft badge-success"
	default:
		return "badge-ghost"
	}
}

// connectionsRowReason is the trailing clause of the quiet body line — the
// account count's companion. It states the connection's situation in muted
// ink; tone lives only in the small pill, never washed across the body. It
// doubles as the status pill's hover title so the full error detail the
// line truncates is still reachable.
func connectionsRowReason(c ConnectionsRow) string {
	switch {
	case c.ErrorMessageValid:
		return components.ErrorMessage(c.ErrorCode)
	case c.Status == "pending_reauth":
		return "Reauthentication needed"
	case c.Status == "active" && c.LastSyncStatus == "error":
		if c.LastSyncErrorMessage != "" {
			return "Last sync failed — " + c.LastSyncErrorMessage
		}
		return "Last sync failed"
	case c.Status == "disconnected":
		return "Disconnected"
	case c.LastSyncedAtValid:
		return "synced " + c.LastSyncedAtRelative
	default:
		return "Never synced"
	}
}

// connectionsGroupIcon returns a small leading lucide glyph for a health
// group header. Only the needs-attention bucket gets one (a quiet warning
// mark, a small accent — not a tinted rail); the rest read as plain-ink
// labels.
func connectionsGroupIcon(key string) string {
	if key == "needs_attention" {
		return "alert-triangle"
	}
	return ""
}

// connectionsNeedsAttentionCount counts connections in the needs-attention
// bucket, for the quiet at-a-glance subtitle line.
func connectionsNeedsAttentionCount(rows []ConnectionsRow) int {
	n := 0
	for _, r := range rows {
		if connectionHealthBucket(r) == "needs_attention" {
			n++
		}
	}
	return n
}

// connectionsAccountCountLabel renders "N accounts" for the body line,
// reusing the pluralizer in connections_scripts.go.
func connectionsAccountCountLabel(n int64) string {
	return fmt.Sprintf("%d %s", n, connectionsAccountSuffix(n, false))
}

// connectionsReauthURL returns the per-connection recovery target. Plaid /
// Teller / SimpleFIN reauth happens through the dedicated reauth page;
// everything else points at the detail page where recovery options live.
func connectionsReauthURL(c ConnectionsRow) string {
	if c.Provider != "csv" {
		return "/connections/" + c.ID + "/reauth"
	}
	return "/connections/" + c.ID
}

// ConnectionsLinkRow is one item on the Account Links tab.
type ConnectionsLinkRow struct {
	ID                      string
	PrimaryAccountName      string
	PrimaryUserName         string
	DependentAccountName    string
	DependentUserName       string
	Enabled                 bool
	MatchCount              int64
	UnmatchedDependentCount int64
	MatchStrategy           string
	MatchToleranceDays      int
}

// ConnectionsLinkAccount is one option in the create-link modal selects.
type ConnectionsLinkAccount struct {
	ID              string
	DisplayName     string
	Mask            string
	UserName        string
	InstitutionName string
}
