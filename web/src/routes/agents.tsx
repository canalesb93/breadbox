import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import {
  AlertCircle,
  Bot,
  CheckCircle2,
  Clock,
  Coins,
  History,
  KeyRound,
  Pencil,
  Play,
  Plus,
  Sparkles,
  Terminal,
  Trash2,
  XCircle,
  Zap,
} from "lucide-react";
import { cronToProseLabel } from "@/lib/cron-prose";
import { PageHeader } from "@/components/page-header";
import { EmptyState } from "@/components/empty-state";
import { PageError } from "@/components/page-error";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { Switch } from "@/components/ui/switch";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
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
import { Textarea } from "@/components/ui/textarea";
import { withMutationToast } from "@/lib/mutation-toast";
import { formatRelativeTime } from "@/lib/format";
import {
  useAgents,
  useAgentSubsystemStatus,
  useCreateAgent,
  useDeleteAgent,
  useRecentErroredAgentRuns,
  useRunAgentNow,
  useToggleAgent,
  PROMPT_PREFIX_MAX_LEN,
  type AgentCostStats,
  type AgentDefinition,
  type AgentRecentCapStats,
  type AgentRecentErrorStats,
  type RecentErroredAgentRun,
} from "@/api/queries/agents";
import { openModal } from "@/lib/modals";
import { useNavigate } from "@tanstack/react-router";

const createAgentSchema = z.object({
  name: z.string().min(1, "Name is required").max(120),
  slug: z
    .string()
    .min(2, "Slug must be at least 2 characters")
    .max(64)
    .regex(
      /^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$/,
      "Use lowercase letters, digits, dashes (kebab-case)",
    ),
  prompt: z.string().min(1, "Prompt is required"),
  schedule_cron: z.string().optional().or(z.literal("")),
});
type CreateAgentForm = z.infer<typeof createAgentSchema>;

export function AgentsPage() {
  const agentsQuery = useAgents();
  const statusQuery = useAgentSubsystemStatus();
  const createAgent = useCreateAgent();
  const deleteAgent = useDeleteAgent();
  const navigate = useNavigate();
  const [createOpen, setCreateOpen] = useState(false);
  const [pendingDelete, setPendingDelete] = useState<AgentDefinition | null>(
    null,
  );

  const agents = agentsQuery.data ?? [];
  const status = statusQuery.data;
  const showOnboardingBanner = Boolean(status) && !status?.ready;

  const form = useForm<CreateAgentForm>({
    resolver: zodResolver(createAgentSchema),
    defaultValues: { name: "", slug: "", prompt: "", schedule_cron: "" },
  });

  const onCreate = form.handleSubmit(async (values) => {
    const ok = await withMutationToast(
      () =>
        createAgent.mutateAsync({
          name: values.name,
          slug: values.slug,
          prompt: values.prompt,
          schedule_cron: values.schedule_cron || null,
        }),
      {
        success: `Created agent ${values.name}`,
        error: "Failed to create agent",
      },
    );
    if (ok) {
      setCreateOpen(false);
      form.reset();
    }
  });

  return (
    <>
      <PageHeader
        eyebrow="System"
        title="Agents"
        description="Recurring Claude Agent SDK runs that call breadbox MCP to enrich, categorize, and review your data."
        actions={
          <Button onClick={() => setCreateOpen(true)}>
            <Plus className="size-4" />
            New agent
          </Button>
        }
      />

      {showOnboardingBanner && status && (
        <Alert>
          <Bot className="size-4" />
          <AlertTitle>Finish setting up the agent subsystem</AlertTitle>
          <AlertDescription className="space-y-2">
            <p className="text-sm">
              The seeded starter agents below won't fire until two pieces
              are in place. Status checked without any API call:
            </p>
            {!status.auth_configured && (
              <p className="text-muted-foreground text-xs">
                <strong>New here?</strong> Pick the subscription token
                option (free under your Claude plan credits, runs{" "}
                <code>claude setup-token</code> once on any machine).
                Switch to an Anthropic API key later if you need
                pay-as-you-go billing past the 2026-06-15 cutover.
              </p>
            )}
            <ul className="text-sm">
              <li className="flex items-center gap-2">
                {status.auth_configured ? (
                  <CheckCircle2 className="size-4 text-emerald-600" />
                ) : (
                  <KeyRound className="text-muted-foreground size-4" />
                )}
                <span>
                  Anthropic credential{" "}
                  {status.auth_configured ? (
                    <span className="text-muted-foreground">— configured</span>
                  ) : (
                    <Button
                      variant="link"
                      size="sm"
                      className="h-auto p-0"
                      onClick={() =>
                        navigate({
                          to: ".",
                          search: openModal("settings", "agents"),
                        })
                      }
                    >
                      Open Settings → Agents
                    </Button>
                  )}
                </span>
              </li>
              <li className="flex items-center gap-2">
                {status.binary_present ? (
                  <CheckCircle2 className="size-4 text-emerald-600" />
                ) : (
                  <Terminal className="text-muted-foreground size-4" />
                )}
                <span>
                  breadbox-agent binary{" "}
                  {status.binary_present ? (
                    <span className="text-muted-foreground">
                      — {status.binary_path}
                    </span>
                  ) : (
                    <code className="bg-muted rounded px-1">
                      make agent-sidecar
                    </code>
                  )}
                </span>
              </li>
            </ul>
          </AlertDescription>
        </Alert>
      )}

      <RecentErrorsBanner />

      {agentsQuery.isError ? (
        <PageError
          resource="agents"
          error={agentsQuery.error}
          onRetry={() => agentsQuery.refetch()}
          retrying={agentsQuery.isFetching}
        />
      ) : agentsQuery.isLoading ? (
        <div className="flex flex-col gap-3">
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className="h-24 w-full rounded-xl" />
          ))}
        </div>
      ) : agents.length === 0 ? (
        <EmptyState
          icon={Bot}
          title="No agents yet"
          description="Create your first agent to schedule recurring Claude runs against your data. Each agent runs locally via the Claude Agent SDK and the breadbox MCP server."
          action={
            <Button onClick={() => setCreateOpen(true)}>
              <Plus className="size-4" />
              Create your first agent
            </Button>
          }
        />
      ) : (
        <div className="flex flex-col gap-3">
          {agents.map((agent) => (
            <AgentRow
              key={agent.id}
              agent={agent}
              onDelete={() => setPendingDelete(agent)}
            />
          ))}
        </div>
      )}

      <Sheet open={createOpen} onOpenChange={setCreateOpen}>
        <SheetContent className="sm:max-w-lg">
          <SheetHeader>
            <SheetTitle>New agent</SheetTitle>
            <SheetDescription>
              Quick-create with prompt + schedule. The full prompt builder
              ships in the next iteration; for now you can edit advanced
              fields via the REST API.
            </SheetDescription>
          </SheetHeader>
          <Form {...form}>
            <form
              onSubmit={onCreate}
              className="mt-4 flex flex-1 flex-col gap-4"
            >
              <FormField
                control={form.control}
                name="name"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Name</FormLabel>
                    <FormControl>
                      <Input
                        placeholder="Weekly transaction review"
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="slug"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Slug</FormLabel>
                    <FormControl>
                      <Input
                        placeholder="weekly-transaction-review"
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      Kebab-case identifier used in URLs and API calls.
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="prompt"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Prompt</FormLabel>
                    <FormControl>
                      <Textarea
                        rows={6}
                        placeholder="Review last week's uncategorized transactions and apply the closest matching category…"
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="schedule_cron"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Schedule (optional)</FormLabel>
                    <FormControl>
                      <Input placeholder="0 9 * * 1 — Mondays at 9 AM" {...field} />
                    </FormControl>
                    <FormDescription>
                      Standard 5-field cron expression. Leave blank for manual
                      triggers only.
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <SheetFooter className="mt-auto flex-row justify-end gap-2 pt-4">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setCreateOpen(false)}
                >
                  Cancel
                </Button>
                <Button type="submit" disabled={createAgent.isPending}>
                  {createAgent.isPending ? "Creating…" : "Create agent"}
                </Button>
              </SheetFooter>
            </form>
          </Form>
        </SheetContent>
      </Sheet>

      <ConfirmDialog
        open={Boolean(pendingDelete)}
        onOpenChange={(open) => !open && setPendingDelete(null)}
        tone="destructive"
        title={`Delete agent ${pendingDelete?.name}?`}
        description="This removes the agent definition. Historical runs are preserved for audit."
        confirmLabel="Delete agent"
        pending={deleteAgent.isPending}
        onConfirm={() => {
          if (!pendingDelete) return;
          const slug = pendingDelete.slug;
          const name = pendingDelete.name;
          void withMutationToast(() => deleteAgent.mutateAsync(slug), {
            success: `Deleted ${name}`,
            error: "Delete failed",
          }).then((ok) => {
            if (ok) setPendingDelete(null);
          });
        }}
      />
    </>
  );
}

interface AgentRowProps {
  agent: AgentDefinition;
  onDelete: () => void;
}

function AgentRow({ agent, onDelete }: AgentRowProps) {
  const toggle = useToggleAgent();

  const handleToggle = (enable: boolean) => {
    void withMutationToast(
      () => toggle.mutateAsync({ slug: agent.slug, enable }),
      {
        success: enable ? "Enabled" : "Disabled",
        error: "Toggle failed",
      },
    );
  };

  return (
    <Card className="p-4">
      <div className="flex flex-wrap items-start gap-4">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <h3 className="text-base font-semibold leading-tight truncate">
              {agent.name}
            </h3>
            <Badge variant="outline" className="font-mono text-[10px]">
              {agent.slug}
            </Badge>
            <Badge
              variant={agent.tool_scope === "read_write" ? "default" : "secondary"}
              className="text-[10px] uppercase tracking-wide"
            >
              {agent.tool_scope.replace("_", " ")}
            </Badge>
          </div>
          <p className="text-muted-foreground mt-1 line-clamp-2 text-sm">
            {agent.prompt}
          </p>
          <div className="text-muted-foreground mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs">
            <span
              className="inline-flex items-center gap-1"
              title={
                agent.schedule_cron
                  ? `Cron expression: ${agent.schedule_cron}`
                  : undefined
              }
            >
              <Clock className="size-3" />
              {cronToProseLabel(agent.schedule_cron)}
            </span>
            {agent.trigger_on_sync_complete && (
              <span
                className="inline-flex items-center gap-1 rounded-full bg-blue-100 px-2 py-0.5 font-medium text-blue-700 dark:bg-blue-950/40 dark:text-blue-300"
                title="Also fires after every successful bank sync (trigger=webhook)"
              >
                <Zap className="size-3" />
                After sync
              </span>
            )}
            <NextFirePill at={agent.next_fire_at} />
            <QuietHoursActivePill
              start={agent.quiet_hours_start}
              end={agent.quiet_hours_end}
              enabled={agent.enabled}
            />
            <span>Model: {agent.model}</span>
            <span>Max turns: {agent.max_turns}</span>
            <CostStatsPill stats={agent.cost_stats_30d} />
            <RecentErrorPill stats={agent.recent_error_stats} />
            <RecentCapPill stats={agent.recent_cap_stats} />
            <LastRunPill run={agent.last_run} />
          </div>
        </div>

        <div className="flex shrink-0 items-center gap-2">
          <div className="flex items-center gap-2 rounded-md border bg-background px-3 py-1.5">
            <Switch
              checked={agent.enabled}
              disabled={toggle.isPending}
              onCheckedChange={handleToggle}
            />
            <span className="text-xs font-medium">
              {agent.enabled ? "Enabled" : "Disabled"}
            </span>
          </div>
          <RunNowDialog
            slug={agent.slug}
            name={agent.name}
            lastPrefix={agent.last_prompt_prefix ?? null}
          />
          <Button asChild variant="ghost" size="icon" aria-label="Run history">
            <Link to="/agents/$slug/runs" params={{ slug: agent.slug }}>
              <History className="size-4" />
            </Link>
          </Button>
          <Button asChild variant="ghost" size="icon" aria-label="Edit agent">
            <Link to="/agents/$slug/edit" params={{ slug: agent.slug }}>
              <Pencil className="size-4" />
            </Link>
          </Button>
          <Button
            variant="ghost"
            size="icon"
            aria-label="Delete agent"
            onClick={onDelete}
          >
            <Trash2 className="size-4" />
          </Button>
        </div>
      </div>
    </Card>
  );
}

// RecentErrorsBanner surfaces errored runs from the last 24h at the top
// of /v2/agents, with one row per recent error and a click-through to the
// matching run's transcript drawer. Hidden when there's nothing to show.
// 60s auto-refetch (hook-side) so a fresh failure surfaces without a
// manual reload. Catches operators who only open the SPA every few days.
function RecentErrorsBanner() {
  const navigate = useNavigate();
  const recent = useRecentErroredAgentRuns(24, 5);
  const rows = recent.data ?? [];
  if (rows.length === 0) return null;

  const openRun = (row: RecentErroredAgentRun) => {
    navigate({
      to: "/agents/$slug/runs",
      params: { slug: row.agent_slug },
      search: { run: row.run_short_id },
    });
  };

  return (
    <Alert variant="destructive">
      <AlertCircle className="size-4" />
      <AlertTitle>
        {rows.length === 1
          ? "1 errored agent run in the last 24h"
          : `${rows.length} errored agent runs in the last 24h`}
      </AlertTitle>
      <AlertDescription className="space-y-1">
        <ul className="space-y-1">
          {rows.map((r) => (
            <li
              key={r.run_short_id}
              className="flex flex-wrap items-baseline gap-2 text-sm"
            >
              <button
                type="button"
                className="font-medium underline-offset-2 hover:underline"
                onClick={() => openRun(r)}
              >
                {r.agent_name}
              </button>
              <span className="text-muted-foreground text-xs">
                {formatRelativeTime(r.started_at)}
              </span>
              {r.error_message && (
                <span
                  className="text-muted-foreground truncate text-xs"
                  title={r.error_message}
                >
                  — {r.error_message}
                </span>
              )}
            </li>
          ))}
        </ul>
      </AlertDescription>
    </Alert>
  );
}

function RunNowDialog({
  slug,
  name,
  lastPrefix,
}: {
  slug: string;
  name: string;
  lastPrefix: string | null;
}) {
  const [open, setOpen] = useState(false);
  const [prefix, setPrefix] = useState("");
  const runNow = useRunAgentNow();
  const navigate = useNavigate();
  const hasLastPrefix = Boolean(lastPrefix && lastPrefix.length > 0);

  const submit = async () => {
    let startedShortId: string | null = null;
    const ok = await withMutationToast(
      async () => {
        const result = await runNow.mutateAsync({ slug, promptPrefix: prefix });
        startedShortId = result.short_id;
        return result;
      },
      {
        success: prefix
          ? `Run started for ${name} (with prefix) — opening transcript`
          : `Run started for ${name} — opening transcript`,
        error:
          "Run failed — check Settings → Agents for auth, or `make agent-sidecar` for the binary",
      },
    );
    if (ok) {
      setOpen(false);
      setPrefix("");
      // Land on the runs page with the freshly-kicked-off run pre-selected.
      // The transcript drawer + 2s polling pick up events as they stream in.
      if (startedShortId) {
        navigate({
          to: "/agents/$slug/runs",
          params: { slug },
          search: { run: startedShortId },
        });
      }
    }
  };

  const tooLong = prefix.length > PROMPT_PREFIX_MAX_LEN;

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!runNow.isPending) setOpen(next);
        if (!next) setPrefix("");
      }}
    >
      <DialogTrigger asChild>
        <Button variant="secondary" size="sm" disabled={runNow.isPending}>
          <Play className="size-3.5" />
          Run now
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Run {name} now</DialogTitle>
          <DialogDescription>
            Fires the agent synchronously. Optionally prepend a one-off note —
            useful for scoped runs like "focus on Amazon Prime transactions
            only".
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-2">
          <label className="text-sm font-medium" htmlFor="run-now-prefix">
            Prompt prefix{" "}
            <span className="text-muted-foreground font-normal">(optional)</span>
          </label>
          <Textarea
            id="run-now-prefix"
            placeholder="Leave blank to use the agent's stored prompt as-is."
            value={prefix}
            onChange={(e) => setPrefix(e.target.value)}
            rows={4}
            disabled={runNow.isPending}
          />
          <div className="flex items-center justify-between gap-2">
            <p
              className={`text-xs ${
                tooLong ? "text-destructive" : "text-muted-foreground"
              }`}
            >
              {prefix.length} / {PROMPT_PREFIX_MAX_LEN} characters
            </p>
            {hasLastPrefix && (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-7 text-xs"
                onClick={() => setPrefix(lastPrefix ?? "")}
                disabled={runNow.isPending || prefix === lastPrefix}
                title={lastPrefix ?? ""}
              >
                <Sparkles className="size-3" />
                Use last prefix
              </Button>
            )}
          </div>
        </div>
        <DialogFooter>
          <Button
            variant="ghost"
            onClick={() => setOpen(false)}
            disabled={runNow.isPending}
          >
            Cancel
          </Button>
          <Button onClick={submit} disabled={runNow.isPending || tooLong}>
            <Play className="size-3.5" />
            Run now
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// RECENT_ERROR_WARN_THRESHOLD is the number of errors in the last 5 runs
// that triggers the warning pill. Bump if it turns out noisy.
const RECENT_ERROR_WARN_THRESHOLD = 3;

// RECENT_CAP_WARN_THRESHOLD is the number of cap-exhausted runs in the
// last 5 that triggers the amber "caps hit" pill. Lower than the error
// threshold because cap-hits indicate the agent's plan exceeds what was
// budgeted for it — even occasional hits are worth noticing.
const RECENT_CAP_WARN_THRESHOLD = 2;

function RecentCapPill({
  stats,
}: {
  stats?: AgentRecentCapStats | null;
}) {
  if (!stats || stats.cap_count < RECENT_CAP_WARN_THRESHOLD) {
    return null;
  }
  return (
    <span
      className="inline-flex items-center gap-1 rounded-full bg-amber-100 px-2 py-0.5 font-medium text-amber-800 dark:bg-amber-950/40 dark:text-amber-300"
      title={`${stats.cap_count} of last ${stats.run_count} runs hit max_turns or max_budget — consider raising the caps or splitting the prompt`}
    >
      <AlertCircle className="size-3" />
      {stats.cap_count}/{stats.run_count} hit cap
    </span>
  );
}

function RecentErrorPill({
  stats,
}: {
  stats?: AgentRecentErrorStats | null;
}) {
  if (!stats || stats.error_count < RECENT_ERROR_WARN_THRESHOLD) {
    return null;
  }
  return (
    <span
      className="inline-flex items-center gap-1 rounded-full bg-red-100 px-2 py-0.5 font-medium text-red-700 dark:bg-red-950/40 dark:text-red-300"
      title={`${stats.error_count} of last ${stats.run_count} runs errored — check the run history`}
    >
      <AlertCircle className="size-3" />
      {stats.error_count}/{stats.run_count} errored
    </span>
  );
}

// QuietHoursActivePill renders an amber "Quiet now" indicator when the
// agent has quiet hours set AND the user's current local time is inside
// the window. Distinct from the iter-20 NextFirePill (which always shows
// "next will fire at X" honoring quiet hours) — this is the real-time
// "this agent is silent RIGHT NOW because of its quiet window" signal,
// which explains why an enabled agent isn't firing even though a cron
// slot just passed. Hidden when the agent is disabled or quiet hours
// aren't configured.
function QuietHoursActivePill({
  start,
  end,
  enabled,
}: {
  start?: string | null;
  end?: string | null;
  enabled: boolean;
}) {
  if (!enabled || !start || !end || start === end) return null;
  if (!isWithinQuietWindow(new Date(), start, end)) return null;
  return (
    <span
      className="inline-flex items-center gap-1 rounded-full bg-amber-100 px-2 py-0.5 font-medium text-amber-800 dark:bg-amber-950/40 dark:text-amber-300"
      title={`Quiet hours ${start}–${end} are active right now; cron fires in this window are skipped`}
    >
      <Clock className="size-3" />
      Quiet now
    </span>
  );
}

// isWithinQuietWindow mirrors internal/service/IsWithinQuietHours on the
// SPA side so we don't need a server round-trip for an at-a-glance check.
// HH:MM 24-hour. Wrap-midnight windows (e.g. 22:00 → 07:00) handled.
// Empty/equal bounds → never quiet (defensive — the caller already
// short-circuits on those, but pin the contract here too).
function isWithinQuietWindow(
  now: Date,
  startHHMM: string,
  endHHMM: string,
): boolean {
  const start = parseHHMM(startHHMM);
  const end = parseHHMM(endHHMM);
  if (start === null || end === null || start === end) return false;
  const cur = now.getHours() * 60 + now.getMinutes();
  if (start < end) {
    // Same-day window: [start, end)
    return cur >= start && cur < end;
  }
  // Wrap-midnight window: [start, 24:00) ∪ [00:00, end)
  return cur >= start || cur < end;
}

function parseHHMM(hhmm: string): number | null {
  const m = /^([01]\d|2[0-3]):([0-5]\d)$/.exec(hhmm);
  if (!m) return null;
  return Number(m[1]) * 60 + Number(m[2]);
}

function NextFirePill({ at }: { at?: string | null }) {
  if (!at) return null;
  return (
    <span
      className="inline-flex items-center gap-1"
      title={`Next scheduled fire: ${new Date(at).toLocaleString()}`}
    >
      <Clock className="size-3" />
      Next: {formatRelativeTime(at)}
    </span>
  );
}

function CostStatsPill({
  stats,
}: {
  stats?: AgentCostStats | null;
}) {
  if (!stats || stats.run_count === 0) {
    return null;
  }
  return (
    <span
      className="inline-flex items-center gap-1"
      title={`${stats.run_count} run${stats.run_count === 1 ? "" : "s"} in the last 30 days`}
    >
      <Coins className="size-3" />${stats.total_cost_usd.toFixed(2)} / 30d
    </span>
  );
}

function LastRunPill({
  run,
}: {
  run: AgentDefinition["last_run"];
}) {
  if (!run) {
    return <span className="opacity-70">No runs yet</span>;
  }
  const Icon =
    run.status === "success"
      ? CheckCircle2
      : run.status === "error"
        ? XCircle
        : run.status === "skipped"
          ? AlertCircle
          : Clock;
  return (
    <span className="inline-flex items-center gap-1">
      <Icon className="size-3" />
      {run.status} · {formatRelativeTime(run.started_at)}
    </span>
  );
}
