import { useMemo, useState } from "react";
import {
  AlertCircle,
  AlertTriangle,
  ChevronDown,
  ChevronRight,
  Coins,
  Cpu,
  Download,
  Search,
  Wrench,
  X,
} from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { formatDuration } from "@/lib/format";
import type {
  AssistantContent,
  AssistantMessageData,
  ResultData,
  ToolResultData,
  ToolUseData,
  TranscriptEvent,
} from "@/api/queries/agents";

interface TranscriptViewerProps {
  events: TranscriptEvent[];
  rawLength: number;
  truncated: boolean;
  shortId: string;
}

// Grouped representation of one assistant turn: the assistant's message
// plus any tool_use blocks and matching tool_result blocks before the next
// assistant_message.
interface TurnGroup {
  assistant: (AssistantMessageData & { ts: number }) | null;
  toolUses: Array<ToolUseData & { ts: number }>;
  toolResults: Array<ToolResultData & { ts: number }>;
}

function groupIntoTurns(events: TranscriptEvent[]): TurnGroup[] {
  const turns: TurnGroup[] = [];
  let current: TurnGroup = { assistant: null, toolUses: [], toolResults: [] };
  for (const ev of events) {
    if (ev.type === "assistant_message") {
      if (current.assistant !== null || current.toolUses.length > 0) {
        turns.push(current);
        current = { assistant: null, toolUses: [], toolResults: [] };
      }
      current.assistant = { ...ev.data, ts: ev.ts };
    } else if (ev.type === "tool_use") {
      current.toolUses.push({ ...ev.data, ts: ev.ts });
    } else if (ev.type === "tool_result") {
      current.toolResults.push({ ...ev.data, ts: ev.ts });
    }
  }
  if (current.assistant !== null || current.toolUses.length > 0) {
    turns.push(current);
  }
  return turns;
}

// eventMatchesQuery does a case-insensitive substring search across the
// event's user-visible content: assistant text blocks, tool names, and
// tool-use input / tool-result content (JSON-stringified for tools so the
// search hits argument values like transaction IDs and category slugs).
function eventMatchesQuery(ev: TranscriptEvent, q: string): boolean {
  if (!q) return true;
  const needle = q.toLowerCase();
  switch (ev.type) {
    case "assistant_message": {
      const blocks = ev.data.message?.content ?? [];
      return blocks.some(
        (b) =>
          b.type === "text" && b.text.toLowerCase().includes(needle),
      );
    }
    case "tool_use":
      if (ev.data.name?.toLowerCase().includes(needle)) return true;
      try {
        return JSON.stringify(ev.data.input).toLowerCase().includes(needle);
      } catch {
        return false;
      }
    case "tool_result":
      try {
        return JSON.stringify(ev.data.content).toLowerCase().includes(needle);
      } catch {
        return false;
      }
    case "error":
      return ev.data.message?.toLowerCase().includes(needle) ?? false;
    case "cost_cap_hit":
      return ev.data.message?.toLowerCase().includes(needle) ?? false;
  }
  return false;
}

export function TranscriptViewer({
  events,
  rawLength,
  truncated,
  shortId,
}: TranscriptViewerProps) {
  const [query, setQuery] = useState("");
  const [toolsOnly, setToolsOnly] = useState(false);
  const [errorsOnly, setErrorsOnly] = useState(false);
  const trimmed = query.trim();
  const searching = trimmed.length > 0;

  const matchingEvents = useMemo(() => {
    let filtered = events;
    if (searching) {
      filtered = filtered.filter((ev) => eventMatchesQuery(ev, trimmed));
    }
    if (toolsOnly) {
      // Keep tool_use + tool_result so input/output pairs render together;
      // also keep error/cost_cap_hit so the headline Alerts up top still
      // fire — those aren't part of the noise the chip suppresses.
      filtered = filtered.filter(
        (ev) =>
          ev.type === "tool_use" ||
          ev.type === "tool_result" ||
          ev.type === "error" ||
          ev.type === "cost_cap_hit",
      );
    }
    if (errorsOnly) {
      // Keep errored tool_results + the error/cost_cap_hit events. Skip
      // tool_use entries whose matching result didn't error (they'd render
      // as orphan "pending" badges).
      const errResultIds = new Set(
        filtered
          .filter((ev) => ev.type === "tool_result" && ev.data.is_error === true)
          .map((ev) => (ev.type === "tool_result" ? ev.data.tool_use_id : "")),
      );
      filtered = filtered.filter((ev) => {
        if (ev.type === "error" || ev.type === "cost_cap_hit") return true;
        if (ev.type === "tool_result") return ev.data.is_error === true;
        if (ev.type === "tool_use") return errResultIds.has(ev.data.id);
        return false;
      });
    }
    return filtered;
  }, [events, trimmed, searching, toolsOnly, errorsOnly]);

  const turns = useMemo(
    () => groupIntoTurns(matchingEvents),
    [matchingEvents],
  );
  const resultEvent = useMemo(() => {
    // The sidecar emits two `result` events per run: the raw SDK message
    // (snake_case fields, nested `usage`) plus a normalized breadbox-shape
    // event with camelCase fields. We always want the normalized one — pick
    // the first event whose `data` has the expected `totalCostUsd` field,
    // falling back to the last `result` event for forward compatibility.
    const resultEvents = events.filter((e) => e.type === "result");
    const normalized = resultEvents.find(
      (e) =>
        e.type === "result" &&
        typeof (e.data as Partial<ResultData>).totalCostUsd === "number",
    );
    return normalized ?? resultEvents[resultEvents.length - 1];
  }, [events]);
  const errorEvent = useMemo(
    () => events.find((e) => e.type === "error"),
    [events],
  );
  const costCapEvent = useMemo(
    () => events.find((e) => e.type === "cost_cap_hit"),
    [events],
  );

  const matchedCount = matchingEvents.length;
  const noMatches = searching && matchedCount === 0;

  return (
    <div className="flex flex-col gap-4 px-1 pb-4">
      <div className="relative">
        <Search className="text-muted-foreground pointer-events-none absolute left-2 top-1/2 size-4 -translate-y-1/2" />
        <Input
          placeholder="Search transcript (assistant text, tool names, args, results)…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          className="pl-8 pr-8"
          aria-label="Search transcript"
        />
        {searching && (
          <button
            type="button"
            onClick={() => setQuery("")}
            className="text-muted-foreground hover:text-foreground absolute right-2 top-1/2 -translate-y-1/2"
            aria-label="Clear search"
          >
            <X className="size-4" />
          </button>
        )}
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <FilterChip
          icon={<Wrench className="size-3" />}
          label="Tools only"
          active={toolsOnly}
          onToggle={() => setToolsOnly((v) => !v)}
        />
        <FilterChip
          icon={<AlertCircle className="size-3" />}
          label="Errors only"
          active={errorsOnly}
          onToggle={() => setErrorsOnly((v) => !v)}
        />
        {(searching || toolsOnly || errorsOnly) && (
          <span className="text-muted-foreground text-xs">
            {noMatches
              ? "No matching events"
              : `Showing ${matchedCount} of ${events.length} events`}
          </span>
        )}
      </div>

      {truncated && (
        <Alert>
          <AlertTriangle className="size-4" />
          <AlertTitle>Transcript truncated</AlertTitle>
          <AlertDescription className="flex items-center gap-2">
            <span>
              Showing first {events.length} of {rawLength} events.
            </span>
            <Button asChild variant="link" size="sm" className="h-auto p-0">
              <a
                href={`/api/v1/agents/runs/${shortId}/transcript`}
                download={`transcript-${shortId}.ndjson`}
              >
                <Download className="size-3" /> Download full
              </a>
            </Button>
          </AlertDescription>
        </Alert>
      )}

      {errorEvent?.type === "error" && (
        <Alert variant="destructive">
          <AlertCircle className="size-4" />
          <AlertTitle>Run errored</AlertTitle>
          <AlertDescription>{errorEvent.data.message}</AlertDescription>
        </Alert>
      )}

      {costCapEvent?.type === "cost_cap_hit" && (
        <Alert>
          <AlertTriangle className="size-4" />
          <AlertTitle>Budget cap reached</AlertTitle>
          <AlertDescription>{costCapEvent.data.message}</AlertDescription>
        </Alert>
      )}

      {turns.length === 0 && !errorEvent && (
        <p className="text-muted-foreground text-sm">
          No assistant messages yet — the run may have failed before the
          model produced output.
        </p>
      )}

      {turns.map((turn, i) => (
        <TurnBlock key={i} turn={turn} index={i} />
      ))}

      {resultEvent?.type === "result" && (
        <ResultFooter data={resultEvent.data} />
      )}
    </div>
  );
}

function TurnBlock({ turn, index }: { turn: TurnGroup; index: number }) {
  return (
    <div className="border-border space-y-2 rounded-lg border p-3">
      <div className="text-muted-foreground text-[10px] uppercase tracking-wider">
        Turn {index + 1}
      </div>
      {turn.assistant && <MessageBubble data={turn.assistant} />}
      {turn.toolUses.map((tu) => {
        const result = turn.toolResults.find(
          (tr) => tr.tool_use_id === tu.id,
        );
        return (
          <ToolCallPair key={tu.id} toolUse={tu} toolResult={result} />
        );
      })}
    </div>
  );
}

function MessageBubble({ data }: { data: AssistantMessageData }) {
  const text = useMemo(() => extractText(data.message.content), [data]);
  if (!text) return null;
  return (
    <div className="bg-muted/40 rounded-md p-3">
      <pre className="text-foreground whitespace-pre-wrap break-words font-sans text-sm">
        {text}
      </pre>
    </div>
  );
}

function extractText(content: AssistantContent[]): string {
  return content
    .filter((b): b is { type: "text"; text: string } => b.type === "text")
    .map((b) => b.text)
    .join("\n\n");
}

interface ToolCallPairProps {
  toolUse: ToolUseData & { ts: number };
  toolResult?: ToolResultData & { ts: number };
}

// FilterChip is a small toggle button used for the iter-43 "Tools only" /
// "Errors only" quick filters above the transcript event stream. Active
// state inverts the visual (filled badge) so the chip reads as "currently
// applied" without a separate state indicator.
function FilterChip({
  icon,
  label,
  active,
  onToggle,
}: {
  icon: React.ReactNode;
  label: string;
  active: boolean;
  onToggle: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onToggle}
      aria-pressed={active}
      className={cn(
        "inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs transition",
        active
          ? "bg-primary text-primary-foreground border-primary"
          : "bg-background hover:bg-accent text-muted-foreground",
      )}
    >
      {icon}
      {label}
    </button>
  );
}

function ToolCallPair({ toolUse, toolResult }: ToolCallPairProps) {
  const [open, setOpen] = useState(false);
  const errored = toolResult?.is_error === true;
  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger asChild>
        <button
          type="button"
          className="hover:bg-accent flex w-full items-center gap-2 rounded-md border bg-background px-2 py-1.5 text-left text-xs"
        >
          {open ? (
            <ChevronDown className="size-3.5 shrink-0" />
          ) : (
            <ChevronRight className="size-3.5 shrink-0" />
          )}
          <Wrench className="text-muted-foreground size-3.5 shrink-0" />
          <span className="font-mono text-xs">{toolUse.name}</span>
          {toolResult && (
            <Badge
              variant={errored ? "destructive" : "outline"}
              className="ml-auto text-[10px]"
            >
              {errored ? "error" : "ok"}
            </Badge>
          )}
          {!toolResult && (
            <Badge variant="secondary" className="ml-auto text-[10px]">
              pending
            </Badge>
          )}
        </button>
      </CollapsibleTrigger>
      <CollapsibleContent className="space-y-2 px-2 pt-2">
        <CollapsibleSection label="Input">
          <CodeJson value={toolUse.input} />
        </CollapsibleSection>
        {toolResult && (
          <CollapsibleSection
            label={errored ? "Error output" : "Output"}
            tone={errored ? "error" : "default"}
          >
            <CodeJson value={toolResult.content} />
          </CollapsibleSection>
        )}
      </CollapsibleContent>
    </Collapsible>
  );
}

function CollapsibleSection({
  label,
  tone = "default",
  children,
}: {
  label: string;
  tone?: "default" | "error";
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-1">
      <div
        className={cn(
          "text-[10px] font-medium uppercase tracking-wider",
          tone === "error" ? "text-destructive" : "text-muted-foreground",
        )}
      >
        {label}
      </div>
      {children}
    </div>
  );
}

function CodeJson({ value }: { value: unknown }) {
  const text = useMemo(() => {
    if (typeof value === "string") return value;
    try {
      return JSON.stringify(value, null, 2);
    } catch {
      return String(value);
    }
  }, [value]);
  return (
    <pre className="bg-muted max-h-48 overflow-auto rounded p-2 text-[11px] leading-tight">
      {text}
    </pre>
  );
}

function ResultFooter({ data }: { data: ResultData }) {
  const stopPalette =
    data.stopReason === "budget_exceeded"
      ? "bg-red-100 text-red-700 dark:bg-red-950/40 dark:text-red-300"
      : data.stopReason === "max_turns"
        ? "bg-amber-100 text-amber-700 dark:bg-amber-950/40 dark:text-amber-300"
        : "bg-emerald-100 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300";
  return (
    <div className="bg-muted/40 mt-2 rounded-lg p-4">
      <div className="mb-3 flex items-center justify-between">
        <span className="text-muted-foreground text-xs uppercase tracking-wider">
          Result
        </span>
        <span
          className={cn(
            "inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-medium",
            stopPalette,
          )}
        >
          {data.stopReason || "completed"}
        </span>
      </div>
      <div className="grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
        <FooterStat
          icon={Coins}
          label="Cost"
          value={
            typeof data.totalCostUsd === "number"
              ? `$${data.totalCostUsd.toFixed(4)}`
              : "—"
          }
        />
        <FooterStat
          icon={Cpu}
          label="Turns"
          value={data.turnCount != null ? String(data.turnCount) : "—"}
        />
        <FooterStat
          icon={Wrench}
          label="Tool calls"
          value={data.numToolCalls != null ? String(data.numToolCalls) : "—"}
        />
        <FooterStat
          icon={Cpu}
          label="Tokens"
          value={`${(data.inputTokens ?? 0).toLocaleString()} in / ${(data.outputTokens ?? 0).toLocaleString()} out`}
          sub={
            (data.cacheReadTokens ?? 0) + (data.cacheCreationTokens ?? 0) > 0
              ? `cache: ${((data.cacheReadTokens ?? 0) + (data.cacheCreationTokens ?? 0)).toLocaleString()}`
              : undefined
          }
        />
      </div>
    </div>
  );
}

function FooterStat({
  icon: Icon,
  label,
  value,
  sub,
}: {
  icon: React.ComponentType<{ className?: string }>;
  label: string;
  value: string;
  sub?: string;
}) {
  return (
    <div className="space-y-0.5">
      <div className="text-muted-foreground flex items-center gap-1 text-[10px] uppercase tracking-wider">
        <Icon className="size-3" />
        {label}
      </div>
      <div className="text-foreground text-sm font-medium">{value}</div>
      {sub && (
        <div className="text-muted-foreground text-[10px]">{sub}</div>
      )}
    </div>
  );
}

// Re-export formatDuration for sites that pair the viewer with run metadata.
// (Importing from "@/lib/format" works too — this is a convenience alias.)
export { formatDuration };
