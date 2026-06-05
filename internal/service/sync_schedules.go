//go:build !lite

package service

import (
	"context"
	"errors"
	"fmt"

	"breadbox/internal/cronspec"
	"breadbox/internal/db"
	"breadbox/internal/pgconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrScheduleNotFound is returned when a sync schedule lookup finds no row.
var ErrScheduleNotFound = errors.New("sync schedule not found")

// SyncScheduleView is the API/UI-facing shape of a sync schedule.
type SyncScheduleView struct {
	ID              string `json:"id"`
	ShortID         string `json:"short_id"`
	Name            string `json:"name"`
	Cron            string `json:"cron"`
	CronHuman       string `json:"cron_human"`
	Preset          string `json:"preset"`
	AppliesToAll    bool   `json:"applies_to_all"`
	Enabled         bool   `json:"enabled"`
	ConnectionCount int64  `json:"connection_count"`
}

// SyncScheduleInput is the create/update payload. PresetKey selects a catalog
// preset; when it is "custom" (or unknown) Cron is used verbatim.
type SyncScheduleInput struct {
	Name         string
	PresetKey    string
	Cron         string
	AppliesToAll bool
	Enabled      bool
	// ConnectionIDs are connection UUIDs or short IDs to target. Ignored when
	// AppliesToAll is true.
	ConnectionIDs []string
}

func scheduleViewFromRow(r db.ListSyncSchedulesRow) SyncScheduleView {
	preset := pgconv.TextOr(r.Preset, "")
	return SyncScheduleView{
		ID:              pgconv.FormatUUID(r.ID),
		ShortID:         r.ShortID,
		Name:            r.Name,
		Cron:            r.Cron,
		CronHuman:       cronspec.Humanize(r.Cron, preset),
		Preset:          preset,
		AppliesToAll:    r.AppliesToAll,
		Enabled:         r.Enabled,
		ConnectionCount: r.ConnectionCount,
	}
}

func scheduleViewFromModel(r db.SyncSchedule, connCount int64) SyncScheduleView {
	preset := pgconv.TextOr(r.Preset, "")
	return SyncScheduleView{
		ID:              pgconv.FormatUUID(r.ID),
		ShortID:         r.ShortID,
		Name:            r.Name,
		Cron:            r.Cron,
		CronHuman:       cronspec.Humanize(r.Cron, preset),
		Preset:          preset,
		AppliesToAll:    r.AppliesToAll,
		Enabled:         r.Enabled,
		ConnectionCount: connCount,
	}
}

// ListSyncSchedules returns every schedule with its connection-target count.
func (s *Service) ListSyncSchedules(ctx context.Context) ([]SyncScheduleView, error) {
	rows, err := s.Queries.ListSyncSchedules(ctx)
	if err != nil {
		return nil, fmt.Errorf("list sync schedules: %w", err)
	}
	out := make([]SyncScheduleView, 0, len(rows))
	for _, r := range rows {
		out = append(out, scheduleViewFromRow(r))
	}
	return out, nil
}

// CreateSyncSchedule validates the input, persists the schedule, and (unless it
// applies to all) sets its connection targets in one transaction.
func (s *Service) CreateSyncSchedule(ctx context.Context, in SyncScheduleInput) (SyncScheduleView, error) {
	name := in.Name
	if name == "" {
		return SyncScheduleView{}, fmt.Errorf("schedule name is required")
	}
	expr, preset, err := cronspec.ResolveCron(in.PresetKey, in.Cron)
	if err != nil {
		return SyncScheduleView{}, err
	}

	connIDs, err := s.resolveConnectionIDs(ctx, in)
	if err != nil {
		return SyncScheduleView{}, err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return SyncScheduleView{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := s.Queries.WithTx(tx)

	row, err := qtx.CreateSyncSchedule(ctx, db.CreateSyncScheduleParams{
		Name:         name,
		Cron:         expr,
		Preset:       pgconv.TextIfNotEmpty(preset),
		AppliesToAll: in.AppliesToAll,
		Enabled:      in.Enabled,
	})
	if err != nil {
		return SyncScheduleView{}, fmt.Errorf("create sync schedule: %w", err)
	}
	for _, cid := range connIDs {
		if err := qtx.AddScheduleConnection(ctx, db.AddScheduleConnectionParams{
			ScheduleID:   row.ID,
			ConnectionID: cid,
		}); err != nil {
			return SyncScheduleView{}, fmt.Errorf("add schedule connection: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return SyncScheduleView{}, fmt.Errorf("commit: %w", err)
	}
	return scheduleViewFromModel(row, int64(len(connIDs))), nil
}

// UpdateSyncSchedule updates a schedule's fields and replaces its connection
// targets. idOrShort accepts a UUID or short ID.
func (s *Service) UpdateSyncSchedule(ctx context.Context, idOrShort string, in SyncScheduleInput) (SyncScheduleView, error) {
	id, err := s.resolveScheduleID(ctx, idOrShort)
	if err != nil {
		return SyncScheduleView{}, err
	}
	name := in.Name
	if name == "" {
		return SyncScheduleView{}, fmt.Errorf("schedule name is required")
	}
	expr, preset, err := cronspec.ResolveCron(in.PresetKey, in.Cron)
	if err != nil {
		return SyncScheduleView{}, err
	}
	connIDs, err := s.resolveConnectionIDs(ctx, in)
	if err != nil {
		return SyncScheduleView{}, err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return SyncScheduleView{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := s.Queries.WithTx(tx)

	row, err := qtx.UpdateSyncSchedule(ctx, db.UpdateSyncScheduleParams{
		ID:           id,
		Name:         name,
		Cron:         expr,
		Preset:       pgconv.TextIfNotEmpty(preset),
		AppliesToAll: in.AppliesToAll,
		Enabled:      in.Enabled,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SyncScheduleView{}, ErrScheduleNotFound
		}
		return SyncScheduleView{}, fmt.Errorf("update sync schedule: %w", err)
	}
	if err := qtx.ClearScheduleConnections(ctx, id); err != nil {
		return SyncScheduleView{}, fmt.Errorf("clear schedule connections: %w", err)
	}
	for _, cid := range connIDs {
		if err := qtx.AddScheduleConnection(ctx, db.AddScheduleConnectionParams{
			ScheduleID:   id,
			ConnectionID: cid,
		}); err != nil {
			return SyncScheduleView{}, fmt.Errorf("add schedule connection: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return SyncScheduleView{}, fmt.Errorf("commit: %w", err)
	}
	return scheduleViewFromModel(row, int64(len(connIDs))), nil
}

// SetSyncScheduleEnabled toggles a schedule's enabled flag.
func (s *Service) SetSyncScheduleEnabled(ctx context.Context, idOrShort string, enabled bool) error {
	id, err := s.resolveScheduleID(ctx, idOrShort)
	if err != nil {
		return err
	}
	if _, err := s.Queries.SetSyncScheduleEnabled(ctx, db.SetSyncScheduleEnabledParams{
		ID:      id,
		Enabled: enabled,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrScheduleNotFound
		}
		return fmt.Errorf("set sync schedule enabled: %w", err)
	}
	return nil
}

// DeleteSyncSchedule removes a schedule (and its connection mappings via cascade).
func (s *Service) DeleteSyncSchedule(ctx context.Context, idOrShort string) error {
	id, err := s.resolveScheduleID(ctx, idOrShort)
	if err != nil {
		return err
	}
	if err := s.Queries.DeleteSyncSchedule(ctx, id); err != nil {
		return fmt.Errorf("delete sync schedule: %w", err)
	}
	return nil
}

// ScheduleRef is a lightweight (name, cron) pair used to describe the schedules
// that apply to a connection — for rendering "next sync" / "syncs on" without
// loading full schedule rows per connection.
type ScheduleRef struct {
	Name string `json:"name"`
	Cron string `json:"cron"`
}

// SyncScheduleResolution loads the enabled schedules once and returns the
// `applies_to_all` schedules plus a per-connection map (keyed by connection
// UUID string) of explicitly-targeted schedules. A connection's effective
// schedules are `all` + `perConn[uuid]` — mirroring the scheduler's resolver,
// but name-carrying and for display. Cheap: two queries regardless of count.
func (s *Service) SyncScheduleResolution(ctx context.Context) (all []ScheduleRef, perConn map[string][]ScheduleRef, err error) {
	rows, err := s.Queries.ListEnabledSyncSchedules(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list enabled sync schedules: %w", err)
	}
	pairs, err := s.Queries.ListSyncScheduleConnectionPairs(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list sync schedule connection pairs: %w", err)
	}

	byID := make(map[[16]byte]ScheduleRef, len(rows))
	appliesAll := make(map[[16]byte]bool, len(rows))
	for _, row := range rows {
		ref := ScheduleRef{Name: row.Name, Cron: row.Cron}
		byID[row.ID.Bytes] = ref
		appliesAll[row.ID.Bytes] = row.AppliesToAll
		if row.AppliesToAll {
			all = append(all, ref)
		}
	}

	perConn = make(map[string][]ScheduleRef)
	for _, p := range pairs {
		if appliesAll[p.ScheduleID.Bytes] {
			continue
		}
		if ref, ok := byID[p.ScheduleID.Bytes]; ok {
			key := pgconv.FormatUUID(p.ConnectionID)
			perConn[key] = append(perConn[key], ref)
		}
	}
	return all, perConn, nil
}

// ListScheduleConnectionShortIDs returns the connection short IDs a schedule
// targets, for pre-checking the edit form.
func (s *Service) ListScheduleConnectionShortIDs(ctx context.Context, idOrShort string) ([]string, error) {
	id, err := s.resolveScheduleID(ctx, idOrShort)
	if err != nil {
		return nil, err
	}
	return s.Queries.ListConnectionShortIDsForSchedule(ctx, id)
}

// resolveScheduleID accepts a UUID or short ID and returns the schedule UUID.
func (s *Service) resolveScheduleID(ctx context.Context, idOrShort string) (pgtype.UUID, error) {
	lookup := func(ctx context.Context, short string) (pgtype.UUID, error) {
		row, err := s.Queries.GetSyncScheduleByShortID(ctx, short)
		if err != nil {
			return pgtype.UUID{}, err
		}
		return row.ID, nil
	}
	return s.resolveID(ctx, idOrShort, lookup, ErrScheduleNotFound)
}

// resolveConnectionIDs maps the input's connection identifiers to UUIDs. Returns
// nil when the schedule applies to all connections (targets are ignored).
func (s *Service) resolveConnectionIDs(ctx context.Context, in SyncScheduleInput) ([]pgtype.UUID, error) {
	if in.AppliesToAll {
		return nil, nil
	}
	out := make([]pgtype.UUID, 0, len(in.ConnectionIDs))
	for _, raw := range in.ConnectionIDs {
		if raw == "" {
			continue
		}
		uid, err := s.resolveConnectionID(ctx, raw)
		if err != nil {
			return nil, fmt.Errorf("resolve connection %q: %w", raw, err)
		}
		out = append(out, uid)
	}
	return out, nil
}
