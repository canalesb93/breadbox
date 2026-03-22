package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	mw "breadbox/internal/middleware"
	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// ListRulesHandler returns a filtered, paginated list of transaction rules.
func ListRulesHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		limit := 50
		if v := q.Get("limit"); v != "" {
			parsed, err := strconv.Atoi(v)
			if err != nil || parsed < 1 || parsed > 200 {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "limit must be between 1 and 200")
				return
			}
			limit = parsed
		}

		params := service.TransactionRuleListParams{
			Limit:  limit,
			Cursor: q.Get("cursor"),
		}

		if v := q.Get("category_slug"); v != "" {
			params.CategorySlug = &v
		}
		if v := q.Get("enabled"); v != "" {
			b, err := strconv.ParseBool(v)
			if err != nil {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "enabled must be true or false")
				return
			}
			params.Enabled = &b
		}
		if v := q.Get("search"); v != "" {
			params.Search = &v
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
			Name         string            `json:"name"`
			Conditions   service.Condition `json:"conditions"`
			CategorySlug string            `json:"category_slug"`
			Priority     int               `json:"priority"`
			ExpiresIn    string            `json:"expires_in"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON body")
			return
		}

		if input.Name == "" {
			mw.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "name is required")
			return
		}
		if input.CategorySlug == "" {
			mw.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "category_slug is required")
			return
		}

		actor := service.ActorFromContext(r.Context())

		rule, err := svc.CreateTransactionRule(r.Context(), service.CreateTransactionRuleParams{
			Name:         input.Name,
			Conditions:   input.Conditions,
			CategorySlug: input.CategorySlug,
			Priority:     input.Priority,
			ExpiresIn:    input.ExpiresIn,
			Actor:        actor,
		})
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
			Name         *string            `json:"name,omitempty"`
			Conditions   *service.Condition `json:"conditions,omitempty"`
			CategorySlug *string            `json:"category_slug,omitempty"`
			Priority     *int               `json:"priority,omitempty"`
			Enabled      *bool              `json:"enabled,omitempty"`
			ExpiresAt    *string            `json:"expires_at,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON body")
			return
		}

		rule, err := svc.UpdateTransactionRule(r.Context(), id, service.UpdateTransactionRuleParams{
			Name:         input.Name,
			Conditions:   input.Conditions,
			CategorySlug: input.CategorySlug,
			Priority:     input.Priority,
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
