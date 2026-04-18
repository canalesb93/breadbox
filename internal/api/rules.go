package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// ListRulesHandler returns a filtered, paginated list of transaction rules.
func ListRulesHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		limit, err := parseIntParam(q, "limit", 50, 1, 500)
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			return
		}

		enabled, err := parseBoolParam(q, "enabled")
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
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
				writeError(w, http.StatusBadRequest, "INVALID_PARAMETER", "Invalid cursor")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list rules")
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
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON body")
			return
		}

		if input.Name == "" {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "name is required")
			return
		}
		if len(input.Actions) == 0 && input.CategorySlug == "" {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "either actions or category_slug is required")
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
				writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
				return
			}
			if errors.Is(err, service.ErrCategoryNotFound) {
				writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Category not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create rule")
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
				writeError(w, http.StatusNotFound, "NOT_FOUND", "Rule not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get rule")
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
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON body")
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
				writeError(w, http.StatusNotFound, "NOT_FOUND", "Rule not found")
				return
			}
			if errors.Is(err, service.ErrInvalidParameter) {
				writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update rule")
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
				writeError(w, http.StatusNotFound, "NOT_FOUND", "Rule not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete rule")
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
				writeError(w, http.StatusNotFound, "NOT_FOUND", "Rule not found")
				return
			}
			if errors.Is(err, service.ErrInvalidParameter) {
				writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to apply rule")
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
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to apply rules")
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

// PreviewRuleHandler previews a rule's conditions against existing transactions.
func PreviewRuleHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Conditions service.Condition `json:"conditions"`
			SampleSize int               `json:"sample_size"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON body")
			return
		}

		result, err := svc.PreviewRule(r.Context(), input.Conditions, input.SampleSize)
		if err != nil {
			if errors.Is(err, service.ErrInvalidParameter) {
				writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to preview rule")
			return
		}

		writeData(w, result)
	}
}
