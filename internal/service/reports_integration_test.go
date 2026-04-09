//go:build integration

package service_test

import (
	"context"
	"testing"

	"breadbox/internal/service"
)

// --- Agent Reports ---

func TestCreateAgentReport_Success(t *testing.T) {
	svc, _, _ := newService(t)
	actor := service.Actor{Type: "agent", ID: "agent-1", Name: "TestAgent"}

	report, err := svc.CreateAgentReport(context.Background(), "Test Report", "Report body content", actor, "info", []string{"tag1"}, "", "")
	if err != nil {
		t.Fatalf("CreateAgentReport: %v", err)
	}
	if report.Title != "Test Report" {
		t.Errorf("expected title 'Test Report', got %q", report.Title)
	}
	if report.Body != "Report body content" {
		t.Errorf("expected body 'Report body content', got %q", report.Body)
	}
	if report.CreatedByType != "agent" {
		t.Errorf("expected created_by_type 'agent', got %q", report.CreatedByType)
	}
	if report.CreatedByName != "TestAgent" {
		t.Errorf("expected created_by_name 'TestAgent', got %q", report.CreatedByName)
	}
	if report.Priority != "info" {
		t.Errorf("expected priority 'info', got %q", report.Priority)
	}
	if len(report.Tags) != 1 || report.Tags[0] != "tag1" {
		t.Errorf("expected tags [tag1], got %v", report.Tags)
	}
	if report.ReadAt != nil {
		t.Error("expected read_at to be nil for new report")
	}
	if report.ID == "" {
		t.Error("expected non-empty ID")
	}
	if report.ShortID == "" {
		t.Error("expected non-empty ShortID")
	}
}

func TestCreateAgentReport_DefaultPriority(t *testing.T) {
	svc, _, _ := newService(t)
	actor := service.Actor{Type: "agent", ID: "agent-1", Name: "TestAgent"}

	report, err := svc.CreateAgentReport(context.Background(), "Test", "Body", actor, "", nil, "", "")
	if err != nil {
		t.Fatalf("CreateAgentReport: %v", err)
	}
	if report.Priority != "info" {
		t.Errorf("expected default priority 'info', got %q", report.Priority)
	}
}

func TestCreateAgentReport_WithAuthorOverride(t *testing.T) {
	svc, _, _ := newService(t)
	actor := service.Actor{Type: "agent", ID: "agent-1", Name: "DefaultName"}

	report, err := svc.CreateAgentReport(context.Background(), "Title", "Body", actor, "warning", nil, "CustomAuthor", "")
	if err != nil {
		t.Fatalf("CreateAgentReport: %v", err)
	}
	if report.CreatedByName != "CustomAuthor" {
		t.Errorf("expected created_by_name 'CustomAuthor', got %q", report.CreatedByName)
	}
}

func TestCreateAgentReport_MissingTitle(t *testing.T) {
	svc, _, _ := newService(t)
	actor := service.Actor{Type: "agent", ID: "agent-1", Name: "TestAgent"}

	_, err := svc.CreateAgentReport(context.Background(), "", "Body", actor, "info", nil, "", "")
	if err == nil {
		t.Fatal("expected error for missing title")
	}
}

func TestCreateAgentReport_MissingBody(t *testing.T) {
	svc, _, _ := newService(t)
	actor := service.Actor{Type: "agent", ID: "agent-1", Name: "TestAgent"}

	_, err := svc.CreateAgentReport(context.Background(), "Title", "", actor, "info", nil, "", "")
	if err == nil {
		t.Fatal("expected error for missing body")
	}
}

func TestCreateAgentReport_InvalidPriority(t *testing.T) {
	svc, _, _ := newService(t)
	actor := service.Actor{Type: "agent", ID: "agent-1", Name: "TestAgent"}

	_, err := svc.CreateAgentReport(context.Background(), "Title", "Body", actor, "urgent", nil, "", "")
	if err == nil {
		t.Fatal("expected error for invalid priority")
	}
}

func TestCreateAgentReport_TooManyTags(t *testing.T) {
	svc, _, _ := newService(t)
	actor := service.Actor{Type: "agent", ID: "agent-1", Name: "TestAgent"}
	tags := make([]string, 11)
	for i := range tags {
		tags[i] = "tag"
	}

	_, err := svc.CreateAgentReport(context.Background(), "Title", "Body", actor, "info", tags, "", "")
	if err == nil {
		t.Fatal("expected error for too many tags")
	}
}

func TestListAgentReports_Empty(t *testing.T) {
	svc, _, _ := newService(t)
	reports, err := svc.ListAgentReports(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListAgentReports: %v", err)
	}
	if len(reports) != 0 {
		t.Errorf("expected 0 reports, got %d", len(reports))
	}
}

func TestListAgentReports_WithData(t *testing.T) {
	svc, _, _ := newService(t)
	actor := service.Actor{Type: "agent", ID: "a1", Name: "Agent"}

	for i := 0; i < 3; i++ {
		_, err := svc.CreateAgentReport(context.Background(), "Report", "Body", actor, "info", nil, "", "")
		if err != nil {
			t.Fatalf("create report %d: %v", i, err)
		}
	}

	reports, err := svc.ListAgentReports(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListAgentReports: %v", err)
	}
	if len(reports) != 3 {
		t.Errorf("expected 3 reports, got %d", len(reports))
	}
}

func TestListAgentReports_LimitClamped(t *testing.T) {
	svc, _, _ := newService(t)
	// Invalid limits should be clamped to default 20
	reports, err := svc.ListAgentReports(context.Background(), 0)
	if err != nil {
		t.Fatalf("ListAgentReports: %v", err)
	}
	if reports == nil {
		t.Error("expected non-nil slice")
	}

	reports, err = svc.ListAgentReports(context.Background(), -1)
	if err != nil {
		t.Fatalf("ListAgentReports with -1: %v", err)
	}
	if reports == nil {
		t.Error("expected non-nil slice for -1")
	}
}

func TestCountUnreadAgentReports_Empty(t *testing.T) {
	svc, _, _ := newService(t)
	count, err := svc.CountUnreadAgentReports(context.Background())
	if err != nil {
		t.Fatalf("CountUnreadAgentReports: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 unread, got %d", count)
	}
}

func TestCountUnreadAgentReports_WithReports(t *testing.T) {
	svc, _, _ := newService(t)
	actor := service.Actor{Type: "agent", ID: "a1", Name: "Agent"}

	for i := 0; i < 3; i++ {
		_, err := svc.CreateAgentReport(context.Background(), "Report", "Body", actor, "info", nil, "", "")
		if err != nil {
			t.Fatalf("create report %d: %v", i, err)
		}
	}

	count, err := svc.CountUnreadAgentReports(context.Background())
	if err != nil {
		t.Fatalf("CountUnreadAgentReports: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 unread, got %d", count)
	}
}

func TestMarkAgentReportRead(t *testing.T) {
	svc, _, _ := newService(t)
	actor := service.Actor{Type: "agent", ID: "a1", Name: "Agent"}

	report, err := svc.CreateAgentReport(context.Background(), "Report", "Body", actor, "info", nil, "", "")
	if err != nil {
		t.Fatalf("CreateAgentReport: %v", err)
	}

	err = svc.MarkAgentReportRead(context.Background(), report.ID)
	if err != nil {
		t.Fatalf("MarkAgentReportRead: %v", err)
	}

	count, err := svc.CountUnreadAgentReports(context.Background())
	if err != nil {
		t.Fatalf("CountUnreadAgentReports: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 unread after marking read, got %d", count)
	}
}

func TestMarkAgentReportRead_InvalidID(t *testing.T) {
	svc, _, _ := newService(t)
	err := svc.MarkAgentReportRead(context.Background(), "not-a-uuid")
	if err == nil {
		t.Fatal("expected error for invalid ID")
	}
}

func TestMarkAllAgentReportsRead(t *testing.T) {
	svc, _, _ := newService(t)
	actor := service.Actor{Type: "agent", ID: "a1", Name: "Agent"}

	for i := 0; i < 3; i++ {
		_, err := svc.CreateAgentReport(context.Background(), "Report", "Body", actor, "info", nil, "", "")
		if err != nil {
			t.Fatalf("create report %d: %v", i, err)
		}
	}

	err := svc.MarkAllAgentReportsRead(context.Background())
	if err != nil {
		t.Fatalf("MarkAllAgentReportsRead: %v", err)
	}

	count, err := svc.CountUnreadAgentReports(context.Background())
	if err != nil {
		t.Fatalf("CountUnreadAgentReports: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 unread after marking all read, got %d", count)
	}
}

func TestListUnreadAgentReports(t *testing.T) {
	svc, _, _ := newService(t)
	actor := service.Actor{Type: "agent", ID: "a1", Name: "Agent"}

	report1, _ := svc.CreateAgentReport(context.Background(), "R1", "Body1", actor, "info", nil, "", "")
	_, _ = svc.CreateAgentReport(context.Background(), "R2", "Body2", actor, "warning", nil, "", "")

	// Mark one as read
	svc.MarkAgentReportRead(context.Background(), report1.ID)

	unread, err := svc.ListUnreadAgentReports(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListUnreadAgentReports: %v", err)
	}
	if len(unread) != 1 {
		t.Errorf("expected 1 unread report, got %d", len(unread))
	}
}

func TestGetAgentReport_NotFound(t *testing.T) {
	svc, _, _ := newService(t)
	_, err := svc.GetAgentReport(context.Background(), "00000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Fatal("expected error for missing report")
	}
}

func TestGetAgentReport_InvalidID(t *testing.T) {
	svc, _, _ := newService(t)
	_, err := svc.GetAgentReport(context.Background(), "bad-id")
	if err == nil {
		t.Fatal("expected error for invalid ID")
	}
}

func TestCreateAgentReport_AllPriorities(t *testing.T) {
	svc, _, _ := newService(t)
	actor := service.Actor{Type: "agent", ID: "a1", Name: "Agent"}

	priorities := []string{"info", "warning", "critical"}
	for _, p := range priorities {
		t.Run(p, func(t *testing.T) {
			// Tables are NOT truncated between sub-tests, so all 3 accumulate.
			// We just validate creation works.
			report, err := svc.CreateAgentReport(context.Background(), "Title", "Body", actor, p, nil, "", "")
			if err != nil {
				t.Fatalf("create report with priority %q: %v", p, err)
			}
			if report.Priority != p {
				t.Errorf("expected priority %q, got %q", p, report.Priority)
			}
		})
	}
}
