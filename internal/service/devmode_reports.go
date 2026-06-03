//go:build !lite

package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"breadbox/internal/appconfig"
)

const (
	devReportTypeBug  = "bug"
	devReportTypeTask = "task"
)

// CreateDevReportInput is the decoded payload from the floating reporter.
type CreateDevReportInput struct {
	Type        string         // "bug" | "task"
	Title       string         // required
	Description string         // free-form; may be empty
	PageURL     string         // absolute URL the report was filed from (query stripped client-side)
	PagePath    string         // path-only, for compact display
	Metadata    map[string]any // browser/page context (viewport, UA, theme, …)
	CreatedBy   string         // admin session username
}

// DevReportResult is what the reporter shows the user after filing.
type DevReportResult struct {
	Status   string `json:"status"`             // always "draft"
	DraftURL string `json:"draft_url,omitempty"` // prefilled GitHub new-issue URL
}

// CreateDevReport builds a prefilled GitHub "new issue" draft URL from the
// report. No token, no persistence, no artifact hosting — the draft rides the
// user's existing GitHub session, and they review + submit it.
//
// Screenshot + HTML-snapshot hosting is intentionally absent here: those move
// to a remote storage backend (TBD) rather than the local DB. When that lands,
// the artifacts upload there and their URLs get embedded in the body.
func (s *Service) CreateDevReport(ctx context.Context, in CreateDevReportInput) (*DevReportResult, error) {
	rtype := normalizeReportType(in.Type)
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, fmt.Errorf("%w: title is required", ErrInvalidParameter)
	}

	repo := appconfig.String(ctx, s.Queries, appconfig.KeyDevModeGithubRepo, appconfig.DevModeDefaultRepo)
	label := appconfig.String(ctx, s.Queries, appconfig.KeyDevModeIssueLabel, appconfig.DevModeDefaultLabel)

	body := buildDevReportIssueBody(rtype, in)
	draftURL := buildDraftURL(repo, issueTitle(rtype, title), body, dedupeLabels(label, rtype))
	if draftURL == "" {
		return nil, fmt.Errorf("%w: invalid GitHub repository", ErrInvalidParameter)
	}
	return &DevReportResult{Status: "draft", DraftURL: draftURL}, nil
}

// ---- helpers ----------------------------------------------------------

func normalizeReportType(t string) string {
	if strings.EqualFold(strings.TrimSpace(t), devReportTypeTask) {
		return devReportTypeTask
	}
	return devReportTypeBug
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
// the user's browser session authorizes the submit.
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
		keep := len(body) - (len(out) - draftURLMaxLen) - 64
		if keep < 0 {
			keep = 0
		}
		out = build(body[:keep] + "\n\n…(truncated)")
	}
	return out
}

// buildDevReportIssueBody renders the GitHub issue markdown: the report
// description plus a context table. (Screenshot + HTML links return once a
// remote storage backend is wired in.)
func buildDevReportIssueBody(rtype string, in CreateDevReportInput) string {
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
	reporter := in.CreatedBy
	if reporter == "" {
		reporter = metaStr(in.Metadata, "reported_by")
	}
	tableRow(&b, "Reported by", reporter)
	tableRow(&b, "Viewport", metaStr(in.Metadata, "viewport"))
	tableRow(&b, "Theme", metaStr(in.Metadata, "theme"))
	tableRow(&b, "App version", metaStr(in.Metadata, "app_version"))
	tableRow(&b, "Browser", code(metaStr(in.Metadata, "user_agent")))
	tableRow(&b, "Filed at", metaStr(in.Metadata, "client_time"))
	b.WriteString("\n")

	b.WriteString("---\n")
	b.WriteString("<sub>Filed via Breadbox Developer Mode.</sub>\n")
	return b.String()
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

// stripCode unwraps the backtick wrapper code() adds, so tableRow's emptiness
// check sees the underlying value.
func stripCode(s string) string {
	return strings.Trim(s, "`")
}
