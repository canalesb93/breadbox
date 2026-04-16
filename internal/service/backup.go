package service

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// BackupInfo describes a backup file on disk.
type BackupInfo struct {
	Filename  string    `json:"filename"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
	Trigger   string    `json:"trigger"` // "manual" or "scheduled"
}

// BackupService handles database backup and restore operations.
type BackupService struct {
	databaseURL string
	backupDir   string
	logger      *slog.Logger
}

// NewBackupService creates a new BackupService.
// backupDir is the directory where backup files are stored.
func NewBackupService(databaseURL, backupDir string, logger *slog.Logger) *BackupService {
	return &BackupService{
		databaseURL: databaseURL,
		backupDir:   backupDir,
		logger:      logger,
	}
}

// BackupDir returns the configured backup directory.
func (bs *BackupService) BackupDir() string {
	return bs.backupDir
}

// EnsureBackupDir creates the backup directory if it doesn't exist.
func (bs *BackupService) EnsureBackupDir() error {
	return os.MkdirAll(bs.backupDir, 0750)
}

// CreateBackup runs pg_dump and compresses the output to a .sql.gz file.
// trigger should be "manual" or "scheduled".
// Returns the filename of the created backup.
func (bs *BackupService) CreateBackup(ctx context.Context, trigger string) (string, error) {
	if err := bs.EnsureBackupDir(); err != nil {
		return "", fmt.Errorf("create backup directory: %w", err)
	}

	if pf := bs.Preflight(ctx); !pf.OK {
		return "", fmt.Errorf("%s", pf.Message)
	}

	timestamp := time.Now().UTC().Format("20060102_150405")
	filename := fmt.Sprintf("breadbox_%s_%s.sql.gz", trigger, timestamp)
	fullPath := filepath.Join(bs.backupDir, filename)

	// Build pg_dump args. Use --no-owner and --no-acl for portability.
	args := []string{
		"--format=plain",
		"--no-owner",
		"--no-acl",
		"--clean",
		"--if-exists",
		bs.databaseURL,
	}

	cmd := exec.CommandContext(ctx, "pg_dump", args...)

	// Capture stderr for error reporting.
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("create stdout pipe: %w", err)
	}

	var fileClosed bool
	outFile, err := os.Create(fullPath)
	if err != nil {
		return "", fmt.Errorf("create backup file: %w", err)
	}
	defer func() {
		if !fileClosed {
			outFile.Close()
			os.Remove(fullPath) // cleanup incomplete backup
		}
	}()

	gzWriter := gzip.NewWriter(outFile)
	defer func() {
		if !fileClosed {
			gzWriter.Close()
		}
	}()

	if err := cmd.Start(); err != nil {
		os.Remove(fullPath)
		return "", fmt.Errorf("start pg_dump: %w", err)
	}
	// Ensure the process is always reaped even if we return early.
	defer func() { _ = cmd.Wait() }()

	if _, err := io.Copy(gzWriter, stdout); err != nil {
		return "", fmt.Errorf("write backup data: %w", err)
	}

	if err := gzWriter.Close(); err != nil {
		return "", fmt.Errorf("close gzip writer: %w", err)
	}
	if err := outFile.Close(); err != nil {
		return "", fmt.Errorf("close backup file: %w", err)
	}
	fileClosed = true

	if err := cmd.Wait(); err != nil {
		os.Remove(fullPath)
		return "", fmt.Errorf("pg_dump failed: %s: %w", stderrBuf.String(), err)
	}

	// Verify the file was actually created with content.
	info, err := os.Stat(fullPath)
	if err != nil || info.Size() == 0 {
		os.Remove(fullPath)
		return "", fmt.Errorf("backup file is empty or missing")
	}

	bs.logger.Info("backup created", "filename", filename, "size", info.Size(), "trigger", trigger)
	return filename, nil
}

// ListBackups returns all backup files sorted by creation time (newest first).
func (bs *BackupService) ListBackups() ([]BackupInfo, error) {
	if err := bs.EnsureBackupDir(); err != nil {
		return nil, fmt.Errorf("ensure backup directory: %w", err)
	}

	entries, err := os.ReadDir(bs.backupDir)
	if err != nil {
		return nil, fmt.Errorf("read backup directory: %w", err)
	}

	var backups []BackupInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".sql.gz") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		trigger := parseTriggerFromFilename(entry.Name())

		backups = append(backups, BackupInfo{
			Filename:  entry.Name(),
			Size:      info.Size(),
			CreatedAt: info.ModTime(),
			Trigger:   trigger,
		})
	}

	// Sort newest first by filename (contains YYYYMMDD_HHMMSS timestamp).
	// Filename sort is more reliable than ModTime because test helpers and
	// file copies can produce identical modification times.
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Filename > backups[j].Filename
	})

	return backups, nil
}

// GetBackupPath returns the full path for a backup filename.
// Returns an error if the filename contains path traversal.
func (bs *BackupService) GetBackupPath(filename string) (string, error) {
	if strings.Contains(filename, "/") || strings.Contains(filename, "\\") || strings.Contains(filename, "..") {
		return "", fmt.Errorf("invalid backup filename")
	}
	if !strings.HasSuffix(filename, ".sql.gz") {
		return "", fmt.Errorf("invalid backup file extension")
	}
	fullPath := filepath.Join(bs.backupDir, filename)

	// Verify the file exists.
	if _, err := os.Stat(fullPath); err != nil {
		return "", fmt.Errorf("backup file not found: %s", filename)
	}
	return fullPath, nil
}

// DeleteBackup removes a backup file.
func (bs *BackupService) DeleteBackup(filename string) error {
	fullPath, err := bs.GetBackupPath(filename)
	if err != nil {
		return err
	}
	if err := os.Remove(fullPath); err != nil {
		return fmt.Errorf("delete backup: %w", err)
	}
	bs.logger.Info("backup deleted", "filename", filename)
	return nil
}

// RestoreBackup decompresses a .sql.gz file and runs it through psql.
// This is a destructive operation that replaces the current database contents.
func (bs *BackupService) RestoreBackup(ctx context.Context, filename string) error {
	fullPath, err := bs.GetBackupPath(filename)
	if err != nil {
		return err
	}

	// Check that psql is available.
	if _, err := exec.LookPath("psql"); err != nil {
		return fmt.Errorf("psql not found on PATH: %w", err)
	}

	// Open and decompress the backup file.
	f, err := os.Open(fullPath)
	if err != nil {
		return fmt.Errorf("open backup file: %w", err)
	}
	defer f.Close()

	gzReader, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("decompress backup: %w", err)
	}
	defer gzReader.Close()

	// Run psql with the decompressed SQL as stdin.
	// Use --single-transaction for atomicity.
	args := []string{
		"--single-transaction",
		"--set", "ON_ERROR_STOP=on",
		bs.databaseURL,
	}

	cmd := exec.CommandContext(ctx, "psql", args...)
	cmd.Stdin = gzReader

	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("psql restore failed: %s: %w", stderrBuf.String(), err)
	}

	bs.logger.Info("backup restored", "filename", filename)
	return nil
}

// RestoreFromReader decompresses a gzipped SQL stream and runs it through psql.
// Used for uploaded backup files that aren't yet saved to disk.
func (bs *BackupService) RestoreFromReader(ctx context.Context, r io.Reader) error {
	// Check that psql is available.
	if _, err := exec.LookPath("psql"); err != nil {
		return fmt.Errorf("psql not found on PATH: %w", err)
	}

	gzReader, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("decompress backup: %w", err)
	}
	defer gzReader.Close()

	args := []string{
		"--single-transaction",
		"--set", "ON_ERROR_STOP=on",
		bs.databaseURL,
	}

	cmd := exec.CommandContext(ctx, "psql", args...)
	cmd.Stdin = gzReader

	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("psql restore failed: %s: %w", stderrBuf.String(), err)
	}

	bs.logger.Info("backup restored from upload")
	return nil
}

// CleanupOldBackups deletes backups older than retentionDays.
// Returns the number of files deleted.
func (bs *BackupService) CleanupOldBackups(retentionDays int) (int, error) {
	if retentionDays <= 0 {
		return 0, nil
	}

	backups, err := bs.ListBackups()
	if err != nil {
		return 0, err
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	deleted := 0
	for _, b := range backups {
		if b.CreatedAt.Before(cutoff) {
			if err := os.Remove(filepath.Join(bs.backupDir, b.Filename)); err != nil {
				bs.logger.Error("failed to delete old backup", "filename", b.Filename, "error", err)
				continue
			}
			deleted++
		}
	}

	if deleted > 0 {
		bs.logger.Info("cleaned up old backups", "deleted", deleted, "retention_days", retentionDays)
	}
	return deleted, nil
}

// TotalBackupSize returns the total size of all backups in bytes.
func (bs *BackupService) TotalBackupSize() (int64, error) {
	backups, err := bs.ListBackups()
	if err != nil {
		return 0, err
	}
	var total int64
	for _, b := range backups {
		total += b.Size
	}
	return total, nil
}

// ParseDatabaseName extracts the database name from a PostgreSQL connection URL
// for display purposes. Returns just the database name, not credentials.
func ParseDatabaseName(databaseURL string) string {
	u, err := url.Parse(databaseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "unknown"
	}
	name := strings.TrimPrefix(u.Path, "/")
	if name == "" {
		return "unknown"
	}
	return name
}

// parseTriggerFromFilename extracts the trigger type from a backup filename.
// Expected format: breadbox_<trigger>_<timestamp>.sql.gz
func parseTriggerFromFilename(filename string) string {
	name := strings.TrimSuffix(filename, ".sql.gz")
	parts := strings.SplitN(name, "_", 3) // breadbox, trigger, timestamp
	if len(parts) >= 2 {
		switch parts[1] {
		case "manual", "scheduled":
			return parts[1]
		}
	}
	return "unknown"
}

// PreflightResult reports whether backup tooling is usable on this host.
type PreflightResult struct {
	OK              bool
	PgDumpAvailable bool
	PsqlAvailable   bool
	PgDumpMajor     int
	PsqlMajor       int
	ServerMajor     int
	Message         string
}

var pgVersionRe = regexp.MustCompile(`\b(\d+)\.\d+`)

// Preflight checks that pg_dump and psql are on PATH and that their major
// version is at least the server's. pg_dump refuses to dump a newer server,
// so a mismatch is a blocker, not a warning.
func (bs *BackupService) Preflight(ctx context.Context) PreflightResult {
	var res PreflightResult

	if _, err := exec.LookPath("pg_dump"); err == nil {
		res.PgDumpAvailable = true
		res.PgDumpMajor = parseClientMajor(ctx, "pg_dump")
	}
	if _, err := exec.LookPath("psql"); err == nil {
		res.PsqlAvailable = true
		res.PsqlMajor = parseClientMajor(ctx, "psql")
	}

	if !res.PgDumpAvailable || !res.PsqlAvailable {
		var missing []string
		if !res.PgDumpAvailable {
			missing = append(missing, "pg_dump")
		}
		if !res.PsqlAvailable {
			missing = append(missing, "psql")
		}
		res.Message = fmt.Sprintf("Missing tool(s) on PATH: %s. Install the PostgreSQL client (e.g. postgresql-client package) on the host.", strings.Join(missing, ", "))
		return res
	}

	res.ServerMajor = queryServerMajor(ctx, bs.databaseURL)

	if res.ServerMajor > 0 && res.PgDumpMajor > 0 && res.PgDumpMajor < res.ServerMajor {
		res.Message = fmt.Sprintf(
			"pg_dump is version %d but the database server is version %d. pg_dump refuses to dump a newer server. Install postgresql-client ≥ %d on the host.",
			res.PgDumpMajor, res.ServerMajor, res.ServerMajor,
		)
		return res
	}

	res.OK = true
	return res
}

// parseClientMajor runs `<tool> --version` and extracts the major version.
// Returns 0 if parsing fails — callers treat that as "unknown, proceed".
func parseClientMajor(ctx context.Context, tool string) int {
	out, err := exec.CommandContext(ctx, tool, "--version").Output()
	if err != nil {
		return 0
	}
	m := pgVersionRe.FindStringSubmatch(string(out))
	if len(m) < 2 {
		return 0
	}
	major, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return major
}

// queryServerMajor asks the server for its version via `psql -t -c 'SHOW server_version_num'`.
// server_version_num is formatted as MMmmpp since PG 10 (e.g. 160013 → major 16).
// Returns 0 on any failure.
func queryServerMajor(ctx context.Context, databaseURL string) int {
	cmd := exec.CommandContext(ctx, "psql", "-t", "-A", "-X", "-c", "SHOW server_version_num", databaseURL)
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	verNum, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil || verNum == 0 {
		return 0
	}
	return verNum / 10000
}

// FormatBytes formats a byte count into a human-readable string.
func FormatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// GetBackupSchedule returns the backup schedule from app_config.
// Returns empty string if not configured (disabled).
func (s *Service) GetBackupSchedule(ctx context.Context) string {
	row, err := s.Queries.GetAppConfig(ctx, "backup_schedule")
	if err != nil {
		return ""
	}
	if !row.Value.Valid || row.Value.String == "" {
		return ""
	}
	return row.Value.String
}

// GetBackupRetentionDays returns the backup retention days from app_config.
// Returns default of 7 if not configured.
func (s *Service) GetBackupRetentionDays(ctx context.Context) int {
	row, err := s.Queries.GetAppConfig(ctx, "backup_retention_days")
	if err != nil {
		return 7
	}
	if !row.Value.Valid || row.Value.String == "" {
		return 7
	}
	days, err := strconv.Atoi(row.Value.String)
	if err != nil || days < 0 {
		return 7
	}
	return days
}
