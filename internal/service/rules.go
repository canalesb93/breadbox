//go:build !lite

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
	"provider_name":              "string",
	"provider_merchant_name":     "string",
	"amount":                     "numeric",
	"provider_category_primary":  "string", // raw provider primary category
	"provider_category_detailed": "string", // raw provider detailed category
	"category":                   "string", // assigned category slug (distinct from provider raw)
	"pending":                    "bool",
	"provider":                   "string",
	"account_id":                 "string",
	"account_name":               "string",
	"user_id":                    "string",
	"user_name":                  "string",
	"tags":                       "tags",
	"series":                     "string", // recurring-series short_id the txn belongs to
	"in_series":                  "bool",   // whether the txn belongs to any recurring series
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

// metadataFieldPrefix marks a condition leaf that reads a key from the
// transaction's free-form metadata blob. The substring after the prefix is the
// metadata key, e.g. field "metadata.tax_deductible" reads metadata["tax_deductible"].
const metadataFieldPrefix = "metadata."

// metadataOps are operators valid for a metadata.<key> field. Metadata values
// are arbitrary JSON, so the set is the union of string, numeric, and presence
// operators; comparison semantics are resolved at eval time from the expected
// value's type (see evalMetadata). exists / not_exists test key presence and
// ignore the value.
var metadataOps = map[string]bool{
	"eq": true, "neq": true, "contains": true, "not_contains": true,
	"matches": true, "in": true,
	"gt": true, "gte": true, "lt": true, "lte": true,
	"exists": true, "not_exists": true,
}

// metadataKeyFromField extracts the metadata key from a "metadata.<key>" field,
// returning ("", false) when field is not a metadata field.
func metadataKeyFromField(field string) (string, bool) {
	if !strings.HasPrefix(field, metadataFieldPrefix) {
		return "", false
	}
	return field[len(metadataFieldPrefix):], true
}

// conditionFieldType returns the value-type bucket for a condition field,
// dispatching "metadata.<key>" dotted fields to the "metadata" bucket and
// otherwise consulting validConditionFields. Returns ("", false) for unknown
// fields.
func conditionFieldType(field string) (string, bool) {
	if _, ok := metadataKeyFromField(field); ok {
		return "metadata", true
	}
	t, ok := validConditionFields[field]
	return t, ok
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
		fieldType, ok := conditionFieldType(c.Field)
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
		case "metadata":
			key, _ := metadataKeyFromField(c.Field)
			if err := validateMetadataKey(key); err != nil {
				return fmt.Errorf("%w (field %q)", err, c.Field)
			}
			if !metadataOps[c.Op] {
				return fmt.Errorf("%w: operator %q not valid for metadata field %q", ErrInvalidParameter, c.Op, c.Field)
			}
			switch c.Op {
			case "exists", "not_exists":
				// Presence test — value is ignored, nothing to validate.
			case "in":
				vals, ok := toStringSlice(c.Value)
				if !ok || len(vals) == 0 {
					return fmt.Errorf("%w: 'in' operator requires a non-empty array for field %q", ErrInvalidParameter, c.Field)
				}
			case "matches":
				s, ok := c.Value.(string)
				if !ok {
					return fmt.Errorf("%w: 'matches' operator requires a string value for field %q", ErrInvalidParameter, c.Field)
				}
				if _, err := regexp.Compile(s); err != nil {
					return fmt.Errorf("%w: invalid regex pattern for field %q: %v", ErrInvalidParameter, c.Field, err)
				}
			case "gt", "gte", "lt", "lte":
				if _, ok := toFloat64(c.Value); !ok {
					return fmt.Errorf("%w: numeric value required for operator %q on field %q", ErrInvalidParameter, c.Op, c.Field)
				}
			default: // eq, neq, contains, not_contains
				if c.Value == nil {
					return fmt.Errorf("%w: operator %q requires a value for field %q", ErrInvalidParameter, c.Op, c.Field)
				}
				switch c.Value.(type) {
				case string, float64, float32, int, int64, bool, json.Number:
				default:
					return fmt.Errorf("%w: operator %q on field %q requires a scalar value (string, number, or boolean)", ErrInvalidParameter, c.Op, c.Field)
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
	case "provider_name":
		return evalString(c, tctx.Name)
	case "provider_merchant_name":
		return evalString(c, tctx.MerchantName)
	case "amount":
		return evalNumeric(c, tctx.Amount)
	case "provider_category_primary":
		return evalString(c, tctx.CategoryPrimary)
	case "provider_category_detailed":
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
	case "series":
		return evalString(c, tctx.SeriesShortID)
	case "in_series":
		return evalBool(c, tctx.InSeries)
	}
	if key, ok := metadataKeyFromField(c.Field); ok {
		return evalMetadata(c, key, tctx.Metadata)
	}
	return false
}

// evalMetadata evaluates a metadata.<key> leaf against the transaction's
// metadata blob. Presence operators (exists / not_exists) test key presence;
// every other operator requires the key to be present (an absent key matches
// only not_exists). Comparison semantics for eq / neq are driven by the
// expected value's type — a numeric expected compares numerically, a bool
// expected compares as bool, otherwise the stored value is stringified and
// compared case-insensitively. contains / not_contains / matches / in always
// operate on the stringified stored value.
func evalMetadata(c *CompiledCondition, key string, meta map[string]any) bool {
	raw, present := meta[key]
	switch c.Op {
	case "exists":
		return present
	case "not_exists":
		return !present
	}
	if !present {
		return false
	}
	switch c.Op {
	case "eq":
		return metadataEquals(raw, c.Value)
	case "neq":
		return !metadataEquals(raw, c.Value)
	case "contains":
		return strings.Contains(strings.ToLower(metadataString(raw)), strings.ToLower(metadataString(c.Value)))
	case "not_contains":
		return !strings.Contains(strings.ToLower(metadataString(raw)), strings.ToLower(metadataString(c.Value)))
	case "matches":
		if c.Regex != nil {
			return c.Regex.MatchString(metadataString(raw))
		}
		return false
	case "in":
		actual := metadataString(raw)
		if vals, ok := toStringSlice(c.Value); ok {
			for _, v := range vals {
				if strings.EqualFold(actual, v) {
					return true
				}
			}
		}
		return false
	case "gt", "gte", "lt", "lte":
		actual, ok1 := metadataFloat(raw)
		expected, ok2 := toFloat64(c.Value)
		if !ok1 || !ok2 {
			return false
		}
		switch c.Op {
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

// metadataEquals compares a stored metadata value against an expected condition
// value. The expected value's type selects the comparison: bool → bool,
// number → numeric, otherwise case-insensitive string compare on the
// stringified stored value.
func metadataEquals(raw, expected any) bool {
	switch exp := expected.(type) {
	case bool:
		b, ok := metadataBool(raw)
		return ok && b == exp
	case float64, float32, int, int64, json.Number:
		ev, _ := toFloat64(expected)
		av, ok := metadataFloat(raw)
		return ok && av == ev
	default:
		return strings.EqualFold(metadataString(raw), metadataString(expected))
	}
}

// metadataString renders a stored metadata value as a string for string
// operators. Scalars render naturally; objects/arrays fall back to their JSON
// encoding so a regex/contains can still match structured values.
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

// metadataFloat coerces a stored metadata value to float64 for numeric
// comparison. Returns false when the value is not numeric.
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

// metadataBool coerces a stored metadata value to bool. Returns false (ok=false)
// when the value isn't a recognizable boolean.
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
	"set_category":    true,
	"add_tag":         true,
	"remove_tag":      true,
	"add_comment":     true,
	"assign_series":   true,
	"set_metadata":    true,
	"remove_metadata": true,
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
			return fmt.Errorf("%w: unknown action type %q (expected set_category|add_tag|remove_tag|add_comment|assign_series|set_metadata|remove_metadata)", ErrInvalidParameter, a.Type)
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
		case "assign_series":
			hasSeries := strings.TrimSpace(a.SeriesShortID) != ""
			hasKey := strings.TrimSpace(a.MerchantKey) != ""
			if hasSeries == hasKey {
				return fmt.Errorf("%w: assign_series requires exactly one of series_short_id or merchant_key", ErrInvalidParameter)
			}
			if hasKey && !a.CreateIfMissing {
				return fmt.Errorf("%w: assign_series with merchant_key requires create_if_missing=true", ErrInvalidParameter)
			}
			if hasSeries {
				if _, err := s.resolveSeriesID(ctx, a.SeriesShortID); err != nil {
					return fmt.Errorf("%w: assign_series series_short_id %q not found", ErrInvalidParameter, a.SeriesShortID)
				}
			}
		case "set_metadata":
			if err := validateMetadataKey(a.MetadataKey); err != nil {
				return fmt.Errorf("%w (set_metadata)", err)
			}
			valBytes, err := json.Marshal(a.MetadataValue)
			if err != nil {
				return fmt.Errorf("%w: set_metadata value is not JSON-serializable: %v", ErrInvalidParameter, err)
			}
			if len(valBytes) > maxMetadataValBytes {
				return fmt.Errorf("%w: set_metadata value exceeds %d bytes", ErrInvalidParameter, maxMetadataValBytes)
			}
		case "remove_metadata":
			if err := validateMetadataKey(a.MetadataKey); err != nil {
				return fmt.Errorf("%w (remove_metadata)", err)
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
		expiresAt = pgconv.Timestamptz(time.Now().Add(dur))
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

	if params.CreatorType != nil && *params.CreatorType != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("tr.created_by_type = $%d", argN))
		args = append(args, *params.CreatorType)
		argN++
	}

	if params.Trigger != nil && *params.Trigger != "" {
		// Normalize the on_update alias to on_change for filtering, but also
		// match rows still stored under the legacy value so neither spelling
		// hides the other.
		trig := *params.Trigger
		if trig == "on_change" || trig == "on_update" {
			whereClauses = append(whereClauses, fmt.Sprintf("tr.trigger IN ($%d, $%d)", argN, argN+1))
			args = append(args, "on_change", "on_update")
			argN += 2
		} else {
			whereClauses = append(whereClauses, fmt.Sprintf("tr.trigger = $%d", argN))
			args = append(args, trig)
			argN++
		}
	}

	if params.OnlyUnused {
		whereClauses = append(whereClauses, "tr.hit_count = 0")
	} else if params.MinHitCount != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("tr.hit_count >= $%d", argN))
		args = append(args, *params.MinHitCount)
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

	// Cursor-based pagination (API/MCP). The cursor key is (created_at, id), so
	// it is only correct for the default created-at-descending ordering. When
	// the caller asks for an explicit sort (hit_count, last_hit_at, name,
	// priority), we return a single top-N page and emit no next_cursor rather
	// than paginate against a key that doesn't match the ORDER BY.
	cursorEligible := params.SortBy == ""
	if cursorEligible && params.Cursor != "" {
		cursorTime, cursorIDStr, err := decodeTimestampCursor(params.Cursor)
		if err != nil {
			return nil, ErrInvalidCursor
		}
		cursorUUID, err := pgconv.ParseUUID(cursorIDStr)
		if err != nil {
			return nil, ErrInvalidCursor
		}
		cursorClause := fmt.Sprintf("(tr.created_at, tr.id) < ($%d, $%d)", argN, argN+1)
		if whereSQL == "" {
			whereSQL = " WHERE " + cursorClause
		} else {
			whereSQL += " AND " + cursorClause
		}
		args = append(args, pgconv.Timestamptz(cursorTime), cursorUUID)
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
	if cursorEligible && hasMore && len(scanned) > 0 {
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
			expiresAt = pgconv.Timestamptz(t)
		}
	} else if existing.ExpiresAt != nil {
		t, _ := time.Parse(time.RFC3339, *existing.ExpiresAt)
		expiresAt = pgconv.Timestamptz(t)
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

	ruleID, _ := pgconv.ParseUUID(id) // already validated by GetTransactionRule above

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
// `category_override = 'none'` guard so overridden rows keep their user-pinned
// category.
const transactionContextQuery = `SELECT t.id, t.provider_name, COALESCE(t.provider_merchant_name, ''), t.amount,
	COALESCE(t.provider_category_primary, ''), COALESCE(t.provider_category_detailed, ''),
	t.pending, bc.provider, t.account_id::text, COALESCE(u.id::text, ''), COALESCE(u.name, ''),
	COALESCE(array_agg(DISTINCT tag.slug) FILTER (WHERE tag.slug IS NOT NULL), ARRAY[]::text[]),
	COALESCE(rs.short_id, ''), (t.series_id IS NOT NULL), t.metadata
	FROM transactions t
	JOIN accounts a ON t.account_id = a.id
	JOIN bank_connections bc ON a.connection_id = bc.id
	LEFT JOIN users u ON bc.user_id = u.id
	LEFT JOIN transaction_tags tt ON tt.transaction_id = t.id
	LEFT JOIN tags tag ON tag.id = tt.tag_id
	LEFT JOIN recurring_series rs ON rs.id = t.series_id
	WHERE t.deleted_at IS NULL
	AND (a.is_dependent_linked = FALSE OR NOT EXISTS (SELECT 1 FROM transaction_matches tm WHERE tm.dependent_transaction_id = t.id))`

// transactionContextGroupBy is the GROUP BY clause matching transactionContextQuery.
const transactionContextGroupBy = ` GROUP BY t.id, t.provider_name, t.provider_merchant_name, t.amount, t.provider_category_primary, t.provider_category_detailed, t.pending, bc.provider, t.account_id, u.id, u.name, rs.short_id, t.series_id, t.metadata`

// transactionContextColumns is the number of columns selected by
// transactionContextQuery (and bound by scanTransactionContextRow). Callers size
// their scan-dest slice with this so adding a column is a single-line change.
const transactionContextColumns = 15

// transactionContextRow holds a scanned transaction row for rule evaluation.
type transactionContextRow struct {
	id          pgtype.UUID
	tctx        TransactionContext
	metadataRaw []byte // raw JSONB; unmarshalled into tctx.Metadata by finalize
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
	dest[12] = &r.tctx.SeriesShortID
	dest[13] = &r.tctx.InSeries
	dest[14] = &r.metadataRaw
	return r
}

// finalize unmarshals the raw metadata JSONB into tctx.Metadata. Call after
// rows.Scan. A malformed blob leaves Metadata nil (metadata conditions simply
// won't match) rather than failing the whole apply pass.
func (r *transactionContextRow) finalize() {
	if len(r.metadataRaw) == 0 {
		return
	}
	var meta map[string]any
	if err := json.Unmarshal(r.metadataRaw, &meta); err == nil {
		r.tctx.Metadata = meta
	}
}

// retroTxnIntent is the net, resolved set of state-mutating rule actions to
// materialize against ONE transaction during a retroactive apply. Both
// retroactive paths — ApplyRuleRetroactively (a single rule) and
// ApplyAllRulesRetroactively (the whole rule set) — reduce their matches to
// this shape and hand it to applyRetroTxnIntent, so the two can never again
// diverge on which actions they materialize. That asymmetry is exactly what
// previously dropped assign_series from the bulk path.
//
// add_comment is deliberately NOT a field here: a comment is narration authored
// at the moment a rule fires during sync, not durable transaction state.
// Replaying it across a bulk historical backfill would manufacture thousands of
// misleading "comment" annotations, so every retroactive path skips it. (The
// sync-time resolver in internal/sync/rule_resolver.go still materializes
// add_comment — retroactive apply is the one place it's intentionally dropped.)
//
// Adding a new state-mutating action (e.g. the P1 set_field family) means
// teaching this one struct + applyRetroTxnIntent; both paths inherit it.
type retroTxnIntent struct {
	txnID pgtype.UUID

	// set_category — last-writer-wins. catID.Valid gates the write; catRule is
	// the winning rule (for the category_set annotation).
	catID   pgtype.UUID
	catSlug string
	catRule ruleapply.Rule

	// add_tag / remove_tag — net-diffed; value is the owning rule for audit.
	tagAdds    map[string]ruleapply.Rule
	tagRemoves map[string]ruleapply.Rule

	// assign_series — last-writer-wins; nil means no assignment.
	series     *RuleAction
	seriesRule ruleapply.Rule

	// set_metadata / remove_metadata — net-diffed, disjoint by construction.
	metadataSet    map[string]any
	metadataRemove []string

	// ruleApplied drives the generic rule_applied audit annotations. Each path
	// populates this with its own granularity (per-action for the single-rule
	// path, per-matching-rule for the bulk path); the materializer just emits.
	ruleApplied []retroRuleApplied
}

// retroRuleApplied is one rule_applied audit annotation to emit for a txn.
type retroRuleApplied struct {
	rule  ruleapply.Rule
	field string // may be "" for a rule-level (action-less) audit
	value string
}

// applyRetroTxnIntent materializes one transaction's net retroactive intent
// within tx. It is THE shared writer for every state-mutating rule action
// (set_category, add_tag, remove_tag, assign_series, set_metadata,
// remove_metadata), so coverage cannot drift between the retroactive paths.
//
// All writes use ruleapply.AppliedByRetroactive — this function is the
// retroactive lane by construction.
func (s *Service) applyRetroTxnIntent(ctx context.Context, tx pgx.Tx, it *retroTxnIntent) error {
	// set_category — guarded by category_override='none' (P3 removes the guard).
	if it.catID.Valid {
		if _, err := tx.Exec(ctx,
			`UPDATE transactions SET category_id = $1, updated_at = NOW()
			WHERE id = $2 AND category_override = 'none' AND deleted_at IS NULL`,
			it.catID, it.txnID); err != nil {
			return fmt.Errorf("update transaction category: %w", err)
		}
		if err := ruleapply.WriteCategorySet(ctx, tx, it.txnID, it.catRule, it.catSlug, ruleapply.AppliedByRetroactive); err != nil {
			return fmt.Errorf("annotate category_set: %w", err)
		}
	}

	for slug, r := range it.tagAdds {
		if _, err := s.materializeRuleTagAdd(ctx, tx, it.txnID, slug, r.ID, r.ShortID, r.Name); err != nil {
			return err
		}
	}
	for slug, r := range it.tagRemoves {
		if _, err := s.materializeRuleTagRemove(ctx, tx, it.txnID, slug, r.ID, r.ShortID, r.Name); err != nil {
			return err
		}
	}

	// assign_series — same resolve-or-mint + back-link the sync path uses, so
	// retroactive and sync-time behave identically.
	if it.series != nil {
		if err := s.AssignSeriesFromRuleTx(ctx, tx, it.txnID, it.series.SeriesShortID, it.series.MerchantKey, it.series.CreateIfMissing); err != nil {
			return fmt.Errorf("apply assign_series retroactively: %w", err)
		}
	}

	// set_metadata / remove_metadata — one UPDATE merges the net intent.
	if len(it.metadataSet) > 0 || len(it.metadataRemove) > 0 {
		if err := applyMetadataToTxns(ctx, tx, []pgtype.UUID{it.txnID}, it.metadataSet, it.metadataRemove); err != nil {
			return err
		}
	}

	for _, ra := range it.ruleApplied {
		if err := ruleapply.WriteRuleApplied(ctx, tx, it.txnID, ra.rule, ra.field, ra.value, ruleapply.AppliedByRetroactive); err != nil {
			return fmt.Errorf("annotate rule_applied: %w", err)
		}
	}
	return nil
}

// ApplyRuleRetroactively applies a single rule to all existing non-deleted
// transactions matching its condition. Materialization flows through the shared
// applyRetroTxnIntent, so it covers every state-mutating action — set_category
// (skipped on category_override<>'none' rows), add_tag, remove_tag,
// assign_series, set_metadata, remove_metadata. add_comment stays sync-only by
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
	var seriesAssign *RuleAction
	metadataSet := map[string]any{}
	var metadataRemove []string
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
		case "assign_series":
			aCopy := a
			seriesAssign = &aCopy
		case "set_metadata":
			if a.MetadataKey == "" {
				continue
			}
			if i := indexExactStr(metadataRemove, a.MetadataKey); i >= 0 {
				metadataRemove = append(metadataRemove[:i], metadataRemove[i+1:]...)
			}
			metadataSet[a.MetadataKey] = a.MetadataValue
		case "remove_metadata":
			if a.MetadataKey == "" {
				continue
			}
			// Last-writer-wins: a remove after a set of the same key must delete
			// it. Drop any queued set and queue the delete; the SQL
			// `metadata - keys` is a harmless no-op on rows lacking the key.
			delete(metadataSet, a.MetadataKey)
			if indexExactStr(metadataRemove, a.MetadataKey) < 0 {
				metadataRemove = append(metadataRemove, a.MetadataKey)
			}
		}
	}
	hasWriteAction := categorySetCatID.Valid || len(tagAdds) > 0 || len(tagRemoves) > 0 || seriesAssign != nil ||
		len(metadataSet) > 0 || len(metadataRemove) > 0
	if !hasWriteAction {
		return 0, fmt.Errorf("%w: rule has no applicable actions", ErrInvalidParameter)
	}

	ruleUUID, _ := pgconv.ParseUUID(ruleID)
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
			dest := make([]any, transactionContextColumns)
			r := scanTransactionContextRow(dest)
			if err := rows.Scan(dest...); err != nil {
				rows.Close()
				return totalMatched, fmt.Errorf("scan transaction: %w", err)
			}
			r.finalize()
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

		// Materialize through the shared per-txn applier so this path covers the
		// exact same action set as the bulk ApplyAllRulesRetroactively path. The
		// rule's net intent (category/tags/series/metadata) is uniform across
		// matched rows, so the per-txn intent differs only by txnID.
		appliedRule := ruleapply.Rule{ID: ruleUUID, ShortID: ruleShortID, Name: ruleName}

		// rule_applied audit shape is identical for every matched txn: one entry
		// per non-comment action intent. add_comment is skipped retroactively
		// (see retroTxnIntent).
		var ruleApplied []retroRuleApplied
		for _, action := range rule.Actions {
			field, value := actionAuditFields(action)
			if field == "" || field == "comment" {
				continue
			}
			ruleApplied = append(ruleApplied, retroRuleApplied{rule: appliedRule, field: field, value: value})
		}

		for _, txnID := range matchIDs {
			it := retroTxnIntent{
				txnID:          txnID,
				catID:          categorySetCatID,
				catSlug:        categorySetSlug,
				catRule:        appliedRule,
				tagAdds:        make(map[string]ruleapply.Rule, len(tagAdds)),
				tagRemoves:     make(map[string]ruleapply.Rule, len(tagRemoves)),
				series:         seriesAssign,
				seriesRule:     appliedRule,
				metadataSet:    metadataSet,
				metadataRemove: metadataRemove,
				ruleApplied:    ruleApplied,
			}
			for _, slug := range tagAdds {
				it.tagAdds[slug] = appliedRule
			}
			for _, slug := range tagRemoves {
				it.tagRemoves[slug] = appliedRule
			}
			if err := s.applyRetroTxnIntent(ctx, tx, &it); err != nil {
				tx.Rollback(ctx)
				return totalMatched, err
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
	case "remove_tag":
		// "tag_remove" mirrors the sync resolver's audit field for removals
		// (internal/sync/rule_resolver.go), keeping retroactive and sync-time
		// rule_applied annotations consistent.
		return "tag_remove", a.TagSlug
	case "add_comment":
		return "comment", a.Content
	case "assign_series":
		if a.SeriesShortID != "" {
			return "series", a.SeriesShortID
		}
		return "series", a.MerchantKey
	case "set_metadata":
		return "metadata", a.MetadataKey
	case "remove_metadata":
		return "metadata_remove", a.MetadataKey
	}
	return "", ""
}

// applyMetadataToTxns merges a rule's net metadata intent into every listed
// transaction in one UPDATE, sharing the caller's tx. set keys overwrite,
// remove keys delete; the two are disjoint by resolver/extraction construction
// so `(metadata - removeKeys) || setObj` applies both unambiguously.
func applyMetadataToTxns(ctx context.Context, tx pgx.Tx, txnIDs []pgtype.UUID, set map[string]any, remove []string) error {
	setJSON := "{}"
	if len(set) > 0 {
		b, err := json.Marshal(set)
		if err != nil {
			return fmt.Errorf("marshal rule metadata set: %w", err)
		}
		setJSON = string(b)
	}
	removeKeys := remove
	if removeKeys == nil {
		removeKeys = []string{}
	}
	if _, err := tx.Exec(ctx,
		`UPDATE transactions
		   SET metadata = (metadata - $1::text[]) || $2::jsonb, updated_at = NOW()
		 WHERE id = ANY($3) AND deleted_at IS NULL`,
		removeKeys, setJSON, txnIDs); err != nil {
		return fmt.Errorf("apply rule metadata: %w", err)
	}
	return nil
}

// indexExactStr returns the index of target in slice using exact
// (case-sensitive) comparison, or -1. Metadata keys are case-sensitive JSONB
// keys, so they must not be folded.
func indexExactStr(slice []string, target string) int {
	for i, s := range slice {
		if s == target {
			return i
		}
	}
	return -1
}

// ApplyAllRulesRetroactively applies all active rules to existing transactions
// in pipeline-stage order (priority ASC, created_at ASC). Per transaction it
// folds the rule set to a net intent and materializes it through the shared
// applyRetroTxnIntent, covering every state-mutating action — set_category
// (last-writer-wins), add_tag, remove_tag, assign_series, set_metadata,
// remove_metadata. add_comment stays sync-only.
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
		uuid, _ := pgconv.ParseUUID(r.ID)
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
		// Metadata: last-writer-wins per key, net-diff set↔remove.
		type txnIntent struct {
			txnID          pgtype.UUID
			catID          pgtype.UUID
			catSlug        string
			catRule        *compiledRule
			tagAdds        map[string]*compiledRule // slug → first rule that added it
			tagRemoves     map[string]*compiledRule
			series         *RuleAction     // net assign_series intent (last-writer-wins)
			seriesRule     *compiledRule   // rule that owns the winning series assignment
			metadataSet    map[string]any  // key → net value to write
			metadataRemove []string        // net keys to delete
			matchingRules  []*compiledRule // every rule whose condition matched (for rule_applied)
		}
		var intents []txnIntent
		rowCount := 0
		for rows.Next() {
			rowCount++
			dest := make([]any, transactionContextColumns)
			r := scanTransactionContextRow(dest)
			if err := rows.Scan(dest...); err != nil {
				rows.Close()
				return hitCounts, fmt.Errorf("scan transaction: %w", err)
			}
			r.finalize()
			lastID = r.id
			tctx := r.tctx
			if len(tctx.Tags) > 0 {
				cp := make([]string, len(tctx.Tags))
				copy(cp, tctx.Tags)
				tctx.Tags = cp
			}
			intent := txnIntent{
				txnID:       r.id,
				tagAdds:     make(map[string]*compiledRule),
				tagRemoves:  make(map[string]*compiledRule),
				metadataSet: make(map[string]any),
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
					case "set_metadata":
						if a.MetadataKey == "" {
							continue
						}
						if i := indexExactStr(intent.metadataRemove, a.MetadataKey); i >= 0 {
							intent.metadataRemove = append(intent.metadataRemove[:i], intent.metadataRemove[i+1:]...)
						}
						intent.metadataSet[a.MetadataKey] = a.MetadataValue
						if tctx.Metadata == nil {
							tctx.Metadata = make(map[string]any)
						}
						tctx.Metadata[a.MetadataKey] = a.MetadataValue
					case "remove_metadata":
						if a.MetadataKey == "" {
							continue
						}
						// Last-writer-wins: a remove after a set of the same
						// key must delete it. Drop any queued set and queue
						// the delete; `metadata - keys` is a no-op on rows
						// lacking the key.
						delete(intent.metadataSet, a.MetadataKey)
						if indexExactStr(intent.metadataRemove, a.MetadataKey) < 0 {
							intent.metadataRemove = append(intent.metadataRemove, a.MetadataKey)
						}
						delete(tctx.Metadata, a.MetadataKey)
					case "assign_series":
						if a.SeriesShortID == "" && a.MerchantKey == "" {
							continue
						}
						// Last-writer-wins: a txn joins at most one series, so a
						// later-stage rule overrides an earlier assignment.
						aCopy := a
						intent.series = &aCopy
						intent.seriesRule = cr
					case "add_comment":
						// Narration authored at sync time; intentionally never
						// replayed on a historical backfill (see retroTxnIntent).
					}
				}
			}
			if intent.catRule != nil || len(intent.tagAdds) > 0 || len(intent.tagRemoves) > 0 ||
				intent.series != nil || len(intent.metadataSet) > 0 || len(intent.metadataRemove) > 0 {
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

		// Materialize each txn's net intent through the shared per-txn applier so
		// this path covers the exact same action set as ApplyRuleRetroactively —
		// including assign_series, which this bulk path previously dropped.
		//
		// rule_applied audit: one annotation per matching rule per txn, without
		// per-action specialization (action_field / action_value left empty) —
		// callers get a rule-level audit. Shape still matches the canonical
		// 5-key payload.
		for i := range intents {
			it := &intents[i]
			ri := retroTxnIntent{
				txnID:          it.txnID,
				tagAdds:        make(map[string]ruleapply.Rule, len(it.tagAdds)),
				tagRemoves:     make(map[string]ruleapply.Rule, len(it.tagRemoves)),
				metadataSet:    it.metadataSet,
				metadataRemove: it.metadataRemove,
			}
			if it.catRule != nil {
				ri.catID = it.catID
				ri.catSlug = it.catSlug
				ri.catRule = ruleapply.Rule{ID: it.catRule.uuid, ShortID: it.catRule.shortID, Name: it.catRule.name}
			}
			for slug, cr := range it.tagAdds {
				ri.tagAdds[slug] = ruleapply.Rule{ID: cr.uuid, ShortID: cr.shortID, Name: cr.name}
			}
			for slug, cr := range it.tagRemoves {
				ri.tagRemoves[slug] = ruleapply.Rule{ID: cr.uuid, ShortID: cr.shortID, Name: cr.name}
			}
			if it.series != nil {
				ri.series = it.series
				ri.seriesRule = ruleapply.Rule{ID: it.seriesRule.uuid, ShortID: it.seriesRule.shortID, Name: it.seriesRule.name}
			}
			for _, cr := range it.matchingRules {
				ri.ruleApplied = append(ri.ruleApplied, retroRuleApplied{
					rule: ruleapply.Rule{ID: cr.uuid, ShortID: cr.shortID, Name: cr.name},
				})
			}
			if err := s.applyRetroTxnIntent(ctx, tx, &ri); err != nil {
				tx.Rollback(ctx)
				return hitCounts, fmt.Errorf("apply-all retroactive batch: %w", err)
			}
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
		id, err := pgconv.ParseUUID(ruleID)
		if err != nil {
			continue
		}
		_, _ = s.Pool.Exec(ctx, "UPDATE transaction_rules SET hit_count = hit_count + $2, last_hit_at = NOW() WHERE id = $1", id, count)
	}

	return hitCounts, nil
}

// RulePreviewMatch contains a sample transaction that matched a rule preview.
type RulePreviewMatch struct {
	TransactionID           string  `json:"transaction_id"`
	ProviderName            string  `json:"provider_name"`
	Amount                  float64 `json:"amount"`
	Date                    string  `json:"date"`
	ProviderCategoryPrimary string  `json:"provider_category_primary"`
	CurrentCategorySlug     string  `json:"current_category_slug,omitempty"`
}

// RulePreviewResult contains the results of a rule preview/dry-run.
type RulePreviewResult struct {
	MatchCount    int64              `json:"match_count"`
	TotalScanned  int64              `json:"total_scanned"`
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

// FindMatchingRulesParams selects what to evaluate the active rule set against.
// Exactly one of TransactionID or Merchant must be supplied.
type FindMatchingRulesParams struct {
	// TransactionID (UUID or short_id) anchors the lookup to a real stored
	// transaction — every condition field (amount, category, tags, provider…)
	// is evaluated against the row's true values.
	TransactionID string
	// Merchant is free text used to build a *synthetic* context with only the
	// name fields populated (provider_name + provider_merchant_name). Rules
	// that condition on amount/category/tags won't match a synthetic context,
	// so this answers the narrower question "is this merchant already covered
	// by a name-based rule?" — the common dedup case.
	Merchant string
}

// MatchingRule is one active rule whose condition matched the evaluated
// transaction context. The shape is deliberately compact — the agent needs to
// know whether a merchant is already covered and by what, not the full rule.
type MatchingRule struct {
	ShortID string `json:"short_id"`
	Name    string `json:"name"`
	// SetsCategory is the slug of the category the rule's first set_category
	// action assigns, or nil when the rule sets no category (e.g. a tag-only
	// or comment-only rule).
	SetsCategory *string `json:"sets_category,omitempty"`
	Trigger      string  `json:"trigger"`
	Priority     int     `json:"priority"`
	Enabled      bool    `json:"enabled"`
	HitCount     int     `json:"hit_count"`
	// MatchAll flags a rule with no conditions (NULL conditions = match-all,
	// e.g. the seeded needs-review tagger). These match every transaction, so
	// surface them distinctly to avoid reading them as merchant coverage.
	MatchAll bool `json:"match_all"`
}

// FindMatchingRulesResult wraps the matched rules with a count for quick
// "already covered?" checks.
type FindMatchingRulesResult struct {
	MatchedCount int            `json:"matched_count"`
	Rules        []MatchingRule `json:"rules"`
}

// FindMatchingRules evaluates every active, non-expired rule against a single
// transaction context and returns those whose condition matches — the inverse
// of PreviewRule (which evaluates one condition against many transactions).
//
// This is the efficient answer to "is this merchant already handled by a rule"
// without loading the entire rule set into an agent's context: all rules are
// compiled and evaluated in-process (O(rules), microseconds for hundreds of
// rules) and only the handful of matches are returned. Rules evaluate in
// pipeline-stage order (priority ASC), so the returned slice is ordered the
// same way the sync resolver would apply them.
//
// Trigger is NOT filtered — a rule is reported if its *condition* matches,
// regardless of on_create/on_change, since the caller wants coverage, not a
// sync-time simulation.
func (s *Service) FindMatchingRules(ctx context.Context, params FindMatchingRulesParams) (*FindMatchingRulesResult, error) {
	hasTxn := strings.TrimSpace(params.TransactionID) != ""
	hasMerchant := strings.TrimSpace(params.Merchant) != ""
	if hasTxn == hasMerchant {
		return nil, fmt.Errorf("%w: provide exactly one of transaction_id or merchant", ErrInvalidParameter)
	}

	// Build the context to evaluate against.
	var tctx TransactionContext
	if hasTxn {
		loaded, err := s.loadTransactionContext(ctx, params.TransactionID)
		if err != nil {
			return nil, err
		}
		tctx = *loaded
	} else {
		// Synthetic context: only the name fields are known. Rule conditions on
		// other fields simply won't match — which is the correct, conservative
		// behavior for a name-only coverage check.
		m := strings.TrimSpace(params.Merchant)
		tctx = TransactionContext{Name: m, MerchantName: m}
	}

	rules, err := s.ListActiveRulesForSync(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active rules: %w", err)
	}

	result := &FindMatchingRulesResult{Rules: []MatchingRule{}}
	for _, r := range rules {
		compiled, err := CompileCondition(r.Conditions)
		if err != nil {
			// A rule with an uncompilable condition can't have matched at sync
			// time either; skip it rather than failing the whole lookup.
			continue
		}
		if !EvaluateCondition(compiled, tctx) {
			continue
		}
		result.Rules = append(result.Rules, MatchingRule{
			ShortID:      r.ShortID,
			Name:         r.Name,
			SetsCategory: r.CategorySlug,
			Trigger:      r.Trigger,
			Priority:     r.Priority,
			Enabled:      r.Enabled,
			HitCount:     r.HitCount,
			MatchAll:     compiled == nil,
		})
	}
	result.MatchedCount = len(result.Rules)
	return result, nil
}

// loadTransactionContext resolves a transaction id/short_id and loads its full
// rule-evaluation context (same JOINs + tag aggregation as the retroactive
// apply path) so every condition field evaluates against the row's real values.
func (s *Service) loadTransactionContext(ctx context.Context, idOrShort string) (*TransactionContext, error) {
	txnUUID, err := s.resolveTransactionID(ctx, idOrShort)
	if err != nil {
		return nil, err
	}

	query := transactionContextQuery + " AND t.id = $1" + transactionContextGroupBy + " LIMIT 1"
	rows, err := s.Pool.Query(ctx, query, txnUUID)
	if err != nil {
		return nil, fmt.Errorf("query transaction context: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("query transaction context: %w", err)
		}
		return nil, fmt.Errorf("%w: transaction not found", ErrNotFound)
	}
	dest := make([]any, transactionContextColumns)
	r := scanTransactionContextRow(dest)
	if err := rows.Scan(dest...); err != nil {
		return nil, fmt.Errorf("scan transaction context: %w", err)
	}
	r.finalize()
	return &r.tctx, nil
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
	// - category_override = 'none' (rules don't overwrite manual overrides)
	// - exclude matched dependent transactions (dedup'd via account links)
	baseQuery := `SELECT t.id, t.provider_name, COALESCE(t.provider_merchant_name, ''), t.amount,
		COALESCE(t.provider_category_primary, ''), COALESCE(t.provider_category_detailed, ''),
		t.pending, bc.provider, t.account_id::text, COALESCE(u.id::text, ''), COALESCE(u.name, ''),
		t.date, COALESCE(c.slug, ''), COALESCE(rs.short_id, ''), (t.series_id IS NOT NULL), t.metadata
		FROM transactions t
		JOIN accounts a ON t.account_id = a.id
		JOIN bank_connections bc ON a.connection_id = bc.id
		LEFT JOIN users u ON bc.user_id = u.id
		LEFT JOIN categories c ON t.category_id = c.id
		LEFT JOIN recurring_series rs ON rs.id = t.series_id
		WHERE t.deleted_at IS NULL AND t.category_override = 'none'
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
				id          pgtype.UUID
				tctx        TransactionContext
				date        pgtype.Date
				catSlug     string
				metadataRaw []byte
			)
			if err := rows.Scan(&id, &tctx.Name, &tctx.MerchantName, &tctx.Amount,
				&tctx.CategoryPrimary, &tctx.CategoryDetailed,
				&tctx.Pending, &tctx.Provider, &tctx.AccountID, &tctx.UserID, &tctx.UserName,
				&date, &catSlug, &tctx.SeriesShortID, &tctx.InSeries, &metadataRaw); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan transaction: %w", err)
			}
			if len(metadataRaw) > 0 {
				var meta map[string]any
				if err := json.Unmarshal(metadataRaw, &meta); err == nil {
					tctx.Metadata = meta
				}
			}
			lastID = id
			result.TotalScanned++

			if EvaluateCondition(compiled, tctx) {
				result.MatchCount++
				if len(result.SampleMatches) < sampleSize {
					match := RulePreviewMatch{
						TransactionID:           formatUUID(id),
						ProviderName:            tctx.Name,
						Amount:                  tctx.Amount,
						ProviderCategoryPrimary: tctx.CategoryPrimary,
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
		id, err := pgconv.ParseUUID(ruleID)
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
	id            pgtype.UUID
	shortID       string
	name          string
	conditions    []byte // NULL -> nil -> match-all
	actions       []byte
	trigger       string
	priority      int32
	enabled       bool
	expiresAt     pgtype.Timestamptz
	createdByType string
	createdByID   pgtype.Text
	createdByName string
	hitCount      int32
	lastHitAt     pgtype.Timestamptz
	createdAt     pgtype.Timestamptz
	updatedAt     pgtype.Timestamptz
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
			slugCopy := cat.Slug
			nameCopy := cat.DisplayName
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
		slug, displayName string
		icon, color       *string
	}
	cache := make(map[string]catInfo, len(seen))
	for slug := range seen {
		if cat, err := s.GetCategoryBySlug(ctx, slug); err == nil {
			cache[slug] = catInfo{
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
				slugCopy := info.slug
				nameCopy := info.displayName
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
	return pgconv.ParseUUID(cat.ID)
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

	var unitDur time.Duration
	switch unit {
	case "h":
		unitDur = time.Hour
	case "d":
		unitDur = 24 * time.Hour
	case "w":
		unitDur = 7 * 24 * time.Hour
	default:
		return 0, fmt.Errorf("unknown duration unit %q (use h, d, or w)", unit)
	}

	// Guard against int64 nanosecond overflow: a large-but-Atoi-valid number
	// (e.g. "200000d") would silently wrap to a negative time.Duration, making
	// expires_at land in the *past* — the rule would be created already-expired,
	// the opposite of the long expiry requested. Reject it with a clear error.
	const maxInt64 = 1<<63 - 1
	if int64(num) > maxInt64/int64(unitDur) {
		return 0, fmt.Errorf("duration too large: %s exceeds the maximum supported expiry (~292 years)", s)
	}
	return time.Duration(num) * unitDur, nil
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
	case "assign_series":
		return "Assign to series"
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
	TotalApplications  int64  `json:"total_applications"`
	UniqueTransactions int64  `json:"unique_transactions"`
	FirstAppliedAt     string `json:"first_applied_at,omitempty"`
	LastAppliedAt      string `json:"last_applied_at,omitempty"`
	SyncApplications   int64  `json:"sync_applications"`
	RetroApplications  int64  `json:"retro_applications"`
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

	query := `SELECT ann.id, ann.transaction_id, t.provider_name, t.amount, t.date,
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
		cursorUUID, err := pgconv.ParseUUID(cursorIDStr)
		if err != nil {
			return nil, false, ErrInvalidCursor
		}
		query += fmt.Sprintf(" AND (ann.created_at, ann.id) < ($%d, $%d)", argN, argN+1)
		args = append(args, pgconv.Timestamptz(cursorTime), cursorUUID)
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
