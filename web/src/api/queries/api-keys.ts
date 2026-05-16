import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api/client";
import type {
  APIKey,
  APIKeyActorType,
  APIKeyScope,
  CreateAPIKeyResult,
} from "@/api/types";

// useAPIKeys loads the full key catalog. The list endpoint requires a
// full_access scope (session admins/editors qualify) — read-only sessions
// will see a 403 surfaced as ApiError.
export function useAPIKeys() {
  return useQuery({
    queryKey: ["api-keys"],
    queryFn: () => api<APIKey[]>("/api/v1/api-keys"),
    staleTime: 30_000,
  });
}

export interface CreateAPIKeyInput {
  name: string;
  scope: APIKeyScope;
  actor_type: APIKeyActorType;
  actor_name?: string;
}

export function useCreateAPIKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateAPIKeyInput) =>
      api<CreateAPIKeyResult>("/api/v1/api-keys", {
        method: "POST",
        body: JSON.stringify(input),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["api-keys"] });
    },
  });
}

export function useRevokeAPIKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api<void>(`/api/v1/api-keys/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["api-keys"] });
    },
  });
}
