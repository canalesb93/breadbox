import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, apiText } from "@/api/client";

// Types mirror the Go-side response shapes in internal/service/agents.go
// and internal/service/agent_settings.go. Endpoints documented in
// docs/api-endpoints.md.

export interface AgentRunSummary {
  short_id: string;
  status: string;
  trigger: string;
  started_at: string;
  completed_at?: string | null;
  duration_ms?: number | null;
  total_cost_usd?: number | null;
}

export interface AgentCostStats {
  run_count: number;
  total_cost_usd: number;
}

export interface AgentRecentErrorStats {
  error_count: number;
  run_count: number; // up to 5
}

export interface AgentRecentCapStats {
  cap_count: number;
  run_count: number; // up to 5
}

export interface RecentErroredAgentRun {
  agent_slug: string;
  agent_name: string;
  run_short_id: string;
  started_at: string;
  error_message?: string | null;
  duration_ms?: number | null;
  hit_cap?: "max_turns" | "max_budget" | null;
}

export interface AgentDefinition {
  id: string;
  short_id: string;
  name: string;
  slug: string;
  prompt: string;
  system_prompt?: string | null;
  schedule_cron?: string | null;
  tool_scope: "read_only" | "read_write";
  allowed_tools: string[];
  model: string;
  max_turns: number;
  max_budget_usd?: number | null;
  enabled: boolean;
  quiet_hours_start?: string | null; // "HH:MM" 24-hour; nil disables window
  quiet_hours_end?: string | null;
  last_run?: AgentRunSummary | null;
  cost_stats_30d?: AgentCostStats | null;
  next_fire_at?: string | null; // RFC3339; nil when no schedule, disabled, or unparseable
  recent_error_stats?: AgentRecentErrorStats | null;
  recent_cap_stats?: AgentRecentCapStats | null;
  last_prompt_prefix?: string | null; // most recent non-null prompt_prefix; powers "Use last prefix"
  trigger_on_sync_complete: boolean; // fire after every successful sync

  created_at: string;
  updated_at: string;
}

export interface AgentRun {
  id: string;
  short_id: string;
  agent_definition_id?: string | null;
  trigger: string;
  status: string;
  started_at: string;
  completed_at?: string | null;
  duration_ms?: number | null;
  total_cost_usd?: number | null;
  input_tokens?: number | null;
  output_tokens?: number | null;
  cache_read_tokens?: number | null;
  cache_creation_tokens?: number | null;
  turn_count?: number | null;
  max_turns_used?: number | null;
  num_tool_calls?: number | null;
  error_message?: string | null;
  transcript_path?: string | null;
  session_id?: string | null;
  operator_note?: string | null;
  prompt_prefix?: string | null;
  // hit_cap names the safety ceiling this run bumped into when it
  // terminated: "max_turns" | "max_budget" | null. Surfaces a "ran into
  // the ceiling" pill on the run history row.
  hit_cap?: "max_turns" | "max_budget" | null;
}

export const AGENT_RUN_NOTE_MAX_LEN = 2000;

// PROMPT_PREFIX_MAX_LEN matches the server-side cap in
// internal/api/agents.go::PromptPrefixMaxLen. Bump both together.
export const PROMPT_PREFIX_MAX_LEN = 2000;

export interface AgentRunListResult {
  runs: AgentRun[];
  limit: number;
  offset: number;
  has_more: boolean;
}

export interface AgentSettings {
  auth_mode: "subscription" | "api_key";
  subscription_token?: string | null; // masked
  anthropic_api_key?: string | null; // masked
  max_concurrent: number;
  global_max_budget_usd?: number | null;
  runtime_path: string;
  transcript_dir: string;
}

export interface CreateAgentInput {
  name: string;
  slug: string;
  prompt: string;
  system_prompt?: string | null;
  schedule_cron?: string | null;
  tool_scope?: "read_only" | "read_write";
  allowed_tools?: string[];
  model?: string;
  max_turns?: number;
  max_budget_usd?: number | null;
  enabled?: boolean;
  quiet_hours_start?: string | null;
  quiet_hours_end?: string | null;
  trigger_on_sync_complete?: boolean;
}

export type UpdateAgentInput = Partial<CreateAgentInput>;

export interface UpdateAgentSettingsInput {
  auth_mode?: "subscription" | "api_key";
  subscription_token?: string | null;
  anthropic_api_key?: string | null;
  max_concurrent?: number;
  global_max_budget_usd?: number | null;
  runtime_path?: string;
  transcript_dir?: string;
}

// --- Definition queries ---

export function useAgents() {
  return useQuery({
    queryKey: ["agents"],
    queryFn: () => api<AgentDefinition[]>("/api/v1/agents"),
  });
}

// useRecentErroredAgentRuns powers the run-failed banner on /v2/agents.
// 24h window + 5-row limit by default; refetches every 60s so a recent
// error surfaces without a manual page refresh.
export function useRecentErroredAgentRuns(hours = 24, limit = 5) {
  return useQuery({
    queryKey: ["agents", "recent-errors", hours, limit],
    queryFn: () =>
      api<RecentErroredAgentRun[]>(
        `/api/v1/agents/runs/recent-errors?hours=${hours}&limit=${limit}`,
      ),
    refetchInterval: 60_000,
  });
}

export function useAgent(slug: string | undefined) {
  return useQuery({
    queryKey: ["agents", slug],
    queryFn: () => api<AgentDefinition>(`/api/v1/agents/${slug}`),
    enabled: Boolean(slug),
  });
}

// --- Definition mutations ---

export function useCreateAgent() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateAgentInput) =>
      api<AgentDefinition>("/api/v1/agents", {
        method: "POST",
        body: JSON.stringify(input),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["agents"] });
    },
  });
}

export function useUpdateAgent(slug: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: UpdateAgentInput) =>
      api<AgentDefinition>(`/api/v1/agents/${slug}`, {
        method: "PATCH",
        body: JSON.stringify(input),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["agents"] });
    },
  });
}

export function useDeleteAgent() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (slug: string) =>
      api<void>(`/api/v1/agents/${slug}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["agents"] });
    },
  });
}

export function useToggleAgent() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ slug, enable }: { slug: string; enable: boolean }) =>
      api<AgentDefinition>(
        `/api/v1/agents/${slug}/${enable ? "enable" : "disable"}`,
        { method: "POST" },
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["agents"] });
    },
  });
}

export interface RunAgentNowParams {
  slug: string;
  promptPrefix?: string; // operator-supplied per-run prefix; max 2000 chars
  // promptOverride replaces def.Prompt entirely for this fire; max 40000 chars.
  // Powers the iter-45 "Test this prompt" button on the edit page. When both
  // promptPrefix and promptOverride are set, override wins on the server.
  promptOverride?: string;
}

export function useRunAgentNow() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ slug, promptPrefix, promptOverride }: RunAgentNowParams) => {
      const body: Record<string, string> = {};
      if (promptPrefix && promptPrefix.length > 0) body.prompt_prefix = promptPrefix;
      if (promptOverride && promptOverride.length > 0) body.prompt = promptOverride;
      return api<AgentRun>(`/api/v1/agents/${slug}/run`, {
        method: "POST",
        body: Object.keys(body).length > 0 ? JSON.stringify(body) : undefined,
      });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["agents"] });
    },
  });
}

// --- Run queries ---

export interface AgentRunsFilters {
  status?: string; // "" | "success" | "error" | "in_progress" | "skipped" | "timeout"
  trigger?: string; // "" | "cron" | "manual" | "webhook"
  hit_cap?: string; // "" | "max_turns" | "max_budget" | "any"
  start?: string; // YYYY-MM-DD or RFC3339
  end?: string;
}

export function useAgentRuns(
  slug: string | undefined,
  filters: AgentRunsFilters = {},
  limit = 50,
  offset = 0,
) {
  return useQuery({
    queryKey: ["agents", slug, "runs", { limit, offset, ...filters }],
    queryFn: () => {
      const qs = new URLSearchParams();
      qs.set("limit", String(limit));
      qs.set("offset", String(offset));
      if (filters.status) qs.set("status", filters.status);
      if (filters.trigger) qs.set("trigger", filters.trigger);
      if (filters.hit_cap) qs.set("hit_cap", filters.hit_cap);
      if (filters.start) qs.set("start", filters.start);
      if (filters.end) qs.set("end", filters.end);
      return api<AgentRunListResult>(
        `/api/v1/agents/${slug}/runs?${qs.toString()}`,
      );
    },
    enabled: Boolean(slug),
  });
}

export function useAgentRun(shortId: string | undefined) {
  return useQuery({
    queryKey: ["agents", "runs", shortId],
    queryFn: () => api<AgentRun>(`/api/v1/agents/runs/${shortId}`),
    enabled: Boolean(shortId),
  });
}

// --- Settings ---

export function useAgentSettings() {
  return useQuery({
    queryKey: ["agents", "settings"],
    queryFn: () => api<AgentSettings>("/api/v1/agents/settings"),
  });
}

export function useUpdateAgentSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: UpdateAgentSettingsInput) =>
      api<AgentSettings>("/api/v1/agents/settings", {
        method: "PUT",
        body: JSON.stringify(input),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["agents", "settings"] });
    },
  });
}

export function useUpdateAgentRunNote() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ shortId, note }: { shortId: string; note: string }) =>
      api<AgentRun>(`/api/v1/agents/runs/${shortId}`, {
        method: "PATCH",
        body: JSON.stringify({ note }),
      }),
    onSuccess: (_run, { shortId }) => {
      qc.invalidateQueries({ queryKey: ["agents", "runs", shortId] });
      qc.invalidateQueries({ queryKey: ["agents"] }); // any list referencing this run
    },
  });
}

// --- Readiness probe (cheap, no API call) ---

export interface AgentSubsystemStatus {
  auth_mode: string;
  auth_configured: boolean;
  binary_present: boolean;
  binary_path?: string;
  ready: boolean;
}

export function useAgentSubsystemStatus() {
  return useQuery({
    queryKey: ["agents", "status"],
    queryFn: () => api<AgentSubsystemStatus>("/api/v1/agents/status"),
  });
}

// --- Smoke test (diagnostic) ---

export interface AgentTestResult {
  auth_mode: string;
  binary_path?: string;
  model: string;
  duration_ms: number;
  total_cost_usd: number;
  input_tokens: number;
  output_tokens: number;
  response?: string;
}

export interface AgentCleanupResult {
  runs_deleted: number;
  transcripts_deleted: number;
  transcripts_scanned: number;
  retention_days: number;
  transcript_dir?: string;
}

export function useRunAgentCleanup() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () =>
      api<AgentCleanupResult>("/api/v1/agents/cleanup", { method: "POST" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["agents"] });
    },
  });
}

export function useSmokeTestAgent() {
  return useMutation({
    mutationFn: () =>
      api<AgentTestResult>("/api/v1/agents/test", { method: "POST" }),
  });
}

// --- Transcript types ---

// Discriminated union of NDJSON event shapes emitted by the breadbox sidecar.
// Payloads pass through from the SDK message objects for assistant_message /
// tool_use / tool_result; the result event has the structured shape the
// Go-side internal/service expects.

export interface AssistantContentText {
  type: "text";
  text: string;
}

export interface AssistantContentToolUse {
  type: "tool_use";
  id: string;
  name: string;
  input: unknown;
}

export type AssistantContent = AssistantContentText | AssistantContentToolUse;

export interface AssistantMessageData {
  message: {
    role: "assistant";
    content: AssistantContent[];
  };
}

export interface ToolUseData {
  type: "tool_use";
  id: string;
  name: string;
  input: Record<string, unknown>;
}

export interface ToolResultData {
  type: "tool_result";
  tool_use_id: string;
  content: unknown; // string | ContentBlock[]
  is_error?: boolean;
}

export interface ResultData {
  totalCostUsd: number;
  inputTokens: number;
  outputTokens: number;
  cacheReadTokens: number;
  cacheCreationTokens: number;
  turnCount: number;
  numToolCalls: number;
  sessionId: string;
  stopReason: string; // "end_turn" | "max_turns" | "budget_exceeded" | ""
}

export interface TranscriptErrorData {
  message: string;
  stack?: string;
  code?: string;
}

export interface CostCapHitData {
  code: string;
  message: string;
}

export interface SystemEventData {
  [key: string]: unknown;
}

interface EventBase {
  ts: number;
}

export type TranscriptEvent =
  | (EventBase & { type: "assistant_message"; data: AssistantMessageData })
  | (EventBase & { type: "tool_use"; data: ToolUseData })
  | (EventBase & { type: "tool_result"; data: ToolResultData })
  | (EventBase & { type: "result"; data: ResultData })
  | (EventBase & { type: "error"; data: TranscriptErrorData })
  | (EventBase & { type: "cost_cap_hit"; data: CostCapHitData })
  | (EventBase & { type: "system"; data: SystemEventData });

export interface TranscriptResult {
  events: TranscriptEvent[];
  rawLength: number;
  truncated: boolean;
}

// Hard cap on parsed events so a multi-MB transcript doesn't crash the
// browser. Above this the viewer renders a banner linking to the raw file.
const TRANSCRIPT_MAX_EVENTS = 500;

export function useTranscript(shortId: string | undefined) {
  return useQuery({
    queryKey: ["agents", "runs", shortId, "transcript"],
    queryFn: async (): Promise<TranscriptResult> => {
      const text = await apiText(
        `/api/v1/agents/runs/${shortId}/transcript`,
      );
      const lines = text.split("\n").filter((l) => l.trim().length > 0);
      const rawLength = lines.length;
      const slice = lines.slice(0, TRANSCRIPT_MAX_EVENTS);
      const events: TranscriptEvent[] = [];
      for (const line of slice) {
        try {
          events.push(JSON.parse(line) as TranscriptEvent);
        } catch {
          // skip malformed lines
        }
      }
      return {
        events,
        rawLength,
        truncated: rawLength > TRANSCRIPT_MAX_EVENTS,
      };
    },
    enabled: Boolean(shortId),
    // Transcripts are immutable once a run completes — keep them cached
    // across Sheet open/close.
    staleTime: 5 * 60 * 1000,
  });
}
