import { useRef, useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import {
  AlertTriangle,
  Calendar,
  CheckCircle2,
  Download,
  HardDrive,
  Loader2,
  RotateCcw,
  Trash2,
  Upload,
} from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  backupDownloadHref,
  useBackups,
  useCreateBackup,
  useDeleteBackup,
  useRestoreExistingBackup,
  useRestoreUploadedBackup,
  useUpdateBackupSchedule,
  type BackupRow,
  type BackupStatus,
} from "@/api/queries/backups";
import { withMutationToast } from "@/lib/mutation-toast";
import { EmptyState } from "@/components/empty-state";

const SCHEDULE_OPTIONS = [
  { value: "off", label: "Disabled — manual only" },
  { value: "daily_2am", label: "Daily at 2:00 AM" },
  { value: "daily_3am", label: "Daily at 3:00 AM" },
  { value: "daily_4am", label: "Daily at 4:00 AM" },
  { value: "weekly", label: "Weekly (Sunday at 3:00 AM)" },
] as const;

const SCHEDULE_LABEL: Record<string, string> = Object.fromEntries(
  SCHEDULE_OPTIONS.map((o) => [o.value === "off" ? "" : o.value, o.label]),
);

export function BackupsSection() {
  const query = useBackups();

  return (
    <div className="space-y-6">
      <div className="space-y-1">
        <h2 className="text-lg font-medium">Backups</h2>
        <p className="text-muted-foreground text-sm">
          Compressed PostgreSQL dumps (<code className="font-mono text-xs">.sql.gz</code>) of the
          household database. Restore replaces every row — handle with care.
        </p>
      </div>

      {query.isLoading ? (
        <BackupsSkeleton />
      ) : query.isError ? (
        <Alert variant="destructive">
          <AlertTriangle className="size-4" />
          <AlertTitle>Couldn't load backups</AlertTitle>
          <AlertDescription>{query.error.message}</AlertDescription>
        </Alert>
      ) : query.data ? (
        <BackupsContent data={query.data.status} backups={query.data.backups} />
      ) : null}
    </div>
  );
}

function BackupsContent({
  data,
  backups,
}: {
  data: BackupStatus;
  backups: BackupRow[];
}) {
  return (
    <div className="space-y-6">
      {!data.service_available && (
        <Alert variant="destructive">
          <AlertTriangle className="size-4" />
          <AlertTitle>Backup service unavailable</AlertTitle>
          <AlertDescription>
            {data.preflight_message ||
              "pg_dump is not on PATH. Install the PostgreSQL client on the host."}
          </AlertDescription>
        </Alert>
      )}

      {data.service_available && !data.preflight_ok && (
        <Alert variant="destructive">
          <AlertTriangle className="size-4" />
          <AlertTitle>Preflight check failed</AlertTitle>
          <AlertDescription>{data.preflight_message}</AlertDescription>
        </Alert>
      )}

      {!data.has_encryption_key && (
        <Alert>
          <AlertTriangle className="size-4" />
          <AlertTitle>No encryption key configured</AlertTitle>
          <AlertDescription>
            Backups include encrypted provider tokens. Without{" "}
            <code className="font-mono text-xs">ENCRYPTION_KEY</code>, they cannot be restored on
            another host.
          </AlertDescription>
        </Alert>
      )}

      <StatusGrid data={data} />

      <Separator />

      <BackupActions disabled={!data.service_available || !data.preflight_ok} />

      <Separator />

      <ScheduleForm
        initialSchedule={data.schedule}
        initialRetentionDays={data.retention_days}
      />

      <Separator />

      <BackupsTable
        backups={backups}
        restoreDisabled={!data.service_available || !data.preflight_ok}
      />
    </div>
  );
}

function StatusGrid({ data }: { data: BackupStatus }) {
  const scheduleLabel =
    data.schedule in SCHEDULE_LABEL
      ? SCHEDULE_LABEL[data.schedule]
      : data.schedule || "Disabled";

  return (
    <dl className="grid grid-cols-2 gap-4 sm:grid-cols-4">
      <Stat label="Backups" value={String(data.backup_count)} icon={<HardDrive className="size-3.5" />} />
      <Stat label="Total size" value={data.total_size} />
      <Stat label="Schedule" value={scheduleLabel} icon={<Calendar className="size-3.5" />} />
      <Stat label="Retention" value={`${data.retention_days} days`} />
      <Stat label="Database" value={data.database_name} mono className="col-span-2" />
      <Stat
        label="Backup directory"
        value={data.backup_dir || "—"}
        mono
        className="col-span-2"
      />
    </dl>
  );
}

function Stat({
  label,
  value,
  mono,
  icon,
  className,
}: {
  label: string;
  value: string;
  mono?: boolean;
  icon?: React.ReactNode;
  className?: string;
}) {
  return (
    <div className={"space-y-0.5 " + (className ?? "")}>
      <dt className="text-muted-foreground flex items-center gap-1 text-xs uppercase tracking-wider">
        {icon}
        {label}
      </dt>
      <dd
        className={
          mono
            ? "truncate font-mono text-sm"
            : "truncate text-sm font-medium"
        }
        title={value}
      >
        {value}
      </dd>
    </div>
  );
}

function BackupActions({ disabled }: { disabled: boolean }) {
  const fileInput = useRef<HTMLInputElement>(null);
  const create = useCreateBackup();
  const upload = useRestoreUploadedBackup();
  const [pendingUpload, setPendingUpload] = useState<File | null>(null);

  const runCreate = () => {
    void withMutationToast(() => create.mutateAsync(), {
      success: "Backup created.",
    });
  };

  const confirmUpload = async () => {
    if (!pendingUpload) return;
    const file = pendingUpload;
    const ok = await withMutationToast(() => upload.mutateAsync(file), {
      success: "Restored from uploaded backup. Restart the server to be safe.",
    });
    if (ok) {
      setPendingUpload(null);
      if (fileInput.current) fileInput.current.value = "";
    }
  };

  return (
    <div className="space-y-3">
      <div className="space-y-1">
        <h3 className="font-medium">Actions</h3>
        <p className="text-muted-foreground text-sm">
          Create an on-demand backup, or restore from a file you have on disk.
        </p>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          onClick={runCreate}
          disabled={disabled || create.isPending}
        >
          {create.isPending ? (
            <Loader2 className="size-4 animate-spin" />
          ) : (
            <HardDrive className="size-4" />
          )}
          {create.isPending ? "Creating…" : "Create backup now"}
        </Button>

        <Button
          type="button"
          variant="outline"
          onClick={() => fileInput.current?.click()}
          disabled={disabled || upload.isPending}
        >
          {upload.isPending ? (
            <Loader2 className="size-4 animate-spin" />
          ) : (
            <Upload className="size-4" />
          )}
          {upload.isPending ? "Restoring…" : "Restore from upload"}
        </Button>

        <input
          ref={fileInput}
          type="file"
          accept=".sql.gz,application/gzip"
          className="hidden"
          onChange={(e) => {
            const file = e.target.files?.[0];
            if (file) setPendingUpload(file);
          }}
        />
      </div>

      <ConfirmDialog
        open={!!pendingUpload}
        onOpenChange={(open) => {
          if (!open) {
            setPendingUpload(null);
            if (fileInput.current) fileInput.current.value = "";
          }
        }}
        icon={Upload}
        title="Restore from uploaded file?"
        description={
          pendingUpload
            ? `This will OVERWRITE the current database with the contents of ${pendingUpload.name}. The action runs in a single transaction and rolls back on error, but on success the existing rows are gone.`
            : ""
        }
        confirmLabel="Restore"
        pendingLabel="Restoring…"
        pending={upload.isPending}
        onConfirm={confirmUpload}
      />
    </div>
  );
}

const scheduleSchema = z.object({
  schedule: z.string(),
  retention_days: z
    .number({ error: "Retention is required" })
    .int()
    .min(1, "Must be at least 1 day")
    .max(365, "Must be 365 days or fewer"),
});

type ScheduleValues = z.infer<typeof scheduleSchema>;

function ScheduleForm({
  initialSchedule,
  initialRetentionDays,
}: {
  initialSchedule: string;
  initialRetentionDays: number;
}) {
  const save = useUpdateBackupSchedule();
  const form = useForm<ScheduleValues>({
    resolver: zodResolver(scheduleSchema),
    defaultValues: {
      schedule: initialSchedule === "" ? "off" : initialSchedule,
      retention_days: initialRetentionDays || 7,
    },
  });

  const onSubmit = async (values: ScheduleValues) => {
    await withMutationToast(
      () =>
        save.mutateAsync({
          schedule: values.schedule === "off" ? "" : values.schedule,
          retention_days: values.retention_days,
        }),
      { success: "Schedule saved." },
    );
  };

  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit(onSubmit)}
        className="space-y-4"
        noValidate
      >
        <div className="space-y-1">
          <h3 className="font-medium">Automatic schedule</h3>
          <p className="text-muted-foreground text-sm">
            Backups older than the retention window are pruned at the end of each scheduled run.
          </p>
        </div>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-[2fr_1fr]">
          <FormField
            control={form.control}
            name="schedule"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Cadence</FormLabel>
                <Select
                  value={field.value}
                  onValueChange={field.onChange}
                  disabled={save.isPending}
                >
                  <FormControl>
                    <SelectTrigger className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent>
                    {SCHEDULE_OPTIONS.map((opt) => (
                      <SelectItem key={opt.value} value={opt.value}>
                        {opt.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name="retention_days"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Retention (days)</FormLabel>
                <FormControl>
                  <Input
                    type="number"
                    inputMode="numeric"
                    min={1}
                    max={365}
                    value={field.value}
                    onChange={(e) => field.onChange(e.target.valueAsNumber)}
                    disabled={save.isPending}
                  />
                </FormControl>
                <FormDescription>1–365</FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </div>

        <Button type="submit" disabled={save.isPending}>
          {save.isPending && <Loader2 className="size-4 animate-spin" />}
          {save.isPending ? "Saving…" : "Save schedule"}
        </Button>
      </form>
    </Form>
  );
}

function BackupsTable({
  backups,
  restoreDisabled,
}: {
  backups: BackupRow[];
  restoreDisabled: boolean;
}) {
  return (
    <div className="space-y-3">
      <div className="space-y-1">
        <h3 className="font-medium">Stored backups</h3>
        <p className="text-muted-foreground text-sm">
          Newest first. Filenames carry the trigger (manual or scheduled) and a UTC timestamp.
        </p>
      </div>

      {backups.length === 0 ? (
        <EmptyState
          variant="card"
          icon={HardDrive}
          title="No backups yet"
          description="Take a manual snapshot above, or enable scheduled backups to start a regular cadence."
        />
      ) : (
        <div className="border-border overflow-hidden rounded-md border">
          <ul className="divide-border divide-y">
            {backups.map((b) => (
              <BackupRowItem
                key={b.filename}
                row={b}
                restoreDisabled={restoreDisabled}
              />
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}

function BackupRowItem({
  row,
  restoreDisabled,
}: {
  row: BackupRow;
  restoreDisabled: boolean;
}) {
  const restore = useRestoreExistingBackup();
  const del = useDeleteBackup();
  const [confirm, setConfirm] = useState<"restore" | "delete" | null>(null);

  const runRestore = async () => {
    const ok = await withMutationToast(() => restore.mutateAsync(row.filename), {
      success: `Restored from ${row.filename}. Restart the server to be safe.`,
    });
    if (ok) setConfirm(null);
  };

  const runDelete = async () => {
    const ok = await withMutationToast(() => del.mutateAsync(row.filename), {
      success: `Deleted ${row.filename}.`,
    });
    if (ok) setConfirm(null);
  };

  const busy = restore.isPending || del.isPending;

  return (
    <li className="flex flex-col gap-2 px-3 py-2 sm:flex-row sm:items-center sm:gap-4">
      <div className="min-w-0 flex-1 space-y-0.5">
        <div className="flex items-center gap-2">
          <span className="truncate font-mono text-xs">{row.filename}</span>
          <Badge variant={row.trigger === "manual" ? "secondary" : "outline"}>
            {row.trigger}
          </Badge>
        </div>
        <div className="text-muted-foreground flex items-center gap-3 text-xs">
          <span>{row.size_formatted}</span>
          <span>·</span>
          <RelativeTime iso={row.created_at} />
        </div>
      </div>

      <TooltipProvider delayDuration={200}>
        <div className="flex items-center gap-1">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                asChild
                aria-label="Download backup"
              >
                <a href={backupDownloadHref(row.filename)}>
                  <Download className="size-4" />
                </a>
              </Button>
            </TooltipTrigger>
            <TooltipContent>Download</TooltipContent>
          </Tooltip>

          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                onClick={() => setConfirm("restore")}
                disabled={busy || restoreDisabled}
                aria-label="Restore from this backup"
              >
                {restore.isPending ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <RotateCcw className="size-4" />
                )}
              </Button>
            </TooltipTrigger>
            <TooltipContent>Restore</TooltipContent>
          </Tooltip>

          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                onClick={() => setConfirm("delete")}
                disabled={busy}
                aria-label="Delete backup"
                className="text-destructive hover:text-destructive"
              >
                {del.isPending ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <Trash2 className="size-4" />
                )}
              </Button>
            </TooltipTrigger>
            <TooltipContent>Delete</TooltipContent>
          </Tooltip>
        </div>
      </TooltipProvider>

      <ConfirmDialog
        open={confirm === "restore"}
        onOpenChange={(open) => {
          if (!open) setConfirm(null);
        }}
        icon={RotateCcw}
        title="Restore this backup?"
        description={`This will OVERWRITE the current database with the contents of ${row.filename}. The action runs in a single transaction and rolls back on error, but on success the existing rows are gone.`}
        confirmLabel="Restore"
        pendingLabel="Restoring…"
        pending={restore.isPending}
        onConfirm={runRestore}
      />

      <ConfirmDialog
        open={confirm === "delete"}
        onOpenChange={(open) => {
          if (!open) setConfirm(null);
        }}
        icon={Trash2}
        title="Delete backup?"
        description="This file will be removed from disk. This cannot be undone."
        body={
          <p className="bg-muted/60 text-foreground rounded-md border px-2 py-1 font-mono text-xs">
            {row.filename}
          </p>
        }
        confirmLabel="Delete backup"
        pendingLabel="Deleting…"
        pending={del.isPending}
        onConfirm={runDelete}
      />
    </li>
  );
}


function BackupsSkeleton() {
  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        <Skeleton className="h-10" />
        <Skeleton className="h-10" />
        <Skeleton className="h-10" />
        <Skeleton className="h-10" />
      </div>
      <Skeleton className="h-9 w-40" />
      <Skeleton className="h-32" />
    </div>
  );
}

function RelativeTime({ iso }: { iso: string }) {
  const date = new Date(iso);
  const label = formatRelative(date);
  return (
    <time dateTime={iso} title={date.toLocaleString()}>
      {label}
    </time>
  );
}

function formatRelative(date: Date): string {
  const diffMs = date.getTime() - Date.now();
  const diffSec = Math.round(diffMs / 1000);
  const abs = Math.abs(diffSec);
  const fmt = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" });
  if (abs < 60) return fmt.format(diffSec, "second");
  if (abs < 3600) return fmt.format(Math.round(diffSec / 60), "minute");
  if (abs < 86_400) return fmt.format(Math.round(diffSec / 3600), "hour");
  if (abs < 2_592_000) return fmt.format(Math.round(diffSec / 86_400), "day");
  if (abs < 31_536_000)
    return fmt.format(Math.round(diffSec / 2_592_000), "month");
  return fmt.format(Math.round(diffSec / 31_536_000), "year");
}
