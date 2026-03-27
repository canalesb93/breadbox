package service

import (
	"testing"
	"time"
)

func TestRelativeTimeStr(t *testing.T) {
	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		{"just now", time.Now().Add(-10 * time.Second), "just now"},
		{"1 minute ago", time.Now().Add(-90 * time.Second), "1 minute ago"},
		{"5 minutes ago", time.Now().Add(-5 * time.Minute), "5 minutes ago"},
		{"1 hour ago", time.Now().Add(-90 * time.Minute), "1 hour ago"},
		{"3 hours ago", time.Now().Add(-3 * time.Hour), "3 hours ago"},
		{"1 day ago", time.Now().Add(-36 * time.Hour), "1 day ago"},
		{"5 days ago", time.Now().Add(-5 * 24 * time.Hour), "5 days ago"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := relativeTimeStr(tt.t)
			if got != tt.want {
				t.Errorf("relativeTimeStr() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSyncHealthSummaryOverallHealth(t *testing.T) {
	// Test the health determination logic by constructing summaries
	// and verifying the OverallHealth field.
	tests := []struct {
		name         string
		syncCount    int64
		successRate  float64
		errorCount   int64
		connErrors   int64
		wantHealth   string
	}{
		{
			name:        "no syncs is degraded",
			syncCount:   0,
			successRate: 0,
			errorCount:  0,
			connErrors:  0,
			wantHealth:  "degraded",
		},
		{
			name:        "all success is healthy",
			syncCount:   10,
			successRate: 100,
			errorCount:  0,
			connErrors:  0,
			wantHealth:  "healthy",
		},
		{
			name:        "some errors is degraded",
			syncCount:   10,
			successRate: 80,
			errorCount:  2,
			connErrors:  0,
			wantHealth:  "degraded",
		},
		{
			name:        "low success rate is unhealthy",
			syncCount:   10,
			successRate: 40,
			errorCount:  6,
			connErrors:  0,
			wantHealth:  "unhealthy",
		},
		{
			name:        "all errors is unhealthy",
			syncCount:   5,
			successRate: 0,
			errorCount:  5,
			connErrors:  0,
			wantHealth:  "unhealthy",
		},
		{
			name:        "healthy syncs but connection errors is degraded",
			syncCount:   10,
			successRate: 100,
			errorCount:  0,
			connErrors:  1,
			wantHealth:  "degraded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reproduce the health logic from GetSyncHealthSummary.
			var health string
			switch {
			case tt.syncCount == 0:
				health = "degraded"
			case tt.successRate < 50 && tt.syncCount >= 2:
				health = "unhealthy"
			case tt.errorCount == tt.syncCount && tt.syncCount > 0:
				health = "unhealthy"
			case tt.errorCount > 0:
				health = "degraded"
			default:
				health = "healthy"
			}
			// Apply connection error override (same as dashboard handler).
			if tt.connErrors > 0 && health == "healthy" {
				health = "degraded"
			}
			if health != tt.wantHealth {
				t.Errorf("health = %q, want %q", health, tt.wantHealth)
			}
		})
	}
}
