import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api/client";
import type {
  Connection,
  ConnectionDetail,
  CreateConnectionResult,
  CsvImportResult,
  CsvPreviewResult,
  LinkSession,
  PlaidExchangeCredentials,
  ProviderInfo,
  TellerExchangeCredentials,
} from "@/api/types";

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

// useProviders lists available bank-data providers along with their
// `configured` flag and credential schema. Used by the Connect-bank Sheet to
// gate the Plaid / Teller cards on whichever providers this server has
// credentials for. Bounded reference data — long staleTime is fine.
export function useProviders() {
  return useQuery({
    queryKey: ["providers"],
    queryFn: () => api<ProviderInfo[]>("/api/v1/providers"),
    staleTime: 5 * 60_000,
  });
}

// useProviderLinkSession requests a fresh link token for a provider that
// needs one (Plaid today). Mutation-shaped because each token is one-shot
// and short-lived — caching would either yield stale tokens or guarantee a
// re-fetch. Returns null for providers without an init flow (Teller, CSV)
// where the server replies 204; the caller should fall through to the
// provider's client-side launcher in that case.
export function useProviderLinkSession() {
  return useMutation({
    mutationFn: async ({
      provider,
      userId,
    }: {
      provider: string;
      userId: string;
    }): Promise<LinkSession | null> => {
      const res = await api<LinkSession | undefined>(
        `/api/v1/providers/${provider}/link-session`,
        {
          method: "POST",
          body: JSON.stringify({ user_id: userId }),
        },
      );
      return res ?? null;
    },
  });
}

// useCreateConnection persists a new connection via the generic dispatch
// endpoint (POST /api/v1/connections). The caller picks the discriminated
// `credentials` shape per provider — Plaid returns the public_token from
// Plaid Link's onSuccess; Teller returns the enrollment payload from
// Teller Connect. On success we invalidate the connections + accounts
// caches so the connections list and the new detail page both see the
// row immediately.
export type CreateConnectionPlaid = {
  provider: "plaid";
  user_id: string;
  credentials: PlaidExchangeCredentials;
};
export type CreateConnectionTeller = {
  provider: "teller";
  user_id: string;
  credentials: TellerExchangeCredentials;
};
export type CreateConnectionInput = CreateConnectionPlaid | CreateConnectionTeller;

export function useCreateConnection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateConnectionInput) =>
      api<CreateConnectionResult>("/api/v1/connections", {
        method: "POST",
        body: JSON.stringify(input),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["connections"] });
      qc.invalidateQueries({ queryKey: ["accounts"] });
    },
  });
}

// useCsvPreview parses a CSV upload server-side and returns the headers +
// preview rows + auto-detected column mapping. Multipart-only — the backend
// also accepts JSON+base64 but using FormData on the client side keeps the
// upload streamable and avoids a 33%-base64 inflation. The endpoint never
// persists, so we don't invalidate any caches.
export function useCsvPreview() {
  return useMutation({
    mutationFn: (file: File) => {
      const fd = new FormData();
      fd.append("file", file);
      return api<CsvPreviewResult>("/api/v1/connections/csv/preview", {
        method: "POST",
        body: fd,
      });
    },
  });
}

// useCsvImport commits a parsed CSV. Either creates a brand-new CSV
// connection (no `connectionId`) or appends rows to an existing one. On
// success we invalidate the connections + accounts + transactions caches so
// every list reflects the new rows immediately.
export interface CsvImportInput {
  file: File;
  columnMapping: Partial<Record<string, number>>;
  positiveIsDebit: boolean;
  hasDebitCredit: boolean;
  dateFormat?: string;
  // For new connections — the household member that owns the import.
  // Ignored when `connectionId` is set (the existing connection's user wins).
  userId?: string;
  accountName?: string;
  // Set to append to an existing CSV connection. Accepts the connection's
  // short_id (the public-facing 8-char ID) or its full UUID.
  connectionId?: string;
}

export function useCsvImport() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CsvImportInput) => {
      const fd = new FormData();
      fd.append("file", input.file);
      fd.append("column_mapping", JSON.stringify(input.columnMapping));
      fd.append("positive_is_debit", String(input.positiveIsDebit));
      fd.append("has_debit_credit", String(input.hasDebitCredit));
      if (input.dateFormat) fd.append("date_format", input.dateFormat);
      if (input.connectionId) {
        fd.append("connection_id", input.connectionId);
      } else {
        if (input.userId) fd.append("user_id", input.userId);
        if (input.accountName) fd.append("account_name", input.accountName);
      }
      return api<CsvImportResult>("/api/v1/connections/csv/import", {
        method: "POST",
        body: fd,
      });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["connections"] });
      qc.invalidateQueries({ queryKey: ["connection"] });
      qc.invalidateQueries({ queryKey: ["accounts"] });
      qc.invalidateQueries({ queryKey: ["transactions"] });
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
