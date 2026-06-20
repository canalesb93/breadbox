//go:build !lite

package service

import (
	"context"
	"os"
	"strings"

	"breadbox/internal/appconfig"
)

// CounterpartyLogoSettings resolves whether counterparty brand-logo hotlinking
// (via logo.dev) is enabled and the optional publishable token to authenticate
// the hotlinks, honoring the project-wide precedence env → app_config → default:
//
//   - enabled: BREADBOX_COUNTERPARTY_LOGOS env ("true"/"1" on, else off) wins;
//     otherwise the counterparty_logos app_config row; otherwise true.
//   - token: LOGO_DEV_TOKEN env wins; otherwise the logo_dev_token app_config
//     row; otherwise "". The token is a logo.dev publishable key (public by
//     design — it rides in the <img src>), so it is stored in plaintext.
//
// The admin counterparty handlers call this once per request and hand the
// resolved (enabled, token) to components.LogoDevURL when building each row's
// avatar, so a single read governs the whole page.
func (s *Service) CounterpartyLogoSettings(ctx context.Context) (enabled bool, token string) {
	if v, ok := os.LookupEnv("BREADBOX_COUNTERPARTY_LOGOS"); ok {
		t := strings.TrimSpace(v)
		enabled = strings.EqualFold(t, "true") || t == "1"
	} else {
		enabled = appconfig.Bool(ctx, s.Queries, appconfig.KeyCounterpartyLogos, true)
	}

	if v := strings.TrimSpace(os.Getenv("LOGO_DEV_TOKEN")); v != "" {
		token = v
	} else {
		token = appconfig.String(ctx, s.Queries, appconfig.KeyLogoDevToken, "")
	}
	return enabled, token
}
