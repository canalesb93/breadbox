package service

import "testing"

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
