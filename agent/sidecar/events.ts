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
 */
export function emit(event: SidecarEvent, _transcriptPath?: string): void {
  const line = JSON.stringify(event) + "\n";
  process.stdout.write(line);
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
