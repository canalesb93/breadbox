//go:build !lite

package service

import (
	"context"
	"time"

	"breadbox/internal/appconfig"
	"breadbox/internal/db"
	"breadbox/internal/pgconv"
)

// WorkflowsConsentAcknowledged reports whether the household has already
// acknowledged that enabling a workflow runs Claude over their financial
// ledger (incurring Anthropic API cost). The acknowledgement is a
// one-time, household-global gate — once given, the configure drawer
// stops showing the consent checkbox. Best-effort: a read error reads as
// "not acknowledged" so the safe (consent-required) path wins.
func (s *Service) WorkflowsConsentAcknowledged(ctx context.Context) bool {
	return appconfig.String(ctx, s.Queries, appconfig.KeyWorkflowsConsentAckAt, "") != ""
}

// AcknowledgeWorkflowsConsent records the household's first-enable consent
// acknowledgement as an RFC3339 timestamp. Idempotent — re-acknowledging
// just refreshes the timestamp.
func (s *Service) AcknowledgeWorkflowsConsent(ctx context.Context) error {
	return s.Queries.SetAppConfig(ctx, db.SetAppConfigParams{
		Key:   appconfig.KeyWorkflowsConsentAckAt,
		Value: pgconv.Text(time.Now().UTC().Format(time.RFC3339)),
	})
}
