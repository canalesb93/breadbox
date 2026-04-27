package pages

// AccessProps mirrors the data map the old access.html read. The handler
// pre-renders relative timestamps and short-formatted creation dates so the
// templ component stays free of time-helper logic.
type AccessProps struct {
	IsAdmin        bool
	CSRFToken      string
	ActiveKeys     []AccessKeyRow
	RevokedKeys    []AccessKeyRow
	HasAnyKeys     bool
	ActiveClients  []AccessClientRow
	RevokedClients []AccessClientRow
	HasAnyClients  bool
}

// AccessKeyRow is the per-row shape for the API keys section. CreatedAtShort
// and LastUsedRelative are pre-formatted in the handler to match the
// `formatDateShort` and `relativeTime` funcMap helpers.
type AccessKeyRow struct {
	ID               string
	Name             string
	KeyPrefix        string
	Scope            string
	CreatedAtShort   string
	LastUsedRelative string // empty if never used
}

// AccessClientRow is the per-row shape for the OAuth clients section. Mirrors
// AccessKeyRow but uses ClientIDPrefix instead of KeyPrefix and has no
// LastUsed field — OAuth clients don't track last-used.
type AccessClientRow struct {
	ID             string
	Name           string
	ClientIDPrefix string
	Scope          string
	CreatedAtShort string
}

// accessAPIKeyRevokeURL returns the POST endpoint that revokes the given
// API key. Centralising it keeps the templ free of string concatenation in
// `action={...}` slots — see the connections page for the same pattern.
func accessAPIKeyRevokeURL(id string) string {
	return "/settings/api-keys/" + id + "/revoke"
}

// accessOAuthClientRevokeURL returns the POST endpoint that revokes the
// given OAuth client.
func accessOAuthClientRevokeURL(id string) string {
	return "/settings/oauth-clients/" + id + "/revoke"
}
