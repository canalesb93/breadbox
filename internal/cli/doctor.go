package cli

import (
	"context"
	"fmt"
	"os"

	"breadbox/internal/cli/config"
	"breadbox/internal/cli/output"
	"breadbox/internal/client"

	"github.com/spf13/cobra"
)

// Check statuses for the doctor command.
const (
	doctorStatusPass = "pass"
	doctorStatusFail = "fail"
	doctorStatusSkip = "skip"
	doctorStatusWarn = "warn"
)

// doctorCheck is one row of the local-mode preflight report.
type doctorCheck struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
}

type doctorReport struct {
	Checks []doctorCheck `json:"checks"`
	OK     bool          `json:"ok"`
}

// AddDoctorCmd registers `breadbox doctor`. The command has two modes:
//
//   - **Local** (default when no --host is configured): runs the same
//     environment + DB + provider preflight checks the legacy
//     `breadbox doctor` did before the cobra port. No HTTP call. Available
//     only in full server builds — under -tags=lite the local mode returns
//     an error pointing the operator at the remote mode.
//   - **Remote** (when --host is set or BREADBOX_HOST is in env): calls
//     `GET /api/v1/headless/bootstrap` and renders a readable report.
func AddDoctorCmd(root *cobra.Command) {
	var skipExternal bool
	var withLive bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Health check: local environment or remote /headless/bootstrap",
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := Flags(cmd)
			if flags.Host != "" || hostConfigured() {
				return runDoctorRemote(cmd.Context(), flags)
			}
			return runDoctorLocal(cmd.Context(), flags.JSON, skipExternal, withLive)
		},
	}
	cmd.Flags().BoolVar(&skipExternal, "skip-external", false, "skip DNS/HTTP reachability checks (local-mode only)")
	cmd.Flags().BoolVar(&withLive, "with-live", false, "additionally run a live agent smoke test (~$0.01, local-mode only)")
	root.AddCommand(cmd)
}

// hostConfigured reports whether any host exists in hosts.toml — used to
// decide whether `breadbox doctor` (no flags) defaults to local or remote.
func hostConfigured() bool {
	h, err := config.Load()
	if err != nil {
		return false
	}
	return len(h.Hosts) > 0
}

// runDoctorRemote calls the HTTP bootstrap endpoint and renders the report.
func runDoctorRemote(ctx context.Context, flags *FlagBag) error {
	hosts, err := config.Load()
	if err != nil {
		return fmt.Errorf("load hosts config: %w", err)
	}
	host, name, err := hosts.Get(flags.Host)
	if err != nil {
		return err
	}
	c := client.New(host, flags.Version)

	resp, err := c.HeadlessBootstrap(ctx)
	if err != nil {
		return err
	}

	if flags.JSON || flags.NDJSON {
		return output.PrintJSON(os.Stdout, resp)
	}
	renderRemoteDoctorReport(os.Stdout, name, host.BaseURL, resp)
	return nil
}

// renderRemoteDoctorReport prints a human-readable summary of the
// /api/v1/headless/bootstrap payload. Generic-map iteration is fine here
// — the shape is small and stable.
func renderRemoteDoctorReport(w *os.File, hostName, baseURL string, resp map[string]any) {
	fmt.Fprintf(w, "breadbox doctor — %s (%s)\n\n", hostName, baseURL)

	get := func(k string) any { return resp[k] }
	getBool := func(k string) bool { v, _ := resp[k].(bool); return v }
	getInt := func(k string) int64 {
		switch x := resp[k].(type) {
		case float64:
			return int64(x)
		case int64:
			return x
		}
		return 0
	}

	dbRaw, _ := get("database").(map[string]any)
	dbConn, _ := dbRaw["connected"].(bool)
	migCurrent, _ := dbRaw["migrations_current"].(bool)
	var migVer int64
	if v, ok := dbRaw["migration_version"].(float64); ok {
		migVer = int64(v)
	}

	mark := func(ok bool) string {
		if ok {
			return "ok  "
		}
		return "FAIL"
	}

	fmt.Fprintf(w, "  %s server reachable (v%v)\n", mark(true), get("version"))
	fmt.Fprintf(w, "  %s database connected\n", mark(dbConn))
	fmt.Fprintf(w, "  %s migrations current (v%d)\n", mark(migCurrent), migVer)
	fmt.Fprintf(w, "  %s encryption key set\n", mark(getBool("encryption_key_set")))
	fmt.Fprintf(w, "  -- %d household user(s)\n", getInt("users_count"))
	fmt.Fprintf(w, "  -- %d login account(s)\n", getInt("login_accounts_count"))
	fmt.Fprintf(w, "  -- %d API key(s)\n", getInt("api_keys_count"))
	fmt.Fprintf(w, "  -- %d active connection(s)\n", getInt("active_connections_count"))
	fmt.Fprintf(w, "  %s scheduler running\n", mark(getBool("scheduler_running")))

	provs, _ := get("providers").([]any)
	if len(provs) == 0 {
		fmt.Fprintln(w, "  !! no providers configured")
	} else {
		for _, p := range provs {
			pm, _ := p.(map[string]any)
			name, _ := pm["name"].(string)
			conf, _ := pm["configured"].(bool)
			env, _ := pm["env"].(string)
			label := "configured"
			if !conf {
				label = "not configured"
			} else if env != "" {
				label = fmt.Sprintf("configured (%s)", env)
			}
			fmt.Fprintf(w, "  %s provider %s — %s\n", mark(conf), name, label)
		}
	}

	if getBool("first_run") {
		fmt.Fprintln(w, "\nfirst_run = true: no login accounts yet; visit /setup or run `breadbox create-admin`.")
	}
}
