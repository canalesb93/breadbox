//go:build !lite

package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	mw "breadbox/internal/middleware"
	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// Sync visibility endpoints. These wrap the read-side service methods that
// were previously only exposed through the admin dashboard:
//
//   - GET /sync/logs                — paginated history with filters
//   - GET /sync/logs/{id}           — single log + per-account rows
//   - GET /sync/health              — aggregate stats over the last 24h
//   - GET /sync/health/providers    — per-provider stats (latest sync, counts)
//   - GET /sync/stats               — aggregate stats matching the same
//                                     filter set as /sync/logs (so callers
//                                     can compute success rate / totals over
//                                     a slice)
//
// All read-scope. The service layer is offset/page-based; the REST surface
// presents a cursor envelope by encoding the next page number into an opaque
// base64-JSON cursor (see encodePageCursor / decodePageCursor below). This
// keeps the public API consistent with /transactions, /rules, and friends
// without dragging an offset/page contract into the public surface.

const (
	defaultSyncLogLimit = 50
	maxSyncLogLimit     = 200
)

// syncLogsResponse is the wire shape of GET /sync/logs.
type syncLogsResponse struct {
	SyncLogs   []syncLogResponse `json:"sync_logs"`
	NextCursor *string           `json:"next_cursor"`
	HasMore    bool              `json:"has_more"`
	Limit      int               `json:"limit"`
	Total      int64             `json:"total"`
}

// syncLogResponse mirrors service.SyncLogRow with explicit JSON tags so the
// REST contract is stable independent of any field renames in the service
// layer.
type syncLogResponse struct {
	ID                   string                 `json:"id"`
	ConnectionID         string                 `json:"connection_id"`
	InstitutionName      string                 `json:"institution_name"`
	Provider             string                 `json:"provider,omitempty"`
	Trigger              string                 `json:"trigger"`
	Status               string                 `json:"status"`
	AddedCount           int32                  `json:"added_count"`
	ModifiedCount        int32                  `json:"modified_count"`
	RemovedCount         int32                  `json:"removed_count"`
	UnchangedCount       int32                  `json:"unchanged_count"`
	ErrorMessage         *string                `json:"error_message,omitempty"`
	FriendlyErrorMessage *string                `json:"friendly_error_message,omitempty"`
	WarningMessage       *string                `json:"warning_message,omitempty"`
	StartedAt            *string                `json:"started_at,omitempty"`
	CompletedAt          *string                `json:"completed_at,omitempty"`
	Duration             *string                `json:"duration,omitempty"`
	DurationMs           *int32                 `json:"duration_ms,omitempty"`
	AccountsAffected     int64                  `json:"accounts_affected"`
	RuleHits             []syncLogRuleHit       `json:"rule_hits,omitempty"`
	TotalRuleHits        int                    `json:"total_rule_hits,omitempty"`
	Accounts             []syncLogAccountResp   `json:"accounts,omitempty"`
}

type syncLogRuleHit struct {
	RuleID   string `json:"rule_id"`
	RuleName string `json:"rule_name"`
	Count    int    `json:"count"`
}

type syncLogAccountResp struct {
	ID             string  `json:"id"`
	SyncLogID      string  `json:"sync_log_id"`
	AccountID      *string `json:"account_id,omitempty"`
	AccountName    string  `json:"account_name"`
	AddedCount     int32   `json:"added_count"`
	ModifiedCount  int32   `json:"modified_count"`
	RemovedCount   int32   `json:"removed_count"`
	UnchangedCount int32   `json:"unchanged_count"`
}

type syncHealthResponse struct {
	OverallHealth     string  `json:"overall_health"`
	LastSyncTime      *string `json:"last_sync_time,omitempty"`
	LastSyncStatus    string  `json:"last_sync_status"`
	RecentSyncCount   int64   `json:"recent_sync_count"`
	RecentSuccessRate float64 `json:"recent_success_rate"`
	RecentErrorCount  int64   `json:"recent_error_count"`
	ConnectionErrors  int64   `json:"connection_errors"`
	NextSyncTime      string  `json:"next_sync_time,omitempty"`
}

type syncProviderHealthResponse struct {
	Provider        string  `json:"provider"`
	ConnectionCount int64   `json:"connection_count"`
	AccountCount    int64   `json:"account_count"`
	LastSyncStatus  string  `json:"last_sync_status,omitempty"`
	LastSyncTime    *string `json:"last_sync_time,omitempty"`
	LastSyncError   *string `json:"last_sync_error,omitempty"`
}

type syncStatsResponse struct {
	TotalSyncs     int64   `json:"total_syncs"`
	SuccessCount   int64   `json:"success_count"`
	ErrorCount     int64   `json:"error_count"`
	WarningCount   int64   `json:"warning_count"`
	SuccessRate    float64 `json:"success_rate"`
	AvgDurationMs  float64 `json:"avg_duration_ms"`
	TotalAdded     int64   `json:"total_added"`
	TotalModified  int64   `json:"total_modified"`
	TotalRemoved   int64   `json:"total_removed"`
	TotalUnchanged int64   `json:"total_unchanged"`
}

// pageCursor is the JSON payload encoded into the opaque next_cursor string.
// We bridge cursor-style pagination on the public API to the service layer's
// page/page_size contract by carrying the next page number forward.
type pageCursor struct {
	Page int `json:"p"`
}

func encodePageCursor(page int) string {
	b, _ := json.Marshal(pageCursor{Page: page})
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodePageCursor(s string) (int, error) {
	if s == "" {
		return 1, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return 0, service.ErrInvalidCursor
	}
	var p pageCursor
	if err := json.Unmarshal(raw, &p); err != nil {
		return 0, service.ErrInvalidCursor
	}
	if p.Page < 1 {
		return 0, service.ErrInvalidCursor
	}
	return p.Page, nil
}

// validSyncStatus / validSyncTrigger guard the enum-typed query params. The
// underlying service casts directly to ::sync_status / ::sync_trigger, so
// validating up front keeps Postgres errors out of the response.
func validSyncStatus(s string) bool {
	switch s {
	case "in_progress", "success", "error":
		return true
	}
	return false
}

func validSyncTrigger(s string) bool {
	switch s {
	case "cron", "webhook", "manual", "initial":
		return true
	}
	return false
}

// parseSyncLogFilters extracts the filter set used by /sync/logs and
// /sync/stats. It writes a 400 INVALID_PARAMETER and returns ok=false when
// any param is malformed. The returned struct is *not* fully populated —
// pagination fields (Page, PageSize) are filled in by callers.
func parseSyncLogFilters(w http.ResponseWriter, r *http.Request) (service.SyncLogListParams, bool) {
	q := r.URL.Query()

	params := service.SyncLogListParams{}

	if v := q.Get("connection_id"); v != "" {
		params.ConnectionID = &v
	}

	if v := q.Get("status"); v != "" {
		if !validSyncStatus(v) {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "status must be one of: in_progress, success, error")
			return params, false
		}
		params.Status = &v
	}

	if v := q.Get("trigger"); v != "" {
		if !validSyncTrigger(v) {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "trigger must be one of: cron, webhook, manual, initial")
			return params, false
		}
		params.Trigger = &v
	}

	if v := q.Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "from must be an RFC3339 timestamp")
			return params, false
		}
		params.DateFrom = &t
	}

	if v := q.Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "to must be an RFC3339 timestamp")
			return params, false
		}
		params.DateTo = &t
	}

	if params.DateFrom != nil && params.DateTo != nil && !params.DateFrom.Before(*params.DateTo) {
		mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "from must be before to")
		return params, false
	}

	return params, true
}

// ListSyncLogsHandler returns a paginated, filterable history of sync runs.
// GET /sync/logs
func ListSyncLogsHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		limit, err := parseIntParam(q, "limit", defaultSyncLogLimit, 1, maxSyncLogLimit)
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
			return
		}

		page, err := decodePageCursor(q.Get("cursor"))
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_CURSOR", "The provided cursor is not valid")
			return
		}

		params, ok := parseSyncLogFilters(w, r)
		if !ok {
			return
		}
		params.Page = page
		params.PageSize = limit

		result, err := svc.ListSyncLogsPaginated(r.Context(), params)
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list sync logs")
			return
		}

		out := syncLogsResponse{
			SyncLogs: make([]syncLogResponse, 0, len(result.Logs)),
			Limit:    limit,
			Total:    result.Total,
		}
		for _, row := range result.Logs {
			out.SyncLogs = append(out.SyncLogs, syncLogRowToResponse(row, false))
		}
		hasMore := page < result.TotalPages
		out.HasMore = hasMore
		if hasMore {
			c := encodePageCursor(page + 1)
			out.NextCursor = &c
		}

		writeData(w, out)
	}
}

// GetSyncLogHandler returns a single sync log plus its per-account breakdown.
// GET /sync/logs/{id}
func GetSyncLogHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		row, err := svc.GetSyncLog(r.Context(), id)
		if err != nil {
			if errors.Is(err, service.ErrNotFound) {
				mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Sync log not found")
				return
			}
			// invalid UUID falls through here (fmt.Errorf wrapping, no sentinel)
			mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Sync log not found")
			return
		}

		accounts, err := svc.ListSyncLogAccounts(r.Context(), id)
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list sync log accounts")
			return
		}

		resp := syncLogRowToResponse(*row, true)
		resp.Accounts = make([]syncLogAccountResp, 0, len(accounts))
		for _, a := range accounts {
			resp.Accounts = append(resp.Accounts, syncLogAccountResp{
				ID:             a.ID,
				SyncLogID:      a.SyncLogID,
				AccountID:      a.AccountID,
				AccountName:    a.AccountName,
				AddedCount:     a.AddedCount,
				ModifiedCount:  a.ModifiedCount,
				RemovedCount:   a.RemovedCount,
				UnchangedCount: a.UnchangedCount,
			})
		}

		writeData(w, resp)
	}
}

// SyncHealthHandler returns aggregate sync health over the last 24h.
// GET /sync/health
func SyncHealthHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		summary, err := svc.GetSyncHealthSummary(r.Context())
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get sync health summary")
			return
		}
		writeData(w, syncHealthResponse{
			OverallHealth:     summary.OverallHealth,
			LastSyncTime:      summary.LastSyncTime,
			LastSyncStatus:    summary.LastSyncStatus,
			RecentSyncCount:   summary.RecentSyncCount,
			RecentSuccessRate: summary.RecentSuccessRate,
			RecentErrorCount:  summary.RecentErrorCount,
			ConnectionErrors:  summary.ConnectionErrors,
			NextSyncTime:      summary.NextSyncTime,
		})
	}
}

// SyncProviderHealthHandler returns per-provider health summaries.
// GET /sync/health/providers
func SyncProviderHealthHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		summaries, err := svc.GetProviderHealthSummaries(r.Context())
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get provider health summaries")
			return
		}
		// Marshal as a stable map keyed by provider name. The map value is a
		// concrete REST type so JSON tags drive the wire shape.
		out := make(map[string]syncProviderHealthResponse, len(summaries))
		for k, v := range summaries {
			out[k] = syncProviderHealthResponse{
				Provider:        v.Provider,
				ConnectionCount: v.ConnectionCount,
				AccountCount:    v.AccountCount,
				LastSyncStatus:  v.LastSyncStatus,
				LastSyncTime:    v.LastSyncTime,
				LastSyncError:   v.LastSyncError,
			}
		}
		writeData(w, map[string]any{"providers": out})
	}
}

// SyncStatsHandler returns aggregate stats matching the same filter set as
// /sync/logs. Useful for "out of N matching this filter, X succeeded" UIs.
// GET /sync/stats
func SyncStatsHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		params, ok := parseSyncLogFilters(w, r)
		if !ok {
			return
		}
		stats, err := svc.SyncLogStats(r.Context(), params)
		if err != nil {
			mw.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get sync stats")
			return
		}
		writeData(w, syncStatsResponse{
			TotalSyncs:     stats.TotalSyncs,
			SuccessCount:   stats.SuccessCount,
			ErrorCount:     stats.ErrorCount,
			WarningCount:   stats.WarningCount,
			SuccessRate:    stats.SuccessRate,
			AvgDurationMs:  stats.AvgDurationMs,
			TotalAdded:     stats.TotalAdded,
			TotalModified:  stats.TotalModified,
			TotalRemoved:   stats.TotalRemoved,
			TotalUnchanged: stats.TotalUnchanged,
		})
	}
}

// syncLogRowToResponse converts a service.SyncLogRow to its REST shape. When
// includeRuleHits is true, the per-rule breakdown is included (used by the
// detail endpoint, omitted from the list endpoint where it would balloon
// the payload).
func syncLogRowToResponse(row service.SyncLogRow, includeRuleHits bool) syncLogResponse {
	out := syncLogResponse{
		ID:                   row.ID,
		ConnectionID:         row.ConnectionID,
		InstitutionName:      row.InstitutionName,
		Provider:             row.Provider,
		Trigger:              row.Trigger,
		Status:               row.Status,
		AddedCount:           row.AddedCount,
		ModifiedCount:        row.ModifiedCount,
		RemovedCount:         row.RemovedCount,
		UnchangedCount:       row.UnchangedCount,
		ErrorMessage:         row.ErrorMessage,
		FriendlyErrorMessage: row.FriendlyErrorMessage,
		WarningMessage:       row.WarningMessage,
		StartedAt:            row.StartedAt,
		CompletedAt:          row.CompletedAt,
		Duration:             row.Duration,
		DurationMs:           row.DurationMs,
		AccountsAffected:     row.AccountsAffected,
	}
	if includeRuleHits && len(row.RuleHits) > 0 {
		out.RuleHits = make([]syncLogRuleHit, 0, len(row.RuleHits))
		for _, h := range row.RuleHits {
			out.RuleHits = append(out.RuleHits, syncLogRuleHit{
				RuleID:   h.RuleID,
				RuleName: h.RuleName,
				Count:    h.Count,
			})
		}
		out.TotalRuleHits = row.TotalRuleHits
	}
	return out
}
