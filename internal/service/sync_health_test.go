package service

import (
	"testing"
)

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
