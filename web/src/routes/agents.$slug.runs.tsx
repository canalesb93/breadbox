import { Link, useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { z } from "zod";
import { ArrowLeft, Clock, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { PageHeader } from "@/components/page-header";
import { PageError } from "@/components/page-error";
import { EmptyState } from "@/components/empty-state";
import {
  useAgent,
  useAgentRuns,
  useTranscript,
  type AgentRun,
} from "@/api/queries/agents";
import { formatDuration, formatRelativeTime } from "@/lib/format";
import { RunStatusPill } from "@/features/agents/run-status-pill";
import { TranscriptViewer } from "@/features/agents/transcript-viewer";

export const agentRunsSearchSchema = z.object({
  run: z.string().optional(),
  page: z.number().int().positive().optional(),
});
type AgentRunsSearch = z.infer<typeof agentRunsSearchSchema>;

const PAGE_SIZE = 25;

export function AgentRunsPage() {
  const { slug } = useParams({ strict: false }) as { slug: string };
  const search = useSearch({ strict: false }) as AgentRunsSearch;
  const navigate = useNavigate();
  const page = search.page ?? 1;

  const agentQuery = useAgent(slug);
  const runsQuery = useAgentRuns(slug, PAGE_SIZE, (page - 1) * PAGE_SIZE);

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

  return (
    <>
      <Button asChild variant="ghost" size="sm" className="-ml-2 mb-2">
        <Link to="/agents">
          <ArrowLeft className="size-4" /> Back to agents
        </Link>
      </Button>
      <PageHeader
        eyebrow="Agent runs"
        title={agentQuery.data ? `${agentQuery.data.name} — runs` : "Run history"}
        description="Every fire of this agent (cron or manual). Click any row to view its transcript."
      />

      {runsQuery.isError ? (
        <PageError
          resource="runs"
          error={runsQuery.error}
          onRetry={() => runsQuery.refetch()}
          retrying={runsQuery.isFetching}
        />
      ) : runsQuery.isLoading ? (
        <div className="flex flex-col gap-2">
          {[0, 1, 2, 3].map((i) => (
            <Skeleton key={i} className="h-14 w-full rounded-md" />
          ))}
        </div>
      ) : runs.length === 0 ? (
        <EmptyState
          icon={Clock}
          title="No runs yet"
          description="Trigger a run from the agents list, or wait for the next scheduled fire."
        />
      ) : (
        <Card className="overflow-hidden p-0">
          <div className="divide-border divide-y">
            {runs.map((run) => (
              <RunRow key={run.id} run={run} onClick={() => openRun(run.short_id)} />
            ))}
          </div>
          {hasMore && (
            <div className="flex justify-center border-t p-3">
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
        </Card>
      )}

      <TranscriptSheet shortId={openRunShortId} onClose={closeRun} />
    </>
  );
}

function RunRow({ run, onClick }: { run: AgentRun; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="hover:bg-accent/40 flex w-full flex-wrap items-center gap-3 px-4 py-3 text-left text-sm"
    >
      <RunStatusPill status={run.status} />
      <span className="text-muted-foreground text-xs uppercase tracking-wide">
        {run.trigger}
      </span>
      <span className="flex-1 text-xs">
        {formatRelativeTime(run.started_at)}
      </span>
      <span className="text-muted-foreground text-xs">
        {formatDuration(run.duration_ms)}
      </span>
      <span className="text-muted-foreground text-xs font-mono">
        {run.total_cost_usd != null ? `$${run.total_cost_usd.toFixed(4)}` : "—"}
      </span>
      <span className="text-muted-foreground text-xs">
        {run.num_tool_calls != null ? `${run.num_tool_calls} tools` : "—"}
      </span>
    </button>
  );
}

interface TranscriptSheetProps {
  shortId: string | null;
  onClose: () => void;
}

function TranscriptSheet({ shortId, onClose }: TranscriptSheetProps) {
  const open = Boolean(shortId);
  const runQuery = useAgentRuns; // unused — kept here as a reminder that the
  // single-run detail comes via useTranscript; the run summary is in the
  // parent's runs list.
  void runQuery;
  const transcript = useTranscript(shortId ?? undefined);

  return (
    <Sheet open={open} onOpenChange={(o) => !o && onClose()}>
      <SheetContent className="flex w-full flex-col gap-0 p-0 sm:max-w-2xl">
        <SheetHeader className="border-b px-6 py-4">
          <SheetTitle>Transcript</SheetTitle>
          <SheetDescription>
            {shortId ? (
              <span className="font-mono text-xs">Run {shortId}</span>
            ) : null}
          </SheetDescription>
        </SheetHeader>
        <div className="flex-1 overflow-y-auto px-6 py-4">
          {transcript.isLoading ? (
            <div className="space-y-3">
              <Skeleton className="h-20 w-full" />
              <Skeleton className="h-32 w-full" />
              <Skeleton className="h-20 w-full" />
            </div>
          ) : transcript.isError ? (
            <PageError
              resource="transcript"
              error={transcript.error}
              onRetry={() => transcript.refetch()}
              retrying={transcript.isFetching}
            />
          ) : transcript.data && shortId ? (
            <TranscriptViewer
              events={transcript.data.events}
              rawLength={transcript.data.rawLength}
              truncated={transcript.data.truncated}
              shortId={shortId}
            />
          ) : null}
        </div>
      </SheetContent>
    </Sheet>
  );
}
