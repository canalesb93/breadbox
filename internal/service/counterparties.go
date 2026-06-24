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

// Annotation kinds for counterparty membership events on the transaction
// activity timeline (see *_annotations_counterparty_kinds.sql).
const (
	annotationKindCounterpartyAssigned = "counterparty_assigned"
	annotationKindCounterpartyUnlinked = "counterparty_unlinked"
)

// counterpartyAssignMaxMembers caps the per-call link batch (mirrors the
// update_transactions / assign_series 50-op ceiling).
const counterpartyAssignMaxMembers = 50

// AssignCounterparty is the imperative create/link entry point shared by the
// `assign_counterparty` MCP tool and REST endpoints. It binds the listed
// transactions to an existing counterparty (CounterpartyShortID) or — surrogate-
// first — resolves-or-creates one by name (CreateIfMissing), then links members
// (NULL-fill only). Default is assign-existing; a counterparty is NEVER
// auto-created unless CreateIfMissing is set.
func (s *Service) AssignCounterparty(ctx context.Context, in AssignCounterpartyInput, actor Actor) (*CounterpartyResponse, error) {
	if len(in.TransactionIDs) > counterpartyAssignMaxMembers {
		return nil, fmt.Errorf("%w: at most %d transactions per call", ErrInvalidParameter, counterpartyAssignMaxMembers)
	}

	switch {
	case in.CounterpartyShortID != nil && strings.TrimSpace(*in.CounterpartyShortID) != "":
		return s.linkCounterpartyMembers(ctx, *in.CounterpartyShortID, in.TransactionIDs, actor)
	case in.CreateIfMissing && strings.TrimSpace(in.Name) != "":
		return s.createCounterpartyByName(ctx, in, actor)
	default:
		return nil, fmt.Errorf("%w: provide counterparty_short_id, or name with create_if_missing", ErrInvalidParameter)
	}
}

// createCounterpartyByName resolves (or creates) a counterparty by its live name
// and links any members in one transaction. Unlike series there is no UNIQUE on
// name, so this is a resolve-or-create that de-dupes on the live name.
// FailIfExists makes it a strict create.
func (s *Service) createCounterpartyByName(ctx context.Context, in AssignCounterpartyInput, actor Actor) (*CounterpartyResponse, error) {
	name := strings.TrimSpace(in.Name)
	memberIDs, err := s.resolveTransactionIDs(ctx, in.TransactionIDs)
	if err != nil {
		return nil, err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin assign counterparty: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := s.Queries.WithTx(tx)

	row, existed, err := resolveOrCreateCounterpartyByName(ctx, qtx, name)
	if err != nil {
		return nil, err
	}
	if existed && in.FailIfExists {
		return nil, fmt.Errorf("%w: a counterparty named %q already exists — edit it instead of creating a new one", ErrConflict, name)
	}

	if len(memberIDs) > 0 {
		if err := linkCounterpartyAndAnnotate(ctx, tx, qtx, row, memberIDs, actor, ""); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit assign counterparty: %w", err)
	}
	resp := counterpartyFromRow(row)
	return &resp, nil
}

// AssignCounterpartyFromRuleTx materializes an `assign_counterparty` rule action
// INSIDE the sync transaction (the engine calls this via the
// Engine.AssignCounterpartyInTx hook, so the sync package never imports service).
// It resolves the target counterparty by short_id, or — surrogate-first —
// resolves-or-creates one by name (createIfMissing), then links the single
// transaction (NULL-fill only). All on the provided tx so it commits atomically
// with the sync. Failure to resolve is a no-op (the rule is skipped) rather than
// a sync failure.
func (s *Service) AssignCounterpartyFromRuleTx(ctx context.Context, tx pgx.Tx, txnID pgtype.UUID, counterpartyShortID, counterpartyName string, createIfMissing bool) error {
	qtx := s.Queries.WithTx(tx)

	var row db.Counterparty
	switch {
	case strings.TrimSpace(counterpartyShortID) != "":
		r, err := qtx.GetCounterpartyByShortID(ctx, counterpartyShortID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil // rule references a missing counterparty — no-op, don't break sync
			}
			return fmt.Errorf("resolve counterparty %q: %w", counterpartyShortID, err)
		}
		row = r
	case createIfMissing && strings.TrimSpace(counterpartyName) != "":
		r, _, err := resolveOrCreateCounterpartyByName(ctx, qtx, strings.TrimSpace(counterpartyName))
		if err != nil {
			return fmt.Errorf("resolve-or-create rule counterparty: %w", err)
		}
		row = r
	default:
		return nil // nothing actionable
	}

	return linkCounterpartyAndAnnotate(ctx, tx, qtx, row, []pgtype.UUID{txnID}, SystemActor(), annotationSourceRule)
}

// resolveOrCreateCounterpartyByName returns the oldest live counterparty with the
// given name, creating one when none exists. The bool reports whether an existing
// row was found (true) versus freshly created (false). There is no UNIQUE on
// name, so this is application-level de-dup — acceptable per the doctrine
// (duplicates roll up later via canonical_counterparty_id).
func resolveOrCreateCounterpartyByName(ctx context.Context, qtx *db.Queries, name string) (db.Counterparty, bool, error) {
	existing, err := qtx.GetCounterpartyByName(ctx, name)
	if err == nil {
		return existing, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return db.Counterparty{}, false, fmt.Errorf("lookup counterparty by name: %w", err)
	}
	created, err := qtx.InsertCounterparty(ctx, db.InsertCounterpartyParams{Name: name})
	if err != nil {
		return db.Counterparty{}, false, fmt.Errorf("create counterparty: %w", err)
	}
	return created, false, nil
}

// linkCounterpartyAndAnnotate links members to a counterparty (NULL-fill only)
// and emits a counterparty_assigned annotation for the charges this call actually
// linked. The caller owns the surrounding transaction. `source` flows to the
// annotation payload — the rule path passes annotationSourceRule so the row is
// deduped against its parent rule_applied row; imperative callers pass "".
func linkCounterpartyAndAnnotate(ctx context.Context, tx pgx.Tx, qtx *db.Queries, cp db.Counterparty, memberIDs []pgtype.UUID, actor Actor, source string) error {
	if len(memberIDs) == 0 {
		return nil
	}
	// Only charges currently counterparty-less get a timeline event — a re-apply
	// NULL-fills already-linked members and must not re-emit.
	newlyLinked, err := unlinkedCounterpartySubset(ctx, tx, memberIDs)
	if err != nil {
		return err
	}
	if _, err := qtx.LinkTransactionCounterparty(ctx, db.LinkTransactionCounterpartyParams{
		CounterpartyID: cp.ID,
		TransactionIds: memberIDs,
	}); err != nil {
		return fmt.Errorf("link counterparty members: %w", err)
	}
	if len(newlyLinked) > 0 {
		if err := emitCounterpartyMembershipAnnotations(ctx, qtx, annotationKindCounterpartyAssigned, newlyLinked, cp.ShortID, cp.Name, actor, source); err != nil {
			return fmt.Errorf("emit counterparty_assigned annotations: %w", err)
		}
	}
	return nil
}

// linkCounterpartyMembers binds transactions to an existing counterparty
// (NULL-fill only) in one transaction. Used by the existing-counterparty branch
// of AssignCounterparty.
func (s *Service) linkCounterpartyMembers(ctx context.Context, idOrShort string, memberIDsOrShorts []string, actor Actor) (*CounterpartyResponse, error) {
	id, err := s.resolveCounterpartyID(ctx, idOrShort)
	if err != nil {
		return nil, err
	}
	memberIDs, err := s.resolveTransactionIDs(ctx, memberIDsOrShorts)
	if err != nil {
		return nil, err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin link counterparty members: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := s.Queries.WithTx(tx)

	row, err := qtx.GetCounterpartyByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get counterparty: %w", err)
	}
	if err := linkCounterpartyAndAnnotate(ctx, tx, qtx, row, memberIDs, actor, ""); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit link counterparty members: %w", err)
	}
	resp := counterpartyFromRow(row)
	return &resp, nil
}

// UnlinkCounterpartyTransactions detaches transactions from a counterparty — the
// inverse of the link path. It clears each charge's counterparty_id and emits a
// counterparty_unlinked timeline event. It refuses if any listed transaction
// isn't currently bound to this counterparty.
func (s *Service) UnlinkCounterpartyTransactions(ctx context.Context, idOrShort string, memberIDsOrShorts []string, actor Actor) (*CounterpartyResponse, error) {
	if len(memberIDsOrShorts) == 0 {
		return nil, fmt.Errorf("%w: at least one transaction to unlink is required", ErrInvalidParameter)
	}
	if len(memberIDsOrShorts) > counterpartyAssignMaxMembers {
		return nil, fmt.Errorf("%w: at most %d transactions per call", ErrInvalidParameter, counterpartyAssignMaxMembers)
	}
	id, err := s.resolveCounterpartyID(ctx, idOrShort)
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
		return nil, fmt.Errorf("begin unlink counterparty: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := s.Queries.WithTx(tx)

	row, err := qtx.GetCounterpartyByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get counterparty: %w", err)
	}

	detached, err := qtx.UnlinkTransactionCounterparty(ctx, db.UnlinkTransactionCounterpartyParams{
		CounterpartyID: row.ID,
		TransactionIds: memberIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("unlink counterparty members: %w", err)
	}
	if int(detached) != len(memberIDs) {
		return nil, fmt.Errorf("%w: %d of %d transactions are not bound to this counterparty",
			ErrInvalidParameter, len(memberIDs)-int(detached), len(memberIDs))
	}

	if err := emitCounterpartyMembershipAnnotations(ctx, qtx, annotationKindCounterpartyUnlinked, memberIDs, row.ShortID, row.Name, actor, ""); err != nil {
		return nil, fmt.Errorf("emit counterparty_unlinked annotations: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit unlink counterparty: %w", err)
	}
	resp := counterpartyFromRow(row)
	return &resp, nil
}

// GetCounterparty returns a single counterparty by short_id or uuid.
func (s *Service) GetCounterparty(ctx context.Context, idOrShort string) (*CounterpartyResponse, error) {
	id, err := s.resolveCounterpartyID(ctx, idOrShort)
	if err != nil {
		return nil, err
	}
	row, err := s.Queries.GetCounterpartyByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get counterparty: %w", err)
	}
	resp := counterpartyFromRow(row)
	return &resp, nil
}

// ListCounterparties returns all live counterparties, alphabetically by name.
func (s *Service) ListCounterparties(ctx context.Context) ([]CounterpartyResponse, error) {
	rows, err := s.Queries.ListCounterparties(ctx)
	if err != nil {
		return nil, fmt.Errorf("list counterparties: %w", err)
	}
	out := make([]CounterpartyResponse, len(rows))
	for i, r := range rows {
		out[i] = counterpartyFromRow(r)
	}
	return out, nil
}

// UpdateCounterparty applies an enrichment-lane edit (name + logo/url/category/
// mcc). Only the supplied (non-nil) fields change.
func (s *Service) UpdateCounterparty(ctx context.Context, idOrShort string, in EditCounterpartyInput, actor Actor) (*CounterpartyResponse, error) {
	id, err := s.resolveCounterpartyID(ctx, idOrShort)
	if err != nil {
		return nil, err
	}
	if in.Name == nil && in.WebsiteURL == nil && in.LogoURL == nil && in.CategoryID == nil && in.MCC == nil {
		return nil, fmt.Errorf("%w: provide at least one field to update", ErrInvalidParameter)
	}
	if in.Name != nil && strings.TrimSpace(*in.Name) == "" {
		return nil, fmt.Errorf("%w: name cannot be empty", ErrInvalidParameter)
	}

	row, err := s.Queries.GetCounterpartyByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get counterparty: %w", err)
	}

	name := row.Name
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
	}

	var categoryID pgtype.UUID
	if in.CategoryID != nil {
		if strings.TrimSpace(*in.CategoryID) == "" {
			return nil, fmt.Errorf("%w: category_id cannot be empty", ErrInvalidParameter)
		}
		cid, err := s.resolveCategoryID(ctx, *in.CategoryID)
		if err != nil {
			return nil, fmt.Errorf("%w: category %q not found", ErrInvalidParameter, *in.CategoryID)
		}
		categoryID = cid
	}

	updated, err := s.Queries.UpdateCounterparty(ctx, db.UpdateCounterpartyParams{
		ID:         id,
		Name:       name,
		WebsiteUrl: pgconv.TextFromPtr(in.WebsiteURL),
		LogoUrl:    pgconv.TextFromPtr(in.LogoURL),
		CategoryID: categoryID,
		Mcc:        pgconv.TextFromPtr(in.MCC),
		Attrs:      nil,
	})
	if err != nil {
		return nil, fmt.Errorf("update counterparty: %w", err)
	}
	resp := counterpartyFromRow(updated)
	return &resp, nil
}

// DeleteCounterparty soft-deletes a counterparty. The ON DELETE SET NULL FK is
// only triggered by a hard delete; a soft delete leaves linked transactions
// pointing at the now-hidden row, which List/Get exclude — acceptable for now,
// the read-model fallback (PR B) renders the provider string when the join misses.
func (s *Service) DeleteCounterparty(ctx context.Context, idOrShort string) error {
	id, err := s.resolveCounterpartyID(ctx, idOrShort)
	if err != nil {
		return err
	}
	n, err := s.Queries.SoftDeleteCounterparty(ctx, id)
	if err != nil {
		return fmt.Errorf("soft delete counterparty: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// CounterpartyTransactionCount returns the live charge count bound to a
// counterparty.
func (s *Service) CounterpartyTransactionCount(ctx context.Context, idOrShort string) (int64, error) {
	id, err := s.resolveCounterpartyID(ctx, idOrShort)
	if err != nil {
		return 0, err
	}
	n, err := s.Queries.CounterpartyTransactionCount(ctx, id)
	if err != nil {
		return 0, fmt.Errorf("count counterparty transactions: %w", err)
	}
	return n, nil
}

// CounterpartyMembers returns the short_ids of the live charges linked to a
// counterparty, newest first — the admin detail page feeds these to
// GetAdminTransactionRowsByIDs so linked charges render through the shared
// transaction-row component (identical to the /transactions list).
func (s *Service) CounterpartyMembers(ctx context.Context, idOrShort string) ([]string, error) {
	id, err := s.resolveCounterpartyID(ctx, idOrShort)
	if err != nil {
		return nil, err
	}
	ids, err := s.Queries.ListCounterpartyMemberShortIDs(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list counterparty members: %w", err)
	}
	return ids, nil
}

// ListCounterpartyMemberCounts returns the live charge count per counterparty,
// keyed by the counterparty's uuid string (matching CounterpartyResponse.ID).
// One query for the whole list page.
func (s *Service) ListCounterpartyMemberCounts(ctx context.Context) (map[string]int, error) {
	rows, err := s.Queries.ListCounterpartyMemberCounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list counterparty member counts: %w", err)
	}
	out := make(map[string]int, len(rows))
	for _, r := range rows {
		out[formatUUID(r.ID)] = int(r.MemberCount)
	}
	return out, nil
}

// ListCounterpartyGoverningRuleCounts tallies, in one pass over every
// `assign_counterparty` rule, how many such rules target each counterparty — by
// its short_id (assign-to-existing) or by its name (resolve-or-create). Returns
// two maps (bySHortID, byName) so the list handler can combine them with the
// counterparties it already has without an N+1 of ListCounterpartyGoverningRules.
func (s *Service) ListCounterpartyGoverningRuleCounts(ctx context.Context) (byShortID map[string]int, byName map[string]int, err error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT actions FROM transaction_rules
		   WHERE actions @> '[{"type":"assign_counterparty"}]'::jsonb`)
	if err != nil {
		return nil, nil, fmt.Errorf("query counterparty governing rule counts: %w", err)
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
			if a.Type != "assign_counterparty" {
				continue
			}
			if sid := strings.TrimSpace(a.CounterpartyShortID); sid != "" {
				byShortID[sid]++
			} else if name := strings.TrimSpace(a.CounterpartyName); name != "" {
				byName[name]++
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate rule actions: %w", err)
	}
	return byShortID, byName, nil
}

// ListCounterpartyGoverningRules returns the rules whose `assign_counterparty`
// action targets this counterparty — by its short_id (assign to existing) or by
// its name (resolve-or-create by name). These rules ARE the counterparty's
// durable definition (rules-as-substrate): its membership is whatever they match.
// Mirrors ListGoverningRules for series (hand-rolled SQL + convertRuleRows).
func (s *Service) ListCounterpartyGoverningRules(ctx context.Context, idOrShort string) ([]TransactionRuleResponse, error) {
	id, err := s.resolveCounterpartyID(ctx, idOrShort)
	if err != nil {
		return nil, err
	}
	row, err := s.Queries.GetCounterpartyByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get counterparty: %w", err)
	}

	query := ruleSelectQuery + `
		WHERE tr.actions @> jsonb_build_array(jsonb_build_object('type', 'assign_counterparty', 'counterparty_short_id', $1::text))
		   OR tr.actions @> jsonb_build_array(jsonb_build_object('type', 'assign_counterparty', 'counterparty_name', $2::text))
		ORDER BY tr.priority DESC, tr.created_at ASC`
	rows, err := s.Pool.Query(ctx, query, row.ShortID, row.Name)
	if err != nil {
		return nil, fmt.Errorf("list counterparty governing rules: %w", err)
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

// unlinkedCounterpartySubset returns the subset of ids whose transactions are not
// yet bound to any counterparty (counterparty_id IS NULL). Used so
// counterparty_assigned annotations are emitted only for charges the link
// actually binds.
func unlinkedCounterpartySubset(ctx context.Context, tx pgx.Tx, ids []pgtype.UUID) ([]pgtype.UUID, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := tx.Query(ctx,
		`SELECT id FROM transactions WHERE id = ANY($1::uuid[]) AND counterparty_id IS NULL AND deleted_at IS NULL`, ids)
	if err != nil {
		return nil, fmt.Errorf("query unlinked counterparty members: %w", err)
	}
	defer rows.Close()
	var out []pgtype.UUID
	for rows.Next() {
		var id pgtype.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan unlinked counterparty member: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// emitCounterpartyMembershipAnnotations writes one counterparty_assigned /
// counterparty_unlinked annotation per affected transaction, atomic with the
// surrounding tx. The payload carries the counterparty short_id + name so the
// timeline can render and deep-link the sentence.
// When source is non-empty it is stamped into the payload (the rule paths pass
// annotationSourceRule so the row is recognised as a rule side-effect and
// deduped against the parent rule_applied row by EnrichAnnotations; user- and
// agent-authored calls pass "" so their events survive).
func emitCounterpartyMembershipAnnotations(ctx context.Context, q *db.Queries, kind string, txnIDs []pgtype.UUID, cpShortID, cpName string, actor Actor, source string) error {
	if len(txnIDs) == 0 {
		return nil
	}
	if actor.Type == "" {
		actor = SystemActor()
	}
	payload := map[string]interface{}{
		"counterparty_id":   cpShortID,
		"counterparty_name": cpName,
	}
	if source != "" {
		payload["source"] = source
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

// counterpartyFromRow converts a db.Counterparty to a CounterpartyResponse.
func counterpartyFromRow(r db.Counterparty) CounterpartyResponse {
	resp := CounterpartyResponse{
		ID:         formatUUID(r.ID),
		ShortID:    r.ShortID,
		Name:       r.Name,
		WebsiteURL: textPtr(r.WebsiteUrl),
		LogoURL:    textPtr(r.LogoUrl),
		MCC:        textPtr(r.Mcc),
		CreatedAt:  pgconv.TimestampStr(r.CreatedAt),
		UpdatedAt:  pgconv.TimestampStr(r.UpdatedAt),
	}
	if r.CategoryID.Valid {
		s := formatUUID(r.CategoryID)
		resp.CategoryID = &s
	}
	if r.CanonicalCounterpartyID.Valid {
		s := formatUUID(r.CanonicalCounterpartyID)
		resp.CanonicalCounterpartyID = &s
	}
	return resp
}
