import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api/client";

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

export function useAgentRuns(slug: string | undefined, limit = 50, offset = 0) {
  return useQuery({
    queryKey: ["agents", slug, "runs", { limit, offset }],
    queryFn: () =>
      api<AgentRunListResult>(
        `/api/v1/agents/${slug}/runs?limit=${limit}&offset=${offset}`,
      ),
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
