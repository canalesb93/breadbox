# Breadbox

Self-hosted financial data aggregation for households. Connect your banks, sync transactions automatically, and query your financial data through a REST API or AI agents via [MCP](https://modelcontextprotocol.io/).

<!-- TODO: Add screenshot of dashboard here -->

## Why Breadbox?

Banks silo your financial data behind their own apps. AI agents can help with budgeting, spending analysis, and anomaly detection -- but they need structured access to your data without touching your bank credentials.

Breadbox syncs your bank data into a PostgreSQL database you control, then exposes it through a structured API that any tool or agent can query. Your data stays on your hardware, encrypted at rest, exportable and deletable at any time.

## Features

- **Bank sync** via [Plaid](https://plaid.com/), [Teller](https://teller.io/), and CSV import
- **MCP server** for AI agent integration (Streamable HTTP + stdio)
- **REST API** with cursor pagination, field selection, and filtering
- **Admin dashboard** with connection management, sync monitoring, and transaction review
- **Transaction rules engine** with recursive AND/OR/NOT conditions for auto-categorization
- **Review queue** for triaging new or uncategorized transactions
- **Account linking** for cross-connection transaction deduplication (e.g., authorized user cards)
- **Multi-user** household support (admin + family members)
- **Category system** with 2-level hierarchy
- **Agent reports** for AI agents to submit summaries and flag transactions
- **API key auth** with scoped access (full/read-only)
- **AES-256-GCM encryption** for provider credentials at rest
- **Single binary** -- one Go binary serves everything (API, MCP, dashboard, webhooks, cron)

## Quick Start

### Docker (recommended)

```bash
git clone https://github.com/canalesb93/breadbox.git && cd breadbox
cp .env.example .docker.env

# Edit .docker.env — set ENCRYPTION_KEY (generate with: openssl rand -hex 32)

docker compose up -d

# Visit http://localhost:8080 to start the setup wizard
```

### From source

```bash
git clone https://github.com/canalesb93/breadbox.git && cd breadbox

# Requires Go 1.24+ and PostgreSQL 16+
make dev

# Visit http://localhost:8080
```

### Binary download

Download the latest release from [GitHub Releases](https://github.com/canalesb93/breadbox/releases):

```bash
# Configure
cp .env.example .local.env
# Edit .local.env — set DATABASE_URL and ENCRYPTION_KEY

# Run
./breadbox serve
```

## MCP Integration

Breadbox exposes financial data to AI agents via the [Model Context Protocol](https://modelcontextprotocol.io/). This is the primary way to connect your financial data to AI assistants.

**Streamable HTTP** (remote): The MCP endpoint is at `/mcp`, authenticated with an API key.

**Stdio** (local): Run `breadbox mcp-stdio` for direct stdin/stdout MCP transport.

### Claude Desktop / Claude Code

```json
{
  "mcpServers": {
    "breadbox": {
      "command": "/path/to/breadbox",
      "args": ["mcp-stdio"],
      "env": {
        "DATABASE_URL": "postgres://breadbox:breadbox@localhost:5432/breadbox?sslmode=disable"
      }
    }
  }
}
```

### Remote MCP (any agent)

Point your agent's MCP client at your Breadbox instance:

```
URL: https://your-host/mcp
Header: X-API-Key: bb_your_api_key
```

## Configuration

All configuration via environment variables. See `.env.example` for the full list.

| Variable | Description | Required |
|---|---|---|
| `DATABASE_URL` | PostgreSQL connection string | Yes |
| `ENCRYPTION_KEY` | 32-byte hex key for AES-256-GCM (`openssl rand -hex 32`) | Yes (with Plaid/Teller) |
| `SERVER_PORT` | HTTP listen port (default: 8080) | No |
| `PLAID_CLIENT_ID` | Plaid API client ID | No |
| `PLAID_SECRET` | Plaid API secret | No |
| `TELLER_APP_ID` | Teller application ID | No |

Provider credentials can also be configured through the setup wizard or admin dashboard.

## CLI

```
breadbox serve           Start the server (API, MCP, dashboard, webhooks, cron)
breadbox create-admin    Create an admin user
breadbox mcp-stdio       Start MCP server on stdin/stdout
breadbox migrate         Run pending database migrations
breadbox version         Print version
```

## Documentation

- [Architecture](docs/architecture.md) -- system design, provider interface, deployment
- [Data Model](docs/data-model.md) -- database schema, enums, relationships
- [REST API](docs/rest-api.md) -- endpoints, authentication, request/response formats
- [MCP Server](docs/mcp-server.md) -- tools, resources, usage
- [Plaid Integration](docs/plaid-integration.md) -- Plaid setup and sync details
- [Teller Integration](docs/teller-integration.md) -- Teller mTLS setup
- [CSV Import](docs/csv-import.md) -- CSV file format and import behavior
- [Admin Dashboard](docs/admin-dashboard.md) -- dashboard pages and features
- [Design System](docs/design-system.md) -- UI framework and component reference
- [Backup & Restore](docs/backup.md) -- database backup strategies

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, testing, and guidelines.

## License

[AGPL-3.0](LICENSE)
