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
  AssistantMessageData,
  ResultData,
  ToolResultData,
  ToolUseData,
  TranscriptEvent,
  UserContent,
} from "@/api/queries/agents";

interface TranscriptViewerProps {
  events: TranscriptEvent[];
  rawLength: number;
  truncated: boolean;
  shortId: string;
}

// Grouped representation of one model turn. The SDK splits a single model
// turn into MULTIPLE assistant_message events — typically one with the text
// (reasoning) block and a separate one with the tool_use block. The viewer
// groups consecutive assistant_messages together so one turn = one card,
// matching the SDK's `num_turns` count in the footer.
interface TurnGroup {
  ts: number;
  text: string;
  toolUses: Array<ToolUseData & { ts: number }>;
  toolResults: Array<ToolResultData & { ts: number }>;
}

function emptyTurn(ts: number): TurnGroup {
  return { ts, text: "", toolUses: [], toolResults: [] };
}

function turnHasContent(t: TurnGroup): boolean {
  return (
    t.text.length > 0 ||
    t.toolUses.length > 0 ||
    t.toolResults.length > 0
  );
}

// extractToolUses pulls tool_use content blocks out of an assistant message.
// The SDK nests them inside `message.content[]`; earlier viewer iterations
// expected standalone top-level "tool_use" events that never actually
// appeared on the wire (iter-46 finding, fixed in iter-48).
function extractToolUses(
  data: AssistantMessageData,
  ts: number,
): Array<ToolUseData & { ts: number }> {
  const out: Array<ToolUseData & { ts: number }> = [];
  for (const block of data.message?.content ?? []) {
    if (block.type === "tool_use") {
      out.push({
        ts,
        type: "tool_use",
        id: block.id,
        name: block.name,
        input: (block.input ?? {}) as Record<string, unknown>,
      });
    }
  }
  return out;
}

// extractToolResults pulls tool_result content blocks out of a user message.
// Same nesting story as extractToolUses.
function extractToolResults(
  content: UserContent[] | undefined,
  ts: number,
): Array<ToolResultData & { ts: number }> {
  const out: Array<ToolResultData & { ts: number }> = [];
  for (const block of content ?? []) {
    if (block.type === "tool_result") {
      out.push({
        ts,
        type: "tool_result",
        tool_use_id: block.tool_use_id,
        content: block.content,
        is_error: block.is_error,
      });
    }
  }
  return out;
}

function extractText(data: AssistantMessageData): string {
  const parts: string[] = [];
  for (const block of data.message?.content ?? []) {
    if (block.type === "text" && block.text) parts.push(block.text);
  }
  return parts.join("\n\n");
}

function groupIntoTurns(events: TranscriptEvent[]): TurnGroup[] {
  const turns: TurnGroup[] = [];
  let current = emptyTurn(0);
  // sawUserSinceLastAssistant tracks whether the next assistant_message
  // should open a new turn. The SDK splits one model turn into multiple
  // assistant_message events (text + tool_use as separate messages); only
  // a user_message (tool_results coming back) marks the end of a turn.
  let sawUserSinceLastAssistant = true;
  for (const ev of events) {
    if (ev.type === "assistant_message") {
      if (sawUserSinceLastAssistant) {
        if (turnHasContent(current)) turns.push(current);
        current = emptyTurn(ev.ts);
        sawUserSinceLastAssistant = false;
      }
      const text = extractText(ev.data);
      if (text) current.text = current.text ? `${current.text}\n\n${text}` : text;
      current.toolUses.push(...extractToolUses(ev.data, ev.ts));
    } else if (ev.type === "user_message") {
      current.toolResults.push(
        ...extractToolResults(ev.data.message?.content, ev.ts),
      );
      sawUserSinceLastAssistant = true;
    } else if (ev.type === "tool_use") {
      current.toolUses.push({ ...ev.data, ts: ev.ts });
    } else if (ev.type === "tool_result") {
      current.toolResults.push({ ...ev.data, ts: ev.ts });
      sawUserSinceLastAssistant = true;
    }
  }
  if (turnHasContent(current)) turns.push(current);
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
      return blocks.some((b) => {
        if (b.type === "text") return b.text.toLowerCase().includes(needle);
        if (b.type === "tool_use") {
          if (b.name?.toLowerCase().includes(needle)) return true;
          try {
            return JSON.stringify(b.input).toLowerCase().includes(needle);
          } catch {
            return false;
          }
        }
        return false;
      });
    }
    case "user_message": {
      const blocks = ev.data.message?.content ?? [];
      return blocks.some((b) => {
        if (b.type === "tool_result") {
          try {
            return JSON.stringify(b.content).toLowerCase().includes(needle);
          } catch {
            return false;
          }
        }
        return false;
      });
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
      // Tool-use lives in assistant_message.content; tool_result lives in
      // user_message.content. Keep those messages when they carry any tool
      // block (a text-only assistant_message gets dropped). Also keep
      // standalone tool_use/tool_result (forward compat) and the headline
      // error/cost_cap_hit events so the Alerts up top still fire.
      filtered = filtered.filter((ev) => {
        if (ev.type === "tool_use" || ev.type === "tool_result") return true;
        if (ev.type === "error" || ev.type === "cost_cap_hit") return true;
        if (ev.type === "assistant_message") {
          return (ev.data.message?.content ?? []).some(
            (b) => b.type === "tool_use",
          );
        }
        if (ev.type === "user_message") {
          return (ev.data.message?.content ?? []).some(
            (b) => b.type === "tool_result",
          );
        }
        return false;
      });
    }
    if (errorsOnly) {
      // Collect erroring tool_result block IDs (nested in user_message
      // content, plus any legacy top-level events). Then keep assistant_
      // messages that fired any of those IDs, the user_messages that carry
      // the errors, and the headline error/cost_cap_hit alerts.
      const errResultIds = new Set<string>();
      for (const ev of filtered) {
        if (ev.type === "tool_result" && ev.data.is_error) {
          errResultIds.add(ev.data.tool_use_id);
        }
        if (ev.type === "user_message") {
          for (const b of ev.data.message?.content ?? []) {
            if (b.type === "tool_result" && b.is_error) {
              errResultIds.add(b.tool_use_id);
            }
          }
        }
      }
      filtered = filtered.filter((ev) => {
        if (ev.type === "error" || ev.type === "cost_cap_hit") return true;
        if (ev.type === "tool_result") return ev.data.is_error === true;
        if (ev.type === "tool_use") return errResultIds.has(ev.data.id);
        if (ev.type === "user_message") {
          return (ev.data.message?.content ?? []).some(
            (b) => b.type === "tool_result" && b.is_error === true,
          );
        }
        if (ev.type === "assistant_message") {
          return (ev.data.message?.content ?? []).some(
            (b) => b.type === "tool_use" && errResultIds.has(b.id),
          );
        }
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
      {turn.text && <MessageBubble text={turn.text} />}
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

function MessageBubble({ text }: { text: string }) {
  if (!text) return null;
  return (
    <div className="bg-muted/40 rounded-md p-3">
      <pre className="text-foreground whitespace-pre-wrap break-words font-sans text-sm">
        {text}
      </pre>
    </div>
  );
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
