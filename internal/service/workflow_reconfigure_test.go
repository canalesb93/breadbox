//go:build !lite

package service

import "testing"

// F3ReconfigureChoiceWithDirective returns a preset whose single option has a
// directive-bearing choice, so the decompose/derive round-trip can be tested
// without a DB. It mirrors the shape of applyModeOption.
func f3ReconfigurePresetFixture() WorkflowPreset {
	return WorkflowPreset{
		Slug:     "f3-recon-fixture",
		Name:     "F3 Recon Fixture",
		Category: "Test",
		Options: []WorkflowPresetOption{
			{
				Key:     "apply_mode",
				Label:   "Apply mode",
				Default: "auto",
				Choices: []WorkflowPresetChoice{
					{Value: "auto", Label: "Auto", Directive: ""},
					{Value: "flag_only", Label: "Flag only", Directive: "FLAG ONLY: do not categorize."},
				},
			},
		},
	}
}

func TestF3ReconfigureSplitAdditionalInstructions(t *testing.T) {
	cases := []struct {
		name      string
		prompt    string
		wantInstr string
		wantBody  string
	}{
		{
			name:      "no tail",
			prompt:    "base prompt body",
			wantInstr: "",
			wantBody:  "base prompt body",
		},
		{
			name:      "with tail",
			prompt:    "base body" + additionalInstructionsHeading + "watch dining out",
			wantInstr: "watch dining out",
			wantBody:  "base body",
		},
		{
			name:      "trims tail whitespace",
			prompt:    "base" + additionalInstructionsHeading + "  spaced  \n",
			wantInstr: "spaced",
			wantBody:  "base",
		},
		{
			name:      "last marker wins",
			prompt:    "x" + additionalInstructionsHeading + "first" + additionalInstructionsHeading + "second",
			wantInstr: "second",
			wantBody:  "x" + additionalInstructionsHeading + "first",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotInstr, gotBody := splitAdditionalInstructions(tc.prompt)
			if gotInstr != tc.wantInstr {
				t.Errorf("instructions = %q, want %q", gotInstr, tc.wantInstr)
			}
			if gotBody != tc.wantBody {
				t.Errorf("body = %q, want %q", gotBody, tc.wantBody)
			}
		})
	}
}

func TestF3ReconfigureDeriveChosenOptions(t *testing.T) {
	preset := f3ReconfigurePresetFixture()

	// A body containing the flag_only directive resolves to flag_only.
	flagBody := "base\n\n## Apply mode\n\nFLAG ONLY: do not categorize."
	got := deriveChosenOptions(preset, flagBody)
	if got["apply_mode"] != "flag_only" {
		t.Fatalf("apply_mode = %q, want flag_only", got["apply_mode"])
	}

	// A body with no directive falls back to the option default (auto).
	got = deriveChosenOptions(preset, "base prompt with no directives")
	if got["apply_mode"] != "auto" {
		t.Fatalf("apply_mode = %q, want auto (default)", got["apply_mode"])
	}
}

func TestF3ReconfigureComposeWorkflowPrompt_TooLong(t *testing.T) {
	preset := f3ReconfigurePresetFixture()
	long := make([]byte, maxAdditionalInstructions+1)
	for i := range long {
		long[i] = 'a'
	}
	// composePresetPrompt will fail first on the empty block list of the
	// fixture, so give the fixture a real block to isolate the length check.
	preset.PromptBlocks = workflowPresets[0].PromptBlocks
	if _, err := composeWorkflowPrompt(preset, nil, string(long)); err == nil {
		t.Fatal("composeWorkflowPrompt accepted over-long additional instructions")
	}
}

func TestF3ReconfigureComposeWorkflowPrompt_RoundTrip(t *testing.T) {
	preset := f3ReconfigurePresetFixture()
	preset.PromptBlocks = workflowPresets[0].PromptBlocks

	prompt, err := composeWorkflowPrompt(preset, map[string]string{"apply_mode": "flag_only"}, "be brief")
	if err != nil {
		t.Fatalf("composeWorkflowPrompt: %v", err)
	}

	instr, body := splitAdditionalInstructions(prompt)
	if instr != "be brief" {
		t.Fatalf("round-trip instructions = %q, want %q", instr, "be brief")
	}
	if got := deriveChosenOptions(preset, body)["apply_mode"]; got != "flag_only" {
		t.Fatalf("round-trip apply_mode = %q, want flag_only", got)
	}
}
