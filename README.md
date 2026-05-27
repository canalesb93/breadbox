# Breadbox

Self-hosted financial data store with an MCP server.

<img src="docs/images/dashboard.png" alt="Breadbox dashboard" width="840">

## What it is

Breadbox aggregates bank transactions into a PostgreSQL database you
control and exposes them over a REST API and an MCP server. The intent
is infrastructure: one normalized data store that AI agents, dashboards,
and scripts can all query without each tool needing your bank credentials.

- **MCP server** at `/mcp` (Streamable HTTP) and `breadbox mcp` (stdio),
  scoped API keys with per-tool read/write permissions
- **REST API** under `/api/v1/*`, specified in [`openapi.yaml`](openapi.yaml)
- **Built-in agent runtime** — schedule [Claude Agent SDK](https://docs.claude.com/en/api/agent-sdk/overview)
  runs on cron / sync-complete / on-demand, with per-run cost + turn caps
  and full NDJSON transcripts. Bring your own Anthropic API key or OAuth
  subscription token.
- **Pluggable bank sync** — a normalized provider interface; integrations
  and CSV import ship today, more on the roadmap
- **Admin dashboard** — transaction review queue, rule engine, sync
  monitoring, multi-user household support

## Quick start

```bash
curl -fsSL https://breadbox.sh/install.sh | bash
```

Detects OS, installs Docker if needed, prompts for an optional public
domain, generates secrets, and brings up the stack. Visit
`http://localhost:8080/setup` (or your domain) to create the admin
account.

Full install docs — binary download, Go source, manual Docker, daemon
registration: **[docs.breadbox.sh/install](https://docs.breadbox.sh/install)**.

## CLI

The same `breadbox` binary doubles as a `gh`-style CLI for driving any
Breadbox instance — locally or remote — over its REST API. Per-host
credentials live in `~/.config/breadbox/hosts.toml`; switch hosts with
`--host <name>` or `BREADBOX_HOST=<name>`.

Already have a Breadbox server somewhere and just want the CLI on your
laptop? Install only the 10 MB lite build — no Docker, no Postgres:

```bash
curl -fsSL https://breadbox.sh/cli.sh | bash
```

Detects your OS, drops `breadbox` into `~/.local/bin` (or `/usr/local/bin`
under `sudo`), and prompts for the host URL to connect to.

### Local (same machine as the server)

```bash
breadbox auth bootstrap           # mint a full-access key, save to hosts.toml
breadbox doctor                   # readiness report
breadbox transactions list --limit 10
breadbox connections link --provider=plaid --user=<short_id> --wait
```

### Remote (device-code login)

```bash
breadbox auth login --host=https://breadbox.example.com
# prints a verification URL + short code; approve on the server's /auth/device page
breadbox accounts list
```

Output is a human table on a TTY and JSON when piped, so
`breadbox transactions list | jq '.[].amount'` just works. Full command
catalog: [`docs/cli-commands.md`](docs/cli-commands.md).

A 10 MB `breadbox-cli` lite build ships separately for remote agents and
scripts that don't need the server packages.

## AI agents

<img src="docs/images/claude-desktop.png" alt="Claude querying Breadbox via MCP" width="840">

Point any MCP client at `https://your-host/mcp` with an API key. Read
transactions, apply categories, write rules, surface anomalies — without
the agent ever touching bank credentials.

Claude Desktop / Claude Code config:

```json
{
  "mcpServers": {
    "breadbox": {
      "command": "/path/to/breadbox",
      "args": ["mcp"],
      "env": {
        "DATABASE_URL": "postgres://breadbox:breadbox@localhost:5432/breadbox?sslmode=disable"
      }
    }
  }
}
```

Or let Breadbox run the agents itself: the built-in runtime ships with
five starter agents (Initial Setup, Bulk Review, Quick Review, Routine
Review, Spending Report) and a prompt builder for your own. See the
[multi-agent reviewer guide](https://docs.breadbox.sh/guides/multi-agent-reviewer).

## Status

Pre-1.0. Breaking changes are documented in [`CHANGELOG.md`](CHANGELOG.md).
Provider credentials are encrypted at rest with AES-256-GCM; the whole
stack runs as a single Go binary alongside Postgres.

## Documentation

- **[docs.breadbox.sh](https://docs.breadbox.sh)** — install, providers, agents, API
- [`docs/`](docs/) in this repo — engineering specs (data model, architecture, MCP tools, rule DSL)
- [`CONTRIBUTING.md`](CONTRIBUTING.md) — development setup

## License

[AGPL-3.0](LICENSE)
