//go:build !lite

package agent

import (
	"errors"
	"strings"
	"testing"
)

func TestRunError_Error_AndUnwrap(t *testing.T) {
	cases := []struct {
		name        string
		runErr      *RunError
		wantErrSub  string // substring required in .Error()
		wantNoSub   string // substring that must NOT appear
	}{
		{
			name:       "with stderr",
			runErr:     &RunError{Code: RunErrorCodeAuth, Message: "rejected", Stderr: "401"},
			wantErrSub: "agent: auth_error: rejected [stderr=401]",
		},
		{
			name:       "without stderr",
			runErr:     &RunError{Code: RunErrorCodeAPI, Message: "overloaded"},
			wantErrSub: "agent: api_error: overloaded",
			wantNoSub:  "stderr=",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.runErr.Error()
			if !strings.Contains(got, tc.wantErrSub) {
				t.Errorf("Error() = %q, want substring %q", got, tc.wantErrSub)
			}
			if tc.wantNoSub != "" && strings.Contains(got, tc.wantNoSub) {
				t.Errorf("Error() = %q, must NOT contain %q", got, tc.wantNoSub)
			}
			// errors.Is(err, ErrSidecarFailed) must still work for legacy callers.
			if !errors.Is(tc.runErr, ErrSidecarFailed) {
				t.Errorf("errors.Is(%v, ErrSidecarFailed) = false; legacy contract broken", tc.runErr)
			}
		})
	}
}

func TestClassifyRunError(t *testing.T) {
	cases := []struct {
		name      string
		event     ErrorPayload
		stderr    string
		wantCode  string
		wantInMsg string
	}{
		{
			name:      "structured auth code",
			event:     ErrorPayload{Code: "auth_error", Message: "invalid api key"},
			wantCode:  RunErrorCodeAuth,
			wantInMsg: "invalid api key",
		},
		{
			name:     "structured network code lowercase variant",
			event:    ErrorPayload{Code: "fetch_failed", Message: "DNS"},
			wantCode: RunErrorCodeNetwork,
		},
		{
			name:     "unrecognized code → unknown",
			event:    ErrorPayload{Code: "boop", Message: "wat"},
			wantCode: RunErrorCodeUnknown,
		},
		{
			name:     "no code, message says rate limit → api_error",
			event:    ErrorPayload{Message: "rate limit exceeded for org"},
			wantCode: RunErrorCodeAPI,
		},
		{
			name:     "no event at all, stderr says ENOTFOUND",
			stderr:   "fetch failed: ENOTFOUND api.anthropic.com",
			wantCode: RunErrorCodeNetwork,
		},
		{
			name:     "no event, no recognizable stderr → unknown",
			stderr:   "totally inscrutable",
			wantCode: RunErrorCodeUnknown,
		},
		{
			name:     "interrupted via SIGTERM in message",
			event:    ErrorPayload{Code: "sigterm", Message: "stopped"},
			wantCode: RunErrorCodeInterrupted,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyRunError(tc.event, tc.stderr)
			if got.Code != tc.wantCode {
				t.Errorf("Code = %q, want %q", got.Code, tc.wantCode)
			}
			if tc.wantInMsg != "" && !strings.Contains(got.Message, tc.wantInMsg) {
				t.Errorf("Message = %q, want substring %q", got.Message, tc.wantInMsg)
			}
		})
	}
}

func TestClassifyRunError_TruncatesLongStderr(t *testing.T) {
	long := strings.Repeat("x", 5_000)
	got := ClassifyRunError(ErrorPayload{Code: "api_error", Message: "z"}, long)
	if len(got.Stderr) > 2_500 {
		t.Errorf("stderr length = %d, want truncated (≤ 2500 incl. suffix)", len(got.Stderr))
	}
	if !strings.HasSuffix(got.Stderr, "…(truncated)") {
		t.Errorf("stderr should be truncated with marker, got tail %q", got.Stderr[len(got.Stderr)-20:])
	}
}
