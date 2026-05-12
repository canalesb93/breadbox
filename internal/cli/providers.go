// Package cli — providers noun group.
//
// `breadbox providers ...` reads the provider registry, writes Plaid/Teller
// credentials, runs round-trip credential checks, and disables a provider by
// clearing its app_config rows. All verbs route through the REST API; no
// service-layer side door.
package cli

import (
	"fmt"
	"os"

	"breadbox/internal/cli/output"
	"breadbox/internal/client"

	"github.com/spf13/cobra"
)

// AddProvidersCmd registers `breadbox providers` and its children.
func AddProvidersCmd(root *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "providers",
		Short: "Manage bank-data providers (Plaid, Teller, CSV)",
	}
	MarkRequiresHost(cmd)

	cmd.AddCommand(newProvidersListCmd())
	cmd.AddCommand(newProvidersConfigCmd())
	cmd.AddCommand(newProvidersTestCmd())
	cmd.AddCommand(newProvidersDisableCmd())

	root.AddCommand(cmd)
}

func newProvidersListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured providers and their status",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			providers, err := c.ListProviders(cmd.Context())
			if err != nil {
				return err
			}
			cfg, err := c.GetProviderConfig(cmd.Context())
			if err != nil {
				// /settings/providers isn't strictly required — surface the
				// list anyway so callers can still see which providers exist.
				return renderProvidersList(flags, providers, nil)
			}
			return renderProvidersList(flags, providers, cfg)
		},
	}
}

func newProvidersConfigCmd() *cobra.Command {
	cfg := &cobra.Command{
		Use:   "config",
		Short: "Set provider credentials (plaid | teller)",
	}
	cfg.AddCommand(newProvidersConfigPlaidCmd())
	cfg.AddCommand(newProvidersConfigTellerCmd())
	return cfg
}

func newProvidersConfigPlaidCmd() *cobra.Command {
	var (
		clientID    string
		secret      string
		env         string
		webhookURL  string
		secretStdin bool
	)
	cmd := &cobra.Command{
		Use:   "plaid",
		Short: "Set Plaid credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)

			if clientID == "" {
				return UsageErrorf("--client-id is required")
			}
			p := client.UpdatePlaidParams{
				ClientID:    clientID,
				Environment: env,
				WebhookURL:  webhookURL,
			}
			if cmd.Flags().Changed("secret") {
				v := secret
				p.Secret = &v
			}
			res, err := c.UpdatePlaidConfig(cmd.Context(), p)
			if err != nil {
				return err
			}
			if flags.Quiet {
				return nil
			}
			return output.PrintJSON(os.Stdout, res)
		},
	}
	cmd.Flags().StringVar(&clientID, "client-id", "", "Plaid client_id (required)")
	cmd.Flags().StringVar(&secret, "secret", "", "Plaid secret (omit to keep existing)")
	cmd.Flags().StringVar(&env, "env", "", "Plaid environment: sandbox | development | production")
	cmd.Flags().StringVar(&webhookURL, "webhook-url", "", "Public HTTPS URL for Plaid webhooks (optional)")
	cmd.Flags().BoolVar(&secretStdin, "secret-stdin", false, "(reserved) read --secret value from stdin")
	return cmd
}

func newProvidersConfigTellerCmd() *cobra.Command {
	var (
		appID         string
		signingSecret string
		pemFile       string
		env           string
	)
	cmd := &cobra.Command{
		Use:   "teller",
		Short: "Set Teller credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)

			if appID == "" {
				return UsageErrorf("--app-id is required")
			}
			p := client.UpdateTellerParams{
				ApplicationID: appID,
				Environment:   env,
			}
			if cmd.Flags().Changed("signing-secret") {
				v := signingSecret
				p.WebhookSecret = &v
			}
			if pemFile != "" {
				cert, key, err := splitTellerPEM(pemFile)
				if err != nil {
					return err
				}
				p.Certificate = &cert
				p.PrivateKey = &key
			}
			res, err := c.UpdateTellerConfig(cmd.Context(), p)
			if err != nil {
				return err
			}
			if flags.Quiet {
				return nil
			}
			return output.PrintJSON(os.Stdout, res)
		},
	}
	cmd.Flags().StringVar(&appID, "app-id", "", "Teller application_id (required)")
	cmd.Flags().StringVar(&signingSecret, "signing-secret", "", "Teller webhook signing secret (omit to keep existing)")
	cmd.Flags().StringVar(&pemFile, "pem-file", "", "path to a combined cert+key PEM file (cert block first, then key)")
	cmd.Flags().StringVar(&env, "env", "", "Teller environment: sandbox | development | production")
	return cmd
}

func newProvidersTestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test <provider>",
		Short: "Run a server-side credentials check",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			res, err := c.TestProvider(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if flags.JSON || flags.NDJSON {
				return output.PrintJSON(os.Stdout, res)
			}
			if res.OK {
				fmt.Printf("%s: ok\n", res.Provider)
				return nil
			}
			fmt.Printf("%s: FAILED (%s)\n", res.Provider, res.Message)
			return UsageErrorf("provider credentials check failed")
		},
	}
	return cmd
}

func newProvidersDisableCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disable <provider>",
		Short: "Disable a provider (clears credentials; existing connections kept)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _ := ClientFromContext(cmd.Context())
			flags := Flags(cmd)
			res, err := c.DisableProvider(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if flags.Quiet {
				return nil
			}
			if flags.JSON || flags.NDJSON {
				return output.PrintJSON(os.Stdout, res)
			}
			fmt.Printf("%s: %s\n", res.Provider, res.Message)
			return nil
		},
	}
	return cmd
}

// renderProvidersList renders the registry as a table (configured + status).
// cfg may be nil when the credential view couldn't be fetched.
func renderProvidersList(flags *FlagBag, providers []client.ProviderInfo, cfg *client.ProviderConfigView) error {
	switch output.Resolve(flags.JSON, flags.NDJSON, os.Stdout) {
	case output.ModeJSON:
		payload := map[string]any{"providers": providers}
		if cfg != nil {
			payload["credentials"] = cfg
		}
		return output.PrintJSON(os.Stdout, payload)
	case output.ModeNDJSON:
		items := make([]any, 0, len(providers))
		for _, p := range providers {
			items = append(items, p)
		}
		return output.PrintNDJSON(os.Stdout, items)
	default:
		tbl := output.NewTable(os.Stdout, []string{"NAME", "CONFIGURED", "NEEDS_LINK", "CAPABILITIES", "SOURCE"})
		for _, p := range providers {
			src := "-"
			switch p.Name {
			case "plaid":
				if cfg != nil {
					switch {
					case cfg.Plaid.FromEnv:
						src = "env"
					case cfg.Plaid.Configured:
						src = "db"
					}
				}
			case "teller":
				if cfg != nil {
					switch {
					case cfg.Teller.FromEnv:
						src = "env"
					case cfg.Teller.Configured:
						src = "db"
					}
				}
			case "csv":
				src = "builtin"
			}
			caps := ""
			for i, cp := range p.Capabilities {
				if i > 0 {
					caps += ","
				}
				caps += cp
			}
			if caps == "" {
				caps = "-"
			}
			tbl.AddRow(p.Name, fmt.Sprintf("%v", p.Configured), fmt.Sprintf("%v", p.NeedsLinkSession), caps, src)
		}
		return tbl.Flush()
	}
}

// splitTellerPEM reads a PEM bundle from disk and splits it into cert + key
// strings. The convention is `CERTIFICATE` block(s) followed by a single
// `PRIVATE KEY` (or `RSA PRIVATE KEY` / `EC PRIVATE KEY`) block.
func splitTellerPEM(path string) (cert, key string, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("read pem-file %s: %w", path, err)
	}
	s := string(raw)
	keyMarker := "-----BEGIN "
	// Find the first PRIVATE KEY block.
	keyIdx := -1
	for _, m := range []string{"-----BEGIN PRIVATE KEY-----", "-----BEGIN RSA PRIVATE KEY-----", "-----BEGIN EC PRIVATE KEY-----"} {
		if i := indexOf(s, m); i >= 0 && (keyIdx < 0 || i < keyIdx) {
			keyIdx = i
		}
	}
	if keyIdx < 0 {
		// Allow cert-only PEM by returning the whole thing as the cert. The
		// server will reject it with a clear error.
		return s, "", nil
	}
	// Find the end of the key block.
	keyEnd := indexOf(s[keyIdx:], "-----END")
	if keyEnd < 0 {
		return "", "", fmt.Errorf("pem-file %s: unterminated PRIVATE KEY block", path)
	}
	// Walk forward to the line break after the END marker.
	tail := s[keyIdx+keyEnd:]
	lineEnd := indexOf(tail, "\n")
	if lineEnd < 0 {
		key = s[keyIdx:]
	} else {
		key = s[keyIdx : keyIdx+keyEnd+lineEnd+1]
	}
	cert = s[:keyIdx]
	// Trim leading whitespace before the first CERTIFICATE block so the
	// server sees clean PEM bytes.
	if i := indexOf(cert, keyMarker); i > 0 {
		cert = cert[i:]
	}
	return cert, key, nil
}

// indexOf is strings.Index inlined to avoid a separate import in this file.
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
