package service

import (
	"testing"
	"time"
)

func TestFormatOffset(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"zero", 0, "+0.0s"},
		{"negative clamped", -5 * time.Second, "+0.0s"},
		{"sub-second", 100 * time.Millisecond, "+0.1s"},
		{"seconds with tenths", 2100 * time.Millisecond, "+2.1s"},
		{"just under a minute", 59500 * time.Millisecond, "+59.5s"},
		{"one minute exact", 60 * time.Second, "+1m00s"},
		{"minute and seconds", 72 * time.Second, "+1m12s"},
		{"just under an hour", 59*time.Minute + 59*time.Second, "+59m59s"},
		{"one hour exact", time.Hour, "+1h00m"},
		{"hours and minutes", 2*time.Hour + 5*time.Minute, "+2h05m"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatOffset(tc.d); got != tc.want {
				t.Errorf("formatOffset(%v) = %q, want %q", tc.d, got, tc.want)
			}
		})
	}
}

func TestSummarizeToolCalls(t *testing.T) {
	tests := []struct {
		name                                       string
		calls                                      []ToolCallLogResponse
		wantErrors, wantWrites, wantReads          int
	}{
		{
			name:  "empty",
			calls: nil,
		},
		{
			name: "mixed classifications and errors",
			calls: []ToolCallLogResponse{
				{Classification: "read"},
				{Classification: "write"},
				{Classification: "write", IsError: true},
				{Classification: "read"},
				{Classification: "read", IsError: true},
			},
			wantErrors: 2,
			wantWrites: 2,
			wantReads:  3,
		},
		{
			name: "unknown classification is ignored in split but error still counted",
			calls: []ToolCallLogResponse{
				{Classification: "other", IsError: true},
				{Classification: "", IsError: false},
				{Classification: "read"},
			},
			wantErrors: 1,
			wantWrites: 0,
			wantReads:  1,
		},
		{
			name: "all clean",
			calls: []ToolCallLogResponse{
				{Classification: "read"},
				{Classification: "write"},
			},
			wantWrites: 1,
			wantReads:  1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotErr, gotWrite, gotRead := summarizeToolCalls(tc.calls)
			if gotErr != tc.wantErrors {
				t.Errorf("errors = %d, want %d", gotErr, tc.wantErrors)
			}
			if gotWrite != tc.wantWrites {
				t.Errorf("writes = %d, want %d", gotWrite, tc.wantWrites)
			}
			if gotRead != tc.wantReads {
				t.Errorf("reads = %d, want %d", gotRead, tc.wantReads)
			}
		})
	}
}
