//go:build !headless && !lite

package pages

import (
)

// ReportDetailProps mirrors the data the old report_detail.html read off
// the layout data map. Built by admin.ReportDetailHandler and rendered
// via TemplateRenderer.RenderWithTempl.
//
// Pre-formatted strings (CreatedAt, CreatedAtRel) and a flat IsRead bool
// keep the templ free of pgtype/funcMap helpers.
type ReportDetailProps struct {
	Report      ReportDetailReport
}

// ReportDetailReport flattens the agent-report fields the detail page
// renders. Body is rendered server-side as Markdown via
// @components.MarkdownLarge (goldmark + bluemonday) into .bb-prose.
type ReportDetailReport struct {
	ID            string
	Title         string
	Body          string
	Priority      string
	DisplayAuthor string
	CreatedAt     string // pre-formatted "Jan 2, 2006 at 3:04 PM"
	CreatedAtRel  string // pre-rendered "2 minutes ago"
	IsRead        bool
}
