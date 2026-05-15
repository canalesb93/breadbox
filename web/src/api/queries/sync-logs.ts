import { useQuery } from "@tanstack/react-query";
import { api } from "@/api/client";

// SyncLog mirrors syncLogResponse in internal/api/sync_visibility.go.
// `connection_id` on the wire is the connection's UUID (not the short_id).
export interface SyncLog {
  id: string;
  connection_id: string;
  institution_name: string;
  provider?: string;
  trigger: string; // cron | webhook | manual | initial
  status: string; // in_progress | success | error
  added_count: number;
  modified_count: number;
  removed_count: number;
  unchanged_count: number;
  error_message?: string;
  friendly_error_message?: string;
  warning_message?: string;
  started_at?: string;
  completed_at?: string;
  duration?: string;
  duration_ms?: number;
  accounts_affected: number;
}

export interface SyncLogsPage {
  sync_logs: SyncLog[];
  next_cursor?: string | null;
  has_more: boolean;
  limit: number;
  total: number;
}

export interface UseSyncLogsParams {
  // Connection UUID (short_id is not accepted by /api/v1/sync/logs).
  connectionId?: string;
  limit?: number;
}

// useSyncLogs fetches the most-recent sync history. Pass a connection UUID
// (not the short_id) — the backend filter only resolves UUIDs. Omitting
// `connectionId` returns the global feed.
export function useSyncLogs({ connectionId, limit = 30 }: UseSyncLogsParams) {
  return useQuery({
    queryKey: ["sync-logs", connectionId ?? null, limit],
    queryFn: () => {
      const qs = new URLSearchParams();
      if (connectionId) qs.set("connection_id", connectionId);
      qs.set("limit", String(limit));
      return api<SyncLogsPage>(`/api/v1/sync/logs?${qs.toString()}`);
    },
    enabled: connectionId == null || !!connectionId,
    staleTime: 30_000,
  });
}
