//go:build !lite

package service

import "context"

// WorkflowMCPView is the compact, agent-facing shape of one enabled workflow.
// It is a deliberate subset of AgentDefinitionResponse — just the fields an
// agent needs to understand "what automation exists and how is it doing" —
// so the list_workflows MCP tool stays lean on tokens. Hand-authored agents
// (no source_template) are excluded; this surface is workflow-presets only.
type WorkflowMCPView struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
	// Preset is the workflow-preset slug this workflow was instantiated from
	// (source_template). Always set — non-preset agents are filtered out.
	Preset string `json:"preset"`
	// Enabled is the workflow's own run toggle (a workflow can be instantiated
	// but paused).
	Enabled bool `json:"enabled"`
	// Trigger names how the workflow fires: "sync" (after each successful
	// sync), "schedule" (cron), or "manual" (neither configured).
	Trigger string `json:"trigger"`
	// ScheduleCron is the cron expression when Trigger == "schedule"; nil
	// otherwise.
	ScheduleCron *string `json:"schedule_cron,omitempty"`
	// ToolScope is the run's permission floor: "read_only" | "read_write".
	ToolScope string `json:"tool_scope"`
	// LastRunStatus is the status of the most recent run ("success" | "error"
	// | "in_progress" | "skipped" | "timeout"), or nil when the workflow has
	// never run.
	LastRunStatus *string `json:"last_run_status,omitempty"`
	// LastRunAt is the RFC3339 start time of the most recent run, or nil.
	LastRunAt *string `json:"last_run_at,omitempty"`
}

// WorkflowPresetMCPView is the agent-facing shape of one available preset in
// the gallery — what an agent could enable. Mirrors the subset of
// WorkflowPresetView relevant to "what automations are available".
type WorkflowPresetMCPView struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Description string `json:"description"`
	ToolScope   string `json:"tool_scope"`
	// Trigger names how the preset fires once enabled: "sync" | "schedule".
	Trigger string `json:"trigger"`
	// ScheduleCron is the preset's default cron when Trigger == "schedule".
	ScheduleCron string `json:"schedule_cron,omitempty"`
	// Enabled is true when this preset has already been instantiated as a
	// workflow.
	Enabled bool `json:"enabled"`
}

// WorkflowsMCPResult is the list_workflows tool payload: the household's
// enabled workflows plus the full catalog of available presets.
type WorkflowsMCPResult struct {
	Workflows []WorkflowMCPView       `json:"workflows"`
	Presets   []WorkflowPresetMCPView `json:"presets"`
}

// ListWorkflowsForMCP assembles the list_workflows MCP payload. It reuses the
// existing ListWorkflowPresets (catalog + enablement state) and
// ListAgentDefinitions (enabled workflow detail + last-run summary), reshaping
// both into the compact agent-facing views. Hand-authored agents (no
// source_template) are excluded — this is the Workflows surface.
func (s *Service) ListWorkflowsForMCP(ctx context.Context) (*WorkflowsMCPResult, error) {
	defs, err := s.ListAgentDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	workflows := make([]WorkflowMCPView, 0, len(defs))
	for _, d := range defs {
		// Only preset-instantiated workflows belong on this surface.
		if d.SourceTemplate == nil {
			continue
		}
		view := WorkflowMCPView{
			Name:         d.Name,
			Slug:         d.Slug,
			Preset:       *d.SourceTemplate,
			Enabled:      d.Enabled,
			Trigger:      workflowTriggerLabel(d.TriggerOnSyncComplete, d.ScheduleCron),
			ScheduleCron: d.ScheduleCron,
			ToolScope:    d.ToolScope,
		}
		if d.LastRun != nil {
			status := d.LastRun.Status
			view.LastRunStatus = &status
			startedAt := d.LastRun.StartedAt
			view.LastRunAt = &startedAt
		}
		workflows = append(workflows, view)
	}

	views, err := s.ListWorkflowPresets(ctx)
	if err != nil {
		return nil, err
	}
	presets := make([]WorkflowPresetMCPView, 0, len(views))
	for _, v := range views {
		var cron *string
		if v.ScheduleCron != "" {
			c := v.ScheduleCron
			cron = &c
		}
		presets = append(presets, WorkflowPresetMCPView{
			Slug:         v.Slug,
			Name:         v.Name,
			Category:     v.Category,
			Description:  v.Description,
			ToolScope:    v.ToolScope,
			Trigger:      workflowTriggerLabel(v.TriggerOnSyncComplete, cron),
			ScheduleCron: v.ScheduleCron,
			Enabled:      v.Enabled,
		})
	}

	return &WorkflowsMCPResult{Workflows: workflows, Presets: presets}, nil
}

// workflowTriggerLabel collapses the (trigger_on_sync_complete, schedule_cron)
// pair into a single label an agent can read: post-sync wins over a cron when
// both are somehow set; "manual" when neither fires the workflow on its own.
func workflowTriggerLabel(onSync bool, cron *string) string {
	switch {
	case onSync:
		return "sync"
	case cron != nil && *cron != "":
		return "schedule"
	default:
		return "manual"
	}
}
