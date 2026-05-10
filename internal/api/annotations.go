package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	mw "breadbox/internal/middleware"
	"breadbox/internal/service"

	"github.com/go-chi/chi/v5"
)

// ListAnnotationsHandler returns the activity-timeline rows for a transaction.
// REST sibling of the MCP list_annotations tool. Wraps service.ListAnnotations
// 1:1 with no service changes.
//
// Path param `{id}` accepts UUID or short_id (the service resolver handles
// both). Query params:
//
//   - kind: repeatable or comma-separated (e.g. ?kind=comment&kind=rule_applied
//     or ?kind=comment,rule_applied) — maps to ListAnnotationsParams.Kinds.
//     Values are the raw DB kinds (comment, rule_applied, tag_added,
//     tag_removed, category_set, sync_started, sync_updated). The MCP tool
//     exposes a coarser generic-kind alias set; REST uses raw kinds for
//     parity with the underlying schema.
//   - actor_type: repeatable or comma-separated (user | agent | system) —
//     maps to ActorTypes.
//   - since: RFC3339 timestamp; only annotations strictly after this time —
//     maps to Since.
//   - limit: integer; 0 (default) = full timeline, capped server-side at
//     service.MaxAnnotationLimit — maps to Limit.
//   - raw: boolean (true/1/yes); bypass enrichment + dedup — maps to Raw.
func ListAnnotationsHandler(svc *service.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		q := r.URL.Query()

		kinds := parseCSVParam(q, "kind")
		actorTypes := parseCSVParam(q, "actor_type")

		var since time.Time
		if raw := q.Get("since"); raw != "" {
			t, err := parseSinceParam(raw)
			if err != nil {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", err.Error())
				return
			}
			since = t
		}

		limit := 0
		if raw := q.Get("limit"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 0 {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "limit must be a non-negative integer")
				return
			}
			limit = n
		}

		rawMode := false
		if raw := q.Get("raw"); raw != "" {
			b, err := strconv.ParseBool(raw)
			if err != nil {
				mw.WriteError(w, http.StatusBadRequest, "INVALID_PARAMETER", "raw must be true or false")
				return
			}
			rawMode = b
		}

		annotations, err := svc.ListAnnotations(r.Context(), id, service.ListAnnotationsParams{
			Kinds:      kinds,
			ActorTypes: actorTypes,
			Since:      since,
			Limit:      limit,
			Raw:        rawMode,
		})
		if err != nil {
			writeServiceError(w, err, "Transaction not found", "Failed to list annotations")
			return
		}

		// Always return a non-nil slice so the JSON shape is `[]` not `null`.
		if annotations == nil {
			annotations = []service.Annotation{}
		}

		writeData(w, map[string]any{"annotations": annotations})
	}
}

// errInvalidSince is returned when the since query param doesn't parse as
// RFC3339 / RFC3339Nano. The handler maps this to a 400 INVALID_PARAMETER.
var errInvalidSince = errors.New("since must be an RFC3339 timestamp (e.g. 2026-04-26T12:00:00Z)")

// parseSinceParam accepts an RFC3339 (or RFC3339Nano) timestamp. Mirrors the
// MCP parseSince behavior so REST and MCP agents see the same accepted shapes.
func parseSinceParam(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Time{}, errInvalidSince
}
