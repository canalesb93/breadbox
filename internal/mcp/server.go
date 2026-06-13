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
// Bounded reference data (accounts, categories, tags, users, sync status,
// rules, overview) is preferred via resources — see registerResources in this
// file — but each resource also has a tool mirror in tools_reads.go so MCP
// clients that don't implement the resources/* methods can still read it.
func (s *MCPServer) buildToolRegistry() {
	s.allTools = []ToolDef{
		// Audit sessions are bound to the transport connection (MCP-Session-Id
		// for HTTP, the per-process fallback id for stdio). Each tool call
		// resolves its session via resolveTransportID + ensureAuditSession in
		// the dispatcher, so agents no longer need to call create_session.

		// --- Reference data (mirrors resources for clients without resource support) ---
		// Resources are the preferred surface: breadbox://overview, ://accounts,
		// ://categories, ://tags, ://users, ://sync-status, ://rules. These tool
		// mirrors keep clients that don't implement resources/* unblocked.
		makeToolDefLogged(ToolSpec{
			Name: "get_overview", Title: "Household Overview", Classification: ToolRead,
			Description: "Get a household snapshot: scope (users, accounts, currencies), freshness (latest sync, errored connections, recent transactions), and backlog (pending review queue). Mirror of breadbox://overview — call this when your client doesn't support MCP resources, or when you want the snapshot inline as a tool result. Read once at the top of a session to ground every later filter (account ids, currency, attribution).",
		}, s.handleGetOverview, s),
		makeToolDefLogged(ToolSpec{
			Name: "list_accounts", Title: "List Accounts", Classification: ToolRead,
			Description: "List bank accounts. Mirror of breadbox://accounts. Each account carries balance, type, currency, and the connection it belongs to. Filter by user_id to scope to a specific household member.",
		}, s.handleListAccounts, s),
		makeToolDefLogged(ToolSpec{
			Name: "list_categories", Title: "List Categories", Classification: ToolRead,
			Description: "List the category taxonomy as a flat array. Mirror of breadbox://categories. Use the returned slugs (e.g. 'food_and_drink_groceries') as the canonical handle for category filters and category_slug fields on writes.",
		}, s.handleListCategories, s),
		makeToolDefLogged(ToolSpec{
			Name: "list_users", Title: "List Household Members", Classification: ToolRead,
			Description: "List household members. Mirror of breadbox://users. Each user carries display name, role, and short_id — use the short_id as user_id on transaction filters and account scoping.",
		}, s.handleListUsers, s),
		makeToolDefLogged(ToolSpec{
			Name: "list_tags", Title: "List Tags", Classification: ToolRead,
			Description: "List the tag vocabulary. Mirror of breadbox://tags. Tags are referenced by slug everywhere (filter, add, remove). New tag slugs auto-register the first time update_transactions adds them — read this list before authoring rules to avoid accidental near-duplicates.",
		}, s.handleListTags, s),
		makeToolDefLogged(ToolSpec{
			Name: "get_sync_status", Title: "Sync Status", Classification: ToolRead,
			Description: "Get connection sync status: provider, status (active|error|pending_reauth|disconnected), last sync time, last error. Mirror of breadbox://sync-status. Call this before reasoning about freshness — an errored or pending_reauth connection means transactions you'd expect to be there might not be.",
		}, s.handleGetSyncStatus, s),
		makeToolDefLogged(ToolSpec{
			Name: "list_transaction_rules", Title: "List Rules", Classification: ToolRead,
			Description: "List transaction rules (the roster). Filter by category_slug, enabled, or search by name. Lean by default: returns a summary projection (name, enabled, priority, trigger, category, hit_count) without the conditions/actions trees — pass fields=all to inspect or audit full rule definitions. Mirror of breadbox://rules. For richer analysis — filter by trigger/creator/hit-count or sort by impact — use query_transaction_rules; to check whether one specific merchant is already covered, use find_matching_rules.",
		}, s.handleListTransactionRules, s),
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
			Description: "List detected recurring series (subscriptions, bills, loans). Optional status filter (active|candidate|paused|cancelled). Lean by default: each row carries identity + renewal prediction (cadence, expected_amount + iso_currency_code — never sum across currencies, next_expected_date, occurrence_count, confidence), but NOT the verbose detection_signals evidence. Read status=candidate to find series awaiting a confirm/reject verdict; pass fields=all (or use get_series for one series) to see detection_signals — interval_cv, amount_branch, merchant_key_is_fallback — before deciding.",
		}, s.handleListSeries, s),
		makeToolDefLogged(ToolSpec{
			Name: "get_series", Title: "Get Recurring Series", Classification: ToolRead,
			Description: "Get one recurring series by short ID or UUID, including its full detection_signals. Use before reviewing a candidate to inspect the evidence (occurrence_count, interval_cv, cadence_snap_error, amount_branch, monotonic drift).",
		}, s.handleGetSeries, s),
		makeToolDefLogged(ToolSpec{
			Name: "explain_series_candidates", Title: "Explain Near-Miss Series", Classification: ToolRead,
			Description: "Answer \"why isn't <merchant> a subscription?\". Reports every recurring-looking merchant group that is NOT already a series, with the detector's verdict: qualifies=true (eligible but not tracked yet — confirm it with assign_series) or a specific reason it fell short (too_few_occurrences, irregular_cadence, interval_too_variable, amount_unstable, same_day_duplicates). Each row carries a human explanation plus the numbers (occurrence_count, nearest_cadence, median_gap_days, interval_cv, amount min/max). Read-only analysis over the trailing detection window — the precision-first detector deliberately stays quiet on these, so this is how you surface what it skipped.",
		}, s.handleExplainSeriesCandidates, s),
		makeToolDefLogged(ToolSpec{
			Name: "review_series", Title: "Review Recurring Series", Classification: ToolWrite,
			Description: "Apply a verdict to a recurring series: confirm (it is recurring → active), reject (NOT recurring → sticky, never re-proposed at that amount band), pause, or cancel. A user's prior confirmation outranks a later agent write. This is how an agent adjudicates the candidates surfaced by list_series(status=candidate).",
		}, s.handleReviewSeries, s),
		makeToolDefLogged(ToolSpec{
			Name: "assign_series", Title: "Assign / Create Recurring Series", Classification: ToolWrite,
			Description: "Create a recurring series detection missed, or link transactions to an existing one — the agent's path to fix gaps. Provide series_id to assign to an existing series, OR merchant_key + create_if_missing:true to mint one (funnels through the same dedup + sticky-reject arbitration as the detector, so re-creating a user-rejected series at the same signature is a no-op). Pass transaction_ids (≤50) to back-link members (NULL-fill only — never steals a charge already in another series). confirm:true flips it straight to active; omit to leave a reviewable candidate. Use after list_series(status=candidate) shows nothing for a recurring charge the user says exists.",
		}, s.handleAssignSeries, s),
		makeToolDefLogged(ToolSpec{
			Name: "update_series", Title: "Edit Recurring Series", Classification: ToolWrite,
			Description: "Edit a recurring series' user-owned attributes: name, expected_amount (+ currency, amount_tolerance), cadence, expected_day, category_id, user_id (owner). Every field is optional — omit to leave unchanged. This is a deliberate override, not a detection proposal: it bypasses the precedence ladder and protects the edited values from being reverted by the next sync's re-detect. Editing cadence re-derives next_expected_date. Changing currency or owner is collision-guarded (they're part of the dedup signature). Use review_series for confirm/pause/cancel, set_series_type for the type axis, and rekey_series for the merchant_key — those have their own semantics and are NOT editable here.",
		}, s.handleUpdateSeries, s),
		makeToolDefLogged(ToolSpec{
			Name: "set_series_type", Title: "Set Recurring Type", Classification: ToolWrite,
			Description: "Set a recurring series' type: subscription (streaming/SaaS/memberships), bill (rent/utilities/insurance/telecom), loan (mortgage/auto/student/personal), or other. Detection infers the type from the charges' category on first detection; use this to correct it. The override is sticky — re-detection won't change it back.",
		}, s.handleSetSeriesType, s),
		makeToolDefLogged(ToolSpec{
			Name: "rekey_series", Title: "Re-key Recurring Series", Classification: ToolWrite,
			Description: "Correct a series' merchant_key when detection grouped it under a wrong or fallback key (e.g. 'payment' → 'spotify'). Repoints the series and its linked transactions to the new key. Refuses to silently merge: errors if a live series already exists at the new key, or that key is sticky-rejected. Corrects historical grouping — future charges still key off the provider name.",
		}, s.handleRekeySeries, s),
		makeToolDefLogged(ToolSpec{
			Name: "split_series", Title: "Split Recurring Series", Classification: ToolWrite,
			Description: "Break an over-grouped series into two: move the given transaction_ids (≤50, each a current member of the source series) into a brand-new series under new_merchant_key. The fix for the detector sweeping a stray charge into a real subscription (e.g. a $4.99 add-on bundled with a $139/yr renewal). The new series inherits the source's currency/user/category; rollups recompute on both. Errors if new_merchant_key equals the source key or already has a series.",
		}, s.handleSplitSeries, s),
		makeToolDefLogged(ToolSpec{
			Name: "unlink_series_transactions", Title: "Unlink Charges from Series", Classification: ToolWrite,
			Description: "Detach transactions (≤50, each a current member) from a recurring series — the inverse of assign_series' link path. Clears each charge's series_id, strips the series' inherited tags from them (a tag the user added directly survives), and recomputes the series' rollups + next expected date. Errors if any listed transaction isn't a current member, so it can't silently no-op or touch another series. Use to remove a charge the detector wrongly swept in; use split_series instead when the stray charges form their own series.",
		}, s.handleUnlinkSeriesTransactions, s),
		makeToolDefLogged(ToolSpec{
			Name: "add_series_tag", Title: "Tag Recurring Series", Classification: ToolWrite,
			Description: "Attach an existing tag to a recurring series. The tag is materialized onto every linked transaction (they inherit it) and applied to future members as they join — so tagging the Netflix series tags all its charges. The tag must already exist (create it first with create_tag).",
		}, s.handleAddSeriesTag, s),
		makeToolDefLogged(ToolSpec{
			Name: "remove_series_tag", Title: "Untag Recurring Series", Classification: ToolWrite,
			Description: "Detach a tag from a recurring series and strip the series-inherited copies from its linked transactions. Provenance-scoped: a tag a user added directly to a transaction survives.",
		}, s.handleRemoveSeriesTag, s),

		// --- Query + aggregate ---
		makeToolDefLogged(ToolSpec{
			Name: "query_transactions", Title: "Query Transactions", Classification: ToolRead,
			Description: "Query bank transactions with optional filters and cursor-based pagination. Amounts: positive = money out (debit), negative = money in (credit). Dates: YYYY-MM-DD, start_date inclusive, end_date exclusive. Filter by category_slug (see breadbox://categories for the slug list); parent slugs include all children. Results ordered by date desc by default. Pagination: pass next_cursor from response. Responses are lean by default — a compact field set (core,category) is returned unless you pass fields=all or an explicit field/alias list. When every row shares one currency, iso_currency_code is returned once at the top level instead of on each row; otherwise each row carries its own.",
		}, s.handleQueryTransactions, s),
		makeToolDefLogged(ToolSpec{
			Name: "count_transactions", Title: "Count Transactions", Classification: ToolRead,
			Description: "Count transactions matching optional filters. Same filters as query_transactions except cursor, limit, sort_by, and sort_order. Use this to get totals before paginating, or to compare counts across date ranges or categories.",
		}, s.handleCountTransactions, s),
		makeToolDefLogged(ToolSpec{
			Name: "transaction_summary", Title: "Spending Summary", Classification: ToolRead,
			Description: "Get aggregated transaction totals grouped by category and/or time period. Replaces the need to paginate through thousands of individual transactions for spending analysis. Amounts follow the convention: positive = money out (debit), negative = money in (credit). Only includes non-deleted, non-pending transactions by default.",
		}, s.handleTransactionSummary, s),
		makeToolDefLogged(ToolSpec{
			Name: "merchant_summary", Title: "Merchant Summary", Classification: ToolRead,
			Description: "Aggregate spend per MERCHANT over a window (default last 90 days) — the merchant-grouped companion to transaction_summary, which groups by category/time but not merchant. Each row: merchant, transaction_count, total_amount, avg_amount, first_date, last_date. Amounts follow the convention positive = money out (debit), negative = money in (credit); never summed across currencies. Use min_count=2 to surface recurring merchants, min_count=3+ for subscriptions; spending_only=true to exclude credits/refunds in a spending report; first_date to spot newly-appearing merchants. Filter by category_slug, account_id, user_id, amount range, or search (fuzzy-tolerant for bank-feed name variants). Capped at 500 merchants.",
		}, s.handleMerchantSummary, s),

		// --- Apply review decisions ---
		// update_transactions is the universal write for review work. It
		// absorbs the per-row variants (categorize, batch-categorize, tag
		// add/remove, comment, reset-category) so an agent can land a full
		// decision atomically per transaction. Idempotent: re-running the
		// same op produces the same final state (the annotations rotate but
		// the row settles in the same place), so hosts can retry safely.
		makeToolDefLogged(ToolSpec{
			Name: "update_transactions", Title: "Update Transactions", Classification: ToolWrite,
			Description: "Compound write for up to 50 transactions at once. Each operation can: set a category (category_slug), add tags (tags_to_add), remove tags (tags_to_remove), and attach a comment — all atomically per transaction, with annotations written for every change. The canonical tool for closing review work (set category + remove needs-review + explain) in one call. Use the `comment` field to capture decision rationale; tag adds/removes carry no per-action note — keep all narrative in the comment. Example operation: {\"transaction_id\":\"k7Xm9pQ2\",\"category_slug\":\"food_and_drink_groceries\",\"tags_to_remove\":[{\"slug\":\"needs-review\"}],\"comment\":\"Clearly groceries — Costco run.\"}. on_error: 'continue' (default — each op in its own DB tx, partial failures OK) or 'abort' (one DB tx, rolls back on first error). Category writes follow precedence user > agent > rule: an agent op that targets a user-locked row comes back with status 'skipped' (its category is left alone; any tags/comment in that op still apply). The response summary carries succeeded / skipped / failed counts.",
			Annotations: &mcpsdk.ToolAnnotations{DestructiveHint: boolPtr(false), IdempotentHint: true},
		}, s.handleUpdateTransactions, s),

		// --- Transaction metadata (free-form JSONB enrichment store) ---
		// Four deliberately-scoped ops; each touches ONLY the metadata column and
		// names exactly what it does so an agent can't clobber sibling keys or
		// other fields. Metadata is returned on every transaction read.
		makeToolDefLogged(ToolSpec{
			Name: "set_transaction_metadata", Title: "Set Transaction Metadata", Classification: ToolWrite,
			Description: "Upsert ONE key in a transaction's free-form metadata JSONB store, leaving every other key untouched. Creates the key if absent, overwrites if present. The value may be any JSON value (string, number, boolean, object, array). Use slug-like keys, max 128 chars (e.g. 'tax_deductible', 'trip', 'reimbursable_by'). Metadata is a place for enrichment your household cares about that isn't a first-class field — it is NOT a substitute for category or tags. Returned on every transaction read (query_transactions, the transaction resource). Example: {\"transaction_id\":\"k7Xm9pQ2\",\"key\":\"tax_deductible\",\"value\":true}.",
			Annotations: &mcpsdk.ToolAnnotations{DestructiveHint: boolPtr(false), IdempotentHint: true},
		}, s.handleSetTransactionMetadata, s),
		makeToolDefLogged(ToolSpec{
			Name: "remove_transaction_metadata", Title: "Remove Transaction Metadata Key", Classification: ToolWrite,
			Description: "Delete ONE key from a transaction's metadata JSONB store. No-op (still succeeds) if the key isn't present. Other keys are untouched.",
			Annotations: &mcpsdk.ToolAnnotations{DestructiveHint: boolPtr(false), IdempotentHint: true},
		}, s.handleRemoveTransactionMetadata, s),
		makeToolDefLogged(ToolSpec{
			Name: "replace_transaction_metadata", Title: "Replace Transaction Metadata", Classification: ToolWrite,
			Description: "Atomically replace the ENTIRE metadata object on a transaction. Use to write a structured payload in one call. Pass {} to clear all keys. Prefer set_transaction_metadata when you only mean to change one key — replace overwrites everything.",
			Annotations: &mcpsdk.ToolAnnotations{DestructiveHint: boolPtr(false), IdempotentHint: true},
		}, s.handleReplaceTransactionMetadata, s),
		makeToolDefLogged(ToolSpec{
			Name: "clear_transaction_metadata", Title: "Clear Transaction Metadata", Classification: ToolWrite,
			Description: "Reset a transaction's metadata to the empty object {}, removing all keys. Equivalent to replace_transaction_metadata with {}.",
			Annotations: &mcpsdk.ToolAnnotations{DestructiveHint: boolPtr(false), IdempotentHint: true},
		}, s.handleClearTransactionMetadata, s),

		// --- Flag (surface a transaction for human attention) ---
		makeToolDefLogged(ToolSpec{
			Name: "flag_transaction", Title: "Flag Transaction", Classification: ToolWrite,
			Description: "Flag a transaction for human attention (sets flagged_at) without changing its category. Pass an optional `reason` — it's recorded as a comment annotation on the timeline. This is the 'look at this' escape hatch: when you auto-categorize but are unsure, or spot something worth a human glance, flag it instead of guessing. Retrieve flagged transactions with query_transactions(flagged=true). Idempotent: re-flagging refreshes the timestamp.",
			Annotations: &mcpsdk.ToolAnnotations{DestructiveHint: boolPtr(false), IdempotentHint: true},
		}, s.handleFlagTransaction, s),
		makeToolDefLogged(ToolSpec{
			Name: "unflag_transaction", Title: "Unflag Transaction", Classification: ToolWrite,
			Description: "Clear the flag on a transaction (sets flagged_at = NULL). No-op if it isn't flagged.",
			Annotations: &mcpsdk.ToolAnnotations{DestructiveHint: boolPtr(false), IdempotentHint: true},
		}, s.handleUnflagTransaction, s),

		// --- Activity timeline ---
		makeToolDefLogged(ToolSpec{
			Name: "list_annotations", Title: "List Activity Timeline", Classification: ToolRead,
			Description: "List the activity timeline for a transaction, ordered by created_at ASC. Each row carries a generic `kind` (comment | rule | tag | category) plus an `action` (added | removed | set | applied) for the specific event — branch on `action` when the distinction matters (e.g. tag added vs removed). Payload carries kind-specific fields (content for comments, slug for tag events, rule_name for rule applications). Filters compose: `kinds=['comment']` is the comment-only view; `actor_types=['user']` is the canonical 'any human input?' check (drops rule churn + agent activity); `since` (RFC3339) skips rows you've already seen; `limit` returns the most recent N (capped at 200). Empty filters return the full timeline. Recommended pattern: before overriding your own categorization, call list_annotations(transaction_id, actor_types=['user']) — if any row exists, a human has weighed in and that decision wins.",
		}, s.handleListAnnotations, s),

		// --- Rules ---
		// See breadbox://rule-dsl for the condition grammar and breadbox://rules
		// for the current ruleset.
		makeToolDefLogged(ToolSpec{
			Name: "create_transaction_rule", Title: "Create Rule", Classification: ToolWrite,
			Description: "Create a transaction rule for automatic categorization, tagging, or commenting. Rules match condition trees against transactions during sync and fire in pipeline-stage order (priority ASC — lower = earlier). Pass `stage` (one of baseline|standard|refinement|override) instead of a raw priority so rules from different agents compose predictably; stage resolves to priority 0/10/50/100. Earlier-stage rules' tag and category mutations feed later-stage rules' conditions, so rules compose: rule A tags 'coffee', rule B conditioned on tags-contains-coffee sets category. Before creating, read breadbox://rules to avoid duplicates; prefer `contains` over exact matches (bank feeds format merchant names inconsistently). Full DSL: breadbox://rule-dsl.",
		}, s.handleCreateTransactionRule, s),
		makeToolDefLogged(ToolSpec{
			Name: "batch_create_rules", Title: "Create Rules in Batch", Classification: ToolWrite,
			Description: "Create multiple transaction rules at once. More efficient than looping create_transaction_rule. Ideal for composable pipelines — use `stage` (baseline|standard|refinement|override) on each item to order rules so earlier-stage rules set up tags/categories that later-stage rules react to. `stage` is preferred over raw `priority` for cross-agent consistency; if both are supplied, priority wins. Each item follows the same shape as create_transaction_rule. Returns created rules plus any per-item errors so partial success is recoverable.",
		}, s.handleBatchCreateRules, s),
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
			Description: "Apply rules retroactively to existing transactions. Pass rule_id to run a single rule in isolation, or omit to run the full active-rule pipeline in priority-ASC order (same chaining semantics as sync). Materializes set_category (skips rows where category_override <> 'none' — an agent or user already set it), add_tag, and remove_tag. add_comment is sync-only and won't fire here. Hit count increments per condition match, matching sync-time semantics. Use for initial setup or explicit back-fills only — routine syncs apply rules automatically.",
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

	s.registerResources(server)
	return server
}

// registerResources adds MCP resources to a server. The catalog is
// agent-facing first: most resources are audienced to the assistant only.
// A handful (overview, accounts, review-guidelines, report-format) are
// dual-audience because users have a real "attach this in chat" flow for
// them — those show up in Claude.ai's paperclip / attachment menu.
func (s *MCPServer) registerResources(server *mcpsdk.Server) {
	// Top-level live snapshot — read first.
	server.AddResource(&mcpsdk.Resource{
		Name:        "Overview",
		Title:       "Household Overview",
		URI:         "breadbox://overview",
		Description: "Live summary of the dataset: users, connections, accounts, transaction counts and date range, recent spending. Read on connect for context.",
		MIMEType:    "application/json",
		Annotations: resourceAnnotations(audienceUserAndAssistant, 1.0, liveResourceModTime),
	}, s.handleOverviewResource)

	// Live state resources — replace what would otherwise be list_* tool calls.
	server.AddResource(&mcpsdk.Resource{
		Name:        "Accounts",
		Title:       "Bank Accounts",
		URI:         "breadbox://accounts",
		Description: "Bank accounts (checking, savings, credit cards, loans, investments) with balances, institution names, and currency.",
		MIMEType:    "application/json",
		Annotations: resourceAnnotations(audienceUserAndAssistant, 0.6, liveResourceModTime),
	}, s.handleAccountsResource)

	server.AddResource(&mcpsdk.Resource{
		Name:        "Categories",
		Title:       "Category Taxonomy",
		URI:         "breadbox://categories",
		Description: "Two-level category taxonomy keyed by slug. Source for valid category_slug values when categorizing or authoring rules.",
		MIMEType:    "application/json",
		Annotations: resourceAnnotations(audienceAssistantOnly, 0.5, liveResourceModTime),
	}, s.handleCategoriesResource)

	server.AddResource(&mcpsdk.Resource{
		Name:        "Tags",
		Title:       "Tag Vocabulary",
		URI:         "breadbox://tags",
		Description: "Current tag vocabulary keyed by slug. The 'needs-review' tag is the review-queue handle.",
		MIMEType:    "application/json",
		Annotations: resourceAnnotations(audienceAssistantOnly, 0.5, liveResourceModTime),
	}, s.handleTagsResource)

	server.AddResource(&mcpsdk.Resource{
		Name:        "Users",
		Title:       "Household Members",
		URI:         "breadbox://users",
		Description: "Household members with their roles (admin, editor, viewer).",
		MIMEType:    "application/json",
		Annotations: resourceAnnotations(audienceAssistantOnly, 0.5, liveResourceModTime),
	}, s.handleUsersResource)

	server.AddResource(&mcpsdk.Resource{
		Name:        "Sync Status",
		Title:       "Connection Sync Status",
		URI:         "breadbox://sync-status",
		Description: "Per-connection sync status and last-sync timestamps. Read to verify data freshness before answering questions about recent activity.",
		MIMEType:    "application/json",
		Annotations: resourceAnnotations(audienceAssistantOnly, 0.6, liveResourceModTime),
	}, s.handleSyncStatusResource)

	server.AddResource(&mcpsdk.Resource{
		Name:        "Rules",
		Title:       "Transaction Rules",
		URI:         "breadbox://rules",
		Description: "Currently registered transaction rules with their conditions, actions, trigger, priority, hit_count, and last_hit_at. Read before authoring new rules to avoid duplicates and to spot stale or never-matching rules.",
		MIMEType:    "application/json",
		Annotations: resourceAnnotations(audienceAssistantOnly, 0.6, liveResourceModTime),
	}, s.handleRulesResource)

	// Workflow guides — markdown, user-overridable via app_config.
	server.AddResource(&mcpsdk.Resource{
		Name:        "Review Guidelines",
		Title:       "Transaction Review Guidelines",
		URI:         "breadbox://review-guidelines",
		Description: "Principles for reviewing transactions and creating rules. Read before touching the review queue.",
		MIMEType:    "text/markdown",
		Annotations: resourceAnnotations(audienceUserAndAssistant, 0.8, staticPromptModTime),
	}, s.handleReviewGuidelinesResource)

	server.AddResource(&mcpsdk.Resource{
		Name:        "Report Format",
		Title:       "Spending Report Format",
		URI:         "breadbox://report-format",
		Description: "Report structure and formatting guidelines. Read before submit_report.",
		MIMEType:    "text/markdown",
		Annotations: resourceAnnotations(audienceUserAndAssistant, 0.8, staticPromptModTime),
	}, s.handleReportFormatResource)

	// Authoring reference — only relevant when the agent is creating rules.
	// Carrying the grammar here instead of in create_transaction_rule's
	// description keeps tools/list lean.
	server.AddResource(&mcpsdk.Resource{
		Name:        "Rule DSL",
		Title:       "Transaction Rule DSL",
		URI:         "breadbox://rule-dsl",
		Description: "Condition grammar, action types, priority bands, sync-vs-retroactive semantics, provider quirks. Read before authoring rules.",
		MIMEType:    "text/markdown",
		Annotations: resourceAnnotations(audienceAssistantOnly, 0.7, staticPromptModTime),
	}, staticMarkdownResource("breadbox://rule-dsl", DefaultRuleDSL))

	// --- Resource templates ---
	// Drill-downs into a single entity. URIs come back from tool results as
	// `resource_link` blocks (PR 05 in this stack) and resolve through these
	// handlers. dual-audience so they appear in template-aware pickers as
	// well; priority is below top-level resources.
	server.AddResourceTemplate(&mcpsdk.ResourceTemplate{
		Name:        "Transaction",
		Title:       "Transaction Detail",
		URITemplate: "breadbox://transaction/{short_id}",
		Description: "Single transaction with its activity timeline (annotations). short_id is the 8-char base62 id surfaced everywhere by query_transactions.",
		MIMEType:    "application/json",
		Annotations: resourceAnnotations(audienceUserAndAssistant, 0.7, liveResourceModTime),
	}, s.handleTransactionTemplate)

	server.AddResourceTemplate(&mcpsdk.ResourceTemplate{
		Name:        "Account",
		Title:       "Account Detail",
		URITemplate: "breadbox://account/{short_id}",
		Description: "Single bank account with balance, currency, and the most recent 25 transactions. short_id is the 8-char base62 id surfaced by list_accounts.",
		MIMEType:    "application/json",
		Annotations: resourceAnnotations(audienceUserAndAssistant, 0.7, liveResourceModTime),
	}, s.handleAccountTemplate)

	server.AddResourceTemplate(&mcpsdk.ResourceTemplate{
		Name:        "Household Member",
		Title:       "Household Member Detail",
		URITemplate: "breadbox://user/{short_id}",
		Description: "Single household member with their connected accounts. short_id is the 8-char base62 id surfaced by list_users.",
		MIMEType:    "application/json",
		Annotations: resourceAnnotations(audienceUserAndAssistant, 0.7, liveResourceModTime),
	}, s.handleUserTemplate)
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
