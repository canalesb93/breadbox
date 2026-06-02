//go:build !lite

package service

import "context"

// GetEnabledWorkflowLastRuns returns each preset-sourced workflow's
// most-recent run, keyed by slug. Workflows with no runs are omitted.
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
