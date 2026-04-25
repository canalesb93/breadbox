package pages

// SessionDetailProps mirrors the data the old session_detail.html
// read off the layout data map. Built by admin.SessionDetailHandler
// and rendered via TemplateRenderer.RenderWithTempl.
//
// Optional service fields are projected to bare Go primitives by the
// handler so the templ stays free of pgtype/funcMap helpers.
type SessionDetailProps struct {
	Session   SessionDetailHeader
	ToolCalls []SessionDetailToolCall
}

// SessionDetailHeader flattens the subset of
// service.MCPSessionDetailResponse that the page header reads.
type SessionDetailHeader struct {
	Purpose       string
	AgentName     string
	APIKeyName    string
	CreatedAt     string // pre-rendered relative time (e.g. "2 minutes ago")
	ToolCallCount int64
	ErrorCount    int
	WriteCount    int
	ReadCount     int
}

// SessionDetailToolCall mirrors service.ToolCallLogResponse with the
// fields the per-call card actually reads. DurationMs is flattened to
// int32 (zero when nil) and the boolean HasDuration toggles rendering.
// Request/Response JSON are pre-rendered (pretty-printed) to bare
// strings so the templ doesn't need a funcMap helper.
type SessionDetailToolCall struct {
	ToolName        string
	Classification  string
	IsError         bool
	Reason          string
	Sequence        int
	OffsetLabel     string
	DurationMs      int32
	HasDuration     bool
	CreatedAt       string // RFC3339 — used in <time datetime=...> attr
	CreatedAtAbs    string // pre-rendered "Jan 2, 2006 3:04 PM"
	CreatedAtRel    string // pre-rendered "2 minutes ago"
	RequestPretty   string // pre-rendered indented JSON; empty when no request
	ResponsePretty  string // pre-rendered indented JSON; empty when no response
}
