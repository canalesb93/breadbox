package pages

import "time"

// FeedProps is the full view model for the new household Feed page — a
// timeline-style replacement candidate for the legacy stats dashboard. The
// page surfaces sync runs, agent reports, transaction comments, rule-driven
// auto-categorisation batches, and per-transaction recategorisations as a
// chronological stream of mixed-weight cards on a single GitHub-style rail.
type FeedProps struct {
	CSRFToken string

	// Hero is the at-a-glance band rendered above the rail — today's
	// snapshot (events, new transactions, last sync, unread reports) plus a
	// one-line generated-at label.
	Hero FeedHero

	// ConnectionAlerts pin to the very top of the page (above the rail) so
	// connections in error/pending_reauth state are impossible to miss when
	// the rest of the feed is dominated by routine syncs.
	ConnectionAlerts []FeedAlert

	// Days is the day-bucketed feed body. Each FeedDay contains its rendered
	// label ("Today", "Yesterday", "Mar 14") and the items that occurred in
	// that bucket, ordered newest-first within the bucket.
	Days []FeedDay

	// TotalItems is the count rendered next to the section heading; useful
	// when the page is dense and the user wants a quick "how much is here"
	// signal before scrolling.
	TotalItems int

	// Now is the request-level time anchor threaded through to the relative-
	// time helpers so day-bucket labels and per-row timestamps share a single
	// reference and never disagree across midnight.
	Now time.Time

	// Filter is the active scope ("" = all, "agents", "household", "syncs",
	// "reports") parsed from the ?filter= query param. The page renders the
	// filter chips with the matching one highlighted; the actual filtering
	// is applied at the handler layer for now.
	Filter string
}

// FeedHero powers the at-a-glance band above the feed rail. Numeric fields
// are derived from today's items; string fields ("Generated") are formatted
// in the handler to keep the templ side free of time arithmetic.
type FeedHero struct {
	Generated            string
	EventsToday          int
	NewTransactionsToday int
	CommentsToday        int
	RuleHitsToday        int
	UnreadReports        int

	LastSyncAt          time.Time
	LastSyncRel         string // human-readable "5 minutes ago"
	LastSyncStatus      string
	LastSyncInstitution string
}

// FeedAlert is one pinned warning rendered above the feed rail. Each alert
// corresponds to a connection currently in error or pending_reauth state.
type FeedAlert struct {
	ConnectionID        string
	Institution         string
	Provider            string
	Status              string // "error" | "pending_reauth"
	ErrorMessage        string
	LastSyncedAt        string // relative time string
	ConsecutiveFailures int
}

// FeedDay is one day-bucket of feed items. The day separator on the rail
// renders the Label; the Items render below it newest-first.
type FeedDay struct {
	Key   string // YYYY-MM-DD — used for stable test snapshots and DOM ids.
	Label string // "Today" / "Yesterday" / "Jan 2"
	Items []FeedItem
	First bool
}

// FeedItem is a single rail row. Type drives the renderer branch; the union
// pointers carry the per-type payload. Exactly one of the pointers is set
// for any given Type; the rest are nil.
type FeedItem struct {
	Type         string    // "sync" | "report" | "comment" | "tag" | "category" | "rule_batch"
	Timestamp    time.Time // sortable wall-clock time
	TimestampStr string    // RFC3339 for the templ row's relative-time helper

	Sync     *FeedSync
	Report   *FeedReport
	Comment  *FeedComment
	Tag      *FeedTagChange
	Category *FeedCategoryChange
	RuleBatch *FeedRuleBatch
}

// FeedTransactionRef is the small card-within-a-card that every transaction-
// anchored row uses to anchor the event to its source ("$87.42 at Whole
// Foods · Chase Sapphire").
type FeedTransactionRef struct {
	ShortID      string
	Name         string
	MerchantName string
	Amount       float64
	Currency     string
	Date         string
	AccountName  string
	Institution  string
}

// FeedSync is the sync-card payload — counts + status + provider/institution
// labels for one sync_logs row.
type FeedSync struct {
	SyncLogID       string
	InstitutionName string
	Provider        string
	Trigger         string
	Status          string
	AddedCount      int
	ModifiedCount   int
	RemovedCount    int
	RuleHits        int
	ErrorMessage    string
}

// FeedReport is the rich agent-report card payload.
type FeedReport struct {
	ID            string
	ShortID       string
	Title         string
	BodyExcerpt   string
	Priority      string // "info" | "warning" | "critical"
	Tags          []string
	DisplayAuthor string
	IsUnread      bool
}

// FeedComment carries everything the comment-bubble row needs to render in
// the global feed (actor + content + linked transaction).
type FeedComment struct {
	ActorName          string
	ActorType          string
	ActorID            string
	ActorAvatarVersion string
	Content            string
	Transaction        FeedTransactionRef
}

// FeedTagChange carries one tag_added / tag_removed event.
type FeedTagChange struct {
	ActorName          string
	ActorType          string
	ActorID            string
	ActorAvatarVersion string
	Action             string // "added" | "removed"
	TagSlug            string
	TagName            string
	TagColor           string
	TagIcon            string
	Note               string
	Transaction        FeedTransactionRef
}

// FeedCategoryChange carries one manual category_set event.
type FeedCategoryChange struct {
	ActorName          string
	ActorType          string
	ActorID            string
	ActorAvatarVersion string
	CategorySlug       string
	CategoryName       string
	CategoryColor      string
	CategoryIcon       string
	Transaction        FeedTransactionRef
}

// FeedRuleBatch is the collapsed (rule, day)-grouped rule_applied card. The
// per-transaction rows fold into one with a count, sample list, and the
// breakdown of action fields ("tag" / "category" / etc).
type FeedRuleBatch struct {
	RuleName     string
	RuleShortID  string
	Count        int
	ActionFields map[string]int // "tag" -> N, "category" -> M
	Samples      []FeedTransactionSample
	DayLabel     string
	LatestTS     time.Time
}

// FeedTransactionSample is one expandable sample row inside a rule batch
// card.
type FeedTransactionSample struct {
	ShortID      string
	MerchantName string
	Amount       float64
	Currency     string
}
