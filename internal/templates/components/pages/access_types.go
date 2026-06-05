//go:build !headless && !lite

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
	// JustCreatedKey / JustCreatedClient carry a one-time plaintext
	// reveal, populated by the handler from a session flash right after a
	// create. When set, the section renders a prominent copy-now block at
	// the top of its content. nil on every other load.
	JustCreatedKey    *AccessReveal
	JustCreatedClient *AccessReveal
}

// AccessReveal is the one-time secret shown immediately after minting an
// API key or OAuth client. The plaintext never leaves the server again —
// it's popped from the session flash on the redirect-back render and shown
// once. ClientID is set only for OAuth clients (the public half, shown
// alongside the secret); empty for API keys.
//
// ID is the new entity's id. When set (API keys), the reveal renders an
// inline, pre-focused Name field that background-saves to the rename
// endpoint — naming happens in the same breath as copying the secret — and
// the just-created key is omitted from the list below so it isn't shown
// twice. Empty for OAuth clients (no rename endpoint yet).
type AccessReveal struct {
	ID       string
	Name     string
	ClientID string
	Secret   string
	Scope    string
}

// accessActiveKeysExcluding returns the active key rows minus the one that's
// currently shown in the reveal block (matched by ID). While the reveal is
// up it IS that key's row — an editable-name, copy-the-secret expansion — so
// rendering it again in the list below would duplicate it under a stale name.
func accessActiveKeysExcluding(keys []AccessKeyRow, reveal *AccessReveal) []AccessKeyRow {
	if reveal == nil || reveal.ID == "" {
		return keys
	}
	out := make([]AccessKeyRow, 0, len(keys))
	for _, k := range keys {
		if k.ID == reveal.ID {
			continue
		}
		out = append(out, k)
	}
	return out
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

// accessAPIKeyRenameURL returns the POST endpoint that renames the given API key.
func accessAPIKeyRenameURL(id string) string {
	return "/settings/api-keys/" + id + "/rename"
}

// accessOAuthClientRevokeURL returns the POST endpoint that revokes the
// given OAuth client.
func accessOAuthClientRevokeURL(id string) string {
	return "/settings/oauth-clients/" + id + "/revoke"
}
