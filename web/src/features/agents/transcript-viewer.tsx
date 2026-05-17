import { useMemo, useState } from "react";
import {
  AlertCircle,
  AlertTriangle,
  ChevronDown,
  ChevronRight,
  Coins,
  Cpu,
  Download,
  Wrench,
} from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
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

export function TranscriptViewer({
  events,
  rawLength,
  truncated,
  shortId,
}: TranscriptViewerProps) {
  const turns = useMemo(() => groupIntoTurns(events), [events]);
  const resultEvent = useMemo(
    () => events.find((e) => e.type === "result"),
    [events],
  );
  const errorEvent = useMemo(
    () => events.find((e) => e.type === "error"),
    [events],
  );
  const costCapEvent = useMemo(
    () => events.find((e) => e.type === "cost_cap_hit"),
    [events],
  );

  return (
    <div className="flex flex-col gap-4 px-1 pb-4">
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
          value={`$${data.totalCostUsd.toFixed(4)}`}
        />
        <FooterStat icon={Cpu} label="Turns" value={String(data.turnCount)} />
        <FooterStat
          icon={Wrench}
          label="Tool calls"
          value={String(data.numToolCalls)}
        />
        <FooterStat
          icon={Cpu}
          label="Tokens"
          value={`${data.inputTokens.toLocaleString()} in / ${data.outputTokens.toLocaleString()} out`}
          sub={
            data.cacheReadTokens + data.cacheCreationTokens > 0
              ? `cache: ${(data.cacheReadTokens + data.cacheCreationTokens).toLocaleString()}`
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
