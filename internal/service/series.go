//go:build !lite

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/slugs"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Recurring-series enum vocabularies (mirrors the CHECK constraints in the
// recurring_series migration). Kept here so the service validates before the DB.
const (
	SeriesCadenceWeekly     = "weekly"
	SeriesCadenceBiweekly   = "biweekly"
	SeriesCadenceMonthly    = "monthly"
	SeriesCadenceQuarterly  = "quarterly"
	SeriesCadenceSemiannual = "semiannual"
	SeriesCadenceAnnual     = "annual"
	SeriesCadenceIrregular  = "irregular"
	SeriesCadenceUnknown    = "unknown"

	SeriesStatusActive    = "active"
	SeriesStatusPaused    = "paused"
	SeriesStatusCancelled = "cancelled"
	SeriesStatusCandidate = "candidate"

	SeriesSourceDeterministic = "deterministic"
	SeriesSourceAgent         = "agent"
	SeriesSourceUser          = "user"
	SeriesSourceRule          = "rule"

	SeriesConfidenceAuto      = "auto"
	SeriesConfidenceConfirmed = "confirmed"
	SeriesConfidenceRejected  = "rejected"

	// Recurring-charge type — the structured classification axis (mirrors the
	// CHECK in the type migration). "subscription" is one type, not the umbrella.
	SeriesTypeSubscription = "subscription" // streaming, SaaS, memberships
	SeriesTypeBill         = "bill"         // rent, utilities, insurance, telecom
	SeriesTypeLoan         = "loan"         // mortgage, auto/student/personal loans
	SeriesTypeOther        = "other"        // recurring but uncategorized

	// Renewal-health buckets — a derived (not stored) read-side signal computed
	// from an active series' projected next_expected_date relative to today.
	// Surfaced on SeriesResponse so agents and the UI can answer "what renews
	// soon" and "what looks cancelled" without re-deriving the cadence math.
	SeriesHealthActive  = "active"   // next charge comfortably in the future
	SeriesHealthDueSoon = "due_soon" // renews within the next week
	SeriesHealthOverdue = "overdue"  // past due but within one cadence cycle (likely just lag)
	SeriesHealthStale   = "stale"    // missed a full cadence cycle — likely cancelled
	SeriesHealthUnknown = "unknown"  // no prediction (irregular/unknown cadence or no charges yet)
)

// renewalDueSoonWindowDays is how far ahead a projected charge still counts as
// "due soon" rather than merely "active".
const renewalDueSoonWindowDays = 7

var validSeriesCadence = map[string]bool{
	SeriesCadenceWeekly: true, SeriesCadenceBiweekly: true, SeriesCadenceMonthly: true,
	SeriesCadenceQuarterly: true, SeriesCadenceSemiannual: true, SeriesCadenceAnnual: true,
	SeriesCadenceIrregular: true, SeriesCadenceUnknown: true,
}

var validSeriesSource = map[string]bool{
	SeriesSourceDeterministic: true, SeriesSourceAgent: true,
	SeriesSourceUser: true, SeriesSourceRule: true,
}

var validSeriesType = map[string]bool{
	SeriesTypeSubscription: true, SeriesTypeBill: true,
	SeriesTypeLoan: true, SeriesTypeOther: true,
}

// inferSeriesType maps a (Plaid PFC) category slug to a recurring-charge type.
// Prefix-based so it's robust to the full taxonomy; unknown/empty → subscription
// (the most common recurring charge that isn't a bill or loan).
func inferSeriesType(categorySlug string) string {
	switch {
	case categorySlug == "":
		return SeriesTypeSubscription
	case categorySlug == "loan_payments_insurance_payment":
		return SeriesTypeBill // insurance is a bill, not a loan
	case categorySlug == "loan_payments_credit_card_payment":
		return SeriesTypeOther // a card payment is a transfer, not a tracked sub/bill/loan
	case strings.HasPrefix(categorySlug, "loan_payments"):
		return SeriesTypeLoan // mortgage, auto, student, personal
	case strings.HasPrefix(categorySlug, "rent_and_utilities"):
		return SeriesTypeBill // rent, gas/electric, internet/cable, phone, water
	case categorySlug == "general_services_insurance":
		return SeriesTypeBill
	case strings.HasPrefix(categorySlug, "entertainment"):
		return SeriesTypeSubscription // streaming, music, games
	default:
		return SeriesTypeSubscription
	}
}

// SeriesVerdict is a human/agent adjudication applied via ReviewSeries.
type SeriesVerdict string

const (
	VerdictConfirm SeriesVerdict = "confirm"
	VerdictReject  SeriesVerdict = "reject"
	VerdictPause   SeriesVerdict = "pause"
	VerdictCancel  SeriesVerdict = "cancel"
)

// SeriesUpsert is the input to the detection funnel. The deterministic detector,
// rule actions, and agents all funnel proposals through it. MerchantKey is
// required — a NULL-merchant charge never produces a series this way (it joins a
// series only via an explicit manual/agent/rule link).
type SeriesUpsert struct {
	UserID           *string  // short_id or uuid; nil = shared/household
	Name             string   // display label; defaults to MerchantKey when empty
	MerchantKey      string   // required, the dedup anchor
	Cadence          string   // defaults to "unknown"
	ExpectedDay      *int32   // day-of-month / day-of-week, when known
	ExpectedAmount   *float64 // dollars, paired with Currency
	AmountTolerance  *float64 // dollars; defaults to 1.00 on insert
	Currency         *string
	CategoryID       *string  // short_id or uuid; advisory
	Source           string   // deterministic|agent|user|rule (defaults deterministic)
	Type             string   // subscription|bill|loan|other — explicit assertion (caller); when empty, inferred at first detection from the members' dominant category, then sticky
	MemberTxnIDs     []string // transactions to back-link (short_id or uuid)
	DetectionSignals []byte   // raw JSON signals (§6.6)
}

// SeriesResponse is the API/MCP shape of a recurring_series row.
type SeriesResponse struct {
	ID               string          `json:"id"`
	ShortID          string          `json:"short_id"`
	UserID           *string         `json:"user_id,omitempty"`
	Name             string          `json:"name"`
	MerchantKey      string          `json:"merchant_key"`
	Cadence          string          `json:"cadence"`
	ExpectedDay      *int            `json:"expected_day,omitempty"`
	ExpectedAmount   *float64        `json:"expected_amount,omitempty"`
	AmountTolerance  *float64        `json:"amount_tolerance,omitempty"`
	IsoCurrencyCode  *string         `json:"iso_currency_code,omitempty"`
	CategoryID       *string         `json:"category_id,omitempty"`
	Status           string          `json:"status"`
	Type             string          `json:"type"`
	DetectionSource  string          `json:"detection_source"`
	Confidence       string          `json:"confidence"`
	ConfirmedByType  *string         `json:"confirmed_by_type,omitempty"`
	LastAmount       *float64        `json:"last_amount,omitempty"`
	LastSeenDate     *string         `json:"last_seen_date,omitempty"`
	NextExpectedDate *string         `json:"next_expected_date,omitempty"`
	// RenewalHealth is a derived bucket (active|due_soon|overdue|stale|unknown)
	// computed from NextExpectedDate vs today; only populated for active series.
	RenewalHealth string `json:"renewal_health,omitempty"`
	// DaysUntilRenewal is the signed day count to NextExpectedDate (negative =
	// overdue). Nil when there's no prediction. Populated for active series.
	DaysUntilRenewal *int `json:"days_until_renewal,omitempty"`
	OccurrenceCount  int  `json:"occurrence_count"`
	DetectionSignals json.RawMessage `json:"detection_signals,omitempty"`
	Tags             []string        `json:"tags,omitempty"`
	CreatedAt        string          `json:"created_at"`
	UpdatedAt        string          `json:"updated_at"`
}

// AssignSeriesInput is the input to AssignSeries — the imperative create/link
// path an agent or user drives (the MCP/REST `assign_series` tool). Either
// assign to an existing series (SeriesID) or mint one by signature
// (MerchantKey + CreateIfMissing); then back-link members and optionally
// confirm in the same call.
type AssignSeriesInput struct {
	SeriesID        *string  // short_id or uuid — assign to an existing series
	MerchantKey     string   // required when minting (CreateIfMissing)
	CreateIfMissing bool     // mint a series if no SeriesID
	Name            string   // optional display label for a minted series
	Cadence         string   // optional proposed cadence for a minted series
	Type            string   // optional subscription|bill|loan|other for a minted series
	ExpectedAmount  *float64 // optional, paired with Currency
	Currency        *string
	CategoryID      *string  // short_id or uuid, advisory
	UserID          *string  // short_id or uuid; nil = shared/household
	TransactionIDs  []string // members to back-link (short_id or uuid), ≤50
	Confirm         bool     // flip straight to confirmed/active in the same call
}

// seriesAssignMaxMembers caps the per-call back-link batch (mirrors the
// update_transactions 50-op ceiling).
const seriesAssignMaxMembers = 50

// AssignSeries is the imperative create/link entry point shared by the
// `assign_series` MCP tool and REST endpoints. It funnels through
// UpsertSeriesCandidate (mint path) or a direct link (existing-series path),
// so the precedence ladder + sticky-reject arbitrate uniformly, then applies
// an optional confirm verdict. Source is derived from the actor (user|agent).
func (s *Service) AssignSeries(ctx context.Context, in AssignSeriesInput, actor Actor) (*SeriesResponse, error) {
	if len(in.TransactionIDs) > seriesAssignMaxMembers {
		return nil, fmt.Errorf("%w: at most %d transactions per call", ErrInvalidParameter, seriesAssignMaxMembers)
	}

	var resp *SeriesResponse
	switch {
	case in.SeriesID != nil && strings.TrimSpace(*in.SeriesID) != "":
		linked, err := s.linkSeriesMembers(ctx, *in.SeriesID, in.TransactionIDs)
		if err != nil {
			return nil, err
		}
		resp = linked
	case in.CreateIfMissing && strings.TrimSpace(in.MerchantKey) != "":
		minted, err := s.UpsertSeriesCandidate(ctx, SeriesUpsert{
			UserID:         in.UserID,
			Name:           in.Name,
			MerchantKey:    in.MerchantKey,
			Cadence:        in.Cadence,
			Type:           in.Type,
			ExpectedAmount: in.ExpectedAmount,
			Currency:       in.Currency,
			CategoryID:     in.CategoryID,
			Source:         seriesSourceForActor(actor),
			MemberTxnIDs:   in.TransactionIDs,
		}, actor)
		if err != nil {
			return nil, err
		}
		resp = minted
	default:
		return nil, fmt.Errorf("%w: provide series_id, or merchant_key with create_if_missing", ErrInvalidParameter)
	}

	if in.Confirm {
		return s.ReviewSeries(ctx, resp.ShortID, VerdictConfirm, actor)
	}
	return resp, nil
}

// AssignSeriesFromRuleTx materializes an `assign_series` rule action INSIDE the
// sync transaction (the engine calls this via the Engine.AssignSeriesInTx hook,
// so the sync package never imports service). It resolves the target series by
// short_id, or matches/mints one by merchant_key signature, then back-links the
// single transaction (NULL-fill only) and recomputes rollups — all on the
// provided tx so it commits atomically with the sync.
//
// It is deliberately link-and-rollup only: it never sharpens cadence/signals,
// so it can't downgrade a detector's snapped values (PATCH A is moot here).
// Sticky-reject is honored (a rejected signature is skipped). Failures to
// resolve are returned as nil (the rule no-ops) rather than failing the sync.
func (s *Service) AssignSeriesFromRuleTx(ctx context.Context, tx pgx.Tx, txnID pgtype.UUID, seriesShortID, merchantKey string, createIfMissing bool) error {
	qtx := s.Queries.WithTx(tx)

	var seriesID pgtype.UUID
	switch {
	case strings.TrimSpace(seriesShortID) != "":
		id, err := qtx.GetRecurringSeriesUUIDByShortID(ctx, seriesShortID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil // rule references a missing series — no-op, don't break sync
			}
			return fmt.Errorf("resolve series %q: %w", seriesShortID, err)
		}
		seriesID = id
	case createIfMissing && strings.TrimSpace(merchantKey) != "":
		key := strings.TrimSpace(merchantKey)
		existing, matchErr := qtx.MatchSeriesForUpdate(ctx, db.MatchSeriesForUpdateParams{
			MerchantKey:     key,
			IsoCurrencyCode: pgtype.Text{}, // rule mints a household-level series
			UserID:          pgtype.UUID{},
		})
		switch {
		case matchErr == nil:
			if existing.Confidence == SeriesConfidenceRejected {
				return nil // sticky reject — never re-mint at this signature
			}
			seriesID = existing.ID
		case errors.Is(matchErr, pgx.ErrNoRows):
			inserted, err := qtx.InsertRecurringSeries(ctx, db.InsertRecurringSeriesParams{
				Name:            key,
				MerchantKey:     key,
				Cadence:         SeriesCadenceUnknown,
				AmountTolerance: pgconv.NumericCents(100),
				Status:          SeriesStatusCandidate,
				DetectionSource: SeriesSourceRule,
				Confidence:      SeriesConfidenceAuto,
			})
			if err != nil {
				return fmt.Errorf("insert rule series: %w", err)
			}
			seriesID = inserted.ID
		default:
			return fmt.Errorf("match series: %w", matchErr)
		}
	default:
		return nil // nothing actionable
	}

	if _, err := qtx.BackLinkSeriesMembers(ctx, db.BackLinkSeriesMembersParams{
		SeriesID:       seriesID,
		TransactionIds: []pgtype.UUID{txnID},
	}); err != nil {
		return fmt.Errorf("back-link series member: %w", err)
	}
	if err := qtx.ApplySeriesTagsToTransactions(ctx, db.ApplySeriesTagsToTransactionsParams{
		SeriesID: seriesID,
		Column2:  []pgtype.UUID{txnID},
	}); err != nil {
		return fmt.Errorf("apply series tags to member: %w", err)
	}

	row, err := qtx.GetRecurringSeriesByID(ctx, seriesID)
	if err != nil {
		return fmt.Errorf("reload series: %w", err)
	}
	occCount, lastAmount, lastSeen, err := s.seriesRollup(ctx, qtx, seriesID)
	if err != nil {
		return err
	}
	upd := updateParamsFromRow(row)
	upd.OccurrenceCount = occCount
	upd.LastAmount = lastAmount
	upd.LastSeenDate = lastSeen
	upd.NextExpectedDate = nextExpectedDate(upd.Cadence, lastSeen)
	if _, err := qtx.UpdateRecurringSeries(ctx, upd); err != nil {
		return fmt.Errorf("update series rollup: %w", err)
	}
	return nil
}

// linkSeriesMembers back-links transactions to an existing series (NULL-fill
// only) and recomputes its rollups, in one transaction. Used by the
// existing-series branch of AssignSeries.
func (s *Service) linkSeriesMembers(ctx context.Context, idOrShort string, memberIDsOrShorts []string) (*SeriesResponse, error) {
	id, err := s.resolveSeriesID(ctx, idOrShort)
	if err != nil {
		return nil, err
	}
	memberIDs, err := s.resolveTransactionIDs(ctx, memberIDsOrShorts)
	if err != nil {
		return nil, err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin link members: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := s.Queries.WithTx(tx)

	row, err := qtx.GetRecurringSeriesByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get series: %w", err)
	}
	if len(memberIDs) > 0 {
		if _, err := qtx.BackLinkSeriesMembers(ctx, db.BackLinkSeriesMembersParams{
			SeriesID:       row.ID,
			TransactionIds: memberIDs,
		}); err != nil {
			return nil, fmt.Errorf("back-link members: %w", err)
		}
		if err := qtx.ApplySeriesTagsToTransactions(ctx, db.ApplySeriesTagsToTransactionsParams{
			SeriesID: row.ID,
			Column2:  memberIDs,
		}); err != nil {
			return nil, fmt.Errorf("apply series tags to members: %w", err)
		}
	}
	occCount, lastAmount, lastSeen, err := s.seriesRollup(ctx, qtx, row.ID)
	if err != nil {
		return nil, err
	}
	upd := updateParamsFromRow(row)
	upd.OccurrenceCount = occCount
	upd.LastAmount = lastAmount
	upd.LastSeenDate = lastSeen
	upd.NextExpectedDate = nextExpectedDate(upd.Cadence, lastSeen)
	updated, err := qtx.UpdateRecurringSeries(ctx, upd)
	if err != nil {
		return nil, fmt.Errorf("update series: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit link members: %w", err)
	}
	resp := seriesFromRow(updated)
	return &resp, nil
}

// seriesSourceForActor maps an actor to the detection_source vocabulary for an
// imperative write: a real user → "user", everything else (agents, system) →
// "agent". Rule-authored writes set Source explicitly and don't use this.
func seriesSourceForActor(actor Actor) string {
	if actor.Type == "user" {
		return SeriesSourceUser
	}
	return SeriesSourceAgent
}

// UpsertSeriesCandidate is the single writer of recurring_series from detection.
// It matches the dedup signature under a row lock, inserts a fresh candidate or
// refreshes an existing one per the precedence ladder, back-links member
// transactions (NULL-fill only), and recomputes rollups — all in one
// transaction so concurrent detector/agent writers converge instead of forking.
func (s *Service) UpsertSeriesCandidate(ctx context.Context, in SeriesUpsert, actor Actor) (*SeriesResponse, error) {
	merchantKey := strings.TrimSpace(in.MerchantKey)
	if merchantKey == "" {
		return nil, fmt.Errorf("%w: merchant_key is required", ErrInvalidParameter)
	}
	cadence := in.Cadence
	if cadence == "" {
		cadence = SeriesCadenceUnknown
	}
	if !validSeriesCadence[cadence] {
		return nil, fmt.Errorf("%w: invalid cadence %q", ErrInvalidParameter, cadence)
	}
	source := in.Source
	if source == "" {
		source = SeriesSourceDeterministic
	}
	if !validSeriesSource[source] {
		return nil, fmt.Errorf("%w: invalid source %q", ErrInvalidParameter, source)
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = merchantKey
	}

	userID, err := s.resolveOptionalUserID(ctx, in.UserID)
	if err != nil {
		return nil, err
	}
	categoryID, err := s.resolveOptionalCategoryID(ctx, in.CategoryID)
	if err != nil {
		return nil, err
	}
	memberIDs, err := s.resolveTransactionIDs(ctx, in.MemberTxnIDs)
	if err != nil {
		return nil, err
	}
	currency := pgconv.TextPtrIfNotEmpty(in.Currency)

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin series upsert: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := s.Queries.WithTx(tx)

	// 1. Match the signature under a row lock.
	existing, matchErr := qtx.MatchSeriesForUpdate(ctx, db.MatchSeriesForUpdateParams{
		MerchantKey:     merchantKey,
		IsoCurrencyCode: currency,
		UserID:          userID,
	})
	haveExisting := matchErr == nil
	if matchErr != nil && !errors.Is(matchErr, pgx.ErrNoRows) {
		return nil, fmt.Errorf("match series: %w", matchErr)
	}

	// 2. A rejected verdict is sticky — never re-propose. Return it untouched.
	if haveExisting && existing.Confidence == SeriesConfidenceRejected {
		resp := seriesFromRow(existing)
		return &resp, nil
	}

	// 3. Establish the target row (insert a fresh candidate, or reuse the match).
	var base db.RecurringSeries
	if haveExisting {
		base = existing
	} else {
		tolerance := pgconv.NumericCents(100) // $1.00 default
		if in.AmountTolerance != nil {
			tolerance = numericDollars(*in.AmountTolerance)
		}
		base, err = qtx.InsertRecurringSeries(ctx, db.InsertRecurringSeriesParams{
			UserID:           userID,
			Name:             name,
			MerchantKey:      merchantKey,
			Cadence:          cadence,
			ExpectedDay:      int4Ptr(in.ExpectedDay),
			ExpectedAmount:   numericDollarsPtr(in.ExpectedAmount),
			AmountTolerance:  tolerance,
			IsoCurrencyCode:  currency,
			CategoryID:       categoryID,
			Status:           SeriesStatusCandidate,
			DetectionSource:  source,
			Confidence:       SeriesConfidenceAuto,
			ConfirmedByType:  pgtype.Text{},
			LastAmount:       pgtype.Numeric{},
			LastSeenDate:     pgtype.Date{},
			NextExpectedDate: pgtype.Date{},
			OccurrenceCount:  0,
			DetectionSignals: in.DetectionSignals,
		})
		if err != nil {
			return nil, fmt.Errorf("insert series: %w", err)
		}
	}

	// 4. Back-link member transactions (NULL-fill only) and materialize the
	// series' tags onto the freshly-linked members (NULL-fill via ON CONFLICT).
	if len(memberIDs) > 0 {
		if _, err := qtx.BackLinkSeriesMembers(ctx, db.BackLinkSeriesMembersParams{
			SeriesID:       base.ID,
			TransactionIds: memberIDs,
		}); err != nil {
			return nil, fmt.Errorf("back-link members: %w", err)
		}
		if err := qtx.ApplySeriesTagsToTransactions(ctx, db.ApplySeriesTagsToTransactionsParams{
			SeriesID: base.ID,
			Column2:  memberIDs,
		}); err != nil {
			return nil, fmt.Errorf("apply series tags to members: %w", err)
		}
	}

	// 5. Recompute rollups from the live members.
	occCount, lastAmount, lastSeen, err := s.seriesRollup(ctx, qtx, base.ID)
	if err != nil {
		return nil, err
	}

	// 6. Build the merged update per the precedence ladder.
	upd := updateParamsFromRow(base)
	upd.OccurrenceCount = occCount
	upd.LastAmount = lastAmount
	upd.LastSeenDate = lastSeen

	// Type: sticky once set. updateParamsFromRow already preserved base.Type. On
	// the FIRST detection of a series (fresh insert) infer it from the members'
	// dominant category; on later writes leave it alone so a re-detect can't
	// override a refined value. An explicit caller assertion (in.Type) always
	// wins — that's how an agent/rule sets the type directly.
	if !haveExisting {
		if t := s.inferTypeFromMembers(ctx, qtx, base.ID); t != "" {
			upd.Type = t
		}
	}
	if in.Type != "" {
		if !validSeriesType[in.Type] {
			return nil, fmt.Errorf("%w: invalid type %q", ErrInvalidParameter, in.Type)
		}
		upd.Type = in.Type
	}

	// An unadjudicated (auto) candidate may have its proposed fields sharpened
	// by a fresh write; a confirmed series gets rollups only (its adjudicated
	// fields are sacred). Confidence/status are never downgraded by the proposal
	// path — verdicts flow through ReviewSeries.
	//
	// PATCH A — source-precedence guard. A write may sharpen the proposed fields
	// only when its source ranks >= the row's current detection_source on the
	// ladder deterministic < rule < agent < user. This stops a lower-precedence
	// writer (e.g. the next deterministic re-detect, or a thin rule-authored
	// link) from clobbering values a higher-precedence actor deliberately set.
	// Two extra carve-outs make a thin write safe regardless of rank:
	//   - never downgrade a known cadence to "unknown" (a link-only write that
	//     passes no cadence must not erase the detector's snapped value);
	//   - never null detection_signals (already guarded by the len>0 check).
	if base.Confidence == SeriesConfidenceAuto &&
		seriesSourceRank(source) >= seriesSourceRank(base.DetectionSource) {
		if cadence != SeriesCadenceUnknown || base.Cadence == SeriesCadenceUnknown {
			upd.Cadence = cadence
		}
		if in.ExpectedAmount != nil {
			upd.ExpectedAmount = numericDollars(*in.ExpectedAmount)
		}
		if in.ExpectedDay != nil {
			upd.ExpectedDay = pgconv.Int4(*in.ExpectedDay)
		}
		if in.AmountTolerance != nil {
			upd.AmountTolerance = numericDollars(*in.AmountTolerance)
		}
		if categoryID.Valid && !base.CategoryID.Valid {
			upd.CategoryID = categoryID
		}
		if strings.TrimSpace(base.Name) == "" || base.Name == base.MerchantKey {
			upd.Name = name
		}
		if len(in.DetectionSignals) > 0 {
			upd.DetectionSignals = in.DetectionSignals
		}
		// Record who last shaped the row so precedence reflects it on the next write.
		upd.DetectionSource = source
	}
	upd.NextExpectedDate = nextExpectedDate(upd.Cadence, upd.LastSeenDate)

	updated, err := qtx.UpdateRecurringSeries(ctx, upd)
	if err != nil {
		return nil, fmt.Errorf("update series: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit series upsert: %w", err)
	}
	resp := seriesFromRow(updated)
	return &resp, nil
}

// ReviewSeries applies a human/agent verdict to a series. Confirm/reject are
// the durable verdict on the confidence axis; pause/cancel move the lifecycle
// status. A user's prior confirmation outranks a later agent write.
func (s *Service) ReviewSeries(ctx context.Context, idOrShort string, verdict SeriesVerdict, actor Actor) (*SeriesResponse, error) {
	id, err := s.resolveSeriesID(ctx, idOrShort)
	if err != nil {
		return nil, err
	}
	row, err := s.Queries.GetRecurringSeriesByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get series: %w", err)
	}

	// User outranks agent on adjudicated rows: an agent cannot overturn a
	// user's confirm/reject verdict.
	if (verdict == VerdictConfirm || verdict == VerdictReject) &&
		row.Confidence == SeriesConfidenceConfirmed &&
		row.ConfirmedByType.Valid && row.ConfirmedByType.String == "user" &&
		actor.Type == "agent" {
		resp := seriesFromRow(row)
		return &resp, nil
	}

	upd := updateParamsFromRow(row)
	switch verdict {
	case VerdictConfirm:
		upd.Confidence = SeriesConfidenceConfirmed
		upd.Status = SeriesStatusActive
		upd.ConfirmedByType = pgconv.Text(confirmerType(actor))
		upd.DetectionSource = confirmerType(actor)
	case VerdictReject:
		upd.Confidence = SeriesConfidenceRejected
		upd.ConfirmedByType = pgconv.Text(confirmerType(actor))
	case VerdictPause:
		upd.Status = SeriesStatusPaused
	case VerdictCancel:
		upd.Status = SeriesStatusCancelled
	default:
		return nil, fmt.Errorf("%w: unknown verdict %q", ErrInvalidParameter, verdict)
	}

	updated, err := s.Queries.UpdateRecurringSeries(ctx, upd)
	if err != nil {
		return nil, fmt.Errorf("review series: %w", err)
	}
	resp := seriesFromRow(updated)
	return &resp, nil
}

// RekeySeries changes a series' merchant_key (correcting a wrong or over-merged
// detection key) and repoints its members' transactions.merchant_key to match,
// so the series and its history stay consistent under the new key. It refuses
// to silently merge: if a live series already exists at the new signature
// (merchant_key + currency + user), or that signature is sticky-rejected, it
// errors and tells the caller to move members instead.
//
// Scope note: incoming charges still derive their key from the provider name at
// sync time, so a future charge of this merchant lands under the
// provider-derived key, not the re-keyed one — re-key corrects the *historical*
// grouping, not the normalizer. A merchant-key alias table is future work.
func (s *Service) RekeySeries(ctx context.Context, idOrShort, newKey string, actor Actor) (*SeriesResponse, error) {
	newKey = strings.TrimSpace(newKey)
	if newKey == "" {
		return nil, fmt.Errorf("%w: new merchant_key is required", ErrInvalidParameter)
	}
	id, err := s.resolveSeriesID(ctx, idOrShort)
	if err != nil {
		return nil, err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin rekey: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := s.Queries.WithTx(tx)

	row, err := qtx.GetRecurringSeriesByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get series: %w", err)
	}
	if strings.TrimSpace(row.MerchantKey) == newKey {
		resp := seriesFromRow(row)
		return &resp, nil // no-op: already at this key
	}

	// Collision / sticky-reject guard on the target signature.
	match, mErr := qtx.MatchSeriesForUpdate(ctx, db.MatchSeriesForUpdateParams{
		MerchantKey:     newKey,
		IsoCurrencyCode: row.IsoCurrencyCode,
		UserID:          row.UserID,
	})
	if mErr != nil && !errors.Is(mErr, pgx.ErrNoRows) {
		return nil, fmt.Errorf("match target key: %w", mErr)
	}
	if mErr == nil && match.ID != row.ID {
		if match.Confidence == SeriesConfidenceRejected {
			return nil, fmt.Errorf("%w: %q is a sticky-rejected signature; re-key would resurrect it", ErrInvalidParameter, newKey)
		}
		return nil, fmt.Errorf("%w: a series already exists at %q — re-key won't merge; move members there instead", ErrInvalidParameter, newKey)
	}

	// Repoint members' merchant_key so detection stays consistent, then the series'.
	if _, err := tx.Exec(ctx,
		`UPDATE transactions SET merchant_key = $2, updated_at = NOW() WHERE series_id = $1 AND deleted_at IS NULL`,
		row.ID, newKey); err != nil {
		return nil, fmt.Errorf("repoint member keys: %w", err)
	}
	upd := updateParamsFromRow(row)
	upd.MerchantKey = newKey
	updated, err := qtx.UpdateRecurringSeries(ctx, upd)
	if err != nil {
		return nil, fmt.Errorf("update series key: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit rekey: %w", err)
	}
	resp := seriesFromRow(updated)
	return &resp, nil
}

// SplitSeries moves a subset of a series' members into a brand-new series under
// newKey — the fix for an over-grouped series (e.g. a variable $4.99 charge
// swept in with a $139/yr renewal). The new series inherits the source's
// currency / user / category / cadence as a starting point; rollups recompute
// on both sides. It errors if newKey equals the source key, if a live series
// already exists at newKey, or if any listed transaction isn't a current member
// of the source — so a split never steals from a third series or no-ops silently.
func (s *Service) SplitSeries(ctx context.Context, idOrShort string, memberIDsOrShorts []string, newKey, newName string, actor Actor) (*SeriesResponse, error) {
	newKey = strings.TrimSpace(newKey)
	if newKey == "" {
		return nil, fmt.Errorf("%w: new merchant_key is required", ErrInvalidParameter)
	}
	if len(memberIDsOrShorts) == 0 {
		return nil, fmt.Errorf("%w: at least one transaction to split out is required", ErrInvalidParameter)
	}
	if len(memberIDsOrShorts) > seriesAssignMaxMembers {
		return nil, fmt.Errorf("%w: at most %d transactions per split", ErrInvalidParameter, seriesAssignMaxMembers)
	}
	id, err := s.resolveSeriesID(ctx, idOrShort)
	if err != nil {
		return nil, err
	}
	memberIDs, err := s.resolveTransactionIDs(ctx, memberIDsOrShorts)
	if err != nil {
		return nil, err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin split: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := s.Queries.WithTx(tx)

	src, err := qtx.GetRecurringSeriesByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get series: %w", err)
	}
	if strings.TrimSpace(src.MerchantKey) == newKey {
		return nil, fmt.Errorf("%w: split key must differ from the source key %q", ErrInvalidParameter, newKey)
	}

	// The target signature must be free — split creates a fresh series.
	_, mErr := qtx.MatchSeriesForUpdate(ctx, db.MatchSeriesForUpdateParams{
		MerchantKey:     newKey,
		IsoCurrencyCode: src.IsoCurrencyCode,
		UserID:          src.UserID,
	})
	if mErr != nil && !errors.Is(mErr, pgx.ErrNoRows) {
		return nil, fmt.Errorf("match target key: %w", mErr)
	}
	if mErr == nil {
		return nil, fmt.Errorf("%w: a series already exists at %q — assign the members to it instead of splitting", ErrInvalidParameter, newKey)
	}

	name := strings.TrimSpace(newName)
	if name == "" {
		name = slugs.TitleCase(newKey)
	}
	nw, err := qtx.InsertRecurringSeries(ctx, db.InsertRecurringSeriesParams{
		UserID:           src.UserID,
		Name:             name,
		MerchantKey:      newKey,
		Cadence:          src.Cadence,
		ExpectedDay:      src.ExpectedDay,
		ExpectedAmount:   pgtype.Numeric{}, // recomputed from the split-out members
		AmountTolerance:  src.AmountTolerance,
		IsoCurrencyCode:  src.IsoCurrencyCode,
		CategoryID:       src.CategoryID,
		Status:           SeriesStatusCandidate,
		DetectionSource:  seriesSourceForActor(actor),
		Confidence:       SeriesConfidenceAuto,
		ConfirmedByType:  pgtype.Text{},
		LastAmount:       pgtype.Numeric{},
		LastSeenDate:     pgtype.Date{},
		NextExpectedDate: pgtype.Date{},
		OccurrenceCount:  0,
		DetectionSignals: nil,
	})
	if err != nil {
		return nil, fmt.Errorf("insert split series: %w", err)
	}

	// Move the listed members from source → new (and repoint their merchant_key).
	// Guarded on series_id = source so it never steals a charge from a third series.
	ct, err := tx.Exec(ctx,
		`UPDATE transactions SET series_id = $2, merchant_key = $3, updated_at = NOW()
		 WHERE id = ANY($4::uuid[]) AND series_id = $1 AND deleted_at IS NULL`,
		src.ID, nw.ID, newKey, memberIDs)
	if err != nil {
		return nil, fmt.Errorf("move members: %w", err)
	}
	if moved := ct.RowsAffected(); int(moved) != len(memberIDs) {
		return nil, fmt.Errorf("%w: %d of %d transactions are not current members of the source series",
			ErrInvalidParameter, len(memberIDs)-int(moved), len(memberIDs))
	}

	// Strip the SOURCE series' inherited tags from the moved members — they no
	// longer belong to it, so its system-provenance tags are stale. Scoped by
	// provenance (added_by_type='system' + added_by_id=source short_id) so a tag
	// the user added directly to the transaction survives. The new series has no
	// tags yet; a later add_series_tag will re-materialize via ApplySeriesTagToAllMembers.
	if _, err := tx.Exec(ctx,
		`DELETE FROM transaction_tags
		 WHERE transaction_id = ANY($1::uuid[]) AND added_by_type = 'system' AND added_by_id = $2`,
		memberIDs, src.ShortID); err != nil {
		return nil, fmt.Errorf("strip source-inherited tags from moved members: %w", err)
	}

	newRow, err := s.applyRollup(ctx, qtx, nw.ID)
	if err != nil {
		return nil, err
	}
	if _, err := s.applyRollup(ctx, qtx, src.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit split: %w", err)
	}
	resp := seriesFromRow(newRow)
	return &resp, nil
}

// inferTypeFromMembers derives a recurring-charge type from the dominant
// category of a series' linked members. Returns "" when every member is
// uncategorized (caller keeps the existing/default type).
func (s *Service) inferTypeFromMembers(ctx context.Context, qtx *db.Queries, seriesID pgtype.UUID) string {
	slug, err := qtx.SeriesDominantMemberCategory(ctx, seriesID)
	if err != nil || slug == "" {
		return ""
	}
	return inferSeriesType(slug)
}

// SetSeriesType is the explicit type override (user/agent correction). Unlike
// detection's first-time inference, this always wins and is sticky thereafter.
func (s *Service) SetSeriesType(ctx context.Context, idOrShort, seriesType string, actor Actor) (*SeriesResponse, error) {
	seriesType = strings.TrimSpace(seriesType)
	if !validSeriesType[seriesType] {
		return nil, fmt.Errorf("%w: type must be one of subscription, bill, loan, other", ErrInvalidParameter)
	}
	id, err := s.resolveSeriesID(ctx, idOrShort)
	if err != nil {
		return nil, err
	}
	row, err := s.Queries.GetRecurringSeriesByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get series: %w", err)
	}
	upd := updateParamsFromRow(row)
	upd.Type = seriesType
	updated, err := s.Queries.UpdateRecurringSeries(ctx, upd)
	if err != nil {
		return nil, fmt.Errorf("set series type: %w", err)
	}
	resp := seriesFromRow(updated)
	return &resp, nil
}

// ReinferSeriesTypes re-derives the type of every series still at the default
// 'subscription' from its members' dominant category — a one-time backfill for
// series detected before the type field existed (type is sticky-after-insert,
// so they never re-type on their own). Series already typed bill/loan/other are
// left untouched, and a series whose members still infer 'subscription' is a
// no-op. Returns the number actually re-typed.
func (s *Service) ReinferSeriesTypes(ctx context.Context) (int, error) {
	rows, err := s.Queries.ListRecurringSeries(ctx)
	if err != nil {
		return 0, fmt.Errorf("list series: %w", err)
	}
	retyped := 0
	for _, row := range rows {
		if row.Type != SeriesTypeSubscription {
			continue // already typed (non-default) — leave deliberate values alone
		}
		inferred := s.inferTypeFromMembers(ctx, s.Queries, row.ID)
		if inferred == "" || inferred == row.Type {
			continue
		}
		upd := updateParamsFromRow(row)
		upd.Type = inferred
		if _, err := s.Queries.UpdateRecurringSeries(ctx, upd); err != nil {
			return retyped, fmt.Errorf("re-type series %s: %w", row.ShortID, err)
		}
		retyped++
	}
	return retyped, nil
}

// GetSeries returns a single series by short_id or uuid.
func (s *Service) GetSeries(ctx context.Context, idOrShort string) (*SeriesResponse, error) {
	id, err := s.resolveSeriesID(ctx, idOrShort)
	if err != nil {
		return nil, err
	}
	row, err := s.Queries.GetRecurringSeriesByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get series: %w", err)
	}
	resp := seriesFromRow(row)
	if slugs, err := s.Queries.ListSeriesTagSlugs(ctx, id); err == nil {
		resp.Tags = slugs
	}
	return &resp, nil
}

// AddSeriesTag attaches an existing tag (by slug) to a series and materializes
// it onto the series' current members (NULL-fill, provenance=system+series).
func (s *Service) AddSeriesTag(ctx context.Context, idOrShort, tagSlug string, actor Actor) error {
	id, err := s.resolveSeriesID(ctx, idOrShort)
	if err != nil {
		return err
	}
	tag, err := s.Queries.GetTagBySlug(ctx, tagSlug)
	if err != nil {
		return fmt.Errorf("%w: tag %q not found", ErrInvalidParameter, tagSlug)
	}
	if _, err := s.Queries.AddSeriesTag(ctx, db.AddSeriesTagParams{SeriesID: id, TagID: tag.ID}); err != nil {
		return fmt.Errorf("add series tag: %w", err)
	}
	if err := s.Queries.ApplySeriesTagToAllMembers(ctx, db.ApplySeriesTagToAllMembersParams{ID: id, TagID: tag.ID}); err != nil {
		return fmt.Errorf("apply series tag to members: %w", err)
	}
	return nil
}

// RemoveSeriesTag detaches a tag from a series and strips the series-inherited
// copies from its members (provenance-scoped, so user-added tags survive).
func (s *Service) RemoveSeriesTag(ctx context.Context, idOrShort, tagSlug string) error {
	id, err := s.resolveSeriesID(ctx, idOrShort)
	if err != nil {
		return err
	}
	row, err := s.Queries.GetRecurringSeriesByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("get series: %w", err)
	}
	tag, err := s.Queries.GetTagBySlug(ctx, tagSlug)
	if err != nil {
		return fmt.Errorf("%w: tag %q not found", ErrInvalidParameter, tagSlug)
	}
	if _, err := s.Queries.RemoveSeriesTag(ctx, db.RemoveSeriesTagParams{SeriesID: id, TagID: tag.ID}); err != nil {
		return fmt.Errorf("remove series tag: %w", err)
	}
	if err := s.Queries.RemoveSeriesTagFromMembers(ctx, db.RemoveSeriesTagFromMembersParams{
		TagID:     tag.ID,
		AddedByID: pgconv.Text(row.ShortID),
	}); err != nil {
		return fmt.Errorf("remove series tag from members: %w", err)
	}
	return nil
}

// ListSeriesTags returns the tag slugs attached to a series.
func (s *Service) ListSeriesTags(ctx context.Context, idOrShort string) ([]string, error) {
	id, err := s.resolveSeriesID(ctx, idOrShort)
	if err != nil {
		return nil, err
	}
	slugs, err := s.Queries.ListSeriesTagSlugs(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list series tags: %w", err)
	}
	return slugs, nil
}

// ListSeries returns series, optionally filtered by status. With no status it
// returns all live series, candidates first then by occurrence count.
func (s *Service) ListSeries(ctx context.Context, status *string) ([]SeriesResponse, error) {
	if status != nil && *status != "" {
		return s.ListSeriesByStatus(ctx, *status)
	}
	rows, err := s.Queries.ListRecurringSeries(ctx)
	if err != nil {
		return nil, fmt.Errorf("list series: %w", err)
	}
	out := make([]SeriesResponse, len(rows))
	for i, r := range rows {
		out[i] = seriesFromRow(r)
	}
	return out, nil
}

// ListSeriesByStatus returns series in a given lifecycle status, newest first.
func (s *Service) ListSeriesByStatus(ctx context.Context, status string) ([]SeriesResponse, error) {
	rows, err := s.Queries.ListRecurringSeriesByStatus(ctx, status)
	if err != nil {
		return nil, fmt.Errorf("list series: %w", err)
	}
	out := make([]SeriesResponse, len(rows))
	for i, r := range rows {
		out[i] = seriesFromRow(r)
	}
	return out, nil
}

// SeriesMember is one linked transaction (charge) of a recurring series.
type SeriesMember struct {
	ShortID      string   `json:"short_id"`
	Date         *string  `json:"date,omitempty"`
	Name         string   `json:"name"`
	MerchantName *string  `json:"merchant_name,omitempty"`
	Amount       *float64 `json:"amount,omitempty"`
	Currency     *string  `json:"iso_currency_code,omitempty"`
}

// SeriesMembers returns the transactions linked to a series, newest first.
func (s *Service) SeriesMembers(ctx context.Context, idOrShort string) ([]SeriesMember, error) {
	id, err := s.resolveSeriesID(ctx, idOrShort)
	if err != nil {
		return nil, err
	}
	rows, err := s.Queries.ListSeriesMembers(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list series members: %w", err)
	}
	out := make([]SeriesMember, len(rows))
	for i, r := range rows {
		out[i] = SeriesMember{
			ShortID:      r.ShortID,
			Date:         dateStr(r.Date),
			Name:         r.ProviderName,
			MerchantName: textPtr(r.ProviderMerchantName),
			Amount:       numericFloat(r.Amount),
			Currency:     textPtr(r.IsoCurrencyCode),
		}
	}
	return out, nil
}

// seriesRollup recomputes (occurrence_count, last_amount, last_seen_date) from
// the live members of a series. Zero members → zeros/NULLs (idempotent).
func (s *Service) seriesRollup(ctx context.Context, q *db.Queries, seriesID pgtype.UUID) (int32, pgtype.Numeric, pgtype.Date, error) {
	roll, err := q.SeriesMemberRollup(ctx, seriesID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, pgtype.Numeric{}, pgtype.Date{}, nil
		}
		return 0, pgtype.Numeric{}, pgtype.Date{}, fmt.Errorf("series rollup: %w", err)
	}
	return int32(roll.OccurrenceCount), roll.LastAmount, roll.LastSeenDate, nil
}

// applyRollup recomputes a series' occurrence/last-amount/last-seen from its
// live members and persists them (plus the projected next_expected_date),
// returning the refreshed row. Used after membership changes (re-key keeps
// membership but is harmless; split changes it on both sides).
func (s *Service) applyRollup(ctx context.Context, qtx *db.Queries, id pgtype.UUID) (db.RecurringSeries, error) {
	row, err := qtx.GetRecurringSeriesByID(ctx, id)
	if err != nil {
		return db.RecurringSeries{}, fmt.Errorf("get series for rollup: %w", err)
	}
	occ, lastAmt, lastSeen, err := s.seriesRollup(ctx, qtx, id)
	if err != nil {
		return db.RecurringSeries{}, err
	}
	upd := updateParamsFromRow(row)
	upd.OccurrenceCount = occ
	upd.LastAmount = lastAmt
	upd.LastSeenDate = lastSeen
	upd.NextExpectedDate = nextExpectedDate(upd.Cadence, lastSeen)
	return qtx.UpdateRecurringSeries(ctx, upd)
}

func (s *Service) resolveOptionalUserID(ctx context.Context, idOrShort *string) (pgtype.UUID, error) {
	if idOrShort == nil || strings.TrimSpace(*idOrShort) == "" {
		return pgtype.UUID{}, nil
	}
	return s.resolveUserID(ctx, *idOrShort)
}

func (s *Service) resolveOptionalCategoryID(ctx context.Context, idOrShort *string) (pgtype.UUID, error) {
	if idOrShort == nil || strings.TrimSpace(*idOrShort) == "" {
		return pgtype.UUID{}, nil
	}
	return s.resolveCategoryID(ctx, *idOrShort)
}

func (s *Service) resolveTransactionIDs(ctx context.Context, idsOrShorts []string) ([]pgtype.UUID, error) {
	out := make([]pgtype.UUID, 0, len(idsOrShorts))
	for _, raw := range idsOrShorts {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		id, err := s.resolveTransactionID(ctx, raw)
		if err != nil {
			return nil, fmt.Errorf("resolve member transaction %q: %w", raw, err)
		}
		out = append(out, id)
	}
	return out, nil
}

// updateParamsFromRow seeds an UpdateRecurringSeriesParams from an existing row
// so callers override only the fields they intend to change.
func updateParamsFromRow(r db.RecurringSeries) db.UpdateRecurringSeriesParams {
	return db.UpdateRecurringSeriesParams{
		ID:               r.ID,
		UserID:           r.UserID,
		Name:             r.Name,
		MerchantKey:      r.MerchantKey,
		Cadence:          r.Cadence,
		ExpectedDay:      r.ExpectedDay,
		ExpectedAmount:   r.ExpectedAmount,
		AmountTolerance:  r.AmountTolerance,
		IsoCurrencyCode:  r.IsoCurrencyCode,
		CategoryID:       r.CategoryID,
		Status:           r.Status,
		DetectionSource:  r.DetectionSource,
		Confidence:       r.Confidence,
		ConfirmedByType:  r.ConfirmedByType,
		LastAmount:       r.LastAmount,
		LastSeenDate:     r.LastSeenDate,
		NextExpectedDate: r.NextExpectedDate,
		OccurrenceCount:  r.OccurrenceCount,
		DetectionSignals: r.DetectionSignals,
		Type:             r.Type, // preserve type on every update unless a caller overrides it
	}
}

// seriesFromRow converts a db.RecurringSeries to a SeriesResponse.
func seriesFromRow(r db.RecurringSeries) SeriesResponse {
	resp := SeriesResponse{
		ID:               formatUUID(r.ID),
		ShortID:          r.ShortID,
		UserID:           uuidPtr(r.UserID),
		Name:             r.Name,
		MerchantKey:      r.MerchantKey,
		Cadence:          r.Cadence,
		ExpectedDay:      int4ToIntPtr(r.ExpectedDay),
		ExpectedAmount:   numericFloat(r.ExpectedAmount),
		AmountTolerance:  numericFloat(r.AmountTolerance),
		IsoCurrencyCode:  textPtr(r.IsoCurrencyCode),
		CategoryID:       uuidPtr(r.CategoryID),
		Status:           r.Status,
		Type:             r.Type,
		DetectionSource:  r.DetectionSource,
		Confidence:       r.Confidence,
		ConfirmedByType:  textPtr(r.ConfirmedByType),
		LastAmount:       numericFloat(r.LastAmount),
		LastSeenDate:     dateStr(r.LastSeenDate),
		NextExpectedDate: dateStr(r.NextExpectedDate),
		OccurrenceCount:  int(r.OccurrenceCount),
		CreatedAt:        pgconv.TimestampStr(r.CreatedAt),
		UpdatedAt:        pgconv.TimestampStr(r.UpdatedAt),
	}
	if len(r.DetectionSignals) > 0 {
		resp.DetectionSignals = json.RawMessage(r.DetectionSignals)
	}
	resp.RenewalHealth, resp.DaysUntilRenewal = seriesRenewalHealth(r.Status, r.Cadence, r.NextExpectedDate, time.Now())
	return resp
}

// seriesSourceRank orders detection sources for the precedence guard in
// UpsertSeriesCandidate (PATCH A): a write may sharpen an auto candidate's
// proposed fields only when its source ranks >= the row's current source.
// deterministic (1) < rule (2) < agent (3) < user (4). Unknown sources rank 0.
func seriesSourceRank(source string) int {
	switch source {
	case SeriesSourceDeterministic:
		return 1
	case SeriesSourceRule:
		return 2
	case SeriesSourceAgent:
		return 3
	case SeriesSourceUser:
		return 4
	default:
		return 0
	}
}

// confirmerType maps an actor to the confirmed_by_type vocabulary (user|agent);
// system/rule actors attribute as agent.
func confirmerType(actor Actor) string {
	if actor.Type == "user" {
		return "user"
	}
	return "agent"
}

// nextExpectedDate projects the next charge date from the cadence and the last
// seen date. Returns NULL for irregular/unknown cadences (no prediction).
func nextExpectedDate(cadence string, lastSeen pgtype.Date) pgtype.Date {
	if !lastSeen.Valid {
		return pgtype.Date{}
	}
	d := lastSeen.Time
	var next time.Time
	switch cadence {
	case SeriesCadenceWeekly:
		next = d.AddDate(0, 0, 7)
	case SeriesCadenceBiweekly:
		next = d.AddDate(0, 0, 14)
	case SeriesCadenceMonthly:
		next = d.AddDate(0, 1, 0)
	case SeriesCadenceQuarterly:
		next = d.AddDate(0, 3, 0)
	case SeriesCadenceSemiannual:
		next = d.AddDate(0, 6, 0)
	case SeriesCadenceAnnual:
		next = d.AddDate(1, 0, 0)
	default:
		return pgtype.Date{}
	}
	return pgconv.Date(next)
}

// seriesRenewalHealth derives the renewal-health bucket and signed days-until
// for an active series, from its projected next_expected_date relative to now.
//
// Buckets: due_soon (0..window ahead), active (further ahead), overdue (past
// due but within one cadence cycle — likely processing lag), stale (missed a
// full cycle — likely cancelled). Returns ("", nil) for non-active series and
// ("unknown", nil) when no projection exists (irregular cadence / no charges).
func seriesRenewalHealth(status, cadence string, nextExpected pgtype.Date, now time.Time) (string, *int) {
	if status != SeriesStatusActive {
		return "", nil
	}
	interval := cadenceIntervalDays(cadence)
	if !nextExpected.Valid || interval == 0 {
		return SeriesHealthUnknown, nil
	}
	// Day-granular difference; truncate both ends to midnight UTC so partial
	// days don't flip the sign.
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	exp := time.Date(nextExpected.Time.Year(), nextExpected.Time.Month(), nextExpected.Time.Day(), 0, 0, 0, 0, time.UTC)
	days := int(exp.Sub(today).Hours() / 24)
	d := days
	switch {
	case days < -interval:
		return SeriesHealthStale, &d
	case days < 0:
		return SeriesHealthOverdue, &d
	case days <= renewalDueSoonWindowDays:
		return SeriesHealthDueSoon, &d
	default:
		return SeriesHealthActive, &d
	}
}

func numericDollars(f float64) pgtype.Numeric {
	return pgconv.NumericCents(int64(math.Round(f * 100)))
}

func numericDollarsPtr(f *float64) pgtype.Numeric {
	if f == nil {
		return pgtype.Numeric{}
	}
	return numericDollars(*f)
}

func int4Ptr(i *int32) pgtype.Int4 {
	if i == nil {
		return pgtype.Int4{}
	}
	return pgconv.Int4(*i)
}

func int4ToIntPtr(i pgtype.Int4) *int {
	if !i.Valid {
		return nil
	}
	v := int(i.Int32)
	return &v
}
