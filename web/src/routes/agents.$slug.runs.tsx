import { useMemo } from "react";
import { useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { type ColumnDef } from "@tanstack/react-table";
import { z } from "zod";
import { Clock, FilterX, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { SoftBackButton } from "@/components/soft-back-button";
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
import {
  useAgent,
  useAgentRuns,
  type AgentRun,
  type AgentRunsFilters,
} from "@/api/queries/agents";
import { formatDuration, formatRelativeTime } from "@/lib/format";
import { RunStatusPill } from "@/features/agents/run-status-pill";
import { TranscriptSheet } from "@/features/agents/transcript-sheet";
import { RunFlagsCell } from "@/routes/agents.runs";

export const agentRunsSearchSchema = z.object({
  run: z.string().optional(),
  page: z.number().int().positive().optional(),
  status: z
    .enum(["success", "error", "in_progress", "skipped", "timeout"])
    .optional(),
  trigger: z.enum(["cron", "manual", "webhook"]).optional(),
  hit_cap: z.enum(["max_turns", "max_budget", "any"]).optional(),
  start: z.string().optional(),
  end: z.string().optional(),
});
type AgentRunsSearch = z.infer<typeof agentRunsSearchSchema>;

const PAGE_SIZE = 25;
const ANY_VALUE = "__any__";

export function AgentRunsPage() {
  const { slug } = useParams({ strict: false }) as { slug: string };
  const search = useSearch({ strict: false }) as AgentRunsSearch;
  const navigate = useNavigate();
  const page = search.page ?? 1;

  const filters: AgentRunsFilters = {
    status: search.status ?? "",
    trigger: search.trigger ?? "",
    hit_cap: search.hit_cap ?? "",
    start: search.start,
    end: search.end,
  };
  const hasActiveFilters = Boolean(
    search.status ||
      search.trigger ||
      search.hit_cap ||
      search.start ||
      search.end,
  );

  const setFilter = (patch: Partial<AgentRunsSearch>) => {
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
        status: undefined,
        trigger: undefined,
        hit_cap: undefined,
        start: undefined,
        end: undefined,
        page: undefined,
      }),
    });
  };

  const agentQuery = useAgent(slug);
  const runsQuery = useAgentRuns(slug, filters, PAGE_SIZE, (page - 1) * PAGE_SIZE);

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

  const columns = useMemo<ColumnDef<AgentRun>[]>(
    () => buildPerAgentRunsColumns(),
    [],
  );

  return (
    <>
      <SoftBackButton to="/agents" className="self-start">
        Back to agents
      </SoftBackButton>
      <PageHeader
        eyebrow="Agent runs"
        title={agentQuery.data ? `${agentQuery.data.name} — runs` : "Run history"}
        description="Every fire of this agent (cron, manual, or webhook). Click any row to view its transcript."
      />

      <div className="flex flex-wrap items-center gap-2">
        <Select
          value={search.status ?? ANY_VALUE}
          onValueChange={(v) =>
            setFilter({
              status:
                v === ANY_VALUE ? undefined : (v as AgentRunsSearch["status"]),
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
                v === ANY_VALUE
                  ? undefined
                  : (v as AgentRunsSearch["trigger"]),
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
                v === ANY_VALUE
                  ? undefined
                  : (v as AgentRunsSearch["hit_cap"]),
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
              title="No runs yet"
              description="Trigger a run from the agents list, or wait for the next scheduled fire."
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

function buildPerAgentRunsColumns(): ColumnDef<AgentRun>[] {
  return [
    {
      id: "status",
      header: "Status",
      meta: { className: "w-[110px]" },
      cell: ({ row }) => <RunStatusPill status={row.original.status} />,
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
      meta: { className: "w-[180px]" },
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
      meta: { className: "w-[100px] text-right tabular-nums" },
      cell: ({ row }) => (
        <span className="text-muted-foreground text-sm">
          {formatDuration(row.original.duration_ms)}
        </span>
      ),
    },
    {
      id: "cost",
      header: "Cost",
      meta: { className: "w-[100px] text-right tabular-nums" },
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
      meta: { className: "w-[80px] text-right tabular-nums" },
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
