package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"breadbox/internal/pgconv"
	"breadbox/internal/ruleapply"
	"breadbox/internal/sliceutil"
	"breadbox/internal/slugs"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// --- Condition validation and evaluation ---

// validConditionFields lists the fields available for rule conditions.
//
// The "tags" field is a slice (TransactionContext.Tags) populated from
// transaction_tags at evaluation time. It supports contains, not_contains,
// and "in" (any-of).
var validConditionFields = map[string]string{
	"name":              "string",
	"merchant_name":     "string",
	"amount":            "numeric",
	"category_primary":  "string", // raw provider primary category
	"category_detailed": "string", // raw provider detailed category
	"category":          "string", // assigned category slug (distinct from provider raw)
	"pending":           "bool",
	"provider":          "string",
	"account_id":        "string",
	"account_name":      "string",
	"user_id":           "string",
	"user_name":         "string",
	"tags":              "tags",
}

// stringOps are operators valid for string fields.
var stringOps = map[string]bool{
	"eq": true, "neq": true, "contains": true, "not_contains": true, "matches": true, "in": true,
}

// numericOps are operators valid for numeric fields.
var numericOps = map[string]bool{
	"eq": true, "neq": true, "gt": true, "gte": true, "lt": true, "lte": true,
}

// boolOps are operators valid for boolean fields.
var boolOps = map[string]bool{
	"eq": true, "neq": true,
}

// tagsOps are operators valid for the "tags" slice field. Semantics:
//   - contains: c.Value (string) is present in tctx.Tags (case-insensitive).
//   - not_contains: inverse of contains.
//   - in: any element of c.Value ([]string) is present in tctx.Tags.
var tagsOps = map[string]bool{
	"contains": true, "not_contains": true, "in": true,
}

// conditionIsEmpty reports whether a Condition is a zero-value match-all.
// Used to map between DB NULL conditions and the "always match" semantic.
func conditionIsEmpty(c Condition) bool {
	return c.Field == "" && len(c.And) == 0 && len(c.Or) == 0 && c.Not == nil
}

// ValidateCondition recursively validates a condition tree.
//
// A zero-value Condition{} (no Field, no And/Or/Not) is accepted as "match all"
// and returns nil — the match-all semantic for rules that fire on every
// transaction.
func ValidateCondition(c Condition) error {
	if conditionIsEmpty(c) {
		return nil
	}
	return validateConditionDepth(c, 0)
}

func validateConditionDepth(c Condition, depth int) error {
	if depth > 10 {
		return fmt.Errorf("%w: condition nesting too deep (max 10)", ErrInvalidParameter)
	}

	isLogical := len(c.And) > 0 || len(c.Or) > 0 || c.Not != nil
	isLeaf := c.Field != ""

	if !isLogical && !isLeaf {
		return fmt.Errorf("%w: condition must have a field or logical operator (and/or/not)", ErrInvalidParameter)
	}
	if isLogical && isLeaf {
		return fmt.Errorf("%w: condition cannot have both a field and logical operators", ErrInvalidParameter)
	}

	if isLeaf {
		fieldType, ok := validConditionFields[c.Field]
		if !ok {
			return fmt.Errorf("%w: unknown field %q", ErrInvalidParameter, c.Field)
		}
		if c.Op == "" {
			return fmt.Errorf("%w: operator is required for field %q", ErrInvalidParameter, c.Field)
		}

		switch fieldType {
		case "string":
			if !stringOps[c.Op] {
				return fmt.Errorf("%w: operator %q not valid for string field %q", ErrInvalidParameter, c.Op, c.Field)
			}
			if c.Op == "in" {
				// Value should be a non-empty array
				vals, ok := toStringSlice(c.Value)
				if !ok {
					return fmt.Errorf("%w: 'in' operator requires an array value for field %q", ErrInvalidParameter, c.Field)
				}
				if len(vals) == 0 {
					return fmt.Errorf("%w: 'in' operator requires a non-empty array for field %q", ErrInvalidParameter, c.Field)
				}
			} else if c.Op == "matches" {
				s, ok := c.Value.(string)
				if !ok {
					return fmt.Errorf("%w: 'matches' operator requires a string value for field %q", ErrInvalidParameter, c.Field)
				}
				if _, err := regexp.Compile(s); err != nil {
					return fmt.Errorf("%w: invalid regex pattern for field %q: %v", ErrInvalidParameter, c.Field, err)
				}
			}
		case "numeric":
			if !numericOps[c.Op] {
				return fmt.Errorf("%w: operator %q not valid for numeric field %q", ErrInvalidParameter, c.Op, c.Field)
			}
			if _, ok := toFloat64(c.Value); !ok {
				return fmt.Errorf("%w: numeric value required for field %q", ErrInvalidParameter, c.Field)
			}
		case "bool":
			if !boolOps[c.Op] {
				return fmt.Errorf("%w: operator %q not valid for boolean field %q", ErrInvalidParameter, c.Op, c.Field)
			}
			if _, ok := toBool(c.Value); !ok {
				return fmt.Errorf("%w: boolean value required for field %q", ErrInvalidParameter, c.Field)
			}
		case "tags":
			if !tagsOps[c.Op] {
				return fmt.Errorf("%w: operator %q not valid for tags field (use contains, not_contains, or in)", ErrInvalidParameter, c.Op)
			}
			if c.Op == "in" {
				vals, ok := toStringSlice(c.Value)
				if !ok {
					return fmt.Errorf("%w: 'in' operator requires an array value for field %q", ErrInvalidParameter, c.Field)
				}
				if len(vals) == 0 {
					return fmt.Errorf("%w: 'in' operator requires a non-empty array for field %q", ErrInvalidParameter, c.Field)
				}
			} else {
				// contains/not_contains expect a single string value
				if _, ok := c.Value.(string); !ok {
					return fmt.Errorf("%w: %s on tags requires a string value", ErrInvalidParameter, c.Op)
				}
			}
		}
	}

	for i, sub := range c.And {
		if err := validateConditionDepth(sub, depth+1); err != nil {
			return fmt.Errorf("and[%d]: %w", i, err)
		}
	}
	for i, sub := range c.Or {
		if err := validateConditionDepth(sub, depth+1); err != nil {
			return fmt.Errorf("or[%d]: %w", i, err)
		}
	}
	if c.Not != nil {
		if err := validateConditionDepth(*c.Not, depth+1); err != nil {
			return fmt.Errorf("not: %w", err)
		}
	}

	return nil
}

// CompiledCondition holds a pre-compiled condition tree for faster evaluation.
type CompiledCondition struct {
	Field string
	Op    string
	Value interface{}
	Regex *regexp.Regexp

	// Pre-lowercased expected value for case-insensitive string comparisons.
	// Set at compile time so evalString avoids repeated ToLower on the expected side.
	lowerValue string
	// Pre-lowercased set for the "in" operator, avoiding per-evaluation ToLower + slice conversion.
	lowerInSet []string

	And []*CompiledCondition
	Or  []*CompiledCondition
	Not *CompiledCondition
}

// CompileCondition pre-compiles regex patterns and pre-lowercases string
// expected values in a condition tree for faster evaluation.
//
// An empty match-all condition (zero-value Condition{}) compiles to (nil, nil).
// A nil *CompiledCondition evaluates to true — see EvaluateCondition.
func CompileCondition(c Condition) (*CompiledCondition, error) {
	if conditionIsEmpty(c) {
		return nil, nil
	}
	cc := &CompiledCondition{
		Field: c.Field,
		Op:    c.Op,
		Value: c.Value,
	}

	fieldType := validConditionFields[c.Field]

	if c.Field != "" && c.Op == "matches" {
		s, _ := c.Value.(string)
		re, err := regexp.Compile(s)
		if err != nil {
			return nil, fmt.Errorf("compile regex for field %q: %w", c.Field, err)
		}
		cc.Regex = re
	}

	// Pre-lowercase string expected values at compile time.
	if fieldType == "string" && c.Field != "" {
		switch c.Op {
		case "eq", "neq", "contains", "not_contains":
			if s, ok := c.Value.(string); ok {
				cc.lowerValue = strings.ToLower(s)
			}
		case "in":
			if vals, ok := toStringSlice(c.Value); ok {
				cc.lowerInSet = make([]string, len(vals))
				for i, v := range vals {
					cc.lowerInSet[i] = strings.ToLower(v)
				}
			}
		}
	}

	// Pre-lowercase tag expected values at compile time.
	if fieldType == "tags" {
		switch c.Op {
		case "contains", "not_contains":
			if s, ok := c.Value.(string); ok {
				cc.lowerValue = strings.ToLower(s)
			}
		case "in":
			if vals, ok := toStringSlice(c.Value); ok {
				cc.lowerInSet = make([]string, len(vals))
				for i, v := range vals {
					cc.lowerInSet[i] = strings.ToLower(v)
				}
			}
		}
	}

	for _, sub := range c.And {
		compiled, err := CompileCondition(sub)
		if err != nil {
			return nil, err
		}
		cc.And = append(cc.And, compiled)
	}
	for _, sub := range c.Or {
		compiled, err := CompileCondition(sub)
		if err != nil {
			return nil, err
		}
		cc.Or = append(cc.Or, compiled)
	}
	if c.Not != nil {
		compiled, err := CompileCondition(*c.Not)
		if err != nil {
			return nil, err
		}
		cc.Not = compiled
	}

	return cc, nil
}

// EvaluateCondition recursively evaluates a compiled condition against a transaction context.
func EvaluateCondition(c *CompiledCondition, tctx TransactionContext) bool {
	if c == nil {
		return true
	}

	// Handle logical operators with short-circuit
	if len(c.And) > 0 {
		for _, sub := range c.And {
			if !EvaluateCondition(sub, tctx) {
				return false
			}
		}
		return true
	}
	if len(c.Or) > 0 {
		for _, sub := range c.Or {
			if EvaluateCondition(sub, tctx) {
				return true
			}
		}
		return false
	}
	if c.Not != nil {
		return !EvaluateCondition(c.Not, tctx)
	}

	// Leaf condition: evaluate field op value
	return evaluateLeaf(c, tctx)
}

func evaluateLeaf(c *CompiledCondition, tctx TransactionContext) bool {
	switch c.Field {
	case "name":
		return evalString(c, tctx.Name)
	case "merchant_name":
		return evalString(c, tctx.MerchantName)
	case "amount":
		return evalNumeric(c, tctx.Amount)
	case "category_primary":
		return evalString(c, tctx.CategoryPrimary)
	case "category_detailed":
		return evalString(c, tctx.CategoryDetailed)
	case "category":
		return evalString(c, tctx.Category)
	case "pending":
		return evalBool(c, tctx.Pending)
	case "provider":
		return evalString(c, tctx.Provider)
	case "account_id":
		return evalString(c, tctx.AccountID)
	case "account_name":
		return evalString(c, tctx.AccountName)
	case "user_id":
		return evalString(c, tctx.UserID)
	case "user_name":
		return evalString(c, tctx.UserName)
	case "tags":
		return evalTags(c, tctx.Tags)
	}
	return false
}

// evalTags handles contains / not_contains / in for the tags slice field.
func evalTags(c *CompiledCondition, tags []string) bool {
	switch c.Op {
	case "contains":
		return sliceutil.ContainsFold(tags, c.lowerValue)
	case "not_contains":
		return !sliceutil.ContainsFold(tags, c.lowerValue)
	case "in":
		for _, v := range c.lowerInSet {
			if sliceutil.ContainsFold(tags, v) {
				return true
			}
		}
		return false
	}
	return false
}

func evalString(c *CompiledCondition, actual string) bool {
	switch c.Op {
	case "eq":
		// EqualFold avoids allocating lowercased copies of both strings.
		return strings.EqualFold(actual, c.lowerValue)
	case "neq":
		return !strings.EqualFold(actual, c.lowerValue)
	case "contains":
		// Only ToLower the actual value; expected was pre-lowercased at compile time.
		return strings.Contains(strings.ToLower(actual), c.lowerValue)
	case "not_contains":
		return !strings.Contains(strings.ToLower(actual), c.lowerValue)
	case "matches":
		if c.Regex != nil {
			return c.Regex.MatchString(actual)
		}
		return false
	case "in":
		// Use pre-lowercased set built at compile time; EqualFold avoids ToLower on actual.
		for _, v := range c.lowerInSet {
			if strings.EqualFold(actual, v) {
				return true
			}
		}
		return false
	}
	return false
}

func evalNumeric(c *CompiledCondition, actual float64) bool {
	expected, ok := toFloat64(c.Value)
	if !ok {
		return false
	}
	switch c.Op {
	case "eq":
		return actual == expected
	case "neq":
		return actual != expected
	case "gt":
		return actual > expected
	case "gte":
		return actual >= expected
	case "lt":
		return actual < expected
	case "lte":
		return actual <= expected
	}
	return false
}

func evalBool(c *CompiledCondition, actual bool) bool {
	expected, ok := toBool(c.Value)
	if !ok {
		return false
	}
	switch c.Op {
	case "eq":
		return actual == expected
	case "neq":
		return actual != expected
	}
	return false
}

// --- Type conversion helpers ---

func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	}
	return 0, false
}

func toBool(v interface{}) (bool, bool) {
	switch b := v.(type) {
	case bool:
		return b, true
	case string:
		parsed, err := strconv.ParseBool(b)
		return parsed, err == nil
	}
	return false, false
}

func toStringSlice(v interface{}) ([]string, bool) {
	switch arr := v.(type) {
	case []string:
		return arr, true
	case []interface{}:
		result := make([]string, 0, len(arr))
		for _, item := range arr {
			if s, ok := item.(string); ok {
				result = append(result, s)
			} else {
				return nil, false
			}
		}
		return result, true
	}
	return nil, false
}

// --- Action validation ---

// validActionTypes lists the typed action types accepted at write time.
// Unknown types are rejected by ValidateActions. Read-time tolerates unknown
// types by skipping them with a logged warning (see sync/rule_resolver).
var validActionTypes = map[string]bool{
	"set_category": true,
	"add_tag":      true,
	"remove_tag":   true,
	"add_comment":  true,
}

// tagSlugPattern enforces the tag slug format: lowercase alphanumerics with
// optional hyphens/colons between, e.g. "needs-review", "subscription:monthly".
var tagSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9\-:]*[a-z0-9]$`)

// ValidateActions validates a slice of typed rule actions.
//
// Rules must have at least one action. set_category validates the category
// slug exists; add_tag validates the slug format; add_comment requires
// non-empty content. Unknown Type values are rejected.
func (s *Service) ValidateActions(ctx context.Context, actions []RuleAction) error {
	if len(actions) == 0 {
		return fmt.Errorf("%w: at least one action is required", ErrInvalidParameter)
	}
	seenCategory := false
	for _, a := range actions {
		if !validActionTypes[a.Type] {
			return fmt.Errorf("%w: unknown action type %q (expected set_category|add_tag|remove_tag|add_comment)", ErrInvalidParameter, a.Type)
		}
		switch a.Type {
		case "set_category":
			if seenCategory {
				return fmt.Errorf("%w: duplicate set_category action", ErrInvalidParameter)
			}
			seenCategory = true
			if a.CategorySlug == "" {
				return fmt.Errorf("%w: set_category action requires category_slug", ErrInvalidParameter)
			}
			if _, err := s.GetCategoryBySlug(ctx, a.CategorySlug); err != nil {
				return fmt.Errorf("%w: category slug %q not found", ErrInvalidParameter, a.CategorySlug)
			}
		case "add_tag", "remove_tag":
			if a.TagSlug == "" {
				return fmt.Errorf("%w: %s action requires tag_slug", ErrInvalidParameter, a.Type)
			}
			if !tagSlugPattern.MatchString(a.TagSlug) {
				return fmt.Errorf("%w: tag_slug %q must match ^[a-z0-9][a-z0-9\\-:]*[a-z0-9]$", ErrInvalidParameter, a.TagSlug)
			}
		case "add_comment":
			if a.Content == "" {
				return fmt.Errorf("%w: add_comment action requires content", ErrInvalidParameter)
			}
		}
	}
	return nil
}

// categoryActionSlug returns the category_slug from the first set_category
// action, or empty string if none. Used by the retroactive-apply and response-
// population paths.
func categoryActionSlug(actions []RuleAction) string {
	for _, a := range actions {
		if a.Type == "set_category" {
			return a.CategorySlug
		}
	}
	return ""
}

// validTriggers lists accepted values for transaction_rules.trigger.
// "on_update" is retained as a back-compat alias for "on_change" — inputs
// are normalized to "on_change" before being persisted, but rows previously
// stored as "on_update" continue to function (both sync and retroactive
// paths accept either value).
var validTriggers = map[string]bool{
	"on_create": true,
	"on_change": true,
	"on_update": true, // alias for on_change
	"always":    true,
}

// DefaultRulePriority is the priority assigned when neither priority nor stage
// is supplied. It corresponds to the "standard" pipeline stage.
const DefaultRulePriority = 10

// stagePriorities maps semantic pipeline-stage names to integer priorities.
// Values mirror the admin UI presets and are considered canonical.
var stagePriorities = map[string]int{
	"baseline":   0,
	"standard":   10,
	"refinement": 50,
	"override":   100,
}

// ResolveRulePriority returns the effective integer priority for a rule,
// accepting either an explicit priority, a semantic stage name, or both.
//
// Precedence (matches REST/MCP semantics):
//   - priority non-nil                    -> use priority as-is
//   - stage set and priority nil          -> map stage name to int
//   - both nil/empty                      -> DefaultRulePriority ("standard")
//   - stage set but unrecognized          -> validation error
//
// The stage string is case-insensitive and trimmed. Passing both is
// intentionally allowed — agents may echo the stage back alongside a raw
// priority for observability.
func ResolveRulePriority(stage string, priority *int) (int, error) {
	if priority != nil {
		return *priority, nil
	}
	stage = strings.ToLower(strings.TrimSpace(stage))
	if stage == "" {
		return DefaultRulePriority, nil
	}
	p, ok := stagePriorities[stage]
	if !ok {
		return 0, fmt.Errorf("%w: invalid stage %q (expected baseline|standard|refinement|override)", ErrInvalidParameter, stage)
	}
	return p, nil
}

// normalizeTrigger returns the canonical trigger string, defaulting to
// "on_create" on empty input. "on_update" is accepted and rewritten to
// "on_change". Returns an error for unknown values.
func normalizeTrigger(trigger string) (string, error) {
	if trigger == "" {
		return "on_create", nil
	}
	if trigger == "on_update" {
		return "on_change", nil
	}
	if !validTriggers[trigger] {
		return "", fmt.Errorf("%w: invalid trigger %q (expected on_create|on_change|always)", ErrInvalidParameter, trigger)
	}
	return trigger, nil
}

// --- Service methods ---

// ruleSelectQuery is the base SELECT for transaction rules. Category info is
// derived from actions[{type:"set_category"}] at response time via
// categorySlug lookup (no denormalized category_id column).
const ruleSelectQuery = `SELECT tr.id, tr.short_id, tr.name, tr.conditions, tr.actions, tr.trigger,
	tr.priority, tr.enabled, tr.expires_at, tr.created_by_type, tr.created_by_id, tr.created_by_name,
	tr.hit_count, tr.last_hit_at, tr.created_at, tr.updated_at
	FROM transaction_rules tr`

// CreateTransactionRule creates a new transaction rule.
//
// Category info is derived from actions[{type:"set_category"}] at response
// time. conditions may be a zero-value Condition{} (= match all; stored as
// NULL). trigger defaults to "on_create" and must be one of
// on_create|on_change|always ("on_update" accepted as legacy alias).
func (s *Service) CreateTransactionRule(ctx context.Context, params CreateTransactionRuleParams) (*TransactionRuleResponse, error) {
	// Validate conditions (zero-value => match-all)
	if err := ValidateCondition(params.Conditions); err != nil {
		return nil, err
	}

	// Resolve actions: Actions takes precedence, CategorySlug is sugar
	actions := params.Actions
	if len(actions) > 0 {
		if err := s.ValidateActions(ctx, actions); err != nil {
			return nil, err
		}
	} else if params.CategorySlug != "" {
		actions = []RuleAction{{Type: "set_category", CategorySlug: params.CategorySlug}}
		if err := s.ValidateActions(ctx, actions); err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("%w: either actions or category_slug is required", ErrInvalidParameter)
	}

	trigger, err := normalizeTrigger(params.Trigger)
	if err != nil {
		return nil, err
	}

	// Marshal conditions; a zero-value Condition is stored as NULL (match-all).
	var conditionsJSON []byte
	if !conditionIsEmpty(params.Conditions) {
		b, err := json.Marshal(params.Conditions)
		if err != nil {
			return nil, fmt.Errorf("marshal conditions: %w", err)
		}
		conditionsJSON = b
	}

	actionsJSON, err := json.Marshal(actions)
	if err != nil {
		return nil, fmt.Errorf("marshal actions: %w", err)
	}

	// Resolve priority from either the raw int or the semantic stage name.
	// Priority > 0 wins. Otherwise stage (if set) maps to its canonical int,
	// else DefaultRulePriority ("standard") applies.
	var priority int
	if params.Priority != 0 {
		priority = params.Priority
	} else {
		p, err := ResolveRulePriority(params.Stage, nil)
		if err != nil {
			return nil, err
		}
		priority = p
	}

	var expiresAt pgtype.Timestamptz
	if params.ExpiresIn != "" {
		dur, err := parseDuration(params.ExpiresIn)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid expires_in %q: %v", ErrInvalidParameter, params.ExpiresIn, err)
		}
		expiresAt = pgtype.Timestamptz{Time: time.Now().Add(dur), Valid: true}
	}

	createdByType := params.Actor.Type
	if createdByType == "" {
		createdByType = "user"
	}
	createdByName := params.Actor.Name
	if createdByName == "" {
		createdByName = "Breadbox"
	}
	var createdByID pgtype.Text
	if params.Actor.ID != "" {
		createdByID = pgconv.Text(params.Actor.ID)
	}

	query := `INSERT INTO transaction_rules (name, conditions, actions, trigger, priority, enabled, expires_at, created_by_type, created_by_id, created_by_name)
		VALUES ($1, $2, $3, $4, $5, TRUE, $6, $7, $8, $9)
		RETURNING id, short_id, name, conditions, actions, trigger, priority, enabled, expires_at, created_by_type, created_by_id, created_by_name, hit_count, last_hit_at, created_at, updated_at`

	// pgx/pgtype treats a nil []byte as NULL on insert — so match-all stores NULL.
	var condArg any = conditionsJSON
	if conditionsJSON == nil {
		condArg = nil
	}

	var row ruleRow
	scanDest := row.scanDest()
	err = s.Pool.QueryRow(ctx, query,
		params.Name,
		condArg,
		actionsJSON,
		trigger,
		priority,
		expiresAt,
		createdByType,
		createdByID,
		createdByName,
	).Scan(scanDest...)
	if err != nil {
		return nil, fmt.Errorf("insert transaction rule: %w", err)
	}

	return s.ruleRowToResponse(ctx, &row), nil
}

// GetTransactionRule returns a transaction rule by ID.
func (s *Service) GetTransactionRule(ctx context.Context, id string) (*TransactionRuleResponse, error) {
	ruleID, err := s.resolveRuleID(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}

	var row ruleRow
	err = s.Pool.QueryRow(ctx, ruleSelectQuery+" WHERE tr.id = $1", ruleID).Scan(row.scanDest()...)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get transaction rule: %w", err)
	}

	return s.ruleRowToResponse(ctx, &row), nil
}

// ruleOrderByClause maps a requested sort to the actual ORDER BY fragment.
// Unknown values fall back to created_at DESC so the admin UI can't inject
// arbitrary SQL via the query string.
func ruleOrderByClause(sortBy, sortDir string) string {
	dir := "DESC"
	if sortDir == "asc" {
		dir = "ASC"
	}
	switch sortBy {
	case "hit_count":
		return " ORDER BY tr.hit_count " + dir + ", tr.id DESC"
	case "last_hit_at":
		// NULLs sort last regardless of direction — rules that have never
		// fired should stay at the bottom.
		nullsOrder := "NULLS LAST"
		if dir == "ASC" {
			nullsOrder = "NULLS LAST"
		}
		return " ORDER BY tr.last_hit_at " + dir + " " + nullsOrder + ", tr.id DESC"
	case "created_at":
		return " ORDER BY tr.created_at " + dir + ", tr.id DESC"
	case "name":
		if sortDir == "" {
			dir = "ASC"
		}
		return " ORDER BY LOWER(tr.name) " + dir + ", tr.id DESC"
	default:
		// Default: pipeline stage ASC — rules execute in priority order,
		// so that's the most useful reading order for the list.
		if sortDir == "" {
			dir = "ASC"
		}
		return " ORDER BY tr.priority " + dir + ", tr.created_at DESC, tr.id DESC"
	}
}

// ListTransactionRules returns a filtered, paginated list of transaction rules.
//
// Category filtering matches against the set_category action inside the
// actions JSONB array.
func (s *Service) ListTransactionRules(ctx context.Context, params TransactionRuleListParams) (*TransactionRuleListResult, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	baseFrom := ` FROM transaction_rules tr`

	var whereClauses []string
	var args []any
	argN := 1

	if params.CategorySlug != nil && *params.CategorySlug != "" {
		// Match rules whose actions contain a set_category action with this slug.
		whereClauses = append(whereClauses, fmt.Sprintf(
			"tr.actions @> jsonb_build_array(jsonb_build_object('type','set_category','category_slug',$%d::text))",
			argN))
		args = append(args, *params.CategorySlug)
		argN++
	}

	if params.Enabled != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("tr.enabled = $%d", argN))
		args = append(args, *params.Enabled)
		argN++
	}

	if params.Search != nil && *params.Search != "" {
		mode := ""
		if params.SearchMode != nil {
			mode = *params.SearchMode
		}
		sc := BuildSearchClause(*params.Search, mode, RuleSearchColumns, RuleNullableColumns, argN)
		if sc.SQL != "" {
			// Strip leading " AND " since this builder uses a slice, not concatenation.
			whereClauses = append(whereClauses, sc.SQL[5:])
			args = append(args, sc.Args...)
			argN = sc.ArgN
		}
	}

	filterWhereSQL := ""
	if len(whereClauses) > 0 {
		filterWhereSQL = " WHERE " + strings.Join(whereClauses, " AND ")
	}

	// Count query uses only filter conditions (no cursor)
	countQuery := "SELECT COUNT(*)" + baseFrom + filterWhereSQL
	countArgs := make([]any, len(args))
	copy(countArgs, args)
	var total int64
	if err := s.Pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count rules: %w", err)
	}

	// Main query uses the shared base SELECT (no category JOIN; category info is
	// derived from actions at response time).
	selectCols := `SELECT tr.id, tr.short_id, tr.name, tr.conditions, tr.actions, tr.trigger,
		tr.priority, tr.enabled, tr.expires_at, tr.created_by_type, tr.created_by_id, tr.created_by_name,
		tr.hit_count, tr.last_hit_at, tr.created_at, tr.updated_at `

	orderBy := ruleOrderByClause(params.SortBy, params.SortDir)
	whereSQL := filterWhereSQL

	// Offset-based pagination (admin UI)
	if params.Page > 0 {
		pageSize := params.PageSize
		if pageSize <= 0 {
			pageSize = 50
		}
		offset := (params.Page - 1) * pageSize
		limitClause := fmt.Sprintf(" LIMIT $%d OFFSET $%d", argN, argN+1)
		args = append(args, pageSize, offset)

		fullQuery := selectCols + baseFrom + whereSQL + orderBy + limitClause
		rows, err := s.Pool.Query(ctx, fullQuery, args...)
		if err != nil {
			return nil, fmt.Errorf("query rules: %w", err)
		}
		defer rows.Close()

		var scanned []ruleRow
		for rows.Next() {
			var row ruleRow
			if err := rows.Scan(row.scanDest()...); err != nil {
				return nil, fmt.Errorf("scan rule: %w", err)
			}
			scanned = append(scanned, row)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iterate rules: %w", err)
		}

		rules := s.convertRuleRows(ctx, scanned)

		totalPages := int(total) / pageSize
		if int(total)%pageSize != 0 {
			totalPages++
		}

		return &TransactionRuleListResult{
			Rules:      rules,
			Total:      total,
			Page:       params.Page,
			PageSize:   pageSize,
			TotalPages: totalPages,
			HasMore:    params.Page < totalPages,
		}, nil
	}

	// Cursor-based pagination (API/MCP)
	if params.Cursor != "" {
		cursorTime, cursorIDStr, err := decodeTimestampCursor(params.Cursor)
		if err != nil {
			return nil, ErrInvalidCursor
		}
		cursorUUID, err := parseUUID(cursorIDStr)
		if err != nil {
			return nil, ErrInvalidCursor
		}
		cursorClause := fmt.Sprintf("(tr.created_at, tr.id) < ($%d, $%d)", argN, argN+1)
		if whereSQL == "" {
			whereSQL = " WHERE " + cursorClause
		} else {
			whereSQL += " AND " + cursorClause
		}
		args = append(args, pgtype.Timestamptz{Time: cursorTime, Valid: true}, cursorUUID)
		argN += 2
	}

	limitClause := fmt.Sprintf(" LIMIT $%d", argN)
	args = append(args, limit+1)

	fullQuery := selectCols + baseFrom + whereSQL + orderBy + limitClause

	rows, err := s.Pool.Query(ctx, fullQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("query rules: %w", err)
	}
	defer rows.Close()

	var scanned []ruleRow
	for rows.Next() {
		var row ruleRow
		if err := rows.Scan(row.scanDest()...); err != nil {
			return nil, fmt.Errorf("scan rule: %w", err)
		}
		scanned = append(scanned, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rules: %w", err)
	}

	hasMore := len(scanned) > limit
	if hasMore {
		scanned = scanned[:limit]
	}

	rules := s.convertRuleRows(ctx, scanned)

	var nextCursor string
	if hasMore && len(scanned) > 0 {
		last := scanned[len(scanned)-1]
		nextCursor = encodeTimestampCursor(last.createdAt.Time.UTC(), formatUUID(last.id))
	}

	return &TransactionRuleListResult{
		Rules:      rules,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Total:      total,
	}, nil
}

// UpdateTransactionRule updates an existing transaction rule.
func (s *Service) UpdateTransactionRule(ctx context.Context, id string, params UpdateTransactionRuleParams) (*TransactionRuleResponse, error) {
	// Fetch existing rule (validates UUID and existence)
	existing, err := s.GetTransactionRule(ctx, id)
	if err != nil {
		return nil, err
	}

	// Build updated fields
	name := existing.Name
	if params.Name != nil {
		name = *params.Name
	}

	conditions := existing.Conditions
	if params.Conditions != nil {
		if err := ValidateCondition(*params.Conditions); err != nil {
			return nil, err
		}
		conditions = *params.Conditions
	}

	// Resolve actions
	actions := existing.Actions
	if params.Actions != nil {
		// Explicit actions replacement
		if err := s.ValidateActions(ctx, *params.Actions); err != nil {
			return nil, err
		}
		actions = *params.Actions
	} else if params.CategorySlug != nil {
		// Sugar: replace the set_category action, keep everything else.
		newActions := make([]RuleAction, 0, len(actions)+1)
		for _, a := range actions {
			if a.Type != "set_category" {
				newActions = append(newActions, a)
			}
		}
		newActions = append(newActions, RuleAction{Type: "set_category", CategorySlug: *params.CategorySlug})
		if err := s.ValidateActions(ctx, newActions); err != nil {
			return nil, err
		}
		actions = newActions
	}

	// existing.Trigger is sourced from a NOT NULL DEFAULT column, and
	// normalizeTrigger guarantees a non-empty value on update — no fallback
	// needed here.
	trigger := existing.Trigger
	if params.Trigger != nil {
		t, err := normalizeTrigger(*params.Trigger)
		if err != nil {
			return nil, err
		}
		trigger = t
	}

	priority := int32(existing.Priority)
	if params.Priority != nil {
		priority = int32(*params.Priority)
	} else if params.Stage != nil && *params.Stage != "" {
		p, err := ResolveRulePriority(*params.Stage, nil)
		if err != nil {
			return nil, err
		}
		priority = int32(p)
	}

	enabled := existing.Enabled
	if params.Enabled != nil {
		enabled = *params.Enabled
	}

	var expiresAt pgtype.Timestamptz
	if params.ExpiresAt != nil {
		if *params.ExpiresAt == "" {
			// Clear expiration
			expiresAt = pgtype.Timestamptz{Valid: false}
		} else {
			t, err := time.Parse(time.RFC3339, *params.ExpiresAt)
			if err != nil {
				return nil, fmt.Errorf("%w: invalid expires_at format, use RFC3339", ErrInvalidParameter)
			}
			expiresAt = pgtype.Timestamptz{Time: t, Valid: true}
		}
	} else if existing.ExpiresAt != nil {
		t, _ := time.Parse(time.RFC3339, *existing.ExpiresAt)
		expiresAt = pgtype.Timestamptz{Time: t, Valid: true}
	}

	var conditionsArg any
	if !conditionIsEmpty(conditions) {
		b, err := json.Marshal(conditions)
		if err != nil {
			return nil, fmt.Errorf("marshal conditions: %w", err)
		}
		conditionsArg = b
	}

	actionsJSON, err := json.Marshal(actions)
	if err != nil {
		return nil, fmt.Errorf("marshal actions: %w", err)
	}

	ruleID, _ := parseUUID(id) // already validated by GetTransactionRule above

	query := `UPDATE transaction_rules
		SET name = $2, conditions = $3, actions = $4, trigger = $5, priority = $6, enabled = $7, expires_at = $8, updated_at = NOW()
		WHERE id = $1
		RETURNING id, short_id, name, conditions, actions, trigger, priority, enabled, expires_at, created_by_type, created_by_id, created_by_name, hit_count, last_hit_at, created_at, updated_at`

	var row ruleRow
	err = s.Pool.QueryRow(ctx, query,
		ruleID, name, conditionsArg, actionsJSON, trigger, priority, enabled, expiresAt,
	).Scan(row.scanDest()...)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("update transaction rule: %w", err)
	}

	return s.ruleRowToResponse(ctx, &row), nil
}

// DeleteTransactionRule deletes a transaction rule by ID.
func (s *Service) DeleteTransactionRule(ctx context.Context, id string) error {
	ruleID, err := s.resolveRuleID(ctx, id)
	if err != nil {
		return ErrNotFound
	}

	result, err := s.Pool.Exec(ctx, "DELETE FROM transaction_rules WHERE id = $1", ruleID)
	if err != nil {
		return fmt.Errorf("delete transaction rule: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListActiveRulesForSync returns all enabled, non-expired rules ordered by
// pipeline stage (priority ASC, created_at ASC) — matches the sync-path
// loadRules ordering. Lower priority runs first (baseline), higher priority
// runs last and wins set_category under last-writer-wins merge.
func (s *Service) ListActiveRulesForSync(ctx context.Context) ([]TransactionRuleResponse, error) {
	query := ruleSelectQuery + ` WHERE tr.enabled = TRUE
		AND (tr.expires_at IS NULL OR tr.expires_at > NOW())
		ORDER BY tr.priority ASC, tr.created_at ASC`

	rows, err := s.Pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list active rules: %w", err)
	}
	defer rows.Close()

	var scanned []ruleRow
	for rows.Next() {
		var row ruleRow
		if err := rows.Scan(row.scanDest()...); err != nil {
			return nil, fmt.Errorf("scan active rule: %w", err)
		}
		scanned = append(scanned, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active rules: %w", err)
	}

	return s.convertRuleRows(ctx, scanned), nil
}

// transactionContextQuery is the base query for loading transactions with full
// context (JOIN through accounts → bank_connections → users) for rule
// evaluation. Aggregates tag slugs from transaction_tags so tag-based
// conditions work during retroactive apply. GROUP BY t.id is required because
// of the LEFT JOIN onto the tag pivot.
//
// NOTE: `category_override=TRUE` rows are intentionally *not* filtered here.
// Hit counts must reflect every condition match (sync-time parity, Q12), and
// non-category actions (add_tag, remove_tag) legitimately fire on overridden
// rows. The `set_category` UPDATE below enforces its own
// `category_override = FALSE` guard so overridden rows keep their user-pinned
// category.
const transactionContextQuery = `SELECT t.id, t.name, COALESCE(t.merchant_name, ''), t.amount,
	COALESCE(t.category_primary, ''), COALESCE(t.category_detailed, ''),
	t.pending, bc.provider, t.account_id::text, COALESCE(u.id::text, ''), COALESCE(u.name, ''),
	COALESCE(array_agg(DISTINCT tag.slug) FILTER (WHERE tag.slug IS NOT NULL), ARRAY[]::text[])
	FROM transactions t
	JOIN accounts a ON t.account_id = a.id
	JOIN bank_connections bc ON a.connection_id = bc.id
	LEFT JOIN users u ON bc.user_id = u.id
	LEFT JOIN transaction_tags tt ON tt.transaction_id = t.id
	LEFT JOIN tags tag ON tag.id = tt.tag_id
	WHERE t.deleted_at IS NULL
	AND (a.is_dependent_linked = FALSE OR NOT EXISTS (SELECT 1 FROM transaction_matches tm WHERE tm.dependent_transaction_id = t.id))`

// transactionContextGroupBy is the GROUP BY clause matching transactionContextQuery.
const transactionContextGroupBy = ` GROUP BY t.id, t.name, t.merchant_name, t.amount, t.category_primary, t.category_detailed, t.pending, bc.provider, t.account_id, u.id, u.name`

// transactionContextRow holds a scanned transaction row for rule evaluation.
type transactionContextRow struct {
	id   pgtype.UUID
	tctx TransactionContext
}

func scanTransactionContextRow(dest []any) *transactionContextRow {
	r := &transactionContextRow{}
	dest[0] = &r.id
	dest[1] = &r.tctx.Name
	dest[2] = &r.tctx.MerchantName
	dest[3] = &r.tctx.Amount
	dest[4] = &r.tctx.CategoryPrimary
	dest[5] = &r.tctx.CategoryDetailed
	dest[6] = &r.tctx.Pending
	dest[7] = &r.tctx.Provider
	dest[8] = &r.tctx.AccountID
	dest[9] = &r.tctx.UserID
	dest[10] = &r.tctx.UserName
	dest[11] = &r.tctx.Tags
	return r
}

// ApplyRuleRetroactively applies a single rule to all existing non-deleted
// transactions matching its condition. Materializes set_category (SQL UPDATE,
// skipped on category_override=TRUE rows), add_tag (upsert transaction_tags),
// and remove_tag (delete transaction_tags). add_comment stays sync-only by
// design — retroactive apply is bulk historical data, not narration.
//
// Returns the number of transactions that matched the condition (hit count —
// not the number of rows physically modified). This matches sync-time
// hit-count semantics so the counter tells you "how often does this rule
// match," not "how often did it produce a DB write."
func (s *Service) ApplyRuleRetroactively(ctx context.Context, ruleID string) (int64, error) {
	rule, err := s.GetTransactionRule(ctx, ruleID)
	if err != nil {
		return 0, err
	}
	if !rule.Enabled {
		return 0, fmt.Errorf("%w: rule is disabled", ErrInvalidParameter)
	}
	if rule.ExpiresAt != nil {
		t, _ := time.Parse(time.RFC3339, *rule.ExpiresAt)
		if !t.IsZero() && t.Before(time.Now()) {
			return 0, fmt.Errorf("%w: rule is expired", ErrInvalidParameter)
		}
	}
	if len(rule.Actions) == 0 {
		return 0, fmt.Errorf("%w: rule has no actions", ErrInvalidParameter)
	}

	compiled, err := CompileCondition(rule.Conditions)
	if err != nil {
		return 0, fmt.Errorf("compile rule conditions: %w", err)
	}

	// Extract action intents once up-front.
	var categorySetCatID pgtype.UUID
	var categorySetSlug string
	var tagAdds []string
	var tagRemoves []string
	for _, a := range rule.Actions {
		switch a.Type {
		case "set_category":
			catID, err := s.categorySlugToUUID(ctx, a.CategorySlug)
			if err != nil || !catID.Valid {
				// Skip unresolvable slug; a warning log lives in the sync path.
				continue
			}
			categorySetCatID = catID
			categorySetSlug = a.CategorySlug
		case "add_tag":
			tagAdds = append(tagAdds, a.TagSlug)
		case "remove_tag":
			tagRemoves = append(tagRemoves, a.TagSlug)
		case "add_comment":
			// sync-only; skip retroactively.
		}
	}
	hasWriteAction := categorySetCatID.Valid || len(tagAdds) > 0 || len(tagRemoves) > 0
	if !hasWriteAction {
		return 0, fmt.Errorf("%w: rule has no applicable actions", ErrInvalidParameter)
	}

	ruleUUID, _ := parseUUID(ruleID)
	ruleShortID, ruleName := s.ruleIdentityForAudit(ctx, ruleUUID)

	var totalMatched int64
	var lastID pgtype.UUID

	for {
		query := transactionContextQuery
		var args []any
		argN := 1
		if lastID.Valid {
			query += fmt.Sprintf(" AND t.id > $%d", argN)
			args = append(args, lastID)
			argN++
		}
		query += transactionContextGroupBy
		query += " ORDER BY t.id ASC"
		query += fmt.Sprintf(" LIMIT $%d", argN)
		args = append(args, 1000)

		rows, err := s.Pool.Query(ctx, query, args...)
		if err != nil {
			return totalMatched, fmt.Errorf("query transactions: %w", err)
		}
		var matchIDs []pgtype.UUID
		rowCount := 0
		for rows.Next() {
			rowCount++
			dest := make([]any, 12)
			r := scanTransactionContextRow(dest)
			if err := rows.Scan(dest...); err != nil {
				rows.Close()
				return totalMatched, fmt.Errorf("scan transaction: %w", err)
			}
			lastID = r.id
			if EvaluateCondition(compiled, r.tctx) {
				matchIDs = append(matchIDs, r.id)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return totalMatched, fmt.Errorf("iterate transactions: %w", err)
		}

		totalMatched += int64(len(matchIDs))
		if len(matchIDs) == 0 {
			if rowCount < 1000 {
				break
			}
			continue
		}

		// Persist in a per-batch tx so set_category + tag writes + annotations
		// are atomic against failure.
		tx, err := s.Pool.Begin(ctx)
		if err != nil {
			return totalMatched, fmt.Errorf("begin retroactive apply tx: %w", err)
		}

		// set_category: bulk UPDATE for matching non-overridden rows.
		if categorySetCatID.Valid {
			_, err := tx.Exec(ctx,
				`UPDATE transactions SET category_id = $1, updated_at = NOW()
				WHERE id = ANY($2) AND category_override = FALSE AND deleted_at IS NULL`,
				categorySetCatID, matchIDs)
			if err != nil {
				tx.Rollback(ctx)
				return totalMatched, fmt.Errorf("update transactions: %w", err)
			}
			// Annotation per touched row.
			rule := ruleapply.Rule{ID: ruleUUID, ShortID: ruleShortID, Name: ruleName}
			for _, txnID := range matchIDs {
				if err := ruleapply.WriteCategorySet(ctx, tx, txnID, rule, categorySetSlug, ruleapply.AppliedByRetroactive); err != nil {
					tx.Rollback(ctx)
					return totalMatched, fmt.Errorf("annotate category_set: %w", err)
				}
			}
		}

		// add_tag: materialize per tag per matching txn.
		for _, txnID := range matchIDs {
			for _, slug := range tagAdds {
				if _, err := s.materializeRuleTagAdd(ctx, tx, txnID, slug, ruleUUID, ruleShortID, ruleName); err != nil {
					tx.Rollback(ctx)
					return totalMatched, err
				}
			}
			for _, slug := range tagRemoves {
				if _, err := s.materializeRuleTagRemove(ctx, tx, txnID, slug, ruleUUID, ruleShortID, ruleName); err != nil {
					tx.Rollback(ctx)
					return totalMatched, err
				}
			}
		}

		// rule_applied audit trail — one per action intent per matched txn.
		appliedRule := ruleapply.Rule{ID: ruleUUID, ShortID: ruleShortID, Name: ruleName}
		for _, action := range rule.Actions {
			field, value := actionAuditFields(action)
			if field == "" || field == "comment" {
				continue
			}
			for _, txnID := range matchIDs {
				if err := ruleapply.WriteRuleApplied(ctx, tx, txnID, appliedRule, field, value, ruleapply.AppliedByRetroactive); err != nil {
					tx.Rollback(ctx)
					return totalMatched, fmt.Errorf("annotate rule_applied: %w", err)
				}
			}
		}

		if err := tx.Commit(ctx); err != nil {
			return totalMatched, fmt.Errorf("commit retroactive apply batch: %w", err)
		}

		if rowCount < 1000 {
			break
		}
	}

	// Update hit count on the rule row (sync-parity: count matches, not
	// UPDATE-touched rows).
	if totalMatched > 0 {
		_, _ = s.Pool.Exec(ctx, "UPDATE transaction_rules SET hit_count = hit_count + $2, last_hit_at = NOW() WHERE id = $1", ruleUUID, totalMatched)
	}

	return totalMatched, nil
}

// ruleIdentityForAudit looks up the (short_id, name) pair for a rule UUID so
// annotation actor fields match the rule's public identity. Returns empty
// strings if the rule isn't found (caller writes without a back-reference).
func (s *Service) ruleIdentityForAudit(ctx context.Context, ruleID pgtype.UUID) (shortID, name string) {
	_ = s.Pool.QueryRow(ctx, `SELECT short_id, name FROM transaction_rules WHERE id = $1`, ruleID).Scan(&shortID, &name)
	return shortID, name
}

// actionAuditFields returns (action_field, action_value) for the audit trail,
// carrying the action's semantic intent onto the annotation payload.
func actionAuditFields(a RuleAction) (string, string) {
	switch a.Type {
	case "set_category":
		return "category", a.CategorySlug
	case "add_tag":
		return "tag", a.TagSlug
	case "add_comment":
		return "comment", a.Content
	}
	return "", ""
}

// ApplyAllRulesRetroactively applies all active rules to existing transactions
// in pipeline-stage order (priority ASC, created_at ASC). Materializes
// set_category (last-writer-wins), add_tag, and remove_tag. add_comment stays
// sync-only.
//
// Returns a map of rule_id → match_count. Hit counts include every condition
// match, not just actions that caused a DB write (sync-time parity).
func (s *Service) ApplyAllRulesRetroactively(ctx context.Context) (map[string]int64, error) {
	rules, err := s.ListActiveRulesForSync(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active rules: %w", err)
	}
	if len(rules) == 0 {
		return map[string]int64{}, nil
	}

	type compiledRule struct {
		id       string
		uuid     pgtype.UUID
		shortID  string
		name     string
		actions  []RuleAction
		compiled *CompiledCondition
	}
	var compiled []compiledRule
	for _, r := range rules {
		if len(r.Actions) == 0 {
			continue
		}
		cc, err := CompileCondition(r.Conditions)
		if err != nil {
			continue
		}
		uuid, _ := parseUUID(r.ID)
		compiled = append(compiled, compiledRule{
			id:       r.ID,
			uuid:     uuid,
			shortID:  r.ShortID,
			name:     r.Name,
			actions:  r.Actions,
			compiled: cc,
		})
	}
	if len(compiled) == 0 {
		return map[string]int64{}, nil
	}

	hitCounts := make(map[string]int64)
	var lastID pgtype.UUID

	for {
		query := transactionContextQuery
		var args []any
		argN := 1
		if lastID.Valid {
			query += fmt.Sprintf(" AND t.id > $%d", argN)
			args = append(args, lastID)
			argN++
		}
		query += transactionContextGroupBy
		query += " ORDER BY t.id ASC"
		query += fmt.Sprintf(" LIMIT $%d", argN)
		args = append(args, 1000)

		rows, err := s.Pool.Query(ctx, query, args...)
		if err != nil {
			return hitCounts, fmt.Errorf("query transactions: %w", err)
		}

		// Per transaction: fold rules to determine net action intents, mirroring
		// the sync resolver. Category: last-writer-wins. Tags: net-diff.
		type txnIntent struct {
			txnID          pgtype.UUID
			catID          pgtype.UUID
			catSlug        string
			catRule        *compiledRule
			tagAdds        map[string]*compiledRule // slug → first rule that added it
			tagRemoves     map[string]*compiledRule
			matchingRules  []*compiledRule // every rule whose condition matched (for rule_applied)
		}
		var intents []txnIntent
		rowCount := 0
		for rows.Next() {
			rowCount++
			dest := make([]any, 12)
			r := scanTransactionContextRow(dest)
			if err := rows.Scan(dest...); err != nil {
				rows.Close()
				return hitCounts, fmt.Errorf("scan transaction: %w", err)
			}
			lastID = r.id
			tctx := r.tctx
			if len(tctx.Tags) > 0 {
				cp := make([]string, len(tctx.Tags))
				copy(cp, tctx.Tags)
				tctx.Tags = cp
			}
			intent := txnIntent{
				txnID:      r.id,
				tagAdds:    make(map[string]*compiledRule),
				tagRemoves: make(map[string]*compiledRule),
			}
			for i := range compiled {
				cr := &compiled[i]
				if !EvaluateCondition(cr.compiled, tctx) {
					continue
				}
				hitCounts[cr.id]++
				intent.matchingRules = append(intent.matchingRules, cr)
				for _, a := range cr.actions {
					switch a.Type {
					case "set_category":
						if catID, err := s.categorySlugToUUID(ctx, a.CategorySlug); err == nil && catID.Valid {
							intent.catID = catID
							intent.catSlug = a.CategorySlug
							intent.catRule = cr
							tctx.Category = a.CategorySlug
						}
					case "add_tag":
						if a.TagSlug == "" {
							continue
						}
						// If a prior-stage remove queued this slug, the later
						// add cancels the remove.
						delete(intent.tagRemoves, a.TagSlug)
						if _, exists := intent.tagAdds[a.TagSlug]; !exists {
							intent.tagAdds[a.TagSlug] = cr
						}
						if !sliceutil.ContainsFold(tctx.Tags, a.TagSlug) {
							tctx.Tags = append(tctx.Tags, a.TagSlug)
						}
					case "remove_tag":
						if a.TagSlug == "" {
							continue
						}
						// If a prior-stage add queued this slug, cancel it.
						if _, was := intent.tagAdds[a.TagSlug]; was {
							delete(intent.tagAdds, a.TagSlug)
						} else if sliceutil.ContainsFold(tctx.Tags, a.TagSlug) {
							if _, exists := intent.tagRemoves[a.TagSlug]; !exists {
								intent.tagRemoves[a.TagSlug] = cr
							}
						}
						tctx.Tags = sliceutil.DropFold(tctx.Tags, a.TagSlug)
					}
				}
			}
			if intent.catRule != nil || len(intent.tagAdds) > 0 || len(intent.tagRemoves) > 0 {
				intents = append(intents, intent)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return hitCounts, fmt.Errorf("iterate transactions: %w", err)
		}

		if len(intents) == 0 {
			if rowCount < 1000 {
				break
			}
			continue
		}

		tx, err := s.Pool.Begin(ctx)
		if err != nil {
			return hitCounts, fmt.Errorf("begin apply-all tx: %w", err)
		}
		aborted := false

		// Group category updates by (catID) for batched UPDATEs.
		catBatches := make(map[pgtype.UUID][]pgtype.UUID)
		for _, it := range intents {
			if it.catRule != nil {
				catBatches[it.catID] = append(catBatches[it.catID], it.txnID)
			}
		}
		for catID, txnIDs := range catBatches {
			if _, err := tx.Exec(ctx,
				`UPDATE transactions SET category_id = $1, updated_at = NOW()
				WHERE id = ANY($2) AND category_override = FALSE AND deleted_at IS NULL`,
				catID, txnIDs); err != nil {
				tx.Rollback(ctx)
				return hitCounts, fmt.Errorf("update transactions: %w", err)
			}
		}

		for _, it := range intents {
			// category_set annotation (one per touched row, owned by the winning rule).
			if it.catRule != nil {
				if err := ruleapply.WriteCategorySet(ctx, tx, it.txnID,
					ruleapply.Rule{ID: it.catRule.uuid, ShortID: it.catRule.shortID, Name: it.catRule.name},
					it.catSlug, ruleapply.AppliedByRetroactive,
				); err != nil {
					aborted = true
					break
				}
			}
			for slug, cr := range it.tagAdds {
				if _, err := s.materializeRuleTagAdd(ctx, tx, it.txnID, slug, cr.uuid, cr.shortID, cr.name); err != nil {
					aborted = true
					break
				}
			}
			if aborted {
				break
			}
			for slug, cr := range it.tagRemoves {
				if _, err := s.materializeRuleTagRemove(ctx, tx, it.txnID, slug, cr.uuid, cr.shortID, cr.name); err != nil {
					aborted = true
					break
				}
			}
			if aborted {
				break
			}

			// rule_applied audit: one per matching rule per txn.
			//
			// Unlike ApplyRuleRetroactively, this path emits one annotation per
			// (matching rule, txn) without per-action specialization — callers
			// get a rule-level audit, so action_field / action_value are left
			// empty. Shape still matches the canonical 5-key payload.
			for _, cr := range it.matchingRules {
				if err := ruleapply.WriteRuleApplied(ctx, tx, it.txnID,
					ruleapply.Rule{ID: cr.uuid, ShortID: cr.shortID, Name: cr.name},
					"", "", ruleapply.AppliedByRetroactive,
				); err != nil {
					aborted = true
					break
				}
			}
			if aborted {
				break
			}
		}

		if aborted {
			tx.Rollback(ctx)
			return hitCounts, fmt.Errorf("apply-all retroactive batch aborted")
		}
		if err := tx.Commit(ctx); err != nil {
			return hitCounts, fmt.Errorf("commit apply-all batch: %w", err)
		}

		if rowCount < 1000 {
			break
		}
	}

	// Update hit counts for all matched rules.
	for ruleID, count := range hitCounts {
		id, err := parseUUID(ruleID)
		if err != nil {
			continue
		}
		_, _ = s.Pool.Exec(ctx, "UPDATE transaction_rules SET hit_count = hit_count + $2, last_hit_at = NOW() WHERE id = $1", id, count)
	}

	return hitCounts, nil
}

// RulePreviewMatch contains a sample transaction that matched a rule preview.
type RulePreviewMatch struct {
	TransactionID      string  `json:"transaction_id"`
	Name               string  `json:"name"`
	Amount             float64 `json:"amount"`
	Date               string  `json:"date"`
	CategoryPrimaryRaw string  `json:"category_primary_raw"`
	CurrentCategorySlug string `json:"current_category_slug,omitempty"`
}

// RulePreviewResult contains the results of a rule preview/dry-run.
type RulePreviewResult struct {
	MatchCount   int64              `json:"match_count"`
	TotalScanned int64              `json:"total_scanned"`
	SampleMatches []RulePreviewMatch `json:"sample_matches"`
}

// PreviewRuleForDetail evaluates conditions and excludes transactions already applied by this rule.
func (s *Service) PreviewRuleForDetail(ctx context.Context, ruleID string, conditions Condition, sampleSize int) (*RulePreviewResult, error) {
	ruleUUID, err := s.resolveRuleID(ctx, ruleID)
	if err != nil {
		return s.PreviewRule(ctx, conditions, sampleSize)
	}
	return s.previewRuleInternal(ctx, &ruleUUID, conditions, sampleSize)
}

// PreviewRule evaluates conditions against existing transactions without modifying anything.
// Returns match count, total scanned, and sample matches.
func (s *Service) PreviewRule(ctx context.Context, conditions Condition, sampleSize int) (*RulePreviewResult, error) {
	return s.previewRuleInternal(ctx, nil, conditions, sampleSize)
}

func (s *Service) previewRuleInternal(ctx context.Context, excludeRuleID *pgtype.UUID, conditions Condition, sampleSize int) (*RulePreviewResult, error) {
	if err := ValidateCondition(conditions); err != nil {
		return nil, err
	}

	compiled, err := CompileCondition(conditions)
	if err != nil {
		return nil, fmt.Errorf("compile conditions: %w", err)
	}

	if sampleSize <= 0 {
		sampleSize = 10
	}
	if sampleSize > 50 {
		sampleSize = 50
	}

	result := &RulePreviewResult{}
	var lastID pgtype.UUID

	// Extended query to also get date and current category slug.
	// Must match the same filters as transactionContextQuery (used by ApplyRuleRetroactively):
	// - category_override = FALSE (rules don't overwrite manual overrides)
	// - exclude matched dependent transactions (dedup'd via account links)
	baseQuery := `SELECT t.id, t.name, COALESCE(t.merchant_name, ''), t.amount,
		COALESCE(t.category_primary, ''), COALESCE(t.category_detailed, ''),
		t.pending, bc.provider, t.account_id::text, COALESCE(u.id::text, ''), COALESCE(u.name, ''),
		t.date, COALESCE(c.slug, '')
		FROM transactions t
		JOIN accounts a ON t.account_id = a.id
		JOIN bank_connections bc ON a.connection_id = bc.id
		LEFT JOIN users u ON bc.user_id = u.id
		LEFT JOIN categories c ON t.category_id = c.id
		WHERE t.deleted_at IS NULL AND t.category_override = FALSE
		AND (a.is_dependent_linked = FALSE OR NOT EXISTS (SELECT 1 FROM transaction_matches tm WHERE tm.dependent_transaction_id = t.id))`

	// When previewing for a specific rule's detail page, exclude transactions
	// already applied by this rule (audit trail lives in annotations with
	// kind='rule_applied').
	var baseArgs []any
	baseArgN := 1
	if excludeRuleID != nil {
		baseQuery += fmt.Sprintf(" AND NOT EXISTS (SELECT 1 FROM annotations ann WHERE ann.transaction_id = t.id AND ann.kind = 'rule_applied' AND ann.rule_id = $%d)", baseArgN)
		baseArgs = append(baseArgs, *excludeRuleID)
		baseArgN++
	}

	for {
		query := baseQuery
		args := make([]any, len(baseArgs))
		copy(args, baseArgs)
		argN := baseArgN

		if lastID.Valid {
			query += fmt.Sprintf(" AND t.id > $%d", argN)
			args = append(args, lastID)
			argN++
		}
		query += " ORDER BY t.id ASC"
		query += fmt.Sprintf(" LIMIT $%d", argN)
		args = append(args, 1000)

		rows, err := s.Pool.Query(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("query transactions: %w", err)
		}

		rowCount := 0
		for rows.Next() {
			rowCount++
			var (
				id           pgtype.UUID
				tctx         TransactionContext
				date         pgtype.Date
				catSlug      string
			)
			if err := rows.Scan(&id, &tctx.Name, &tctx.MerchantName, &tctx.Amount,
				&tctx.CategoryPrimary, &tctx.CategoryDetailed,
				&tctx.Pending, &tctx.Provider, &tctx.AccountID, &tctx.UserID, &tctx.UserName,
				&date, &catSlug); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan transaction: %w", err)
			}
			lastID = id
			result.TotalScanned++

			if EvaluateCondition(compiled, tctx) {
				result.MatchCount++
				if len(result.SampleMatches) < sampleSize {
					match := RulePreviewMatch{
						TransactionID:      formatUUID(id),
						Name:               tctx.Name,
						Amount:             tctx.Amount,
						CategoryPrimaryRaw: tctx.CategoryPrimary,
					}
					if date.Valid {
						match.Date = date.Time.Format("2006-01-02")
					}
					if catSlug != "" {
						match.CurrentCategorySlug = catSlug
					}
					result.SampleMatches = append(result.SampleMatches, match)
				}
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iterate transactions: %w", err)
		}

		if rowCount < 1000 {
			break
		}
	}

	if result.SampleMatches == nil {
		result.SampleMatches = []RulePreviewMatch{}
	}

	return result, nil
}

// BatchIncrementHitCounts updates hit counts for rules that matched.
func (s *Service) BatchIncrementHitCounts(ctx context.Context, hits map[string]int) error {
	for ruleID, count := range hits {
		id, err := parseUUID(ruleID)
		if err != nil {
			continue
		}
		_, err = s.Pool.Exec(ctx, "UPDATE transaction_rules SET hit_count = hit_count + $2, last_hit_at = NOW() WHERE id = $1", id, count)
		if err != nil {
			return fmt.Errorf("increment hit count for %s: %w", ruleID, err)
		}
	}
	return nil
}

// --- Helpers ---

// ruleRow holds the scanned columns for a transaction rule row. Category info
// is populated at response time by looking up the set_category action's slug
// (no JOINed category columns).
type ruleRow struct {
	id             pgtype.UUID
	shortID        string
	name           string
	conditions     []byte // NULL -> nil -> match-all
	actions        []byte
	trigger        string
	priority       int32
	enabled        bool
	expiresAt      pgtype.Timestamptz
	createdByType  string
	createdByID    pgtype.Text
	createdByName  string
	hitCount       int32
	lastHitAt      pgtype.Timestamptz
	createdAt      pgtype.Timestamptz
	updatedAt      pgtype.Timestamptz
}

// scanDest returns a slice of pointers for use with rows.Scan.
func (r *ruleRow) scanDest() []any {
	return []any{
		&r.id, &r.shortID, &r.name, &r.conditions, &r.actions, &r.trigger,
		&r.priority, &r.enabled, &r.expiresAt, &r.createdByType, &r.createdByID, &r.createdByName,
		&r.hitCount, &r.lastHitAt, &r.createdAt, &r.updatedAt,
	}
}

// toResponseBase builds the shared fields of a TransactionRuleResponse from a
// scanned row, without populating any of the derived category fields. Callers
// (ruleRowToResponse / convertRuleRows) fill those in via the shared slug cache.
func (r *ruleRow) toResponseBase() TransactionRuleResponse {
	var cond Condition
	if len(r.conditions) > 0 {
		_ = json.Unmarshal(r.conditions, &cond)
	}

	var actions []RuleAction
	if len(r.actions) > 0 {
		_ = json.Unmarshal(r.actions, &actions)
	}
	if actions == nil {
		actions = []RuleAction{}
	}

	return TransactionRuleResponse{
		ID:            formatUUID(r.id),
		ShortID:       r.shortID,
		Name:          r.name,
		Conditions:    cond,
		Actions:       actions,
		Trigger:       r.trigger,
		Priority:      int(r.priority),
		Enabled:       r.enabled,
		ExpiresAt:     timestampStr(r.expiresAt),
		CreatedByType: r.createdByType,
		CreatedByID:   textPtr(r.createdByID),
		CreatedByName: r.createdByName,
		HitCount:      int(r.hitCount),
		LastHitAt:     timestampStr(r.lastHitAt),
		CreatedAt:     pgconv.TimestampStr(r.createdAt),
		UpdatedAt:     pgconv.TimestampStr(r.updatedAt),
	}
}

// ruleRowToResponse turns a scanned ruleRow into a response, populating the
// derived category_* fields from the first set_category action (if any) via a
// category lookup.
func (s *Service) ruleRowToResponse(ctx context.Context, row *ruleRow) *TransactionRuleResponse {
	resp := row.toResponseBase()
	if slug := categoryActionSlug(resp.Actions); slug != "" {
		if cat, err := s.GetCategoryBySlug(ctx, slug); err == nil {
			id := cat.ID
			slugCopy := cat.Slug
			nameCopy := cat.DisplayName
			resp.CategoryID = &id
			resp.CategorySlug = &slugCopy
			resp.CategoryName = &nameCopy
			if cat.Icon != nil {
				resp.CategoryIcon = cat.Icon
			}
			if cat.Color != nil {
				resp.CategoryColor = cat.Color
			}
		}
	}
	return &resp
}

// convertRuleRows batch-converts scanned rows to responses, reusing a single
// slug lookup per unique category_slug to avoid N+1 GetCategoryBySlug calls.
func (s *Service) convertRuleRows(ctx context.Context, scanned []ruleRow) []TransactionRuleResponse {
	if len(scanned) == 0 {
		return nil
	}

	// First pass: collect unique category slugs.
	seen := make(map[string]struct{})
	for i := range scanned {
		resp := scanned[i].toResponseBase()
		if slug := categoryActionSlug(resp.Actions); slug != "" {
			seen[slug] = struct{}{}
		}
	}

	// Resolve each unique slug once.
	type catInfo struct {
		id, slug, displayName string
		icon, color           *string
	}
	cache := make(map[string]catInfo, len(seen))
	for slug := range seen {
		if cat, err := s.GetCategoryBySlug(ctx, slug); err == nil {
			cache[slug] = catInfo{
				id:          cat.ID,
				slug:        cat.Slug,
				displayName: cat.DisplayName,
				icon:        cat.Icon,
				color:       cat.Color,
			}
		}
	}

	// Second pass: build responses.
	rules := make([]TransactionRuleResponse, len(scanned))
	for i := range scanned {
		resp := scanned[i].toResponseBase()
		if slug := categoryActionSlug(resp.Actions); slug != "" {
			if info, ok := cache[slug]; ok {
				id := info.id
				slugCopy := info.slug
				nameCopy := info.displayName
				resp.CategoryID = &id
				resp.CategorySlug = &slugCopy
				resp.CategoryName = &nameCopy
				resp.CategoryIcon = info.icon
				resp.CategoryColor = info.color
			}
		}
		rules[i] = resp
	}
	return rules
}

// materializeRuleTagAdd applies an add_tag action retroactively: auto-creates
// the tag if needed, upserts transaction_tags with rule provenance, and writes
// a tag_added annotation. Runs inside the caller's pgx.Tx. Idempotent — returns
// (wasAdded=false, nil) when the tag was already attached. Malformed slugs are
// dropped silently (write-time validation is authoritative; this is belt-and-
// suspenders defense against direct-DB tampering).
func (s *Service) materializeRuleTagAdd(ctx context.Context, tx pgx.Tx, txnID pgtype.UUID, slug string, ruleID pgtype.UUID, ruleShortID, ruleName string) (bool, error) {
	if !tagSlugPattern.MatchString(slug) {
		return false, nil
	}
	var tagID pgtype.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM tags WHERE slug = $1`, slug).Scan(&tagID); err != nil {
		// Auto-create on miss. ON CONFLICT DO UPDATE SET updated_at=... is a
		// harmless no-op-returning-id dance so we always get the tag id back.
		if err2 := tx.QueryRow(ctx, `
			INSERT INTO tags (slug, display_name)
			VALUES ($1, $2)
			ON CONFLICT (slug) DO UPDATE SET updated_at = tags.updated_at
			RETURNING id`, slug, slugs.TitleCase(slug)).Scan(&tagID); err2 != nil {
			return false, fmt.Errorf("get or create tag %q: %w", slug, err2)
		}
	}

	cmd, err := tx.Exec(ctx,
		`INSERT INTO transaction_tags (transaction_id, tag_id, added_by_type, added_by_id, added_by_name)
		VALUES ($1, $2, 'rule', $3, $4)
		ON CONFLICT (transaction_id, tag_id) DO NOTHING`,
		txnID, tagID, ruleShortID, ruleName)
	if err != nil {
		return false, fmt.Errorf("upsert transaction_tag: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		// Already attached — no annotation written (idempotent).
		return false, nil
	}

	payload := map[string]any{
		"slug":       slug,
		"source":     "rule",
		"applied_by": "retroactive",
		"rule_id":    ruleShortID,
		"rule_name":  ruleName,
	}
	if err := writeAnnotation(ctx, s.Queries.WithTx(tx), writeAnnotationParams{
		TransactionID: txnID,
		Kind:          "tag_added",
		ActorType:     "system",
		ActorID:       ruleShortID,
		ActorName:     ruleName,
		Payload:       payload,
		TagID:         tagID,
		RuleID:        ruleID,
	}); err != nil {
		return true, fmt.Errorf("annotate tag_added: %w", err)
	}
	return true, nil
}

// materializeRuleTagRemove applies a remove_tag action retroactively: deletes
// the (transaction, tag) row and writes a tag_removed annotation with the rule
// as actor. No-op if the tag isn't attached. Runs inside the caller's pgx.Tx.
// Malformed slugs are dropped silently.
func (s *Service) materializeRuleTagRemove(ctx context.Context, tx pgx.Tx, txnID pgtype.UUID, slug string, ruleID pgtype.UUID, ruleShortID, ruleName string) (bool, error) {
	if !tagSlugPattern.MatchString(slug) {
		return false, nil
	}
	var tagID pgtype.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM tags WHERE slug = $1`, slug).Scan(&tagID); err != nil {
		// Tag doesn't exist — nothing to remove.
		return false, nil
	}
	cmd, err := tx.Exec(ctx,
		`DELETE FROM transaction_tags WHERE transaction_id = $1 AND tag_id = $2`,
		txnID, tagID)
	if err != nil {
		return false, fmt.Errorf("delete transaction_tag: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return false, nil
	}

	payload := map[string]any{
		"slug":       slug,
		"source":     "rule",
		"applied_by": "retroactive",
		"rule_id":    ruleShortID,
		"rule_name":  ruleName,
		"note":       fmt.Sprintf("Removed by rule: %s", ruleName),
	}
	if err := writeAnnotation(ctx, s.Queries.WithTx(tx), writeAnnotationParams{
		TransactionID: txnID,
		Kind:          "tag_removed",
		ActorType:     "system",
		ActorID:       ruleShortID,
		ActorName:     ruleName,
		Payload:       payload,
		TagID:         tagID,
		RuleID:        ruleID,
	}); err != nil {
		return true, fmt.Errorf("annotate tag_removed: %w", err)
	}
	return true, nil
}

// categorySlugToUUID resolves a category slug to its UUID via the service-layer
// lookup. Returns an invalid UUID (Valid=false) if the slug is unknown.
func (s *Service) categorySlugToUUID(ctx context.Context, slug string) (pgtype.UUID, error) {
	if slug == "" {
		return pgtype.UUID{}, nil
	}
	cat, err := s.GetCategoryBySlug(ctx, slug)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("%w: category slug %q not found", ErrInvalidParameter, slug)
	}
	return parseUUID(cat.ID)
}

// parseDuration parses a duration string like "30d", "24h", "1w".
func parseDuration(s string) (time.Duration, error) {
	if len(s) < 2 {
		return 0, fmt.Errorf("duration too short")
	}
	numStr := s[:len(s)-1]
	unit := s[len(s)-1:]

	num, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, fmt.Errorf("invalid duration number: %w", err)
	}
	if num <= 0 {
		return 0, fmt.Errorf("duration must be positive (got %d)", num)
	}

	switch unit {
	case "h":
		return time.Duration(num) * time.Hour, nil
	case "d":
		return time.Duration(num) * 24 * time.Hour, nil
	case "w":
		return time.Duration(num) * 7 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unknown duration unit %q (use h, d, or w)", unit)
	}
}

// ConditionSummary returns a human-readable summary of a condition tree.
//
// A zero-value (empty) Condition renders as "All transactions" — the
// match-all semantic for rules with NULL conditions. UI surfaces (rules list,
// detail page, preview banner) rely on this string to communicate the
// catch-all behaviour.
func ConditionSummary(c Condition) string {
	if conditionIsEmpty(c) {
		return "All transactions"
	}
	if len(c.And) > 0 {
		parts := make([]string, len(c.And))
		for i, sub := range c.And {
			parts[i] = ConditionSummary(sub)
		}
		return strings.Join(parts, " AND ")
	}
	if len(c.Or) > 0 {
		parts := make([]string, len(c.Or))
		for i, sub := range c.Or {
			parts[i] = ConditionSummary(sub)
		}
		return "(" + strings.Join(parts, " OR ") + ")"
	}
	if c.Not != nil {
		return "NOT (" + ConditionSummary(*c.Not) + ")"
	}

	valStr := fmt.Sprintf("%v", c.Value)
	switch c.Op {
	case "eq":
		return fmt.Sprintf("%s = %q", c.Field, valStr)
	case "neq":
		return fmt.Sprintf("%s != %q", c.Field, valStr)
	case "contains":
		return fmt.Sprintf("%s contains %q", c.Field, valStr)
	case "not_contains":
		return fmt.Sprintf("%s not contains %q", c.Field, valStr)
	case "matches":
		return fmt.Sprintf("%s matches /%s/", c.Field, valStr)
	case "gt":
		return fmt.Sprintf("%s > %s", c.Field, valStr)
	case "gte":
		return fmt.Sprintf("%s >= %s", c.Field, valStr)
	case "lt":
		return fmt.Sprintf("%s < %s", c.Field, valStr)
	case "lte":
		return fmt.Sprintf("%s <= %s", c.Field, valStr)
	case "in":
		return fmt.Sprintf("%s in %v", c.Field, c.Value)
	default:
		return fmt.Sprintf("%s %s %q", c.Field, c.Op, valStr)
	}
}

// ActionsSummary returns a short human-readable summary of a rule's actions.
//
//   - 1 action  → "Set category: Groceries" / "Add tag needs-review" / "Add comment"
//   - 2+ actions → "3 actions"
//   - Empty   → "(no actions)" — should never happen for a valid rule.
//
// The optional categoryName arg is used when an action is a set_category and
// the caller has already resolved the friendly category display name (kept on
// the response struct via TransactionRuleResponse.CategoryName). When empty,
// the slug is used.
func ActionsSummary(actions []RuleAction, categoryName string) string {
	if len(actions) == 0 {
		return "(no actions)"
	}
	if len(actions) > 1 {
		return fmt.Sprintf("%d actions", len(actions))
	}
	a := actions[0]
	switch a.Type {
	case "set_category":
		label := a.CategorySlug
		if categoryName != "" {
			label = categoryName
		}
		return "Set category: " + label
	case "add_tag":
		return "Add tag " + a.TagSlug
	case "remove_tag":
		return "Remove tag " + a.TagSlug
	case "add_comment":
		return "Add comment"
	default:
		return a.Type
	}
}

// TriggerLabel returns the human-readable label for a rule trigger value.
// The "On sync …" phrasing makes it explicit that rules fire during provider
// sync, not on arbitrary admin edits. "on_update" is accepted as a back-compat
// alias for "on_change". Falls back to the raw value for unknown triggers and
// "On sync create" when the value is empty.
func TriggerLabel(trigger string) string {
	switch trigger {
	case "", "on_create":
		return "On sync create"
	case "on_change", "on_update":
		return "On sync change"
	case "always":
		return "On sync create or change"
	default:
		return trigger
	}
}

// --- Rule application tracking ---

// RuleApplicationRow represents a transaction affected by a rule.
type RuleApplicationRow struct {
	ID              string  `json:"id"`
	TransactionID   string  `json:"transaction_id"`
	TransactionName string  `json:"transaction_name"`
	Amount          float64 `json:"amount"`
	Date            string  `json:"date"`
	ActionField     string  `json:"action_field"`
	ActionValue     string  `json:"action_value"`
	AppliedBy       string  `json:"applied_by"`
	AppliedAt       string  `json:"applied_at"`
}

// RuleStats contains aggregate stats about a rule's impact.
type RuleStats struct {
	TotalApplications    int64  `json:"total_applications"`
	UniqueTransactions   int64  `json:"unique_transactions"`
	FirstAppliedAt       string `json:"first_applied_at,omitempty"`
	LastAppliedAt        string `json:"last_applied_at,omitempty"`
	SyncApplications     int64  `json:"sync_applications"`
	RetroApplications    int64  `json:"retro_applications"`
}

// GetRuleStats returns aggregate stats about a rule's applications, sourced
// from annotations (kind='rule_applied').
func (s *Service) GetRuleStats(ctx context.Context, ruleID string) (*RuleStats, error) {
	ruleUUID, err := s.resolveRuleID(ctx, ruleID)
	if err != nil {
		return &RuleStats{}, nil
	}

	stats := &RuleStats{}
	err = s.Pool.QueryRow(ctx, `SELECT
		COUNT(*) AS total,
		COUNT(DISTINCT transaction_id) AS unique_txns,
		COALESCE(MIN(created_at)::text, ''),
		COALESCE(MAX(created_at)::text, ''),
		COUNT(*) FILTER (WHERE payload->>'applied_by' = 'sync'),
		COUNT(*) FILTER (WHERE payload->>'applied_by' = 'retroactive')
		FROM annotations WHERE kind = 'rule_applied' AND rule_id = $1`, ruleUUID).Scan(
		&stats.TotalApplications,
		&stats.UniqueTransactions,
		&stats.FirstAppliedAt,
		&stats.LastAppliedAt,
		&stats.SyncApplications,
		&stats.RetroApplications,
	)
	if err != nil {
		return &RuleStats{}, nil
	}
	return stats, nil
}

// ListRuleApplications returns transactions affected by a rule, paginated,
// sourced from annotations(kind='rule_applied').
func (s *Service) ListRuleApplications(ctx context.Context, ruleID string, limit int, cursor string) ([]RuleApplicationRow, bool, error) {
	ruleUUID, err := s.resolveRuleID(ctx, ruleID)
	if err != nil {
		return nil, false, ErrNotFound
	}

	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	query := `SELECT ann.id, ann.transaction_id, t.name, t.amount, t.date,
		COALESCE(ann.payload->>'action_field', ''),
		COALESCE(ann.payload->>'action_value', ''),
		COALESCE(ann.payload->>'applied_by', 'sync'),
		ann.created_at
		FROM annotations ann
		JOIN transactions t ON ann.transaction_id = t.id
		WHERE ann.kind = 'rule_applied' AND ann.rule_id = $1`

	args := []any{ruleUUID}
	argN := 2

	if cursor != "" {
		cursorTime, cursorIDStr, err := decodeTimestampCursor(cursor)
		if err != nil {
			return nil, false, ErrInvalidCursor
		}
		cursorUUID, err := parseUUID(cursorIDStr)
		if err != nil {
			return nil, false, ErrInvalidCursor
		}
		query += fmt.Sprintf(" AND (ann.created_at, ann.id) < ($%d, $%d)", argN, argN+1)
		args = append(args, pgtype.Timestamptz{Time: cursorTime, Valid: true}, cursorUUID)
		argN += 2
	}

	query += " ORDER BY ann.created_at DESC, ann.id DESC"
	query += fmt.Sprintf(" LIMIT $%d", argN)
	args = append(args, limit+1)

	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, false, fmt.Errorf("query rule applications: %w", err)
	}
	defer rows.Close()

	var results []RuleApplicationRow
	for rows.Next() {
		var r RuleApplicationRow
		var txnID pgtype.UUID
		var amount float64
		var date pgtype.Date
		var appliedAt pgtype.Timestamptz
		var appID pgtype.UUID

		if err := rows.Scan(&appID, &txnID, &r.TransactionName, &amount, &date, &r.ActionField, &r.ActionValue, &r.AppliedBy, &appliedAt); err != nil {
			return nil, false, fmt.Errorf("scan rule application: %w", err)
		}
		r.ID = formatUUID(appID)
		r.TransactionID = formatUUID(txnID)
		r.Amount = amount
		if date.Valid {
			r.Date = date.Time.Format("2006-01-02")
		}
		r.AppliedAt = pgconv.TimestampStr(appliedAt)
		results = append(results, r)
	}

	hasMore := len(results) > limit
	if hasMore {
		results = results[:limit]
	}

	return results, hasMore, nil
}

// CountRuleApplications returns the number of unique transactions affected by a rule.
func (s *Service) CountRuleApplications(ctx context.Context, ruleID string) (int64, error) {
	ruleUUID, err := s.resolveRuleID(ctx, ruleID)
	if err != nil {
		return 0, nil
	}
	var count int64
	err = s.Pool.QueryRow(ctx, "SELECT COUNT(DISTINCT transaction_id) FROM annotations WHERE kind = 'rule_applied' AND rule_id = $1", ruleUUID).Scan(&count)
	return count, err
}

// GetRuleSyncHistory returns recent sync logs where this rule matched transactions.
func (s *Service) GetRuleSyncHistory(ctx context.Context, ruleID string, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := s.Pool.Query(ctx,
		`SELECT sl.id, sl.started_at, sl.status, sl.rule_hits, bc.institution_name
		FROM sync_logs sl
		JOIN bank_connections bc ON sl.connection_id = bc.id
		WHERE sl.rule_hits IS NOT NULL AND sl.rule_hits ? $1
		ORDER BY sl.started_at DESC LIMIT $2`, ruleID, limit)
	if err != nil {
		return nil, fmt.Errorf("query sync history: %w", err)
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var id pgtype.UUID
		var startedAt pgtype.Timestamptz
		var status string
		var ruleHits []byte
		var instName pgtype.Text

		if err := rows.Scan(&id, &startedAt, &status, &ruleHits, &instName); err != nil {
			return nil, fmt.Errorf("scan sync history: %w", err)
		}

		var hits map[string]int
		_ = json.Unmarshal(ruleHits, &hits)

		entry := map[string]any{
			"sync_id":     formatUUID(id),
			"started_at":  pgconv.TimestampStr(startedAt),
			"status":      status,
			"hit_count":   hits[ruleID],
			"institution": textPtr(instName),
		}
		results = append(results, entry)
	}

	return results, nil
}

// TransactionRuleApplicationDetail holds a rule application with rule metadata for the transaction detail page.
type TransactionRuleApplicationDetail struct {
	RuleID              string `json:"rule_id"`
	RuleName            string `json:"rule_name"`
	ActionField         string `json:"action_field"`
	ActionValue         string `json:"action_value"`
	CategoryDisplayName string `json:"category_display_name,omitempty"`
	AppliedBy           string `json:"applied_by"`
	AppliedAt           string `json:"applied_at"`
}

// ListRuleApplicationsByTransactionID returns all rules that applied actions
// to a transaction, sourced from annotations(kind='rule_applied').
func (s *Service) ListRuleApplicationsByTransactionID(ctx context.Context, transactionID string) ([]TransactionRuleApplicationDetail, error) {
	txnUUID, err := s.resolveTransactionID(ctx, transactionID)
	if err != nil {
		return nil, nil
	}

	rows, err := s.Pool.Query(ctx, `SELECT ann.rule_id, COALESCE(tr.name, 'Deleted rule'),
		COALESCE(ann.payload->>'action_field', ''),
		COALESCE(ann.payload->>'action_value', ''),
		COALESCE(c.display_name, ''),
		COALESCE(ann.payload->>'applied_by', 'sync'),
		ann.created_at
		FROM annotations ann
		LEFT JOIN transaction_rules tr ON tr.id = ann.rule_id
		LEFT JOIN categories c ON ann.payload->>'action_field' = 'category' AND c.slug = ann.payload->>'action_value'
		WHERE ann.kind = 'rule_applied' AND ann.transaction_id = $1
		ORDER BY ann.created_at ASC`, txnUUID)
	if err != nil {
		return nil, fmt.Errorf("query rule applications by txn: %w", err)
	}
	defer rows.Close()

	var results []TransactionRuleApplicationDetail
	for rows.Next() {
		var d TransactionRuleApplicationDetail
		var ruleID pgtype.UUID
		var appliedAt pgtype.Timestamptz
		if err := rows.Scan(&ruleID, &d.RuleName, &d.ActionField, &d.ActionValue,
			&d.CategoryDisplayName, &d.AppliedBy, &appliedAt); err != nil {
			return nil, fmt.Errorf("scan rule application: %w", err)
		}
		d.RuleID = formatUUID(ruleID)
		d.AppliedAt = pgconv.TimestampStr(appliedAt)
		results = append(results, d)
	}

	return results, nil
}
