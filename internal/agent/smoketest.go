//go:build !lite

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"breadbox/internal/appconfig"
)

// SmokeResult is the outcome of a single SmokeTest invocation. Used by
// `breadbox agent test`, `breadbox doctor`, and the admin Settings →
// Agents diagnostics panel (which reads the snake_case JSON keys
// below via Alpine x-text bindings).
type SmokeResult struct {
	AuthMode      string  `json:"auth_mode"`      // "subscription" | "api_key"
	BinaryPath    string  `json:"binary_path"`    // resolved path to breadbox-agent
	Model         string  `json:"model"`          // model that ran
	DurationMs    int64   `json:"duration_ms"`
	TotalCostUSD  float64 `json:"total_cost_usd"`
	InputTokens   int     `json:"input_tokens"`
	OutputTokens  int     `json:"output_tokens"`
	StopReason    string  `json:"stop_reason"`
	AssistantText string  `json:"response"` // first text block from the assistant
}

// SmokeTestPrompt is the tiny diagnostic prompt sent to the model. Deliberately
// trivial so a single turn satisfies it — keeps the cost in the fractional-cent
// range across all model sizes.
const SmokeTestPrompt = "Reply with the single word OK and nothing else."

// SmokeTestMaxTurns is the turn cap for the diagnostic. Should never need
// more than 1; setting 2 leaves a tiny margin if the SDK does an internal
// rewrite.
const SmokeTestMaxTurns = 2

// SmokeTestMaxBudgetUSD is the budget cap for the diagnostic. 5¢ is well
// above what a one-token "OK" response should cost across any Claude model.
const SmokeTestMaxBudgetUSD = 0.05

// SmokeAuthReader is the minimal appconfig surface SmokeTest needs.
// *db.Queries satisfies it.
type SmokeAuthReader interface {
	appconfig.Reader
}

// SmokeTest runs a single tiny prompt through the sidecar with no MCP
// servers attached. Use it to validate the full chain — auth config →
// binary discovery → sidecar spawn → SDK call → result — without minting
// an API key, registering an agent_definition, or writing an agent_runs
// row.
//
// Returns a populated SmokeResult on success. Returns ErrAuthNotConfigured
// (wrapped) when no token is set, ErrBinaryNotFound when the sidecar
// binary can't be located, or whatever the runner produces on a real
// failure.
//
// `runner` is normally a *Sidecar; tests inject a RunnerFunc.
func SmokeTest(ctx context.Context, r SmokeAuthReader, encKey []byte, runner Runner, sidecarPath string) (*SmokeResult, error) {
	authMode := appconfig.String(ctx, r, appconfig.KeyAgentAuthMode, appconfig.AuthModeSubscription)

	var token string
	switch authMode {
	case appconfig.AuthModeSubscription:
		t, ok, err := appconfig.ReadEncrypted(ctx, r, appconfig.KeyAgentSubscriptionToken, encKey)
		if err != nil {
			return nil, fmt.Errorf("smoke: read subscription token: %w", err)
		}
		if !ok {
			return nil, fmt.Errorf("%w (auth_mode=subscription, no token in app_config — paste one via Settings → Agents or set it with `breadbox config set %s <value>`)", ErrAuthNotConfigured, appconfig.KeyAgentSubscriptionToken)
		}
		token = t
	case appconfig.AuthModeAPIKey:
		t, ok, err := appconfig.ReadEncrypted(ctx, r, appconfig.KeyAgentAnthropicAPIKey, encKey)
		if err != nil {
			return nil, fmt.Errorf("smoke: read api key: %w", err)
		}
		if !ok {
			return nil, fmt.Errorf("%w (auth_mode=api_key, no key in app_config — paste one via Settings → Agents)", ErrAuthNotConfigured)
		}
		token = t
	default:
		return nil, fmt.Errorf("smoke: unknown auth_mode %q in app_config", authMode)
	}

	model := "claude-haiku-4-5" // smallest available — cost optimization for diagnostics
	spec := JobSpec{
		RunID:        "smoketest",
		Prompt:       SmokeTestPrompt,
		Model:        model,
		MaxTurns:     SmokeTestMaxTurns,
		MaxBudgetUsd: SmokeTestMaxBudgetUSD,
		ToolScope:    "read_only",
		AllowedTools: []string{},                  // no tools
		MCPServers:   map[string]MCPServerConfig{}, // no MCP servers — pure round-trip
		Auth: AuthConfig{
			Mode:  authMode,
			Token: token,
		},
	}

	// Capture assistant text from the event stream — sidecar.go does NOT
	// expose the assistant content directly on RunResult, so we sniff it
	// via the event handler.
	var assistantText strings.Builder
	handler := func(ev Event) error {
		if ev.Type != EventTypeAssistantMessage {
			return nil
		}
		// The assistant_message data shape is pass-through from the SDK:
		// {"type":"assistant_message","ts":...,"data":{"message":{"role":"assistant","content":[{"type":"text","text":"OK"}]}}}
		// We extract via best-effort substring; failing that, leave it
		// empty and let the caller fall back to a generic "success" line.
		var probe struct {
			Data struct {
				Message struct {
					Content []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
				} `json:"message"`
			} `json:"data"`
		}
		if jerr := json.Unmarshal(ev.Raw, &probe); jerr != nil {
			return nil
		}
		for _, c := range probe.Data.Message.Content {
			if c.Type == "text" {
				if assistantText.Len() > 0 {
					assistantText.WriteString("\n")
				}
				assistantText.WriteString(c.Text)
			}
		}
		return nil
	}

	result, err := runner.Run(ctx, spec, handler)
	if err != nil {
		if errors.Is(err, ErrBinaryNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("smoke: run sidecar: %w", err)
	}

	// Resolve the binary path the runner actually used so the diagnostic
	// surface shows where the sidecar was discovered (per-user install,
	// PATH lookup, dev `./bin/breadbox-agent`, etc.) instead of the
	// empty appconfig override.
	resolvedBinary, _ := LocateBinary(sidecarPath)

	return &SmokeResult{
		AuthMode:      authMode,
		BinaryPath:    resolvedBinary,
		Model:         model,
		DurationMs:    result.DurationMs,
		TotalCostUSD:  result.TotalCostUSD,
		InputTokens:   result.InputTokens,
		OutputTokens:  result.OutputTokens,
		StopReason:    "", // SDK doesn't surface this directly on the Go side; transcripts have it
		AssistantText: strings.TrimSpace(assistantText.String()),
	}, nil
}

