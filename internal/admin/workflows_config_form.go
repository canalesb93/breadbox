//go:build !headless && !lite

package admin

import (
	"net/http"
	"strconv"
	"strings"
)

// workflowFormConfig holds the shared configurable fields the setup +
// reconfigure drawers POST. Pointers mean "present in the form" so a
// reconfigure leaves an omitted field untouched.
type workflowFormConfig struct {
	TriggerOnSync *bool
	ScheduleCron  *string
	Model         *string
	MaxTurns      *int
	MaxBudgetUSD  *float64
}

// parseWorkflowFormConfig pulls the shared drawer fields (trigger, schedule,
// model, advanced caps) off the form and returns them plus the set of form
// keys it consumed — callers union this with their own control keys so the
// remaining fields can be treated as preset-specialized options.
func parseWorkflowFormConfig(r *http.Request) (workflowFormConfig, map[string]bool) {
	control := map[string]bool{
		"trigger_on_sync": true,
		"schedule_cron":   true,
		"model":           true,
		"max_turns":       true,
		"max_budget_usd":  true,
	}
	var c workflowFormConfig
	if v := r.FormValue("trigger_on_sync"); v != "" {
		b := v == "true"
		c.TriggerOnSync = &b
	}
	if v := strings.TrimSpace(r.FormValue("schedule_cron")); v != "" {
		c.ScheduleCron = &v
	}
	if v := strings.TrimSpace(r.FormValue("model")); v != "" {
		c.Model = &v
	}
	if v := strings.TrimSpace(r.FormValue("max_turns")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.MaxTurns = &n
		}
	}
	if v := strings.TrimSpace(r.FormValue("max_budget_usd")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			c.MaxBudgetUSD = &f
		}
	}
	return c, control
}
