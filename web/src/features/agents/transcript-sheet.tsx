import { useEffect, useState } from "react";
import {
  AlertTriangle,
  Loader2,
  Sparkles,
  StickyNote,
  XCircle,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Textarea } from "@/components/ui/textarea";
import { PageError } from "@/components/page-error";
import { TranscriptViewer } from "@/features/agents/transcript-viewer";
import { withMutationToast } from "@/lib/mutation-toast";
import {
  AGENT_RUN_NOTE_MAX_LEN,
  useAgentRun,
  useTranscript,
  useUpdateAgentRunNote,
} from "@/api/queries/agents";

export interface TranscriptSheetProps {
  shortId: string | null;
  onClose: () => void;
}

// TranscriptSheet renders one run's full audit trail: prompt prefix (if
// any), operator note editor, then the streaming transcript itself.
// Shared between /agents/$slug/runs (per-agent) and /agents/runs (global)
// so the deep-link behavior (?run=<shortId> opens the drawer) is
// identical in both contexts.
export function TranscriptSheet({ shortId, onClose }: TranscriptSheetProps) {
  const open = Boolean(shortId);
  const runDetail = useAgentRun(shortId ?? undefined);
  const inProgress = runDetail.data?.status === "in_progress";
  const transcript = useTranscript(shortId ?? undefined, { inProgress });

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
        <div className="flex-1 overflow-y-auto overscroll-contain [-webkit-overflow-scrolling:touch] px-6 py-4">
          {runDetail.data?.status === "error" &&
            runDetail.data?.error_message && (
              <RunErrorBlock message={runDetail.data.error_message} />
            )}
          {runDetail.data?.prompt_prefix && (
            <PromptPrefixBlock prefix={runDetail.data.prompt_prefix} />
          )}
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
          ) : transcript.isError && inProgress ? (
            <InProgressTranscriptPlaceholder />
          ) : transcript.isError ? (
            <PageError
              resource="transcript"
              error={transcript.error}
              onRetry={() => transcript.refetch()}
              retrying={transcript.isFetching}
            />
          ) : transcript.data && shortId ? (
            <>
              {inProgress && <InProgressBanner />}
              <TranscriptViewer
                events={transcript.data.events}
                rawLength={transcript.data.rawLength}
                truncated={transcript.data.truncated}
                shortId={shortId}
              />
            </>
          ) : null}
        </div>
      </SheetContent>
    </Sheet>
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
        success:
          storedNote === ""
            ? "Note added"
            : draft === ""
              ? "Note cleared"
              : "Note saved",
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
          loading
            ? "Loading…"
            : "Add context — why this fired, what you're investigating, follow-ups…"
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
          {update.isPending ? <Loader2 className="size-3.5 animate-spin" /> : null}
          {draft === "" && storedNote !== "" ? "Clear note" : "Save note"}
        </Button>
      </div>
    </div>
  );
}

// RunErrorBlock surfaces the orchestrator's error_message at the top of
// the transcript sheet for failed runs. Without this, a run shows up with
// status=error but no human-readable reason — the operator has to grep
// container logs to find out what happened (e.g. a sidecar crash before
// any transcript events were emitted).
function RunErrorBlock({ message }: { message: string }) {
  return (
    <div className="mb-4 rounded-md border border-destructive/40 bg-destructive/5 p-3">
      <div className="text-destructive mb-1 inline-flex items-center gap-1 text-xs font-medium uppercase tracking-wide">
        <XCircle className="size-3.5" />
        Run error
      </div>
      <pre className="text-foreground/90 max-h-64 overflow-auto overscroll-contain whitespace-pre-wrap break-words font-mono text-xs leading-relaxed">
        {message}
      </pre>
    </div>
  );
}

function PromptPrefixBlock({ prefix }: { prefix: string }) {
  return (
    <div className="mb-4 rounded-md border border-dashed bg-muted/40 p-3">
      <div className="text-muted-foreground mb-1 inline-flex items-center gap-1 text-xs font-medium uppercase tracking-wide">
        <Sparkles className="size-3.5" />
        Prompt prefix (this run only)
      </div>
      <p className="whitespace-pre-wrap text-sm leading-relaxed">{prefix}</p>
    </div>
  );
}

// HitCapPill flags runs that bumped into a safety ceiling. max_turns is
// amber (clean termination but probably incomplete work — operator may
// want to raise the cap or split the prompt); max_budget is red (mid-run
// abort — the agent's plan exceeded what was budgeted).
export function HitCapPill({
  cap,
}: {
  cap: "max_turns" | "max_budget" | null;
}) {
  if (!cap) return null;
  if (cap === "max_turns") {
    return (
      <span
        className="inline-flex items-center gap-1 rounded-full bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-800 dark:bg-amber-950/40 dark:text-amber-300"
        title="Run hit max_turns — work may be incomplete. Consider raising max_turns or splitting the prompt."
      >
        <AlertTriangle className="size-3" />
        max turns
      </span>
    );
  }
  return (
    <span
      className="inline-flex items-center gap-1 rounded-full bg-red-100 px-2 py-0.5 text-xs font-medium text-red-700 dark:bg-red-950/40 dark:text-red-300"
      title="Run exceeded max_budget_usd — terminated mid-task. Consider raising the budget cap or narrowing the agent's scope."
    >
      <AlertTriangle className="size-3" />
      over budget
    </span>
  );
}

function InProgressTranscriptPlaceholder() {
  return (
    <div className="text-muted-foreground flex flex-col items-center gap-3 py-12 text-center text-sm">
      <Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
      <div className="space-y-1">
        <div className="text-foreground font-medium">Run starting…</div>
        <p className="max-w-xs">
          Transcript will appear here as the agent begins streaming events.
        </p>
      </div>
    </div>
  );
}

function InProgressBanner() {
  return (
    <div className="bg-muted/40 text-muted-foreground mb-4 flex items-center gap-2 rounded-md border px-3 py-2 text-xs">
      <Loader2 className="h-3.5 w-3.5 animate-spin" />
      <span>Run in progress — events will keep arriving below.</span>
    </div>
  );
}
