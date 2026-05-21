import { useMemo, useState } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import { type ColumnDef } from "@tanstack/react-table";
import {
  AlertCircle,
  Bot,
  CheckCircle2,
  Clock,
  History,
  KeyRound,
  MoreHorizontal,
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
import { Switch } from "@/components/ui/switch";
import { DataTable } from "@/components/data-table";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";
import { withMutationToast } from "@/lib/mutation-toast";
import { formatRelativeTime } from "@/lib/format";
import {
  useAgents,
  useAgentSubsystemStatus,
  useDeleteAgent,
  useRunAgentNow,
  useToggleAgent,
  PROMPT_PREFIX_MAX_LEN,
  type AgentDefinition,
} from "@/api/queries/agents";
import { openModal } from "@/lib/modals";
import { AgentsTabs } from "@/features/agents/agents-tabs";

export function AgentsPage() {
  const agentsQuery = useAgents();
  const statusQuery = useAgentSubsystemStatus();
  const deleteAgent = useDeleteAgent();
  const navigate = useNavigate();
  const [pendingDelete, setPendingDelete] = useState<AgentDefinition | null>(
    null,
  );

  const agents = agentsQuery.data ?? [];
  const status = statusQuery.data;
  const showOnboardingBanner = Boolean(status) && !status?.ready;

  const columns = useMemo<ColumnDef<AgentDefinition>[]>(
    () => buildColumns({ onDelete: (a) => setPendingDelete(a) }),
    [],
  );

  return (
    <>
      <PageHeader
        eyebrow="System"
        title="Agents"
        description="Recurring Claude Agent SDK runs that call breadbox MCP to enrich, categorize, and review your data."
        actions={
          <Button asChild>
            <Link to="/agents/new">
              <Plus className="size-4" />
              New agent
            </Link>
          </Button>
        }
      />

      <AgentsTabs value="agents" />

      {showOnboardingBanner && status && (
        <Alert>
          <Bot className="size-4" />
          <AlertTitle>Finish setting up the agent subsystem</AlertTitle>
          <AlertDescription className="space-y-2">
            <p className="text-sm">
              The seeded starter agents below won't fire until two pieces are in
              place. Status checked without any API call:
            </p>
            {!status.auth_configured && (
              <p className="text-muted-foreground text-xs">
                <strong>New here?</strong> Pick the subscription token option
                (free under your Claude plan credits, runs{" "}
                <code>claude setup-token</code> once on any machine). Switch to
                an Anthropic API key later if you need pay-as-you-go billing
                past the 2026-06-15 cutover.
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
                    <span className="text-muted-foreground">
                      — download <code className="bg-muted rounded px-1">
                        breadbox-agent-&lt;os&gt;-&lt;arch&gt;
                      </code>{" "}
                      from the latest{" "}
                      <a
                        href="https://github.com/canalesb93/breadbox/releases/latest"
                        target="_blank"
                        rel="noreferrer noopener"
                        className="text-primary underline-offset-4 hover:underline"
                      >
                        GitHub release
                      </a>{" "}
                      and place it on your PATH or at{" "}
                      <code className="bg-muted rounded px-1">
                        ~/.breadbox/agent-bin/breadbox-agent
                      </code>. The Docker image already includes it.
                    </span>
                  )}
                </span>
              </li>
            </ul>
          </AlertDescription>
        </Alert>
      )}

      {agentsQuery.isError ? (
        <PageError
          resource="agents"
          error={agentsQuery.error}
          onRetry={() => agentsQuery.refetch()}
          retrying={agentsQuery.isFetching}
        />
      ) : (
        <DataTable
          columns={columns}
          data={agents}
          isLoading={agentsQuery.isLoading}
          getRowId={(a) => a.id}
          onRowClick={(a) =>
            // No `viewTransition` — it blanks iOS Safari's back-swipe preview
            // on scrolled list→detail navs (see transactions.tsx / csswg#8333).
            navigate({
              to: "/agents/$slug/edit",
              params: { slug: a.slug },
            })
          }
          refinedHeader
          emptyState={
            <EmptyState
              icon={Bot}
              title="No agents yet"
              description="Create your first agent to schedule recurring Claude runs against your data. Each agent runs locally via the Claude Agent SDK and the breadbox MCP server."
              action={
                <Button asChild>
                  <Link to="/agents/new">
                    <Plus className="size-4" />
                    Create your first agent
                  </Link>
                </Button>
              }
            />
          }
        />
      )}

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

// buildColumns is a function (not a top-level constant) so the Delete
// callback can be threaded in without spreading state into every cell.
// Memoize at the caller site.
function buildColumns({
  onDelete,
}: {
  onDelete: (agent: AgentDefinition) => void;
}): ColumnDef<AgentDefinition>[] {
  return [
    {
      id: "name",
      header: "Agent",
      meta: {
        className:
          "w-[28%] min-w-[200px] max-sm:w-full max-sm:min-w-[140px]",
      },
      cell: ({ row }) => {
        const a = row.original;
        return (
          // `min-w-0` + `truncate` keep long agent names from blowing out
          // the column width on iPhone SE (which showed names truncated to
          // 3-5 chars like "M...", "Spe..." because peer badges were
          // stealing the space).
          <div className="flex min-w-0 items-center gap-2">
            <span className="truncate font-medium leading-tight">
              {a.name}
            </span>
            {a.trigger_on_sync_complete && (
              <span
                className="text-blue-600 dark:text-blue-400 shrink-0"
                title="Also fires after every successful bank sync"
              >
                <Zap className="size-3.5" />
              </span>
            )}
          </div>
        );
      },
    },
    {
      id: "schedule",
      header: "Schedule",
      meta: { className: "w-[18%]" },
      cell: ({ row }) => (
        <span className="text-muted-foreground text-sm">
          {cronToProseLabel(row.original.schedule_cron)}
        </span>
      ),
    },
    {
      id: "last_run",
      header: "Last run",
      meta: { className: "w-[18%]" },
      cell: ({ row }) => <LastRunCell run={row.original.last_run} />,
    },
    {
      id: "next_run",
      header: "Next",
      meta: { className: "w-[10%]" },
      cell: ({ row }) => {
        const at = row.original.next_fire_at;
        if (!at) return <span className="text-muted-foreground">—</span>;
        return (
          <span
            className="text-muted-foreground text-sm"
            title={new Date(at).toLocaleString()}
          >
            {formatRelativeTime(at)}
          </span>
        );
      },
    },
    {
      id: "cost",
      header: "30d cost",
      meta: { className: "w-[10%] text-right tabular-nums" },
      cell: ({ row }) => {
        const stats = row.original.cost_stats_30d;
        if (!stats || stats.run_count === 0) {
          return <span className="text-muted-foreground">—</span>;
        }
        return (
          <span
            className="text-sm"
            title={`${stats.run_count} run${stats.run_count === 1 ? "" : "s"} in the last 30 days`}
          >
            ${stats.total_cost_usd.toFixed(2)}
          </span>
        );
      },
    },
    {
      id: "status",
      header: "Enabled",
      meta: { className: "w-[80px]" },
      cell: ({ row }) => <EnabledSwitchCell agent={row.original} />,
    },
    {
      id: "actions",
      header: () => <span className="sr-only">Actions</span>,
      meta: { className: "w-[120px] text-right" },
      cell: ({ row }) => (
        <ActionsCell
          agent={row.original}
          onDelete={() => onDelete(row.original)}
        />
      ),
    },
  ];
}

// LastRunCell shows the most recent run's status icon plus relative
// time. If there are no runs yet we surface that directly so an
// operator can tell "never fired" apart from a stale row.
function LastRunCell({ run }: { run: AgentDefinition["last_run"] }) {
  if (!run) {
    return (
      <span className="text-muted-foreground text-sm opacity-70">No runs yet</span>
    );
  }
  const Icon =
    run.status === "success"
      ? CheckCircle2
      : run.status === "error"
        ? XCircle
        : run.status === "skipped"
          ? AlertCircle
          : Clock;
  const color =
    run.status === "success"
      ? "text-emerald-600"
      : run.status === "error"
        ? "text-red-600"
        : run.status === "skipped"
          ? "text-amber-600"
          : "text-muted-foreground";
  return (
    <span
      className="inline-flex items-center gap-1.5 text-sm"
      title={`${run.status} — ${new Date(run.started_at).toLocaleString()}`}
    >
      <Icon className={`size-3.5 ${color}`} />
      <span className="text-muted-foreground">
        {formatRelativeTime(run.started_at)}
      </span>
    </span>
  );
}

// stopRowClick wraps inline interactive controls so clicking them
// doesn't also bubble up to the row's "navigate to edit" handler.
function stopRowClick<E extends Element>(e: React.MouseEvent<E>) {
  e.stopPropagation();
}

function EnabledSwitchCell({ agent }: { agent: AgentDefinition }) {
  const toggle = useToggleAgent();
  const handleChange = (enable: boolean) => {
    void withMutationToast(
      () => toggle.mutateAsync({ slug: agent.slug, enable }),
      {
        success: enable ? "Enabled" : "Disabled",
        error: "Toggle failed",
      },
    );
  };
  return (
    <div onClick={stopRowClick} className="inline-flex">
      <Switch
        checked={agent.enabled}
        disabled={toggle.isPending}
        onCheckedChange={handleChange}
        aria-label={agent.enabled ? "Disable agent" : "Enable agent"}
      />
    </div>
  );
}

function ActionsCell({
  agent,
  onDelete,
}: {
  agent: AgentDefinition;
  onDelete: () => void;
}) {
  return (
    <div
      onClick={stopRowClick}
      className="flex items-center justify-end gap-1"
    >
      <RunNowDialog
        slug={agent.slug}
        name={agent.name}
        lastPrefix={agent.last_prompt_prefix ?? null}
      />
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size="icon" aria-label="More actions">
            <MoreHorizontal className="size-4" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-44">
          <DropdownMenuGroup>
            <DropdownMenuItem asChild>
              <Link to="/agents/$slug/edit" params={{ slug: agent.slug }}>
                <Pencil className="size-4" /> Edit agent
              </Link>
            </DropdownMenuItem>
            <DropdownMenuItem asChild>
              <Link to="/agents/$slug/runs" params={{ slug: agent.slug }}>
                <History className="size-4" /> View runs
              </Link>
            </DropdownMenuItem>
          </DropdownMenuGroup>
          <DropdownMenuSeparator />
          <DropdownMenuItem variant="destructive" onSelect={onDelete}>
            <Trash2 className="size-4" /> Delete
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
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
          "Run failed — check Settings → Agents for auth, or install the breadbox-agent binary (see onboarding banner)",
      },
    );
    if (ok) {
      setOpen(false);
      setPrefix("");
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
          Run
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
