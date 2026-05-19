#!/usr/bin/env bun
/**
 * breadbox-agent sidecar.
 *
 * Reads one JobSpec as JSON from stdin, executes a Claude Agent SDK query,
 * streams NDJSON events on stdout, and exits.
 *
 * Auth precedence trap: ANTHROPIC_API_KEY wins over CLAUDE_CODE_OAUTH_TOKEN
 * when both are set. We scrub the unused var before invoking the SDK.
 */
import { query } from "@anthropic-ai/claude-agent-sdk";
import cliAsset from "@anthropic-ai/claude-agent-sdk/cli.js" with { type: "file" };
import { existsSync, mkdirSync, writeFileSync, statSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { JobSpecSchema, type JobSpec } from "./spec";
import { emit, emitError } from "./events";

// resolveCliPath extracts the bundled cli.js to a real path on disk so the
// SDK's fs.existsSync check can see it. Inside a `bun build --compile`
// binary, cliAsset resolves to a bunfs path that the SDK's spawn helper
// cannot read. We materialize once per process startup, cached by mtime+
// size so repeated cold-starts on the same binary reuse the extracted copy.
async function resolveCliPath(): Promise<string> {
  const dir = join(tmpdir(), "breadbox-agent-sidecar");
  mkdirSync(dir, { recursive: true });
  const bytes = await Bun.file(cliAsset).bytes();
  const cached = join(dir, `cli-${bytes.length}.js`);
  if (!existsSync(cached) || statSync(cached).size !== bytes.length) {
    writeFileSync(cached, bytes);
  }
  return cached;
}

async function readStdin(): Promise<string> {
  return new Promise((resolve, reject) => {
    let buf = "";
    process.stdin.setEncoding("utf8");
    process.stdin.on("data", (chunk: string) => {
      buf += chunk;
    });
    process.stdin.on("end", () => resolve(buf));
    process.stdin.on("error", (err: Error) => reject(err));
  });
}

function configureAuth(spec: JobSpec): void {
  if (spec.auth.mode === "api_key") {
    delete process.env.CLAUDE_CODE_OAUTH_TOKEN;
    process.env.ANTHROPIC_API_KEY = spec.auth.token;
  } else {
    delete process.env.ANTHROPIC_API_KEY;
    process.env.CLAUDE_CODE_OAUTH_TOKEN = spec.auth.token;
  }
}

async function main() {
  let spec: JobSpec;
  let transcriptPath: string | undefined;

  try {
    const raw = await readStdin();
    if (!raw.trim()) {
      throw new Error("empty stdin: expected a JobSpec JSON document");
    }
    const parsed = JSON.parse(raw);
    spec = JobSpecSchema.parse(parsed);
    transcriptPath = spec.transcriptPath;
  } catch (err) {
    const e = err instanceof Error ? err : new Error(String(err));
    emitError(`spec parse: ${e.message}`, e.stack);
    process.exit(2);
  }

  configureAuth(spec);

  // Track cumulative cost defensively even though the SDK enforces maxBudgetUsd.
  let cumulativeCostUsd = 0;
  let turnCount = 0;
  let numToolCalls = 0;

  // SDK spawns `<executable> cli.js` under the hood and fs.existsSync's the
  // path. bun --compile bundles cli.js into bunfs which fs.existsSync can't
  // read, so we extract to a tmp file first. See resolveCliPath above.
  const pathToClaudeCodeExecutable = await resolveCliPath();

  // Force the spawn executable to "node". The SDK defaults to
  // isRunningWithBun() ? "bun" : "node" — and we ARE running under bun
  // (this binary was built with `bun build --compile`), so it would
  // otherwise pick "bun" and spawn ENOENT in the runtime image where
  // only nodejs is installed. The ENOENT fires on an unhandled spawn
  // error handler inside the SDK; the async iterator below then ends
  // silently with zero messages, which we'd misclassify as a clean
  // success with $0/0-turn metrics. Pinning to "node" makes the spawn
  // deterministic and matches the runtime apk install in the Dockerfile.
  try {
    const stream = query({
      prompt: spec.prompt,
      options: {
        model: spec.model,
        systemPrompt: spec.systemPrompt,
        maxTurns: spec.maxTurns,
        // maxBudgetUsd is supported on recent SDK versions; passing it is a no-op
        // on older builds. The post-result check below is the durable belt.
        maxBudgetUsd: spec.maxBudgetUsd,
        allowedTools: spec.allowedTools.length > 0 ? spec.allowedTools : undefined,
        mcpServers: spec.mcpServers,
        permissionMode: "dontAsk",
        resume: spec.sessionId,
        pathToClaudeCodeExecutable,
        executable: "node",
      },
    });

    let messageCount = 0;
    for await (const message of stream as AsyncIterable<any>) {
      messageCount += 1;
      const ts = Date.now();
      const rawType = (message?.type as string | undefined) ?? "system";

      // Normalize SDK type names to the breadbox-side contract documented in
      // internal/agent/event.go and consumed by web/src/features/agents/
      // transcript-viewer.tsx. The SDK currently emits "assistant" /
      // "user" for content events; iter-1's spec named these "assistant_message"
      // / "user_message" assuming an earlier SDK shape. Tool_use blocks
      // arrive as content blocks INSIDE the assistant event, not as their
      // own top-level events — counting them is handled below by inspecting
      // the nested message content.
      let type = rawType;
      if (rawType === "assistant") type = "assistant_message";
      else if (rawType === "user") type = "user_message";

      if (rawType === "assistant" && Array.isArray(message?.message?.content)) {
        for (const block of message.message.content) {
          if (block?.type === "tool_use") numToolCalls += 1;
        }
      }

      emit({ type: type as any, ts, data: message }, transcriptPath);

      if (type === "result") {
        // The SDK's ResultMessage shape varies; tolerate both flat and nested.
        const totalCostUsd =
          (message?.total_cost_usd as number | undefined) ??
          (message?.totalCostUsd as number | undefined) ??
          0;
        const numTurns =
          (message?.num_turns as number | undefined) ??
          (message?.numTurns as number | undefined) ??
          0;
        const usage = (message?.usage as Record<string, number> | undefined) ?? {};
        const sessionId =
          (message?.session_id as string | undefined) ??
          (message?.sessionId as string | undefined) ??
          "";
        const stopReasonRaw =
          (message?.stop_reason as string | undefined) ??
          (message?.stopReason as string | undefined) ??
          "";
        const isError = Boolean(message?.is_error);
        const subtype = (message?.subtype as string | undefined) ?? "";

        cumulativeCostUsd = totalCostUsd;
        turnCount = numTurns;

        let stopReason = stopReasonRaw;
        if (subtype.startsWith("error_max_budget")) stopReason = "budget_exceeded";
        else if (subtype === "error_max_turns") stopReason = "max_turns";
        else if (!stopReason && subtype === "success") stopReason = "end_turn";

        // Emit a structured `result` event with the breadbox-side shape.
        emit(
          {
            type: "result",
            ts: Date.now(),
            data: {
              totalCostUsd,
              inputTokens: Number(usage.input_tokens ?? 0),
              outputTokens: Number(usage.output_tokens ?? 0),
              cacheReadTokens: Number(usage.cache_read_input_tokens ?? 0),
              cacheCreationTokens: Number(usage.cache_creation_input_tokens ?? 0),
              turnCount,
              numToolCalls,
              sessionId,
              stopReason,
            },
          },
          transcriptPath,
        );

        if (stopReason === "budget_exceeded") {
          emit(
            {
              type: "cost_cap_hit",
              ts: Date.now(),
              data: {
                code: "BUDGET_EXCEEDED",
                message: `cumulative cost ${cumulativeCostUsd} exceeded cap ${spec.maxBudgetUsd}`,
              },
            },
            transcriptPath,
          );
          process.exit(1);
        }

        if (isError) process.exit(1);
        process.exit(0);
      }
    }

    // Stream ended without ever emitting a `result` event. Historically
    // this branch exited 0 with zero usage, which let SDK-side silent
    // failures (e.g. spawn ENOENT on the cli.js subprocess because the
    // configured executable isn't on PATH) sail through as phantom
    // "successes" with empty transcripts. Surface as an explicit error
    // event + non-zero exit so the orchestrator records something the
    // operator can actually read in the transcript drawer.
    const reason =
      messageCount === 0
        ? "without yielding any messages — likely a subprocess spawn failure (executable not on PATH?) or an invalid auth credential"
        : `after ${messageCount} message(s) without emitting a result event`;
    emitError(`agent SDK stream ended ${reason}`, undefined, transcriptPath);
    process.exit(1);
  } catch (err) {
    const e = err instanceof Error ? err : new Error(String(err));
    emitError(e.message, e.stack, transcriptPath);
    process.exit(1);
  }
}

process.on("SIGTERM", () => {
  emitError("sidecar interrupted: SIGTERM");
  process.exit(130);
});

process.on("SIGINT", () => {
  emitError("sidecar interrupted: SIGINT");
  process.exit(130);
});

main().catch((err) => {
  const e = err instanceof Error ? err : new Error(String(err));
  emitError(e.message, e.stack);
  process.exit(1);
});
