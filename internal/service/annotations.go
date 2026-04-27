package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"breadbox/internal/db"
	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5/pgtype"
)

// Annotation is the canonical timeline event for a transaction — the single
// source of truth for comments, rule applications, tag changes, and category
// sets.
//
// The derived fields (Action through RuleName, plus Summary) are populated by
// ListAnnotations after enrichment and are empty on rows fetched via the raw
// row converters. They give MCP agents and the admin UI a uniform, ready-to-
// render view without forcing every consumer to fish keys out of Payload.
type Annotation struct {
	ID            string                 `json:"id"`
	ShortID       string                 `json:"short_id"`
	TransactionID string                 `json:"transaction_id"`
	Kind          string                 `json:"kind"` // comment | rule_applied | tag_added | tag_removed | category_set
	ActorType     string                 `json:"actor_type"`
	ActorID       *string                `json:"actor_id,omitempty"`
	ActorName     string                 `json:"actor_name"`
	SessionID     *string                `json:"session_id,omitempty"`
	Payload       map[string]interface{} `json:"payload,omitempty"`
	TagID         *string                `json:"tag_id,omitempty"`
	RuleID        *string                `json:"rule_id,omitempty"`
	CreatedAt     string                 `json:"created_at"`

	// IsDeleted flags a soft-deleted (tombstoned) annotation. Today only
	// comments can be soft-deleted via DeleteComment; the row stays on the
	// timeline with actor + timestamp intact while the UI renders a muted
	// "<Actor> deleted a comment" line in place of the comment bubble.
	IsDeleted bool `json:"is_deleted,omitempty"`

	// ---- Derived fields (populated by ListAnnotations enrichment) ----

	// Action is the normalized verb for the event:
	//   added | removed | set | applied | commented
	// It strips the verb out of Kind ("tag_added" → "added") so consumers
	// can branch on a single-word action without parsing the kind string.
	Action string `json:"action,omitempty"`

	// Summary is a one-line human-readable rendering of the event:
	//   "Alice added the Food tag"
	//   "Rule \"Auto-tag new transactions\" added tag needs-review during sync"
	//   "Bob set category to Groceries"
	// Designed so MCP agents can read activity without composing sentences
	// themselves and the admin UI can render rows with a single Summary draw.
	Summary string `json:"summary,omitempty"`

	// Subject is the canonical object of the event — the tag display-name,
	// the category display-name, the rule name, or the comment body. Empty
	// when the kind has no object (rare).
	Subject string `json:"subject,omitempty"`

	// Origin describes how a rule-applied row reached the transaction:
	// "during sync" or "retroactively". Empty for non-rule events.
	Origin string `json:"origin,omitempty"`

	// Source surfaces the payload's source qualifier ("rule" when the row
	// was written as part of a rule application). Most rule-source rows are
	// elided by enrichment in favor of the parent rule_applied annotation,
	// but Source remains populated where they survive.
	Source string `json:"source,omitempty"`

	// Content is the comment body for kind=comment, surfaced from payload
	// so comment readers don't need to peek at the untyped map.
	Content string `json:"content,omitempty"`

	// Note is the optional rationale recorded alongside a tag add/remove,
	// surfaced from payload.
	Note string `json:"note,omitempty"`

	// Top-level resource references surfaced from payload so consumers can
	// cross-link without parsing the untyped map.
	TagSlug      string `json:"tag_slug,omitempty"`
	CategorySlug string `json:"category_slug,omitempty"`
	RuleName     string `json:"rule_name,omitempty"`
	// RuleShortID is the rule's 8-char short_id, surfaced from
	// payload.rule_id so admin UI consumers can link to /rules/<short_id>
	// without resolving the FK separately. The DB row's rule_id column is
	// the UUID and stays exposed via Annotation.RuleID; this field is the
	// human-friendly handle the timeline link uses.
	RuleShortID string `json:"rule_short_id,omitempty"`

	// ActorAvatarVersion is the unix timestamp of the user's most recent
	// users.updated_at, used as a cache-busting `?v=<ts>` query string on
	// avatar URLs in the activity timeline. Empty for non-user actors and
	// for user actors whose row has been deleted. Mirrors the pattern in
	// users.html — see admin/templates.go avatarURL helper.
	ActorAvatarVersion string `json:"-"`
}

// writeAnnotationParams is the shared input for writing an annotation row via
// either the pool-backed Queries or a transaction-scoped db.Queries (WithTx).
type writeAnnotationParams struct {
	TransactionID pgtype.UUID
	Kind          string
	ActorType     string
	ActorID       string
	ActorName     string
	SessionID     pgtype.UUID
	Payload       map[string]interface{}
	TagID         pgtype.UUID
	RuleID        pgtype.UUID
}

// writeAnnotation inserts an annotation row via the supplied db.Queries handle.
// Passing a tx-scoped handle (from WithTx) keeps the write atomic with any
// surrounding DB transaction. Failures are returned to the caller.
func writeAnnotation(ctx context.Context, q *db.Queries, params writeAnnotationParams) error {
	var payload []byte
	if params.Payload != nil {
		b, err := json.Marshal(params.Payload)
		if err != nil {
			return fmt.Errorf("marshal annotation payload: %w", err)
		}
		payload = b
	} else {
		payload = []byte(`{}`)
	}

	actorID := pgtype.Text{}
	if params.ActorID != "" {
		actorID = pgconv.Text(params.ActorID)
	}

	_, err := q.InsertAnnotation(ctx, db.InsertAnnotationParams{
		TransactionID: params.TransactionID,
		Kind:          params.Kind,
		ActorType:     params.ActorType,
		ActorID:       actorID,
		ActorName:     params.ActorName,
		SessionID:     params.SessionID,
		Payload:       payload,
		TagID:         params.TagID,
		RuleID:        params.RuleID,
	})
	if err != nil {
		return fmt.Errorf("insert annotation: %w", err)
	}
	return nil
}

// ListAnnotationsParams carries optional filters for ListAnnotations. Empty
// fields preserve current behavior (return everything).
type ListAnnotationsParams struct {
	// Kinds limits results to specific annotation kinds (any of:
	// comment | rule_applied | tag_added | tag_removed | category_set).
	// Empty = no filter.
	Kinds []string

	// Raw, when true, skips enrichment and dedup. The returned rows have
	// their structural fields populated but no Summary/Action/etc., and
	// rule-source duplicates and same-actor adjacent comment-vs-tag-note
	// duplicates are kept. Useful for audit/debugging callers that need
	// the unmodified DB view. Defaults to false (enriched + deduped).
	Raw bool
}

// ListAnnotations returns annotations for a transaction, ordered by created_at
// ASC. Drives the transaction detail activity timeline and the MCP
// list_annotations tool. By default the rows are enriched (Summary, Action,
// Subject, top-level resource refs) and deduplicated (rule-source tag/category
// rows are folded into the parent rule_applied row, and same-actor adjacent
// comment-vs-tag-note pairs are collapsed). Pass Raw=true to bypass enrichment.
//
// Pass an empty ListAnnotationsParams for the full timeline; use Kinds to
// scope the result (e.g. comments only).
func (s *Service) ListAnnotations(ctx context.Context, transactionID string, params ListAnnotationsParams) ([]Annotation, error) {
	txnID, err := s.resolveTransactionID(ctx, transactionID)
	if err != nil {
		return nil, ErrNotFound
	}

	// Joined variant carries the actor user's updated_at so the timeline
	// can cache-bust avatar URLs (`?v=<unix>`), matching the pattern used
	// by users.html. Without it the browser cached the avatar bytes for up
	// to 24h after a user uploaded a new picture.
	rows, err := s.Queries.ListAnnotationsWithActorByTransaction(ctx, txnID)
	if err != nil {
		return nil, fmt.Errorf("list annotations: %w", err)
	}

	all := make([]Annotation, 0, len(rows))
	for _, r := range rows {
		all = append(all, annotationFromActorRow(r))
	}

	if params.Raw {
		return filterAnnotationKinds(all, params.Kinds), nil
	}

	enriched, err := s.enrichAnnotations(ctx, all)
	if err != nil {
		return nil, fmt.Errorf("enrich annotations: %w", err)
	}
	return filterAnnotationKinds(enriched, params.Kinds), nil
}

// filterAnnotationKinds returns the subset of `in` whose Kind matches one of
// the listed kinds. An empty kinds slice is treated as a no-op (returns the
// input unchanged). Filtering happens after enrichment so dedup decisions
// see the full set, not a kind-filtered slice that would miss the parent
// rule_applied row needed to elide its rule-source duplicates.
func filterAnnotationKinds(in []Annotation, kinds []string) []Annotation {
	keep := annotationKindFilter(kinds)
	if keep == nil {
		return in
	}
	out := make([]Annotation, 0, len(in))
	for _, a := range in {
		if keep[a.Kind] {
			out = append(out, a)
		}
	}
	return out
}

// annotationKindFilter builds an O(1) lookup set from a kinds slice. Returns
// nil when no filter is supplied so callers can short-circuit.
func annotationKindFilter(kinds []string) map[string]bool {
	if len(kinds) == 0 {
		return nil
	}
	set := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		set[k] = true
	}
	return set
}

// annotationFromActorRow converts a joined annotation+actor row into its
// service-layer response, surfacing the actor's updated_at as a unix-timestamp
// avatar version string.
//
// For user-attributed rows we prefer the live users.name carried by the join
// (a.ActorUserName) over the annotations.actor_name that was frozen in at
// write time. Actor.Name from a logged-in admin session is the
// auth_accounts.username (typically an email), so without this preference
// the timeline rendered "admin@example.com added the Food tag" even when
// the linked household member had a real profile name. Falls back to the
// stored actor_name when the join missed (rare, e.g. the user was
// hard-deleted) or the profile name is blank. Non-user actors (system,
// agent) always fall through to the stored actor_name.
func annotationFromActorRow(a db.ListAnnotationsWithActorByTransactionRow) Annotation {
	displayName := a.ActorName
	if a.ActorType == "user" && a.ActorUserName != "" {
		displayName = a.ActorUserName
	}

	ann := Annotation{
		ID:            formatUUID(a.ID),
		ShortID:       a.ShortID,
		TransactionID: formatUUID(a.TransactionID),
		Kind:          a.Kind,
		ActorType:     a.ActorType,
		ActorID:       textPtr(a.ActorID),
		ActorName:     displayName,
		CreatedAt:     pgconv.TimestampStr(a.CreatedAt),
	}

	if a.SessionID.Valid {
		s := formatUUID(a.SessionID)
		ann.SessionID = &s
	}
	if a.TagID.Valid {
		s := formatUUID(a.TagID)
		ann.TagID = &s
	}
	if a.RuleID.Valid {
		s := formatUUID(a.RuleID)
		ann.RuleID = &s
	}

	if len(a.Payload) > 0 && string(a.Payload) != "{}" {
		var payload map[string]interface{}
		if err := json.Unmarshal(a.Payload, &payload); err == nil {
			ann.Payload = payload
		}
	}

	if a.ActorUpdatedAt.Valid {
		ann.ActorAvatarVersion = strconv.FormatInt(a.ActorUpdatedAt.Time.Unix(), 10)
	}

	if a.DeletedAt.Valid {
		ann.IsDeleted = true
	}

	return ann
}
