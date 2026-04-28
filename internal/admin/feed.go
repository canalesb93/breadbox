package admin

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"breadbox/internal/app"
	"breadbox/internal/pgconv"
	"breadbox/internal/ptrutil"
	"breadbox/internal/service"
	"breadbox/internal/templates/components/pages"
)

// FeedHandler serves GET /feed — the activity-style household feed page,
// designed as a candidate replacement for the legacy stats dashboard. The
// page renders a chronological, GitHub-style timeline of household events
// (syncs, agent reports, transaction comments, rule applications, manual
// recategorizations) with rich cards on the high-signal items and compact
// rows on the low-signal ones.
func FeedHandler(a *app.App, svc *service.Service, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		now := time.Now()

		// Build display lookups so enriched annotation summaries render the
		// human-friendly category and tag names instead of raw slugs.
		categoryTree, err := svc.ListCategories(ctx)
		if err != nil {
			a.Logger.Error("feed: list categories", "error", err)
		}
		categoryDetail := categoryDetailLookup(categoryTree)
		tags, err := svc.ListTags(ctx)
		if err != nil {
			a.Logger.Error("feed: list tags", "error", err)
		}
		tagDisplayFn := tagDisplayLookup(tags)

		// 1. Pull cross-transaction annotations (the substrate for most feed
		//    items: comments, rule applications, manual category sets, manual
		//    tag actions).
		feedRows, err := svc.ListFeedActivity(ctx, 250)
		if err != nil {
			a.Logger.Error("feed: list activity", "error", err)
		}

		// 2. Pull recent sync logs — one feed card per sync run keeps the
		//    "X transactions arrived from Chase" event compact.
		syncResult, err := svc.ListSyncLogsPaginated(ctx, service.SyncLogListParams{
			Page:     1,
			PageSize: 25,
		})
		if err != nil {
			a.Logger.Error("feed: list sync logs", "error", err)
		}

		// 3. Recent agent reports — rich cards. We pull the wider list (read
		//    + unread) so the feed shows the full activity stream rather than
		//    only the unread queue.
		recentReports, err := svc.ListAgentReports(ctx, 20)
		if err != nil {
			a.Logger.Error("feed: list agent reports", "error", err)
		}

		// 4. Connection alerts — pinned warning cards at the top for any
		//    connection currently in error/pending_reauth state.
		var connectionAlerts []pages.FeedAlert
		bankConnections, err := a.Queries.ListBankConnections(ctx)
		if err != nil {
			a.Logger.Error("feed: list bank connections", "error", err)
		}
		for _, conn := range bankConnections {
			status := string(conn.Status)
			if status != "error" && status != "pending_reauth" {
				continue
			}
			alert := pages.FeedAlert{
				ConnectionID:   pgconv.FormatUUID(conn.ID),
				Institution:    pgconv.TextOr(conn.InstitutionName, "Unknown bank"),
				Provider:       string(conn.Provider),
				Status:         status,
				ErrorMessage:   pgconv.TextOr(conn.ErrorMessage, ""),
				LastSyncedAt:   "Never",
				ConsecutiveFailures: int(conn.ConsecutiveFailures),
			}
			if conn.LastSyncedAt.Valid {
				alert.LastSyncedAt = relativeTime(conn.LastSyncedAt.Time)
			}
			connectionAlerts = append(connectionAlerts, alert)
		}

		// 5. Build feed items.
		items := make([]pages.FeedItem, 0, 64)

		// Sync items — emit one per sync run with non-zero changes OR errors.
		// Successful no-op syncs are noisy; suppress them so the feed stays
		// signal-rich.
		if syncResult != nil {
			for _, log := range syncResult.Logs {
				hasChanges := log.AddedCount+log.ModifiedCount+log.RemovedCount > 0
				isFailure := log.Status == "error"
				if !hasChanges && !isFailure {
					continue
				}
				ts := time.Time{}
				if log.StartedAt != nil {
					if t, err := time.Parse(time.RFC3339, *log.StartedAt); err == nil {
						ts = t
					}
				}
				if ts.IsZero() {
					continue
				}
				item := pages.FeedItem{
					Type:         "sync",
					Timestamp:    ts,
					TimestampStr: ts.UTC().Format(time.RFC3339),
					Sync: &pages.FeedSync{
						SyncLogID:       log.ID,
						InstitutionName: log.InstitutionName,
						Provider:        log.Provider,
						Trigger:         log.Trigger,
						Status:          log.Status,
						AddedCount:      int(log.AddedCount),
						ModifiedCount:   int(log.ModifiedCount),
						RemovedCount:    int(log.RemovedCount),
						RuleHits:        int(log.TotalRuleHits),
						ErrorMessage:    ptrutil.DerefOr(log.ErrorMessage, ""),
					},
				}
				items = append(items, item)
			}
		}

		// Agent report items.
		for _, rep := range recentReports {
			if rep.CreatedAt == "" {
				continue
			}
			ts, err := time.Parse(time.RFC3339, rep.CreatedAt)
			if err != nil {
				continue
			}
			item := pages.FeedItem{
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
			}
			items = append(items, item)
		}

		// Annotation-driven items. We group rule_applied rows by (rule_id, day)
		// to collapse "Rule X auto-categorized 47 transactions" into a single
		// card instead of 47 separate ones. Comments, manual category sets,
		// and manual tag actions stay as individual rows.
		ruleBatches := make(map[string]*pages.FeedRuleBatch)
		for _, fr := range feedRows {
			ann := fr.Annotation
			if ann.CreatedAtTime.IsZero() {
				continue
			}
			ts := ann.CreatedAtTime
			tsStr := ts.UTC().Format(time.RFC3339)

			switch ann.Kind {
			case "rule_applied":
				dayKey := ts.Local().Format("2006-01-02")
				ruleKey := dayKey + "|" + safeStr(ann.RuleID) + "|" + ann.RuleName
				batch, ok := ruleBatches[ruleKey]
				if !ok {
					batch = &pages.FeedRuleBatch{
						RuleName:    ann.RuleName,
						RuleShortID: ann.RuleShortID,
						DayLabel:    ts.Local().Format("Jan 2"),
						LatestTS:    ts,
					}
					ruleBatches[ruleKey] = batch
				}
				batch.Count++
				field, _ := ann.Payload["action_field"].(string)
				if field != "" {
					if batch.ActionFields == nil {
						batch.ActionFields = map[string]int{}
					}
					batch.ActionFields[field]++
				}
				if ts.After(batch.LatestTS) {
					batch.LatestTS = ts
				}
				// Track up to 4 sample transactions for the expand affordance.
				if len(batch.Samples) < 4 {
					batch.Samples = append(batch.Samples, pages.FeedTransactionSample{
						ShortID:      fr.TransactionShortID,
						MerchantName: pickMerchant(fr),
						Amount:       fr.Amount,
						Currency:     fr.IsoCurrencyCode,
					})
				}

			case "comment":
				if ann.IsDeleted {
					continue
				}
				items = append(items, pages.FeedItem{
					Type:         "comment",
					Timestamp:    ts,
					TimestampStr: tsStr,
					Comment: &pages.FeedComment{
						ActorName:          ann.ActorName,
						ActorType:          ann.ActorType,
						ActorID:            safeStr(ann.ActorID),
						ActorAvatarVersion: ann.ActorAvatarVersion,
						Content:            ann.Content,
						Transaction:        feedTransactionRef(fr),
					},
				})

			case "tag_added", "tag_removed":
				display := tagDisplayFn(ann.TagSlug)
				name := display.DisplayName
				if name == "" {
					name = ann.TagSlug
				}
				items = append(items, pages.FeedItem{
					Type:         "tag",
					Timestamp:    ts,
					TimestampStr: tsStr,
					Tag: &pages.FeedTagChange{
						ActorName:          ann.ActorName,
						ActorType:          ann.ActorType,
						ActorID:            safeStr(ann.ActorID),
						ActorAvatarVersion: ann.ActorAvatarVersion,
						Action:             strings.TrimPrefix(ann.Kind, "tag_"),
						TagSlug:            ann.TagSlug,
						TagName:            name,
						TagColor:           ptrutil.DerefOr(display.Color, ""),
						TagIcon:            ptrutil.DerefOr(display.Icon, ""),
						Note:               ann.Note,
						Transaction:        feedTransactionRef(fr),
					},
				})

			case "category_set":
				cd := categoryDetail(ann.CategorySlug)
				name := cd.DisplayName
				if name == "" {
					name = ann.CategorySlug
				}
				items = append(items, pages.FeedItem{
					Type:         "category",
					Timestamp:    ts,
					TimestampStr: tsStr,
					Category: &pages.FeedCategoryChange{
						ActorName:          ann.ActorName,
						ActorType:          ann.ActorType,
						ActorID:            safeStr(ann.ActorID),
						ActorAvatarVersion: ann.ActorAvatarVersion,
						CategorySlug:       ann.CategorySlug,
						CategoryName:       name,
						CategoryColor:      ptrutil.DerefOr(cd.Color, ""),
						CategoryIcon:       ptrutil.DerefOr(cd.Icon, ""),
						Transaction:        feedTransactionRef(fr),
					},
				})
			}
		}
		for _, batch := range ruleBatches {
			items = append(items, pages.FeedItem{
				Type:         "rule_batch",
				Timestamp:    batch.LatestTS,
				TimestampStr: batch.LatestTS.UTC().Format(time.RFC3339),
				RuleBatch:    batch,
			})
		}

		// 6. Sort newest-first. Feed convention is reverse-chronological so
		//    the most recent activity is always on top — the inverse of the
		//    per-transaction timeline (chat-style; new at the bottom).
		sort.Slice(items, func(i, j int) bool {
			return items[i].Timestamp.After(items[j].Timestamp)
		})

		// 7. Group into day buckets for the day-separator chrome.
		days := groupFeedByDay(items, now)

		// 8. At-a-glance hero stats — quick "today" snapshot derived from the
		//    feed itself plus a couple of cheap counts.
		hero := buildFeedHero(now, items, feedRows, recentReports, syncResult)

		data := map[string]any{
			"PageTitle":   "Feed",
			"CurrentPage": "feed",
			"CSRFToken":   GetCSRFToken(r),
		}
		body := pages.Feed(pages.FeedProps{
			CSRFToken:        GetCSRFToken(r),
			Hero:             hero,
			ConnectionAlerts: connectionAlerts,
			Days:             days,
			TotalItems:       len(items),
			Now:              now,
			Filter:           strings.TrimSpace(r.URL.Query().Get("filter")),
		})
		tr.RenderWithTempl(w, r, data, body)
	}
}

// pickMerchant prefers merchant_name when present, falling back to the
// transaction name (which often carries the raw description from the
// provider).
func pickMerchant(fr service.FeedActivityRow) string {
	if fr.MerchantName != "" {
		return fr.MerchantName
	}
	return fr.TransactionName
}

// feedTransactionRef builds the FeedTransactionRef projection that every
// transaction-anchored card uses for the "$amount at Merchant" subtitle.
func feedTransactionRef(fr service.FeedActivityRow) pages.FeedTransactionRef {
	return pages.FeedTransactionRef{
		ShortID:      fr.TransactionShortID,
		Name:         fr.TransactionName,
		MerchantName: pickMerchant(fr),
		Amount:       fr.Amount,
		Currency:     fr.IsoCurrencyCode,
		Date:         fr.TransactionDate,
		AccountName:  fr.AccountName,
		Institution:  fr.InstitutionName,
	}
}

// safeStr dereferences a *string with an empty-string fallback. Used for the
// many *string-typed fields on Annotation that the feed projection wants to
// pass through as plain strings.
func safeStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// excerpt returns the first `n` runes of `s`, trimmed at the nearest word
// boundary if the cut would land mid-word. Trailing whitespace is stripped
// and an ellipsis is appended when the original was longer.
func excerpt(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 || len(s) <= n {
		return s
	}
	// Cut on rune boundaries.
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

// groupFeedByDay buckets the feed into per-day groups using the server's
// local timezone. The first bucket gets a "Today" / "Yesterday" friendly
// label; older buckets render the calendar date.
func groupFeedByDay(items []pages.FeedItem, now time.Time) []pages.FeedDay {
	if len(items) == 0 {
		return nil
	}
	out := make([]pages.FeedDay, 0)
	var current *pages.FeedDay
	for i := range items {
		ts := items[i].Timestamp.Local()
		key := ts.Format("2006-01-02")
		if current == nil || current.Key != key {
			out = append(out, pages.FeedDay{
				Key:   key,
				Label: friendlyDayLabel(ts, now.Local()),
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

// buildFeedHero derives the at-a-glance numbers shown above the feed: events
// today, sync status, unread-report count, and the count of pinned alerts.
func buildFeedHero(now time.Time, items []pages.FeedItem, rows []service.FeedActivityRow, reports []service.AgentReportResponse, syncResult *service.SyncLogListResult) pages.FeedHero {
	hero := pages.FeedHero{
		Generated: now.Format("Mon, Jan 2"),
	}
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	for _, it := range items {
		if it.Timestamp.After(startOfDay) {
			hero.EventsToday++
			switch it.Type {
			case "sync":
				if it.Sync != nil {
					hero.NewTransactionsToday += it.Sync.AddedCount
				}
			case "comment":
				hero.CommentsToday++
			case "rule_batch":
				if it.RuleBatch != nil {
					hero.RuleHitsToday += it.RuleBatch.Count
				}
			}
		}
	}
	for _, r := range reports {
		if r.ReadAt == nil {
			hero.UnreadReports++
		}
	}
	if syncResult != nil {
		for _, log := range syncResult.Logs {
			if log.StartedAt == nil {
				continue
			}
			t, err := time.Parse(time.RFC3339, *log.StartedAt)
			if err != nil {
				continue
			}
			if hero.LastSyncAt.IsZero() || t.After(hero.LastSyncAt) {
				hero.LastSyncAt = t
				hero.LastSyncStatus = log.Status
				hero.LastSyncInstitution = log.InstitutionName
			}
		}
	}
	if !hero.LastSyncAt.IsZero() {
		hero.LastSyncRel = relativeTime(hero.LastSyncAt)
	}
	_ = rows
	return hero
}
