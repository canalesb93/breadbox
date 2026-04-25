package service

import (
	"archive/tar"
	"bufio"
	"bytes"
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

	"breadbox/internal/appconfig"
)

// Bundle layout (inside the .tar.gz produced by CreateBackup):
//
//   dump.sql.gz       — the gzipped pg_dump output (always present)
//   encryption.key    — optional; the auto-managed AES key used to decrypt
//                        provider credentials. Included when the running
//                        process has a key file path it can read.
//
// Restore tolerates the legacy single-file layout (.sql.gz) for backups taken
// before this format change. New backups always use the bundled .tar.gz.
const (
	bundleDumpEntry          = "dump.sql.gz"
	bundleEncryptionKeyEntry = "encryption.key"
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
	databaseURL       string
	backupDir         string
	encryptionKeyPath string // path to the on-disk auto-managed key, "" when using BYO env-var key
	logger            *slog.Logger
}

// NewBackupService creates a new BackupService.
// backupDir is the directory where backup files are stored.
// encryptionKeyPath is the on-disk location of the auto-managed encryption.key
// (empty string when the operator supplies ENCRYPTION_KEY via the environment
// — backup bundles then carry no key file, by design).
func NewBackupService(databaseURL, backupDir, encryptionKeyPath string, logger *slog.Logger) *BackupService {
	return &BackupService{
		databaseURL:       databaseURL,
		backupDir:         backupDir,
		encryptionKeyPath: encryptionKeyPath,
		logger:            logger,
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

// CreateBackup runs pg_dump and produces a .tar.gz bundle containing the
// gzipped SQL dump and (if available) the auto-managed encryption.key. Bundling
// the key means a single archive can fully restore an install — no separate
// "remember to back up your key" step.
//
// trigger should be "manual" or "scheduled".
// Returns the filename of the created bundle.
func (bs *BackupService) CreateBackup(ctx context.Context, trigger string) (string, error) {
	if err := bs.EnsureBackupDir(); err != nil {
		return "", fmt.Errorf("create backup directory: %w", err)
	}

	if pf := bs.Preflight(ctx); !pf.OK {
		return "", fmt.Errorf("%s", pf.Message)
	}

	timestamp := time.Now().UTC().Format("20060102_150405")
	filename := fmt.Sprintf("breadbox_%s_%s.tar.gz", trigger, timestamp)
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

	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("create stdout pipe: %w", err)
	}

	// Buffer the gzipped dump in memory so we can write a tar header with the
	// final size. pg_dump is normally tens of MB on home installs — tractable
	// to buffer. If this becomes a problem on huge data volumes we can switch
	// to a temp-file dance, but it adds complexity without measurable benefit
	// at the current scale.
	var dumpBuf bytes.Buffer
	dumpGz := gzip.NewWriter(&dumpBuf)

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start pg_dump: %w", err)
	}

	if _, err := io.Copy(dumpGz, stdout); err != nil {
		_ = cmd.Wait()
		return "", fmt.Errorf("write backup data: %w", err)
	}
	if err := dumpGz.Close(); err != nil {
		_ = cmd.Wait()
		return "", fmt.Errorf("close gzip writer: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("pg_dump failed: %s: %w", stderrBuf.String(), err)
	}

	// Read the auto-managed encryption key (best-effort). We tolerate a missing
	// file because the BYO-env-var path leaves encryptionKeyPath empty; we
	// don't want to scare users who knowingly opted out of bundled key storage.
	var keyBytes []byte
	if bs.encryptionKeyPath != "" {
		if data, err := os.ReadFile(bs.encryptionKeyPath); err == nil {
			keyBytes = data
		} else if !os.IsNotExist(err) {
			bs.logger.Warn("backup: could not read encryption key, omitting from bundle",
				"path", bs.encryptionKeyPath, "error", err)
		}
	}

	// Write the tar.gz bundle to disk atomically: build into a tmp file, then
	// rename. Avoids leaving a partial tarball if the process dies mid-write.
	tmpPath := fullPath + ".tmp"
	outFile, err := os.Create(tmpPath)
	if err != nil {
		return "", fmt.Errorf("create backup file: %w", err)
	}
	cleanup := func() { _ = os.Remove(tmpPath) }

	bundleGz := gzip.NewWriter(outFile)
	tw := tar.NewWriter(bundleGz)

	now := time.Now().UTC()

	if err := writeTarEntry(tw, bundleDumpEntry, dumpBuf.Bytes(), 0o600, now); err != nil {
		outFile.Close()
		cleanup()
		return "", fmt.Errorf("write dump entry: %w", err)
	}

	if len(keyBytes) > 0 {
		if err := writeTarEntry(tw, bundleEncryptionKeyEntry, keyBytes, 0o600, now); err != nil {
			outFile.Close()
			cleanup()
			return "", fmt.Errorf("write key entry: %w", err)
		}
	}

	if err := tw.Close(); err != nil {
		outFile.Close()
		cleanup()
		return "", fmt.Errorf("close tar: %w", err)
	}
	if err := bundleGz.Close(); err != nil {
		outFile.Close()
		cleanup()
		return "", fmt.Errorf("close bundle gzip: %w", err)
	}
	if err := outFile.Sync(); err != nil {
		outFile.Close()
		cleanup()
		return "", fmt.Errorf("sync bundle: %w", err)
	}
	if err := outFile.Close(); err != nil {
		cleanup()
		return "", fmt.Errorf("close bundle: %w", err)
	}
	if err := os.Rename(tmpPath, fullPath); err != nil {
		cleanup()
		return "", fmt.Errorf("rename bundle: %w", err)
	}

	info, err := os.Stat(fullPath)
	if err != nil || info.Size() == 0 {
		os.Remove(fullPath)
		return "", fmt.Errorf("backup file is empty or missing")
	}

	bs.logger.Info("backup created",
		"filename", filename,
		"size", info.Size(),
		"trigger", trigger,
		"includes_key", len(keyBytes) > 0,
	)
	return filename, nil
}

// writeTarEntry writes a single in-memory blob into the tar archive.
func writeTarEntry(tw *tar.Writer, name string, data []byte, mode int64, mtime time.Time) error {
	if err := tw.WriteHeader(&tar.Header{
		Name:    name,
		Mode:    mode,
		Size:    int64(len(data)),
		ModTime: mtime,
	}); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
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
		if !isBackupFilename(entry.Name()) {
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
	if !isBackupFilename(filename) {
		return "", fmt.Errorf("invalid backup file extension")
	}
	fullPath := filepath.Join(bs.backupDir, filename)

	// Verify the file exists.
	if _, err := os.Stat(fullPath); err != nil {
		return "", fmt.Errorf("backup file not found: %s", filename)
	}
	return fullPath, nil
}

// isBackupFilename returns true for both the new .tar.gz bundles and the
// legacy .sql.gz dumps.
func isBackupFilename(name string) bool {
	return strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".sql.gz")
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

// RestoreBackup restores from a backup file on disk. Supports both the new
// .tar.gz bundle format (containing dump.sql.gz + optional encryption.key) and
// the legacy single .sql.gz dump.
//
// Destructive: replaces current database contents.
func (bs *BackupService) RestoreBackup(ctx context.Context, filename string) error {
	fullPath, err := bs.GetBackupPath(filename)
	if err != nil {
		return err
	}

	if _, err := exec.LookPath("psql"); err != nil {
		return fmt.Errorf("psql not found on PATH: %w", err)
	}

	f, err := os.Open(fullPath)
	if err != nil {
		return fmt.Errorf("open backup file: %w", err)
	}
	defer f.Close()

	if err := bs.restoreFromStream(ctx, f); err != nil {
		return err
	}

	bs.logger.Info("backup restored", "filename", filename)
	return nil
}

// RestoreFromReader restores from an uploaded archive. Supports both .tar.gz
// bundles and legacy .sql.gz dumps — the format is detected from the gzip
// payload (tar header inside, vs raw SQL).
func (bs *BackupService) RestoreFromReader(ctx context.Context, r io.Reader) error {
	if _, err := exec.LookPath("psql"); err != nil {
		return fmt.Errorf("psql not found on PATH: %w", err)
	}

	if err := bs.restoreFromStream(ctx, r); err != nil {
		return err
	}

	bs.logger.Info("backup restored from upload")
	return nil
}

// restoreFromStream is the shared body of RestoreBackup and RestoreFromReader.
// It auto-detects bundle vs legacy and dispatches accordingly.
func (bs *BackupService) restoreFromStream(ctx context.Context, r io.Reader) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("decompress backup: %w", err)
	}
	defer gz.Close()

	// Peek enough bytes to tell whether the gzip payload is a tar archive
	// (POSIX ustar header puts "ustar" at offset 257) or raw SQL. We use
	// bufio so the peek doesn't consume the stream — both branches replay
	// from the buffered reader.
	br := bufio.NewReaderSize(gz, 8192)
	header, err := br.Peek(512)
	if err != nil && err != io.EOF {
		return fmt.Errorf("peek backup format: %w", err)
	}

	if isUstarHeader(header) {
		return bs.restoreFromBundle(ctx, br)
	}
	return bs.runPsqlRestore(ctx, br)
}

// isUstarHeader returns true if buf looks like the first block of a POSIX tar
// archive (ustar magic at offset 257).
func isUstarHeader(buf []byte) bool {
	const magicOffset = 257
	if len(buf) < magicOffset+5 {
		return false
	}
	return string(buf[magicOffset:magicOffset+5]) == "ustar"
}

// restoreFromBundle reads the .tar payload (already gunzipped + buffered),
// extracts dump.sql.gz, restores it, and writes encryption.key back into the
// data dir if it differs from the live key file.
func (bs *BackupService) restoreFromBundle(ctx context.Context, r io.Reader) error {
	tr := tar.NewReader(r)

	var (
		dumpData    []byte
		keyData     []byte
		dumpPresent bool
	)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read backup bundle: %w", err)
		}

		switch hdr.Name {
		case bundleDumpEntry:
			data, err := io.ReadAll(tr)
			if err != nil {
				return fmt.Errorf("read dump from bundle: %w", err)
			}
			dumpData = data
			dumpPresent = true
		case bundleEncryptionKeyEntry:
			data, err := io.ReadAll(tr)
			if err != nil {
				return fmt.Errorf("read encryption key from bundle: %w", err)
			}
			keyData = data
		default:
			// Tolerate (and skip) unknown entries so the format can grow.
			if _, err := io.Copy(io.Discard, tr); err != nil {
				return fmt.Errorf("skip bundle entry %q: %w", hdr.Name, err)
			}
		}
	}

	if !dumpPresent {
		return fmt.Errorf("backup bundle missing %s entry", bundleDumpEntry)
	}

	// Restore SQL first so a key-write failure doesn't leave a half-applied DB.
	dumpGz, err := gzip.NewReader(bytes.NewReader(dumpData))
	if err != nil {
		return fmt.Errorf("decompress dump entry: %w", err)
	}
	defer dumpGz.Close()

	if err := bs.runPsqlRestore(ctx, dumpGz); err != nil {
		return err
	}

	// Restore the encryption key file when (a) the bundle includes one and
	// (b) we have a configured destination path. We write it next to whatever
	// the running process believes is the current key file — operators using
	// BYO env vars opt out by leaving encryptionKeyPath empty.
	if len(keyData) > 0 && bs.encryptionKeyPath != "" {
		if err := bs.writeRestoredKey(keyData); err != nil {
			return fmt.Errorf("restore encryption key: %w", err)
		}
		bs.logger.Info("encryption key restored from backup bundle", "path", bs.encryptionKeyPath)
	} else if len(keyData) > 0 {
		bs.logger.Warn("backup bundle contains an encryption key but no destination path is configured; ignoring")
	}

	return nil
}

// writeRestoredKey atomically replaces the live encryption key file with the
// bundle's copy. Skips the write when the contents already match — avoids a
// no-op rename and makes restores against the same install idempotent.
func (bs *BackupService) writeRestoredKey(data []byte) error {
	if existing, err := os.ReadFile(bs.encryptionKeyPath); err == nil {
		if bytes.Equal(bytes.TrimSpace(existing), bytes.TrimSpace(data)) {
			return nil
		}
	}

	dir := filepath.Dir(bs.encryptionKeyPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("ensure key dir: %w", err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(bs.encryptionKeyPath)+".tmp.*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	return os.Rename(tmpName, bs.encryptionKeyPath)
}

// runPsqlRestore pipes the supplied SQL stream into psql.
func (bs *BackupService) runPsqlRestore(ctx context.Context, r io.Reader) error {
	args := []string{
		"--single-transaction",
		"--set", "ON_ERROR_STOP=on",
		bs.databaseURL,
	}

	cmd := exec.CommandContext(ctx, "psql", args...)
	cmd.Stdin = r

	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("psql restore failed: %s: %w", stderrBuf.String(), err)
	}
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
// Expected format: breadbox_<trigger>_<timestamp>.{tar.gz|sql.gz}
func parseTriggerFromFilename(filename string) string {
	name := strings.TrimSuffix(filename, ".tar.gz")
	name = strings.TrimSuffix(name, ".sql.gz")
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
	return appconfig.String(ctx, s.Queries, "backup_schedule", "")
}

// GetBackupRetentionDays returns the backup retention days from app_config.
// Returns default of 7 if not configured or negative.
func (s *Service) GetBackupRetentionDays(ctx context.Context) int {
	days := appconfig.Int(ctx, s.Queries, "backup_retention_days", 7)
	if days < 0 {
		return 7
	}
	return days
}
