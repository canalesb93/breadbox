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

// Switch stdout to blocking mode at module load. Without this, writeSync on
// a pipe whose kernel buffer is full throws EAGAIN (Bun pipes stdout in
// O_NONBLOCK by default). A ~64 KB MCP tool result (e.g. list_categories)
// hits the pipe boundary mid-write, the next writeSync throws EAGAIN, and
// the NDJSON line gets truncated — concatenating the next event's bytes
// onto the tail with no newline and dropping the tool_result.
//
// setBlocking(true) makes writeSync block until the kernel accepts every
// byte — the natural single-writer behavior we want for an NDJSON stream
// where each line is one event.
(process.stdout as unknown as { _handle?: { setBlocking?: (b: boolean) => void } })?._handle?.setBlocking?.(true);

/**
 * writeAll loops writeSync until every byte is on the wire. Even with
 * blocking stdout, writeSync may still return short (e.g. interrupted
 * syscall) and may rarely throw EAGAIN if blocking didn't take effect —
 * so the loop is the robust shape regardless.
 */
function writeAll(fd: number, str: string): void {
  const buf = Buffer.from(str, "utf8");
  let offset = 0;
  while (offset < buf.length) {
    try {
      const n = writeSync(fd, buf, offset, buf.length - offset);
      if (n <= 0) break;
      offset += n;
    } catch (e) {
      const code = (e as NodeJS.ErrnoException).code;
      if (code === "EAGAIN" || code === "EWOULDBLOCK" || code === "EINTR") {
        // Pipe wasn't ready; give the reader a tick to drain and retry.
        // Bun.sleepSync is synchronous — fine here because we're already
        // blocking and don't yield to the event loop during emit().
        try {
          // @ts-expect-error Bun global at runtime
          Bun.sleepSync(1);
        } catch {
          // ignore if not running under Bun
        }
        continue;
      }
      throw e;
    }
  }
}

/**
 * Emit one NDJSON event to stdout. The Go orchestrator (sidecar.go) is the
 * sole writer of the transcript file on disk — it reads each line off our
 * stdout and persists it.
 *
 * Synchronous so the line lands in the kernel pipe immediately (the SPA
 * polls live), and loops on short writes / EAGAIN so events larger than
 * the kernel pipe buffer never truncate.
 */
export function emit(event: SidecarEvent, _transcriptPath?: string): void {
  writeAll(1, JSON.stringify(event) + "\n");
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
