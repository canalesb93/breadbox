//go:build !lite

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"breadbox/internal/appconfig"
	"breadbox/internal/db"
)

// Developer-Mode report types + GitHub label colors (6-hex, no leading #).
const (
	devReportTypeBug  = "bug"
	devReportTypeTask = "task"

	devReportFlowLabelColor = "8957E5" // purple, matches the repo's stack labels
	devReportBugLabelColor  = "D73A4A"
	devReportTaskLabelColor = "0E8A16"
)

// CreateDevReportInput is the decoded payload from the floating reporter.
type CreateDevReportInput struct {
	Type                  string         // "bug" | "task"
	Title                 string         // required
	Description           string         // free-form; may be empty
	PageURL               string         // absolute URL the report was filed from
	PagePath              string         // path-only, for compact display
	ScreenshotData        []byte         // decoded image bytes (may be empty)
	ScreenshotContentType string         // e.g. "image/jpeg"
	HTMLSnapshot          string         // outerHTML of the page (may be empty)
	Metadata              map[string]any // browser/page context (viewport, UA, theme, …)
	CreatedBy             string         // admin session username
}

// DevReportResult is what the reporter shows the user after filing.
type DevReportResult struct {
	ShortID           string `json:"short_id"`
	Status            string `json:"status"` // pending | open | draft | saved | failed
	GithubIssueNumber int    `json:"github_issue_number,omitempty"`
	GithubIssueURL    string `json:"github_issue_url,omitempty"`
	// DraftURL is a prefilled GitHub new-issue URL — set when no token is
	// configured so the client can open a draft for the user to submit.
	DraftURL string `json:"draft_url,omitempty"`
	Error    string `json:"error,omitempty"` // non-fatal reason when status=failed
}

// DevReportSummary is one row of the Settings → Developer history list.
type DevReportSummary struct {
	ShortID           string
	Type              string
	Title             string
	PagePath          string
	Status            string
	GithubIssueNumber int
	GithubIssueURL    string
	GithubLabel       string
	CreatedBy         string
	Error             string
	CreatedAt         time.Time
}

// CreateDevReport persists the report (screenshot + HTML snapshot + metadata)
// and files a labelled GitHub issue. The row is written first so the durable
// artifact URLs resolve and the audit trail survives even when GitHub filing
// fails — in that case the row is marked "failed" and the reason returned,
// rather than erroring the whole call.
func (s *Service) CreateDevReport(ctx context.Context, in CreateDevReportInput, encKey []byte) (*DevReportResult, error) {
	rtype := normalizeReportType(in.Type)
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, fmt.Errorf("%w: title is required", ErrInvalidParameter)
	}

	repo := appconfig.String(ctx, s.Queries, appconfig.KeyDevModeGithubRepo, appconfig.DevModeDefaultRepo)
	label := appconfig.String(ctx, s.Queries, appconfig.KeyDevModeIssueLabel, appconfig.DevModeDefaultLabel)
	publicBase := strings.TrimRight(appconfig.String(ctx, s.Queries, appconfig.KeyNotifyPublicBaseURL, ""), "/")
	token, _, err := appconfig.ReadEncrypted(ctx, s.Queries, appconfig.KeyDevModeGithubToken, encKey)
	if err != nil {
		return nil, fmt.Errorf("read github token: %w", err)
	}

	metaJSON, err := json.Marshal(in.Metadata)
	if err != nil || metaJSON == nil {
		metaJSON = []byte("{}")
	}

	row, err := s.Queries.CreateDevReport(ctx, db.CreateDevReportParams{
		ReportType:            rtype,
		Title:                 title,
		Description:           in.Description,
		PageUrl:               in.PageURL,
		PagePath:              in.PagePath,
		Metadata:              metaJSON,
		Screenshot:            in.ScreenshotData,
		ScreenshotContentType: in.ScreenshotContentType,
		HtmlSnapshot:          in.HTMLSnapshot,
		GithubLabel:           label,
		CreatedBy:             in.CreatedBy,
	})
	if err != nil {
		return nil, fmt.Errorf("persist dev report: %w", err)
	}
	shortID := row.ShortID
	result := &DevReportResult{ShortID: shortID, Status: "pending"}

	// saved = soft state: the report is persisted but GitHub isn't wired up.
	saved := func(msg string) (*DevReportResult, error) {
		_ = s.Queries.SetDevReportSaved(ctx, db.SetDevReportSavedParams{ShortID: shortID, ErrorMessage: msg})
		result.Status = "saved"
		result.Error = msg
		return result, nil
	}
	// fail = a GitHub filing error after a genuine attempt.
	fail := func(msg string) (*DevReportResult, error) {
		_ = s.Queries.SetDevReportError(ctx, db.SetDevReportErrorParams{ShortID: shortID, ErrorMessage: msg})
		result.Status = "failed"
		result.Error = msg
		return result, nil
	}

	if repo == "" {
		return saved("No GitHub repository is configured — the report was saved to Breadbox.")
	}

	// Best-effort: upload the screenshot for a public, renderable image URL.
	// The durable copy in Breadbox is always linked in the body. Used by both
	// the API and draft paths.
	var imageURL string
	if len(in.ScreenshotData) > 0 && len(in.ScreenshotData) <= img402MaxBytes {
		if u, uerr := uploadToImg402(ctx, in.ScreenshotData, "screenshot.jpg"); uerr == nil {
			imageURL = u
		}
	}

	issueTitleStr := issueTitle(rtype, title)
	body := buildDevReportIssueBody(rtype, in, shortID, imageURL, publicBase)
	labels := dedupeLabels(label, rtype)

	// No token → open a prefilled GitHub draft the user submits themselves.
	// Zero-config path: rides the user's existing GitHub session, no PAT.
	if token == "" {
		draftURL := buildDraftURL(repo, issueTitleStr, body, labels)
		if draftURL == "" {
			return saved("Couldn’t build a GitHub draft — check the repository setting.")
		}
		if uerr := s.Queries.SetDevReportDraft(ctx, db.SetDevReportDraftParams{ShortID: shortID, GithubIssueUrl: draftURL}); uerr != nil {
			return nil, fmt.Errorf("record draft: %w", uerr)
		}
		result.Status = "draft"
		result.DraftURL = draftURL
		result.GithubIssueURL = draftURL
		return result, nil
	}

	// Token → file via the API in one click.
	gh, err := newGithubIssueClient(token, repo)
	if err != nil {
		return fail(err.Error())
	}

	// Best-effort: make sure the labels exist before referencing them.
	_ = gh.ensureLabel(ctx, label, devReportFlowLabelColor, "Filed via Breadbox Developer Mode")
	_ = gh.ensureLabel(ctx, rtype, typeLabelColor(rtype), "Breadbox Developer Mode report type")

	number, htmlURL, err := gh.createIssue(ctx, issueTitleStr, body, labels)
	if err != nil {
		return fail(err.Error())
	}

	if uerr := s.Queries.SetDevReportIssue(ctx, db.SetDevReportIssueParams{
		ShortID:           shortID,
		GithubIssueNumber: int32(number),
		GithubIssueUrl:    htmlURL,
	}); uerr != nil {
		return nil, fmt.Errorf("record issue link: %w", uerr)
	}
	result.Status = "open"
	result.GithubIssueNumber = number
	result.GithubIssueURL = htmlURL
	return result, nil
}

// GetDevReportArtifact returns the stored screenshot bytes + content type.
func (s *Service) GetDevReportArtifact(ctx context.Context, shortID string) ([]byte, string, error) {
	rep, err := s.Queries.GetDevReportByShortID(ctx, strings.TrimSpace(shortID))
	if err != nil {
		return nil, "", err
	}
	if len(rep.Screenshot) == 0 {
		return nil, "", fmt.Errorf("%w: no screenshot", ErrInvalidParameter)
	}
	ct := rep.ScreenshotContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	return rep.Screenshot, ct, nil
}

// GetDevReportSnapshot returns the stored raw HTML snapshot.
func (s *Service) GetDevReportSnapshot(ctx context.Context, shortID string) (string, error) {
	rep, err := s.Queries.GetDevReportByShortID(ctx, strings.TrimSpace(shortID))
	if err != nil {
		return "", err
	}
	return rep.HtmlSnapshot, nil
}

// ListDevReports returns the most recent reports for the settings history.
func (s *Service) ListDevReports(ctx context.Context, limit int) ([]DevReportSummary, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.Queries.ListDevReports(ctx, int32(limit))
	if err != nil {
		return nil, err
	}
	out := make([]DevReportSummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, DevReportSummary{
			ShortID:           r.ShortID,
			Type:              r.ReportType,
			Title:             r.Title,
			PagePath:          r.PagePath,
			Status:            r.Status,
			GithubIssueNumber: int(r.GithubIssueNumber),
			GithubIssueURL:    r.GithubIssueUrl,
			GithubLabel:       r.GithubLabel,
			CreatedBy:         r.CreatedBy,
			Error:             r.ErrorMessage,
			CreatedAt:         r.CreatedAt.Time,
		})
	}
	return out, nil
}

// ---- helpers ----------------------------------------------------------

func normalizeReportType(t string) string {
	if strings.EqualFold(strings.TrimSpace(t), devReportTypeTask) {
		return devReportTypeTask
	}
	return devReportTypeBug
}

func typeLabelColor(rtype string) string {
	if rtype == devReportTypeTask {
		return devReportTaskLabelColor
	}
	return devReportBugLabelColor
}

func issueTitle(rtype, title string) string {
	prefix := "[Bug]"
	if rtype == devReportTypeTask {
		prefix = "[Task]"
	}
	return prefix + " " + title
}

// dedupeLabels returns [flow, type] minus any empty/duplicate entries.
func dedupeLabels(flow, rtype string) []string {
	seen := map[string]bool{}
	var out []string
	for _, l := range []string{strings.TrimSpace(flow), rtype} {
		if l == "" || seen[l] {
			continue
		}
		seen[l] = true
		out = append(out, l)
	}
	return out
}

// draftURLMaxLen bounds the prefilled new-issue URL. GitHub accepts long
// prefills, but browsers cap URL length — keep it comfortably under common
// limits, trimming the body (never the title) if needed.
const draftURLMaxLen = 7000

// buildDraftURL returns a prefilled GitHub "new issue" URL. No token needed —
// the user's browser session authorizes the submit. The body is trimmed if
// the URL would get too long for the browser.
func buildDraftURL(repo, title, body string, labels []string) string {
	owner, name, err := splitOwnerRepo(repo)
	if err != nil {
		return ""
	}
	build := func(b string) string {
		q := url.Values{}
		q.Set("title", title)
		q.Set("body", b)
		if len(labels) > 0 {
			q.Set("labels", strings.Join(labels, ","))
		}
		return fmt.Sprintf("https://github.com/%s/%s/issues/new?%s", owner, name, q.Encode())
	}
	out := build(body)
	if len(out) > draftURLMaxLen {
		// Trim the body to fit, leaving room for the rest of the URL.
		keep := len(body) - (len(out) - draftURLMaxLen) - 64
		if keep < 0 {
			keep = 0
		}
		out = build(body[:keep] + "\n\n…(truncated — full context in the linked Breadbox artifacts)")
	}
	return out
}

// buildDevReportIssueBody renders the GitHub issue markdown: the report
// description, the embedded screenshot (when hosted), a context table, and a
// collapsible block of durable artifact links back into Breadbox.
func buildDevReportIssueBody(rtype string, in CreateDevReportInput, shortID, imageURL, publicBase string) string {
	var b strings.Builder

	typeLabel := "Bug"
	if rtype == devReportTypeTask {
		typeLabel = "Task"
	}
	fmt.Fprintf(&b, "**Type:** %s\n\n", typeLabel)

	desc := strings.TrimSpace(in.Description)
	if desc == "" {
		desc = "_No description provided._"
	}
	b.WriteString(desc)
	b.WriteString("\n\n")

	b.WriteString("### Screenshot\n\n")
	if imageURL != "" {
		fmt.Fprintf(&b, "<img src=%q width=\"900\" alt=\"screenshot\">\n\n", imageURL)
	} else if len(in.ScreenshotData) > 0 {
		b.WriteString("_Screenshot stored in Breadbox (see artifacts below)._\n\n")
	} else {
		b.WriteString("_No screenshot captured._\n\n")
	}

	b.WriteString("### Context\n\n")
	b.WriteString("| Field | Value |\n| --- | --- |\n")
	page := in.PagePath
	if page == "" {
		page = in.PageURL
	}
	tableRow(&b, "Page", code(page))
	if in.PageURL != "" {
		tableRow(&b, "URL", code(in.PageURL))
	}
	if v := metaStr(in.Metadata, "current_page"); v != "" {
		tableRow(&b, "View", v)
	}
	reporter := in.CreatedBy
	if reporter == "" {
		reporter = metaStr(in.Metadata, "reported_by")
	}
	tableRow(&b, "Reported by", reporter)
	tableRow(&b, "Viewport", metaStr(in.Metadata, "viewport"))
	tableRow(&b, "Theme", metaStr(in.Metadata, "theme"))
	tableRow(&b, "App version", metaStr(in.Metadata, "app_version"))
	tableRow(&b, "Browser", code(metaStr(in.Metadata, "user_agent")))
	tableRow(&b, "Redacted", redactedLabel(metaStr(in.Metadata, "redacted")))
	tableRow(&b, "Filed at", metaStr(in.Metadata, "client_time"))
	b.WriteString("\n")

	b.WriteString("<details>\n<summary>Artifacts (Breadbox)</summary>\n\n")
	scrURL := artifactURL(publicBase, shortID, "screenshot")
	snapURL := artifactURL(publicBase, shortID, "snapshot.html")
	if len(in.ScreenshotData) > 0 {
		fmt.Fprintf(&b, "- [Screenshot](%s)\n", scrURL)
	}
	if in.HTMLSnapshot != "" {
		fmt.Fprintf(&b, "- [HTML snapshot](%s)\n", snapURL)
	}
	fmt.Fprintf(&b, "\nReport `%s`\n\n", shortID)
	if publicBase == "" {
		b.WriteString("_Artifact links are relative — set a public base URL in Settings → Notifications for absolute links._\n")
	}
	b.WriteString("</details>\n\n")

	b.WriteString("---\n")
	b.WriteString("<sub>Filed via Breadbox Developer Mode.</sub>\n")
	return b.String()
}

// redactedLabel renders the privacy state of the capture for the issue body.
func redactedLabel(v string) string {
	switch v {
	case "true":
		return "Yes — financial data masked"
	case "false":
		return "⚠️ No — raw data included"
	default:
		return ""
	}
}

func artifactURL(publicBase, shortID, leaf string) string {
	p := "/-/dev-reports/" + shortID + "/" + leaf
	if publicBase == "" {
		return p
	}
	return publicBase + p
}

func tableRow(b *strings.Builder, field, value string) {
	if strings.TrimSpace(stripCode(value)) == "" {
		return
	}
	fmt.Fprintf(b, "| %s | %s |\n", field, value)
}

// metaStr pulls a string-ish value from the metadata map, cleaning pipes and
// newlines so it's safe inside a markdown table cell.
func metaStr(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	var s string
	switch t := v.(type) {
	case string:
		s = t
	case float64:
		s = trimFloat(t)
	case bool:
		if t {
			s = "true"
		} else {
			s = "false"
		}
	default:
		s = fmt.Sprintf("%v", t)
	}
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", "\\|")
	return strings.TrimSpace(s)
}

func trimFloat(f float64) string {
	if f == float64(int64(f)) {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%g", f)
}

func code(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return "`" + strings.ReplaceAll(s, "`", "ʼ") + "`"
}

// stripCode unwraps the backtick wrapper code() adds, so tableRow's
// emptiness check sees the underlying value.
func stripCode(s string) string {
	return strings.Trim(s, "`")
}
