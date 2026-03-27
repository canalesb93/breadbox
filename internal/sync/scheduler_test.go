package sync

import "testing"

func TestBackoffInterval(t *testing.T) {
	tests := []struct {
		name                string
		baseMinutes         int
		consecutiveFailures int32
		want                int
	}{
		{
			name:                "no failures returns base",
			baseMinutes:         15,
			consecutiveFailures: 0,
			want:                15,
		},
		{
			name:                "1 failure doubles interval",
			baseMinutes:         15,
			consecutiveFailures: 1,
			want:                30, // 15 * 2^1
		},
		{
			name:                "2 failures quadruples interval",
			baseMinutes:         15,
			consecutiveFailures: 2,
			want:                60, // 15 * 2^2
		},
		{
			name:                "3 failures 8x interval",
			baseMinutes:         15,
			consecutiveFailures: 3,
			want:                120, // 15 * 2^3
		},
		{
			name:                "4 failures 16x interval (max)",
			baseMinutes:         15,
			consecutiveFailures: 4,
			want:                240, // 15 * 2^4
		},
		{
			name:                "5 failures still capped at 16x",
			baseMinutes:         15,
			consecutiveFailures: 5,
			want:                240, // 15 * 2^4 (capped)
		},
		{
			name:                "100 failures still capped at 16x",
			baseMinutes:         15,
			consecutiveFailures: 100,
			want:                240, // 15 * 2^4 (capped)
		},
		{
			name:                "60min base with 3 failures",
			baseMinutes:         60,
			consecutiveFailures: 3,
			want:                480, // 60 * 2^3
		},
		{
			name:                "negative failures treated as zero",
			baseMinutes:         15,
			consecutiveFailures: -1,
			want:                15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := backoffInterval(tt.baseMinutes, tt.consecutiveFailures)
			if got != tt.want {
				t.Errorf("backoffInterval(%d, %d) = %d, want %d",
					tt.baseMinutes, tt.consecutiveFailures, got, tt.want)
			}
		})
	}
}
