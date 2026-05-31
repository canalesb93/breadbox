//go:build !lite

package service

import "context"

// GetEnabledWorkflowLastRuns returns the most-recent run for every workflow
// instantiated from a preset, keyed by the workflow's slug. It reuses the
// existing per-definition last-run query that ListAgentDefinitions already
// inlines (GetLatestAgentRun via lastRunSummary) — there's no extra query
// here, just a projection of the catalog into a slug → last-run map the
// gallery can render per enabled card.
//
// Definitions with no run history (instantiated but never run) are omitted
// from the map; callers treat a missing key as "never run". Only
// preset-sourced definitions (SourceTemplate != nil) are included — the
// gallery only renders cards for presets, so hand-authored agents are
// irrelevant here.
func (s *Service) GetEnabledWorkflowLastRuns(ctx context.Context) (map[string]*AgentRunSummary, error) {
	defs, err := s.ListAgentDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*AgentRunSummary, len(defs))
	for _, d := range defs {
		if d.SourceTemplate == nil || d.LastRun == nil {
			continue
		}
		out[d.Slug] = d.LastRun
	}
	return out, nil
}
