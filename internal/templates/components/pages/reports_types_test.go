//go:build !headless && !lite

package pages

import (
	"testing"

	"breadbox/internal/templates/components"
)

func TestPartitionReportsByRead(t *testing.T) {
	rows := []components.ReportRowProps{
		{ID: "a", IsRead: false},
		{ID: "b", IsRead: true},
		{ID: "c", IsRead: false},
		{ID: "d", IsRead: true},
	}

	unread, read := partitionReportsByRead(rows)

	if len(unread) != 2 {
		t.Fatalf("want 2 unread, got %d", len(unread))
	}
	if len(read) != 2 {
		t.Fatalf("want 2 read, got %d", len(read))
	}
	// Order within each partition must be preserved (recency order).
	if unread[0].ID != "a" || unread[1].ID != "c" {
		t.Errorf("unread order not preserved: got %s, %s", unread[0].ID, unread[1].ID)
	}
	if read[0].ID != "b" || read[1].ID != "d" {
		t.Errorf("read order not preserved: got %s, %s", read[0].ID, read[1].ID)
	}
}

func TestPartitionReportsByRead_Empty(t *testing.T) {
	unread, read := partitionReportsByRead(nil)
	if unread != nil || read != nil {
		t.Errorf("want nil slices for empty input, got unread=%v read=%v", unread, read)
	}
}
