package sync

import (
	"testing"
	"time"

	"breadbox/internal/db"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestClassifyUpsertResult(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name          string
		createdAt     time.Time
		updatedAt     time.Time
		upsertStart   time.Time
		wantNew       bool
		wantChanged   bool
	}{
		{
			name:        "new row: created_at == updated_at (both NOW)",
			createdAt:   now,
			updatedAt:   now,
			upsertStart: now.Add(-100 * time.Millisecond),
			wantNew:     true,
			wantChanged: true,
		},
		{
			name:        "new row: created_at and updated_at within 1s tolerance",
			createdAt:   now,
			updatedAt:   now.Add(500 * time.Millisecond),
			upsertStart: now.Add(-100 * time.Millisecond),
			wantNew:     true,
			wantChanged: true,
		},
		{
			name:        "modified: updated_at just bumped (within sync window)",
			createdAt:   now.Add(-24 * time.Hour),
			updatedAt:   now,
			upsertStart: now.Add(-100 * time.Millisecond),
			wantNew:     false,
			wantChanged: true,
		},
		{
			name:        "unchanged: updated_at is old (not bumped)",
			createdAt:   now.Add(-24 * time.Hour),
			updatedAt:   now.Add(-12 * time.Hour),
			upsertStart: now.Add(-100 * time.Millisecond),
			wantNew:     false,
			wantChanged: false,
		},
		{
			name:        "unchanged: updated_at is minutes before upsert start",
			createdAt:   now.Add(-48 * time.Hour),
			updatedAt:   now.Add(-5 * time.Minute),
			upsertStart: now.Add(-100 * time.Millisecond),
			wantNew:     false,
			wantChanged: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			txn := db.Transaction{
				CreatedAt: pgtype.Timestamptz{Time: tt.createdAt, Valid: true},
				UpdatedAt: pgtype.Timestamptz{Time: tt.updatedAt, Valid: true},
			}

			gotNew, gotChanged := classifyUpsertResult(txn, tt.upsertStart)

			if gotNew != tt.wantNew {
				t.Errorf("isNew = %v, want %v", gotNew, tt.wantNew)
			}
			if gotChanged != tt.wantChanged {
				t.Errorf("isChanged = %v, want %v", gotChanged, tt.wantChanged)
			}
		})
	}
}
