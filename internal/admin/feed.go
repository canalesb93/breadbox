package admin

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"breadbox/internal/app"
	"breadbox/internal/appconfig"
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
// Until onboarding is dismissed, the root path redirects to /getting-started
// (matching the old DashboardHandler behaviour).
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

		// Redirect to getting-started page if onboarding is not dismissed.
		if !appconfig.Bool(ctx, a.Queries, "onboarding_dismissed", false) {
			http.Redirect(w, r, "/getting-started", http.StatusSeeOther)
			return
		}

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
		// Zero-value durations indicate "skipped" — e.g. q_categories=0 when
		// there are no bulk_action events to resolve.
		reqStart := time.Now()
		var qCategories, qTags, qEvents, qReports, qAlerts time.Duration

		// `?before=<rfc3339>` lets the user roll the 3-day window backward
		// in chunks via the "Load older activity" footer button. Cap at
		// 30 days back from now — the service layer enforces the same
		// ceiling, but clamping here keeps the rendered window/footer
		// consistent and makes the cap discoverable from a single layer.
		var beforeTime time.Time
		if rawBefore := strings.TrimSpace(r.URL.Query().Get("before")); rawBefore != "" {
			if parsed, err := time.Parse(time.RFC3339, rawBefore); err == nil {
				beforeTime = parsed
				minBefore := now.Add(-service.FeedMaxLookback)
				if beforeTime.Before(minBefore) {
					beforeTime = minBefore
				}
				if beforeTime.After(now) {
					beforeTime = time.Time{}
				}
			}
		}

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

		// 1. Reports — windowed to match the feed bound. Fetched ahead of
		// the events call so the service can fold matching reports into
		// bulk_action / agent_session events (one card per agent run
		// instead of two adjacent cards). Skipped under the
		// syncs/comments/sessions/me chips, which scope the page to events
		// the service layer produces.
		var reports []service.AgentReportResponse
		if filter == "" || filter == "reports" {
			t0 := time.Now()
			var rerr error
			reports, rerr = svc.ListAgentReports(ctx, 50)
			qReports = time.Since(t0)
			if rerr != nil {
				a.Logger.Error("feed: list agent reports", "error", rerr)
			}
		}

		// 2. Grouped events from the service. Run after the report fetch so
		// the service can fold matching reports into bulk_action /
		// agent_session events. Reports that don't fold into anything come
		// back via `leftoverReports` for standalone-card rendering. We also
		// decide here whether the (relatively expensive) tag + category
		// lookups are needed — they only feed bulk_action subject rendering.
		// Skipping them on the common no-bulk path saves 5-10ms per request.
		t0 := time.Now()
		events, leftoverReports, err := svc.ListFeedEventsWithReports(ctx, service.FeedEventsParams{
			Window:        window,
			BulkThreshold: 3,
			SampleLimit:   5,
			Filter:        filter,
			ActorID:       sessionActorID,
			Before:        beforeTime,
		}, reports)
		qEvents = time.Since(t0)
		if err != nil {
			a.Logger.Error("feed: list events", "error", err)
		}

		// Display lookups so bulk-action subjects render with friendly tag
		// and category display names instead of raw slugs. Only fetched when
		// at least one bulk_action event is present — the common case (sync
		// + comment + agent_session events only) skips both queries.
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
			qCategories = time.Since(t0)
			if cerr != nil {
				a.Logger.Error("feed: list categories", "error", cerr)
			}
			categoryDetail = categoryDetailLookup(categoryTree)
			t0 = time.Now()
			tags, terr := svc.ListTags(ctx)
			qTags = time.Since(t0)
			if terr != nil {
				a.Logger.Error("feed: list tags", "error", terr)
			}
			tagDisplayFn = tagDisplayLookup(tags)
		} else {
			// projectFeedBulkAction won't fire on this path, but
			// projectFeedEvent unconditionally accepts the lookups. Provide
			// no-op stubs so the call site stays simple.
			categoryDetail = func(string) categoryDisplay { return categoryDisplay{} }
			tagDisplayFn = func(string) tagDisplay { return tagDisplay{} }
		}

		// 3. Connection alerts + empty-state meta — current state, not
		// windowed. Alerts hide under any active chip so the filtered view
		// is exclusively the chip's scope; the alerts re-appear on the
		// unfiltered "All" page. The meta (hasConnections + global last-
		// sync-at) is always collected so the empty-state branch can pick
		// the right copy regardless of filter.
		t0 = time.Now()
		alerts, hasConnections, globalLastSyncAt := buildFeedConnectionMeta(ctx, a)
		qAlerts = time.Since(t0)
		if filter != "" {
			alerts = nil
		}

		// 4. Project FeedEvents onto templ-side FeedItems. The window is
		// anchored at `windowEnd` (now, or the `?before=` timestamp when
		// paginating); `windowStart` is `windowEnd - window`.
		windowEnd := now
		if !beforeTime.IsZero() {
			windowEnd = beforeTime
		}
		windowStart := windowEnd.Add(-window)

		items := make([]pages.FeedItem, 0, len(events)+len(reports))
		var lastSyncAt time.Time
		var lastSyncStatus, lastSyncInstitution string
		var totalNewTransactionsToday, ruleHitsToday int
		startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

		for _, ev := range events {
			if ev.Timestamp.Before(windowStart) || ev.Timestamp.After(windowEnd) {
				continue
			}
			it := projectFeedEvent(ev, tagDisplayFn, categoryDetail)
			if it == nil {
				continue
			}
			items = append(items, *it)

			// Hero-stat collection over the same loop.
			switch ev.Type {
			case "sync":
				if ev.Sync != nil && ev.Sync.StartedAt.After(lastSyncAt) {
					lastSyncAt = ev.Sync.StartedAt
					lastSyncStatus = ev.Sync.Status
					lastSyncInstitution = ev.Sync.InstitutionName
				}
				if ev.Sync != nil && ev.Timestamp.After(startOfDay) {
					totalNewTransactionsToday += ev.Sync.AddedCount
					for _, ro := range ev.Sync.RuleOutcomes {
						ruleHitsToday += ro.Count
					}
				}
			}
		}

		// 5. Append leftover (un-folded) reports as their own feed items.
		// Reports that the service folded into a bulk_action / agent_session
		// card are NOT in `leftoverReports` — they render as part of that
		// card's headline.
		for _, rep := range leftoverReports {
			if rep.CreatedAt == "" {
				continue
			}
			ts, err := time.Parse(time.RFC3339, rep.CreatedAt)
			if err != nil {
				continue
			}
			if ts.Before(windowStart) || ts.After(windowEnd) {
				continue
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
					Tags:          rep.Tags,
					DisplayAuthor: reportDisplayAuthor(rep.CreatedByName, rep.Author),
					IsUnread:      rep.ReadAt == nil,
				},
			})
		}

		// 6. Sort + day-bucket.
		sort.Slice(items, func(i, j int) bool {
			return items[i].Timestamp.After(items[j].Timestamp)
		})
		days := groupFeedByDay(items, now, loc)

		// 7. Hero band.
		var commentsToday, eventsToday, unreadReports int
		for _, it := range items {
			if it.Timestamp.After(startOfDay) {
				eventsToday++
				if it.Type == "comment" {
					commentsToday++
				}
			}
		}
		for _, r := range reports {
			if r.ReadAt == nil {
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

		body := pages.Feed(pages.FeedProps{
			CSRFToken:        GetCSRFToken(r),
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
			"q_categories_ms", qCategories.Milliseconds(),
			"q_tags_ms", qTags.Milliseconds(),
			"q_events_total_ms", qEvents.Milliseconds(),
			"q_reports_ms", qReports.Milliseconds(),
			"q_alerts_ms", qAlerts.Milliseconds(),
		)

		tr.RenderWithTempl(w, r, data, body)
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
	if s.Report != nil {
		out.ReportID = s.Report.ID
		out.ReportShortID = s.Report.ShortID
		out.ReportTitle = s.Report.Title
		out.ReportPriority = s.Report.Priority
		out.ReportTags = s.Report.Tags
		out.ReportIsUnread = s.Report.IsUnread
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
	if b.Report != nil {
		out.ReportID = b.Report.ID
		out.ReportShortID = b.Report.ShortID
		out.ReportTitle = b.Report.Title
		out.ReportPriority = b.Report.Priority
		out.ReportTags = b.Report.Tags
		out.ReportIsUnread = b.Report.IsUnread
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
