import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api/client";

export interface BackupRow {
  filename: string;
  size: number;
  size_formatted: string;
  created_at: string;
  trigger: string; // "manual" | "scheduled" | "unknown"
}

export interface BackupStatus {
  service_available: boolean;
  has_encryption_key: boolean;
  backup_count: number;
  total_size_bytes: number;
  total_size: string;
  schedule: string; // "" | "daily_2am" | "daily_3am" | "daily_4am" | "weekly"
  retention_days: number;
  backup_dir: string;
  database_name: string;
  preflight_ok: boolean;
  preflight_message: string;
}

export interface BackupsListResponse {
  status: BackupStatus;
  backups: BackupRow[];
}

const BACKUPS_KEY = ["backups"] as const;

export function useBackups() {
  return useQuery({
    queryKey: BACKUPS_KEY,
    queryFn: () => api<BackupsListResponse>("/web/v1/backups"),
    staleTime: 15_000,
  });
}

export function useCreateBackup() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () =>
      api<{ filename: string }>("/web/v1/backups", { method: "POST" }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: BACKUPS_KEY });
    },
  });
}

export function useDeleteBackup() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (filename: string) =>
      api<void>(`/web/v1/backups/${encodeURIComponent(filename)}`, {
        method: "DELETE",
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: BACKUPS_KEY });
    },
  });
}

export function useRestoreExistingBackup() {
  return useMutation({
    mutationFn: (filename: string) =>
      api<void>(`/web/v1/backups/${encodeURIComponent(filename)}/restore`, {
        method: "POST",
      }),
  });
}

export function useRestoreUploadedBackup() {
  return useMutation({
    mutationFn: (file: File) => {
      const fd = new FormData();
      fd.append("backup_file", file);
      return api<void>("/web/v1/backups/restore", {
        method: "POST",
        body: fd,
      });
    },
  });
}

export interface UpdateBackupScheduleInput {
  schedule: string;
  retention_days: number;
}

export function useUpdateBackupSchedule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: UpdateBackupScheduleInput) =>
      api<void>("/web/v1/backups/schedule", {
        method: "PUT",
        body: JSON.stringify(input),
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: BACKUPS_KEY });
    },
  });
}

export function backupDownloadHref(filename: string): string {
  return `/web/v1/backups/${encodeURIComponent(filename)}/download`;
}
