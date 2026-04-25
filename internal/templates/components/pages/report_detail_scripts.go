package pages

import _ "embed"

// reportDetailScriptBody holds the full inline <script> block that powers
// the report-detail page: the reportDetail() Alpine factory (toast +
// mark-read toggle + copy-link) plus the DOMContentLoaded listener that
// runs DOMPurify.sanitize(marked.parse(...)) over each `.bb-report-body`
// element and rewrites external links to open in a new tab.
//
// Lifted verbatim from the original
// internal/templates/pages/report_detail.html so the templ port stays
// byte-for-byte equivalent.
//
//go:embed assets/report_detail_scripts.js.html
var reportDetailScriptBody string
