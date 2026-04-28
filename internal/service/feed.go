package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5/pgtype"
)

// FeedActivityRow is a global activity entry — an annotation joined with
// transaction context (merchant, amount, currency, account, institution) so
// the home Feed page can render rich cards for events that happened on any
// transaction without re-fetching transaction details per row.
type FeedActivityRow struct {
	Annotation Annotation

	TransactionShortID string
	TransactionName    string
	MerchantName       string
	Amount             float64
	IsoCurrencyCode    string
	TransactionDate    string

	AccountName     string
	InstitutionName string
}

// ListFeedActivity returns the most recent annotations across every
// transaction, joined with merchant/amount/account context so the global
// feed page can render rich rows without N+1 lookups. Annotations are
// returned newest-first (DESC) — the feed surface shows newest at the top
// (inverse of the per-transaction timeline, which puts newest at the
// bottom near the composer).
//
// EnrichAnnotations is applied after the SQL fetch with descending order
// reversed temporarily, since enrichment dedup logic assumes ASC ordering
// (rule-source duplicates appear adjacent to their parent rule_applied row).
func (s *Service) ListFeedActivity(ctx context.Context, limit int) ([]FeedActivityRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}

	const q = `
SELECT
    a.id, a.short_id, a.transaction_id, a.kind, a.actor_type, a.actor_id, a.actor_name,
    a.payload, a.tag_id, a.rule_id, a.created_at,
    t.short_id, t.provider_name, t.provider_merchant_name, t.amount, t.iso_currency_code, t.date,
    ac.name, bc.institution_name
FROM annotations a
JOIN transactions t ON a.transaction_id = t.id
LEFT JOIN accounts ac ON t.account_id = ac.id
LEFT JOIN bank_connections bc ON ac.connection_id = bc.id
WHERE t.deleted_at IS NULL
ORDER BY a.created_at DESC
LIMIT $1
`

	rows, err := s.Pool.Query(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("list feed activity: %w", err)
	}
	defer rows.Close()

	out := make([]FeedActivityRow, 0, limit)
	for rows.Next() {
		var (
			id, txnID, tagID, ruleID                    pgtype.UUID
			actorID, actorName, kind, actorType         string
			actorIDOpt                                  pgtype.Text
			shortID                                     string
			payload                                     []byte
			createdAt                                   pgtype.Timestamptz
			txShort, txName                             string
			merchantName                                pgtype.Text
			amount                                      pgtype.Numeric
			isoCcy                                      pgtype.Text
			txDate                                      pgtype.Date
			accountName, institutionName                pgtype.Text
		)

		if err := rows.Scan(
			&id, &shortID, &txnID, &kind, &actorType, &actorIDOpt, &actorName,
			&payload, &tagID, &ruleID, &createdAt,
			&txShort, &txName, &merchantName, &amount, &isoCcy, &txDate,
			&accountName, &institutionName,
		); err != nil {
			return nil, fmt.Errorf("scan feed activity row: %w", err)
		}

		ann := Annotation{
			ID:            formatUUID(id),
			ShortID:       shortID,
			TransactionID: formatUUID(txnID),
			Kind:          kind,
			ActorType:     actorType,
			ActorName:     actorName,
			CreatedAt:     pgconv.TimestampStr(createdAt),
			CreatedAtTime: createdAt.Time.UTC(),
		}
		if actorIDOpt.Valid && actorIDOpt.String != "" {
			s := actorIDOpt.String
			ann.ActorID = &s
		}
		_ = actorID
		if tagID.Valid {
			s := formatUUID(tagID)
			ann.TagID = &s
		}
		if ruleID.Valid {
			s := formatUUID(ruleID)
			ann.RuleID = &s
		}
		if len(payload) > 0 && string(payload) != "{}" {
			var p map[string]interface{}
			if err := json.Unmarshal(payload, &p); err == nil {
				ann.Payload = p
			}
		}

		row := FeedActivityRow{
			Annotation:         ann,
			TransactionShortID: txShort,
			TransactionName:    txName,
			MerchantName:       pgconv.TextOr(merchantName, ""),
			IsoCurrencyCode:    pgconv.TextOr(isoCcy, "USD"),
			AccountName:        pgconv.TextOr(accountName, ""),
			InstitutionName:    pgconv.TextOr(institutionName, ""),
		}
		if amount.Valid {
			if f, err := amount.Float64Value(); err == nil && f.Valid {
				row.Amount = f.Float64
			}
		}
		if txDate.Valid {
			row.TransactionDate = txDate.Time.Format("2006-01-02")
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate feed activity rows: %w", err)
	}

	// Enrichment expects ASC ordering for dedup heuristics; reverse, enrich,
	// reverse back.
	asc := make([]Annotation, len(out))
	for i, r := range out {
		asc[len(out)-1-i] = r.Annotation
	}
	enriched := EnrichAnnotations(asc, EnrichOptions{})

	// Map enriched annotations back by ID and rebuild DESC-ordered slice.
	byID := make(map[string]Annotation, len(enriched))
	for _, a := range enriched {
		byID[a.ID] = a
	}
	desc := make([]FeedActivityRow, 0, len(enriched))
	for _, r := range out {
		if a, ok := byID[r.Annotation.ID]; ok {
			r.Annotation = a
			desc = append(desc, r)
		}
	}

	// Best-effort actor avatar version for user actors. The dashboard
	// already does the same trick via a join on users.updated_at; for the
	// feed we issue a single follow-up batch lookup keyed by actor_id so
	// we don't have to re-do the whole join.
	if err := s.hydrateFeedAvatarVersions(ctx, desc); err != nil {
		s.Logger.Debug("hydrate feed avatar versions", "error", err)
	}

	_ = strconv.Itoa // keep import used across small refactors
	_ = time.Now
	return desc, nil
}

// hydrateFeedAvatarVersions populates Annotation.ActorAvatarVersion on every
// user-attributed row in `rows` using a single batch query against users.
// Best-effort: failures are returned to the caller for logging, but the feed
// still renders without avatar cache-busting.
func (s *Service) hydrateFeedAvatarVersions(ctx context.Context, rows []FeedActivityRow) error {
	ids := make([]string, 0, len(rows))
	seen := make(map[string]bool)
	for _, r := range rows {
		if r.Annotation.ActorType != "user" || r.Annotation.ActorID == nil {
			continue
		}
		id := *r.Annotation.ActorID
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}

	const q = `SELECT id::text, EXTRACT(EPOCH FROM updated_at)::bigint FROM users WHERE id::text = ANY($1::text[])`
	pgrows, err := s.Pool.Query(ctx, q, ids)
	if err != nil {
		return err
	}
	defer pgrows.Close()
	versions := make(map[string]string, len(ids))
	for pgrows.Next() {
		var id string
		var ts int64
		if err := pgrows.Scan(&id, &ts); err != nil {
			return err
		}
		versions[id] = strconv.FormatInt(ts, 10)
	}
	for i := range rows {
		if rows[i].Annotation.ActorType != "user" || rows[i].Annotation.ActorID == nil {
			continue
		}
		if v, ok := versions[*rows[i].Annotation.ActorID]; ok {
			rows[i].Annotation.ActorAvatarVersion = v
		}
	}
	return nil
}
