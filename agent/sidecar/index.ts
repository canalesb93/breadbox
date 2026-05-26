#!/usr/bin/env bun
/**
 * breadbox-agent sidecar.
 *
 * Reads one JobSpec as JSON from stdin, executes a Claude Agent SDK query,
 * streams NDJSON events on stdout, and exits.
 *
 * Auth: the Go runner (internal/agent/sidecar.go::authEnvFor) sets exactly
 * one of {ANTHROPIC_API_KEY, CLAUDE_CODE_OAUTH_TOKEN} in our process env
 * before exec. We still defensively scrub the inactive var here so a
 * misconfigured Go-side caller can't silently land in the precedence trap
 * (ANTHROPIC_API_KEY wins over CLAUDE_CODE_OAUTH_TOKEN).
 */
import { query } from "@anthropic-ai/claude-agent-sdk";
import cliAsset from "@anthropic-ai/claude-agent-sdk/cli.js" with { type: "file" };
import { existsSync, mkdirSync, writeFileSync, statSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { randomBytes } from "node:crypto";
import { JobSpecSchema, type JobSpec } from "./spec";
import { emit, emitError, ERROR_CODE } from "./events";

// resolveCliPath extracts the bundled cli.js to a real path on disk so the
// SDK's fs.existsSync check can see it. Inside a `bun build --compile`
// binary, cliAsset resolves to a bunfs path that the SDK's spawn helper
// cannot read. We materialize once per process startup, cached by mtime+
// size so repeated cold-starts on the same binary reuse the extracted copy.
//
// Also writes a sibling package.json with {"type": "module"} so node
// (the executable we pin in query()) treats the extracted .js as ESM.
// Without it node refuses to load the SDK's import statements and the
// spawned subprocess exits 1 with no useful event — surfacing here as
// "Claude Code process exited with code 1".
async function resolveCliPath(): Promise<string> {
  const dir = join(tmpdir(), "breadbox-agent-sidecar");
  mkdirSync(dir, { recursive: true });
  const bytes = await Bun.file(cliAsset).bytes();
  const cached = join(dir, `cli-${bytes.length}.js`);
  if (!existsSync(cached) || statSync(cached).size !== bytes.length) {
    writeFileSync(cached, bytes);
  }
  const pkgPath = join(dir, "package.json");
  if (!existsSync(pkgPath)) {
    writeFileSync(pkgPath, '{"type":"module"}');
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
  // The Go runner sets exactly one auth env var on our process env before
  // exec; here we only scrub the OTHER one so the SDK's "ANTHROPIC_API_KEY
  // beats CLAUDE_CODE_OAUTH_TOKEN" precedence rule can't bite if both
  // happen to be set (e.g. operator has ANTHROPIC_API_KEY in their shell
  // env and breadbox serve inherited it, then configures subscription
  // mode — Go already scrubs in that path, this is belt-and-suspenders).
  if (spec.auth.mode === "api_key") {
    delete process.env.CLAUDE_CODE_OAUTH_TOKEN;
  } else {
    delete process.env.ANTHROPIC_API_KEY;
  }
}

// configureRuntimeEnv carves out an isolated HOME so the spawned Claude
// Code subprocess writes its config (~/.claude.json + transcript cache
// + fs.watch state) into a per-run scratch dir instead of fighting the
// operator's logged-in Claude Code CLI for the global file. Without
// this the subprocess dies at startup with
//   EPERM: operation not permitted, open '/Users/<u>/.claude.json'
// any time another Claude process (cmux, the user's own session, a
// concurrent agent run) is holding the global config — the run never
// reaches the prompt and the operator gets an opaque
// "Claude Code process exited with code 1".
function configureRuntimeEnv(spec: JobSpec): void {
  const scratch = join(
    tmpdir(),
    `breadbox-agent-home-${spec.runId || randomBytes(4).toString("hex")}`,
  );
  mkdirSync(scratch, { recursive: true });
  process.env.HOME = scratch;
  // CLAUDE_CONFIG_DIR is the explicit override Claude Code 2.x honors
  // when set; we set both so the cli.js init path can pick whichever
  // it consults first.
  process.env.CLAUDE_CONFIG_DIR = scratch;
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
    emitError(`spec parse: ${e.message}`, e.stack, undefined, ERROR_CODE.SPEC_INVALID);
    process.exit(2);
  }

  configureAuth(spec);
  configureRuntimeEnv(spec);

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
  // Buffer stderr from the spawned Claude Code subprocess so we can
  // surface it on the error event below. The SDK swallows stderr by
  // default; without this the failure message that lands on the run row
  // is the useless "Claude Code process exited with code 1" — which
  // tells operators nothing about what actually went wrong.
  const stderrBuf: string[] = [];

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
        // Breadbox agents are autonomous — there's no human attached to
        // answer SDK permission prompts. `dontAsk` denies anything not
        // pre-approved via allowedTools / our MCP wildcard, which is the
        // correct posture for a scheduled run.
        permissionMode: "dontAsk",
        // settingSources defaults to ["user","project","local"] — the SDK
        // would otherwise load ~/.claude/CLAUDE.md, project-level
        // .claude/settings.json hooks/skills/permission rules, and any
        // local override discovered from cwd. In a self-hosted sidecar
        // the cwd is wherever Go exec'd from, so anyone who can write to
        // that path can inject agent-run config. Forcing `[]` keeps the
        // sidecar fully self-contained and reproducible across hosts.
        settingSources: [],
        resume: spec.sessionId,
        pathToClaudeCodeExecutable,
        executable: "node",
        stderr: (chunk: string) => {
          stderrBuf.push(chunk);
        },
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
    if (messageCount === 0) {
      // Zero messages typically means an auth failure (SDK aborted before
      // emitting anything) or a spawn failure (cli.js subprocess didn't
      // start). Default to AUTH since that's by far the more common cause
      // in practice — the spawn case is closed by pathToClaudeCodeExecutable.
      emitError(
        "agent SDK stream ended without yielding any messages — likely an invalid auth credential or subprocess spawn failure" +
          formatStderr(stderrBuf),
        undefined,
        transcriptPath,
        ERROR_CODE.AUTH,
      );
    } else {
      emitError(
        `agent SDK stream ended after ${messageCount} message(s) without emitting a result event` +
          formatStderr(stderrBuf),
        undefined,
        transcriptPath,
        ERROR_CODE.API,
      );
    }
    process.exit(1);
  } catch (err) {
    const e = err instanceof Error ? err : new Error(String(err));
    // Best-effort classification of the thrown error. The SDK wraps most
    // failures but some (network) leak through as raw fetch errors. We
    // classify off e.message + the captured stderr so e.g. an "EPERM
    // open ~/.claude.json" line in stderr would still surface as
    // RunErrorCodeUnknown — accurate, since none of our buckets cover
    // host-FS issues, and the operator gets the raw stderr appended
    // either way.
    const haystack = (e.message + " " + stderrBuf.join("")).toLowerCase();
    let code: (typeof ERROR_CODE)[keyof typeof ERROR_CODE] = ERROR_CODE.UNKNOWN;
    if (haystack.includes("unauthorized") || haystack.includes("invalid api key") || haystack.includes("invalid_api_key")) {
      code = ERROR_CODE.AUTH;
    } else if (haystack.includes("overloaded") || haystack.includes("rate limit")) {
      code = ERROR_CODE.API;
    } else if (haystack.includes("enotfound") || haystack.includes("econnrefused") || haystack.includes("fetch failed")) {
      code = ERROR_CODE.NETWORK;
    }
    emitError(e.message + formatStderr(stderrBuf), e.stack, transcriptPath, code);
    process.exit(1);
  }
}

// formatStderr trims and prefixes the captured Claude Code subprocess
// stderr onto the breadbox-facing error message. Empty buffers stay
// empty so successful runs don't get a trailing "—" stub.
function formatStderr(buf: string[]): string {
  const joined = buf.join("").trim();
  if (!joined) return "";
  // Cap at ~2 KB so a misbehaving subprocess doesn't blow up the row.
  const max = 2048;
  const body = joined.length > max ? joined.slice(0, max) + " …(truncated)" : joined;
  return `\n\n— stderr —\n${body}`;
}

process.on("SIGTERM", () => {
  emitError("sidecar interrupted: SIGTERM", undefined, undefined, ERROR_CODE.INTERRUPTED);
  process.exit(130);
});

process.on("SIGINT", () => {
  emitError("sidecar interrupted: SIGINT", undefined, undefined, ERROR_CODE.INTERRUPTED);
  process.exit(130);
});

main().catch((err) => {
  const e = err instanceof Error ? err : new Error(String(err));
  emitError(e.message, e.stack, undefined, ERROR_CODE.UNKNOWN);
  process.exit(1);
});
