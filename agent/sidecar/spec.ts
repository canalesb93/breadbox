import { z } from "zod";

// MCP server: either stdio (command+args) or HTTP (type=http + URL).
const StdioServerConfig = z.object({
  command: z.string(),
  args: z.array(z.string()).optional(),
  env: z.record(z.string(), z.string()).optional(),
});

const HttpServerConfig = z.object({
  type: z.literal("http"),
  url: z.string().url(),
  headers: z.record(z.string(), z.string()).optional(),
});

export const McpServerConfigSchema = z.union([StdioServerConfig, HttpServerConfig]);

// Auth mode: metadata only. The actual secret is delivered via env vars
// (ANTHROPIC_API_KEY or CLAUDE_CODE_OAUTH_TOKEN) set by the Go runner's
// cmd.Env before exec — NEVER through this spec — so plaintext tokens
// can't leak via stdin captures, /proc snapshots, or a stray log of the
// spec object. See internal/agent/sidecar.go::authEnvFor.
const AuthConfigSchema = z.object({
  mode: z.enum(["subscription", "api_key"]),
});

// JobSpec: the JSON document read from stdin.
export const JobSpecSchema = z.object({
  // Identity (forwarded for log correlation; not sent to the SDK)
  runId: z.string().optional().default(""),
  workflowId: z.string().optional().default(""),

  // Prompt
  prompt: z.string().min(1),
  systemPrompt: z.string().optional(),

  // Model parameters
  model: z.string().min(1),
  maxTurns: z.number().int().positive(),
  maxBudgetUsd: z.number().positive(),

  // Tool config
  toolScope: z.enum(["read_only", "read_write"]).default("read_write"),
  allowedTools: z.array(z.string()).default([]),

  // MCP servers
  mcpServers: z.record(z.string(), McpServerConfigSchema).default({}),

  // Auth
  auth: AuthConfigSchema,

  // Transcript file (optional; Go side opens it too — we duplicate for crash safety)
  transcriptPath: z.string().optional(),

  // Resume a prior SDK session
  sessionId: z.string().optional(),
});

export type JobSpec = z.infer<typeof JobSpecSchema>;
export type McpServerConfig = z.infer<typeof McpServerConfigSchema>;
export type AuthConfig = z.infer<typeof AuthConfigSchema>;
