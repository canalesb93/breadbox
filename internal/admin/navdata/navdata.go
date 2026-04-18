// Package navdata defines the prop types shared between the admin package
// and the sidebar nav templ component. Lives outside both packages to avoid
// an import cycle: admin → components → admin.
package navdata

// Badges holds sidebar notification counts.
type Badges struct {
	// PendingReviews is the count of transactions currently tagged
	// "needs-review". Displayed next to the Tags nav link.
	PendingReviews       int64
	ConnectionsAttention int64
	UnreadReports        int64
	ShowGettingStarted   bool
}

// Props is the full data surface consumed by the Nav component.
type Props struct {
	CurrentPage          string
	NavBadges            *Badges
	NavUpdateAvailable   bool
	NavLatestVersion     string
	NavLatestURL         string
	AppVersion           string
	IsAdmin              bool
	IsEditor             bool
	AdminUsername        string
	SessionUserID        string
	SessionAvatarVersion string
	RoleDisplay          string
	HasLinkedUser        bool
	CSRFToken            string
}
