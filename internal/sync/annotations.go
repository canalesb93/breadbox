package sync

import (
	"context"
	"encoding/json"
	"fmt"

	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// providerDisplayName returns the human-friendly name for a provider type.
// Used as ActorName on sync_started / sync_updated annotations so the activity
// timeline reads "Plaid imported this transaction" rather than the raw enum.
func providerDisplayName(provider string) string {
	switch provider {
	case "plaid":
		return "Plaid"
	case "teller":
		return "Teller"
	case "csv":
		return "CSV import"
	}
	return provider
}

// writeSyncStartedAnnotation records that a transaction was first imported
// during the initial sync of a connection. One row is written per newly
// inserted transaction, attributed to the system actor named after the
// provider. The connection and sync_log short ids in the payload let the UI
// link back to the originating sync run.
func writeSyncStartedAnnotation(ctx context.Context, tx pgx.Tx, txnID pgtype.UUID, providerType, connShortID, syncLogShortID string) error {
	payload := map[string]any{
		"provider":      providerType,
		"connection_id": connShortID,
		"sync_log_id":   syncLogShortID,
	}
	return insertSyncAnnotation(ctx, tx, syncAnnotationRow{
		TransactionID: txnID,
		Kind:          "sync_started",
		ActorType:     "system",
		ActorID:       connShortID,
		ActorName:     providerDisplayName(providerType),
		Payload:       payload,
	})
}

// writeSyncUpdatedAnnotation records that a subsequent sync touched a
// transaction whose `pending` flag flipped. Only emitted on a pending status
// change — silent for plain field touches and rule-driven category changes
// (those have their own annotation kinds).
func writeSyncUpdatedAnnotation(ctx context.Context, tx pgx.Tx, txnID pgtype.UUID, providerType, connShortID, syncLogShortID string, fromPending, toPending bool) error {
	payload := map[string]any{
		"provider":      providerType,
		"connection_id": connShortID,
		"sync_log_id":   syncLogShortID,
		"status_change": map[string]any{
			"from": pendingLabel(fromPending),
			"to":   pendingLabel(toPending),
		},
	}
	return insertSyncAnnotation(ctx, tx, syncAnnotationRow{
		TransactionID: txnID,
		Kind:          "sync_updated",
		ActorType:     "system",
		ActorID:       connShortID,
		ActorName:     providerDisplayName(providerType),
		Payload:       payload,
	})
}

// pendingLabel maps the boolean pending flag to the timeline-friendly label
// used in the sync_updated payload's status_change object.
func pendingLabel(pending bool) string {
	if pending {
		return "pending"
	}
	return "posted"
}

// syncAnnotationRow is the column set required to insert a sync-attributed
// annotation. Mirrors the shape used by ruleapply.writeAnnotation but lives
// inside the sync package so we don't pull a rule-flavoured helper into a
// non-rule code path.
type syncAnnotationRow struct {
	TransactionID pgtype.UUID
	Kind          string
	ActorType     string
	ActorID       string
	ActorName     string
	Payload       map[string]any
}

func insertSyncAnnotation(ctx context.Context, tx pgx.Tx, r syncAnnotationRow) error {
	var payloadJSON []byte
	if r.Payload == nil {
		payloadJSON = []byte(`{}`)
	} else {
		b, err := json.Marshal(r.Payload)
		if err != nil {
			return fmt.Errorf("marshal sync annotation payload: %w", err)
		}
		payloadJSON = b
	}

	actorID := pgtype.Text{}
	if r.ActorID != "" {
		actorID = pgconv.Text(r.ActorID)
	}

	// created_at = clock_timestamp() so sync_started writes preserve their
	// pre-rule ordering on the timeline; see the matching note in engine.go's
	// writeSyncAnnotation.
	if _, err := tx.Exec(ctx,
		`INSERT INTO annotations (transaction_id, kind, actor_type, actor_id, actor_name, session_id, payload, created_at)
		VALUES ($1, $2, $3, $4, $5, NULL, $6::jsonb, clock_timestamp())`,
		r.TransactionID, r.Kind, r.ActorType, actorID, r.ActorName, payloadJSON,
	); err != nil {
		return fmt.Errorf("insert %s annotation: %w", r.Kind, err)
	}
	return nil
}
