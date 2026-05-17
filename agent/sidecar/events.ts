import { writeSync } from "node:fs";

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
 * Emit one NDJSON event to stdout. The Go orchestrator (sidecar.go) is the
 * sole writer of the transcript file on disk — it reads each line off our
 * stdout and persists it. Letting both processes touch the same file races
 * and interleaves mid-JSON bytes (the malformed-line bug we hit in iter-47
 * dogfooding). The transcriptPath argument is kept on the signature for
 * call-site compatibility but is intentionally unused.
 *
 * Uses writeSync(1, …) instead of process.stdout.write because Bun pipes
 * stdout in non-blocking mode and may delay actual flushes until the next
 * I/O tick. For a streaming run watched live in the SPA, that delay turned
 * up as "events only appear at the end" (iter-47 dogfooding finding #7).
 * Sync writes are fine here — each line is small (<1 KB typical) and we
 * emit on the order of dozens per run, not thousands per second.
 */
export function emit(event: SidecarEvent, _transcriptPath?: string): void {
  const line = JSON.stringify(event) + "\n";
  writeSync(1, line);
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
