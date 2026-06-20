//go:build !lite

package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"breadbox/internal/pgconv"
	"breadbox/internal/sliceutil"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Condition represents a single condition or condition tree in a rule's JSONB conditions.
type Condition struct {
	Field string      `json:"field,omitempty"`
	Op    string      `json:"op,omitempty"`
	Value interface{} `json:"value,omitempty"`
	// Tolerance is the ± window for the numeric `approx` operator; Min/Max
	// bound the `between` operator. Mirrors service.Condition so a rule's JSONB
	// round-trips identically between the write-time validator and this
	// sync-time evaluator.
	Tolerance *float64    `json:"tolerance,omitempty"`
	Min       *float64    `json:"min,omitempty"`
	Max       *float64    `json:"max,omitempty"`
	And       []Condition `json:"and,omitempty"`
	Or        []Condition `json:"or,omitempty"`
	Not       *Condition  `json:"not,omitempty"`
}

// TransactionContext holds the fields available for rule evaluation during sync.
type TransactionContext struct {
	Name             string
	MerchantName     string // may be empty
	Amount           float64
	CategoryPrimary  string // raw provider category
	CategoryDetailed string // raw provider detailed category
	// Category is the transaction's *assigned* category slug (distinct from
	// CategoryPrimary's raw provider value). It updates mid-resolver as
	// earlier-stage rules' set_category actions fire, so later-stage rules
	// can condition on the current category via field="category".
	Category    string
	Pending     bool
	Provider    string // "plaid", "teller", "csv"
	AccountID   string // UUID string
	AccountName string // account display name (for field="account_name" conditions)
	UserID      string // UUID string
	UserName    string // family member name
	// Tags is populated from transaction_tags so tag-based conditions
	// (field: "tags") can match against the transaction's current tags.
	// Updated mid-resolver as earlier-stage add_tag actions apply.
	Tags []string
	// SeriesShortID is the short_id of the recurring series this transaction
	// belongs to (field: "series"), empty when unassigned. Resolved from the
	// resolver's series cache at sync time. For newly-synced rows it is empty
	// until the post-sync detector links them — series conditions therefore
	// matter mostly for changed / re-synced rows and retroactive apply.
	SeriesShortID string
	// InSeries reports whether the transaction is linked to any recurring
	// series (field: "in_series"). Same timing caveat as SeriesShortID.
	InSeries bool
	// CounterpartyShortID is the short_id of the counterparty this transaction is
	// bound to (field: "counterparty"), empty when unassigned. Resolved from the
	// resolver's counterparty cache at sync time. Like series_id, counterparty_id
	// is NULL on freshly-synced rows (resolved post-upsert by assign_counterparty
	// rules), so this matters mostly for changed / re-synced rows + retroactive.
	CounterpartyShortID string
	// HasCounterparty reports whether the transaction is bound to any counterparty
	// (field: "has_counterparty"). Same timing caveat as CounterpartyShortID.
	HasCounterparty bool
	// Metadata holds the transaction's free-form metadata blob (the JSONB
	// `metadata` column) so conditions on dotted fields (field:
	// "metadata.<key>") can read arbitrary enrichment values. Updated
	// mid-resolver as earlier-stage set_metadata / remove_metadata actions
	// apply, so later-stage rules observe the running blob.
	Metadata map[string]any
	// Date is the transaction's tz-naive posting date (the `date` column, no
	// time component). The derived date-part condition fields (day_of_month /
	// month / day_of_week / day_of_year) are computed from it; a zero Date makes
	// those conditions evaluate to false.
	Date time.Time
}

// RuleActionSource tracks which rule contributed which action for audit.
// RuleName and RuleShortID are populated so the sync engine can write
// annotations with human-readable actor fields.
type RuleActionSource struct {
	RuleID      pgtype.UUID
	RuleShortID string
	RuleName    string
	ActionField string // "category", "tag", "tag_remove", "comment", "series", "metadata", "metadata_remove"
	ActionValue string // slug for category/tag, content for comment, key for metadata
}

// RuleActions holds the merged actions to apply to a transaction after resolving
// all matching rules under pipeline-stage (priority ASC) ordering.
//
// Merge semantics:
//   - set_category: last-writer-wins. Lower-priority rules run first (baseline),
//     higher-priority rules run later and may overwrite the category.
//   - add_tag: accumulates unique slugs across matching rules.
//   - remove_tag: accumulates slugs to delete. If an earlier-stage rule added
//     a slug that a later-stage rule removes in the same pass, both cancel
//     and neither appears in the DB-write set.
//   - add_comment: accumulates all content strings across matching rules.
//
// Rules evaluate against a live-mutating TransactionContext, so later-priority
// rules can react to earlier rules' tag and category changes via the `tags`
// and `category` condition fields.
type RuleActions struct {
	// CategorySlug is the slug chosen by the last rule whose set_category
	// action matched. Empty when no rule set a category.
	CategorySlug string
	// TagsToAdd is the net list of unique tag slugs to insert into
	// transaction_tags. Cancelled by a later-stage remove_tag in the same pass.
	TagsToAdd []string
	// TagsToRemove is the net list of tag slugs to delete from transaction_tags.
	// Only slugs that were present in the transaction's initial tag set and
	// were not re-added by an earlier-stage rule appear here.
	TagsToRemove []string
	// Comments is the accumulated list of comment content strings from
	// add_comment actions.
	Comments []string
	// SeriesAssign, when non-nil, links the transaction to a recurring series.
	// Last-writer-wins: the highest-priority matching rule owns the assignment
	// (a transaction belongs to at most one series).
	SeriesAssign *SeriesAssignIntent
	// CounterpartyAssign, when non-nil, binds the transaction to a counterparty.
	// Last-writer-wins: the highest-priority matching rule owns the binding
	// (a transaction binds at most one counterparty).
	CounterpartyAssign *CounterpartyAssignIntent
	// MetadataSet maps metadata keys to the value the net-surviving set_metadata
	// action wrote. Last-writer-wins per key across the pipeline. A later-stage
	// remove_metadata for the same key cancels the set (the key won't appear
	// here).
	MetadataSet map[string]any
	// MetadataRemove is the net list of metadata keys to delete. A key set by an
	// earlier-stage rule then removed by a later one cancels (appears in
	// neither map); a key removed then re-set ends up only in MetadataSet.
	MetadataRemove []string
	// FlagIntent is the net flag/unflag decision for the transaction:
	// "flag" sets flagged_at = NOW(), "unflag" clears it, "" leaves it
	// untouched. Last-writer-wins across the pipeline (a higher-priority rule's
	// flag/unflag overrides a lower one), mirroring the flag_transaction MCP
	// tool's flagged_at write.
	FlagIntent string
	// Sources records per-action provenance for the audit trail. For
	// set_category, only the winning (last) rule's source is retained.
	// For tag actions, only net-surviving adds/removes have sources.
	Sources []RuleActionSource
}

// typedAction is the in-package parsed shape of a rule action.
// Kept local so the sync package doesn't import service (preserves the
// one-way service → sync dependency direction).
type typedAction struct {
	Type                string
	CategorySlug        string
	TagSlug             string
	Content             string
	SeriesShortID       string
	SeriesName          string
	CounterpartyShortID string
	CounterpartyName    string
	CreateIfMissing     bool
	MetadataKey         string
	MetadataValue       any
}

// SeriesAssignIntent is the resolved assign_series action: link the
// transaction to an existing series (SeriesShortID) or mint one by name
// (SeriesName + CreateIfMissing). A transaction joins at most one series, so
// this is last-writer-wins across matching rules.
type SeriesAssignIntent struct {
	SeriesShortID   string
	SeriesName      string
	CreateIfMissing bool
}

// CounterpartyAssignIntent is the resolved assign_counterparty action: bind the
// transaction to an existing counterparty (CounterpartyShortID) or resolve-or-
// create one by name (CounterpartyName + CreateIfMissing). A transaction binds at
// most one counterparty, so this is last-writer-wins across matching rules.
type CounterpartyAssignIntent struct {
	CounterpartyShortID string
	CounterpartyName    string
	CreateIfMissing     bool
}

// RuleResolver loads transaction rules and evaluates them during sync.
// All matching rules contribute actions (merge non-conflicting).
type RuleResolver struct {
	rules           []compiledRule
	slugCache       map[[16]byte]string    // category UUID bytes -> slug
	slugToID        map[string]pgtype.UUID // category slug -> UUID (reverse cache)
	seriesShortID   map[[16]byte]string    // recurring_series UUID bytes -> short_id
	cpShortID       map[[16]byte]string    // counterparties UUID bytes -> short_id
	uncategorizedID pgtype.UUID
	hitCounts       map[[16]byte]int // rule UUID bytes -> hit count accumulator
}

type compiledRule struct {
	id        pgtype.UUID
	shortID   string
	name      string
	actions   []typedAction
	trigger   string // "on_create", "on_update", or "always"
	condition *compiledCondition
}

type compiledCondition struct {
	field string
	op    string
	value interface{}
	regex *regexp.Regexp // pre-compiled for "matches" operator
	// tolerance / min / max carry the approx / between numeric-operator params.
	tolerance *float64
	min       *float64
	max       *float64
	and       []*compiledCondition
	or        []*compiledCondition
	not       *compiledCondition
}

// ruleRow holds the raw data from the transaction_rules table query.
type ruleRow struct {
	id          pgtype.UUID
	shortID     string
	name        string
	conditions  []byte // may be NULL (match-all)
	actionsJSON []byte
	trigger     string
}

// NewRuleResolver creates a resolver pre-loaded with transaction rules.
// If the transaction_rules table does not exist, it logs a warning and proceeds with no rules.
func NewRuleResolver(ctx context.Context, pool *pgxpool.Pool, provider string, logger *slog.Logger) (*RuleResolver, error) {
	r := &RuleResolver{
		slugCache:     make(map[[16]byte]string),
		slugToID:      make(map[string]pgtype.UUID),
		seriesShortID: make(map[[16]byte]string),
		cpShortID:     make(map[[16]byte]string),
		hitCounts:     make(map[[16]byte]int),
	}

	// Load transaction rules. Gracefully handle missing table.
	rules, err := loadRules(ctx, pool, logger)
	if err != nil {
		logger.Warn("failed to load transaction rules", "error", err)
	} else {
		r.rules = rules
	}

	// Load the uncategorized category ID.
	err = pool.QueryRow(ctx, "SELECT id FROM categories WHERE slug = 'uncategorized'").Scan(&r.uncategorizedID)
	if err != nil {
		return nil, fmt.Errorf("load uncategorized category: %w", err)
	}

	// Load category slug cache — populate both id→slug and slug→id maps in the
	// same pass so slug lookups are O(1) without requiring a separate query.
	slugRows, err := pool.Query(ctx, "SELECT id, slug FROM categories")
	if err != nil {
		return nil, fmt.Errorf("load category slugs: %w", err)
	}
	defer slugRows.Close()
	for slugRows.Next() {
		var id pgtype.UUID
		var slug string
		if err := slugRows.Scan(&id, &slug); err != nil {
			return nil, fmt.Errorf("scan category slug: %w", err)
		}
		r.slugCache[id.Bytes] = slug
		r.slugToID[slug] = id
	}

	// Load the recurring-series id→short_id cache so the `series` condition
	// field can resolve a transaction's series_id to its short_id without a
	// per-row query. Tolerate a missing table (older deployments / lite) the
	// same way loadRules does — series conditions simply won't match.
	seriesRows, err := pool.Query(ctx, "SELECT id, short_id FROM recurring_series")
	if err != nil {
		logger.Warn("failed to load recurring series cache; series conditions will not match", "error", err)
	} else {
		defer seriesRows.Close()
		for seriesRows.Next() {
			var id pgtype.UUID
			var shortID string
			if err := seriesRows.Scan(&id, &shortID); err != nil {
				return nil, fmt.Errorf("scan recurring series short_id: %w", err)
			}
			r.seriesShortID[id.Bytes] = shortID
		}
	}

	// Load the counterparty id→short_id cache so the `counterparty` condition
	// field can resolve a transaction's counterparty_id to its short_id without a
	// per-row query. Tolerate a missing table (older deployments / lite / before
	// the P4 migration) the same way the series cache does — counterparty
	// conditions simply won't match.
	cpRows, err := pool.Query(ctx, "SELECT id, short_id FROM counterparties")
	if err != nil {
		logger.Warn("failed to load counterparties cache; counterparty conditions will not match", "error", err)
	} else {
		defer cpRows.Close()
		for cpRows.Next() {
			var id pgtype.UUID
			var shortID string
			if err := cpRows.Scan(&id, &shortID); err != nil {
				return nil, fmt.Errorf("scan counterparty short_id: %w", err)
			}
			r.cpShortID[id.Bytes] = shortID
		}
	}

	return r, nil
}

// loadRules queries the transaction_rules table for active, non-expired rules,
// compiles their conditions, parses actions, and returns them sorted by
// priority ASC, created_at ASC — pipeline-stage order. Lower priority runs
// first (baseline / foundation), higher priority runs last and wins
// set_category under the last-writer-wins resolver merge.
func loadRules(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) ([]compiledRule, error) {
	query := `SELECT id, short_id, name, conditions, actions, trigger
		FROM transaction_rules
		WHERE enabled = true
		  AND (expires_at IS NULL OR expires_at > NOW())
		ORDER BY priority ASC, created_at ASC`

	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query transaction_rules: %w", err)
	}
	defer rows.Close()

	var rawRules []ruleRow
	for rows.Next() {
		var rr ruleRow
		if err := rows.Scan(&rr.id, &rr.shortID, &rr.name, &rr.conditions, &rr.actionsJSON, &rr.trigger); err != nil {
			return nil, fmt.Errorf("scan transaction rule: %w", err)
		}
		rawRules = append(rawRules, rr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transaction rules: %w", err)
	}

	compiled := make([]compiledRule, 0, len(rawRules))
	for _, rr := range rawRules {
		// NULL or empty conditions == match-all.
		var cc *compiledCondition
		if len(rr.conditions) > 0 {
			var cond Condition
			if err := json.Unmarshal(rr.conditions, &cond); err != nil {
				logger.Warn("skipping rule with invalid conditions JSON",
					"rule_id", pgconv.FormatUUID(rr.id), "error", err)
				continue
			}
			compiled2, err := compileCondition(&cond)
			if err != nil {
				logger.Warn("skipping rule with invalid condition",
					"rule_id", pgconv.FormatUUID(rr.id), "error", err)
				continue
			}
			cc = compiled2
		}

		// Parse typed actions JSONB. Unknown types are skipped with a warning
		// (read-time tolerance — so unknown future types don't brick sync).
		actions := parseTypedActions(rr.actionsJSON, rr.id, logger)
		// Skip rules whose action list is empty after parsing — either the
		// JSONB stored an empty array (shouldn't happen post-validation) or
		// every action was an unknown type that read-time tolerance dropped.
		// Loading such a rule would bump hit_count for every matching txn
		// without producing any DB effect — misleading metric noise.
		if len(actions) == 0 {
			logger.Warn("skipping rule with no effective actions",
				"rule_id", pgconv.FormatUUID(rr.id), "name", rr.name)
			continue
		}

		trigger := rr.trigger
		if trigger == "" {
			trigger = "on_create"
		}

		compiled = append(compiled, compiledRule{
			id:        rr.id,
			shortID:   rr.shortID,
			name:      rr.name,
			actions:   actions,
			trigger:   trigger,
			condition: cc,
		})
	}

	return compiled, nil
}

// parseTypedActions unmarshals the actions JSONB column into typedAction
// values, tolerating unknown types (logged warning, skipped).
func parseTypedActions(raw []byte, ruleID pgtype.UUID, logger *slog.Logger) []typedAction {
	if len(raw) == 0 {
		return nil
	}
	var rawActions []map[string]any
	if err := json.Unmarshal(raw, &rawActions); err != nil {
		logger.Warn("rule actions invalid JSON",
			"rule_id", pgconv.FormatUUID(ruleID), "error", err)
		return nil
	}
	out := make([]typedAction, 0, len(rawActions))
	for _, m := range rawActions {
		t, _ := m["type"].(string)
		switch t {
		case "set_category":
			slug, _ := m["category_slug"].(string)
			out = append(out, typedAction{Type: t, CategorySlug: slug})
		case "add_tag", "remove_tag":
			slug, _ := m["tag_slug"].(string)
			out = append(out, typedAction{Type: t, TagSlug: slug})
		case "add_comment":
			content, _ := m["content"].(string)
			out = append(out, typedAction{Type: t, Content: content})
		case "assign_series":
			seriesShortID, _ := m["series_short_id"].(string)
			seriesName, _ := m["series_name"].(string)
			// Backward-compat: a rule authored before the surrogate-first rebuild
			// (P2) stored the mint target under `merchant_key`. Map it onto
			// SeriesName so existing rules keep firing. An explicit series_name wins.
			if seriesName == "" {
				seriesName, _ = m["merchant_key"].(string)
			}
			createIfMissing, _ := m["create_if_missing"].(bool)
			out = append(out, typedAction{Type: t, SeriesShortID: seriesShortID, SeriesName: seriesName, CreateIfMissing: createIfMissing})
		case "assign_counterparty":
			cpShortID, _ := m["counterparty_short_id"].(string)
			cpName, _ := m["counterparty_name"].(string)
			createIfMissing, _ := m["create_if_missing"].(bool)
			out = append(out, typedAction{Type: t, CounterpartyShortID: cpShortID, CounterpartyName: cpName, CreateIfMissing: createIfMissing})
		case "set_metadata":
			key, _ := m["metadata_key"].(string)
			out = append(out, typedAction{Type: t, MetadataKey: key, MetadataValue: m["metadata_value"]})
		case "remove_metadata":
			key, _ := m["metadata_key"].(string)
			out = append(out, typedAction{Type: t, MetadataKey: key})
		case "flag", "unflag":
			// No parameters — surfaces / clears the transaction's flag.
			out = append(out, typedAction{Type: t})
		default:
			logger.Warn("skipping unknown rule action type",
				"rule_id", pgconv.FormatUUID(ruleID), "type", t)
		}
	}
	return out
}

// compileCondition converts a parsed Condition into a compiledCondition tree,
// pre-compiling regexes for "matches" operators.
//
// Returns (nil, nil) for a zero-value Condition{} — a nil compiledCondition
// evaluates to true (match-all).
func compileCondition(c *Condition) (*compiledCondition, error) {
	if c == nil {
		return nil, nil
	}
	isLogical := len(c.And) > 0 || len(c.Or) > 0 || c.Not != nil
	if !isLogical && c.Field == "" {
		// Empty match-all sentinel.
		return nil, nil
	}

	cc := &compiledCondition{}

	// Branch: AND
	if len(c.And) > 0 {
		cc.and = make([]*compiledCondition, 0, len(c.And))
		for i := range c.And {
			child, err := compileCondition(&c.And[i])
			if err != nil {
				return nil, err
			}
			cc.and = append(cc.and, child)
		}
		return cc, nil
	}

	// Branch: OR
	if len(c.Or) > 0 {
		cc.or = make([]*compiledCondition, 0, len(c.Or))
		for i := range c.Or {
			child, err := compileCondition(&c.Or[i])
			if err != nil {
				return nil, err
			}
			cc.or = append(cc.or, child)
		}
		return cc, nil
	}

	// Branch: NOT
	if c.Not != nil {
		child, err := compileCondition(c.Not)
		if err != nil {
			return nil, err
		}
		cc.not = child
		return cc, nil
	}

	// Leaf node: field + op + value
	cc.field = c.Field
	cc.op = c.Op
	cc.value = c.Value
	cc.tolerance = c.Tolerance
	cc.min = c.Min
	cc.max = c.Max

	// Pre-compile regex for "matches" operator.
	if c.Op == "matches" {
		pattern, ok := c.Value.(string)
		if !ok {
			return nil, fmt.Errorf("matches operator requires string pattern, got %T", c.Value)
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid regex pattern %q: %w", pattern, err)
		}
		cc.regex = re
	}

	return cc, nil
}

// CategorySlug returns the slug for a category UUID, or empty string if unknown.
func (r *RuleResolver) CategorySlug(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return r.slugCache[id.Bytes]
}

// CategoryIDForSlug returns the pgtype.UUID for a category slug. If the slug
// is unknown, the returned UUID has Valid=false.
func (r *RuleResolver) CategoryIDForSlug(slug string) pgtype.UUID {
	if slug == "" {
		return pgtype.UUID{}
	}
	return r.slugToID[slug]
}

// SeriesShortID returns the short_id for a recurring-series UUID, or empty
// string if the series is unknown to the resolver's cache (e.g. minted after
// resolver construction, or the table was unavailable at load time).
func (r *RuleResolver) SeriesShortID(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return r.seriesShortID[id.Bytes]
}

// CounterpartyShortID returns the short_id for a counterparty UUID, or empty
// string if the counterparty is unknown to the resolver's cache (e.g. created
// after resolver construction, or the table was unavailable at load time).
func (r *RuleResolver) CounterpartyShortID(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return r.cpShortID[id.Bytes]
}

// ResolveWithContext evaluates all transaction rules in pipeline-stage order
// (priority ASC, created_at ASC) and returns the merged actions. Rules whose
// trigger doesn't match isNew are skipped.
//
// Trigger semantics:
//   - "on_create": fires only when isNew is true.
//   - "on_update": fires only when isNew is false (caller decides isChanged).
//   - "always":    fires regardless.
//
// Chaining: rules operate against a live-mutating copy of TransactionContext.
// As earlier-stage rules apply their add_tag / set_category actions, the
// local context updates so later-stage rules' conditions can observe them.
// This enables composition ("rule A tags 'coffee'; rule B reacts to the tag
// and picks a category"). Provenance/precedence was removed in P3 — rules
// write category_id directly (last-writer-wins), so there is no override that
// suppresses set_category.
//
// Action merging:
//   - set_category: last-writer-wins. The highest-priority matching rule
//     owns the category; its source is the only one retained for the
//     category field in the audit trail.
//   - add_tag:      accumulates unique slugs across matching rules.
//   - add_comment:  accumulates all content strings.
//
// Returns nil when no rule matches.
func (r *RuleResolver) ResolveWithContext(providerName string, txn TransactionContext, isNew bool) *RuleActions {
	// Work on a local copy so chaining mutations don't leak back to the caller.
	tctx := txn
	if len(txn.Tags) > 0 {
		tctx.Tags = append([]string(nil), txn.Tags...)
	}
	// Snapshot the keys present in the original DB blob and work on a copy of the
	// metadata map so chaining mutations don't leak into the caller's map.
	// origMetaKeys lets the remove_metadata net-diff below distinguish a
	// pre-existing key (a set-then-remove must still delete it) from a key
	// created within this pass (a true no-op).
	var origMetaKeys map[string]struct{}
	if len(txn.Metadata) > 0 {
		origMetaKeys = make(map[string]struct{}, len(txn.Metadata))
		tctx.Metadata = make(map[string]any, len(txn.Metadata))
		for k, v := range txn.Metadata {
			origMetaKeys[k] = struct{}{}
			tctx.Metadata[k] = v
		}
	}

	var result *RuleActions
	for i := range r.rules {
		rule := &r.rules[i]
		if !triggerMatches(rule.trigger, isNew) {
			continue
		}
		if !evaluateCondition(rule.condition, tctx) {
			continue
		}
		r.hitCounts[rule.id.Bytes]++
		if result == nil {
			result = &RuleActions{}
		}
		for _, a := range rule.actions {
			switch a.Type {
			case "set_category":
				if a.CategorySlug == "" {
					continue
				}
				// Last-writer-wins: overwrite prior category and drop the
				// superseded source so the audit trail reflects only the
				// rule that actually owns the final category.
				result.CategorySlug = a.CategorySlug
				result.Sources = dropCategorySource(result.Sources)
				result.Sources = append(result.Sources, RuleActionSource{
					RuleID:      rule.id,
					RuleShortID: rule.shortID,
					RuleName:    rule.name,
					ActionField: "category",
					ActionValue: a.CategorySlug,
				})
				// Mirror the new category into the live context so later
				// rules referencing the `category` field (assigned slug)
				// observe this pipeline-stage result.
				tctx.Category = a.CategorySlug
			case "add_tag":
				if a.TagSlug == "" {
					continue
				}
				// If a prior-stage rule's remove_tag had queued this slug,
				// the later add cancels it — strip from TagsToRemove and
				// drop the prior remove source.
				if i := sliceutil.IndexFold(result.TagsToRemove, a.TagSlug); i >= 0 {
					result.TagsToRemove = append(result.TagsToRemove[:i], result.TagsToRemove[i+1:]...)
					result.Sources = dropTagSource(result.Sources, "tag_remove", a.TagSlug)
				}
				if !sliceutil.Contains(result.TagsToAdd, a.TagSlug) {
					result.TagsToAdd = append(result.TagsToAdd, a.TagSlug)
					result.Sources = append(result.Sources, RuleActionSource{
						RuleID:      rule.id,
						RuleShortID: rule.shortID,
						RuleName:    rule.name,
						ActionField: "tag",
						ActionValue: a.TagSlug,
					})
				}
				// Mirror into live context so later rules' tags conditions
				// can observe the addition.
				if !sliceutil.Contains(tctx.Tags, a.TagSlug) {
					tctx.Tags = append(tctx.Tags, a.TagSlug)
				}
			case "remove_tag":
				if a.TagSlug == "" {
					continue
				}
				// If a same-pass earlier-stage rule added this slug, cancel
				// that add rather than queueing a delete. The net effect is
				// no DB write for this slug.
				if i := sliceutil.IndexFold(result.TagsToAdd, a.TagSlug); i >= 0 {
					result.TagsToAdd = append(result.TagsToAdd[:i], result.TagsToAdd[i+1:]...)
					result.Sources = dropTagSource(result.Sources, "tag", a.TagSlug)
				} else if sliceutil.Contains(tctx.Tags, a.TagSlug) {
					// Only queue a delete if the slug is actually present on
					// the transaction (loaded initial tags). Otherwise the
					// remove is a no-op.
					if !sliceutil.Contains(result.TagsToRemove, a.TagSlug) {
						result.TagsToRemove = append(result.TagsToRemove, a.TagSlug)
						result.Sources = append(result.Sources, RuleActionSource{
							RuleID:      rule.id,
							RuleShortID: rule.shortID,
							RuleName:    rule.name,
							ActionField: "tag_remove",
							ActionValue: a.TagSlug,
						})
					}
				}
				// Mirror into live context so later rules see the slug gone.
				if i := sliceutil.IndexFold(tctx.Tags, a.TagSlug); i >= 0 {
					tctx.Tags = append(tctx.Tags[:i], tctx.Tags[i+1:]...)
				}
			case "add_comment":
				if a.Content == "" {
					continue
				}
				result.Comments = append(result.Comments, a.Content)
				result.Sources = append(result.Sources, RuleActionSource{
					RuleID:      rule.id,
					RuleShortID: rule.shortID,
					RuleName:    rule.name,
					ActionField: "comment",
					ActionValue: a.Content,
				})
			case "assign_series":
				if a.SeriesShortID == "" && a.SeriesName == "" {
					continue
				}
				// Last-writer-wins: a transaction joins at most one series, so a
				// later-stage rule overrides an earlier assignment and supersedes
				// its audit source.
				result.SeriesAssign = &SeriesAssignIntent{
					SeriesShortID:   a.SeriesShortID,
					SeriesName:      a.SeriesName,
					CreateIfMissing: a.CreateIfMissing,
				}
				result.Sources = dropSeriesSource(result.Sources)
				result.Sources = append(result.Sources, RuleActionSource{
					RuleID:      rule.id,
					RuleShortID: rule.shortID,
					RuleName:    rule.name,
					ActionField: "series",
					ActionValue: seriesActionValue(a),
				})
			case "assign_counterparty":
				if a.CounterpartyShortID == "" && a.CounterpartyName == "" {
					continue
				}
				// Last-writer-wins: a transaction binds at most one counterparty,
				// so a later-stage rule overrides an earlier binding and supersedes
				// its audit source.
				result.CounterpartyAssign = &CounterpartyAssignIntent{
					CounterpartyShortID: a.CounterpartyShortID,
					CounterpartyName:    a.CounterpartyName,
					CreateIfMissing:     a.CreateIfMissing,
				}
				result.Sources = dropCounterpartySource(result.Sources)
				result.Sources = append(result.Sources, RuleActionSource{
					RuleID:      rule.id,
					RuleShortID: rule.shortID,
					RuleName:    rule.name,
					ActionField: "counterparty",
					ActionValue: counterpartyActionValue(a),
				})
			case "set_metadata":
				if a.MetadataKey == "" {
					continue
				}
				// Last-writer-wins per key. Metadata keys are case-sensitive
				// (JSONB), so matching is exact. If a prior-stage remove queued
				// this key, the later set cancels the remove.
				if i := indexExact(result.MetadataRemove, a.MetadataKey); i >= 0 {
					result.MetadataRemove = append(result.MetadataRemove[:i], result.MetadataRemove[i+1:]...)
					result.Sources = dropMetadataSource(result.Sources, "metadata_remove", a.MetadataKey)
				}
				if result.MetadataSet == nil {
					result.MetadataSet = make(map[string]any)
				}
				result.MetadataSet[a.MetadataKey] = a.MetadataValue
				// Refresh the source so the audit trail credits the winning rule.
				result.Sources = dropMetadataSource(result.Sources, "metadata", a.MetadataKey)
				result.Sources = append(result.Sources, RuleActionSource{
					RuleID:      rule.id,
					RuleShortID: rule.shortID,
					RuleName:    rule.name,
					ActionField: "metadata",
					ActionValue: a.MetadataKey,
				})
				// Mirror into the live context so later-stage rules' metadata
				// conditions observe the write.
				if tctx.Metadata == nil {
					tctx.Metadata = make(map[string]any)
				}
				tctx.Metadata[a.MetadataKey] = a.MetadataValue
			case "remove_metadata":
				if a.MetadataKey == "" {
					continue
				}
				// If a same-pass earlier-stage rule set this key, drop that
				// queued set and its source.
				if _, was := result.MetadataSet[a.MetadataKey]; was {
					delete(result.MetadataSet, a.MetadataKey)
					result.Sources = dropMetadataSource(result.Sources, "metadata", a.MetadataKey)
				}
				// Mirror into the live context so later-stage rules see it gone.
				delete(tctx.Metadata, a.MetadataKey)
				// Queue a DB delete only when the key exists in the original blob.
				// Last-writer-wins: a remove of a pre-existing key wins over an
				// earlier set; a key created and removed within this pass (or one
				// that never existed) is a net no-op needing no write or source.
				if _, existed := origMetaKeys[a.MetadataKey]; !existed {
					continue
				}
				if indexExact(result.MetadataRemove, a.MetadataKey) < 0 {
					result.MetadataRemove = append(result.MetadataRemove, a.MetadataKey)
					result.Sources = append(result.Sources, RuleActionSource{
						RuleID:      rule.id,
						RuleShortID: rule.shortID,
						RuleName:    rule.name,
						ActionField: "metadata_remove",
						ActionValue: a.MetadataKey,
					})
				}
			case "flag", "unflag":
				// Last-writer-wins: a later-stage rule's flag/unflag supersedes
				// an earlier one (a transaction is flagged or it isn't). Drop the
				// superseded source so the rule_applied audit credits the winning
				// rule only. ActionField is the action type so the dedup key and
				// audit annotation distinguish flag from unflag.
				result.FlagIntent = a.Type
				result.Sources = dropFlagSource(result.Sources)
				result.Sources = append(result.Sources, RuleActionSource{
					RuleID:      rule.id,
					RuleShortID: rule.shortID,
					RuleName:    rule.name,
					ActionField: a.Type,
				})
			}
		}
	}
	return result
}

// dropFlagSource removes any prior flag/unflag source so the final
// RuleActionSource slice records only the winning (last) rule's provenance for
// the flag decision. Non-flag sources are preserved.
func dropFlagSource(src []RuleActionSource) []RuleActionSource {
	if len(src) == 0 {
		return src
	}
	kept := src[:0]
	for _, s := range src {
		if s.ActionField == "flag" || s.ActionField == "unflag" {
			continue
		}
		kept = append(kept, s)
	}
	return kept
}

// dropCategorySource removes any prior category source so the final
// RuleActionSource slice records only the winning (last) rule's provenance
// for set_category. Non-category sources are preserved.
func dropCategorySource(src []RuleActionSource) []RuleActionSource {
	if len(src) == 0 {
		return src
	}
	kept := src[:0]
	for _, s := range src {
		if s.ActionField == "category" {
			continue
		}
		kept = append(kept, s)
	}
	return kept
}

// seriesActionValue is the audit value for an assign_series action: the
// series short_id when assigning to an existing series, else the series name
// the series is minted under.
func seriesActionValue(a typedAction) string {
	if a.SeriesShortID != "" {
		return a.SeriesShortID
	}
	return a.SeriesName
}

// dropSeriesSource removes any prior series source so the final source slice
// records only the winning (last) rule's provenance for the assignment.
func dropSeriesSource(src []RuleActionSource) []RuleActionSource {
	if len(src) == 0 {
		return src
	}
	kept := src[:0]
	for _, s := range src {
		if s.ActionField == "series" {
			continue
		}
		kept = append(kept, s)
	}
	return kept
}

// counterpartyActionValue is the audit value for an assign_counterparty action:
// the counterparty short_id when binding to an existing counterparty, else the
// name it resolves-or-creates under.
func counterpartyActionValue(a typedAction) string {
	if a.CounterpartyShortID != "" {
		return a.CounterpartyShortID
	}
	return a.CounterpartyName
}

// dropCounterpartySource removes any prior counterparty source so the final
// source slice records only the winning (last) rule's provenance for the binding.
func dropCounterpartySource(src []RuleActionSource) []RuleActionSource {
	if len(src) == 0 {
		return src
	}
	kept := src[:0]
	for _, s := range src {
		if s.ActionField == "counterparty" {
			continue
		}
		kept = append(kept, s)
	}
	return kept
}

// dropMetadataSource removes a (field, key) metadata source entry — used when a
// later-stage rule supersedes (set→set, last-writer-wins) or cancels
// (set↔remove net-diff) an earlier-stage rule's metadata intent. field is
// "metadata" (set) or "metadata_remove" (remove); value is the metadata key.
func dropMetadataSource(src []RuleActionSource, field, key string) []RuleActionSource {
	if len(src) == 0 {
		return src
	}
	kept := src[:0]
	for _, s := range src {
		if s.ActionField == field && s.ActionValue == key {
			continue
		}
		kept = append(kept, s)
	}
	return kept
}

// indexExact returns the index of target in slice using exact (case-sensitive)
// comparison, or -1. Metadata keys are JSONB keys and therefore case-sensitive,
// unlike tag slugs which match case-insensitively.
func indexExact(slice []string, target string) int {
	for i, s := range slice {
		if s == target {
			return i
		}
	}
	return -1
}

// dropTagSource removes a specific (field, value) source entry — used when a
// later-stage rule cancels an earlier-stage rule's add/remove_tag intent
// (the cancelled intent produces no DB write, so its source doesn't belong
// in the audit trail).
func dropTagSource(src []RuleActionSource, field, value string) []RuleActionSource {
	if len(src) == 0 {
		return src
	}
	kept := src[:0]
	for _, s := range src {
		if s.ActionField == field && s.ActionValue == value {
			continue
		}
		kept = append(kept, s)
	}
	return kept
}

// triggerMatches reports whether a rule with the given trigger should fire
// given an isNew signal from the sync classification. `on_update` is accepted
// as a back-compat alias for `on_change` (the preferred canonical name).
func triggerMatches(trigger string, isNew bool) bool {
	switch trigger {
	case "on_create":
		return isNew
	case "on_change", "on_update":
		return !isNew
	case "always", "":
		return true
	}
	return false
}

// HitCountsJSON returns the per-rule hit counts from this sync run as JSON bytes
// suitable for storing in the sync_logs.rule_hits JSONB column.
// Returns nil if no rules matched.
func (r *RuleResolver) HitCountsJSON() []byte {
	if len(r.hitCounts) == 0 {
		return nil
	}

	result := make(map[string]int, len(r.hitCounts))
	for uuidBytes, count := range r.hitCounts {
		id := pgtype.UUID{Bytes: uuidBytes, Valid: true}
		result[pgconv.FormatUUID(id)] = count
	}

	data, err := json.Marshal(result)
	if err != nil {
		return nil
	}
	return data
}

// FlushHitCounts updates hit_count and last_hit_at for all rules that matched
// during this sync run. Should be called after the sync transaction commits.
func (r *RuleResolver) FlushHitCounts(ctx context.Context, pool *pgxpool.Pool) error {
	if len(r.hitCounts) == 0 {
		return nil
	}

	for uuidBytes, count := range r.hitCounts {
		id := pgtype.UUID{Bytes: uuidBytes, Valid: true}
		_, err := pool.Exec(ctx,
			"UPDATE transaction_rules SET hit_count = hit_count + $1, last_hit_at = NOW() WHERE id = $2",
			count, id)
		if err != nil {
			return fmt.Errorf("update hit count for rule %s: %w", pgconv.FormatUUID(id), err)
		}
	}

	return nil
}

// evaluateCondition recursively evaluates a compiled condition tree against a transaction context.
// A nil receiver means match-all and returns true.
func evaluateCondition(c *compiledCondition, tctx TransactionContext) bool {
	if c == nil {
		return true
	}

	// Branch: AND (short-circuit on first false)
	if c.and != nil {
		for _, child := range c.and {
			if !evaluateCondition(child, tctx) {
				return false
			}
		}
		return true
	}

	// Branch: OR (short-circuit on first true)
	if c.or != nil {
		for _, child := range c.or {
			if evaluateCondition(child, tctx) {
				return true
			}
		}
		return false
	}

	// Branch: NOT
	if c.not != nil {
		return !evaluateCondition(c.not, tctx)
	}

	// Leaf: evaluate field op value
	return evaluateLeaf(c, tctx)
}

// evaluateLeaf evaluates a single field/op/value comparison.
func evaluateLeaf(c *compiledCondition, tctx TransactionContext) bool {
	switch c.field {
	case "provider_name":
		return evaluateString(c, tctx.Name)
	case "provider_merchant_name":
		return evaluateString(c, tctx.MerchantName)
	case "amount":
		return evaluateNumeric(c, tctx.Amount)
	case "provider_category_primary":
		return evaluateString(c, tctx.CategoryPrimary)
	case "provider_category_detailed":
		return evaluateString(c, tctx.CategoryDetailed)
	case "category":
		return evaluateString(c, tctx.Category)
	case "pending":
		return evaluateBool(c, tctx.Pending)
	case "provider":
		return evaluateString(c, tctx.Provider)
	case "account_id":
		return evaluateString(c, tctx.AccountID)
	case "account_name":
		return evaluateString(c, tctx.AccountName)
	case "user_id":
		return evaluateString(c, tctx.UserID)
	case "user_name":
		return evaluateString(c, tctx.UserName)
	case "tags":
		return evaluateTags(c, tctx.Tags)
	case "series":
		return evaluateString(c, tctx.SeriesShortID)
	case "in_series":
		return evaluateBool(c, tctx.InSeries)
	case "counterparty":
		return evaluateString(c, tctx.CounterpartyShortID)
	case "has_counterparty":
		return evaluateBool(c, tctx.HasCounterparty)
	case "day_of_month":
		if tctx.Date.IsZero() {
			return false
		}
		return evaluateDayOfMonth(c, tctx.Date)
	case "month":
		if tctx.Date.IsZero() {
			return false
		}
		return evaluateNumeric(c, float64(int(tctx.Date.Month())))
	case "day_of_week":
		if tctx.Date.IsZero() {
			return false
		}
		return evaluateNumeric(c, float64(int(tctx.Date.Weekday())))
	case "day_of_year":
		if tctx.Date.IsZero() {
			return false
		}
		return evaluateNumeric(c, float64(tctx.Date.YearDay()))
	default:
		if key, ok := metadataKeyFromField(c.field); ok {
			return evaluateMetadata(c, key, tctx.Metadata)
		}
		return false // unknown field
	}
}

// evaluateDayOfMonth mirrors service.evalDayOfMonth: approx uses cyclic +
// clamped matching against the transaction's actual month length; every other
// operator falls back to a plain numeric comparison on the literal day (1..31).
func evaluateDayOfMonth(c *compiledCondition, date time.Time) bool {
	day := date.Day()
	if c.op != "approx" {
		return evaluateNumeric(c, float64(day))
	}
	if c.tolerance == nil {
		return false
	}
	target := toFloat64(c.value)
	monthLen := daysInMonth(date.Year(), date.Month())
	return dayOfMonthApproxMatch(day, int(math.Round(target)), *c.tolerance, monthLen)
}

// daysInMonth returns the number of days in the given year/month.
func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// dayOfMonthApproxMatch reports whether actualDay is within tolerance of
// targetDay on a cyclic month of monthLen days. targetDay clamps into
// [1, monthLen]; distance is the smaller of the direct and wrap-around gaps.
func dayOfMonthApproxMatch(actualDay, targetDay int, tolerance float64, monthLen int) bool {
	if monthLen <= 0 {
		return false
	}
	if targetDay > monthLen {
		targetDay = monthLen
	}
	if targetDay < 1 {
		targetDay = 1
	}
	diff := actualDay - targetDay
	if diff < 0 {
		diff = -diff
	}
	if wrap := monthLen - diff; wrap < diff {
		diff = wrap
	}
	return float64(diff) <= tolerance
}

// metadataFieldPrefix marks a condition leaf that reads a key from the
// transaction's free-form metadata blob. Mirrors service.metadataFieldPrefix.
const metadataFieldPrefix = "metadata."

// metadataKeyFromField extracts the metadata key from a "metadata.<key>" field.
func metadataKeyFromField(field string) (string, bool) {
	if !strings.HasPrefix(field, metadataFieldPrefix) {
		return "", false
	}
	return field[len(metadataFieldPrefix):], true
}

// evaluateMetadata mirrors service.evalMetadata: presence operators test key
// presence; every other operator requires the key to be present. eq/neq are
// driven by the expected value's type, while contains/not_contains/matches/in
// operate on the stringified stored value. Keeping the two implementations in
// lockstep means a rule that passes write-time validation evaluates the same at
// sync time as it does in preview / retroactive apply.
func evaluateMetadata(c *compiledCondition, key string, meta map[string]any) bool {
	raw, present := meta[key]
	switch c.op {
	case "exists":
		return present
	case "not_exists":
		return !present
	}
	if !present {
		return false
	}
	switch c.op {
	case "eq":
		return metadataEquals(raw, c.value)
	case "neq":
		return !metadataEquals(raw, c.value)
	case "contains":
		return strings.Contains(strings.ToLower(metadataString(raw)), strings.ToLower(metadataString(c.value)))
	case "not_contains":
		return !strings.Contains(strings.ToLower(metadataString(raw)), strings.ToLower(metadataString(c.value)))
	case "matches":
		if c.regex != nil {
			return c.regex.MatchString(metadataString(raw))
		}
		return false
	case "in":
		actual := metadataString(raw)
		if list, ok := c.value.([]interface{}); ok {
			for _, item := range list {
				if strings.EqualFold(actual, metadataString(item)) {
					return true
				}
			}
		}
		return false
	case "gt", "gte", "lt", "lte":
		actual, ok1 := metadataFloat(raw)
		expected, ok2 := metadataFloat(c.value)
		if !ok1 || !ok2 {
			return false
		}
		switch c.op {
		case "gt":
			return actual > expected
		case "gte":
			return actual >= expected
		case "lt":
			return actual < expected
		case "lte":
			return actual <= expected
		}
	}
	return false
}

// metadataEquals compares a stored metadata value against the expected
// condition value, selecting the comparison from the expected value's type.
func metadataEquals(raw, expected any) bool {
	switch exp := expected.(type) {
	case bool:
		b, ok := metadataBool(raw)
		return ok && b == exp
	case float64, float32, int, int64, json.Number:
		ev, ok1 := metadataFloat(expected)
		av, ok2 := metadataFloat(raw)
		return ok1 && ok2 && av == ev
	default:
		return strings.EqualFold(metadataString(raw), metadataString(expected))
	}
}

// metadataString renders a metadata value as a string for string operators.
func metadataString(v any) string {
	switch val := v.(type) {
	case nil:
		return ""
	case string:
		return val
	case bool:
		if val {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case json.Number:
		return val.String()
	default:
		if b, err := json.Marshal(val); err == nil {
			return string(b)
		}
		return fmt.Sprintf("%v", val)
	}
}

// metadataFloat coerces a metadata value to float64 for numeric comparison.
func metadataFloat(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case json.Number:
		f, err := val.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(val, 64)
		return f, err == nil
	}
	return 0, false
}

// metadataBool coerces a metadata value to bool for eq/neq against a bool expected.
func metadataBool(v any) (bool, bool) {
	switch val := v.(type) {
	case bool:
		return val, true
	case string:
		b, err := strconv.ParseBool(val)
		return b, err == nil
	}
	return false, false
}

// evaluateString applies string operators.
func evaluateString(c *compiledCondition, fieldVal string) bool {
	switch c.op {
	case "eq":
		return strings.EqualFold(fieldVal, toString(c.value))
	case "neq":
		return !strings.EqualFold(fieldVal, toString(c.value))
	case "contains":
		return strings.Contains(strings.ToLower(fieldVal), strings.ToLower(toString(c.value)))
	case "not_contains":
		return !strings.Contains(strings.ToLower(fieldVal), strings.ToLower(toString(c.value)))
	case "matches":
		if c.regex != nil {
			return c.regex.MatchString(fieldVal)
		}
		return false
	case "in":
		return stringInList(fieldVal, c.value)
	default:
		return false
	}
}

// evaluateNumeric applies numeric operators.
func evaluateNumeric(c *compiledCondition, fieldVal float64) bool {
	if c.op == "between" {
		if c.min == nil || c.max == nil {
			return false
		}
		return fieldVal >= *c.min && fieldVal <= *c.max
	}
	v := toFloat64(c.value)

	switch c.op {
	case "eq":
		return fieldVal == v
	case "neq":
		return fieldVal != v
	case "gt":
		return fieldVal > v
	case "gte":
		return fieldVal >= v
	case "lt":
		return fieldVal < v
	case "lte":
		return fieldVal <= v
	case "approx":
		if c.tolerance == nil {
			return false
		}
		return math.Abs(fieldVal-v) <= *c.tolerance
	default:
		return false
	}
}

// evaluateBool applies boolean operators.
func evaluateBool(c *compiledCondition, fieldVal bool) bool {
	v := toBool(c.value)

	switch c.op {
	case "eq":
		return fieldVal == v
	case "neq":
		return fieldVal != v
	default:
		return false
	}
}

// evaluateTags applies slice operators for the tags field against the
// transaction's current tag slugs.
func evaluateTags(c *compiledCondition, tags []string) bool {
	switch c.op {
	case "contains":
		return sliceutil.ContainsFold(tags, toString(c.value))
	case "not_contains":
		return !sliceutil.ContainsFold(tags, toString(c.value))
	case "in":
		list, ok := c.value.([]interface{})
		if !ok {
			return false
		}
		for _, item := range list {
			if sliceutil.ContainsFold(tags, toString(item)) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// --- value conversion helpers ---

func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return fmt.Sprintf("%g", val)
	case bool:
		if val {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", val)
	}
}

func toFloat64(v interface{}) float64 {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case string:
		var f float64
		fmt.Sscanf(val, "%f", &f)
		return f
	default:
		return 0
	}
}

func toBool(v interface{}) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		// Mirror the write-time validator (service.ValidateCondition parses
		// bool values with strconv.ParseBool) and the service-side preview
		// evaluator, so a rule that passed validation evaluates the same way
		// here at sync time. ParseBool accepts "1"/"t"/"T"/"true"/… as true and
		// "0"/"f"/"F"/"false"/… as false; the previous EqualFold(val,"true")
		// check silently treated validator-accepted forms like "1" and "t" as
		// false, making such a rule fire on the opposite set of transactions
		// from what preview promised.
		b, err := strconv.ParseBool(val)
		if err != nil {
			return false
		}
		return b
	case float64:
		return val != 0
	default:
		return false
	}
}

// stringInList checks if fieldVal is in the value list (case-insensitive).
// The value may be a []interface{} (from JSON unmarshal) or other types.
func stringInList(fieldVal string, v interface{}) bool {
	if v == nil {
		return false
	}
	switch list := v.(type) {
	case []interface{}:
		for _, item := range list {
			if strings.EqualFold(fieldVal, toString(item)) {
				return true
			}
		}
		return false
	default:
		// Single value comparison
		return strings.EqualFold(fieldVal, toString(v))
	}
}
