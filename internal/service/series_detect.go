//go:build !lite

package service

import (
	"math"
	"sort"
	"time"
)

// DetectorVersion is bumped when the gate thresholds or analysis change, so a
// persisted detection_signals payload can be compared against the current
// detector. Pairs with sync.MerchantKeyVersion.
const DetectorVersion = 1

// Precision-first gate defaults (§6.2). Tunable via app_config later; hardcoded
// here for v1. The posture optimizes precision over recall — a missed series is
// recoverable, a false positive is corrosive.
const (
	seriesMinOccurrences       = 3    // sub-annual cadences
	seriesMinOccurrencesAnnual = 2    // annual/semiannual (backfill's whole point)
	seriesMaxIntervalCV        = 0.15 // coefficient of variation of day-gaps
	seriesCadenceSnapTol       = 0.15 // |median_gap - center| / center
	seriesAmountAbsFloorCents  = 100  // $1.00 tight-band floor
	seriesAmountPct            = 0.05 // 5% tight-band
	seriesDriftMaxStepPct      = 0.25 // per-renewal price step ceiling
	seriesDriftMaxTotalRatio   = 2.0  // max/min spread ceiling for drift
)

// cadenceCenter maps a canonical cadence to its day-gap center.
type cadenceCenter struct {
	name   string
	center float64
}

var cadenceCenters = []cadenceCenter{
	{SeriesCadenceWeekly, 7},
	{SeriesCadenceBiweekly, 14},
	{SeriesCadenceMonthly, 30.44},
	{SeriesCadenceQuarterly, 91.31},
	{SeriesCadenceSemiannual, 182.62},
	{SeriesCadenceAnnual, 365.25},
}

// chargePoint is one occurrence in a candidate group.
type chargePoint struct {
	date        time.Time
	amountCents int64
}

// detectionSignals is the raw, inspectable evidence the detector used (§6.6).
// Persisted as recurring_series.detection_signals so a reviewing agent/UI can
// calibrate confidence from numbers rather than an opaque flag.
type detectionSignals struct {
	Version           int     `json:"version"`
	MerchantKey       string  `json:"merchant_key"`
	OccurrenceCount   int     `json:"occurrence_count"`
	SpanDays          int     `json:"span_days"`
	IntervalCV        float64 `json:"interval_cv"`
	MedianGapDays     float64 `json:"median_gap_days"`
	Cadence           string  `json:"cadence"`
	CadenceSnapError  float64 `json:"cadence_snap_error"`
	AmountBranch      string  `json:"amount_branch"` // "tight" | "monotonic_drift"
	AmountMedian      float64 `json:"amount_median"`
	AmountMin         float64 `json:"amount_min"`
	AmountMax         float64 `json:"amount_max"`
	AmountSpreadRatio float64 `json:"amount_spread_ratio"`
	AmountMonotonic   bool    `json:"amount_monotonic"`
	Currency          string  `json:"currency"`
	DetectorVersion   int     `json:"detector_version"`
}

// Rejection reasons surfaced by evaluateGroup (and the explain feed). The empty
// string means the group qualifies. These are stable identifiers — agents and
// the UI map them to human copy ("why isn't this a subscription?").
const (
	seriesRejectTooFewCharges     = "too_few_charges"       // fewer than 2 charges — nothing to compare
	seriesRejectSameDayDuplicates = "same_day_duplicates"   // all charges land on one day, no cadence
	seriesRejectIrregularCadence  = "irregular_cadence"     // gap doesn't snap to any known cadence
	seriesRejectTooFewOccurrences = "too_few_occurrences"   // below the precision-first occurrence floor
	seriesRejectIntervalVariable  = "interval_too_variable" // day-gaps too irregular (interval_cv over gate)
	seriesRejectAmountUnstable    = "amount_unstable"       // amounts neither tight-band nor clean drift
)

// groupAnalysis is the verdict for one candidate group.
type groupAnalysis struct {
	cadence             string
	expectedAmountCents int64 // most-recent for drift, median for tight
	expectedDay         *int32
	signals             detectionSignals
}

// groupDiagnostics carries the best-effort numbers computed about a group
// regardless of whether it qualified — so a rejected near-miss can still
// explain itself (how many charges, nearest cadence, interval variance, amount
// spread). Reason is "" when the group qualified.
type groupDiagnostics struct {
	Reason            string
	OccurrenceCount   int
	NearestCadence    string  // closest cadence center even if it didn't snap
	MedianGapDays     float64
	IntervalCV        float64
	AmountMinCents    int64
	AmountMaxCents    int64
	AmountSpreadRatio float64
}

// analyzeGroup applies the precision-first gates to one merchant+currency group
// of charges and decides whether it is a recurring series. Pure arithmetic, no
// I/O — the unit-tested core of the detector. Thin wrapper over evaluateGroup
// preserving the historical (analysis, ok) shape for the detection path.
func analyzeGroup(charges []chargePoint, merchantKey, currency string) (groupAnalysis, bool) {
	a, diag := evaluateGroup(charges, merchantKey, currency)
	return a, diag.Reason == ""
}

// evaluateGroup is the gate core. It returns the accepted analysis (when
// diag.Reason == "") plus diagnostics that are populated as far as the gates
// got — so the explain feed can report *why* a near-miss fell short using the
// same arithmetic the detector used. The accepted path is byte-identical to the
// pre-refactor analyzeGroup.
func evaluateGroup(charges []chargePoint, merchantKey, currency string) (groupAnalysis, groupDiagnostics) {
	diag := groupDiagnostics{OccurrenceCount: len(charges)}
	if len(charges) < 2 {
		diag.Reason = seriesRejectTooFewCharges
		return groupAnalysis{}, diag
	}
	sorted := make([]chargePoint, len(charges))
	copy(sorted, charges)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].date.Before(sorted[j].date) })

	// Amount spread is meaningful for diagnostics regardless of the gate outcome.
	amounts := make([]int64, len(sorted))
	for i, c := range sorted {
		amounts[i] = c.amountCents
	}
	minAmt, maxAmt := minMaxInt(amounts)
	diag.AmountMinCents, diag.AmountMaxCents = minAmt, maxAmt
	if minAmt > 0 {
		diag.AmountSpreadRatio = round3(float64(maxAmt) / float64(minAmt))
	}

	// Day gaps between consecutive charges.
	gaps := make([]float64, 0, len(sorted)-1)
	for i := 1; i < len(sorted); i++ {
		gaps = append(gaps, sorted[i].date.Sub(sorted[i-1].date).Hours()/24)
	}
	medGap := median(gaps)
	diag.MedianGapDays = round3(medGap)
	if medGap <= 0 {
		diag.Reason = seriesRejectSameDayDuplicates
		return groupAnalysis{}, diag
	}

	cadence, snapErr := snapCadence(medGap)
	diag.NearestCadence = nearestCadenceName(medGap)
	if cadence == SeriesCadenceIrregular {
		diag.Reason = seriesRejectIrregularCadence // precision-first: detector never emits irregular
		return groupAnalysis{}, diag
	}

	// Occurrence floor depends on cadence: annual/semiannual qualify at 2.
	floor := seriesMinOccurrences
	if cadence == SeriesCadenceAnnual || cadence == SeriesCadenceSemiannual {
		floor = seriesMinOccurrencesAnnual
	}
	if len(sorted) < floor {
		diag.NearestCadence = cadence
		diag.Reason = seriesRejectTooFewOccurrences
		return groupAnalysis{}, diag
	}

	// Interval regularity — only meaningful with ≥2 gaps. The 2-charge annual
	// case relies on the cadence-snap gate instead.
	cv := coeffVar(gaps)
	diag.IntervalCV = round3(cv)
	diag.NearestCadence = cadence
	if len(gaps) >= 2 && cv > seriesMaxIntervalCV {
		diag.Reason = seriesRejectIntervalVariable
		return groupAnalysis{}, diag
	}

	// Amount stability: tight band OR monotonic-drift (gated on clean cadence).
	branch, ok := amountStability(amounts, snapErr)
	if !ok {
		diag.Reason = seriesRejectAmountUnstable
		return groupAnalysis{}, diag
	}

	medAmt := medianInt(amounts)
	expected := medAmt
	if branch == "monotonic_drift" {
		expected = amounts[len(amounts)-1] // current price renews
	}
	spread := 1.0
	if minAmt > 0 {
		spread = float64(maxAmt) / float64(minAmt)
	}

	sig := detectionSignals{
		Version:           1,
		MerchantKey:       merchantKey,
		OccurrenceCount:   len(sorted),
		SpanDays:          int(sorted[len(sorted)-1].date.Sub(sorted[0].date).Hours() / 24),
		IntervalCV:        round3(cv),
		MedianGapDays:     round3(medGap),
		Cadence:           cadence,
		CadenceSnapError:  round3(snapErr),
		AmountBranch:      branch,
		AmountMedian:      float64(medAmt) / 100,
		AmountMin:         float64(minAmt) / 100,
		AmountMax:         float64(maxAmt) / 100,
		AmountSpreadRatio: round3(spread),
		AmountMonotonic:   isMonotonic(amounts),
		Currency:          currency,
		DetectorVersion:   DetectorVersion,
	}
	return groupAnalysis{
		cadence:             cadence,
		expectedAmountCents: expected,
		expectedDay:         modalDayOfMonth(sorted, cadence),
		signals:             sig,
	}, diag
}

// nearestCadenceName returns the closest cadence center to a median gap,
// ignoring the snap tolerance — used for diagnostics so an irregular near-miss
// can still say "looks roughly monthly".
func nearestCadenceName(medGap float64) string {
	best := SeriesCadenceIrregular
	bestErr := math.MaxFloat64
	for _, c := range cadenceCenters {
		if e := math.Abs(medGap-c.center) / c.center; e < bestErr {
			bestErr, best = e, c.name
		}
	}
	return best
}

// snapCadence maps a median day-gap to a canonical cadence if it lands within
// the snap tolerance of a center; otherwise "irregular". Returns the relative
// snap error to the chosen center.
func snapCadence(medGap float64) (string, float64) {
	bestName := SeriesCadenceIrregular
	bestErr := math.MaxFloat64
	for _, c := range cadenceCenters {
		e := math.Abs(medGap-c.center) / c.center
		if e < bestErr {
			bestErr = e
			bestName = c.name
		}
	}
	if bestErr > seriesCadenceSnapTol {
		return SeriesCadenceIrregular, bestErr
	}
	return bestName, bestErr
}

// amountStability returns the satisfying branch ("tight" or "monotonic_drift")
// or ok=false. Drift is permitted only on a rock-solid cadence (snapErr small),
// monotonic, with bounded per-step and total spread — so a real price-changing
// subscription survives while random scatter is rejected.
func amountStability(amounts []int64, snapErr float64) (string, bool) {
	med := medianInt(amounts)
	if med <= 0 {
		return "", false
	}
	tol := int64(seriesAmountAbsFloorCents)
	if pct := int64(float64(med) * seriesAmountPct); pct > tol {
		tol = pct
	}
	tight := true
	for _, a := range amounts {
		if abs64(a-med) > tol {
			tight = false
			break
		}
	}
	if tight {
		return "tight", true
	}

	// Drift branch — gated on clean cadence, not a stricter CV.
	if snapErr > seriesCadenceSnapTol {
		return "", false
	}
	if !isMonotonic(amounts) {
		return "", false
	}
	for i := 1; i < len(amounts); i++ {
		prev, cur := amounts[i-1], amounts[i]
		if prev <= 0 {
			return "", false
		}
		step := math.Abs(float64(cur-prev)) / float64(prev)
		if step > seriesDriftMaxStepPct {
			return "", false
		}
	}
	mn, mx := minMaxInt(amounts)
	if mn <= 0 || float64(mx)/float64(mn) > seriesDriftMaxTotalRatio {
		return "", false
	}
	return "monotonic_drift", true
}

// modalDayOfMonth returns the most common day-of-month for monthly+ cadences;
// nil for weekly/biweekly and sub-monthly where a single integer is meaningless.
func modalDayOfMonth(charges []chargePoint, cadence string) *int32 {
	switch cadence {
	case SeriesCadenceMonthly, SeriesCadenceQuarterly, SeriesCadenceSemiannual, SeriesCadenceAnnual:
	default:
		return nil
	}
	counts := map[int]int{}
	best, bestN := 0, 0
	for _, c := range charges {
		d := c.date.Day()
		counts[d]++
		if counts[d] > bestN {
			bestN, best = counts[d], d
		}
	}
	if best == 0 {
		return nil
	}
	v := int32(best)
	return &v
}

// --- small numeric helpers ---

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	cp := append([]float64(nil), xs...)
	sort.Float64s(cp)
	n := len(cp)
	if n%2 == 1 {
		return cp[n/2]
	}
	return (cp[n/2-1] + cp[n/2]) / 2
}

func medianInt(xs []int64) int64 {
	if len(xs) == 0 {
		return 0
	}
	cp := append([]int64(nil), xs...)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	n := len(cp)
	if n%2 == 1 {
		return cp[n/2]
	}
	return (cp[n/2-1] + cp[n/2]) / 2
}

func coeffVar(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var sum float64
	for _, x := range xs {
		sum += x
	}
	mean := sum / float64(len(xs))
	if mean == 0 {
		return math.MaxFloat64
	}
	var variance float64
	for _, x := range xs {
		d := x - mean
		variance += d * d
	}
	variance /= float64(len(xs))
	return math.Sqrt(variance) / mean
}

func isMonotonic(xs []int64) bool {
	if len(xs) < 2 {
		return true
	}
	nonDec, nonInc := true, true
	for i := 1; i < len(xs); i++ {
		if xs[i] < xs[i-1] {
			nonDec = false
		}
		if xs[i] > xs[i-1] {
			nonInc = false
		}
	}
	return nonDec || nonInc
}

func minMaxInt(xs []int64) (int64, int64) {
	mn, mx := xs[0], xs[0]
	for _, x := range xs[1:] {
		if x < mn {
			mn = x
		}
		if x > mx {
			mx = x
		}
	}
	return mn, mx
}

func abs64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

func round3(f float64) float64 {
	return math.Round(f*1000) / 1000
}
