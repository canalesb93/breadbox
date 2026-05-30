//go:build !lite

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"breadbox/internal/pgconv"
	"breadbox/internal/slugs"
	"breadbox/internal/sync"

	"github.com/jackc/pgx/v5/pgtype"
)

// seriesDetectWindowDays bounds live per-sync detection so an 8-connection
// household doesn't trigger full-history scans on every sync. The one-time
// backfill ignores it (all history).
const seriesDetectWindowDays = 400

// seriesBackfillMaxCandidates caps how many candidates the all-history backfill
// surfaces per user, keeping the first /subscriptions list trustworthy.
const seriesBackfillMaxCandidates = 100

type detectOptions struct {
	since         *time.Time // nil = all history
	staleGate     bool       // drop candidates whose last charge is too old
	maxCandidates int        // 0 = unlimited
}

type chargeRow struct {
	id          pgtype.UUID
	merchantKey string
	currency    string // "" = NULL
	date        time.Time
	amountCents int64
}

// DetectSeriesForConnection runs the deterministic detector for the user owning
// a just-synced connection, over a trailing window. Invoked from OnSyncComplete
// (before the agent fire). Errors are returned for logging; the caller should
// not fail the sync on a detection error.
func (s *Service) DetectSeriesForConnection(ctx context.Context, connID pgtype.UUID) (int, error) {
	if !s.seriesDetectorEnabled(ctx) {
		return 0, nil
	}
	var userID pgtype.UUID
	err := s.Pool.QueryRow(ctx, `SELECT user_id FROM bank_connections WHERE id = $1`, connID).Scan(&userID)
	if err != nil {
		return 0, fmt.Errorf("resolve connection user: %w", err)
	}
	since := time.Now().AddDate(0, 0, -seriesDetectWindowDays)
	return s.detectForUser(ctx, userID, detectOptions{since: &since})
}

// BackfillSeriesDetection runs an aggressive, all-history detection pass for
// every distinct user, with the staleness gate and per-user cap engaged so a
// multi-year scan surfaces trustworthy active subscriptions rather than
// resurrecting long-cancelled ones. Returns total candidates emitted.
func (s *Service) BackfillSeriesDetection(ctx context.Context) (int, error) {
	users, err := s.distinctEffectiveUsers(ctx)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, u := range users {
		n, err := s.detectForUser(ctx, u, detectOptions{
			staleGate:     true,
			maxCandidates: seriesBackfillMaxCandidates,
		})
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (s *Service) detectForUser(ctx context.Context, effUser pgtype.UUID, opts detectOptions) (int, error) {
	if err := s.populateMerchantKeys(ctx, effUser); err != nil {
		return 0, err
	}
	rows, err := s.loadCandidateCharges(ctx, effUser, opts.since)
	if err != nil {
		return 0, err
	}

	type qualified struct {
		merchantKey string
		currency    string
		analysis    groupAnalysis
		members     []pgtype.UUID
		lastSeen    time.Time
		occ         int
	}
	var quals []qualified

	i := 0
	for i < len(rows) {
		j := i
		for j < len(rows) && rows[j].merchantKey == rows[i].merchantKey && rows[j].currency == rows[i].currency {
			j++
		}
		group := rows[i:j]
		i = j

		charges := make([]chargePoint, len(group))
		members := make([]pgtype.UUID, len(group))
		var lastSeen time.Time
		for k, r := range group {
			charges[k] = chargePoint{date: r.date, amountCents: r.amountCents}
			members[k] = r.id
			if r.date.After(lastSeen) {
				lastSeen = r.date
			}
		}
		analysis, ok := analyzeGroup(charges, group[0].merchantKey, group[0].currency)
		if !ok {
			continue
		}
		if opts.staleGate && seriesIsStale(analysis.cadence, lastSeen) {
			continue
		}
		quals = append(quals, qualified{
			merchantKey: group[0].merchantKey,
			currency:    group[0].currency,
			analysis:    analysis,
			members:     members,
			lastSeen:    lastSeen,
			occ:         len(group),
		})
	}

	// Surface the highest-occurrence (most trustworthy) candidates first; cap.
	sort.SliceStable(quals, func(a, b int) bool { return quals[a].occ > quals[b].occ })
	if opts.maxCandidates > 0 && len(quals) > opts.maxCandidates {
		quals = quals[:opts.maxCandidates]
	}

	emitted := 0
	for _, q := range quals {
		signals, _ := json.Marshal(q.analysis.signals)
		memberStrs := make([]string, len(q.members))
		for k, m := range q.members {
			memberStrs[k] = formatUUID(m)
		}
		var currency *string
		if q.currency != "" {
			c := q.currency
			currency = &c
		}
		expected := float64(q.analysis.expectedAmountCents) / 100
		_, err := s.UpsertSeriesCandidate(ctx, SeriesUpsert{
			Name:             slugs.TitleCase(q.merchantKey),
			MerchantKey:      q.merchantKey,
			Cadence:          q.analysis.cadence,
			ExpectedDay:      q.analysis.expectedDay,
			ExpectedAmount:   &expected,
			Currency:         currency,
			Source:           SeriesSourceDeterministic,
			MemberTxnIDs:     memberStrs,
			DetectionSignals: signals,
		}, SystemActor())
		if err != nil {
			return emitted, fmt.Errorf("upsert detected series %q: %w", q.merchantKey, err)
		}
		emitted++
	}
	return emitted, nil
}

// populateMerchantKeys derives transactions.merchant_key for the user's rows
// that don't have one yet, using the same Go normalizer the sync path uses. The
// backfill's "UPDATE merchant_key first" step; a no-op once rows are populated.
func (s *Service) populateMerchantKeys(ctx context.Context, effUser pgtype.UUID) error {
	rows, err := s.Pool.Query(ctx, `
		SELECT t.id, COALESCE(t.provider_merchant_name, ''), t.provider_name
		FROM transactions t
		JOIN accounts a ON t.account_id = a.id
		LEFT JOIN bank_connections bc ON a.connection_id = bc.id
		WHERE t.merchant_key IS NULL AND t.deleted_at IS NULL
		  AND COALESCE(t.attributed_user_id, bc.user_id) IS NOT DISTINCT FROM $1`, effUser)
	if err != nil {
		return fmt.Errorf("load rows for merchant_key fill: %w", err)
	}
	type kv struct {
		id  pgtype.UUID
		key string
	}
	var updates []kv
	for rows.Next() {
		var id pgtype.UUID
		var merchant, name string
		if err := rows.Scan(&id, &merchant, &name); err != nil {
			rows.Close()
			return fmt.Errorf("scan merchant row: %w", err)
		}
		if key := sync.MerchantKey(merchant, name); key != "" {
			updates = append(updates, kv{id: id, key: key})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate merchant rows: %w", err)
	}
	for _, u := range updates {
		if _, err := s.Pool.Exec(ctx,
			`UPDATE transactions SET merchant_key = $2 WHERE id = $1 AND merchant_key IS NULL`,
			u.id, u.key); err != nil {
			return fmt.Errorf("set merchant_key: %w", err)
		}
	}
	return nil
}

func (s *Service) loadCandidateCharges(ctx context.Context, effUser pgtype.UUID, since *time.Time) ([]chargeRow, error) {
	var sinceArg pgtype.Date
	if since != nil {
		sinceArg = pgconv.Date(*since)
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT t.id, t.merchant_key, COALESCE(t.iso_currency_code, ''), t.date, t.amount
		FROM transactions t
		JOIN accounts a ON t.account_id = a.id
		LEFT JOIN bank_connections bc ON a.connection_id = bc.id
		WHERE t.deleted_at IS NULL AND t.pending = false AND t.merchant_key IS NOT NULL
		  AND COALESCE(t.attributed_user_id, bc.user_id) IS NOT DISTINCT FROM $1
		  AND ($2::date IS NULL OR t.date >= $2)
		ORDER BY t.merchant_key, COALESCE(t.iso_currency_code, ''), t.date`,
		effUser, sinceArg)
	if err != nil {
		return nil, fmt.Errorf("load candidate charges: %w", err)
	}
	defer rows.Close()
	var out []chargeRow
	for rows.Next() {
		var r chargeRow
		var date pgtype.Date
		var amount pgtype.Numeric
		if err := rows.Scan(&r.id, &r.merchantKey, &r.currency, &date, &amount); err != nil {
			return nil, fmt.Errorf("scan charge: %w", err)
		}
		r.date = date.Time
		r.amountCents = numericToCents(amount)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Service) distinctEffectiveUsers(ctx context.Context) ([]pgtype.UUID, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT DISTINCT COALESCE(t.attributed_user_id, bc.user_id)
		FROM transactions t
		JOIN accounts a ON t.account_id = a.id
		LEFT JOIN bank_connections bc ON a.connection_id = bc.id
		WHERE t.deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("distinct users: %w", err)
	}
	defer rows.Close()
	var out []pgtype.UUID
	for rows.Next() {
		var u pgtype.UUID
		if err := rows.Scan(&u); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// seriesIsStale reports whether a candidate's last charge is too old to present
// as a live subscription — guards the aggressive backfill from resurrecting
// cancelled subscriptions. Threshold ≈ 1.5× cadence, floored at 90 days.
func seriesIsStale(cadence string, lastSeen time.Time) bool {
	intervalDays := cadenceIntervalDays(cadence)
	threshold := int(1.5 * float64(intervalDays))
	if threshold < 90 {
		threshold = 90
	}
	return time.Since(lastSeen) > time.Duration(threshold)*24*time.Hour
}

func cadenceIntervalDays(cadence string) int {
	switch cadence {
	case SeriesCadenceWeekly:
		return 7
	case SeriesCadenceBiweekly:
		return 14
	case SeriesCadenceMonthly:
		return 30
	case SeriesCadenceQuarterly:
		return 91
	case SeriesCadenceSemiannual:
		return 182
	case SeriesCadenceAnnual:
		return 365
	default:
		return 30
	}
}

// seriesDetectorEnabled reads the series_deterministic_detector app_config flag
// (default on when unset/unreadable). Lets a user run pure-agent detection.
func (s *Service) seriesDetectorEnabled(ctx context.Context) bool {
	var v pgtype.Text
	if err := s.Pool.QueryRow(ctx,
		`SELECT value FROM app_config WHERE key = 'series_deterministic_detector'`).Scan(&v); err != nil {
		return true
	}
	return v.Valid && v.String != "false"
}

func numericToCents(n pgtype.Numeric) int64 {
	f, ok := pgconv.NumericToFloat(n)
	if !ok {
		return 0
	}
	return int64(math.Round(f * 100))
}
