package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// --- Condition validation and evaluation ---

// validConditionFields lists the fields available for rule conditions.
var validConditionFields = map[string]string{
	"name":              "string",
	"merchant_name":     "string",
	"amount":            "numeric",
	"category_primary":  "string",
	"category_detailed": "string",
	"pending":           "bool",
	"provider":          "string",
	"account_id":        "string",
	"user_id":           "string",
	"user_name":         "string",
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

// ValidateCondition recursively validates a condition tree.
func ValidateCondition(c Condition) error {
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

	And []*CompiledCondition
	Or  []*CompiledCondition
	Not *CompiledCondition
}

// CompileCondition pre-compiles regex patterns in a condition tree.
func CompileCondition(c Condition) (*CompiledCondition, error) {
	cc := &CompiledCondition{
		Field: c.Field,
		Op:    c.Op,
		Value: c.Value,
	}

	if c.Field != "" && c.Op == "matches" {
		s, _ := c.Value.(string)
		re, err := regexp.Compile(s)
		if err != nil {
			return nil, fmt.Errorf("compile regex for field %q: %w", c.Field, err)
		}
		cc.Regex = re
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
	case "pending":
		return evalBool(c, tctx.Pending)
	case "provider":
		return evalString(c, tctx.Provider)
	case "account_id":
		return evalString(c, tctx.AccountID)
	case "user_id":
		return evalString(c, tctx.UserID)
	case "user_name":
		return evalString(c, tctx.UserName)
	}
	return false
}

func evalString(c *CompiledCondition, actual string) bool {
	expected, _ := c.Value.(string)
	actualLower := strings.ToLower(actual)
	expectedLower := strings.ToLower(expected)

	switch c.Op {
	case "eq":
		return actualLower == expectedLower
	case "neq":
		return actualLower != expectedLower
	case "contains":
		return strings.Contains(actualLower, expectedLower)
	case "not_contains":
		return !strings.Contains(actualLower, expectedLower)
	case "matches":
		if c.Regex != nil {
			return c.Regex.MatchString(actual)
		}
		return false
	case "in":
		if vals, ok := toStringSlice(c.Value); ok {
			for _, v := range vals {
				if strings.ToLower(v) == actualLower {
					return true
				}
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

// validActionFields lists fields that rule actions can set.
var validActionFields = map[string]bool{
	"category": true,
	// future: "merchant_name", "notes", etc.
}

// ValidateActions validates a slice of rule actions.
func (s *Service) ValidateActions(ctx context.Context, actions []RuleAction) error {
	if len(actions) == 0 {
		return fmt.Errorf("%w: at least one action is required", ErrInvalidParameter)
	}
	seen := make(map[string]bool, len(actions))
	for _, a := range actions {
		if !validActionFields[a.Field] {
			return fmt.Errorf("%w: unknown action field %q", ErrInvalidParameter, a.Field)
		}
		if seen[a.Field] {
			return fmt.Errorf("%w: duplicate action field %q", ErrInvalidParameter, a.Field)
		}
		seen[a.Field] = true

		if a.Value == "" {
			return fmt.Errorf("%w: action value is required for field %q", ErrInvalidParameter, a.Field)
		}

		switch a.Field {
		case "category":
			if _, err := s.GetCategoryBySlug(ctx, a.Value); err != nil {
				return fmt.Errorf("%w: category slug %q not found", ErrInvalidParameter, a.Value)
			}
		}
	}
	return nil
}

// resolveActionsToCategory extracts the category action from actions and resolves it to a UUID.
// Returns a valid pgtype.UUID if a category action exists, invalid UUID otherwise.
func (s *Service) resolveActionsToCategory(ctx context.Context, actions []RuleAction) (pgtype.UUID, error) {
	for _, a := range actions {
		if a.Field == "category" {
			cat, err := s.GetCategoryBySlug(ctx, a.Value)
			if err != nil {
				return pgtype.UUID{}, fmt.Errorf("%w: category slug %q not found", ErrInvalidParameter, a.Value)
			}
			return parseUUID(cat.ID)
		}
	}
	return pgtype.UUID{}, nil
}

// categoryActionSlug returns the value of the category action, or empty string.
func categoryActionSlug(actions []RuleAction) string {
	for _, a := range actions {
		if a.Field == "category" {
			return a.Value
		}
	}
	return ""
}

// --- Service methods ---

// ruleSelectQuery is the base SELECT for transaction rules with category JOIN.
const ruleSelectQuery = `SELECT tr.id, tr.short_id, tr.name, tr.conditions, tr.category_id, tr.priority, tr.enabled,
	tr.expires_at, tr.created_by_type, tr.created_by_id, tr.created_by_name,
	tr.hit_count, tr.last_hit_at, tr.created_at, tr.updated_at,
	tr.actions, c.slug AS category_slug, c.display_name AS category_display_name,
	COALESCE(c.icon, cp.icon) AS category_icon, COALESCE(c.color, cp.color) AS category_color
	FROM transaction_rules tr
	LEFT JOIN categories c ON tr.category_id = c.id
	LEFT JOIN categories cp ON c.parent_id = cp.id`

// CreateTransactionRule creates a new transaction rule.
func (s *Service) CreateTransactionRule(ctx context.Context, params CreateTransactionRuleParams) (*TransactionRuleResponse, error) {
	// Validate conditions
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
		actions = []RuleAction{{Field: "category", Value: params.CategorySlug}}
	} else {
		return nil, fmt.Errorf("%w: either actions or category_slug is required", ErrInvalidParameter)
	}

	// Derive category_id from category action (denormalized cache)
	catID, err := s.resolveActionsToCategory(ctx, actions)
	if err != nil {
		return nil, err
	}

	conditionsJSON, err := json.Marshal(params.Conditions)
	if err != nil {
		return nil, fmt.Errorf("marshal conditions: %w", err)
	}

	actionsJSON, err := json.Marshal(actions)
	if err != nil {
		return nil, fmt.Errorf("marshal actions: %w", err)
	}

	priority := params.Priority
	if priority == 0 {
		priority = 10
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
		createdByID = pgtype.Text{String: params.Actor.ID, Valid: true}
	}

	query := `INSERT INTO transaction_rules (name, conditions, category_id, actions, priority, enabled, expires_at, created_by_type, created_by_id, created_by_name)
		VALUES ($1, $2, $3, $4, $5, TRUE, $6, $7, $8, $9)
		RETURNING id, name, conditions, category_id, priority, enabled, expires_at, created_by_type, created_by_id, created_by_name, hit_count, last_hit_at, created_at, updated_at, actions`

	var row ruleRow
	scanDest := row.scanDest()
	// INSERT RETURNING doesn't include JOINed category columns, so scan only the first 15
	err = s.Pool.QueryRow(ctx, query,
		params.Name,
		conditionsJSON,
		catID,
		actionsJSON,
		priority,
		expiresAt,
		createdByType,
		createdByID,
		createdByName,
	).Scan(scanDest[:15]...)
	if err != nil {
		return nil, fmt.Errorf("insert transaction rule: %w", err)
	}

	// Set category slug/name for response if there's a category action
	if slug := categoryActionSlug(actions); slug != "" {
		if cat, err := s.GetCategoryBySlug(ctx, slug); err == nil {
			row.categorySlug = pgtype.Text{String: cat.Slug, Valid: true}
			row.categoryDispName = pgtype.Text{String: cat.DisplayName, Valid: true}
		}
	}

	resp := row.toResponse()
	return &resp, nil
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

	resp := row.toResponse()
	return &resp, nil
}

// ListTransactionRules returns a filtered, paginated list of transaction rules.
func (s *Service) ListTransactionRules(ctx context.Context, params TransactionRuleListParams) (*TransactionRuleListResult, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	baseFrom := `FROM transaction_rules tr LEFT JOIN categories c ON tr.category_id = c.id LEFT JOIN categories cp ON c.parent_id = cp.id`

	var whereClauses []string
	var args []any
	argN := 1

	if params.CategorySlug != nil && *params.CategorySlug != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("c.slug = $%d", argN))
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
	countQuery := "SELECT COUNT(*) " + baseFrom + filterWhereSQL
	countArgs := make([]any, len(args))
	copy(countArgs, args)
	var total int64
	if err := s.Pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count rules: %w", err)
	}

	// Add cursor condition for the main query
	whereSQL := filterWhereSQL
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

	// Main query
	selectCols := `SELECT tr.id, tr.short_id, tr.name, tr.conditions, tr.category_id, tr.priority, tr.enabled,
		tr.expires_at, tr.created_by_type, tr.created_by_id, tr.created_by_name,
		tr.hit_count, tr.last_hit_at, tr.created_at, tr.updated_at,
		tr.actions, c.slug AS category_slug, c.display_name AS category_display_name,
		COALESCE(c.icon, cp.icon) AS category_icon, COALESCE(c.color, cp.color) AS category_color `

	orderBy := " ORDER BY tr.created_at DESC, tr.id DESC"
	limitClause := fmt.Sprintf(" LIMIT $%d", argN)
	args = append(args, limit+1)

	fullQuery := selectCols + baseFrom + whereSQL + orderBy + limitClause

	rows, err := s.Pool.Query(ctx, fullQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("query rules: %w", err)
	}
	defer rows.Close()

	type ruleWithTime struct {
		resp TransactionRuleResponse
		ts   time.Time
	}
	var allRules []ruleWithTime

	for rows.Next() {
		var row ruleRow
		if err := rows.Scan(row.scanDest()...); err != nil {
			return nil, fmt.Errorf("scan rule: %w", err)
		}
		allRules = append(allRules, ruleWithTime{resp: row.toResponse(), ts: row.createdAt.Time.UTC()})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rules: %w", err)
	}

	hasMore := len(allRules) > limit
	if hasMore {
		allRules = allRules[:limit]
	}

	rules := make([]TransactionRuleResponse, len(allRules))
	for i, r := range allRules {
		rules[i] = r.resp
	}

	var nextCursor string
	if hasMore && len(allRules) > 0 {
		last := allRules[len(allRules)-1]
		nextCursor = encodeTimestampCursor(last.ts, last.resp.ID)
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
		// Sugar: replace/add category action, keep other actions
		newActions := make([]RuleAction, 0, len(actions))
		for _, a := range actions {
			if a.Field != "category" {
				newActions = append(newActions, a)
			}
		}
		newActions = append(newActions, RuleAction{Field: "category", Value: *params.CategorySlug})
		if err := s.ValidateActions(ctx, newActions); err != nil {
			return nil, err
		}
		actions = newActions
	}

	// Derive category_id from category action
	catID, err := s.resolveActionsToCategory(ctx, actions)
	if err != nil {
		return nil, err
	}

	priority := int32(existing.Priority)
	if params.Priority != nil {
		priority = int32(*params.Priority)
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

	conditionsJSON, err := json.Marshal(conditions)
	if err != nil {
		return nil, fmt.Errorf("marshal conditions: %w", err)
	}

	actionsJSON, err := json.Marshal(actions)
	if err != nil {
		return nil, fmt.Errorf("marshal actions: %w", err)
	}

	ruleID, _ := parseUUID(id) // already validated by GetTransactionRule above

	query := `UPDATE transaction_rules
		SET name = $2, conditions = $3, category_id = $4, actions = $5, priority = $6, enabled = $7, expires_at = $8, updated_at = NOW()
		WHERE id = $1
		RETURNING id, name, conditions, category_id, priority, enabled, expires_at, created_by_type, created_by_id, created_by_name, hit_count, last_hit_at, created_at, updated_at, actions`

	var row ruleRow
	err = s.Pool.QueryRow(ctx, query,
		ruleID, name, conditionsJSON, catID, actionsJSON, priority, enabled, expiresAt,
	).Scan(row.scanDest()[:15]...)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("update transaction rule: %w", err)
	}

	// Resolve category slug for response
	if row.categoryID.Valid {
		_ = s.Pool.QueryRow(ctx, "SELECT slug, display_name FROM categories WHERE id = $1", row.categoryID).Scan(&row.categorySlug, &row.categoryDispName)
	}

	resp := row.toResponse()
	return &resp, nil
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

// ListActiveRulesForSync returns all enabled, non-expired rules ordered by priority.
func (s *Service) ListActiveRulesForSync(ctx context.Context) ([]TransactionRuleResponse, error) {
	query := ruleSelectQuery + ` WHERE tr.enabled = TRUE
		AND (tr.expires_at IS NULL OR tr.expires_at > NOW())
		ORDER BY tr.priority DESC, tr.created_at DESC`

	rows, err := s.Pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list active rules: %w", err)
	}
	defer rows.Close()

	var rules []TransactionRuleResponse
	for rows.Next() {
		var row ruleRow
		if err := rows.Scan(row.scanDest()...); err != nil {
			return nil, fmt.Errorf("scan active rule: %w", err)
		}
		rules = append(rules, row.toResponse())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active rules: %w", err)
	}

	return rules, nil
}

// transactionContextQuery is the base query for loading transactions with full context
// (JOIN through accounts → bank_connections → users) for rule evaluation.
const transactionContextQuery = `SELECT t.id, t.name, COALESCE(t.merchant_name, ''), t.amount,
	COALESCE(t.category_primary, ''), COALESCE(t.category_detailed, ''),
	t.pending, bc.provider, t.account_id::text, COALESCE(u.id::text, ''), COALESCE(u.name, '')
	FROM transactions t
	JOIN accounts a ON t.account_id = a.id
	JOIN bank_connections bc ON a.connection_id = bc.id
	LEFT JOIN users u ON bc.user_id = u.id
	WHERE t.deleted_at IS NULL AND t.category_override = FALSE
	AND (a.is_dependent_linked = FALSE OR NOT EXISTS (SELECT 1 FROM transaction_matches tm WHERE tm.dependent_transaction_id = t.id))`

// transactionContextRow holds a scanned transaction row for rule evaluation.
type transactionContextRow struct {
	id        pgtype.UUID
	tctx      TransactionContext
	accountID pgtype.UUID
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
	return r
}

// ApplyRuleRetroactively applies a single rule to all existing non-deleted, non-overridden
// transactions. Returns count of affected rows.
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

	// Build SET clause from actions
	setClauses, setArgs, err := s.buildActionSetClause(ctx, rule.Actions)
	if err != nil {
		return 0, fmt.Errorf("build action set clause: %w", err)
	}
	if len(setClauses) == 0 {
		return 0, fmt.Errorf("%w: rule has no applicable actions", ErrInvalidParameter)
	}

	var totalAffected int64
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
		query += " ORDER BY t.id ASC"
		query += fmt.Sprintf(" LIMIT $%d", argN)
		args = append(args, 1000)

		rows, err := s.Pool.Query(ctx, query, args...)
		if err != nil {
			return totalAffected, fmt.Errorf("query transactions: %w", err)
		}

		var matchIDs []pgtype.UUID
		rowCount := 0

		for rows.Next() {
			rowCount++
			dest := make([]any, 11)
			r := scanTransactionContextRow(dest)
			if err := rows.Scan(dest...); err != nil {
				rows.Close()
				return totalAffected, fmt.Errorf("scan transaction: %w", err)
			}
			lastID = r.id

			if EvaluateCondition(compiled, r.tctx) {
				matchIDs = append(matchIDs, r.id)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return totalAffected, fmt.Errorf("iterate transactions: %w", err)
		}

		// Bulk update matching transactions
		if len(matchIDs) > 0 {
			// Build: UPDATE transactions SET <actions>, updated_at = NOW() WHERE id = ANY($N) AND ...
			updateArgs := make([]any, len(setArgs))
			copy(updateArgs, setArgs)
			idsArgN := len(updateArgs) + 1
			updateArgs = append(updateArgs, matchIDs)

			updateSQL := fmt.Sprintf(`UPDATE transactions SET %s, updated_at = NOW()
				WHERE id = ANY($%d) AND category_override = FALSE AND deleted_at IS NULL`,
				strings.Join(setClauses, ", "), idsArgN)

			tag, err := s.Pool.Exec(ctx, updateSQL, updateArgs...)
			if err != nil {
				return totalAffected, fmt.Errorf("update transactions: %w", err)
			}
			totalAffected += tag.RowsAffected()

			// Record rule applications
			ruleUUID, _ := parseUUID(ruleID)
			for _, action := range rule.Actions {
				s.recordRuleApplications(ctx, ruleUUID, matchIDs, action.Field, action.Value, "retroactive")
			}
		}

		if rowCount < 1000 {
			break
		}
	}

	// Update hit count
	if totalAffected > 0 {
		ruleUUID, _ := parseUUID(ruleID)
		_, _ = s.Pool.Exec(ctx, "UPDATE transaction_rules SET hit_count = hit_count + $2, last_hit_at = NOW() WHERE id = $1", ruleUUID, totalAffected)
	}

	return totalAffected, nil
}

// ApplyAllRulesRetroactively applies all active rules (priority DESC) to existing transactions.
// Multiple rules can match — actions are merged (first rule to set a field wins).
// Returns a map of rule_id → match_count.
func (s *Service) ApplyAllRulesRetroactively(ctx context.Context) (map[string]int64, error) {
	rules, err := s.ListActiveRulesForSync(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active rules: %w", err)
	}
	if len(rules) == 0 {
		return map[string]int64{}, nil
	}

	// Compile all rules
	type compiledRule struct {
		id       string
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
		compiled = append(compiled, compiledRule{id: r.ID, actions: r.Actions, compiled: cc})
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
		query += " ORDER BY t.id ASC"
		query += fmt.Sprintf(" LIMIT $%d", argN)
		args = append(args, 1000)

		rows, err := s.Pool.Query(ctx, query, args...)
		if err != nil {
			return hitCounts, fmt.Errorf("query transactions: %w", err)
		}

		// Per transaction: merge actions from all matching rules, keyed by category_id for batching
		type txnRuleApp struct {
			txnID       pgtype.UUID
			ruleID      string
			actionField string
			actionValue string
		}
		updates := make(map[pgtype.UUID][]pgtype.UUID) // category_id → transaction IDs
		var applications []txnRuleApp
		rowCount := 0

		for rows.Next() {
			rowCount++
			dest := make([]any, 11)
			r := scanTransactionContextRow(dest)
			if err := rows.Scan(dest...); err != nil {
				rows.Close()
				return hitCounts, fmt.Errorf("scan transaction: %w", err)
			}
			lastID = r.id

			// Merge actions from all matching rules (first to set a field wins)
			var mergedCatID pgtype.UUID
			var winningRuleID string
			var winningSlug string
			for _, cr := range compiled {
				if EvaluateCondition(cr.compiled, r.tctx) {
					hitCounts[cr.id]++
					// Merge category action (first wins)
					if !mergedCatID.Valid {
						if slug := categoryActionSlug(cr.actions); slug != "" {
							if catID, err := s.resolveActionsToCategory(ctx, cr.actions); err == nil && catID.Valid {
								mergedCatID = catID
								winningRuleID = cr.id
								winningSlug = slug
							}
						}
					}
					// future: merge other action fields here
				}
			}

			if mergedCatID.Valid {
				updates[mergedCatID] = append(updates[mergedCatID], r.id)
				applications = append(applications, txnRuleApp{
					txnID: r.id, ruleID: winningRuleID, actionField: "category", actionValue: winningSlug,
				})
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return hitCounts, fmt.Errorf("iterate transactions: %w", err)
		}

		// Bulk update per category
		for catID, txnIDs := range updates {
			_, err := s.Pool.Exec(ctx,
				`UPDATE transactions SET category_id = $1, updated_at = NOW()
				WHERE id = ANY($2) AND category_override = FALSE AND deleted_at IS NULL`,
				catID, txnIDs)
			if err != nil {
				return hitCounts, fmt.Errorf("update transactions: %w", err)
			}
		}

		// Record rule applications
		for _, app := range applications {
			ruleUUID, _ := parseUUID(app.ruleID)
			s.recordRuleApplications(ctx, ruleUUID, []pgtype.UUID{app.txnID}, app.actionField, app.actionValue, "retroactive")
		}

		if rowCount < 1000 {
			break
		}
	}

	// Update hit counts for all matched rules
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

	// When previewing for a specific rule's detail page, exclude transactions already applied by this rule
	var baseArgs []any
	baseArgN := 1
	if excludeRuleID != nil {
		baseQuery += fmt.Sprintf(" AND NOT EXISTS (SELECT 1 FROM transaction_rule_applications tra WHERE tra.transaction_id = t.id AND tra.rule_id = $%d)", baseArgN)
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

// ruleRow holds the scanned columns for a transaction rule row.
type ruleRow struct {
	id               pgtype.UUID
	shortID          string
	name             string
	conditions       []byte
	categoryID       pgtype.UUID
	priority         int32
	enabled          bool
	expiresAt        pgtype.Timestamptz
	createdByType    string
	createdByID      pgtype.Text
	createdByName    string
	hitCount         int32
	lastHitAt        pgtype.Timestamptz
	createdAt        pgtype.Timestamptz
	updatedAt        pgtype.Timestamptz
	actions          []byte
	categorySlug     pgtype.Text
	categoryDispName pgtype.Text
	categoryIcon     pgtype.Text
	categoryColor    pgtype.Text
}

// scanDest returns a slice of pointers for use with rows.Scan.
func (r *ruleRow) scanDest() []any {
	return []any{
		&r.id, &r.shortID, &r.name, &r.conditions, &r.categoryID, &r.priority, &r.enabled,
		&r.expiresAt, &r.createdByType, &r.createdByID, &r.createdByName,
		&r.hitCount, &r.lastHitAt, &r.createdAt, &r.updatedAt,
		&r.actions, &r.categorySlug, &r.categoryDispName, &r.categoryIcon, &r.categoryColor,
	}
}

// toResponse converts a scanned rule row to a TransactionRuleResponse.
func (r *ruleRow) toResponse() TransactionRuleResponse {
	var cond Condition
	_ = json.Unmarshal(r.conditions, &cond)

	var actions []RuleAction
	_ = json.Unmarshal(r.actions, &actions)
	if actions == nil {
		actions = []RuleAction{}
	}

	return TransactionRuleResponse{
		ID:            formatUUID(r.id),
		ShortID:       r.shortID,
		Name:          r.name,
		Conditions:    cond,
		Actions:       actions,
		CategoryID:    uuidPtr(r.categoryID),
		CategorySlug:  textPtr(r.categorySlug),
		CategoryName:  textPtr(r.categoryDispName),
		CategoryIcon:  textPtr(r.categoryIcon),
		CategoryColor: textPtr(r.categoryColor),
		Priority:      int(r.priority),
		Enabled:       r.enabled,
		ExpiresAt:     timestampStr(r.expiresAt),
		CreatedByType: r.createdByType,
		CreatedByID:   textPtr(r.createdByID),
		CreatedByName: r.createdByName,
		HitCount:      int(r.hitCount),
		LastHitAt:     timestampStr(r.lastHitAt),
		CreatedAt:     r.createdAt.Time.UTC().Format(time.RFC3339),
		UpdatedAt:     r.updatedAt.Time.UTC().Format(time.RFC3339),
	}
}

// recordRuleApplications inserts rule application records for a batch of transactions.
func (s *Service) recordRuleApplications(ctx context.Context, ruleID pgtype.UUID, txnIDs []pgtype.UUID, actionField, actionValue, appliedBy string) {
	for _, txnID := range txnIDs {
		_, _ = s.Pool.Exec(ctx,
			`INSERT INTO transaction_rule_applications (transaction_id, rule_id, action_field, action_value, applied_by)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (transaction_id, rule_id, action_field) DO UPDATE SET applied_at = NOW(), action_value = EXCLUDED.action_value, applied_by = EXCLUDED.applied_by`,
			txnID, ruleID, actionField, actionValue, appliedBy)
	}
}

// buildActionSetClause converts rule actions into SQL SET clause components.
// Returns setClauses (e.g., ["category_id = $1"]), args, and error.
func (s *Service) buildActionSetClause(ctx context.Context, actions []RuleAction) ([]string, []any, error) {
	var setClauses []string
	var args []any
	argN := 1

	for _, a := range actions {
		switch a.Field {
		case "category":
			catID, err := s.resolveActionsToCategory(ctx, []RuleAction{a})
			if err != nil {
				return nil, nil, err
			}
			if catID.Valid {
				setClauses = append(setClauses, fmt.Sprintf("category_id = $%d", argN))
				args = append(args, catID)
				argN++
			}
		// future: case "merchant_name": ...
		}
	}

	return setClauses, args, nil
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
func ConditionSummary(c Condition) string {
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

// GetRuleStats returns aggregate stats about a rule's applications.
func (s *Service) GetRuleStats(ctx context.Context, ruleID string) (*RuleStats, error) {
	ruleUUID, err := s.resolveRuleID(ctx, ruleID)
	if err != nil {
		return &RuleStats{}, nil
	}

	stats := &RuleStats{}
	err = s.Pool.QueryRow(ctx, `SELECT
		COUNT(*) AS total,
		COUNT(DISTINCT transaction_id) AS unique_txns,
		COALESCE(MIN(applied_at)::text, ''),
		COALESCE(MAX(applied_at)::text, ''),
		COUNT(*) FILTER (WHERE applied_by = 'sync'),
		COUNT(*) FILTER (WHERE applied_by = 'retroactive')
		FROM transaction_rule_applications WHERE rule_id = $1`, ruleUUID).Scan(
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

// ListRuleApplications returns transactions affected by a rule, paginated.
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

	query := `SELECT tra.id, tra.transaction_id, t.name, t.amount, t.date, tra.action_field, tra.action_value, tra.applied_by, tra.applied_at
		FROM transaction_rule_applications tra
		JOIN transactions t ON tra.transaction_id = t.id
		WHERE tra.rule_id = $1`

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
		query += fmt.Sprintf(" AND (tra.applied_at, tra.id) < ($%d, $%d)", argN, argN+1)
		args = append(args, pgtype.Timestamptz{Time: cursorTime, Valid: true}, cursorUUID)
		argN += 2
	}

	query += " ORDER BY tra.applied_at DESC, tra.id DESC"
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
		if appliedAt.Valid {
			r.AppliedAt = appliedAt.Time.UTC().Format(time.RFC3339)
		}
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
	err = s.Pool.QueryRow(ctx, "SELECT COUNT(DISTINCT transaction_id) FROM transaction_rule_applications WHERE rule_id = $1", ruleUUID).Scan(&count)
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
			"started_at":  startedAt.Time.UTC().Format(time.RFC3339),
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

// ListRuleApplicationsByTransactionID returns all rules that applied actions to a transaction.
func (s *Service) ListRuleApplicationsByTransactionID(ctx context.Context, transactionID string) ([]TransactionRuleApplicationDetail, error) {
	txnUUID, err := s.resolveTransactionID(ctx, transactionID)
	if err != nil {
		return nil, nil
	}

	rows, err := s.Pool.Query(ctx, `SELECT tra.rule_id, tr.name, tra.action_field, tra.action_value,
		COALESCE(c.display_name, ''), tra.applied_by, tra.applied_at
		FROM transaction_rule_applications tra
		JOIN transaction_rules tr ON tr.id = tra.rule_id
		LEFT JOIN categories c ON tra.action_field = 'category' AND c.slug = tra.action_value
		WHERE tra.transaction_id = $1
		ORDER BY tra.applied_at ASC`, txnUUID)
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
		if appliedAt.Valid {
			d.AppliedAt = appliedAt.Time.UTC().Format(time.RFC3339)
		}
		results = append(results, d)
	}

	return results, nil
}
