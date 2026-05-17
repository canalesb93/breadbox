import { useEffect, useState } from "react";
import { Link, useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { z } from "zod";
import { ArrowLeft, Clock, FilterX, Loader2, StickyNote } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
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
  DateRangeFilter,
  type DateRangeValue,
} from "@/components/date-range-filter";
import {
  AGENT_RUN_NOTE_MAX_LEN,
  useAgent,
  useAgentRun,
  useAgentRuns,
  useTranscript,
  useUpdateAgentRunNote,
  type AgentRun,
  type AgentRunsFilters,
} from "@/api/queries/agents";
import { withMutationToast } from "@/lib/mutation-toast";
import { Textarea } from "@/components/ui/textarea";
import { formatDuration, formatRelativeTime } from "@/lib/format";
import { RunStatusPill } from "@/features/agents/run-status-pill";
import { TranscriptViewer } from "@/features/agents/transcript-viewer";

export const agentRunsSearchSchema = z.object({
  run: z.string().optional(),
  page: z.number().int().positive().optional(),
  status: z
    .enum(["success", "error", "in_progress", "skipped", "timeout"])
    .optional(),
  trigger: z.enum(["cron", "manual", "webhook"]).optional(),
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
    start: search.start,
    end: search.end,
  };
  const hasActiveFilters = Boolean(
    search.status || search.trigger || search.start || search.end,
  );

  const setFilter = (patch: Partial<AgentRunsSearch>) => {
    navigate({
      to: ".",
      search: (prev: Record<string, unknown>) => ({
        ...prev,
        ...patch,
        page: undefined, // reset pagination on filter change
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

      <div className="mb-4 flex flex-wrap items-center gap-2">
        <Select
          value={search.status ?? ANY_VALUE}
          onValueChange={(v) =>
            setFilter({ status: v === ANY_VALUE ? undefined : (v as AgentRunsSearch["status"]) })
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
            setFilter({ trigger: v === ANY_VALUE ? undefined : (v as AgentRunsSearch["trigger"]) })
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

interface OperatorNoteEditorProps {
  shortId: string;
  storedNote: string;
  loading: boolean;
}

function OperatorNoteEditor({
  shortId,
  storedNote,
  loading,
}: OperatorNoteEditorProps) {
  const update = useUpdateAgentRunNote();
  const [draft, setDraft] = useState(storedNote);

  // Re-hydrate when the loaded note changes (e.g. opening a different run
  // in the same Sheet instance after navigating).
  useEffect(() => {
    setDraft(storedNote);
  }, [storedNote, shortId]);

  const dirty = draft !== storedNote;
  const tooLong = draft.length > AGENT_RUN_NOTE_MAX_LEN;
  const onSave = () => {
    if (tooLong) return;
    void withMutationToast(
      () => update.mutateAsync({ shortId, note: draft }),
      {
        success: storedNote === "" ? "Note added" : draft === "" ? "Note cleared" : "Note saved",
        error: "Failed to save note",
      },
    );
  };

  return (
    <div className="mb-4 rounded-md border p-3">
      <label
        htmlFor={`note-${shortId}`}
        className="text-muted-foreground mb-1.5 flex items-center gap-1.5 text-xs uppercase tracking-wider"
      >
        <StickyNote className="size-3.5" />
        Operator note
      </label>
      <Textarea
        id={`note-${shortId}`}
        rows={2}
        placeholder={
          loading ? "Loading…" : "Add context — why this fired, what you're investigating, follow-ups…"
        }
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        disabled={loading}
        aria-invalid={tooLong}
      />
      <div className="mt-1.5 flex items-center justify-between">
        <span
          className={`text-xs ${tooLong ? "text-destructive" : "text-muted-foreground"}`}
        >
          {draft.length} / {AGENT_RUN_NOTE_MAX_LEN}
        </span>
        <Button
          type="button"
          size="sm"
          variant={dirty ? "default" : "outline"}
          onClick={onSave}
          disabled={!dirty || tooLong || update.isPending}
        >
          {update.isPending ? (
            <Loader2 className="size-3.5 animate-spin" />
          ) : null}
          {draft === "" && storedNote !== "" ? "Clear note" : "Save note"}
        </Button>
      </div>
    </div>
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
  const transcript = useTranscript(shortId ?? undefined);
  const runDetail = useAgentRun(shortId ?? undefined);

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
          {shortId && (
            <OperatorNoteEditor
              shortId={shortId}
              storedNote={runDetail.data?.operator_note ?? ""}
              loading={runDetail.isLoading}
            />
          )}
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
