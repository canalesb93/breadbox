import { useMemo } from "react";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { type ColumnDef } from "@tanstack/react-table";
import { z } from "zod";
import {
  Clock,
  FilterX,
  Loader2,
  Sparkles,
  StickyNote,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { PageHeader } from "@/components/page-header";
import { PageError } from "@/components/page-error";
import { EmptyState } from "@/components/empty-state";
import { DataTable } from "@/components/data-table";
import {
  DateRangeFilter,
  type DateRangeValue,
} from "@/components/date-range-filter";
import { formatDuration, formatRelativeTime } from "@/lib/format";
import { RunStatusPill } from "@/features/agents/run-status-pill";
import {
  HitCapPill,
  TranscriptSheet,
} from "@/features/agents/transcript-sheet";
import { AgentsTabs } from "@/features/agents/agents-tabs";
import {
  useAgents,
  useAllAgentRuns,
  type AgentRunWithAgent,
} from "@/api/queries/agents";

export const agentsRunsSearchSchema = z.object({
  run: z.string().optional(),
  page: z.number().int().positive().optional(),
  agent: z.string().optional(),
  status: z
    .enum(["success", "error", "in_progress", "skipped", "timeout"])
    .optional(),
  trigger: z.enum(["cron", "manual", "webhook"]).optional(),
  hit_cap: z.enum(["max_turns", "max_budget", "any"]).optional(),
  start: z.string().optional(),
  end: z.string().optional(),
});
type AgentsRunsSearch = z.infer<typeof agentsRunsSearchSchema>;

const PAGE_SIZE = 25;
const ANY_VALUE = "__any__";

export function AgentsRunsPage() {
  const search = useSearch({ strict: false }) as AgentsRunsSearch;
  const navigate = useNavigate();
  const page = search.page ?? 1;

  const filters = {
    agent: search.agent,
    status: search.status ?? "",
    trigger: search.trigger ?? "",
    hit_cap: search.hit_cap ?? "",
    start: search.start,
    end: search.end,
  };
  const hasActiveFilters = Boolean(
    search.agent ||
      search.status ||
      search.trigger ||
      search.hit_cap ||
      search.start ||
      search.end,
  );

  const agentsQuery = useAgents();
  const runsQuery = useAllAgentRuns(filters, PAGE_SIZE, (page - 1) * PAGE_SIZE);

  const setFilter = (patch: Partial<AgentsRunsSearch>) => {
    navigate({
      to: ".",
      search: (prev: Record<string, unknown>) => ({
        ...prev,
        ...patch,
        page: undefined,
      }),
    });
  };
  const clearFilters = () => {
    navigate({
      to: ".",
      search: (prev: Record<string, unknown>) => ({
        ...prev,
        agent: undefined,
        status: undefined,
        trigger: undefined,
        hit_cap: undefined,
        start: undefined,
        end: undefined,
        page: undefined,
      }),
    });
  };

  const openRunShortId = search.run ?? null;
  const openRun = (shortId: string) => {
    navigate({
      to: ".",
      search: (prev: Record<string, unknown>) => ({ ...prev, run: shortId }),
    });
  };
  const closeRun = () => {
    navigate({
      to: ".",
      search: (prev: Record<string, unknown>) => ({ ...prev, run: undefined }),
    });
  };
  const loadMore = () => {
    navigate({
      to: ".",
      search: (prev: Record<string, unknown>) => ({ ...prev, page: page + 1 }),
    });
  };

  const runs = runsQuery.data?.runs ?? [];
  const hasMore = runsQuery.data?.has_more ?? false;
  const agents = agentsQuery.data ?? [];

  const columns = useMemo<ColumnDef<AgentRunWithAgent>[]>(
    () => buildGlobalRunsColumns(),
    [],
  );

  return (
    <>
      <PageHeader
        eyebrow="System"
        title="Agent runs"
        description="Every fire of every agent in this household — cron, manual, or webhook. Click any row to view its transcript."
      />
      <AgentsTabs value="runs" />

      <div className="mb-4 flex flex-wrap items-center gap-2">
        <Select
          value={search.agent ?? ANY_VALUE}
          onValueChange={(v) =>
            setFilter({ agent: v === ANY_VALUE ? undefined : v })
          }
        >
          <SelectTrigger size="sm" className="w-52">
            <SelectValue placeholder="All agents" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ANY_VALUE}>All agents</SelectItem>
            {agents.map((a) => (
              <SelectItem key={a.id} value={a.slug}>
                {a.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select
          value={search.status ?? ANY_VALUE}
          onValueChange={(v) =>
            setFilter({
              status:
                v === ANY_VALUE ? undefined : (v as AgentsRunsSearch["status"]),
            })
          }
        >
          <SelectTrigger size="sm" className="w-40">
            <SelectValue placeholder="All statuses" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ANY_VALUE}>All statuses</SelectItem>
            <SelectItem value="success">Success</SelectItem>
            <SelectItem value="error">Error</SelectItem>
            <SelectItem value="in_progress">Running</SelectItem>
            <SelectItem value="skipped">Skipped</SelectItem>
            <SelectItem value="timeout">Timeout</SelectItem>
          </SelectContent>
        </Select>

        <Select
          value={search.trigger ?? ANY_VALUE}
          onValueChange={(v) =>
            setFilter({
              trigger:
                v === ANY_VALUE ? undefined : (v as AgentsRunsSearch["trigger"]),
            })
          }
        >
          <SelectTrigger size="sm" className="w-40">
            <SelectValue placeholder="All triggers" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ANY_VALUE}>All triggers</SelectItem>
            <SelectItem value="cron">Cron</SelectItem>
            <SelectItem value="manual">Manual</SelectItem>
            <SelectItem value="webhook">Webhook</SelectItem>
          </SelectContent>
        </Select>

        <Select
          value={search.hit_cap ?? ANY_VALUE}
          onValueChange={(v) =>
            setFilter({
              hit_cap:
                v === ANY_VALUE ? undefined : (v as AgentsRunsSearch["hit_cap"]),
            })
          }
        >
          <SelectTrigger size="sm" className="w-40">
            <SelectValue placeholder="All caps" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ANY_VALUE}>Any cap state</SelectItem>
            <SelectItem value="any">Hit any cap</SelectItem>
            <SelectItem value="max_turns">Hit max turns</SelectItem>
            <SelectItem value="max_budget">Over budget</SelectItem>
          </SelectContent>
        </Select>

        <DateRangeFilter
          value={{ start: search.start, end: search.end } as DateRangeValue}
          onChange={(v) => setFilter({ start: v.start, end: v.end })}
          label="Started"
        />

        {hasActiveFilters && (
          <Button variant="ghost" size="sm" onClick={clearFilters}>
            <FilterX className="size-3.5" />
            Clear filters
          </Button>
        )}
      </div>

      {runsQuery.isError ? (
        <PageError
          resource="runs"
          error={runsQuery.error}
          onRetry={() => runsQuery.refetch()}
          retrying={runsQuery.isFetching}
        />
      ) : (
        <DataTable
          columns={columns}
          data={runs}
          isLoading={runsQuery.isLoading}
          getRowId={(r) => r.id}
          onRowClick={(r) => openRun(r.short_id)}
          refinedHeader
          emptyState={
            <EmptyState
              icon={Clock}
              title={hasActiveFilters ? "No runs match these filters" : "No runs yet"}
              description={
                hasActiveFilters
                  ? "Try widening the date range or clearing some filters."
                  : "Trigger a run from the Agents tab, or wait for the next scheduled fire."
              }
              action={
                hasActiveFilters ? (
                  <Button variant="outline" size="sm" onClick={clearFilters}>
                    <FilterX className="size-3.5" />
                    Clear filters
                  </Button>
                ) : undefined
              }
            />
          }
        />
      )}

      {hasMore && (
        <div className="mt-3 flex justify-center">
          <Button
            variant="outline"
            size="sm"
            disabled={runsQuery.isFetching}
            onClick={loadMore}
          >
            {runsQuery.isFetching && (
              <Loader2 className="size-4 animate-spin" />
            )}
            Load more
          </Button>
        </div>
      )}

      <TranscriptSheet shortId={openRunShortId} onClose={closeRun} />
    </>
  );
}

function buildGlobalRunsColumns(): ColumnDef<AgentRunWithAgent>[] {
  return [
    {
      id: "status",
      header: "Status",
      meta: { className: "w-[110px]" },
      cell: ({ row }) => <RunStatusPill status={row.original.status} />,
    },
    {
      id: "agent",
      header: "Agent",
      meta: { className: "w-[22%] min-w-[160px]" },
      cell: ({ row }) => (
        <Link
          to="/agents/$slug/edit"
          params={{ slug: row.original.agent_slug }}
          className="hover:text-foreground truncate text-sm font-medium"
          onClick={(e) => e.stopPropagation()}
          title={`Edit ${row.original.agent_name}`}
        >
          {row.original.agent_name}
        </Link>
      ),
    },
    {
      id: "trigger",
      header: "Trigger",
      meta: { className: "w-[90px]" },
      cell: ({ row }) => (
        <span className="text-muted-foreground text-xs uppercase tracking-wide">
          {row.original.trigger}
        </span>
      ),
    },
    {
      id: "started",
      header: "Started",
      meta: { className: "w-[140px]" },
      cell: ({ row }) => (
        <span
          className="text-muted-foreground text-sm"
          title={new Date(row.original.started_at).toLocaleString()}
        >
          {formatRelativeTime(row.original.started_at)}
        </span>
      ),
    },
    {
      id: "duration",
      header: "Duration",
      meta: { className: "w-[80px] text-right tabular-nums" },
      cell: ({ row }) => (
        <span className="text-muted-foreground text-sm">
          {formatDuration(row.original.duration_ms)}
        </span>
      ),
    },
    {
      id: "cost",
      header: "Cost",
      meta: { className: "w-[90px] text-right tabular-nums" },
      cell: ({ row }) => (
        <span className="text-muted-foreground text-sm font-mono">
          {row.original.total_cost_usd != null
            ? `$${row.original.total_cost_usd.toFixed(4)}`
            : "—"}
        </span>
      ),
    },
    {
      id: "tools",
      header: "Tools",
      meta: { className: "w-[70px] text-right tabular-nums" },
      cell: ({ row }) => (
        <span className="text-muted-foreground text-sm">
          {row.original.num_tool_calls != null
            ? row.original.num_tool_calls
            : "—"}
        </span>
      ),
    },
    {
      id: "flags",
      header: () => <span className="sr-only">Flags</span>,
      meta: { className: "w-[110px] text-right" },
      cell: ({ row }) => <RunFlagsCell run={row.original} />,
    },
  ];
}

// RunFlagsCell collapses the per-row indicators (prefix, operator note,
// hit_cap) into a single right-aligned cluster of icons + cap pill.
// Title attributes carry the preview text so hover reveals context.
export function RunFlagsCell({
  run,
}: {
  run: { operator_note?: string | null; prompt_prefix?: string | null; hit_cap?: "max_turns" | "max_budget" | null };
}) {
  const hasNote = Boolean(run.operator_note && run.operator_note.trim() !== "");
  const notePreview = hasNote
    ? (run.operator_note ?? "").trim().slice(0, 120)
    : "";
  const hasPrefix = Boolean(
    run.prompt_prefix && run.prompt_prefix.trim() !== "",
  );
  const prefixPreview = hasPrefix
    ? (run.prompt_prefix ?? "").trim().slice(0, 120)
    : "";
  return (
    <span className="inline-flex items-center justify-end gap-1.5">
      {hasPrefix && (
        <span
          title={`Prompt prefix: ${prefixPreview}`}
          aria-label="Has prompt prefix"
        >
          <Sparkles className="text-muted-foreground size-3.5" />
        </span>
      )}
      {hasNote && (
        <span
          title={`Operator note: ${notePreview}`}
          aria-label="Has operator note"
        >
          <StickyNote className="text-muted-foreground size-3.5" />
        </span>
      )}
      <HitCapPill cap={run.hit_cap ?? null} />
    </span>
  );
}
