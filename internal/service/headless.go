package service

import (
	"context"
	"fmt"
)

// HeadlessBootstrapInput carries the process-scoped facts the service can't
// derive from the database alone (encryption key presence, provider config,
// scheduler state, version). The handler at /api/v1/headless/bootstrap fills
// it in from *app.App and *config.Config and hands it to HeadlessBootstrap;
// the service stitches that together with DB-derived counts.
type HeadlessBootstrapInput struct {
	Version          string
	EncryptionKeySet bool
	SchedulerRunning bool
	Providers        []HeadlessProvider
	// LatestMigrationVersion is the highest embedded migration version
	// (from cmd/breadbox/doctor.go::latestEmbeddedMigration). The handler
	// supplies it so the service stays free of `io/fs` migration reads.
	LatestMigrationVersion int64
}

// HeadlessProvider is one provider row in the bootstrap report.
type HeadlessProvider struct {
	Name       string `json:"name"`
	Configured bool   `json:"configured"`
	Env        string `json:"env,omitempty"`
}

// HeadlessDatabase reports on DB readiness for the bootstrap report.
type HeadlessDatabase struct {
	Connected         bool  `json:"connected"`
	MigrationsCurrent bool  `json:"migrations_current"`
	MigrationVersion  int64 `json:"migration_version"`
}

// HeadlessBootstrapResponse is the JSON shape returned by
// GET /api/v1/headless/bootstrap. Used by `breadbox doctor` and any
// orchestrator that wants a one-shot readiness summary.
type HeadlessBootstrapResponse struct {
	Version                string             `json:"version"`
	EncryptionKeySet       bool               `json:"encryption_key_set"`
	Database               HeadlessDatabase   `json:"database"`
	Providers              []HeadlessProvider `json:"providers"`
	UsersCount             int64              `json:"users_count"`
	LoginAccountsCount     int64              `json:"login_accounts_count"`
	APIKeysCount           int64              `json:"api_keys_count"`
	ActiveConnectionsCount int64              `json:"active_connections_count"`
	SchedulerRunning       bool               `json:"scheduler_running"`
	FirstRun               bool               `json:"first_run"`
}

// HeadlessBootstrap returns a one-shot readiness report for the CLI's
// `doctor` command (and any external orchestrator).
//
// The service is responsible for the DB-derived rows (counts, applied
// migration version) and stitches the process-scoped facts from `in` onto
// the response. If a count query fails the field stays zero — the report is
// best-effort and never blocks a CLI session.
func (s *Service) HeadlessBootstrap(ctx context.Context, in HeadlessBootstrapInput) (*HeadlessBootstrapResponse, error) {
	resp := &HeadlessBootstrapResponse{
		Version:          in.Version,
		EncryptionKeySet: in.EncryptionKeySet,
		Providers:        in.Providers,
		SchedulerRunning: in.SchedulerRunning,
	}
	if resp.Providers == nil {
		resp.Providers = []HeadlessProvider{}
	}

	// DB ping doubles as "are we connected": if the pool's Ping fails we
	// surface that, but the request itself only got here because the DB
	// pool is alive. We still call Ping so the result reflects reality at
	// call time rather than assuming.
	resp.Database.Connected = s.Pool.Ping(ctx) == nil

	// Applied goose version. Query the goose_db_version table directly so we
	// don't pull goose's runtime into the service layer. If the table is
	// missing (fresh DB), applied stays 0 and migrations_current is false.
	var applied int64
	err := s.Pool.QueryRow(ctx,
		"SELECT COALESCE(MAX(version_id), 0) FROM goose_db_version WHERE is_applied = true",
	).Scan(&applied)
	if err != nil {
		s.Logger.Warn("headless bootstrap: read goose_db_version failed", "error", err)
	}
	resp.Database.MigrationVersion = applied
	resp.Database.MigrationsCurrent = in.LatestMigrationVersion > 0 && applied >= in.LatestMigrationVersion

	// Counts. Each failure is best-effort logged; the field stays zero.
	if n, err := s.Queries.CountUsers(ctx); err == nil {
		resp.UsersCount = n
	} else {
		s.Logger.Warn("headless bootstrap: count users failed", "error", err)
	}
	if n, err := s.Queries.CountAuthAccounts(ctx); err == nil {
		resp.LoginAccountsCount = n
	} else {
		s.Logger.Warn("headless bootstrap: count auth accounts failed", "error", err)
	}
	if n, err := s.Queries.CountActiveApiKeys(ctx); err == nil {
		resp.APIKeysCount = n
	} else {
		s.Logger.Warn("headless bootstrap: count api keys failed", "error", err)
	}
	// "Active" connections — status='active', un-paused. Soft-disconnected
	// connections are excluded so the count matches "what's syncing".
	var activeConns int64
	if err := s.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM bank_connections WHERE status = 'active'`,
	).Scan(&activeConns); err != nil {
		s.Logger.Warn("headless bootstrap: count active connections failed", "error", err)
	}
	resp.ActiveConnectionsCount = activeConns

	// first_run mirrors the admin redirect heuristic: zero login accounts
	// means the operator hasn't completed setup yet.
	resp.FirstRun = resp.LoginAccountsCount == 0

	// Defensive: surface a non-nil error only when DB is plainly unreachable.
	if !resp.Database.Connected {
		return resp, fmt.Errorf("database is not reachable")
	}
	return resp, nil
}
