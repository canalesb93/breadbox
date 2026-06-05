//go:build !headless && !lite

package pages

import "strconv"

// ConnectorsSettingsProps drives the /settings/connectors tab — the global
// custom-MCP connector library. The top level is a Providers-style directory;
// adding/editing happens in a modal. Header values never reach the browser.
type ConnectorsSettingsProps struct {
	Connectors  []ConnectorView
	CSRFToken   string
	FormError   string
	FormSuccess string
}

// ConnectorView is the settings-page view of one library connector. Header
// values are never sent — only the header NAMES and whether any value is stored.
type ConnectorView struct {
	ShortID     string
	Name        string
	URL         string
	Transport   string
	Note        string
	HeaderNames []string
	HasSecret   bool
}

// connectorsJSONData shapes the connector list for the Alpine factory's
// @templ.JSONScript hydration. Header values are intentionally absent.
func connectorsJSONData(cs []ConnectorView) []map[string]any {
	out := make([]map[string]any, 0, len(cs))
	for _, c := range cs {
		names := c.HeaderNames
		if names == nil {
			names = []string{}
		}
		out = append(out, map[string]any{
			"short_id":  c.ShortID,
			"name":      c.Name,
			"url":       c.URL,
			"transport": c.Transport,
			"note":      c.Note,
			"headers":   names,
		})
	}
	return out
}

// connectorHeaderSummary renders a short "Authorization +1" style summary of a
// connector's header names for the directory row.
func connectorHeaderSummary(c ConnectorView) string {
	if len(c.HeaderNames) == 0 {
		return "No headers"
	}
	if len(c.HeaderNames) == 1 {
		return c.HeaderNames[0]
	}
	return c.HeaderNames[0] + " +" + strconv.Itoa(len(c.HeaderNames)-1)
}
