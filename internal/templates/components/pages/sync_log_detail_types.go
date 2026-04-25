package pages

import (
	"breadbox/internal/templates/components"
)

// SyncLogDetailProps mirrors the data the old sync_log_detail.html
// read off the layout data map. Built by admin.SyncLogDetailHandler
// and rendered via TemplateRenderer.RenderWithTempl.
//
// Optional service fields are projected to bare strings (or
// pre-formatted values) by the handler so the templ stays free of
// pgtype/funcMap helpers.
type SyncLogDetailProps struct {
	Log         SyncLogDetailLog
	Accounts    []SyncLogDetailAccount
	Breadcrumbs []components.Breadcrumb
}

// SyncLogDetailLog flattens service.SyncLogRow into the subset the
// detail page renders.
type SyncLogDetailLog struct {
	ID                string
	ConnectionID      string
	InstitutionName   string
	Provider          string
	Trigger           string
	Status            string
	AddedCount        int32
	ModifiedCount     int32
	RemovedCount      int32
	UnchangedCount    int32
	ErrorMessage      string // empty when nil
	WarningMessage    string // empty when nil
	StartedAtRelative string // pre-rendered "2 minutes ago" — empty when no timestamp
	Duration          string // pre-rendered duration string — empty when nil
	AccountsAffected  int64
	RuleHits          []SyncLogDetailRuleHit
	TotalRuleHits     int
}

// SyncLogDetailRuleHit mirrors service.RuleHitEntry but pre-renders
// the conditionSummary so the templ doesn't need a funcMap helper.
type SyncLogDetailRuleHit struct {
	RuleID           string
	RuleName         string
	Count            int
	ConditionSummary string // empty when no conditions
}

// SyncLogDetailAccount mirrors service.SyncLogAccountRow with the
// fields the per-account breakdown card actually reads.
type SyncLogDetailAccount struct {
	AccountName    string
	AddedCount     int32
	ModifiedCount  int32
	RemovedCount   int32
	UnchangedCount int32
}
