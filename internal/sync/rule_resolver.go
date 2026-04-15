package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Condition represents a single condition or condition tree in a rule's JSONB conditions.
type Condition struct {
	Field string      `json:"field,omitempty"`
	Op    string      `json:"op,omitempty"`
	Value interface{} `json:"value,omitempty"`
	And   []Condition `json:"and,omitempty"`
	Or    []Condition `json:"or,omitempty"`
	Not   *Condition  `json:"not,omitempty"`
}

// TransactionContext holds the fields available for rule evaluation during sync.
type TransactionContext struct {
	Name             string
	MerchantName     string  // may be empty
	Amount           float64
	CategoryPrimary  string  // raw provider category
	CategoryDetailed string  // raw provider detailed category
	Pending          bool
	Provider         string  // "plaid", "teller", "csv"
	AccountID        string  // UUID string
	UserID           string  // UUID string
	UserName         string  // family member name
	// Tags is nil in Phase 1. Phase 2 populates this from transaction_tags so
	// tag-based conditions (field: "tags") can match against it.
	Tags []string
}

// RuleActionSource tracks which rule contributed which action for audit.
type RuleActionSource struct {
	RuleID      pgtype.UUID
	ActionField string // "category", "tag", "comment"
	ActionValue string // slug for category/tag, content for comment
}

// RuleActions holds the merged actions to apply to a transaction after resolving
// all matching rules. First-writer-wins per action category (set_category fires
// at most once; add_tag and add_comment accumulate across rules).
type RuleActions struct {
	// CategorySlug is the slug chosen by the first rule whose set_category
	// action matched. Empty when no rule set a category.
	CategorySlug string
	// TagsToAdd is the accumulated list of unique tag slugs from add_tag
	// actions. Phase 1: plumbed but not persisted; Phase 2 wires persistence.
	TagsToAdd []string
	// Comments is the accumulated list of comment content strings from
	// add_comment actions. Phase 1: plumbed but not persisted.
	Comments []string
	// Sources records per-action provenance for the audit trail.
	Sources []RuleActionSource
}

// typedAction is the in-package parsed shape of a rule action (Phase 1).
// Kept local so the sync package doesn't import service (preserves the
// one-way service → sync dependency direction).
type typedAction struct {
	Type         string
	CategorySlug string
	TagSlug      string
	Content      string
}

// RuleResolver loads transaction rules and evaluates them during sync.
// All matching rules contribute actions (merge non-conflicting).
type RuleResolver struct {
	rules           []compiledRule
	slugCache       map[[16]byte]string      // category UUID bytes -> slug
	slugToID        map[string]pgtype.UUID   // category slug -> UUID (reverse cache)
	uncategorizedID pgtype.UUID
	hitCounts       map[[16]byte]int // rule UUID bytes -> hit count accumulator
}

type compiledRule struct {
	id        pgtype.UUID
	actions   []typedAction
	trigger   string // "on_create", "on_update", or "always"
	condition *compiledCondition
}

type compiledCondition struct {
	field string
	op    string
	value interface{}
	regex *regexp.Regexp // pre-compiled for "matches" operator
	and   []*compiledCondition
	or    []*compiledCondition
	not   *compiledCondition
}

// ruleRow holds the raw data from the transaction_rules table query.
type ruleRow struct {
	id          pgtype.UUID
	conditions  []byte // may be NULL (match-all)
	actionsJSON []byte
	trigger     string
}

// NewRuleResolver creates a resolver pre-loaded with transaction rules.
// If the transaction_rules table does not exist, it logs a warning and proceeds with no rules.
func NewRuleResolver(ctx context.Context, pool *pgxpool.Pool, provider string, logger *slog.Logger) (*RuleResolver, error) {
	r := &RuleResolver{
		slugCache: make(map[[16]byte]string),
		slugToID:  make(map[string]pgtype.UUID),
		hitCounts: make(map[[16]byte]int),
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

	return r, nil
}

// loadRules queries the transaction_rules table for active, non-expired rules,
// compiles their conditions, parses actions, and returns them sorted by
// priority DESC, created_at DESC.
func loadRules(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) ([]compiledRule, error) {
	query := `SELECT id, conditions, actions, trigger
		FROM transaction_rules
		WHERE enabled = true
		  AND (expires_at IS NULL OR expires_at > NOW())
		ORDER BY priority DESC, created_at DESC`

	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query transaction_rules: %w", err)
	}
	defer rows.Close()

	var rawRules []ruleRow
	for rows.Next() {
		var rr ruleRow
		if err := rows.Scan(&rr.id, &rr.conditions, &rr.actionsJSON, &rr.trigger); err != nil {
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

		trigger := rr.trigger
		if trigger == "" {
			trigger = "on_create"
		}

		compiled = append(compiled, compiledRule{
			id:        rr.id,
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
		case "add_tag":
			slug, _ := m["tag_slug"].(string)
			out = append(out, typedAction{Type: t, TagSlug: slug})
		case "add_comment":
			content, _ := m["content"].(string)
			out = append(out, typedAction{Type: t, Content: content})
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

// UncategorizedID returns the UUID of the "uncategorized" fallback category.
func (r *RuleResolver) UncategorizedID() pgtype.UUID {
	return r.uncategorizedID
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

// ResolveWithContext evaluates all transaction rules in priority order and
// returns the merged actions. Rules whose trigger doesn't match isNew are
// skipped.
//
// Trigger semantics:
//   - "on_create": fires only when isNew is true.
//   - "on_update": fires only when isNew is false (caller decides isChanged).
//   - "always":    fires regardless.
//
// Action merging:
//   - set_category: first-writer-wins by priority DESC.
//   - add_tag:      accumulates unique slugs across matching rules.
//   - add_comment:  accumulates all content strings.
//
// Returns nil when no rule matches.
func (r *RuleResolver) ResolveWithContext(providerName string, txn TransactionContext, isNew bool) *RuleActions {
	var result *RuleActions
	for i := range r.rules {
		rule := &r.rules[i]
		if !triggerMatches(rule.trigger, isNew) {
			continue
		}
		if !evaluateCondition(rule.condition, txn) {
			continue
		}
		r.hitCounts[rule.id.Bytes]++
		if result == nil {
			result = &RuleActions{}
		}
		for _, a := range rule.actions {
			switch a.Type {
			case "set_category":
				// First set_category wins.
				if result.CategorySlug == "" && a.CategorySlug != "" {
					result.CategorySlug = a.CategorySlug
					result.Sources = append(result.Sources, RuleActionSource{
						RuleID:      rule.id,
						ActionField: "category",
						ActionValue: a.CategorySlug,
					})
				}
			case "add_tag":
				if a.TagSlug == "" {
					continue
				}
				if !stringSliceContains(result.TagsToAdd, a.TagSlug) {
					result.TagsToAdd = append(result.TagsToAdd, a.TagSlug)
					result.Sources = append(result.Sources, RuleActionSource{
						RuleID:      rule.id,
						ActionField: "tag",
						ActionValue: a.TagSlug,
					})
				}
			case "add_comment":
				if a.Content == "" {
					continue
				}
				result.Comments = append(result.Comments, a.Content)
				result.Sources = append(result.Sources, RuleActionSource{
					RuleID:      rule.id,
					ActionField: "comment",
					ActionValue: a.Content,
				})
			}
		}
	}
	return result
}

// triggerMatches reports whether a rule with the given trigger should fire
// given an isNew signal from the sync classification.
func triggerMatches(trigger string, isNew bool) bool {
	switch trigger {
	case "on_create":
		return isNew
	case "on_update":
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
	case "name":
		return evaluateString(c, tctx.Name)
	case "merchant_name":
		return evaluateString(c, tctx.MerchantName)
	case "amount":
		return evaluateNumeric(c, tctx.Amount)
	case "category_primary":
		return evaluateString(c, tctx.CategoryPrimary)
	case "category_detailed":
		return evaluateString(c, tctx.CategoryDetailed)
	case "pending":
		return evaluateBool(c, tctx.Pending)
	case "provider":
		return evaluateString(c, tctx.Provider)
	case "account_id":
		return evaluateString(c, tctx.AccountID)
	case "user_id":
		return evaluateString(c, tctx.UserID)
	case "user_name":
		return evaluateString(c, tctx.UserName)
	case "tags":
		return evaluateTags(c, tctx.Tags)
	default:
		return false // unknown field
	}
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

// evaluateTags applies slice operators for the tags field.
//
// In Phase 1, tctx.Tags is nil during sync. This means:
//   - contains → false ("no tags" never contains the target)
//   - not_contains → true
//   - in → false
//
// Phase 2 populates tctx.Tags and these operators exercise real semantics.
func evaluateTags(c *compiledCondition, tags []string) bool {
	switch c.op {
	case "contains":
		return stringSliceContainsFold(tags, toString(c.value))
	case "not_contains":
		return !stringSliceContainsFold(tags, toString(c.value))
	case "in":
		list, ok := c.value.([]interface{})
		if !ok {
			return false
		}
		for _, item := range list {
			if stringSliceContainsFold(tags, toString(item)) {
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
		return strings.EqualFold(val, "true")
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

// stringSliceContains reports whether slice contains target (case-sensitive).
func stringSliceContains(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}

// stringSliceContainsFold reports whether slice contains target (case-insensitive).
func stringSliceContainsFold(slice []string, target string) bool {
	if target == "" {
		return false
	}
	for _, s := range slice {
		if strings.EqualFold(s, target) {
			return true
		}
	}
	return false
}
