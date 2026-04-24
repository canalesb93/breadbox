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

## Installation

### Docker (recommended)

No need to clone the repo. This pulls the pre-built image from GHCR and starts Breadbox with PostgreSQL:

```bash
mkdir breadbox && cd breadbox
curl -O https://raw.githubusercontent.com/canalesb93/breadbox/main/deploy/docker-compose.prod.yml
mv docker-compose.prod.yml docker-compose.yml
```

Create a `.env` file with required configuration:

```bash
cat > .env <<EOF
DATABASE_URL=postgres://breadbox:breadbox@db:5432/breadbox?sslmode=disable
ENCRYPTION_KEY=$(openssl rand -hex 32)
SERVER_PORT=8080
ENVIRONMENT=docker
POSTGRES_USER=breadbox
POSTGRES_PASSWORD=$(openssl rand -base64 32 | tr -d '=/+')
POSTGRES_DB=breadbox
EOF
```

Start the services:

```bash
docker compose up -d breadbox db
# Visit http://localhost:8080 to start the setup wizard
```

> **Note:** This starts Breadbox and PostgreSQL without the Caddy reverse proxy, exposing port 8080 directly. For production with automatic HTTPS, run `docker compose up -d` (all services) and configure your domain in the Caddyfile -- see [Production Deployment](#production-deployment) below.

To pin a specific version instead of `latest`, edit `docker-compose.yml` and change the image tag:

```yaml
image: ghcr.io/canalesb93/breadbox:v0.1.0
```

### Binary download

Pre-built binaries for Linux and macOS (amd64/arm64) are available on the [GitHub Releases](https://github.com/canalesb93/breadbox/releases) page.

```bash
# Download the binary for your platform (example: Linux amd64)
curl -fsSL https://github.com/canalesb93/breadbox/releases/latest/download/breadbox-linux-amd64 -o breadbox
chmod +x breadbox

# Requires a running PostgreSQL instance
export DATABASE_URL="postgres://user:pass@localhost:5432/breadbox?sslmode=disable"
export ENCRYPTION_KEY="$(openssl rand -hex 32)"

./breadbox serve
# Visit http://localhost:8080
```

### Go install

Build from source using Go:

```bash
git clone https://github.com/canalesb93/breadbox.git && cd breadbox
go install ./cmd/breadbox

# Requires a running PostgreSQL instance
export DATABASE_URL="postgres://user:pass@localhost:5432/breadbox?sslmode=disable"
export ENCRYPTION_KEY="$(openssl rand -hex 32)"

breadbox serve
# Visit http://localhost:8080
```

> **Note:** The module path is `breadbox`, so `go install github.com/...@latest` is not supported. Clone the repo and install locally.

### From source (development)

```bash
git clone https://github.com/canalesb93/breadbox.git && cd breadbox

# Requires Go 1.24+ and PostgreSQL 16+
make dev

# Visit http://localhost:8080
```

### Production deployment

For production with automatic HTTPS via Caddy, use the one-liner install script:

```bash
curl -fsSL https://raw.githubusercontent.com/canalesb93/breadbox/main/deploy/install.sh | sudo bash
```

This installs Docker if needed, downloads the deployment files, generates secrets, and starts Breadbox with Caddy for automatic TLS. See [`deploy/`](deploy/) for details.

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
breadbox doctor          Validate config and connectivity without booting the server
breadbox version         Print version
```

Run `breadbox doctor` if the server fails to start or something feels off — it
surfaces bad `DATABASE_URL`/`ENCRYPTION_KEY`, missing migrations, unreadable
Teller certs, a missing admin account, and unreachable `PUBLIC_URL` in a single
pass. See [Doctor](docs/doctor.md) for the full check list and `--json` /
`--skip-external` flags.

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
- [Doctor](docs/doctor.md) -- `breadbox doctor` pre-flight / readiness check

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, testing, and guidelines.

## License

[AGPL-3.0](LICENSE)
