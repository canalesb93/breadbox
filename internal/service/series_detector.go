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

// seriesExplainMaxRows caps the explain feed so a noisy account doesn't return
// hundreds of one-off merchants.
const seriesExplainMaxRows = 50

// SeriesNearMiss is one merchant group that is NOT currently a recurring series,
// annotated with the detector's verdict: either it qualifies (eligible but not
// yet tracked) or it fell short of a specific precision-first gate. This is the
// read-side "why isn't this a subscription?" feed — pure analysis over existing
// transactions, no writes.
type SeriesNearMiss struct {
	MerchantKey       string   `json:"merchant_key"`
	Name              string   `json:"name"`
	Currency          *string  `json:"iso_currency_code,omitempty"`
	OccurrenceCount   int      `json:"occurrence_count"`
	Qualifies         bool     `json:"qualifies"`        // true = passes every gate but isn't tracked yet
	Reason            string   `json:"reason,omitempty"` // gate it failed (empty when Qualifies)
	Explanation       string   `json:"explanation"`      // human one-liner
	NearestCadence    string   `json:"nearest_cadence,omitempty"`
	MedianGapDays     float64  `json:"median_gap_days,omitempty"`
	IntervalCV        float64  `json:"interval_cv,omitempty"`
	AmountMin         *float64 `json:"amount_min,omitempty"`
	AmountMax         *float64 `json:"amount_max,omitempty"`
	AmountSpreadRatio float64  `json:"amount_spread_ratio,omitempty"`
	FirstSeen         *string  `json:"first_seen,omitempty"`
	LastSeen          *string  `json:"last_seen,omitempty"`
}

// ExplainSeriesCandidates reports, per merchant group that is not already a
// recurring series, the detector's verdict over the trailing detection window —
// the same gates the live detector runs, but surfacing the *reason* a group did
// (or didn't) qualify. Read-only: no merchant_key backfill, no upserts.
//
// Groups already represented by a recurring_series row (any status) are
// excluded — they're hits, not near-misses. Single-charge groups are dropped as
// noise. Results are ordered most-charges-first (closest to qualifying) and
// capped at seriesExplainMaxRows.
func (s *Service) ExplainSeriesCandidates(ctx context.Context) ([]SeriesNearMiss, error) {
	users, err := s.distinctEffectiveUsers(ctx)
	if err != nil {
		return nil, err
	}
	existing, err := s.existingSeriesMerchantKeys(ctx)
	if err != nil {
		return nil, err
	}
	since := time.Now().AddDate(0, 0, -seriesDetectWindowDays)

	var out []SeriesNearMiss
	for _, u := range users {
		rows, err := s.loadCandidateCharges(ctx, u, &since)
		if err != nil {
			return nil, err
		}
		i := 0
		for i < len(rows) {
			j := i
			for j < len(rows) && rows[j].merchantKey == rows[i].merchantKey && rows[j].currency == rows[i].currency {
				j++
			}
			group := rows[i:j]
			i = j

			key := group[0].merchantKey
			if existing[key] {
				continue // already a series — not a near-miss
			}
			if len(group) < 2 {
				continue // single charge: nothing to explain
			}

			charges := make([]chargePoint, len(group))
			for k, r := range group {
				charges[k] = chargePoint{date: r.date, amountCents: r.amountCents}
			}
			_, diag := evaluateGroup(charges, key, group[0].currency)

			nm := SeriesNearMiss{
				MerchantKey:       key,
				Name:              slugs.TitleCase(key),
				OccurrenceCount:   diag.OccurrenceCount,
				Qualifies:         diag.Reason == "",
				Reason:            diag.Reason,
				Explanation:       nearMissExplanation(diag),
				NearestCadence:    diag.NearestCadence,
				MedianGapDays:     diag.MedianGapDays,
				IntervalCV:        diag.IntervalCV,
				AmountSpreadRatio: diag.AmountSpreadRatio,
			}
			if group[0].currency != "" {
				c := group[0].currency
				nm.Currency = &c
			}
			if diag.AmountMaxCents > 0 {
				mn := float64(diag.AmountMinCents) / 100
				mx := float64(diag.AmountMaxCents) / 100
				nm.AmountMin, nm.AmountMax = &mn, &mx
			}
			// Rows are ordered by date asc within a group.
			fs := group[0].date.Format("2006-01-02")
			ls := group[len(group)-1].date.Format("2006-01-02")
			nm.FirstSeen, nm.LastSeen = &fs, &ls
			out = append(out, nm)
		}
	}

	sort.SliceStable(out, func(a, b int) bool { return out[a].OccurrenceCount > out[b].OccurrenceCount })
	if len(out) > seriesExplainMaxRows {
		out = out[:seriesExplainMaxRows]
	}
	return out, nil
}

// existingSeriesMerchantKeys returns the set of merchant_keys that already have
// a recurring_series row (any status), so the explain feed can skip them.
func (s *Service) existingSeriesMerchantKeys(ctx context.Context) (map[string]bool, error) {
	rows, err := s.Pool.Query(ctx, `SELECT DISTINCT merchant_key FROM recurring_series`)
	if err != nil {
		return nil, fmt.Errorf("load existing series keys: %w", err)
	}
	defer rows.Close()
	set := map[string]bool{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, fmt.Errorf("scan series key: %w", err)
		}
		set[k] = true
	}
	return set, rows.Err()
}

// nearMissExplanation renders a one-line human explanation for a group's verdict,
// folding in the actual numbers so the message is concrete.
func nearMissExplanation(diag groupDiagnostics) string {
	switch diag.Reason {
	case "":
		return fmt.Sprintf("Looks like a %s subscription (%d charges) — eligible but not tracked yet.", diag.NearestCadence, diag.OccurrenceCount)
	case seriesRejectTooFewOccurrences:
		return fmt.Sprintf("Only %d charges seen; a %s series needs more before it's trustworthy.", diag.OccurrenceCount, diag.NearestCadence)
	case seriesRejectIrregularCadence:
		return fmt.Sprintf("Charge timing is irregular (~%.0f-day gaps); doesn't match a known cadence.", diag.MedianGapDays)
	case seriesRejectIntervalVariable:
		return fmt.Sprintf("Charge intervals vary too much (interval_cv %.2f) for a reliable %s cadence.", diag.IntervalCV, diag.NearestCadence)
	case seriesRejectAmountUnstable:
		return fmt.Sprintf("Amounts swing too much ($%.2f–$%.2f) to be a steady subscription.", float64(diag.AmountMinCents)/100, float64(diag.AmountMaxCents)/100)
	case seriesRejectSameDayDuplicates:
		return "All charges fall on one day — looks like duplicates, not a recurring cycle."
	case seriesRejectTooFewCharges:
		return "Only one charge so far — nothing to compare yet."
	default:
		return "Did not qualify as a recurring series."
	}
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
