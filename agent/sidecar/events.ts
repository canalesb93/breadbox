import { appendFileSync } from "node:fs";

export type SidecarEventType =
  | "assistant_message"
  | "tool_use"
  | "tool_result"
  | "result"
  | "error"
  | "system"
  | "cost_cap_hit";

export interface SidecarEvent {
  type: SidecarEventType;
  ts: number; // Unix ms
  data: Record<string, unknown>;
}

/**
 * Emit one NDJSON event to stdout and (if configured) append to the
 * transcript file. Writes are synchronous so they survive a crash.
 */
export function emit(event: SidecarEvent, transcriptPath?: string): void {
  const line = JSON.stringify(event) + "\n";
  process.stdout.write(line);
  if (transcriptPath) {
    try {
      appendFileSync(transcriptPath, line, "utf8");
    } catch {
      // Best-effort: don't fail the run if the transcript file is unwritable.
    }
  }
}

export function emitError(
  message: string,
  stack?: string,
  transcriptPath?: string,
): void {
  emit(
    {
      type: "error",
      ts: Date.now(),
      data: { message, stack: stack ?? "" },
    },
    transcriptPath,
  );
}
