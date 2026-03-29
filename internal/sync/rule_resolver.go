package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

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
}

// RuleActionSource tracks which rule contributed which action.
type RuleActionSource struct {
	RuleID      pgtype.UUID
	ActionField string // e.g., "category"
	ActionValue string // e.g., "food_and_drink_coffee"
}

// RuleActions holds the merged actions to apply to a transaction.
type RuleActions struct {
	CategoryID pgtype.UUID
	// future: MerchantName pgtype.Text, etc.

	Sources []RuleActionSource // tracks which rule set which field
}

// RuleResolver loads transaction rules and evaluates them during sync.
// All matching rules contribute actions (merge non-conflicting).
type RuleResolver struct {
	rules           []compiledRule
	slugCache       map[[16]byte]string // category UUID bytes -> slug
	uncategorizedID pgtype.UUID
	hitCounts       map[[16]byte]int // rule UUID bytes -> hit count accumulator
}

type compiledRule struct {
	id        pgtype.UUID
	actions   RuleActions
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
	id         pgtype.UUID
	categoryID pgtype.UUID
	conditions []byte
}

// NewRuleResolver creates a resolver pre-loaded with transaction rules.
// If the transaction_rules table does not exist, it logs a warning and proceeds with no rules.
func NewRuleResolver(ctx context.Context, pool *pgxpool.Pool, provider string, logger *slog.Logger) (*RuleResolver, error) {
	r := &RuleResolver{
		slugCache: make(map[[16]byte]string),
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

	// Load category slug cache for suggestion filtering.
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
	}

	return r, nil
}

// loadRules queries the transaction_rules table for active, non-expired rules,
// compiles their conditions, and returns them sorted by priority DESC, created_at DESC.
func loadRules(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) ([]compiledRule, error) {
	query := `SELECT id, category_id, conditions
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
		if err := rows.Scan(&rr.id, &rr.categoryID, &rr.conditions); err != nil {
			return nil, fmt.Errorf("scan transaction rule: %w", err)
		}
		rawRules = append(rawRules, rr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transaction rules: %w", err)
	}

	compiled := make([]compiledRule, 0, len(rawRules))
	for _, rr := range rawRules {
		var cond Condition
		if err := json.Unmarshal(rr.conditions, &cond); err != nil {
			logger.Warn("skipping rule with invalid conditions JSON",
				"rule_id", formatUUID(rr.id), "error", err)
			continue
		}

		cc, err := compileCondition(&cond)
		if err != nil {
			logger.Warn("skipping rule with invalid condition",
				"rule_id", formatUUID(rr.id), "error", err)
			continue
		}

		compiled = append(compiled, compiledRule{
			id:        rr.id,
			actions:   RuleActions{CategoryID: rr.categoryID},
			condition: cc,
		})
	}

	return compiled, nil
}

// compileCondition converts a parsed Condition into a compiledCondition tree,
// pre-compiling regexes for "matches" operators.
func compileCondition(c *Condition) (*compiledCondition, error) {
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

// ResolveWithContext evaluates all transaction rules in priority order.
// All matching rules contribute actions — first rule to set a field wins.
// Returns nil when no rule matches.
func (r *RuleResolver) ResolveWithContext(providerName string, txn TransactionContext) *RuleActions {
	var result *RuleActions
	for i := range r.rules {
		rule := &r.rules[i]
		if evaluateCondition(rule.condition, txn) {
			r.hitCounts[rule.id.Bytes]++
			if result == nil {
				result = &RuleActions{}
			}
			// Merge: first rule to set a field wins
			if !result.CategoryID.Valid && rule.actions.CategoryID.Valid {
				result.CategoryID = rule.actions.CategoryID
				slug := r.CategorySlug(rule.actions.CategoryID)
				result.Sources = append(result.Sources, RuleActionSource{
					RuleID:      rule.id,
					ActionField: "category",
					ActionValue: slug,
				})
			}
			// future: same pattern for other fields
		}
	}
	return result
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
		result[formatUUID(id)] = count
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
			return fmt.Errorf("update hit count for rule %s: %w", formatUUID(id), err)
		}
	}

	return nil
}

// evaluateCondition recursively evaluates a compiled condition tree against a transaction context.
func evaluateCondition(c *compiledCondition, tctx TransactionContext) bool {
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
