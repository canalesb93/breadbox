//go:build !lite

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Recurring-charge type — the structured classification axis (mirrors the CHECK
// in the recurring_series migration). "subscription" is one type, not the
// umbrella. A series is otherwise a thin, rule-maintained entity: surrogate
// id/short_id, an agent/user-authored name, and this type. Membership comes from
// assign_series rules (plus first-class agent one-off assigns) — there is no
// shipped detector, no cadence/amount/next-date stats, no confidence/lifecycle.
const (
	SeriesTypeSubscription = "subscription" // streaming, SaaS, memberships
	SeriesTypeBill         = "bill"         // rent, utilities, insurance, telecom
	SeriesTypeLoan         = "loan"         // mortgage, auto/student/personal loans
	SeriesTypeOther        = "other"        // recurring but uncategorized
)

var validSeriesType = map[string]bool{
	SeriesTypeSubscription: true, SeriesTypeBill: true,
	SeriesTypeLoan: true, SeriesTypeOther: true,
}

// SeriesResponse is the thin API/MCP shape of a recurring_series row.
type SeriesResponse struct {
	ID        string   `json:"id"`
	ShortID   string   `json:"short_id"`
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	Tags      []string `json:"tags,omitempty"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

// AssignSeriesInput is the input to AssignSeries — the imperative create/link
// path an agent or user drives (the MCP/REST `assign_series` tool). Either
// assign to an existing series (SeriesID), or mint one by name (Name +
// CreateIfMissing); then back-link members in the same call. Surrogate-first:
// the mint is idempotent on the unique live name.
type AssignSeriesInput struct {
	SeriesID        *string  // short_id or uuid — assign to an existing series
	Name            string   // display label + mint key (required when CreateIfMissing)
	CreateIfMissing bool      // mint a series (by Name) if no SeriesID
	Type            string   // optional subscription|bill|loan|other for a minted series
	TransactionIDs  []string // members to back-link (short_id or uuid), ≤50
	// FailIfExists turns a mint into a strict create: return ErrConflict when a
	// live series already exists with this name (instead of resolving to it). The
	// rule/agent paths leave this false (mint-by-name is idempotent); the
	// deliberate "create from scratch" UI sets it true.
	FailIfExists bool
}

// EditSeriesInput is the partial-update payload for UpdateSeries / PatchSeries.
// Every field is a pointer: nil leaves the column unchanged. A thin series owns
// exactly two editable attributes — its name and its type.
type EditSeriesInput struct {
	Name *string // non-empty display label; "" is rejected
	Type *string // subscription|bill|loan|other
}

// seriesAssignMaxMembers caps the per-call back-link batch (mirrors the
// update_transactions 50-op ceiling).
const seriesAssignMaxMembers = 50

// AssignSeries is the imperative create/link entry point shared by the
// `assign_series` MCP tool and REST endpoints. It assigns to an existing series
// (SeriesID) or mints one surrogate-first by name (CreateIfMissing), then
// back-links members.
func (s *Service) AssignSeries(ctx context.Context, in AssignSeriesInput, actor Actor) (*SeriesResponse, error) {
	if len(in.TransactionIDs) > seriesAssignMaxMembers {
		return nil, fmt.Errorf("%w: at most %d transactions per call", ErrInvalidParameter, seriesAssignMaxMembers)
	}

	switch {
	case in.SeriesID != nil && strings.TrimSpace(*in.SeriesID) != "":
		return s.linkSeriesMembers(ctx, *in.SeriesID, in.TransactionIDs, actor)
	case in.CreateIfMissing && strings.TrimSpace(in.Name) != "":
		return s.mintSeriesByName(ctx, in, actor)
	default:
		return nil, fmt.Errorf("%w: provide series_id, or name with create_if_missing", ErrInvalidParameter)
	}
}

// mintSeriesByName resolves (or creates) a series by its unique live name and
// back-links any members in one transaction. Idempotent — the same name always
// resolves the same surrogate. FailIfExists makes it a strict create.
func (s *Service) mintSeriesByName(ctx context.Context, in AssignSeriesInput, actor Actor) (*SeriesResponse, error) {
	name := strings.TrimSpace(in.Name)
	seriesType := strings.TrimSpace(in.Type)
	if seriesType == "" {
		seriesType = SeriesTypeSubscription
	}
	if !validSeriesType[seriesType] {
		return nil, fmt.Errorf("%w: invalid type %q", ErrInvalidParameter, seriesType)
	}
	memberIDs, err := s.resolveTransactionIDs(ctx, in.TransactionIDs)
	if err != nil {
		return nil, err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin assign series: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := s.Queries.WithTx(tx)

	if in.FailIfExists {
		if existing, gerr := qtx.GetRecurringSeriesByName(ctx, name); gerr == nil {
			_ = existing
			return nil, fmt.Errorf("%w: a recurring series named %q already exists — edit it from the Recurring list instead of creating a new one", ErrConflict, name)
		} else if !errors.Is(gerr, pgx.ErrNoRows) {
			return nil, fmt.Errorf("check existing series: %w", gerr)
		}
	}

	seriesID, err := qtx.UpsertRecurringSeriesByName(ctx, db.UpsertRecurringSeriesByNameParams{
		Name: name,
		Type: seriesType,
	})
	if err != nil {
		return nil, fmt.Errorf("mint series: %w", err)
	}

	if len(memberIDs) > 0 {
		if err := backLinkAndTag(ctx, tx, qtx, seriesID, memberIDs, actor); err != nil {
			return nil, err
		}
	}

	row, err := qtx.GetRecurringSeriesByID(ctx, seriesID)
	if err != nil {
		return nil, fmt.Errorf("reload series: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit assign series: %w", err)
	}
	resp := seriesFromRow(row)
	return &resp, nil
}

// AssignSeriesFromRuleTx materializes an `assign_series` rule action INSIDE the
// sync transaction (the engine calls this via the Engine.AssignSeriesInTx hook,
// so the sync package never imports service). It resolves the target series by
// short_id, or — surrogate-first — mints one by name (createIfMissing), then
// back-links the single transaction (NULL-fill only). All on the provided tx so
// it commits atomically with the sync. Failure to resolve is a no-op (the rule
// is skipped) rather than a sync failure.
func (s *Service) AssignSeriesFromRuleTx(ctx context.Context, tx pgx.Tx, txnID pgtype.UUID, seriesShortID, seriesName string, createIfMissing bool) error {
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
	case createIfMissing && strings.TrimSpace(seriesName) != "":
		id, err := qtx.UpsertRecurringSeriesByName(ctx, db.UpsertRecurringSeriesByNameParams{
			Name: strings.TrimSpace(seriesName),
			Type: SeriesTypeSubscription,
		})
		if err != nil {
			return fmt.Errorf("mint rule series: %w", err)
		}
		seriesID = id
	default:
		return nil // nothing actionable
	}

	if err := backLinkAndTag(ctx, tx, qtx, seriesID, []pgtype.UUID{txnID}, SystemActor()); err != nil {
		return err
	}
	return nil
}

// backLinkAndTag back-links members to a series (NULL-fill only), materializes
// the series' tags onto the freshly-linked members, and emits a series_assigned
// annotation for the charges this call actually linked. Shared by the mint,
// link, and rule paths. The caller owns the surrounding transaction.
func backLinkAndTag(ctx context.Context, tx pgx.Tx, qtx *db.Queries, seriesID pgtype.UUID, memberIDs []pgtype.UUID, actor Actor) error {
	if len(memberIDs) == 0 {
		return nil
	}
	// Only the charges actually linked by this call (currently series-less) get a
	// timeline event — a re-apply NULL-fills already-linked members and must not
	// re-emit.
	newlyLinked, err := unlinkedMemberSubset(ctx, tx, memberIDs)
	if err != nil {
		return err
	}
	if _, err := qtx.BackLinkSeriesMembers(ctx, db.BackLinkSeriesMembersParams{
		SeriesID:       seriesID,
		TransactionIds: memberIDs,
	}); err != nil {
		return fmt.Errorf("back-link series members: %w", err)
	}
	if err := qtx.ApplySeriesTagsToTransactions(ctx, db.ApplySeriesTagsToTransactionsParams{
		SeriesID: seriesID,
		Column2:  memberIDs,
	}); err != nil {
		return fmt.Errorf("apply series tags to members: %w", err)
	}
	if len(newlyLinked) > 0 {
		row, err := qtx.GetRecurringSeriesByID(ctx, seriesID)
		if err != nil {
			return fmt.Errorf("reload series for annotation: %w", err)
		}
		if err := emitSeriesMembershipAnnotations(ctx, qtx, annotationKindSeriesAssigned, newlyLinked, row.ShortID, row.Name, actor); err != nil {
			return fmt.Errorf("emit series_assigned annotations: %w", err)
		}
	}
	return nil
}

// linkSeriesMembers back-links transactions to an existing series (NULL-fill
// only) in one transaction. Used by the existing-series branch of AssignSeries.
func (s *Service) linkSeriesMembers(ctx context.Context, idOrShort string, memberIDsOrShorts []string, actor Actor) (*SeriesResponse, error) {
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
	if err := backLinkAndTag(ctx, tx, qtx, row.ID, memberIDs, actor); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit link members: %w", err)
	}
	resp := seriesFromRow(row)
	if slugs, err := s.Queries.ListSeriesTagSlugs(ctx, row.ID); err == nil {
		resp.Tags = slugs
	}
	return &resp, nil
}

// UpdateSeries applies a partial edit to a series' user-owned attributes (name,
// type). Renaming onto an existing live name is collision-guarded by the unique
// index.
func (s *Service) UpdateSeries(ctx context.Context, idOrShort string, in EditSeriesInput, actor Actor) (*SeriesResponse, error) {
	id, err := s.resolveSeriesID(ctx, idOrShort)
	if err != nil {
		return nil, err
	}
	if in.Name == nil && in.Type == nil {
		return nil, fmt.Errorf("%w: provide name and/or type", ErrInvalidParameter)
	}
	if in.Name != nil && strings.TrimSpace(*in.Name) == "" {
		return nil, fmt.Errorf("%w: name cannot be empty", ErrInvalidParameter)
	}
	if in.Type != nil && !validSeriesType[strings.TrimSpace(*in.Type)] {
		return nil, fmt.Errorf("%w: type must be one of subscription, bill, loan, other", ErrInvalidParameter)
	}

	row, err := s.Queries.GetRecurringSeriesByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get series: %w", err)
	}

	name := row.Name
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
	}
	seriesType := row.Type
	if in.Type != nil {
		seriesType = strings.TrimSpace(*in.Type)
	}

	updated, err := s.Queries.UpdateRecurringSeries(ctx, db.UpdateRecurringSeriesParams{
		ID:   id,
		Name: name,
		Type: seriesType,
	})
	if err != nil {
		return nil, fmt.Errorf("update series: %w", err)
	}
	resp := seriesFromRow(updated)
	if slugs, err := s.Queries.ListSeriesTagSlugs(ctx, id); err == nil {
		resp.Tags = slugs
	}
	return &resp, nil
}

// PatchSeries applies a partial edit (name/type) — the thin entity has no
// lifecycle verdict, so this is edit-only. Kept as a distinct entry point so the
// REST PATCH handler and MCP update tool share one shape.
func (s *Service) PatchSeries(ctx context.Context, idOrShort string, edit *EditSeriesInput, actor Actor) (*SeriesResponse, error) {
	if edit == nil {
		return nil, fmt.Errorf("%w: provide an edit", ErrInvalidParameter)
	}
	return s.UpdateSeries(ctx, idOrShort, *edit, actor)
}

// UnlinkSeriesTransactions detaches transactions from a series — the inverse of
// the link path. It clears each charge's series_id, strips the series'
// system-provenance inherited tags from the detached charges (a tag the user
// added directly survives), and emits a series_unlinked timeline event. It
// refuses if any listed transaction isn't a current member.
func (s *Service) UnlinkSeriesTransactions(ctx context.Context, idOrShort string, memberIDsOrShorts []string, actor Actor) (*SeriesResponse, error) {
	if len(memberIDsOrShorts) == 0 {
		return nil, fmt.Errorf("%w: at least one transaction to unlink is required", ErrInvalidParameter)
	}
	if len(memberIDsOrShorts) > seriesAssignMaxMembers {
		return nil, fmt.Errorf("%w: at most %d transactions per call", ErrInvalidParameter, seriesAssignMaxMembers)
	}
	id, err := s.resolveSeriesID(ctx, idOrShort)
	if err != nil {
		return nil, err
	}
	memberIDs, err := s.resolveTransactionIDs(ctx, memberIDsOrShorts)
	if err != nil {
		return nil, err
	}
	memberIDs = dedupeUUIDs(memberIDs)

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin unlink: %w", err)
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

	detached, err := qtx.UnlinkSeriesMembers(ctx, db.UnlinkSeriesMembersParams{
		SeriesID:       row.ID,
		TransactionIds: memberIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("unlink members: %w", err)
	}
	if int(detached) != len(memberIDs) {
		return nil, fmt.Errorf("%w: %d of %d transactions are not current members of this series",
			ErrInvalidParameter, len(memberIDs)-int(detached), len(memberIDs))
	}

	// Strip the series' inherited (system-provenance) tags from the detached
	// charges — they no longer belong to it. Scoped by provenance so a tag the
	// user added to the transaction directly survives.
	if _, err := tx.Exec(ctx,
		`DELETE FROM transaction_tags
		 WHERE transaction_id = ANY($1::uuid[]) AND added_by_type = 'system' AND added_by_id = $2`,
		memberIDs, row.ShortID); err != nil {
		return nil, fmt.Errorf("strip series tags from unlinked members: %w", err)
	}

	if err := emitSeriesMembershipAnnotations(ctx, qtx, annotationKindSeriesUnlinked, memberIDs, row.ShortID, row.Name, actor); err != nil {
		return nil, fmt.Errorf("emit series_unlinked annotations: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit unlink: %w", err)
	}
	resp := seriesFromRow(row)
	if slugs, err := s.Queries.ListSeriesTagSlugs(ctx, row.ID); err == nil {
		resp.Tags = slugs
	}
	return &resp, nil
}

// GetSeries returns a single series by short_id or uuid, with its tags.
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

// ListSeries returns all live series, alphabetically by name. The status
// parameter is retained for signature compatibility but is ignored — a thin
// series has no lifecycle status.
func (s *Service) ListSeries(ctx context.Context, _ *string) ([]SeriesResponse, error) {
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

// ListSeriesMemberCounts returns the live member-charge count for every series
// that has at least one member, keyed by the series' UUID string (matching
// SeriesResponse.ID). Series with no members are absent from the map (count 0).
// One round-trip — the admin /recurring list uses it to label each series row.
func (s *Service) ListSeriesMemberCounts(ctx context.Context) (map[string]int, error) {
	rows, err := s.Queries.CountSeriesMembersGrouped(ctx)
	if err != nil {
		return nil, fmt.Errorf("count series members: %w", err)
	}
	out := make(map[string]int, len(rows))
	for _, r := range rows {
		out[formatUUID(r.SeriesID)] = int(r.MemberCount)
	}
	return out, nil
}

// ListSeriesGoverningRuleCounts tallies, in one pass over every `assign_series`
// rule, how many such rules target each series — by its short_id (assign-to-
// existing) or by its name (mint-by-name). Returns two maps (byShortID, byName)
// so the /recurring list handler can label each row's governing-rule count
// without an N+1 of ListGoverningRules. Mirrors
// ListCounterpartyGoverningRuleCounts.
func (s *Service) ListSeriesGoverningRuleCounts(ctx context.Context) (byShortID map[string]int, byName map[string]int, err error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT actions FROM transaction_rules
		   WHERE actions @> '[{"type":"assign_series"}]'::jsonb`)
	if err != nil {
		return nil, nil, fmt.Errorf("query series governing rule counts: %w", err)
	}
	defer rows.Close()

	byShortID = map[string]int{}
	byName = map[string]int{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, nil, fmt.Errorf("scan rule actions: %w", err)
		}
		var actions []RuleAction
		if err := json.Unmarshal(raw, &actions); err != nil {
			continue // a malformed action blob shouldn't break the whole tally
		}
		for _, a := range actions {
			if a.Type != "assign_series" {
				continue
			}
			if sid := strings.TrimSpace(a.SeriesShortID); sid != "" {
				byShortID[sid]++
			} else if name := strings.TrimSpace(a.SeriesName); name != "" {
				byName[name]++
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate rule actions: %w", err)
	}
	return byShortID, byName, nil
}

// ListGoverningRules returns the rules whose `assign_series` action targets this
// series — by its short_id (assign to existing) or by its name (mint by name).
// These rules ARE the series' durable definition (rules-as-substrate): its
// membership is whatever they match. Rules use hand-rolled SQL (not sqlc), so
// this reuses ruleSelectQuery + convertRuleRows.
func (s *Service) ListGoverningRules(ctx context.Context, idOrShort string) ([]TransactionRuleResponse, error) {
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

	query := ruleSelectQuery + `
		WHERE tr.actions @> jsonb_build_array(jsonb_build_object('type', 'assign_series', 'series_short_id', $1::text))
		   OR tr.actions @> jsonb_build_array(jsonb_build_object('type', 'assign_series', 'series_name', $2::text))
		ORDER BY tr.priority DESC, tr.created_at ASC`
	rows, err := s.Pool.Query(ctx, query, row.ShortID, row.Name)
	if err != nil {
		return nil, fmt.Errorf("list governing rules: %w", err)
	}
	defer rows.Close()

	var scanned []ruleRow
	for rows.Next() {
		var rr ruleRow
		if err := rows.Scan(rr.scanDest()...); err != nil {
			return nil, fmt.Errorf("scan governing rule: %w", err)
		}
		scanned = append(scanned, rr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate governing rules: %w", err)
	}
	return s.convertRuleRows(ctx, scanned), nil
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

// SeriesMember is one linked transaction (charge) of a recurring series. It
// carries the category color/icon + pending flag + tag count so the admin UI
// can render each charge through the shared transaction-row component.
type SeriesMember struct {
	ShortID       string   `json:"short_id"`
	Date          *string  `json:"date,omitempty"`
	Name          string   `json:"name"`
	MerchantName  *string  `json:"merchant_name,omitempty"`
	Amount        *float64 `json:"amount,omitempty"`
	Currency      *string  `json:"iso_currency_code,omitempty"`
	Pending       bool     `json:"pending"`
	CategoryColor *string  `json:"category_color,omitempty"`
	CategoryIcon  *string  `json:"category_icon,omitempty"`
	TagCount      int      `json:"tag_count"`
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
			ShortID:       r.ShortID,
			Date:          dateStr(r.Date),
			Name:          r.ProviderName,
			MerchantName:  textPtr(r.ProviderMerchantName),
			Amount:        numericFloat(r.Amount),
			Currency:      textPtr(r.IsoCurrencyCode),
			Pending:       r.Pending,
			CategoryColor: textPtr(r.CategoryColor),
			CategoryIcon:  textPtr(r.CategoryIcon),
			TagCount:      int(r.TagCount),
		}
	}
	return out, nil
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

// Annotation kinds for recurring-series membership events on the transaction
// activity timeline (see internal/db/migrations/*_annotations_series_kinds.sql).
const (
	annotationKindSeriesAssigned = "series_assigned"
	annotationKindSeriesUnlinked = "series_unlinked"
)

// unlinkedMemberSubset returns the subset of ids whose transactions are not yet
// in any series (series_id IS NULL). Used so series_assigned annotations are
// emitted only for charges the back-link actually links.
func unlinkedMemberSubset(ctx context.Context, tx pgx.Tx, ids []pgtype.UUID) ([]pgtype.UUID, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := tx.Query(ctx,
		`SELECT id FROM transactions WHERE id = ANY($1::uuid[]) AND series_id IS NULL AND deleted_at IS NULL`, ids)
	if err != nil {
		return nil, fmt.Errorf("query unlinked members: %w", err)
	}
	defer rows.Close()
	var out []pgtype.UUID
	for rows.Next() {
		var id pgtype.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan unlinked member: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// emitSeriesMembershipAnnotations writes one series_assigned / series_unlinked
// annotation per affected transaction, atomic with the surrounding tx (q is the
// tx-scoped *db.Queries). The payload carries the series short_id + name so the
// timeline can render and deep-link the sentence.
func emitSeriesMembershipAnnotations(ctx context.Context, q *db.Queries, kind string, txnIDs []pgtype.UUID, seriesShortID, seriesName string, actor Actor) error {
	if len(txnIDs) == 0 {
		return nil
	}
	if actor.Type == "" {
		actor = SystemActor()
	}
	payload := map[string]interface{}{
		"series_id":   seriesShortID,
		"series_name": seriesName,
	}
	for _, txnID := range txnIDs {
		if err := writeAnnotation(ctx, q, writeAnnotationParams{
			TransactionID: txnID,
			Kind:          kind,
			ActorType:     normalizeAnnotationActorType(actor.Type),
			ActorID:       actor.ID,
			ActorName:     actor.Name,
			Payload:       payload,
		}); err != nil {
			return err
		}
	}
	return nil
}

// dedupeUUIDs returns ids with duplicate values removed, preserving order.
func dedupeUUIDs(ids []pgtype.UUID) []pgtype.UUID {
	seen := make(map[[16]byte]struct{}, len(ids))
	out := make([]pgtype.UUID, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id.Bytes]; ok {
			continue
		}
		seen[id.Bytes] = struct{}{}
		out = append(out, id)
	}
	return out
}

// seriesFromRow converts a thin db.RecurringSeries to a SeriesResponse.
func seriesFromRow(r db.RecurringSeries) SeriesResponse {
	return SeriesResponse{
		ID:        formatUUID(r.ID),
		ShortID:   r.ShortID,
		Name:      r.Name,
		Type:      r.Type,
		CreatedAt: pgconv.TimestampStr(r.CreatedAt),
		UpdatedAt: pgconv.TimestampStr(r.UpdatedAt),
	}
}
