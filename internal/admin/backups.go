package admin

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"breadbox/internal/app"
	"breadbox/internal/db"
	"breadbox/internal/pgconv"
	"breadbox/internal/service"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// BackupsPageHandler serves GET /backups — the backup management page.
func BackupsPageHandler(a *app.App, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		if a.BackupService == nil {
			data := BaseTemplateData(r, sm, "backups", "Backups")
			data["Error"] = "Backup service is not available. pg_dump may not be installed."
			props := pages.BackupsProps{
				CSRFToken: GetCSRFToken(r),
				Error:     "Backup service is not available. pg_dump may not be installed.",
			}
			renderBackups(w, r, tr, data, props)
			return
		}

		backups, err := a.BackupService.ListBackups()
		if err != nil {
			a.Logger.Error("list backups", "error", err)
			backups = nil
		}

		totalSize, _ := a.BackupService.TotalBackupSize()

		schedule := a.Service.GetBackupSchedule(ctx)
		retentionDays := a.Service.GetBackupRetentionDays(ctx)
		preflight := a.BackupService.Preflight(ctx)

		data := BaseTemplateData(r, sm, "backups", "Backups")
		data["Backups"] = backups
		data["BackupCount"] = len(backups)
		data["TotalSize"] = service.FormatBytes(totalSize)
		data["Schedule"] = schedule
		data["RetentionDays"] = retentionDays
		data["HasEncryptionKey"] = len(a.Config.EncryptionKey) > 0
		data["DatabaseName"] = service.ParseDatabaseName(a.Config.DatabaseURL)
		data["BackupDir"] = a.BackupService.BackupDir()
		data["PreflightOK"] = preflight.OK
		data["PreflightMessage"] = preflight.Message

		props := pages.BackupsProps{
			CSRFToken:        GetCSRFToken(r),
			HasEncryptionKey: len(a.Config.EncryptionKey) > 0,
			BackupCount:      len(backups),
			TotalSize:        service.FormatBytes(totalSize),
			Schedule:         schedule,
			RetentionDays:    retentionDays,
			BackupDir:        a.BackupService.BackupDir(),
			PreflightOK:      preflight.OK,
			PreflightMessage: preflight.Message,
			Backups:          make([]pages.BackupRow, 0, len(backups)),
		}
		for _, b := range backups {
			props.Backups = append(props.Backups, pages.BackupRow{
				Filename:      b.Filename,
				SizeFormatted: service.FormatBytes(b.Size),
				CreatedAtRel:  relativeTime(b.CreatedAt),
				Trigger:       b.Trigger,
				DownloadHref:  "/-/backups/" + b.Filename + "/download",
				RestoreAction: "/-/backups/" + b.Filename + "/restore",
				DeleteAction:  "/-/backups/" + b.Filename + "/delete",
			})
		}

		renderBackups(w, r, tr, data, props)
	}
}

// renderBackups mirrors the renderLogs / renderCSVImport pattern: hands
// the typed BackupsProps to the templ component and uses RenderWithTempl
// to host it inside base.html.
func renderBackups(w http.ResponseWriter, r *http.Request, tr *TemplateRenderer, data map[string]any, props pages.BackupsProps) {
	tr.RenderWithTempl(w, r, data, pages.Backups(props))
}

// CreateBackupHandler serves POST /-/backups/create — triggers a manual backup.
func CreateBackupHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		if a.BackupService == nil {
			SetFlash(ctx, sm, "error", "Backup service is not available.")
			http.Redirect(w, r, "/backups", http.StatusSeeOther)
			return
		}

		filename, err := a.BackupService.CreateBackup(ctx, "manual")
		if err != nil {
			a.Logger.Error("create backup", "error", err)
			SetFlash(ctx, sm, "error", "Failed to create backup: "+err.Error())
			http.Redirect(w, r, "/backups", http.StatusSeeOther)
			return
		}

		SetFlash(ctx, sm, "success", fmt.Sprintf("Backup created: %s", filename))
		http.Redirect(w, r, "/backups", http.StatusSeeOther)
	}
}

// DownloadBackupHandler serves GET /-/backups/{filename}/download.
func DownloadBackupHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.BackupService == nil {
			http.Error(w, "Backup service not available", http.StatusServiceUnavailable)
			return
		}

		filename := chi.URLParam(r, "filename")
		fullPath, err := a.BackupService.GetBackupPath(filename)
		if err != nil {
			http.Error(w, "Backup not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
		http.ServeFile(w, r, fullPath)
	}
}

// DeleteBackupHandler serves DELETE /-/backups/{filename}.
func DeleteBackupHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		if a.BackupService == nil {
			SetFlash(ctx, sm, "error", "Backup service not available.")
			http.Redirect(w, r, "/backups", http.StatusSeeOther)
			return
		}

		filename := chi.URLParam(r, "filename")
		if err := a.BackupService.DeleteBackup(filename); err != nil {
			a.Logger.Error("delete backup", "error", err, "filename", filename)
			SetFlash(ctx, sm, "error", "Failed to delete backup.")
			http.Redirect(w, r, "/backups", http.StatusSeeOther)
			return
		}

		SetFlash(ctx, sm, "success", fmt.Sprintf("Backup deleted: %s", filename))
		http.Redirect(w, r, "/backups", http.StatusSeeOther)
	}
}

// RestoreBackupHandler serves POST /-/backups/restore — restores from an uploaded file.
func RestoreBackupHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		if a.BackupService == nil {
			SetFlash(ctx, sm, "error", "Backup service not available.")
			http.Redirect(w, r, "/backups", http.StatusSeeOther)
			return
		}

		// Limit upload to 500MB.
		r.Body = http.MaxBytesReader(w, r.Body, 500<<20)

		source := r.FormValue("restore_source")

		switch source {
		case "upload":
			file, header, err := r.FormFile("backup_file")
			if err != nil {
				SetFlash(ctx, sm, "error", "No backup file provided.")
				http.Redirect(w, r, "/backups", http.StatusSeeOther)
				return
			}
			defer file.Close()

			if !strings.HasSuffix(header.Filename, ".sql.gz") {
				SetFlash(ctx, sm, "error", "Invalid file type. Only .sql.gz files are supported.")
				http.Redirect(w, r, "/backups", http.StatusSeeOther)
				return
			}

			if err := a.BackupService.RestoreFromReader(ctx, file); err != nil {
				a.Logger.Error("restore from upload", "error", err)
				SetFlash(ctx, sm, "error", "Restore failed: "+err.Error())
				http.Redirect(w, r, "/backups", http.StatusSeeOther)
				return
			}

			SetFlash(ctx, sm, "success", "Database restored successfully from uploaded file. You may need to restart the server.")

		case "existing":
			filename := r.FormValue("backup_filename")
			if filename == "" {
				SetFlash(ctx, sm, "error", "No backup file selected.")
				http.Redirect(w, r, "/backups", http.StatusSeeOther)
				return
			}

			if err := a.BackupService.RestoreBackup(ctx, filename); err != nil {
				a.Logger.Error("restore from existing", "error", err, "filename", filename)
				SetFlash(ctx, sm, "error", "Restore failed: "+err.Error())
				http.Redirect(w, r, "/backups", http.StatusSeeOther)
				return
			}

			SetFlash(ctx, sm, "success", fmt.Sprintf("Database restored from %s. You may need to restart the server.", filename))

		default:
			SetFlash(ctx, sm, "error", "Invalid restore source.")
		}

		http.Redirect(w, r, "/backups", http.StatusSeeOther)
	}
}

// BackupScheduleHandler serves POST /-/backups/schedule — saves backup schedule settings.
func BackupScheduleHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		schedule := r.FormValue("backup_schedule")
		retentionStr := r.FormValue("backup_retention_days")

		// Validate schedule.
		validSchedules := map[string]bool{
			"":          true, // disabled
			"daily_2am": true,
			"daily_3am": true,
			"daily_4am": true,
			"weekly":    true,
		}
		if !validSchedules[schedule] {
			SetFlash(ctx, sm, "error", "Invalid backup schedule.")
			http.Redirect(w, r, "/backups", http.StatusSeeOther)
			return
		}

		// Validate retention.
		retentionDays, err := strconv.Atoi(retentionStr)
		if err != nil || retentionDays < 1 || retentionDays > 365 {
			SetFlash(ctx, sm, "error", "Invalid retention period. Must be 1-365 days.")
			http.Redirect(w, r, "/backups", http.StatusSeeOther)
			return
		}

		// Save schedule.
		if err := a.Queries.SetAppConfig(ctx, db.SetAppConfigParams{
			Key:   "backup_schedule",
			Value: pgconv.Text(schedule),
		}); err != nil {
			a.Logger.Error("save backup schedule", "error", err)
			SetFlash(ctx, sm, "error", "Failed to save backup schedule.")
			http.Redirect(w, r, "/backups", http.StatusSeeOther)
			return
		}

		// Save retention.
		if err := a.Queries.SetAppConfig(ctx, db.SetAppConfigParams{
			Key:   "backup_retention_days",
			Value: pgtype.Text{String: strconv.Itoa(retentionDays), Valid: true},
		}); err != nil {
			a.Logger.Error("save backup retention", "error", err)
			SetFlash(ctx, sm, "error", "Failed to save retention setting.")
			http.Redirect(w, r, "/backups", http.StatusSeeOther)
			return
		}

		if schedule == "" {
			SetFlash(ctx, sm, "success", "Scheduled backups disabled.")
		} else {
			SetFlash(ctx, sm, "success", fmt.Sprintf("Backup schedule saved. Retention: %d days.", retentionDays))
		}
		http.Redirect(w, r, "/backups", http.StatusSeeOther)
	}
}

// RestoreExistingBackupHandler serves POST /-/backups/{filename}/restore.
func RestoreExistingBackupHandler(a *app.App, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		if a.BackupService == nil {
			SetFlash(ctx, sm, "error", "Backup service not available.")
			http.Redirect(w, r, "/backups", http.StatusSeeOther)
			return
		}

		filename := chi.URLParam(r, "filename")
		if err := a.BackupService.RestoreBackup(ctx, filename); err != nil {
			a.Logger.Error("restore backup", "error", err, "filename", filename)
			SetFlash(ctx, sm, "error", "Restore failed: "+err.Error())
			http.Redirect(w, r, "/backups", http.StatusSeeOther)
			return
		}

		SetFlash(ctx, sm, "success", fmt.Sprintf("Database restored from %s. You may need to restart the server.", filename))
		http.Redirect(w, r, "/backups", http.StatusSeeOther)
	}
}
