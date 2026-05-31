//go:build !lite

package service

import (
	"context"
	"fmt"
	"time"

	"breadbox/internal/appconfig"
	"breadbox/internal/db"
	"breadbox/internal/pgconv"
)

// HouseholdCeilingWindow is the rolling window over which the household
// spend ceiling (KeyAgentGlobalMaxBudgetUSD) is measured. A rolling
// window (vs a calendar month) avoids a reset-day spend spike.
const HouseholdCeilingWindow = 30 * 24 * time.Hour

// PostSyncDebounceWindow coalesces rapid post-sync triggers: when a
// workflow already ran (non-skipped) within this window, a fresh
// sync-complete event for it is debounced rather than fanning out to
// another run. Sync can fire frequently (webhook bursts, manual
// re-syncs); the next run still picks up everything synced since, so
// coalescing loses no coverage. Hardcoded — not a knob users tune.
const PostSyncDebounceWindow = 15 * time.Minute

// RecentRunExistsForDefinition reports whether a non-skipped run for the
// definition started at/after `since`. Drives the post-sync debounce.
func (s *Service) RecentRunExistsForDefinition(ctx context.Context, defID string, since time.Time) (bool, error) {
	uid, err := pgconv.ParseUUID(defID)
	if err != nil {
		return false, fmt.Errorf("recent run exists: parse def id: %w", err)
	}
	return s.Queries.ExistsRecentRunForDefinition(ctx, db.ExistsRecentRunForDefinitionParams{
		DefinitionID: uid,
		Since:        pgconv.Timestamptz(since),
	})
}

// HouseholdCostSince sums total_cost_usd across every workflow/agent run
// started at/after `since` (skipped rows excluded). Powers the spend
// ceiling gate and the settings spend display.
func (s *Service) HouseholdCostSince(ctx context.Context, since time.Time) (float64, error) {
	raw, err := s.Queries.GetHouseholdCostSince(ctx, pgconv.Timestamptz(since))
	if err != nil {
		return 0, fmt.Errorf("household cost since %s: %w", since.Format(time.RFC3339), err)
	}
	v, _ := pgconv.NumericToFloat(raw)
	return v, nil
}

// HouseholdSpendStatus is the rolling-window spend snapshot for the
// settings display: how much has been spent in the window and the active
// ceiling (nil = no cap).
type HouseholdSpendStatus struct {
	WindowDays int
	SpentUSD   float64
	CeilingUSD *float64
}

// HouseholdSpendStatus returns the current rolling-window spend + the
// configured ceiling for display in agent/workflow settings.
func (s *Service) HouseholdSpendStatus(ctx context.Context) (HouseholdSpendStatus, error) {
	spent, err := s.HouseholdCostSince(ctx, time.Now().Add(-HouseholdCeilingWindow))
	if err != nil {
		return HouseholdSpendStatus{WindowDays: 30}, err
	}
	return HouseholdSpendStatus{
		WindowDays: 30,
		SpentUSD:   spent,
		CeilingUSD: readOptionalFloat(ctx, s.Queries, appconfig.KeyAgentGlobalMaxBudgetUSD),
	}, nil
}

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
