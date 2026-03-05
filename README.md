# Breadbox

Self-hosted financial data aggregation for families. Syncs bank data via [Plaid](https://plaid.com/) and [Teller](https://teller.io/), stores it in PostgreSQL, and exposes it through a REST API and MCP server for AI agents.

## Tech Stack

- **Go 1.24+** -- single binary, all components in one process
- **PostgreSQL 16+** -- primary data store
- **chi/v5** -- HTTP router
- **pgx/v5 + sqlc** -- database driver and query generation
- **goose** -- schema migrations
- **robfig/cron** -- scheduled sync engine
- **Pico CSS + Alpine.js** -- admin dashboard (no build step)
- **MCP server** -- Streamable HTTP at `/mcp`, stdio via `breadbox mcp-stdio`

## Prerequisites

- Go 1.24+
- PostgreSQL 16+
- Docker and Docker Compose (optional, for containerized deployment)

## Docker Quickstart (Recommended)

```bash
cp .env.example .docker.env

# Edit .docker.env:
#   - Set ENCRYPTION_KEY (generate with: openssl rand -hex 32)
#   - Configure Plaid or Teller credentials (or skip and use the setup wizard)

docker compose up -d

# Visit http://localhost:8080 to start the setup wizard
```

The `docker-compose.yml` runs both the app and a PostgreSQL 16 container. Migrations run automatically on startup.

## Manual Install

```bash
# Clone and build
git clone <repo-url> && cd breadbox
go build -o breadbox ./cmd/breadbox

# Create a PostgreSQL database
createdb breadbox

# Configure environment
cp .env.example .local.env
# Edit .local.env: set DATABASE_URL, ENCRYPTION_KEY, provider credentials

# Run migrations and start the server
./breadbox migrate
./breadbox serve
```

The server starts on port 8080 by default. Visit `http://localhost:8080` to open the setup wizard.

## Configuration Reference

All configuration is via environment variables. Copy `.env.example` to `.local.env` (local dev) or `.docker.env` (Docker).

| Variable | Description | Default | Required |
|---|---|---|---|
| `DATABASE_URL` | PostgreSQL connection string | `postgres://breadbox:breadbox@localhost:5432/breadbox?sslmode=disable` | Yes |
| `ENCRYPTION_KEY` | 32-byte hex key for AES-256-GCM encryption of bank tokens. Generate with `openssl rand -hex 32` | -- | Yes (when using Plaid/Teller) |
| `SERVER_PORT` | HTTP listen port | `8080` | No |
| `ENVIRONMENT` | `local` or `docker`. Controls log format and env file loading | `local` | No |
| `LOG_LEVEL` | Log verbosity: `debug`, `info`, `warn`, `error` (case-insensitive). Overrides environment-based defaults | -- | No |
| `PLAID_CLIENT_ID` | Plaid API client ID (also configurable via setup wizard) | -- | No |
| `PLAID_SECRET` | Plaid API secret | -- | No |
| `PLAID_ENV` | Plaid environment: `sandbox`, `development`, `production` | `sandbox` | No |
| `TELLER_APP_ID` | Teller application ID (also configurable via admin settings) | -- | No |
| `TELLER_CERT_PATH` | Path to Teller mTLS certificate file | -- | No |
| `TELLER_KEY_PATH` | Path to Teller mTLS private key file | -- | No |
| `TELLER_ENV` | Teller environment: `sandbox`, `development`, `production` | `sandbox` | No |
| `TELLER_WEBHOOK_SECRET` | Teller webhook signing secret | -- | No |
| `SYNC_TIMEOUT_SECONDS` | Per-connection sync timeout | `300` | No |
| `DB_MAX_CONNS` | Maximum database pool connections | `25` | No |
| `DB_MIN_CONNS` | Minimum database pool connections | `2` | No |
| `DB_MAX_CONN_LIFETIME_MINUTES` | Maximum connection lifetime | `60` | No |
| `HTTP_READ_TIMEOUT_SECONDS` | HTTP server read timeout | `30` | No |
| `HTTP_WRITE_TIMEOUT_SECONDS` | HTTP server write timeout | `60` | No |
| `HTTP_IDLE_TIMEOUT_SECONDS` | HTTP server idle timeout | `120` | No |

Environment variables take precedence over values stored in the `app_config` database table, which take precedence over defaults.

## First-Run Walkthrough

On first launch, Breadbox presents a setup wizard at the root URL. The wizard has six steps:

1. **Admin Account** -- create the administrator username and password.
2. **Bank Providers** -- choose Plaid, Teller, both, or skip. Enter API credentials.
3. **Family Member** -- create your first family member (name and email). This prevents an empty member dropdown when connecting a bank.
4. **Sync Interval** -- set how often Breadbox automatically syncs bank data.
5. **Webhooks** -- configure webhook URLs for real-time updates from providers (optional).
6. **Review** -- confirm settings and complete setup.

After setup:

- **Add family members** via the admin dashboard under Members.
- **Connect a bank** by navigating to Connections and linking an account through Plaid Link or Teller Connect.

## CLI Commands

```
breadbox serve            Start the HTTP server (API, MCP, admin dashboard, webhooks, cron)
breadbox migrate          Run pending database migrations
breadbox seed             Insert sandbox test data (development only)
breadbox mcp-stdio        Start MCP server on stdin/stdout (for local AI agent dev)
breadbox api-keys          Manage API keys
  api-keys list            List all API keys
  api-keys create <name>   Create a new API key
breadbox reset-password   Reset an admin user's password
breadbox version          Print build version and exit
```

## MCP Integration

Breadbox exposes financial data to AI agents via the [Model Context Protocol](https://modelcontextprotocol.io/).

**Streamable HTTP** (production): The MCP endpoint is available at `/mcp` when running `breadbox serve`. Authenticate with an API key in the `X-API-Key` header.

**Stdio** (local development): Run `breadbox mcp-stdio` for a stdio-based MCP server that reads/writes the MCP protocol on stdin/stdout. No authentication required.

### Claude Desktop

Add to your Claude Desktop MCP config:

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

### Claude Code

Add to your Claude Code MCP settings:

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

## Documentation

Detailed documentation lives in the `docs/` directory:

- [Architecture and Deployment](docs/architecture.md) -- system design, provider interface, configuration, security, Docker deployment
- [Data Model](docs/data-model.md) -- database schema, enums, relationships
- [REST API](docs/rest-api.md) -- API endpoints, authentication, request/response formats
- [MCP Server](docs/mcp-server.md) -- MCP tools, schemas, usage examples
- [Plaid Integration](docs/plaid-integration.md) -- Plaid provider details
- [Teller Integration](docs/teller-integration.md) -- Teller provider details, mTLS setup
- [CSV Import](docs/csv-import.md) -- CSV file import format and behavior
- [Admin Dashboard](docs/admin-dashboard.md) -- dashboard pages and features
- [Backup and Restore](docs/backup.md) -- database backup strategies and restore procedures
