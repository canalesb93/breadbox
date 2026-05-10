package api

import (
	"errors"
	"net/http"

	mw "breadbox/internal/middleware"
	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// ListRulesHandler returns a filtered, paginated list of transaction rules.
func ListRulesHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		limit, err := parseIntParam(q, "limit", 50, 1, 500)
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			return
		}

		enabled, err := parseBoolParam(q, "enabled")
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			return
		}

		params := service.TransactionRuleListParams{
			Limit:        limit,
			Cursor:       q.Get("cursor"),
			CategorySlug: parseOptionalStringParam(q, "category_slug"),
			Enabled:      enabled,
			Search:       parseOptionalStringParam(q, "search"),
		}

		result, err := svc.ListTransactionRules(r.Context(), params)
		if err != nil {
			if errors.Is(err, service.ErrInvalidCursor) {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "Invalid cursor")
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list rules")
			return
		}

		writeData(w, result)
	}
}

// CreateRuleHandler creates a new transaction rule.
func CreateRuleHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Name         string               `json:"name"`
			Conditions   *service.Condition    `json:"conditions"`
			Actions      []service.RuleAction  `json:"actions"`
			CategorySlug string                `json:"category_slug"`
			Trigger      string                `json:"trigger"`
			// Stage is the semantic alias for priority:
			//   baseline=0, standard=10, refinement=50, override=100.
			// If both stage and priority are supplied, priority wins.
			Stage     string `json:"stage"`
			Priority  int    `json:"priority"`
			ExpiresIn string `json:"expires_in"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}

		if input.Name == "" {
			mw.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "name is required")
			return
		}
		if len(input.Actions) == 0 && input.CategorySlug == "" {
			mw.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "either actions or category_slug is required")
			return
		}

		actor := service.ActorFromContext(r.Context())

		params := service.CreateTransactionRuleParams{
			Name:         input.Name,
			Actions:      input.Actions,
			CategorySlug: input.CategorySlug,
			Trigger:      input.Trigger,
			Priority:     input.Priority,
			Stage:        input.Stage,
			ExpiresIn:    input.ExpiresIn,
			Actor:        actor,
		}
		if input.Conditions != nil {
			params.Conditions = *input.Conditions
		}

		rule, err := svc.CreateTransactionRule(r.Context(), params)
		if err != nil {
			if errors.Is(err, service.ErrInvalidParameter) {
				mw.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
				return
			}
			if errors.Is(err, service.ErrCategoryNotFound) {
				mw.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Category not found")
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create rule")
			return
		}

		writeJSON(w, http.StatusCreated, rule)
	}
}

// GetRuleHandler returns a single transaction rule by ID.
func GetRuleHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		rule, err := svc.GetTransactionRule(r.Context(), id)
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Rule not found")
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get rule")
			return
		}

		writeData(w, rule)
	}
}

// UpdateRuleHandler updates a transaction rule by ID.
func UpdateRuleHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var input struct {
			Name         *string               `json:"name,omitempty"`
			Conditions   *service.Condition     `json:"conditions,omitempty"`
			Actions      *[]service.RuleAction  `json:"actions,omitempty"`
			CategorySlug *string                `json:"category_slug,omitempty"`
			Trigger      *string                `json:"trigger,omitempty"`
			// Stage is the semantic alias for priority. If both are supplied, priority wins.
			Stage     *string `json:"stage,omitempty"`
			Priority  *int    `json:"priority,omitempty"`
			Enabled   *bool   `json:"enabled,omitempty"`
			ExpiresAt *string `json:"expires_at,omitempty"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}

		rule, err := svc.UpdateTransactionRule(r.Context(), id, service.UpdateTransactionRuleParams{
			Name:         input.Name,
			Conditions:   input.Conditions,
			Actions:      input.Actions,
			CategorySlug: input.CategorySlug,
			Trigger:      input.Trigger,
			Priority:     input.Priority,
			Stage:        input.Stage,
			Enabled:      input.Enabled,
			ExpiresAt:    input.ExpiresAt,
		})
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Rule not found")
				return
			}
			if errors.Is(err, service.ErrInvalidParameter) {
				mw.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update rule")
			return
		}

		writeData(w, rule)
	}
}

// DeleteRuleHandler deletes a transaction rule by ID.
func DeleteRuleHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		if err := svc.DeleteTransactionRule(r.Context(), id); err != nil {
			if errors.Is(err, service.ErrNotFound) {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Rule not found")
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete rule")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// ApplyRuleHandler applies a single rule retroactively to existing transactions.
func ApplyRuleHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		count, err := svc.ApplyRuleRetroactively(r.Context(), id)
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Rule not found")
				return
			}
			if errors.Is(err, service.ErrInvalidParameter) {
				mw.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to apply rule")
			return
		}

		writeData(w, map[string]any{
			"rule_id":        id,
			"affected_count": count,
		})
	}
}

// ApplyAllRulesHandler applies all active rules retroactively.
func ApplyAllRulesHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		results, err := svc.ApplyAllRulesRetroactively(r.Context())
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to apply rules")
			return
		}

		var totalAffected int64
		for _, count := range results {
			totalAffected += count
		}

		writeData(w, map[string]any{
			"rules_applied":  results,
			"total_affected": totalAffected,
		})
	}
}

// batchCreateRulesRequest mirrors the MCP `batch_create_rules` tool's input
// shape 1:1 (snake_case, identical field names).
type batchCreateRulesRequest struct {
	Rules   []batchCreateRuleItem `json:"rules"`
	OnError string                `json:"on_error"`
}

// batchCreateRuleItem is a single rule create payload, mirroring the
// per-item shape used by both the single-create REST handler and the MCP
// batch_create_rules tool.
type batchCreateRuleItem struct {
	Name         string               `json:"name"`
	Conditions   *service.Condition   `json:"conditions"`
	Actions      []service.RuleAction `json:"actions"`
	CategorySlug string               `json:"category_slug"`
	Trigger      string               `json:"trigger"`
	Stage        string               `json:"stage"`
	Priority     int                  `json:"priority"`
	ExpiresIn    string               `json:"expires_in"`
}

// batchCreateRuleResult is a single per-op outcome.
type batchCreateRuleResult struct {
	Index   int                          `json:"index"`
	Status  string                       `json:"status"` // "ok" or "error"
	RuleID  string                       `json:"rule_id,omitempty"`
	Rule    *service.TransactionRuleResponse `json:"rule,omitempty"`
	Error   *batchCreateRuleResultError  `json:"error,omitempty"`
}

type batchCreateRuleResultError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// BatchCreateRulesHandler is the REST sibling of the MCP `batch_create_rules`
// tool. Up to 50 rules per call. Per-op errors live inside `results[]`; the
// top-level call returns 200 unless the input itself is malformed (empty/
// oversized rules array), in which case it returns `400 INVALID_PARAMETER`.
//
// In `on_error=continue` (default), each rule's failure is isolated. In
// `on_error=abort`, the first failure wraps the previously-created rules in a
// rollback by deleting them and returns the partial-results envelope with
// `aborted=true`.
//
// POST /rules/batch
func BatchCreateRulesHandler(svc *service.Service) http.HandlerFunc {
	const maxBatchSize = 50

	return func(w http.ResponseWriter, r *http.Request) {
		var input batchCreateRulesRequest
		if !decodeJSON(w, r, &input) {
			return
		}

		if len(input.Rules) == 0 {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "rules array is required and must not be empty")
			return
		}
		if len(input.Rules) > maxBatchSize {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER",
				"too many rules in batch (max 50)")
			return
		}

		onError := input.OnError
		if onError == "" {
			onError = "continue"
		}
		if onError != "continue" && onError != "abort" {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "on_error must be 'continue' or 'abort'")
			return
		}

		actor := service.ActorFromContext(r.Context())

		results := make([]batchCreateRuleResult, 0, len(input.Rules))
		var createdIDs []string
		succeeded := 0
		failed := 0
		aborted := false

		for i, item := range input.Rules {
			res := batchCreateRuleResult{Index: i, Status: "ok"}

			if item.Name == "" {
				failed++
				res.Status = "error"
				res.Error = &batchCreateRuleResultError{
					Code:    "VALIDATION_ERROR",
					Message: "name is required",
				}
				results = append(results, res)
				if onError == "abort" {
					aborted = true
					break
				}
				continue
			}
			if len(item.Actions) == 0 && item.CategorySlug == "" {
				failed++
				res.Status = "error"
				res.Error = &batchCreateRuleResultError{
					Code:    "VALIDATION_ERROR",
					Message: "either actions or category_slug is required",
				}
				results = append(results, res)
				if onError == "abort" {
					aborted = true
					break
				}
				continue
			}

			params := service.CreateTransactionRuleParams{
				Name:         item.Name,
				Actions:      item.Actions,
				CategorySlug: item.CategorySlug,
				Trigger:      item.Trigger,
				Priority:     item.Priority,
				Stage:        item.Stage,
				ExpiresIn:    item.ExpiresIn,
				Actor:        actor,
			}
			if item.Conditions != nil {
				params.Conditions = *item.Conditions
			}

			rule, err := svc.CreateTransactionRule(r.Context(), params)
			if err != nil {
				failed++
				res.Status = "error"
				code, msg := batchCreateRuleErrorMapping(err)
				res.Error = &batchCreateRuleResultError{Code: code, Message: msg}
				results = append(results, res)
				if onError == "abort" {
					aborted = true
					break
				}
				continue
			}

			succeeded++
			res.RuleID = rule.ID
			res.Rule = rule
			results = append(results, res)
			createdIDs = append(createdIDs, rule.ID)
		}

		// Abort mode: roll back any rules created before the failure so the
		// batch is all-or-nothing.
		if aborted && len(createdIDs) > 0 {
			for _, id := range createdIDs {
				_ = svc.DeleteTransactionRule(r.Context(), id)
			}
			succeeded = 0
		}

		payload := map[string]any{
			"results":   results,
			"succeeded": succeeded,
			"failed":    failed,
		}
		if aborted {
			payload["aborted"] = true
		}
		writeData(w, payload)
	}
}

// batchCreateRuleErrorMapping translates a service-layer error from
// CreateTransactionRule into the per-op error envelope's (code, message).
func batchCreateRuleErrorMapping(err error) (string, string) {
	switch {
	case errors.Is(err, service.ErrInvalidParameter):
		return "VALIDATION_ERROR", err.Error()
	case errors.Is(err, service.ErrCategoryNotFound):
		return "VALIDATION_ERROR", "Category not found"
	default:
		return "INTERNAL_ERROR", err.Error()
	}
}

// GetRuleSyncHistoryHandler returns recent sync runs that triggered this rule
// (last N rows from sync_logs where rule_hits contains this rule's UUID).
//
// GET /rules/{id}/sync-history?limit=10
func GetRuleSyncHistoryHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		limit, err := parseIntParam(r.URL.Query(), "limit", 10, 1, 100)
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			return
		}

		// Resolve via Get to enforce existence + accept short_id. The service
		// helper returns the canonical UUID-form ID on the response, which is
		// the key sync_logs.rule_hits is indexed by.
		rule, err := svc.GetTransactionRule(r.Context(), id)
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Rule not found")
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get rule")
			return
		}

		history, err := svc.GetRuleSyncHistory(r.Context(), rule.ID, limit)
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get rule sync history")
			return
		}
		if history == nil {
			history = []map[string]any{}
		}

		writeData(w, map[string]any{"history": history})
	}
}

// PreviewRuleHandler previews a rule's conditions against existing transactions.
func PreviewRuleHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Conditions service.Condition `json:"conditions"`
			SampleSize int               `json:"sample_size"`
		}
		if !decodeJSON(w, r, &input) {
			return
		}

		result, err := svc.PreviewRule(r.Context(), input.Conditions, input.SampleSize)
		if err != nil {
			if errors.Is(err, service.ErrInvalidParameter) {
				mw.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to preview rule")
			return
		}

		writeData(w, result)
	}
}
