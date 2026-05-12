// Package cli — `breadbox backup`.
//
// Local-only commands: dump the database via pg_dump, list previous backups,
// and restore from one. There's no REST surface for backups by design —
// shipping a multi-megabyte dump over HTTP is the wrong tool for the job,
// and restore must run with the server stopped anyway.
package cli

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"breadbox/internal/cli/output"
	bbconfig "breadbox/internal/config"
	"breadbox/internal/service"

	"github.com/spf13/cobra"
)

// AddBackupCmd registers `breadbox backup` (L-scoped).
func AddBackupCmd(root *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Create, list, and restore PostgreSQL backups (local only)",
	}
	cmd.AddCommand(newBackupCreateCmd())
	cmd.AddCommand(newBackupListCmd())
	cmd.AddCommand(newBackupRestoreCmd())
	root.AddCommand(cmd)
}

// defaultBackupDir resolves the user-local backup directory used when the
// user doesn't pass --out. ~/.local/share/breadbox/backups is the XDG
// convention; on systems without HOME set we fall back to ./backups so the
// command still works in CI environments.
func defaultBackupDir() (string, error) {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "breadbox", "backups"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".", "backups"), nil
	}
	return filepath.Join(home, ".local", "share", "breadbox", "backups"), nil
}

func newBackupCreateCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Run pg_dump and write a gzipped SQL backup",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := bbconfig.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			if cfg.DatabaseURL == "" {
				return fmt.Errorf("DATABASE_URL is not set; required for pg_dump")
			}
			dir := strings.TrimSpace(out)
			if dir == "" {
				dir, err = defaultBackupDir()
				if err != nil {
					return err
				}
			}
			// If --out points at a file, accept it as the destination path;
			// otherwise treat it as a directory and let BackupService name
			// the file.
			info, statErr := os.Stat(dir)
			if statErr == nil && !info.IsDir() {
				return fmt.Errorf("--out %q already exists; pass a directory", dir)
			}
			bs := service.NewBackupService(cfg.DatabaseURL, dir, newCLIBackupLogger())
			name, err := bs.CreateBackup(cmd.Context(), "manual")
			if err != nil {
				return err
			}
			full := filepath.Join(dir, name)
			fmt.Println(full)
			return nil
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "directory to write the backup into (default: ~/.local/share/breadbox/backups)")
	return cmd
}

func newBackupListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List backups in the local backup directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := Flags(cmd)
			dir, err := defaultBackupDir()
			if err != nil {
				return err
			}
			bs := service.NewBackupService("", dir, newCLIBackupLogger())
			backups, err := bs.ListBackups()
			if err != nil {
				return err
			}
			switch output.Resolve(flags.JSON, flags.NDJSON, os.Stdout) {
			case output.ModeJSON:
				return output.PrintJSON(os.Stdout, backups)
			case output.ModeNDJSON:
				items := make([]any, 0, len(backups))
				for _, b := range backups {
					items = append(items, b)
				}
				return output.PrintNDJSON(os.Stdout, items)
			default:
				tbl := output.NewTable(os.Stdout, []string{"FILENAME", "SIZE", "CREATED_AT", "TRIGGER"})
				for _, b := range backups {
					tbl.AddRow(b.Filename, fmt.Sprintf("%d", b.Size), b.CreatedAt.Format(time.RFC3339), b.Trigger)
				}
				return tbl.Flush()
			}
		},
	}
	return cmd
}

func newBackupRestoreCmd() *cobra.Command {
	var (
		yes     bool
		baseDir string
	)
	cmd := &cobra.Command{
		Use:   "restore <file>",
		Short: "Restore the database from a backup file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := bbconfig.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			if cfg.DatabaseURL == "" {
				return fmt.Errorf("DATABASE_URL is not set; required for psql restore")
			}
			if running, addr := serverLikelyRunning(cfg); running {
				if !yes {
					return fmt.Errorf("a process is listening on %s — stop the server and re-run with --yes", addr)
				}
				fmt.Fprintf(os.Stderr, "warning: a process is listening on %s; proceeding because --yes was set\n", addr)
			}
			if !yes {
				return fmt.Errorf("restore is destructive; re-run with --yes to confirm")
			}
			dir := strings.TrimSpace(baseDir)
			if dir == "" {
				dir, err = defaultBackupDir()
				if err != nil {
					return err
				}
			}
			// Accept either a bare filename (resolved relative to the
			// backups dir) or a full path.
			target := args[0]
			if !strings.ContainsAny(target, "/\\") {
				target = filepath.Join(dir, target)
			}
			if _, err := os.Stat(target); err != nil {
				return fmt.Errorf("open backup file: %w", err)
			}
			bs := service.NewBackupService(cfg.DatabaseURL, filepath.Dir(target), newCLIBackupLogger())
			if err := bs.RestoreBackup(cmd.Context(), filepath.Base(target)); err != nil {
				return err
			}
			fmt.Println("restore complete")
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm the restore (required because it overwrites data)")
	cmd.Flags().StringVar(&baseDir, "dir", "", "directory the backup file lives in (defaults to the same XDG path as `backup list`)")
	return cmd
}

// serverLikelyRunning probes the configured SERVER_PORT (default 8080) for a
// listener and returns true when something answers. We don't try to be
// clever — false positives just nudge the user to pass --yes.
func serverLikelyRunning(cfg *bbconfig.Config) (bool, string) {
	port := strings.TrimSpace(cfg.ServerPort)
	if port == "" {
		port = "8080"
	}
	addr := net.JoinHostPort("127.0.0.1", port)
	conn, err := net.DialTimeout("tcp", addr, 250*time.Millisecond)
	if err != nil {
		return false, addr
	}
	_ = conn.Close()
	return true, addr
}

// newCLIBackupLogger returns a slog.Logger that discards everything. The
// BackupService API takes *slog.Logger; CLI flows don't want structured
// log noise mixed with the user-facing output, so we hand it a no-op.
func newCLIBackupLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
