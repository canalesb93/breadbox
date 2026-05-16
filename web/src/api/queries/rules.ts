import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api/client";
import type {
  Condition,
  CreateRuleInput,
  RulePreviewResult,
  RulesPage,
  TransactionRule,
  UpdateRuleInput,
} from "@/api/types";

// All transaction-rule queries hit the public /api/v1/rules surface — session
// auth on /api/v1/* (see internal/api/auth_session.go) makes the cookie
// sufficient. The endpoint shape is documented in docs/api-endpoints.md and
// the rule DSL grammar lives in docs/rule-dsl.md.

export interface RulesFilters {
  /** Free-text substring search across rule name. */
  search?: string;
  /** Enabled-state filter. */
  enabled?: boolean;
  /** Category slug filter — narrows to rules whose set_category targets it. */
  category?: string;
  /** Whitelisted in the admin path: created_at | hit_count | last_hit_at | priority | name. */
  sortBy?: string;
  sortDir?: "asc" | "desc";
}

// Use the offset-paginated path (the API accepts both — when `page` is set
// the service returns total / page / total_pages alongside the rule rows).
// 50 rows per page matches the v1 default.
export const RULES_PAGE_SIZE = 50;

function buildRulesParams(
  filters: RulesFilters,
  page: number,
  pageSize: number,
): URLSearchParams {
  const params = new URLSearchParams();
  params.set("page", String(page));
  params.set("page_size", String(pageSize));
  if (filters.search) params.set("search", filters.search);
  if (filters.enabled != null) params.set("enabled", String(filters.enabled));
  if (filters.category) params.set("category_slug", filters.category);
  if (filters.sortBy) params.set("sort_by", filters.sortBy);
  if (filters.sortDir) params.set("sort_dir", filters.sortDir);
  return params;
}

export function useRules(
  filters: RulesFilters,
  page: number,
  pageSize: number = RULES_PAGE_SIZE,
) {
  const key = { ...filters, page, pageSize };
  return useQuery({
    queryKey: ["rules", "page", key],
    queryFn: () => {
      const qs = buildRulesParams(filters, page, pageSize).toString();
      return api<RulesPage>(`/api/v1/rules?${qs}`);
    },
    placeholderData: (prev) => prev,
  });
}

export function useRule(id: string | undefined) {
  return useQuery({
    queryKey: ["rule", id],
    queryFn: () => api<TransactionRule>(`/api/v1/rules/${id}`),
    enabled: !!id,
  });
}

// Last N sync runs that touched this rule. The endpoint returns
// { history: [...] } — we unwrap to keep the consumer surface terse.
export interface RuleSyncRun {
  id: string;
  started_at: string;
  status: string;
  trigger: string;
  rule_hits?: Record<string, number>;
}

export function useRuleSyncHistory(id: string | undefined, limit = 10) {
  return useQuery({
    queryKey: ["rule", id, "sync-history", limit],
    queryFn: async () => {
      const res = await api<{ history: RuleSyncRun[] }>(
        `/api/v1/rules/${id}/sync-history?limit=${limit}`,
      );
      return res.history ?? [];
    },
    enabled: !!id,
  });
}

export function useCreateRule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateRuleInput) =>
      api<TransactionRule>("/api/v1/rules", {
        method: "POST",
        body: JSON.stringify(input),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["rules"] });
    },
  });
}

export function useUpdateRule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: UpdateRuleInput }) =>
      api<TransactionRule>(`/api/v1/rules/${id}`, {
        method: "PUT",
        body: JSON.stringify(input),
      }),
    onSuccess: (rule) => {
      qc.invalidateQueries({ queryKey: ["rules"] });
      qc.invalidateQueries({ queryKey: ["rule", rule.short_id] });
    },
  });
}

// useToggleRule flips `enabled`.
export function useToggleRule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) =>
      api<TransactionRule>(`/api/v1/rules/${id}`, {
        method: "PUT",
        body: JSON.stringify({ enabled }),
      }),
    onSuccess: (rule) => {
      qc.invalidateQueries({ queryKey: ["rules"] });
      qc.invalidateQueries({ queryKey: ["rule", rule.short_id] });
    },
  });
}

export function useDeleteRule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api<void>(`/api/v1/rules/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["rules"] });
    },
  });
}

// useApplyRule runs the rule retroactively across existing transactions.
// Returns the number of affected rows. Invalidates transactions + the rule's
// own caches so hit counts and recent applications update immediately.
export function useApplyRule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api<{ rule_id: string; affected_count: number }>(
        `/api/v1/rules/${id}/apply`,
        { method: "POST" },
      ),
    onSuccess: (_res, id) => {
      qc.invalidateQueries({ queryKey: ["rules"] });
      qc.invalidateQueries({ queryKey: ["rule", id] });
      qc.invalidateQueries({ queryKey: ["transactions"] });
    },
  });
}

// usePreviewRule is a dry-run: evaluates the conditions against the live
// transaction store and returns the match count + a sample. The form uses
// this for the live preview panel; nothing is written.
export function usePreviewRule() {
  return useMutation({
    mutationFn: ({
      conditions,
      sampleSize = 10,
    }: {
      conditions: Condition;
      sampleSize?: number;
    }) =>
      api<RulePreviewResult>("/api/v1/rules/preview", {
        method: "POST",
        body: JSON.stringify({ conditions, sample_size: sampleSize }),
      }),
  });
}
