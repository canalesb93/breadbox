//go:build !headless && !lite

package admin

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"breadbox/internal/app"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	"breadbox/internal/templates/components/pages"
)

// feedWindowDays is the bounded lookback window for the home Feed page.
// Events older than this drop off; the cap ensures the page never grows
// unbounded and that the day buckets stay interpretable. Tuned with the
// product owner — three days lines up with "what's happened this weekend".
const feedWindowDays = 3

// FeedHandler serves GET / — the activity-style household home page.
// The Getting Started guide stays linked from the sidebar (driven by
// `onboarding_dismissed`) so first-run users can still reach it, but the
// root path no longer bounces there. Post-setup landing happens once,
// from the setup handler — see CreateAdminHandler in setup.go.
//
// The aggregation pipeline is:
//
//  1. service.ListFeedEvents returns already-grouped sync / agent_session /
//     bulk_action / comment events for the last `feedWindowDays`.
//  2. Reports + connection alerts are pulled here (unrelated to grouping).
//  3. We project FeedEvents onto the templ-side `pages.FeedItem` shape,
//     resolve tag/category display names for bulk-action subjects, and
//     bucket the merged stream by day for the rail's day separators.
func FeedHandler(a *app.App, svc *service.Service, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Anchor every wall-clock decision (day buckets, "Today" hero
		// counters, absolute-time tooltips) to the viewer's browser
		// timezone via the bb_tz cookie. Falls back to server local when
		// the cookie isn't set yet — see `UserLocation`.
		loc := UserLocation(r)
		now := time.Now().In(loc)
		window := time.Duration(feedWindowDays) * 24 * time.Hour
		filter := strings.TrimSpace(r.URL.Query().Get("filter"))

		// Per-query timings, emitted as a single structured slog line at the
		// end of the request so we can spot regressions in production logs.
		// The reports/events/category/tag timings come back from
		// feedWindowItems; the alerts query is timed inline below.
		reqStart := time.Now()
		var qAlerts time.Duration

		// `?before=<rfc3339>` lets the user roll the 3-day window backward in
		// chunks via the "Load older activity" footer. Parsed + clamped to the
		// 30-day lookback ceiling here and again in the service layer (defence
		// in depth). The home feed's inline pagination (FeedRowsHandler) shares
		// the same parse helper so its cursor honours the identical bounds.
		beforeTime := parseFeedBefore(r.URL.Query().Get("before"), now)

		// Resolve the session actor for the "me" chip. Prefer the linked
		// household user_id; fall back to the auth_account id for the
		// initial admin (which writes annotations against its account id).
		// If neither resolves we silently downgrade to "All" rather than
		// erroring — see filterFeedEvents.
		var sessionActorID string
		if filter == "me" {
			sessionActorID = SessionUserID(tr.sm, r)
			if sessionActorID == "" {
				sessionActorID = SessionAccountID(tr.sm, r)
			}
			if sessionActorID == "" {
				filter = ""
			}
		}

		// Aggregate the window: agent reports + grouped events projected onto
		// FeedItems, newest-first. FeedRowsHandler runs the same helper so the
		// inline "Load older activity" rows are byte-identical to a full page
		// render. `reports` is the windowed report list, kept here only for the
		// unread-count hero tile below.
		items, reports, timings, err := feedWindowItems(ctx, a, svc, feedQuery{
			now:            now,
			window:         window,
			before:         beforeTime,
			filter:         filter,
			sessionActorID: sessionActorID,
		})
		if err != nil {
			// Already logged inside the helper. Render with whatever items came
			// back (nil → empty state) rather than 500-ing the home page.
			items = nil
		}

		// Connection alerts + empty-state meta — current state, not windowed.
		// Alerts hide under any active chip so the filtered view is exclusively
		// the chip's scope; they re-appear on the unfiltered "All" page. The
		// meta (hasConnections + global last-sync-at) is always collected so the
		// empty-state branch can pick the right copy regardless of filter.
		t0 := time.Now()
		alerts, hasConnections, globalLastSyncAt := buildFeedConnectionMeta(ctx, a)
		qAlerts = time.Since(t0)
		if filter != "" {
			alerts = nil
		}

		// Hero-stat collection over the projected items. Sync items carry the
		// same AddedCount / RuleOutcomes / Status / StartedAt the raw events do,
		// so the hero tiles derive entirely from `items`; reports drive the
		// unread count below.
		startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		var lastSyncAt time.Time
		var lastSyncStatus, lastSyncInstitution string
		var totalNewTransactionsToday, ruleHitsToday int
		var eventsToday, commentsToday int
		for _, it := range items {
			if it.Timestamp.After(startOfDay) {
				eventsToday++
				if it.Type == "comment" {
					commentsToday++
				}
			}
			if it.Type == "sync" && it.Sync != nil {
				if it.Sync.StartedAt.After(lastSyncAt) {
					lastSyncAt = it.Sync.StartedAt
					lastSyncStatus = it.Sync.Status
					lastSyncInstitution = it.Sync.InstitutionName
				}
				if it.Timestamp.After(startOfDay) {
					totalNewTransactionsToday += it.Sync.AddedCount
					for _, ro := range it.Sync.RuleOutcomes {
						ruleHitsToday += ro.Count
					}
				}
			}
		}
		days := groupFeedByDay(items, now, loc)

		var unreadReports int
		for _, rep := range reports {
			if rep.ReadAt == nil {
				unreadReports++
			}
		}
		hero := pages.FeedHero{
			Generated:            now.Format("Mon, Jan 2"),
			EventsToday:          eventsToday,
			NewTransactionsToday: totalNewTransactionsToday,
			CommentsToday:        commentsToday,
			RuleHitsToday:        ruleHitsToday,
			UnreadReports:        unreadReports,
			LastSyncAt:           lastSyncAt,
			LastSyncStatus:       lastSyncStatus,
			LastSyncInstitution:  lastSyncInstitution,
		}
		if !lastSyncAt.IsZero() {
			hero.LastSyncRel = relativeTime(lastSyncAt)
		}
		// Next-sync ETA — drives the "Next sync in ~6h" sub-line under the
		// Last Sync hero tile so the page answers "why no new transactions
		// yet?" inline. The scheduler is nil in test env (no cron); leave
		// the field empty there and the templ hides the sub-line.
		if a.Scheduler != nil {
			hero.NextSyncRel = formatNextSync(a.Scheduler.NextRun())
		}

		data := map[string]any{
			"PageTitle":   "Feed",
			"CurrentPage": "feed",
			"CSRFToken":   GetCSRFToken(r),
		}
		// Compute the oldest visible event timestamp + the at-cap flag that
		// drive the "Load older activity" button. We pass the oldest item's
		// timestamp through so the button's href rolls the window backward
		// in 3-day chunks. AtMaxLookback hides the button (replaced by
		// "End of feed") when the next chunk would exceed the 30-day cap.
		var oldestVisible time.Time
		for _, it := range items {
			if oldestVisible.IsZero() || it.Timestamp.Before(oldestVisible) {
				oldestVisible = it.Timestamp
			}
		}
		atMaxLookback := false
		if !oldestVisible.IsZero() {
			// Once the oldest visible event is older than (now - 30d), or
			// the next-chunk's upper bound would be, treat as end-of-feed.
			if oldestVisible.Before(now.Add(-service.FeedMaxLookback)) {
				atMaxLookback = true
			}
		}

		// "Finish setting up" banner: surfaces incomplete onboarding at the
		// top of the home feed (nil once setup is complete or dismissed).
		onboarding := onboardingBannerProps(computeOnboardingProgress(r.Context(), a), GetCSRFToken(r))

		body := pages.Feed(pages.FeedProps{
			CSRFToken:        GetCSRFToken(r),
			Onboarding:       onboarding,
			Hero:             hero,
			ConnectionAlerts: alerts,
			Days:             days,
			TotalItems:       len(items),
			WindowDays:       feedWindowDays,
			Now:              now,
			Filter:           filter,
			HasConnections:   hasConnections,
			LastSyncAt:       globalLastSyncAt,
			IsAdmin:          IsAdmin(tr.sm, r),
			OldestVisible:    oldestVisible,
			AtMaxLookback:    atMaxLookback,
		})
		// Single structured log line per request — read in production logs
		// to spot regressions across the per-query timings. Durations are
		// rounded to milliseconds; q_categories=0 means "skipped" (no bulk
		// action events to resolve).
		a.Logger.Info("feed: rendered",
			"duration_ms", time.Since(reqStart).Milliseconds(),
			"events", len(items),
			"reports", len(reports),
			"q_categories_ms", timings.Categories.Milliseconds(),
			"q_tags_ms", timings.Tags.Milliseconds(),
			"q_events_total_ms", timings.Events.Milliseconds(),
			"q_reports_ms", timings.Reports.Milliseconds(),
			"q_alerts_ms", qAlerts.Milliseconds(),
		)

		tr.RenderWithTempl(w, r, data, body)
	}
}

// feedQuery carries the windowing inputs shared by the full-page feed handler
// and the inline-pagination rows endpoint.
type feedQuery struct {
	now            time.Time
	window         time.Duration
	before         time.Time // zero → window anchored at now
	filter         string
	sessionActorID string
}

// feedQueryTimings reports the per-query durations feedWindowItems spent, so
// the page handler can keep emitting its structured timing log. Zero values
// mean "skipped" (e.g. category/tag lookups only run when a bulk_action event
// is present).
type feedQueryTimings struct {
	Reports    time.Duration
	Events     time.Duration
	Categories time.Duration
	Tags       time.Duration
}

// parseFeedBefore parses the `?before=` pagination cursor (RFC3339) and clamps
// it into the valid (now-FeedMaxLookback, now] range. Returns the zero time
// when absent, unparseable, or in the future — callers treat that as "no
// cursor, anchor the window at now". The service layer re-clamps, so this is
// defence in depth that also keeps the rendered window/footer consistent.
func parseFeedBefore(raw string, now time.Time) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	if parsed.After(now) {
		return time.Time{}
	}
	if minBefore := now.Add(-service.FeedMaxLookback); parsed.Before(minBefore) {
		parsed = minBefore
	}
	return parsed
}

// feedWindowItems runs the feed aggregation pipeline for a single window and
// returns the projected, newest-first items plus the agent reports fetched for
// the window. Both FeedHandler (initial page) and FeedRowsHandler (inline
// "Load older activity") call this so the rendered rows + windowing stay
// byte-identical — the rows endpoint grafts onto the same rail the page drew.
//
// The returned reports slice is the full windowed report set (the page uses it
// for the unread-count hero tile; the rows endpoint ignores it). All errors are
// logged here and surfaced to the caller, which degrades to an empty render
// rather than failing the request.
func feedWindowItems(ctx context.Context, a *app.App, svc *service.Service, q feedQuery) ([]pages.FeedItem, []service.AgentReportResponse, feedQueryTimings, error) {
	var timings feedQueryTimings

	// Reports — windowed; gated to the chips that actually surface them.
	var reports []service.AgentReportResponse
	if q.filter == "" || q.filter == "reports" {
		t0 := time.Now()
		var rerr error
		reports, rerr = svc.ListAgentReports(ctx, 50)
		timings.Reports = time.Since(t0)
		if rerr != nil {
			a.Logger.Error("feed: list agent reports", "error", rerr)
		}
	}

	// Grouped events. The windowed report list comes straight back as
	// `standaloneReports` — every report renders as its own comment-bubble row.
	t0 := time.Now()
	events, standaloneReports, err := svc.ListFeedEventsWithReports(ctx, service.FeedEventsParams{
		Window:        q.window,
		BulkThreshold: 3,
		SampleLimit:   5,
		Filter:        q.filter,
		ActorID:       q.sessionActorID,
		Before:        q.before,
	}, reports)
	timings.Events = time.Since(t0)
	if err != nil {
		a.Logger.Error("feed: list events", "error", err)
		return nil, reports, timings, err
	}

	// Display lookups for bulk-action subjects — only fetched when at least one
	// bulk_action event is present (the common sync/comment/session path skips
	// both queries).
	hasBulkAction := false
	for _, ev := range events {
		if ev.Type == "bulk_action" {
			hasBulkAction = true
			break
		}
	}
	var categoryDetail func(string) categoryDisplay
	var tagDisplayFn func(string) tagDisplay
	if hasBulkAction {
		t0 = time.Now()
		categoryTree, cerr := svc.ListCategories(ctx)
		timings.Categories = time.Since(t0)
		if cerr != nil {
			a.Logger.Error("feed: list categories", "error", cerr)
		}
		categoryDetail = categoryDetailLookup(categoryTree)
		t0 = time.Now()
		tags, terr := svc.ListTags(ctx)
		timings.Tags = time.Since(t0)
		if terr != nil {
			a.Logger.Error("feed: list tags", "error", terr)
		}
		tagDisplayFn = tagDisplayLookup(tags)
	} else {
		categoryDetail = func(string) categoryDisplay { return categoryDisplay{} }
		tagDisplayFn = func(string) tagDisplay { return tagDisplay{} }
	}

	// Window bounds — anchored at now, or at `before` when paginating.
	windowEnd := q.now
	if !q.before.IsZero() {
		windowEnd = q.before
	}
	windowStart := windowEnd.Add(-q.window)

	items := make([]pages.FeedItem, 0, len(events)+len(reports))
	for _, ev := range events {
		if ev.Timestamp.Before(windowStart) || ev.Timestamp.After(windowEnd) {
			continue
		}
		it := projectFeedEvent(ev, tagDisplayFn, categoryDetail)
		if it == nil {
			continue
		}
		items = append(items, *it)
	}

	// Every report renders as its own comment-bubble feed item. agentSeedCache
	// memoizes the report-author → avatar-seed resolution so repeat agents
	// don't re-query ("" is cached too — a miss is a decision, not a retry).
	agentSeedCache := map[string]string{}
	for _, rep := range standaloneReports {
		if rep.CreatedAt == "" {
			continue
		}
		ts, perr := time.Parse(time.RFC3339, rep.CreatedAt)
		if perr != nil {
			continue
		}
		if ts.Before(windowStart) || ts.After(windowEnd) {
			continue
		}
		reportAvatarSeed := ""
		if rep.CreatedByType == "agent" && rep.CreatedByID != nil {
			id := *rep.CreatedByID
			seed, cached := agentSeedCache[id]
			if !cached {
				if slug, ok := svc.ResolveAgentSlugForActor(ctx, id); ok {
					seed = slug
				}
				agentSeedCache[id] = seed
			}
			reportAvatarSeed = seed
		}
		items = append(items, pages.FeedItem{
			Type:         "report",
			Timestamp:    ts,
			TimestampStr: ts.UTC().Format(time.RFC3339),
			Report: &pages.FeedReport{
				ID:            rep.ID,
				ShortID:       rep.ShortID,
				Title:         rep.Title,
				BodyExcerpt:   excerpt(rep.Body, 220),
				Priority:      rep.Priority,
				DisplayAuthor: reportDisplayAuthor(rep.CreatedByName, rep.Author),
				IsUnread:      rep.ReadAt == nil,
				AvatarSeed:    reportAvatarSeed,
			},
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Timestamp.After(items[j].Timestamp)
	})
	return items, reports, timings, nil
}

// FeedRowsHandler serves GET /-/feed/rows?before=<rfc3339>[&filter=<slug>][&last_day=<YYYY-MM-DD>].
//
// Powers the inline "Load older activity" pagination on the home Feed: the
// feedPagination Alpine factory GETs this endpoint, appends the returned rail
// <li> rows to the existing <ol class="bb-timeline">, and advances its cursor
// from the response headers — so the timeline continues in place instead of
// reloading the whole page from the top.
//
// It reuses feedWindowItems (the same aggregation the page handler runs) so the
// appended rows are byte-identical to a full render. Empty 3-day windows are
// skipped server-side (walking `before` backward by the window up to the 30-day
// lookback cap) so a single tap always lands on content while older events
// remain.
//
// Response:
//   - Body: rendered <li> rows for the next non-empty window (day separators +
//     feed rows). The leading day separator is omitted when its calendar day
//     equals `last_day` (the tail day already on the client) so the rows
//     continue under the existing heading instead of repeating it.
//   - X-Feed-Next-Before: RFC3339 cursor to pass as `before` on the next tap
//     (the oldest item just rendered).
//   - X-Feed-Last-Day: the oldest day key just rendered (the client's new
//     dedup anchor).
//   - X-Feed-At-Max: "1" once the lookback cap is reached (the client swaps the
//     button for "End of feed"); "0" otherwise.
//
// An empty body with X-Feed-At-Max:1 means no older events remain.
func FeedRowsHandler(a *app.App, svc *service.Service, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		loc := UserLocation(r)
		now := time.Now().In(loc)
		window := time.Duration(feedWindowDays) * 24 * time.Hour
		filter := strings.TrimSpace(r.URL.Query().Get("filter"))
		lastDay := strings.TrimSpace(r.URL.Query().Get("last_day"))

		// A rows request with no usable cursor has nothing to page from.
		before := parseFeedBefore(r.URL.Query().Get("before"), now)
		if before.IsZero() {
			w.Header().Set("X-Feed-At-Max", "1")
			w.WriteHeader(http.StatusOK)
			return
		}

		// Resolve the "me" actor the same way the page does.
		var sessionActorID string
		if filter == "me" {
			sessionActorID = SessionUserID(tr.sm, r)
			if sessionActorID == "" {
				sessionActorID = SessionAccountID(tr.sm, r)
			}
			if sessionActorID == "" {
				filter = ""
			}
		}

		// Skip empty windows so a tap always lands on content. Bounded by the
		// lookback span (≤ FeedMaxLookback/window iterations) — once the cursor
		// reaches the cap we do one final clamped fetch and stop.
		minBefore := now.Add(-service.FeedMaxLookback)
		var items []pages.FeedItem
		cursor := before
		for {
			winItems, _, _, err := feedWindowItems(ctx, a, svc, feedQuery{
				now:            now,
				window:         window,
				before:         cursor,
				filter:         filter,
				sessionActorID: sessionActorID,
			})
			if err != nil {
				writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load feed")
				return
			}
			if len(winItems) > 0 {
				items = winItems
				break
			}
			cursor = cursor.Add(-window)
			if !cursor.After(minBefore) {
				break
			}
		}

		if len(items) == 0 {
			// No older events remain within the lookback window.
			w.Header().Set("X-Feed-At-Max", "1")
			w.WriteHeader(http.StatusOK)
			return
		}

		var oldest time.Time
		for _, it := range items {
			if oldest.IsZero() || it.Timestamp.Before(oldest) {
				oldest = it.Timestamp
			}
		}

		days := groupFeedByDay(items, now, loc)
		omitLeading := ""
		if len(days) > 0 && days[0].Key == lastDay {
			omitLeading = lastDay
		}
		lastRenderedDay := ""
		if len(days) > 0 {
			lastRenderedDay = days[len(days)-1].Key
		}

		w.Header().Set("X-Feed-Next-Before", oldest.UTC().Format(time.RFC3339))
		w.Header().Set("X-Feed-Last-Day", lastRenderedDay)
		if oldest.Before(minBefore) {
			w.Header().Set("X-Feed-At-Max", "1")
		} else {
			w.Header().Set("X-Feed-At-Max", "0")
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		comp := pages.FeedRows(pages.FeedRowsProps{
			Days:              days,
			Now:               now,
			OmitLeadingDayKey: omitLeading,
		})
		if err := comp.Render(ctx, w); err != nil {
			a.Logger.Error("feed rows: render", "error", err)
		}
	}
}

// buildFeedConnectionMeta does one ListBankConnections pass and projects out
// three things the page needs: the pinned alert cards (for connections in
// error/pending_reauth), whether *any* connection exists (drives the first-
// run empty state), and the most recent successful sync time across the
// household (drives the "quiet around here · last sync was {rel}" empty
// state). Co-locating the projections means the empty-state branch can
// pick the right copy without a second query.
func buildFeedConnectionMeta(ctx context.Context, a *app.App) (alerts []pages.FeedAlert, hasConnections bool, globalLastSyncAt time.Time) {
	bankConnections, err := a.Queries.ListBankConnections(ctx)
	if err != nil {
		a.Logger.Error("feed: list bank connections", "error", err)
		return nil, false, time.Time{}
	}
	alerts = make([]pages.FeedAlert, 0)
	for _, conn := range bankConnections {
		hasConnections = true
		if conn.LastSyncedAt.Valid && conn.LastSyncedAt.Time.After(globalLastSyncAt) {
			globalLastSyncAt = conn.LastSyncedAt.Time
		}
		status := string(conn.Status)
		if status != "error" && status != "pending_reauth" {
			continue
		}
		alert := pages.FeedAlert{
			ConnectionID:        pgconv.FormatUUID(conn.ID),
			Institution:         pgconv.TextOr(conn.InstitutionName, "Unknown bank"),
			Provider:            string(conn.Provider),
			Status:              status,
			ErrorMessage:        pgconv.TextOr(conn.ErrorMessage, ""),
			LastSyncedAt:        "Never",
			ConsecutiveFailures: int(conn.ConsecutiveFailures),
		}
		if conn.LastSyncedAt.Valid {
			alert.LastSyncedAt = relativeTime(conn.LastSyncedAt.Time)
		}
		alerts = append(alerts, alert)
	}
	return alerts, hasConnections, globalLastSyncAt
}

// projectFeedEvent maps one service-layer FeedEvent onto its templ-side
// FeedItem projection. Returns nil for events with unrecognised Type so the
// caller can skip them. tagDisplayFn / categoryDetail resolve display
// metadata for bulk-action subjects.
func projectFeedEvent(ev service.FeedEvent, tagDisplayFn func(string) tagDisplay, categoryDetail func(string) categoryDisplay) *pages.FeedItem {
	tsStr := ev.Timestamp.UTC().Format(time.RFC3339)
	switch ev.Type {
	case "sync":
		if ev.Sync == nil {
			return nil
		}
		return &pages.FeedItem{
			Type:         "sync",
			Timestamp:    ev.Timestamp,
			TimestampStr: tsStr,
			Sync:         projectFeedSync(ev.Sync),
		}
	case "comment":
		if ev.Comment == nil {
			return nil
		}
		return &pages.FeedItem{
			Type:         "comment",
			Timestamp:    ev.Timestamp,
			TimestampStr: tsStr,
			Comment: &pages.FeedComment{
				CommentShortID:     ev.Comment.CommentShortID,
				ActorName:          ev.Comment.ActorName,
				ActorType:          ev.Comment.ActorType,
				ActorID:            ev.Comment.ActorID,
				ActorAvatarVersion: ev.Comment.ActorAvatarVersion,
				Content:            ev.Comment.Content,
				Transaction:        projectSampleTx(ev.Comment.Transaction),
			},
		}
	case "agent_session":
		if ev.AgentSession == nil {
			return nil
		}
		return &pages.FeedItem{
			Type:         "agent_session",
			Timestamp:    ev.Timestamp,
			TimestampStr: tsStr,
			AgentSession: projectFeedAgentSession(ev.AgentSession),
		}
	case "bulk_action":
		if ev.BulkAction == nil {
			return nil
		}
		return &pages.FeedItem{
			Type:         "bulk_action",
			Timestamp:    ev.Timestamp,
			TimestampStr: tsStr,
			BulkAction:   projectFeedBulkAction(ev.BulkAction, tagDisplayFn, categoryDetail),
		}
	}
	return nil
}

func projectFeedSync(s *service.FeedSyncEvent) *pages.FeedSync {
	out := &pages.FeedSync{
		SyncLogID:       s.SyncLogID,
		InstitutionName: s.InstitutionName,
		Provider:        s.Provider,
		Trigger:         s.Trigger,
		Status:          s.Status,
		ErrorMessage:    s.ErrorMessage,
		AddedCount:      s.AddedCount,
		ModifiedCount:   s.ModifiedCount,
		RemovedCount:    s.RemovedCount,
		StartedAt:       s.StartedAt,
		RetryCount:      s.RetryCount,
		FirstFailureAt:  s.FirstFailureAt,
		AdditionalCount: s.AdditionalCount,
	}
	out.SampleTransactions = make([]pages.FeedTransactionRef, 0, len(s.SampleTransactions))
	for _, tx := range s.SampleTransactions {
		out.SampleTransactions = append(out.SampleTransactions, projectSampleTx(tx))
	}
	out.RuleOutcomes = make([]pages.FeedRuleOutcome, 0, len(s.RuleOutcomes))
	for _, ro := range s.RuleOutcomes {
		out.RuleOutcomes = append(out.RuleOutcomes, pages.FeedRuleOutcome{
			RuleName:    ro.RuleName,
			RuleShortID: ro.RuleShortID,
			Count:       ro.Count,
		})
	}
	return out
}

func projectFeedAgentSession(s *service.FeedAgentSessionEvent) *pages.FeedAgentSession {
	out := &pages.FeedAgentSession{
		SessionID:          s.SessionID,
		SessionShortID:     s.SessionShortID,
		APIKeyName:         s.APIKeyName,
		Purpose:            s.Purpose,
		ActorName:          s.ActorName,
		ActorType:          s.ActorType,
		ActorID:            s.ActorID,
		ActorAvatarVersion: s.ActorAvatarVersion,
		StartedAt:          s.StartedAt,
		EndedAt:            s.EndedAt,
		AnnotationCount:    s.AnnotationCount,
		UniqueTransactions: s.UniqueTransactions,
		Categorised:        s.KindCounts["category_set"],
		Tagged:             s.KindCounts["tag_added"],
		UntaggedRemoved:    s.KindCounts["tag_removed"],
		Commented:          s.KindCounts["comment"],
		RuleApplied:        s.KindCounts["rule_applied"],
	}
	out.SampleTransactions = make([]pages.FeedTransactionRef, 0, len(s.SampleTransactions))
	for _, tx := range s.SampleTransactions {
		out.SampleTransactions = append(out.SampleTransactions, projectSampleTx(tx))
	}
	return out
}

func projectFeedBulkAction(b *service.FeedBulkActionEvent, tagDisplayFn func(string) tagDisplay, categoryDetail func(string) categoryDisplay) *pages.FeedBulkAction {
	kindCounts := make(map[string]int, len(b.KindCounts))
	for k, v := range b.KindCounts {
		kindCounts[k] = v
	}
	out := &pages.FeedBulkAction{
		ActorName:          b.ActorName,
		ActorType:          b.ActorType,
		ActorID:            b.ActorID,
		ActorAvatarVersion: b.ActorAvatarVersion,
		Kind:               b.Kind,
		Count:              b.Count,
		KindCounts:         kindCounts,
		StartedAt:          b.StartedAt,
		EndedAt:            b.EndedAt,
	}
	for _, sub := range b.Subjects {
		display := pages.FeedBulkSubject{
			Name:  sub.Name,
			Slug:  sub.Slug,
			Count: sub.Count,
		}
		switch b.Kind {
		case "tag_added", "tag_removed":
			td := tagDisplayFn(sub.Slug)
			if td.DisplayName != "" {
				display.Name = td.DisplayName
			}
			if td.Color != nil {
				display.Color = *td.Color
			}
			if td.Icon != nil {
				display.Icon = *td.Icon
			}
		case "category_set":
			cd := categoryDetail(sub.Slug)
			if cd.DisplayName != "" {
				display.Name = cd.DisplayName
			}
			if cd.Color != nil {
				display.Color = *cd.Color
			}
			if cd.Icon != nil {
				display.Icon = *cd.Icon
			}
		}
		if display.Name == "" {
			display.Name = sub.Slug
		}
		out.Subjects = append(out.Subjects, display)
	}
	out.SampleTransactions = make([]pages.FeedTransactionRef, 0, len(b.SampleTransactions))
	for _, tx := range b.SampleTransactions {
		out.SampleTransactions = append(out.SampleTransactions, projectSampleTx(tx))
	}
	return out
}

func projectSampleTx(tx service.FeedSampleTx) pages.FeedTransactionRef {
	return pages.FeedTransactionRef{
		ShortID:             tx.ShortID,
		Name:                tx.Name,
		MerchantName:        tx.MerchantName,
		Amount:              tx.Amount,
		Currency:            tx.Currency,
		Date:                tx.Date,
		AccountName:         tx.AccountName,
		Institution:         tx.Institution,
		Pending:             tx.Pending,
		CategoryDisplayName: tx.CategoryDisplayName,
		CategoryColor:       tx.CategoryColor,
		CategoryIcon:        tx.CategoryIcon,
		CategorySlug:        tx.CategorySlug,
		TagCount:            tx.TagCount,
	}
}

// excerpt returns the first `n` runes of `s`, trimmed at the nearest word
// boundary if the cut would land mid-word.
func excerpt(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 || len(s) <= n {
		return s
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	cut := string(runes[:n])
	if i := strings.LastIndexAny(cut, " \n\t"); i > 0 && i > n-30 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " \n\t.,;:") + "…"
}

// groupFeedByDay buckets the feed into per-day groups using the supplied
// location (typically the viewer's browser timezone — see `UserLocation`).
// The first bucket gets a "Today" / "Yesterday" friendly label; older
// buckets render the calendar date.
func groupFeedByDay(items []pages.FeedItem, now time.Time, loc *time.Location) []pages.FeedDay {
	if len(items) == 0 {
		return nil
	}
	if loc == nil {
		loc = time.Local
	}
	nowInLoc := now.In(loc)
	out := make([]pages.FeedDay, 0)
	var current *pages.FeedDay
	for i := range items {
		ts := items[i].Timestamp.In(loc)
		key := ts.Format("2006-01-02")
		if current == nil || current.Key != key {
			out = append(out, pages.FeedDay{
				Key:   key,
				Label: friendlyDayLabel(ts, nowInLoc),
				First: len(out) == 0,
			})
			current = &out[len(out)-1]
		}
		current.Items = append(current.Items, items[i])
	}
	return out
}

// friendlyDayLabel renders "Today" / "Yesterday" / "Mar 14" depending on the
// distance between `t` and `now`. Both arguments must be in the same
// timezone — the caller passes Local() values.
func friendlyDayLabel(t, now time.Time) string {
	yt, mt, dt := t.Date()
	yn, mn, dn := now.Date()
	if yt == yn && mt == mn && dt == dn {
		return "Today"
	}
	yest := now.AddDate(0, 0, -1)
	yy, my, dy := yest.Date()
	if yt == yy && mt == my && dt == dy {
		return "Yesterday"
	}
	if yt == yn {
		return t.Format("Jan 2")
	}
	return t.Format("Jan 2, 2006")
}
