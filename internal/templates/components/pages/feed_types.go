package pages

import "time"

// FeedProps is the full view model for the home Feed page. The page
// surfaces sync runs, agent reports, MCP agent sessions, bulk-action
// bursts, and standalone comments as a chronological stream of mixed-
// weight cards on a single GitHub-style rail.
//
// The grouped-events shape (sync / agent_session / bulk_action / comment)
// is computed by `service.ListFeedEvents`; this view-model is a thin
// projection that rebinds the service shapes onto display-side types so
// the templ never has to import the service package.
type FeedProps struct {
	CSRFToken string

	// Hero is the at-a-glance band rendered above the rail.
	Hero FeedHero

	// ConnectionAlerts pin to the very top of the page (above the rail) so
	// connections in error/pending_reauth state are impossible to miss.
	ConnectionAlerts []FeedAlert

	// Days is the day-bucketed feed body. Each FeedDay contains its
	// rendered label ("Today", "Yesterday", "Jan 2") and the items that
	// occurred in that bucket, ordered newest-first within the bucket.
	Days []FeedDay

	// TotalItems is the count rendered next to the section heading.
	TotalItems int

	// WindowDays is rendered as a footer note ("showing the last 3 days").
	WindowDays int

	// Now is the request-level time anchor used for relative-time helpers.
	Now time.Time

	// Filter is the active scope ("" = all, "syncs", "reports", "comments",
	// "agents", "me") parsed from the ?filter= query param. Renders chip
	// state only — actual filtering applies at the handler layer.
	Filter string

	// HasConnections is true when at least one bank connection exists,
	// regardless of status. Drives the empty-state branch — first-run
	// (no connections) gets a different copy + CTA than "quiet around
	// here" (has connections, no events in window).
	HasConnections bool

	// LastSyncAt is the most recent successful sync timestamp across the
	// household, mirrored from FeedHero so the empty-state copy can
	// render "Last sync was {relative-time}" without re-deriving it.
	// Zero value means "no sync yet".
	LastSyncAt time.Time

	// IsAdmin is true when the current session has the admin role. The
	// "Sync now" empty-state CTA POSTs to /-/connections/sync-all which
	// is admin-only; non-admin members see the link to /transactions
	// instead.
	IsAdmin bool

	// OldestVisible is the timestamp of the earliest event currently in the
	// rendered window. Drives the "Load older activity" button's href —
	// `?before=<oldestVisible.RFC3339>` rolls the window backward in
	// `WindowDays`-sized chunks. Zero value means the rail is empty (no
	// button should render).
	OldestVisible time.Time

	// AtMaxLookback is true when the oldest visible event is at-or-past the
	// service-layer 30-day lookback cap. The footer renders an "End of
	// feed" sentence instead of the load-older button so users have a
	// clear stop signal.
	AtMaxLookback bool

	// Filter is forwarded into load-older hrefs so an active chip survives
	// pagination. Already exists above; documented here as part of the
	// pagination contract.
}

// FeedHero powers the at-a-glance band above the feed rail.
type FeedHero struct {
	Generated            string
	EventsToday          int
	NewTransactionsToday int
	CommentsToday        int
	RuleHitsToday        int
	UnreadReports        int

	LastSyncAt          time.Time
	LastSyncRel         string
	LastSyncStatus      string
	LastSyncInstitution string

	// NextSyncRel is a human-readable countdown to the next scheduled cron
	// fire (e.g. "in ~6h", "in 12m"). Empty when the scheduler is unset
	// (test env) or when the connection is failing — in the failing case
	// we suppress it because the next tick won't help until reauth.
	NextSyncRel string
}

// FeedAlert is one pinned warning for a connection in error /
// pending_reauth state.
type FeedAlert struct {
	ConnectionID        string
	Institution         string
	Provider            string
	Status              string
	ErrorMessage        string
	LastSyncedAt        string
	ConsecutiveFailures int
}

// FeedDay is one day-bucket of feed items.
type FeedDay struct {
	Key   string
	Label string
	Items []FeedItem
	First bool
}

// FeedItem is a single rail row. Type drives the renderer branch; the
// union pointers carry the per-type payload. Exactly one is set per row.
type FeedItem struct {
	Type         string
	Timestamp    time.Time
	TimestampStr string

	Sync         *FeedSync
	Report       *FeedReport
	Comment      *FeedComment
	AgentSession *FeedAgentSession
	BulkAction   *FeedBulkAction
}

// FeedTransactionRef is the small card-within-a-card that anchors any
// transaction-anchored row to its source ("$87.42 at Whole Foods · Chase").
type FeedTransactionRef struct {
	ShortID      string
	Name         string
	MerchantName string
	Amount       float64
	Currency     string
	Date         string
	AccountName  string
	Institution  string
	// Pending mirrors `transactions.pending`. When true the inline tx-ref
	// row renders the same small clock-icon mark used on the per-tx page
	// and the transactions list, signalling the row is still preliminary
	// (provider may re-issue it as posted later).
	Pending bool

	// Category presentation, mirrored from `tx_row_compact.templ`. When the
	// underlying transaction is categorised the inline tx-ref row renders
	// the same coloured-circle avatar used on /transactions; nil falls back
	// to a neutral letter avatar so feed sample rows match the list layout.
	CategoryDisplayName *string
	CategoryColor       *string
	CategoryIcon        *string
	CategorySlug        *string

	// TagCount is the current number of tags on the transaction. Drives
	// the small `[tag] N` chip on the feed one-liner when > 0.
	TagCount int
}

// FeedSync is the sync-card payload. Inline transaction samples and rule
// outcomes are rendered directly inside the card so a sync card answers
// "what came in, and what did Breadbox do with it" in one glance.
type FeedSync struct {
	SyncLogID       string
	InstitutionName string
	Provider        string
	Trigger         string
	Status          string
	ErrorMessage    string

	AddedCount    int
	ModifiedCount int
	RemovedCount  int

	StartedAt time.Time

	// RetryCount + FirstFailureAt are populated only on error syncs that
	// folded N earlier same-error retry attempts; the card renders them
	// as "Failing for 18h · 49 attempts" so a flapping connection takes
	// up one rail row instead of dozens.
	RetryCount     int
	FirstFailureAt time.Time

	SampleTransactions []FeedTransactionRef
	AdditionalCount    int

	RuleOutcomes []FeedRuleOutcome
}

// FeedRuleOutcome is one (rule, count) line inside a sync card.
type FeedRuleOutcome struct {
	RuleName    string
	RuleShortID string
	Count       int
}

// FeedReport is the rich agent-report card payload.
type FeedReport struct {
	ID            string
	ShortID       string
	Title         string
	BodyExcerpt   string
	Priority      string
	Tags          []string
	DisplayAuthor string
	IsUnread      bool
}

// FeedComment carries one standalone comment row.
type FeedComment struct {
	// CommentShortID is the annotation's short_id, used by feed.templ to
	// build a `/transactions/{tx}#comment-{id}` link so the row's verb
	// jumps to the matching comment on the per-tx timeline.
	CommentShortID     string
	ActorName          string
	ActorType          string
	ActorID            string
	ActorAvatarVersion string
	Content            string
	Transaction        FeedTransactionRef
}

// FeedAgentSession is the rich card that collapses every annotation in
// one MCP session into a single row.
type FeedAgentSession struct {
	SessionID      string
	SessionShortID string
	APIKeyName     string
	Purpose        string

	ActorName          string
	ActorType          string
	ActorID            string
	ActorAvatarVersion string

	StartedAt time.Time
	EndedAt   time.Time

	AnnotationCount    int
	UniqueTransactions int

	// Counts breaks the session's annotations down by category-of-action
	// for the inline summary line ("23 categorised · 12 tagged · 8
	// commented · 4 rules applied"). Order is fixed by the templ.
	Categorised    int
	Tagged         int
	UntaggedRemoved int
	Commented      int
	RuleApplied    int

	SampleTransactions []FeedTransactionRef

	// Report fields are populated when the service folded an agent_report
	// whose `session_id` matches this session into the card. The templ
	// uses ReportTitle as the headline (instead of the generic "ran a
	// session" line) and renders priority badges + tags inline. Empty
	// when no report was folded — render the plain session card.
	ReportID       string
	ReportShortID  string
	ReportTitle    string
	ReportPriority string
	ReportTags     []string
	ReportIsUnread bool
}

// FeedBulkAction is the rich card that collapses ≥3 same-actor
// annotations from a 15-minute bucket into a single row. As of iteration
// 13 the bucket key is (actor, time-window) only — `kind` is absent — so
// a single agent run that categorises some rows, removes a tag from
// others, and updates more in the same window collapses into ONE
// bulk_action card. KindCounts surfaces the per-kind breakdown for inline
// rendering ("5 categorised · 8 tag-removed · 8 updated").
type FeedBulkAction struct {
	ActorName          string
	ActorType          string
	ActorID            string
	ActorAvatarVersion string

	// Kind is "mixed" when the bucket spans multiple kinds; otherwise the
	// single homogeneous kind. The templ branches on "mixed" to render
	// the per-kind breakdown line in lieu of a dedicated verb phrasing.
	Kind  string
	Count int

	// KindCounts breaks the bucket down by kind. Always populated; the
	// templ renders it as the "5 categorised · 8 tag-removed · 8 updated"
	// breakdown when Kind == "mixed", or skips it for homogeneous
	// buckets (the dedicated verb phrasing already conveys the kind).
	KindCounts map[string]int

	Subjects []FeedBulkSubject

	StartedAt time.Time
	EndedAt   time.Time

	SampleTransactions []FeedTransactionRef

	// Report fields are populated when the service folded a matching
	// agent_report into the card (see service.foldReportsIntoEvents). The
	// templ uses ReportTitle as the bold headline instead of the generic
	// "Alice updated N transactions" line and renders priority badges +
	// tags inline. Empty when no report was folded.
	ReportID       string
	ReportShortID  string
	ReportTitle    string
	ReportPriority string
	ReportTags     []string
	ReportIsUnread bool
}

// FeedBulkSubject is one (subject, count) chip inside a bulk-action card.
type FeedBulkSubject struct {
	Name  string
	Slug  string
	Color string
	Icon  string
	Count int
}
