//go:build !lite

package api

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"breadbox/internal/app"
	mw "breadbox/internal/middleware"
	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// --- Request envelopes ---

type createAgentRequest struct {
	Name         string   `json:"name"`
	Slug         string   `json:"slug"`
	Prompt       string   `json:"prompt"`
	SystemPrompt *string  `json:"system_prompt"`
	ScheduleCron *string  `json:"schedule_cron"`
	ToolScope    string   `json:"tool_scope"`
	AllowedTools []string `json:"allowed_tools"`
	Model        string   `json:"model"`
	MaxTurns     int      `json:"max_turns"`
	MaxBudgetUSD *float64 `json:"max_budget_usd"`
	Enabled      bool     `json:"enabled"`
}

type updateAgentRequest struct {
	Name         *string   `json:"name"`
	Slug         *string   `json:"slug"`
	Prompt       *string   `json:"prompt"`
	SystemPrompt *string   `json:"system_prompt"`
	ScheduleCron *string   `json:"schedule_cron"`
	ToolScope    *string   `json:"tool_scope"`
	AllowedTools *[]string `json:"allowed_tools"`
	Model        *string   `json:"model"`
	MaxTurns     *int      `json:"max_turns"`
	MaxBudgetUSD *float64  `json:"max_budget_usd"`
	Enabled      *bool     `json:"enabled"`
}

type updateAgentSettingsRequest struct {
	AuthMode           *string  `json:"auth_mode"`
	SubscriptionToken  *string  `json:"subscription_token"`
	AnthropicAPIKey    *string  `json:"anthropic_api_key"`
	MaxConcurrent      *int     `json:"max_concurrent"`
	GlobalMaxBudgetUSD *float64 `json:"global_max_budget_usd"`
	RuntimePath        *string  `json:"runtime_path"`
	TranscriptDir      *string  `json:"transcript_dir"`
}

// --- Handlers: definitions ---

// ListAgentDefinitionsHandler returns all agents with last_run inlined.
func ListAgentDefinitionsHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := svc.ListAgentDefinitions(r.Context())
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list agents")
			return
		}
		writeData(w, out)
	}
}

// GetAgentDefinitionHandler resolves by slug/short_id/UUID.
func GetAgentDefinitionHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		def, err := svc.GetAgentDefinition(r.Context(), slug)
		if err != nil {
			writeAgentError(w, err, "agent not found")
			return
		}
		writeData(w, def)
	}
}

// CreateAgentDefinitionHandler creates a new agent.
func CreateAgentDefinitionHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createAgentRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		out, err := svc.CreateAgentDefinition(r.Context(), service.CreateAgentDefinitionParams{
			Name:         req.Name,
			Slug:         req.Slug,
			Prompt:       req.Prompt,
			SystemPrompt: req.SystemPrompt,
			ScheduleCron: req.ScheduleCron,
			ToolScope:    req.ToolScope,
			AllowedTools: req.AllowedTools,
			Model:        req.Model,
			MaxTurns:     req.MaxTurns,
			MaxBudgetUSD: req.MaxBudgetUSD,
			Enabled:      req.Enabled,
		})
		if err != nil {
			if errors.Is(err, service.ErrInvalidParameter) {
				mw.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
				return
			}
			if strings.Contains(strings.ToLower(err.Error()), "duplicate key") || strings.Contains(strings.ToLower(err.Error()), "unique constraint") {
				mw.WriteError(w, http.StatusConflict, "CONFLICT", "An agent with this slug already exists")
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create agent")
			return
		}
		w.WriteHeader(http.StatusCreated)
		writeData(w, out)
	}
}

// UpdateAgentDefinitionHandler patches an existing agent.
func UpdateAgentDefinitionHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		var req updateAgentRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		out, err := svc.UpdateAgentDefinition(r.Context(), slug, service.UpdateAgentDefinitionParams{
			Name:         req.Name,
			Slug:         req.Slug,
			Prompt:       req.Prompt,
			SystemPrompt: req.SystemPrompt,
			ScheduleCron: req.ScheduleCron,
			ToolScope:    req.ToolScope,
			AllowedTools: req.AllowedTools,
			Model:        req.Model,
			MaxTurns:     req.MaxTurns,
			MaxBudgetUSD: req.MaxBudgetUSD,
			Enabled:      req.Enabled,
		})
		if err != nil {
			writeAgentError(w, err, "agent not found")
			return
		}
		writeData(w, out)
	}
}

// DeleteAgentDefinitionHandler deletes an agent (runs preserved).
func DeleteAgentDefinitionHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		if err := svc.DeleteAgentDefinition(r.Context(), slug); err != nil {
			writeAgentError(w, err, "agent not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// EnableAgentHandler flips enabled=true.
func EnableAgentHandler(svc *service.Service) http.HandlerFunc {
	return setAgentEnabled(svc, true)
}

// DisableAgentHandler flips enabled=false.
func DisableAgentHandler(svc *service.Service) http.HandlerFunc {
	return setAgentEnabled(svc, false)
}

func setAgentEnabled(svc *service.Service, enabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		out, err := svc.SetAgentDefinitionEnabled(r.Context(), slug, enabled)
		if err != nil {
			writeAgentError(w, err, "agent not found")
			return
		}
		writeData(w, out)
	}
}

// --- Handlers: runs ---

// ListAgentRunsHandler returns paginated runs for one agent.
func ListAgentRunsHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		q := r.URL.Query()
		limit, err := parseIntParam(q, "limit", 50, 1, 200)
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			return
		}
		offset, err := parseIntParam(q, "offset", 0, 0, 1<<20)
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			return
		}
		out, err := svc.ListAgentRuns(r.Context(), slug, limit, offset)
		if err != nil {
			writeAgentError(w, err, "agent not found")
			return
		}
		writeData(w, out)
	}
}

// GetAgentRunHandler resolves by short_id or UUID.
func GetAgentRunHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "shortId")
		out, err := svc.GetAgentRun(r.Context(), id)
		if err != nil {
			writeAgentError(w, err, "run not found")
			return
		}
		writeData(w, out)
	}
}

// GetAgentRunTranscriptHandler streams the NDJSON transcript file.
func GetAgentRunTranscriptHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "shortId")
		run, err := svc.GetAgentRun(r.Context(), id)
		if err != nil {
			writeAgentError(w, err, "run not found")
			return
		}
		if run.TranscriptPath == nil || *run.TranscriptPath == "" {
			mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Transcript not available for this run")
			return
		}
		f, err := os.Open(*run.TranscriptPath)
		if err != nil {
			mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Transcript file missing on disk")
			return
		}
		defer f.Close()
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		// Best-effort copy; partial reads are OK for a viewer.
		buf := make([]byte, 32*1024)
		for {
			n, rerr := f.Read(buf)
			if n > 0 {
				if _, werr := w.Write(buf[:n]); werr != nil {
					return
				}
			}
			if rerr != nil {
				return
			}
		}
	}
}

// --- Handlers: settings ---

// GetAgentSettingsHandler returns the agent.* config with masked tokens.
func GetAgentSettingsHandler(svc *service.Service, a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := svc.GetAgentSettings(r.Context(), a.Config.EncryptionKey)
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to read agent settings")
			return
		}
		writeData(w, out)
	}
}

// UpdateAgentSettingsHandler applies a PATCH-style update.
func UpdateAgentSettingsHandler(svc *service.Service, a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req updateAgentSettingsRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		out, err := svc.UpdateAgentSettings(r.Context(), service.UpdateAgentSettingsParams{
			AuthMode:           req.AuthMode,
			SubscriptionToken:  req.SubscriptionToken,
			AnthropicAPIKey:    req.AnthropicAPIKey,
			MaxConcurrent:      req.MaxConcurrent,
			GlobalMaxBudgetUSD: req.GlobalMaxBudgetUSD,
			RuntimePath:        req.RuntimePath,
			TranscriptDir:      req.TranscriptDir,
		}, a.Config.EncryptionKey)
		if err != nil {
			if errors.Is(err, service.ErrInvalidParameter) {
				mw.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
				return
			}
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update agent settings")
			return
		}
		writeData(w, out)
	}
}

// writeAgentError maps a service error to the JSON error envelope.
func writeAgentError(w http.ResponseWriter, err error, notFoundMsg string) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", notFoundMsg)
	case errors.Is(err, service.ErrInvalidParameter):
		mw.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Unexpected error")
	}
}
