import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api/client";
import type {
  ProviderConfigResponse,
  ProviderHealthResponse,
  ProviderTestResult,
  UpdatePlaidConfigRequest,
  UpdateTellerConfigRequest,
} from "@/api/types";

// useProviderConfig returns the redacted provider configuration (Plaid +
// Teller). Sensitive fields are exposed as boolean *_set flags only.
export function useProviderConfig() {
  return useQuery({
    queryKey: ["provider-config"],
    queryFn: () => api<ProviderConfigResponse>("/api/v1/settings/providers"),
    staleTime: 30_000,
  });
}

// useProviderHealth surfaces per-provider connection counts and last-sync
// status. Used to render the health badge on each card.
export function useProviderHealth() {
  return useQuery({
    queryKey: ["provider-health"],
    queryFn: async () => {
      const res = await api<{ providers: Record<string, ProviderHealthResponse> }>(
        "/api/v1/sync/health/providers",
      );
      return res.providers;
    },
    staleTime: 30_000,
  });
}

export function useUpdatePlaidConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: UpdatePlaidConfigRequest) =>
      api<ProviderConfigResponse>("/api/v1/settings/providers/plaid", {
        method: "PUT",
        body: JSON.stringify(body),
      }),
    onSuccess: (data) => {
      qc.setQueryData(["provider-config"], data);
      qc.invalidateQueries({ queryKey: ["provider-health"] });
    },
  });
}

export function useUpdateTellerConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: UpdateTellerConfigRequest) =>
      api<ProviderConfigResponse>("/api/v1/settings/providers/teller", {
        method: "PUT",
        body: JSON.stringify(body),
      }),
    onSuccess: (data) => {
      qc.setQueryData(["provider-config"], data);
      qc.invalidateQueries({ queryKey: ["provider-health"] });
    },
  });
}

// useDisableProvider clears stored credentials and tears down the live
// provider. Existing connections stay in the DB but their syncs will fail
// until credentials are restored. Calls DELETE /api/v1/providers/{name}.
export function useDisableProvider() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (name: "plaid" | "teller") =>
      api<ProviderTestResult>(`/api/v1/providers/${name}`, {
        method: "DELETE",
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["provider-config"] });
      qc.invalidateQueries({ queryKey: ["provider-health"] });
    },
  });
}

// useTestProvider runs the server-side credentials check for plaid/teller.
// 200 OK with {ok:false, message} for invalid creds — caller branches on
// data.ok rather than catching ApiError.
export function useTestProvider() {
  return useMutation({
    mutationFn: (name: "plaid" | "teller") =>
      api<ProviderTestResult>(`/api/v1/providers/${name}/test`, {
        method: "POST",
      }),
  });
}
