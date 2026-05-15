import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api/client";
import type { Connection, ConnectionDetail } from "@/api/types";

// useConnections lists every household connection. The endpoint returns a
// bare array (no envelope). Read with the v2 session cookie via the synthetic
// API key in internal/api/auth_session.go.
export function useConnections() {
  return useQuery({
    queryKey: ["connections"],
    queryFn: () => api<Connection[]>("/api/v1/connections"),
    staleTime: 30_000,
  });
}

// useConnection fetches a single connection by short_id (or UUID). Returns the
// detail payload — same fields as the list shape plus paused, sync interval
// override, consecutive_failures, and account_count.
export function useConnection(id: string | undefined) {
  return useQuery({
    queryKey: ["connection", id],
    queryFn: () => api<ConnectionDetail>(`/api/v1/connections/${id}`),
    enabled: !!id,
    staleTime: 30_000,
  });
}

// useSetSyncInterval writes the per-connection sync-interval override. Pass
// `minutes: null` to clear the override and fall back to the global default.
export function useSetSyncInterval() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, minutes }: { id: string; minutes: number | null }) =>
      api<ConnectionDetail>(`/api/v1/connections/${id}/sync-interval`, {
        method: "POST",
        body: JSON.stringify({ interval_minutes: minutes }),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["connection"] });
      qc.invalidateQueries({ queryKey: ["connections"] });
    },
  });
}

// useSyncAll triggers a sync across every active connection. The endpoint
// returns 202 with no body — we just toast on success and let the next
// /connections fetch (after invalidation) reflect new last_synced_at values.
export function useSyncAll() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () =>
      api<void>("/api/v1/sync", {
        method: "POST",
        body: JSON.stringify({}),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["connections"] });
    },
  });
}

// useSyncConnection triggers a sync for one connection. Same async semantics
// as useSyncAll — server returns 202 immediately, the worker runs in the
// background.
export function useSyncConnection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api<void>(`/api/v1/connections/${id}/sync`, {
        method: "POST",
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["connections"] });
      qc.invalidateQueries({ queryKey: ["connection"] });
      qc.invalidateQueries({ queryKey: ["sync-logs"] });
    },
  });
}

// usePauseConnection toggles the paused flag (omits scheduled syncs). Manual
// sync still works; user-initiated sync ignores the paused flag.
export function usePauseConnection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, paused }: { id: string; paused: boolean }) =>
      api<Connection>(`/api/v1/connections/${id}/paused`, {
        method: "POST",
        body: JSON.stringify({ paused }),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["connections"] });
      qc.invalidateQueries({ queryKey: ["connection"] });
    },
  });
}

// useDisconnectConnection soft-disconnects: encrypted tokens are wiped, the
// connection row's status flips to 'disconnected', and its transactions are
// soft-deleted. Irreversible from the UI — the user has to reconnect from
// scratch.
export function useDisconnectConnection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api<void>(`/api/v1/connections/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["connections"] });
      qc.invalidateQueries({ queryKey: ["accounts"] });
    },
  });
}
