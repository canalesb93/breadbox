//go:build !headless && !lite

package webui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"breadbox/internal/admin"
	"breadbox/internal/app"
	"breadbox/internal/db"
	mw "breadbox/internal/middleware"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// MountBackupRoutes attaches /web/v1/backups/* under an already-mounted,
// already session-gated chi.Router. Routes are admin-only — anyone else gets
// 403. Backups are infrastructure-tier; the standard editor/viewer split is
// not strict enough.
func MountBackupRoutes(r chi.Router, a *app.App, sm *scs.SessionManager) {
	r.Group(func(r chi.Router) {
		r.Use(requireAdminJSON(sm))
		r.Get("/backups", BackupsListHandler(a))
		r.Post("/backups", CreateBackupHandler(a))
		r.Put("/backups/schedule", UpdateBackupScheduleHandler(a))
		r.Get("/backups/{filename}/download", DownloadBackupHandler(a))
		r.Delete("/backups/{filename}", DeleteBackupHandler(a))
		r.Post("/backups/{filename}/restore", RestoreExistingBackupHandler(a))
		r.Post("/backups/restore", RestoreUploadedBackupHandler(a))
	})
}

func requireAdminJSON(sm *scs.SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !admin.IsAdmin(sm, r) {
				mw.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Admin role required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// BackupRow is the wire shape for one backup entry.
type BackupRow struct {
	Filename      string `json:"filename"`
	Size          int64  `json:"size"`
	SizeFormatted string `json:"size_formatted"`
	CreatedAt     string `json:"created_at"`
	Trigger       string `json:"trigger"`
}

// BackupStatus is the at-a-glance summary the page renders above the list.
type BackupStatus struct {
	ServiceAvailable bool   `json:"service_available"`
	HasEncryptionKey bool   `json:"has_encryption_key"`
	BackupCount      int    `json:"backup_count"`
	TotalSizeBytes   int64  `json:"total_size_bytes"`
	TotalSize        string `json:"total_size"`
	Schedule         string `json:"schedule"`
	RetentionDays    int    `json:"retention_days"`
	BackupDir        string `json:"backup_dir"`
	DatabaseName     string `json:"database_name"`
	PreflightOK      bool   `json:"preflight_ok"`
	PreflightMessage string `json:"preflight_message"`
}

// BackupsListResponse is the GET /web/v1/backups payload.
type BackupsListResponse struct {
	Status  BackupStatus `json:"status"`
	Backups []BackupRow  `json:"backups"`
}

// BackupsListHandler returns the full overview the page needs in one round
// trip — status + the file list. Avoids a second fetch dance from the SPA.
func BackupsListHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		status := BackupStatus{
			HasEncryptionKey: len(a.Config.EncryptionKey) > 0,
			DatabaseName:     service.ParseDatabaseName(a.Config.DatabaseURL),
		}

		if a.BackupService == nil {
			status.PreflightMessage = "Backup service is not available. pg_dump may not be installed."
			mw.WriteJSON(w, http.StatusOK, BackupsListResponse{Status: status, Backups: []BackupRow{}})
			return
		}

		status.ServiceAvailable = true
		status.BackupDir = a.BackupService.BackupDir()
		status.Schedule = a.Service.GetBackupSchedule(ctx)
		status.RetentionDays = a.Service.GetBackupRetentionDays(ctx)

		preflight := a.BackupService.Preflight(ctx)
		status.PreflightOK = preflight.OK
		status.PreflightMessage = preflight.Message

		infos, err := a.BackupService.ListBackups()
		if err != nil {
			a.Logger.Error("list backups", "error", err)
			mw.WriteError(w, http.StatusInternalServerError, "LIST_FAILED", "Failed to list backups")
			return
		}

		var total int64
		rows := make([]BackupRow, 0, len(infos))
		for _, b := range infos {
			total += b.Size
			rows = append(rows, BackupRow{
				Filename:      b.Filename,
				Size:          b.Size,
				SizeFormatted: service.FormatBytes(b.Size),
				CreatedAt:     b.CreatedAt.UTC().Format(time.RFC3339),
				Trigger:       b.Trigger,
			})
		}
		status.BackupCount = len(rows)
		status.TotalSizeBytes = total
		status.TotalSize = service.FormatBytes(total)

		mw.WriteJSON(w, http.StatusOK, BackupsListResponse{Status: status, Backups: rows})
	}
}

// CreateBackupHandler triggers a manual backup.
func CreateBackupHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.BackupService == nil {
			mw.WriteError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Backup service is not available")
			return
		}
		filename, err := a.BackupService.CreateBackup(r.Context(), "manual")
		if err != nil {
			a.Logger.Error("create backup", "error", err)
			mw.WriteError(w, http.StatusInternalServerError, "CREATE_FAILED", err.Error())
			return
		}
		mw.WriteJSON(w, http.StatusCreated, map[string]string{"filename": filename})
	}
}

// DeleteBackupHandler removes a backup file.
func DeleteBackupHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.BackupService == nil {
			mw.WriteError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Backup service is not available")
			return
		}
		filename := chi.URLParam(r, "filename")
		if err := a.BackupService.DeleteBackup(filename); err != nil {
			a.Logger.Error("delete backup", "error", err, "filename", filename)
			mw.WriteError(w, http.StatusBadRequest, "DELETE_FAILED", err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// DownloadBackupHandler streams a .sql.gz file back to the browser.
func DownloadBackupHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.BackupService == nil {
			mw.WriteError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Backup service is not available")
			return
		}
		filename := chi.URLParam(r, "filename")
		fullPath, err := a.BackupService.GetBackupPath(filename)
		if err != nil {
			mw.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Backup file not found")
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
		http.ServeFile(w, r, fullPath)
	}
}

// RestoreExistingBackupHandler restores from a stored backup file.
func RestoreExistingBackupHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.BackupService == nil {
			mw.WriteError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Backup service is not available")
			return
		}
		filename := chi.URLParam(r, "filename")
		if err := a.BackupService.RestoreBackup(r.Context(), filename); err != nil {
			a.Logger.Error("restore backup", "error", err, "filename", filename)
			mw.WriteError(w, http.StatusInternalServerError, "RESTORE_FAILED", err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// RestoreUploadedBackupHandler accepts a multipart .sql.gz upload and pipes
// it through psql. Capped at 500MB to match the v1 handler.
func RestoreUploadedBackupHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.BackupService == nil {
			mw.WriteError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Backup service is not available")
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 500<<20)

		file, header, err := r.FormFile("backup_file")
		if err != nil {
			mw.WriteError(w, http.StatusBadRequest, "MISSING_FILE", "No backup file provided")
			return
		}
		defer file.Close()

		if !strings.HasSuffix(header.Filename, ".sql.gz") {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_FILE", "Only .sql.gz files are supported")
			return
		}

		if err := a.BackupService.RestoreFromReader(r.Context(), file); err != nil {
			a.Logger.Error("restore from upload", "error", err)
			mw.WriteError(w, http.StatusInternalServerError, "RESTORE_FAILED", err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// UpdateBackupScheduleRequest is the PUT body for /web/v1/backups/schedule.
type UpdateBackupScheduleRequest struct {
	Schedule      string `json:"schedule"`
	RetentionDays int    `json:"retention_days"`
}

var validBackupSchedules = map[string]struct{}{
	"":          {}, // disabled
	"daily_2am": {},
	"daily_3am": {},
	"daily_4am": {},
	"weekly":    {},
}

// UpdateBackupScheduleHandler persists the schedule + retention settings.
func UpdateBackupScheduleHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req UpdateBackupScheduleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			mw.WriteError(w, http.StatusBadRequest, "INVALID_BODY", "Request body must be valid JSON")
			return
		}

		if _, ok := validBackupSchedules[req.Schedule]; !ok {
			mw.WriteError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Invalid backup schedule")
			return
		}
		if req.RetentionDays < 1 || req.RetentionDays > 365 {
			mw.WriteError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Retention must be between 1 and 365 days")
			return
		}

		ctx := r.Context()
		if err := saveBackupSchedule(ctx, a.Queries, req.Schedule, req.RetentionDays); err != nil {
			a.Logger.Error("save backup schedule", "error", err)
			mw.WriteError(w, http.StatusInternalServerError, "SAVE_FAILED", "Failed to save backup settings")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func saveBackupSchedule(ctx context.Context, q *db.Queries, schedule string, retentionDays int) error {
	if err := q.SetAppConfig(ctx, db.SetAppConfigParams{
		Key:   "backup_schedule",
		Value: pgconv.Text(schedule),
	}); err != nil {
		return errors.New("set backup_schedule: " + err.Error())
	}
	return q.SetAppConfig(ctx, db.SetAppConfigParams{
		Key:   "backup_retention_days",
		Value: pgconv.Text(fmt.Sprintf("%d", retentionDays)),
	})
}
