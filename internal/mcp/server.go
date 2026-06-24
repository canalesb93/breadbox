//go:build !lite

package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"time"

	mw "breadbox/internal/middleware"
	"breadbox/internal/service"
	"breadbox/internal/shortid"
	"breadbox/prompts"
	"breadbox/static"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// breadboxImplementation is the Implementation block sent in every
// initialize response. Title, websiteUrl, and the icon are spec-optional
// fields that surface in connector pickers (Claude.ai's Settings →
// Connectors list, MCP Inspector, etc.) — without them the connector
// renders as "breadbox <version>" with no icon and no link to docs.
//
// The icon is the same package outline used by the admin UI's favicon,
// inlined as a data URI so the metadata is host-agnostic (works on
// localhost, dev.breadbox.host, or any future deployment without
// pointing at an external asset URL).
func breadboxImplementation(version string) *mcpsdk.Implementation {
	impl := &mcpsdk.Implementation{
		Name:       "breadbox",
		Title:      "Breadbox",
		Version:    version,
		WebsiteURL: "https://breadbox.sh",
	}
	if icon := loadBreadboxIcon(); icon != nil {
		impl.Icons = []mcpsdk.Icon{*icon}
	}
	return impl
}

var (
	breadboxIcon     *mcpsdk.Icon
	breadboxIconOnce bool
)

// loadBreadboxIcon reads the favicon from the embedded static FS once and
// caches the resulting Icon. If the read fails (shouldn't, since the file
// is embedded at build time), it returns nil and the implementation goes
// out without icons rather than failing the initialize response.
func loadBreadboxIcon() *mcpsdk.Icon {
	if breadboxIconOnce {
		return breadboxIcon
	}
	breadboxIconOnce = true
	data, err := fs.ReadFile(static.FS, "favicon.svg")
	if err != nil {
		return nil
	}
	src := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(data)
	breadboxIcon = &mcpsdk.Icon{
		Source:   src,
		MIMEType: "image/svg+xml",
		Sizes:    []string{"any"},
	}
	return breadboxIcon
}

// Default MCP prompts sourced from the top-level prompts/ package — the
// canonical place to edit them is prompts/mcp/*.md. Used when the user has
// not overridden them via app_config.
var (
	DefaultInstructions     = prompts.MCP("instructions")
	DefaultReviewGuidelines = prompts.MCP("review-guidelines")
	DefaultReportFormat     = prompts.MCP("report-format")
	DefaultRuleDSL          = prompts.MCP("rule-dsl")
)

// ToolClassification indicates whether a tool is read-only or performs writes.
type ToolClassification string

const (
	ToolRead  ToolClassification = "read"
	ToolWrite ToolClassification = "write"
)

// ToolDef holds a tool definition, its handler, and classification metadata.
type ToolDef struct {
	Tool           mcpsdk.Tool
	Classification ToolClassification
	// register is a function that registers this tool on a server.
	register func(server *mcpsdk.Server)
}

// ToolSpec is the input shape for makeToolDefLogged. Carries the user-facing
// metadata the SDK exposes (Title, Annotations) alongside the runtime fields
// the registry needs (Name, Classification). Title surfaces in Claude.ai's
// Settings → Connectors → Configure Tools list; Annotations drive
// confirmation prompts and tool-list rendering. Leaving Annotations nil lets
// the registry fill in classification-derived defaults.
type ToolSpec struct {
	Name           string
	Title          string
	Description    string
	Classification ToolClassification
	Annotations    *mcpsdk.ToolAnnotations
}

// boolPtr returns a pointer to v. Used for optional bool fields on
// ToolAnnotations where nil means "not set" (the SDK defaults specified in
// the protocol take effect).
func boolPtr(v bool) *bool { return &v }

// defaultAnnotations builds the baseline ToolAnnotations for a tool from its
// classification when the registration site doesn't override it. Read tools
// get ReadOnlyHint=true; write tools default to non-destructive (the per-tool
// registration opts in to DestructiveHint=true for the few tools that delete
// or reset state). OpenWorldHint is left at the SDK default (true) — Breadbox
// is a closed-world server, but the protocol's default already covers that
// when ReadOnlyHint=true and the tool annotation block is otherwise empty.
func defaultAnnotations(classification ToolClassification, title string) *mcpsdk.ToolAnnotations {
	a := &mcpsdk.ToolAnnotations{Title: title}
	switch classification {
	case ToolRead:
		a.ReadOnlyHint = true
	case ToolWrite:
		// Default writes to non-destructive so hosts only confirm the few
		// tools that actually delete or reset state.
		a.DestructiveHint = boolPtr(false)
	}
	return a
}

// MCPServerConfig holds runtime MCP permissions loaded from app_config + API key.
type MCPServerConfig struct {
	Mode          string   // "read_only" or "read_write"
	DisabledTools []string // tool names to suppress
	Instructions  string   // full server instructions (uses DefaultInstructions if empty)
	APIKeyScope   string   // "full_access" or "read_only" — from request context
}

// MCPServer wraps the MCP SDK server and the breadbox service layer.
type MCPServer struct {
	svc      *service.Service
	version  string
	allTools []ToolDef
	// stdioFallbackTransportID is the per-process transport id used when
	// the underlying connection has no native session id (stdio). Stable
	// for the lifetime of the process so every tool call from the same
	// `breadbox mcp-stdio` invocation lands on the same audit-session
	// row. HTTP requests get the real MCP-Session-Id from the SDK and
	// ignore this field.
	stdioFallbackTransportID string
}

// NewMCPServer creates a new MCP server with all tools registered in a registry.
func NewMCPServer(svc *service.Service, version string) *MCPServer {
	s := &MCPServer{
		svc:                      svc,
		version:                  version,
		stdioFallbackTransportID: stdioFallbackID(),
	}
	s.buildToolRegistry()
	return s
}

// stdioFallbackID returns a stable per-process transport id for stdio
// connections that don't surface a native session id. Defends against the
// shortid generator's error path by falling back to a fixed prefix — better
// to land every call on one row than to scatter them.
func stdioFallbackID() string {
	id, err := shortid.Generate()
	if err != nil || id == "" {
		return "stdio-fallback"
	}
	return "stdio-" + id
}

// resolveTransportID returns the transport-level identity for an in-flight
// tool call. Streamable HTTP gives every connection a session id (the value
// of MCP-Session-Id, surfaced via req.Session.ID()); stdio has none, so we
// substitute the per-process fallback so audit logging still groups calls
// from one `breadbox mcp-stdio` invocation under one row.
func (s *MCPServer) resolveTransportID(req *mcpsdk.CallToolRequest) string {
	if req == nil || req.Session == nil {
		return s.stdioFallbackTransportID
	}
	if id := req.Session.ID(); id != "" {
		return id
	}
	return s.stdioFallbackTransportID
}

// rebindActorFromClientInfo upgrades the ctx's actor identity to the
// per-client agent key resolved from the MCP `initialize` handshake's
// clientInfo block. Returns the original ctx unchanged when there's
// no session, no initialize-params, or the service helper fails — the
// pre-PR behaviour (Local MCP fallback singleton attached at process
// start, or the HTTP request's own API key) is preserved.
//
// MUST be called BEFORE ensureAuditSession in the dispatcher.
// ensureAuditSession reads ActorFromContext(ctx) to stamp the
// mcp_sessions row's api_key_id + api_key_name on first call, and
// those columns never re-stamp. Rebinding after the session row is
// created would permanently record the wrong key for every session.
func (s *MCPServer) rebindActorFromClientInfo(ctx context.Context, req *mcpsdk.CallToolRequest) context.Context {
	if req == nil || req.Session == nil {
		return ctx
	}
	// Never clobber a scheduled agent's run key. A run is already
	// authenticated as its per-run key (agent:<slug>:<runID>, bound from
	// BREADBOX_API_KEY in runMCPStdio), which carries the agent's real
	// identity + agent_definition link. The clientInfo the Claude Agent
	// SDK sends is the generic shared "claude-code" host identity —
	// rebinding to it collapses every agent onto one client key and
	// erases per-agent attribution (the bug that made one session show
	// three different actor names + avatars). Leave the run key in place.
	//
	// IsAgentRunContext also checks actor_type='agent', so a non-agent key
	// merely *named* "agent:..." can't suppress the rebind and spoof an
	// agent identity.
	if service.IsAgentRunContext(ctx) {
		return ctx
	}
	ip := req.Session.InitializeParams()
	if ip == nil || ip.ClientInfo == nil {
		return ctx
	}
	transport := "stdio"
	if req.Session.ID() != "" {
		// HTTP sessions advertise their MCP-Session-Id; stdio has
		// none and falls back to the per-process transport id we
		// stamped ourselves.
		transport = "http"
	}
	clientInfo := service.MCPClientInfo{
		Name:       ip.ClientInfo.Name,
		Version:    ip.ClientInfo.Version,
		Title:      ip.ClientInfo.Title,
		WebsiteURL: ip.ClientInfo.WebsiteURL,
	}
	key, err := s.svc.EnsureMCPClientAgentKey(ctx, clientInfo, transport)
	if err != nil || key == nil {
		// Service-layer fallback already returned the Local MCP
		// singleton when possible; outright failure means the
		// migration hasn't applied. Keep the existing ctx so the
		// caller's pre-PR behaviour stays intact.
		return ctx
	}
	return service.ContextWithAPIKey(ctx, key)
}

// ensureAuditSession resolves (lazy-creating on first call) the
// mcp_sessions row bound to a transport id and returns its UUID as a
// string for LogToolCall. Captures clientInfo from the initialize
// request so the audit row carries the host's name/version. Logging
// failures are swallowed here — the audit trail must not block tool
// execution. Returns "" when the actor isn't an API key (the row schema
// treats that as legacy).
func (s *MCPServer) ensureAuditSession(ctx context.Context, req *mcpsdk.CallToolRequest, transportID string) string {
	if transportID == "" {
		return ""
	}
	actor := service.ActorFromContext(ctx)

	var info service.MCPClientInfo
	if req != nil && req.Session != nil {
		if ip := req.Session.InitializeParams(); ip != nil && ip.ClientInfo != nil {
			// SDK v1.5.0's Implementation block exposes Name, Title,
			// Version, WebsiteURL, Icons. There's no Description field
			// today; leave the column empty when the host doesn't
			// supply one.
			info = service.MCPClientInfo{
				Name:       ip.ClientInfo.Name,
				Version:    ip.ClientInfo.Version,
				Title:      ip.ClientInfo.Title,
				WebsiteURL: ip.ClientInfo.WebsiteURL,
			}
		}
	}

	session, err := s.svc.EnsureMCPSessionForTransport(ctx, transportID, actor, info)
	if err != nil {
		return ""
	}
	return session.ID
}

// metaReason pulls the optional "reason" string out of a tool call's
// _meta block. Hosts use this to label the call ("processing review
// queue") without polluting the tool's input schema.
func metaReason(req *mcpsdk.CallToolRequest) string {
	if req == nil || req.Params == nil {
		return ""
	}
	meta := req.Params.GetMeta()
	if meta == nil {
		return ""
	}
	if v, ok := meta["reason"].(string); ok {
		return v
	}
	return ""
}

// auditSessionContextKey carries the resolved audit-session UUID through
// to handlers that need to bind a created row to it (e.g. submit_report
// → agent_reports.session_id). The dispatcher stamps this on ctx after
// resolving the transport binding, before invoking the handler.
type auditSessionContextKey struct{}

func contextWithAuditSession(ctx context.Context, sessionID string) context.Context {
	if sessionID == "" {
		return ctx
	}
	return context.WithValue(ctx, auditSessionContextKey{}, sessionID)
}

func auditSessionFromContext(ctx context.Context) string {
	v, _ := ctx.Value(auditSessionContextKey{}).(string)
	return v
}

// buildToolRegistry populates the allTools slice with all available tools and
// their classifications. The registry carves around what an agent does
// (query, decide, write, configure) rather than every underlying entity.
// Everything is a tool — MCP resources were retired. Bounded reference data
// (accounts, categories, tags, users, sync status, rules, overview) is read
// through dedicated read tools (handlers in tools_reads.go); the near-static
// operating-guidance docs are served by get_reference(kind=…).
func (s *MCPServer) buildToolRegistry() {
	s.allTools = []ToolDef{
		// Audit sessions are bound to the transport connection (MCP-Session-Id
		// for HTTP, the per-process fallback id for stdio). Each tool call
		// resolves its session via resolveTransportID + ensureAuditSession in
		// the dispatcher, so agents no longer need to call create_session.

		// --- Reference data reads ---
		// Bounded lookup datasets, each its own tool. MCP resources were retired,
		// so these tools are the only way to read this data. get_reference (below)
		// is a separate tool that serves the operating-guidance docs, not data.
		makeToolDefLogged(ToolSpec{
			Name: "get_overview", Title: "Household Overview", Classification: ToolRead,
			Description: "Get a household snapshot: scope (users, accounts, currencies), freshness (latest sync, errored connections, recent transactions), and backlog (pending review queue). Read once at the top of a session to ground every later filter (account ids, currency, attribution).",
		}, s.handleGetOverview, s),
		makeToolDefLogged(ToolSpec{
			Name: "list_accounts", Title: "List Accounts", Classification: ToolRead,
			Description: "List bank accounts. Each account carries balance, type, currency, and the connection it belongs to. Filter by user_id to scope to a specific household member.",
		}, s.handleListAccounts, s),
		makeToolDefLogged(ToolSpec{
			Name: "list_categories", Title: "List Categories", Classification: ToolRead,
			Description: "List the category taxonomy as a flat array. Use the returned slugs (e.g. 'food_and_drink_groceries') as the canonical handle for category filters and category_slug fields on writes.",
		}, s.handleListCategories, s),
		makeToolDefLogged(ToolSpec{
			Name: "list_users", Title: "List Household Members", Classification: ToolRead,
			Description: "List household members. Each user carries display name, role, and short_id — use the short_id as user_id on transaction filters and account scoping.",
		}, s.handleListUsers, s),
		makeToolDefLogged(ToolSpec{
			Name: "list_tags", Title: "List Tags", Classification: ToolRead,
			Description: "List the tag vocabulary. Tags are referenced by slug everywhere (filter, add, remove). New tag slugs auto-register the first time update_transactions adds them — read this list before authoring rules to avoid accidental near-duplicates.",
		}, s.handleListTags, s),
		makeToolDefLogged(ToolSpec{
			Name: "get_sync_status", Title: "Sync Status", Classification: ToolRead,
			Description: "Get connection sync status: provider, status (active|error|pending_reauth|disconnected), last sync time, last error. Call this before reasoning about freshness — an errored or pending_reauth connection means transactions you'd expect to be there might not be.",
		}, s.handleGetSyncStatus, s),
		makeToolDefLogged(ToolSpec{
			Name: "list_transaction_rules", Title: "List Rules", Classification: ToolRead,
			Description: "List transaction rules (the roster). Filter by category_slug, enabled, or search by name. Lean by default: returns a summary projection (name, enabled, priority, trigger, category, hit_count) without the conditions/actions trees — pass fields=all to inspect or audit full rule definitions. For richer analysis — filter by trigger/creator/hit-count or sort by impact — use query_transaction_rules; to check whether one specific merchant is already covered, use find_matching_rules.",
		}, s.handleListTransactionRules, s),

		// --- Operating-guidance docs ---
		// The near-static markdown that teaches an agent how to drive the server.
		// Formerly breadbox:// markdown resources; served as a tool so clients
		// that can't read MCP resources can still pull them.
		makeToolDefLogged(ToolSpec{
			Name: "get_reference", Title: "Get Guidance Doc", Classification: ToolRead,
			Description: "Read an operating-guidance doc by `kind` — the near-static markdown that explains how to drive this server. kinds: 'instructions' (data model + conventions overview, how the surface is organized), 'rule-dsl' (the full transaction-rule condition grammar, action types, pipeline-stage ordering, and sync-vs-retroactive semantics — read before authoring rules), 'review-guidelines' (principles for reviewing transactions and creating rules — read before working the needs-review queue), 'report-format' (structure + formatting conventions for submit_report). Returns markdown. instructions/review-guidelines/report-format reflect any operator customization; rule-dsl is the fixed grammar.",
		}, s.handleGetReference, s),
		makeToolDefLogged(ToolSpec{
			Name: "query_transaction_rules", Title: "Query Rules", Classification: ToolRead,
			Description: "Query and analyze the rule set — the rules analogue of query_transactions. Filter by category_slug, enabled, trigger (on_create|on_change|always), creator_type (user|agent|system), name search, min_hit_count, or only_unused (rules that have never fired). Sort by priority (default, pipeline order), hit_count, last_hit_at, created_at, or name. Lean by default (summary projection: name, enabled, priority, trigger, category, hit_count, last_hit_at — no conditions/actions trees); pass fields=all for the full definitions. Use this to audit coverage and prune dead rules (only_unused=true) without dumping the whole roster. To check coverage for ONE merchant before creating a rule, prefer find_matching_rules. Cursor pagination applies only to the default priority sort; an explicit sort_by returns a single top-N page (raise limit, max 500).",
		}, s.handleQueryTransactionRules, s),
		makeToolDefLogged(ToolSpec{
			Name: "list_workflows", Title: "List Workflows", Classification: ToolRead,
			Description: "List the household's automation layer: the `workflows` it has enabled (each carries name, slug, trigger sync|schedule|manual, schedule_cron, tool_scope, the source `preset` it was instantiated from, plus last_run_status + last_run_at), and the full catalog of available `presets` it could enable (slug, name, category, description, tool_scope, trigger, default schedule_cron, and whether it's already enabled). Read this to see what runs automatically before suggesting new rules or reports — an existing workflow may already cover the task. Enabling/configuring workflows is an admin-UI action (the /workflows gallery), not an MCP write.",
		}, s.handleListWorkflows, s),
		makeToolDefLogged(ToolSpec{
			Name: "list_series", Title: "List Recurring Series", Classification: ToolRead,
			Description: "List recurring series (subscriptions, bills, loans) — thin, rule-maintained entities: each is a surrogate identity (id/short_id), a name, and a type, plus its tags. Membership comes from assign_series rules, not a shipped detector. Lean by default (name, type, tags); pass fields=all for timestamps too. Use get_series for one series, query_transactions(series_id=...) for its charges.",
		}, s.handleListSeries, s),
		makeToolDefLogged(ToolSpec{
			Name: "get_series", Title: "Get Recurring Series", Classification: ToolRead,
			Description: "Get one recurring series by short ID or UUID: its name, type, and tags. A series' linked charges come from query_transactions(series_id=...); its governing rules (the assign_series rules that define its membership) are visible on the admin Recurring detail page.",
		}, s.handleGetSeries, s),
		makeToolDefLogged(ToolSpec{
			Name: "assign_series", Title: "Assign / Create Recurring Series", Classification: ToolWrite,
			Description: "Link transactions to a recurring series, creating it if needed — the agent's path for a one-off assignment (encode a durable pattern as an assign_series RULE instead when you want future charges to resolve automatically). Provide series_id to assign to an existing series, OR series_name + create_if_missing:true to mint/resolve one by name (surrogate-first: the same name always resolves the same series). Optional type (subscription|bill|loan|other) for a minted series. Pass transaction_ids (≤50) to back-link members (NULL-fill only — never steals a charge already in another series).",
		}, s.handleAssignSeries, s),
		makeToolDefLogged(ToolSpec{
			Name: "update_series", Title: "Edit Recurring Series", Classification: ToolWrite,
			Description: "Edit a recurring series' name and/or type (subscription, bill, loan, other). Both optional — omit to leave unchanged. Renaming onto an existing live series name is rejected (the name is the series' unique mint key).",
		}, s.handleUpdateSeries, s),
		makeToolDefLogged(ToolSpec{
			Name: "unlink_series_transactions", Title: "Unlink Charges from Series", Classification: ToolWrite,
			Description: "Detach transactions (≤50, each a current member) from a recurring series — the inverse of assign_series' link path. Clears each charge's series_id and strips the series' inherited tags from them (a tag the user added directly survives). Errors if any listed transaction isn't a current member, so it can't silently no-op or touch another series.",
		}, s.handleUnlinkSeriesTransactions, s),
		makeToolDefLogged(ToolSpec{
			Name: "add_series_tag", Title: "Tag Recurring Series", Classification: ToolWrite,
			Description: "Attach an existing tag to a recurring series. The tag is materialized onto every linked transaction (they inherit it) and applied to future members as they join — so tagging the Netflix series tags all its charges. The tag must already exist (create it first with create_tag).",
		}, s.handleAddSeriesTag, s),
		makeToolDefLogged(ToolSpec{
			Name: "remove_series_tag", Title: "Untag Recurring Series", Classification: ToolWrite,
			Description: "Detach a tag from a recurring series and strip the series-inherited copies from its linked transactions. Provenance-scoped: a tag a user added directly to a transaction survives.",
		}, s.handleRemoveSeriesTag, s),

		// --- Counterparties (rules-as-substrate, P4) ---
		makeToolDefLogged(ToolSpec{
			Name: "list_counterparties", Title: "List Counterparties", Classification: ToolRead,
			Description: "List counterparties — the canonical, cross-provider 'other side' of a charge (merchants AND non-merchants: Venmo, people, employers). Each is a surrogate identity (id/short_id) with a name and optional enrichment (website_url, logo_url, category, mcc). Membership comes from assign_counterparty rules, not a normalizer. Use get_counterparty for one, query_transactions for its charges.",
		}, s.handleListCounterparties, s),
		makeToolDefLogged(ToolSpec{
			Name: "get_counterparty", Title: "Get Counterparty", Classification: ToolRead,
			Description: "Get one counterparty by short ID or UUID: its name and enrichment fields. Its governing rules (the assign_counterparty rules that define its membership) are on the admin Counterparties detail page; its linked charges come from query_transactions.",
		}, s.handleGetCounterparty, s),
		makeToolDefLogged(ToolSpec{
			Name: "create_counterparty", Title: "Create Counterparty", Classification: ToolWrite,
			Description: "Create a new counterparty with a name and optional enrichment (website_url, logo_url, category_id, mcc). Creating onto an existing live name is rejected — edit that one instead. To bind charges, use assign_counterparty (one-off) or author an assign_counterparty RULE (durable).",
		}, s.handleCreateCounterparty, s),
		makeToolDefLogged(ToolSpec{
			Name: "update_counterparty", Title: "Enrich Counterparty", Classification: ToolWrite,
			Description: "Enrich a counterparty: edit its name, website_url, logo_url, category_id (slug or short ID), and/or mcc. Every field optional — omit to leave unchanged; an empty name is rejected. This is the enrichment lane (no auto-fetch).",
		}, s.handleUpdateCounterparty, s),
		makeToolDefLogged(ToolSpec{
			Name: "assign_counterparty", Title: "Assign Counterparty", Classification: ToolWrite,
			Description: "Bind transactions to a counterparty, creating it if needed. This is a ONE-OFF assignment. For durable patterns, author an assign_counterparty RULE instead so every future matching charge resolves automatically. Provide counterparty_id to bind to an existing counterparty, OR name + create_if_missing:true to resolve-or-create one by name (surrogate-first; de-dupes on the live name). Pass transaction_ids (≤50) to link members (NULL-fill only — never steals a charge already bound to another counterparty).",
		}, s.handleAssignCounterparty, s),
		makeToolDefLogged(ToolSpec{
			Name: "unlink_counterparty_transaction", Title: "Unlink Charge from Counterparty", Classification: ToolWrite,
			Description: "Detach transactions (≤50, each a current member) from a counterparty — the inverse of assign_counterparty's link path. Clears each charge's counterparty_id. Errors if any listed transaction isn't a current member, so it can't silently no-op or touch another counterparty.",
		}, s.handleUnlinkCounterpartyTransaction, s),

		// --- Query + aggregate ---
		makeToolDefLogged(ToolSpec{
			Name: "query_transactions", Title: "Query Transactions", Classification: ToolRead,
			Description: "Query bank transactions with optional filters and cursor-based pagination. Amounts: positive = money out (debit), negative = money in (credit). Dates: YYYY-MM-DD, start_date inclusive, end_date exclusive. Filter by category_slug (see list_categories for the slug list); parent slugs include all children. Results ordered by date desc by default. Pagination: pass next_cursor from response. Responses are lean by default — a compact field set (core,category) is returned unless you pass fields=all or an explicit field/alias list. When every row shares one currency, iso_currency_code is returned once at the top level instead of on each row; otherwise each row carries its own. Pass count_only=true to get just {\"count\": N} for the same filters (no rows) — use it to size a result set or compare counts across ranges before paginating.",
		}, s.handleQueryTransactions, s),
		makeToolDefLogged(ToolSpec{
			Name: "transaction_summary", Title: "Spending Summary", Classification: ToolRead,
			Description: "Get aggregated transaction totals grouped by category and/or time period. Replaces the need to paginate through thousands of individual transactions for spending analysis. Amounts follow the convention: positive = money out (debit), negative = money in (credit). Only includes non-deleted, non-pending transactions by default.",
		}, s.handleTransactionSummary, s),

		// --- Apply review decisions ---
		// update_transactions is the universal write for review work. It
		// absorbs the per-row variants (categorize, batch-categorize, tag
		// add/remove, comment, reset-category) so an agent can land a full
		// decision atomically per transaction. Idempotent: re-running the
		// same op produces the same final state (the annotations rotate but
		// the row settles in the same place), so hosts can retry safely.
		makeToolDefLogged(ToolSpec{
			Name: "update_transactions", Title: "Update Transactions", Classification: ToolWrite,
			Description: "Compound write for up to 50 transactions at once. Each operation can: set a category (category_slug), add tags (tags_to_add), remove tags (tags_to_remove), and attach a comment — all atomically per transaction, with annotations written for every change. The canonical tool for closing review work (set category + remove needs-review + explain) in one call. Use the `comment` field to capture decision rationale; tag adds/removes carry no per-action note — keep all narrative in the comment. Example operation: {\"transaction_id\":\"k7Xm9pQ2\",\"category_slug\":\"food_and_drink_groceries\",\"tags_to_remove\":[{\"slug\":\"needs-review\"}],\"comment\":\"Clearly groceries — Costco run.\"}. on_error: 'continue' (default — each op in its own DB tx, partial failures OK) or 'abort' (one DB tx, rolls back on first error). Category writes are last-writer-wins (no provenance/locks). The response summary carries succeeded / failed counts.",
			Annotations: &mcpsdk.ToolAnnotations{DestructiveHint: boolPtr(false), IdempotentHint: true},
		}, s.handleUpdateTransactions, s),

		// --- Transaction metadata (free-form JSONB enrichment store) ---
		// One compound op (set / unset / replace) that touches ONLY the metadata
		// column, so an agent can't clobber sibling keys or first-class fields.
		// Metadata is returned on every transaction read.
		makeToolDefLogged(ToolSpec{
			Name: "set_transaction_metadata", Title: "Set Transaction Metadata", Classification: ToolWrite,
			Description: "Write a transaction's free-form metadata JSONB store. `set` upserts key→value pairs (MERGE — keys you don't list stay untouched); `unset` deletes keys (no-op if absent); `replace:true` makes the result EXACTLY the set object (clears every pre-existing key first), and replace:true with set omitted clears all metadata. Keys are slug-like, max 128 chars (e.g. 'tax_deductible', 'trip'); values may be any JSON. Metadata is for household enrichment that isn't a first-class field — it is NOT a substitute for category or tags. Returned on every transaction read (query_transactions, the transaction resource). Examples: merge → {\"transaction_id\":\"k7Xm9pQ2\",\"set\":{\"tax_deductible\":true}}; remove a key → {\"transaction_id\":\"k7Xm9pQ2\",\"unset\":[\"trip\"]}; replace all → {\"transaction_id\":\"k7Xm9pQ2\",\"replace\":true,\"set\":{\"trip\":\"q2\"}}; clear → {\"transaction_id\":\"k7Xm9pQ2\",\"replace\":true}.",
			Annotations: &mcpsdk.ToolAnnotations{DestructiveHint: boolPtr(false), IdempotentHint: true},
		}, s.handleSetTransactionMetadata, s),

		// --- Activity timeline ---
		makeToolDefLogged(ToolSpec{
			Name: "list_annotations", Title: "List Activity Timeline", Classification: ToolRead,
			Description: "List the activity timeline for a transaction, ordered by created_at ASC. Each row carries a generic `kind` (comment | rule | tag | category) plus an `action` (added | removed | set | applied) for the specific event — branch on `action` when the distinction matters (e.g. tag added vs removed). Payload carries kind-specific fields (content for comments, slug for tag events, rule_name for rule applications). Filters compose: `kinds=['comment']` is the comment-only view; `actor_types=['user']` is the canonical 'any human input?' check (drops rule churn + agent activity); `since` (RFC3339) skips rows you've already seen; `limit` returns the most recent N (capped at 200). Empty filters return the full timeline. Recommended pattern: before overriding your own categorization, call list_annotations(transaction_id, actor_types=['user']) — if any row exists, a human has weighed in and that decision wins.",
		}, s.handleListAnnotations, s),

		// --- Rules ---
		// See get_reference(kind=rule-dsl) for the condition grammar and
		// list_transaction_rules for the current ruleset.
		makeToolDefLogged(ToolSpec{
			Name: "create_transaction_rule", Title: "Create Rule", Classification: ToolWrite,
			Description: "Create one or more transaction rules for automatic categorization, tagging, or commenting. Pass `rules`: an array of 1..100 rule specs (a single rule is just a one-element array). Rules match condition trees against transactions during sync and fire in pipeline-stage order (priority ASC — lower = earlier). Pass `stage` (one of baseline|standard|refinement|override) per rule instead of a raw priority so rules from different agents compose predictably; stage resolves to priority 0/10/50/100. Earlier-stage rules' tag and category mutations feed later-stage rules' conditions, so rules compose: rule A tags 'coffee', rule B conditioned on tags-contains-coffee sets category — author such pipelines in one call. Set apply_retroactively=true on a rule to immediately back-fill it against existing transactions. Before creating, read the rules roster (get_reference kind=rules) to avoid duplicates; prefer `contains` over exact matches (bank feeds format merchant names inconsistently). Returns the created rules plus any per-item errors so a partial batch is recoverable. Full DSL: breadbox://rule-dsl.",
		}, s.handleCreateTransactionRule, s),
		makeToolDefLogged(ToolSpec{
			Name: "update_transaction_rule", Title: "Update Rule", Classification: ToolWrite,
			Description: "Update a transaction rule's fields. Every field is optional; omit to leave unchanged. Pass conditions={} to explicitly clear conditions (match-all). Pass actions=[...] to replace the entire action set (rules must retain at least one action). Pass expires_at=\"\" to clear expiry. Pass `stage` (baseline|standard|refinement|override) to re-slot a rule into the pipeline without guessing a numeric priority. See breadbox://rule-dsl.",
			Annotations: &mcpsdk.ToolAnnotations{DestructiveHint: boolPtr(false), IdempotentHint: true},
		}, s.handleUpdateTransactionRule, s),
		makeToolDefLogged(ToolSpec{
			Name: "delete_transaction_rule", Title: "Delete Rule", Classification: ToolWrite,
			Description: "Delete a transaction rule by ID. System-seeded rules (like the needs-review tagger) cannot be deleted — disable them instead with update_transaction_rule.enabled=false.",
			// Destructive: deletes the rule row. Re-running with the same id
			// is a no-op (already gone) so still IdempotentHint=true.
			Annotations: &mcpsdk.ToolAnnotations{DestructiveHint: boolPtr(true), IdempotentHint: true},
		}, s.handleDeleteTransactionRule, s),
		makeToolDefLogged(ToolSpec{
			Name: "apply_rules", Title: "Apply Rules Retroactively", Classification: ToolWrite,
			Description: "Apply rules retroactively to existing transactions. Pass rule_id to run a single rule in isolation, or omit to run the full active-rule pipeline in priority-ASC order (same chaining semantics as sync). Materializes set_category (writes category_id directly — last-writer-wins, no override protection), add_tag, and remove_tag. add_comment is sync-only and won't fire here. Hit count increments per condition match, matching sync-time semantics. Use for initial setup or explicit back-fills only — routine syncs apply rules automatically.",
			// Not idempotent — hit_count increments on every run.
		}, s.handleApplyRules, s),
		makeToolDefLogged(ToolSpec{
			Name: "preview_rule", Title: "Preview Rule", Classification: ToolRead,
			Description: "Dry-run a condition tree against existing transactions without any writes. Returns match_count + total_scanned + a sample of matching transactions. IMPORTANT: this evaluates only the supplied condition in isolation — it does NOT simulate the full rule pipeline, so tags or categories that other rules would have added mid-pass aren't visible. Use this to answer 'what does this condition match today' before creating a rule.",
		}, s.handlePreviewRule, s),
		makeToolDefLogged(ToolSpec{
			Name: "find_matching_rules", Title: "Find Matching Rules", Classification: ToolRead,
			Description: "Find which existing active rules already match a transaction — the inverse of preview_rule. Pass transaction_id to evaluate the full rule set against a real row (all condition fields checked), or merchant to check name-based coverage for free text. Returns only the matching rules (short_id, name, sets_category, trigger, priority, hit_count, match_all), ordered priority-ASC like the sync pipeline. USE THIS BEFORE creating a rule: ask 'is this merchant already covered?' with one call instead of listing all rules and scanning them. A returned rule with sets_category already handling the merchant means you should NOT create a duplicate. match_all=true flags broad conditionless rules (e.g. the needs-review tagger) that match everything — not merchant coverage.",
		}, s.handleFindMatchingRules, s),

		// --- Tag admin ---
		// Most agents won't need these — add_tag-on-transaction implicitly
		// creates new tag slugs via update_transactions. These are for
		// curating the tag vocabulary itself (renames, deletes, deliberate
		// up-front display_name/color/icon).
		makeToolDefLogged(ToolSpec{
			Name: "create_tag", Title: "Create Tag", Classification: ToolWrite,
			Description: "Register a new tag in the system. Most agents can skip this — passing a new tag slug to update_transactions auto-creates the tag. Use create_tag only when you need to set display_name/color/icon up front. Slug regex: ^[a-z0-9][a-z0-9\\-:]*[a-z0-9]$.",
		}, s.handleCreateTag, s),
		makeToolDefLogged(ToolSpec{
			Name: "update_tag", Title: "Update Tag", Classification: ToolWrite,
			Description: "Update a tag's mutable fields (display_name, description, color, icon). Slug is immutable — to rename, create a new tag + bulk re-tag + delete old. Identify the tag by UUID, short ID, or slug.",
			Annotations: &mcpsdk.ToolAnnotations{DestructiveHint: boolPtr(false), IdempotentHint: true},
		}, s.handleUpdateTag, s),
		makeToolDefLogged(ToolSpec{
			Name: "delete_tag", Title: "Delete Tag", Classification: ToolWrite,
			Description: "Delete a tag. Cascades to transaction_tags (removes the tag from every transaction). Annotations that reference the tag keep their rows with tag_id=NULL (preserves audit trail). Identify the tag by UUID, short ID, or slug.",
			// Destructive: cascades to transaction_tags. Idempotent on
			// re-run (already gone).
			Annotations: &mcpsdk.ToolAnnotations{DestructiveHint: boolPtr(true), IdempotentHint: true},
		}, s.handleDeleteTag, s),

		// --- Reporting ---
		makeToolDefLogged(ToolSpec{
			Name: "submit_report", Title: "Submit Report", Classification: ToolWrite,
			Description: "Send a message to the family's dashboard. The title is the main message — write it as a concise, self-contained 1-2 sentence summary the family can understand at a glance without expanding. The body provides the detailed breakdown (markdown with headers, bullets, transaction links). Use priority to signal urgency and author to identify your role. See breadbox://report-format for structure conventions.",
			// Not idempotent — every call posts a new dashboard message.
		}, s.handleSubmitReport, s),
	}
}

// makeToolDefLogged creates a ToolDef with logging and transport-bound audit
// session resolution. This is called during buildToolRegistry; the *MCPServer
// argument carries the service handle plus the transport-binding helpers
// (resolveTransportID, ensureAuditSession). If spec.Annotations is nil,
// defaults are derived from the classification (read → ReadOnlyHint=true,
// write → DestructiveHint=false).
func makeToolDefLogged[T any](spec ToolSpec, handler func(context.Context, *mcpsdk.CallToolRequest, T) (*mcpsdk.CallToolResult, any, error), s *MCPServer) ToolDef {
	svc := s.svc
	annotations := spec.Annotations
	if annotations == nil {
		annotations = defaultAnnotations(spec.Classification, spec.Title)
	} else if annotations.Title == "" {
		// Hosts that surface the annotation Title (some pre-spec-2025-06-18
		// clients) need it populated even when the registration site only
		// set hint flags. Tool.Title is the spec field the modern picker
		// reads; ToolAnnotations.Title is the legacy fallback.
		annotations.Title = spec.Title
	}
	tool := mcpsdk.Tool{
		Name:        spec.Name,
		Title:       spec.Title,
		Description: spec.Description,
		Annotations: annotations,
	}

	return ToolDef{
		Tool:           tool,
		Classification: spec.Classification,
		register: func(server *mcpsdk.Server) {
			wrappedHandler := func(ctx context.Context, req *mcpsdk.CallToolRequest, input T) (*mcpsdk.CallToolResult, any, error) {
				// Resolve the audit session bound to this transport
				// connection. For Streamable HTTP that's keyed off the
				// MCP-Session-Id header (req.Session.ID()); for stdio
				// the session has no native id, so we fall back to the
				// MCPServer's per-process id stamped at NewMCPServer.
				// Lazy-create the mcp_sessions row on first call so the
				// audit trail captures clientInfo without requiring
				// agents to explicitly call create_session.
				transportID := s.resolveTransportID(req)

				// Upgrade the actor from whatever auth/middleware put
				// on the ctx (typically the stdio "Local MCP" singleton
				// or an HTTP API key) to the per-client agent identity
				// keyed off clientInfo + transport. MUST happen before
				// ensureAuditSession runs — that helper reads
				// ActorFromContext(ctx) to stamp mcp_sessions.api_key_id
				// and mcp_sessions.api_key_name on first call, and those
				// columns are write-once. Doing this swap after would
				// permanently record the wrong key for every session.
				ctx = s.rebindActorFromClientInfo(ctx, req)

				auditSessionID := s.ensureAuditSession(ctx, req, transportID)

				// Optional per-call reason via _meta.reason — replaces the
				// previously-required `reason` input field on every write
				// tool. Hosts can keep tagging high-cardinality writes
				// without polluting the tool's input schema.
				reason := metaReason(req)

				// Stash the resolved audit-session id on the context so
				// handlers that bind their created rows to it
				// (submit_report → agent_reports.session_id) don't have
				// to re-resolve the transport binding themselves.
				ctx = contextWithAuditSession(ctx, auditSessionID)

				start := time.Now()
				result, out, err := handler(ctx, req, input)
				duration := time.Since(start)

				// Log tool call asynchronously.
				go func() {
					logCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()

					var reqJSON []byte
					if req != nil && req.Params.Arguments != nil {
						reqJSON = truncateBytes(req.Params.Arguments, maxLogBytes)
					}
					var respJSON []byte
					if result != nil {
						if b, err := json.Marshal(result); err == nil {
							respJSON = truncateBytes(b, maxLogBytes)
						}
					}

					actor := service.ActorFromContext(ctx)
					isErr := (result != nil && result.IsError) || err != nil
					svc.LogToolCall(logCtx, service.ToolCallLogInput{
						SessionID:      auditSessionID,
						ToolName:       spec.Name,
						Classification: string(spec.Classification),
						Reason:         reason,
						RequestJSON:    reqJSON,
						ResponseJSON:   respJSON,
						IsError:        isErr,
						Actor:          actor,
						DurationMs:     int(duration.Milliseconds()),
					})
				}()

				return result, out, err
			}
			// Pass the same Tool we stored on the def so titles and
			// annotations land on the wire identically to AllToolDefs() output.
			toolForRegistration := tool
			mcpsdk.AddTool(server, &toolForRegistration, wrappedHandler)
		},
	}
}

const maxLogBytes = 32768 // 32KB max for stored request/response JSON

// truncateBytes returns b if len <= max, otherwise truncates and appends a marker.
func truncateBytes(b []byte, max int) []byte {
	if len(b) <= max {
		return b
	}
	return append(b[:max-50], []byte(`... [truncated]"}`)...)
}

// BuildServer creates a filtered *mcpsdk.Server for the given config.
func (s *MCPServer) BuildServer(cfg MCPServerConfig) *mcpsdk.Server {
	instructions := cfg.Instructions
	if instructions == "" {
		instructions = DefaultInstructions
	}

	server := mcpsdk.NewServer(
		breadboxImplementation(s.version),
		&mcpsdk.ServerOptions{Instructions: instructions},
	)

	disabledSet := make(map[string]bool)
	for _, name := range cfg.DisabledTools {
		disabledSet[name] = true
	}

	for _, td := range s.allTools {
		if disabledSet[td.Tool.Name] {
			continue
		}
		if td.Classification == ToolWrite && cfg.APIKeyScope == "read_only" {
			continue
		}
		td.register(server)
	}

	// MCP resources (resources/* + resource templates) were retired entirely.
	// They were invisible on clients that can't resources/list (e.g. Claude.ai),
	// so every read goes through tools instead: the bounded data reads are
	// standalone tools (get_overview, list_accounts, …) and the operating-
	// guidance docs (instructions, rule-dsl, review-guidelines, report-format)
	// are served by get_reference(kind=…).
	return server
}

// AllToolDefs returns the full tool registry for admin display.
func (s *MCPServer) AllToolDefs() []ToolDef {
	return s.allTools
}

// NewHTTPHandler wraps the MCP server in a Streamable HTTP handler with per-request filtering.
func NewHTTPHandler(s *MCPServer, svc *service.Service) http.Handler {
	return mcpsdk.NewStreamableHTTPHandler(
		func(r *http.Request) *mcpsdk.Server {
			// Load MCP config from DB.
			mcpCfg, err := svc.GetMCPConfig(r.Context())
			if err != nil {
				// Fall back to defaults on error.
				mcpCfg = &service.MCPConfig{
					Mode:          "read_write",
					DisabledTools: []string{},
				}
			}

			// Get API key scope from context.
			apiKeyScope := "full_access"
			if apiKey := mw.GetAPIKey(r.Context()); apiKey != nil {
				apiKeyScope = apiKey.Scope
			}

			return s.BuildServer(MCPServerConfig{
				Mode:          mcpCfg.Mode,
				DisabledTools: mcpCfg.DisabledTools,
				Instructions:  mcpCfg.Instructions,
				APIKeyScope:   apiKeyScope,
			})
		},
		nil,
	)
}

// checkWritePermission verifies the requesting API key has write access and
// that the global MCP mode allows writes. This is a belt-and-suspenders guard
// since BuildServer already filters out write tools — but protects against
// TOCTOU races between config changes and server construction.
func (s *MCPServer) checkWritePermission(ctx context.Context) error {
	if apiKey := mw.GetAPIKey(ctx); apiKey != nil && apiKey.Scope == "read_only" {
		return fmt.Errorf("this API key has read-only access and cannot perform write operations")
	}
	return nil
}
