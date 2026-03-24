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
				// Value should be an array
				if _, ok := toStringSlice(c.Value); !ok {
					return fmt.Errorf("%w: 'in' operator requires an array value for field %q", ErrInvalidParameter, c.Field)
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

// --- Service methods ---

// ruleSelectQuery is the base SELECT for transaction rules with category JOIN.
const ruleSelectQuery = `SELECT tr.id, tr.name, tr.conditions, tr.category_id, tr.priority, tr.enabled,
	tr.expires_at, tr.created_by_type, tr.created_by_id, tr.created_by_name,
	tr.hit_count, tr.last_hit_at, tr.created_at, tr.updated_at,
	c.slug AS category_slug, c.display_name AS category_display_name
	FROM transaction_rules tr
	LEFT JOIN categories c ON tr.category_id = c.id`

// CreateTransactionRule creates a new transaction rule.
func (s *Service) CreateTransactionRule(ctx context.Context, params CreateTransactionRuleParams) (*TransactionRuleResponse, error) {
	// Validate conditions
	if err := ValidateCondition(params.Conditions); err != nil {
		return nil, err
	}

	// Resolve category slug to ID
	cat, err := s.GetCategoryBySlug(ctx, params.CategorySlug)
	if err != nil {
		return nil, fmt.Errorf("%w: category slug %q not found", ErrInvalidParameter, params.CategorySlug)
	}

	catID, err := parseUUID(cat.ID)
	if err != nil {
		return nil, fmt.Errorf("parse category id: %w", err)
	}

	conditionsJSON, err := json.Marshal(params.Conditions)
	if err != nil {
		return nil, fmt.Errorf("marshal conditions: %w", err)
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

	query := `INSERT INTO transaction_rules (name, conditions, category_id, priority, enabled, expires_at, created_by_type, created_by_id, created_by_name)
		VALUES ($1, $2, $3, $4, TRUE, $5, $6, $7, $8)
		RETURNING id, name, conditions, category_id, priority, enabled, expires_at, created_by_type, created_by_id, created_by_name, hit_count, last_hit_at, created_at, updated_at`

	var row ruleRow
	scanDest := row.scanDest()
	// INSERT RETURNING doesn't include JOINed category columns, so scan only the first 14
	err = s.Pool.QueryRow(ctx, query,
		params.Name,
		conditionsJSON,
		catID,
		priority,
		expiresAt,
		createdByType,
		createdByID,
		createdByName,
	).Scan(scanDest[:14]...)
	if err != nil {
		return nil, fmt.Errorf("insert transaction rule: %w", err)
	}
	row.categorySlug = pgtype.Text{String: cat.Slug, Valid: true}
	row.categoryDispName = pgtype.Text{String: cat.DisplayName, Valid: true}

	resp := row.toResponse()
	return &resp, nil
}

// GetTransactionRule returns a transaction rule by ID.
func (s *Service) GetTransactionRule(ctx context.Context, id string) (*TransactionRuleResponse, error) {
	ruleID, err := parseUUID(id)
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
	if limit > 200 {
		limit = 200
	}

	baseFrom := `FROM transaction_rules tr LEFT JOIN categories c ON tr.category_id = c.id`

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
		whereClauses = append(whereClauses, fmt.Sprintf("tr.name ILIKE $%d", argN))
		args = append(args, "%"+*params.Search+"%")
		argN++
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
	selectCols := `SELECT tr.id, tr.name, tr.conditions, tr.category_id, tr.priority, tr.enabled,
		tr.expires_at, tr.created_by_type, tr.created_by_id, tr.created_by_name,
		tr.hit_count, tr.last_hit_at, tr.created_at, tr.updated_at,
		c.slug AS category_slug, c.display_name AS category_display_name `

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

	var categoryID pgtype.UUID
	if params.CategorySlug != nil {
		cat, err := s.GetCategoryBySlug(ctx, *params.CategorySlug)
		if err != nil {
			return nil, fmt.Errorf("%w: category slug %q not found", ErrInvalidParameter, *params.CategorySlug)
		}
		uid, _ := parseUUID(cat.ID)
		categoryID = uid
	} else if existing.CategoryID != nil {
		uid, _ := parseUUID(*existing.CategoryID)
		categoryID = uid
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

	ruleID, _ := parseUUID(id) // already validated by GetTransactionRule above

	query := `UPDATE transaction_rules
		SET name = $2, conditions = $3, category_id = $4, priority = $5, enabled = $6, expires_at = $7, updated_at = NOW()
		WHERE id = $1
		RETURNING id, name, conditions, category_id, priority, enabled, expires_at, created_by_type, created_by_id, created_by_name, hit_count, last_hit_at, created_at, updated_at`

	var row ruleRow
	err = s.Pool.QueryRow(ctx, query,
		ruleID, name, conditionsJSON, categoryID, priority, enabled, expiresAt,
	).Scan(row.scanDest()[:14]...)
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
	ruleID, err := parseUUID(id)
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
	id              pgtype.UUID
	name            string
	conditions      []byte
	categoryID      pgtype.UUID
	priority        int32
	enabled         bool
	expiresAt       pgtype.Timestamptz
	createdByType   string
	createdByID     pgtype.Text
	createdByName   string
	hitCount        int32
	lastHitAt       pgtype.Timestamptz
	createdAt       pgtype.Timestamptz
	updatedAt       pgtype.Timestamptz
	categorySlug    pgtype.Text
	categoryDispName pgtype.Text
}

// scanDest returns a slice of pointers for use with rows.Scan.
func (r *ruleRow) scanDest() []any {
	return []any{
		&r.id, &r.name, &r.conditions, &r.categoryID, &r.priority, &r.enabled,
		&r.expiresAt, &r.createdByType, &r.createdByID, &r.createdByName,
		&r.hitCount, &r.lastHitAt, &r.createdAt, &r.updatedAt,
		&r.categorySlug, &r.categoryDispName,
	}
}

// toResponse converts a scanned rule row to a TransactionRuleResponse.
func (r *ruleRow) toResponse() TransactionRuleResponse {
	var cond Condition
	_ = json.Unmarshal(r.conditions, &cond)

	return TransactionRuleResponse{
		ID:            formatUUID(r.id),
		Name:          r.name,
		Conditions:    cond,
		CategoryID:    uuidPtr(r.categoryID),
		CategorySlug:  textPtr(r.categorySlug),
		CategoryName:  textPtr(r.categoryDispName),
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
