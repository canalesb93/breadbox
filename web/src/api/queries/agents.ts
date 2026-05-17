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
  last_run?: AgentRunSummary | null;
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
}

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

export function useRunAgentNow() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (slug: string) =>
      api<AgentRun>(`/api/v1/agents/${slug}/run`, { method: "POST" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["agents"] });
    },
  });
}

// --- Run queries ---

export interface AgentRunsFilters {
  status?: string; // "" | "success" | "error" | "in_progress" | "skipped" | "timeout"
  trigger?: string; // "" | "cron" | "manual" | "webhook"
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
