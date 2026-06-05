//go:build !headless && !lite

package admin

import (
	"errors"
	"net/http"
	"strings"

	"breadbox/internal/app"
	"breadbox/internal/cronspec"
	"breadbox/internal/service"
	"breadbox/internal/templates/components/pages"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
)

// scheduleFormConnections loads the connection list for the target picker.
func scheduleFormConnections(svc *service.Service, r *http.Request) []service.ConnectionResponse {
	conns, err := svc.ListConnections(r.Context(), nil)
	if err != nil {
		return nil
	}
	return conns
}

// ScheduleFormPageHandler serves GET /settings/sync/schedules/new and
// /settings/sync/schedules/{shortID}/edit.
func ScheduleFormPageHandler(a *app.App, svc *service.Service, sm *scs.SessionManager, tr *TemplateRenderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		props := pages.ScheduleFormProps{
			CSRFToken:     GetCSRFToken(r),
			Presets:       cronspec.Presets,
			Connections:   scheduleFormConnections(svc, r),
			SelectedConns: map[string]bool{},
			Enabled:       true,
			AppliesToAll:  true,
			PresetKey:     "twice_daily",
		}

		shortID := chi.URLParam(r, "shortID")
		if shortID != "" {
			sched, err := findScheduleByShortID(svc, r, shortID)
			if err != nil {
				FlashRedirect(w, r, sm, "error", "Schedule not found.", "/settings/general#sync")
				return
			}
			props.IsEdit = true
			props.ShortID = sched.ShortID
			props.Name = sched.Name
			props.PresetKey = sched.Preset
			if props.PresetKey == "" {
				props.PresetKey = cronspec.CustomKey
			}
			props.Cron = sched.Cron
			props.AppliesToAll = sched.AppliesToAll
			props.Enabled = sched.Enabled
			props.SelectedConns = scheduleSelectedConns(svc, r, sched.ShortID)
		}

		title := "New sync schedule"
		if props.IsEdit {
			title = "Edit sync schedule"
		}
		tr.RenderWithTempl(w, r, map[string]any{
			"PageTitle":   title,
			"CurrentPage": "settings",
			"CSRFToken":   GetCSRFToken(r),
		}, pages.ScheduleForm(props))
	}
}

// findScheduleByShortID looks up a single schedule via the list (the service
// exposes list + mutations; a single-row read isn't needed elsewhere).
func findScheduleByShortID(svc *service.Service, r *http.Request, shortID string) (service.SyncScheduleView, error) {
	all, err := svc.ListSyncSchedules(r.Context())
	if err != nil {
		return service.SyncScheduleView{}, err
	}
	for _, s := range all {
		if s.ShortID == shortID {
			return s, nil
		}
	}
	return service.SyncScheduleView{}, errors.New("not found")
}

// scheduleSelectedConns returns the set of connection short IDs targeted by a
// schedule, for pre-checking the edit form.
func scheduleSelectedConns(svc *service.Service, r *http.Request, shortID string) map[string]bool {
	out := map[string]bool{}
	ids, err := svc.ListScheduleConnectionShortIDs(r.Context(), shortID)
	if err != nil {
		return out
	}
	for _, id := range ids {
		out[id] = true
	}
	return out
}

// ScheduleCreateHandler serves POST /settings/sync/schedules.
func ScheduleCreateHandler(a *app.App, svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		in, err := parseScheduleForm(r)
		if err != nil {
			FlashRedirect(w, r, sm, "error", err.Error(), "/settings/sync/schedules/new")
			return
		}
		if _, err := svc.CreateSyncSchedule(r.Context(), in); err != nil {
			FlashRedirect(w, r, sm, "error", "Failed to create schedule: "+err.Error(), "/settings/sync/schedules/new")
			return
		}
		reloadScheduler(a, r)
		FlashRedirect(w, r, sm, "success", "Schedule created.", "/settings/general#sync")
	}
}

// ScheduleUpdateHandler serves POST /settings/sync/schedules/{shortID}.
func ScheduleUpdateHandler(a *app.App, svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		shortID := chi.URLParam(r, "shortID")
		in, err := parseScheduleForm(r)
		if err != nil {
			FlashRedirect(w, r, sm, "error", err.Error(), "/settings/sync/schedules/"+shortID+"/edit")
			return
		}
		if _, err := svc.UpdateSyncSchedule(r.Context(), shortID, in); err != nil {
			if errors.Is(err, service.ErrScheduleNotFound) {
				FlashRedirect(w, r, sm, "error", "Schedule not found.", "/settings/general#sync")
				return
			}
			FlashRedirect(w, r, sm, "error", "Failed to save schedule: "+err.Error(), "/settings/sync/schedules/"+shortID+"/edit")
			return
		}
		reloadScheduler(a, r)
		FlashRedirect(w, r, sm, "success", "Schedule saved.", "/settings/general#sync")
	}
}

// ScheduleToggleHandler serves POST /settings/sync/schedules/{shortID}/toggle.
func ScheduleToggleHandler(a *app.App, svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		shortID := chi.URLParam(r, "shortID")
		_ = r.ParseForm()
		enabled := r.FormValue("enabled") != ""
		if err := svc.SetSyncScheduleEnabled(r.Context(), shortID, enabled); err != nil {
			FlashRedirect(w, r, sm, "error", "Failed to update schedule.", "/settings/general#sync")
			return
		}
		reloadScheduler(a, r)
		// Toggle posts via the in-modal auto-save form; respond 204 so the
		// Alpine handler shows its inline toast without a full redirect.
		w.WriteHeader(http.StatusNoContent)
	}
}

// ScheduleDeleteHandler serves POST /settings/sync/schedules/{shortID}/delete.
func ScheduleDeleteHandler(a *app.App, svc *service.Service, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		shortID := chi.URLParam(r, "shortID")
		if err := svc.DeleteSyncSchedule(r.Context(), shortID); err != nil {
			FlashRedirect(w, r, sm, "error", "Failed to delete schedule.", "/settings/general#sync")
			return
		}
		reloadScheduler(a, r)
		FlashRedirect(w, r, sm, "success", "Schedule deleted.", "/settings/general#sync")
	}
}

// parseScheduleForm builds a service input from the submitted form.
func parseScheduleForm(r *http.Request) (service.SyncScheduleInput, error) {
	if err := r.ParseForm(); err != nil {
		return service.SyncScheduleInput{}, errors.New("invalid form")
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		return service.SyncScheduleInput{}, errors.New("Name is required.")
	}
	return service.SyncScheduleInput{
		Name:          name,
		PresetKey:     strings.TrimSpace(r.FormValue("preset")),
		Cron:          strings.TrimSpace(r.FormValue("cron")),
		AppliesToAll:  r.FormValue("applies_to_all") != "",
		Enabled:       r.FormValue("enabled") != "",
		ConnectionIDs: r.Form["connection_ids"],
	}, nil
}

// reloadScheduler is a hook point: schedule mutations take effect on the next
// 15-minute tick automatically (the scheduler reloads schedules from the DB
// every tick), so there is nothing to push. Kept as a named no-op so callers
// read intentionally and a future hot-reload has one wiring site.
func reloadScheduler(a *app.App, r *http.Request) {}
