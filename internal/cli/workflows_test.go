package cli

import (
	"testing"
)

// TestWorkflowSchedule_Manual asserts a nil or empty cron renders as the
// "manual" sentinel, matching the `breadbox agent list` column semantics.
func TestWorkflowSchedule_Manual(t *testing.T) {
	empty := ""
	cron := "0 9 * * *"
	cases := []struct {
		name string
		in   *string
		want string
	}{
		{"nil cron", nil, "manual"},
		{"empty cron", &empty, "manual"},
		{"real cron passes through", &cron, "0 9 * * *"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := workflowSchedule(tc.in); got != tc.want {
				t.Fatalf("workflowSchedule(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestFormatCostPtr renders an optional USD cost with a stable 4-decimal
// format and a "-" placeholder for nil.
func TestFormatCostPtr(t *testing.T) {
	cost := 0.1234
	if got := formatCostPtr(nil); got != "-" {
		t.Errorf("formatCostPtr(nil) = %q, want -", got)
	}
	if got := formatCostPtr(&cost); got != "$0.1234" {
		t.Errorf("formatCostPtr(&0.1234) = %q, want $0.1234", got)
	}
}

// TestWorkflowsCmd_Subcommands verifies the noun group wires exactly the
// three read subcommands (list, runs, presets) and that `runs` exposes the
// filter/pagination flags the global feed supports. Argument/flag wiring is
// the contract scripts depend on, so pin it down.
func TestWorkflowsCmd_Subcommands(t *testing.T) {
	root := NewRootCmd("test")
	wf, _, err := root.Find([]string{"workflows"})
	if err != nil {
		t.Fatalf("find workflows: %v", err)
	}
	if wf == nil || wf.Name() != "workflows" {
		t.Fatalf("workflows command not registered on root")
	}

	want := map[string]bool{"list": false, "runs": false, "presets": false}
	for _, sub := range wf.Commands() {
		if _, ok := want[sub.Name()]; ok {
			want[sub.Name()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("workflows subcommand %q not registered", name)
		}
	}

	runs, _, err := wf.Find([]string{"runs"})
	if err != nil {
		t.Fatalf("find workflows runs: %v", err)
	}
	for _, flag := range []string{"workflow", "status", "trigger", "offset"} {
		if runs.Flags().Lookup(flag) == nil {
			t.Errorf("workflows runs missing --%s flag", flag)
		}
	}
}
