# breadbox-agent sidecar

Self-contained TypeScript subprocess that runs a single Claude Agent SDK
query on behalf of the Go breadbox server. Reads one `JobSpec` as JSON on
stdin, streams NDJSON events on stdout, exits.

Build the standalone binary (embeds the Bun runtime, ~50 MB):

```sh
bun install
bun run build   # writes bin/breadbox-agent
```

Then point breadbox at it via the `agent.runtime_path` app_config key, or
the `BREADBOX_AGENT_BIN` env var, or by placing the binary at
`./bin/breadbox-agent` relative to the breadbox process working directory.
