// Package ruleapply provides shared helpers for writing rule-driven annotation
// rows. Both the sync engine (internal/sync) and the retroactive apply paths
// (internal/service) need to emit identical annotation shapes for
// `rule_applied` and `category_set` rows; keeping the payload construction in
// one place avoids the drift we saw before the helpers existed.
//
// This package depends only on the database driver and is positioned as a
// neutral leaf that both service and sync can import without creating a
// dependency cycle (sync must not import service).
package ruleapply

import (
	"context"
	"encoding/json"
	"fmt"

	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// AppliedBy values communicate the write origin in the annotation payload. The
// admin UI branches on these strings when rendering the activity timeline.
const (
	AppliedBySync        = "sync"
	AppliedByRetroactive = "retroactive"
)

// Rule carries just enough rule metadata to attribute an annotation. Callers
// pass short_id + name for the actor columns and the UUID for the rule_id FK.
type Rule struct {
	ID      pgtype.UUID
	ShortID string
	Name    string
}

// WriteRuleApplied writes a `rule_applied` annotation for a single rule firing
// on a single transaction. Canonical payload shape:
//
//	{ rule_id, rule_name, action_field, action_value, applied_by }
//
// All five keys are always emitted so downstream consumers (admin UI
// `internal/admin/transactions.go`, analytics, etc.) see a stable shape —
// action_field / action_value may be empty strings when the caller is
// recording a rule-level audit without a specific action specialization.
func WriteRuleApplied(ctx context.Context, tx pgx.Tx, txnID pgtype.UUID, rule Rule, actionField, actionValue, appliedBy string) error {
	payload := map[string]any{
		"rule_id":      rule.ShortID,
		"rule_name":    rule.Name,
		"action_field": actionField,
		"action_value": actionValue,
		"applied_by":   appliedBy,
	}
	return writeAnnotation(ctx, tx, annotationRow{
		TransactionID: txnID,
		Kind:          "rule_applied",
		ActorType:     "system",
		ActorID:       rule.ShortID,
		ActorName:     rule.Name,
		Payload:       payload,
		RuleID:        rule.ID,
	})
}

// WriteCategorySet writes a `category_set` annotation for a rule that applied
// a category to a transaction. Canonical payload shape:
//
//	{ category_slug, source: "rule", applied_by, rule_id, rule_name }
//
// Callers supply the slug (already validated upstream) and the appliedBy
// origin. The annotation is attributed to the winning rule.
func WriteCategorySet(ctx context.Context, tx pgx.Tx, txnID pgtype.UUID, rule Rule, slug, appliedBy string) error {
	payload := map[string]any{
		"category_slug": slug,
		"source":        "rule",
		"applied_by":    appliedBy,
		"rule_id":       rule.ShortID,
		"rule_name":     rule.Name,
	}
	return writeAnnotation(ctx, tx, annotationRow{
		TransactionID: txnID,
		Kind:          "category_set",
		ActorType:     "system",
		ActorID:       rule.ShortID,
		ActorName:     rule.Name,
		Payload:       payload,
		RuleID:        rule.ID,
	})
}

// annotationRow is the minimum column set required to insert an annotation
// attributed to a rule. Other callers (comments, manual category changes, tag
// writes) still use their own helpers in the service and sync packages.
type annotationRow struct {
	TransactionID pgtype.UUID
	Kind          string
	ActorType     string
	ActorID       string
	ActorName     string
	Payload       map[string]any
	TagID         pgtype.UUID
	RuleID        pgtype.UUID
}

func writeAnnotation(ctx context.Context, tx pgx.Tx, r annotationRow) error {
	var payloadJSON []byte
	if r.Payload == nil {
		payloadJSON = []byte(`{}`)
	} else {
		b, err := json.Marshal(r.Payload)
		if err != nil {
			return fmt.Errorf("marshal annotation payload: %w", err)
		}
		payloadJSON = b
	}

	actorID := pgtype.Text{}
	if r.ActorID != "" {
		actorID = pgconv.Text(r.ActorID)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO annotations (transaction_id, kind, actor_type, actor_id, actor_name, session_id, payload, tag_id, rule_id)
		VALUES ($1, $2, $3, $4, $5, NULL, $6::jsonb, $7, $8)`,
		r.TransactionID, r.Kind, r.ActorType, actorID, r.ActorName, payloadJSON,
		r.TagID, r.RuleID,
	); err != nil {
		return fmt.Errorf("insert %s annotation: %w", r.Kind, err)
	}
	return nil
}
