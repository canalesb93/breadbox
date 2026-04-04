# Breadbox CLI Tool Design

## Overview

A standalone CLI tool that talks to any Breadbox server instance via its REST API, giving users a `gh`-like experience for managing their financial data from the terminal. Designed for a deployment model where Breadbox instances may be hosted centrally while users interact from their own machines.

## Goals

- **Standalone distribution** — users download the CLI without needing the server
- Provide a full-featured alternative to the MCP path for power users
- Follow the `gh` CLI pattern: `breadbox <noun> <verb>` with consistent flags
- **Multi-instance support** — connect to different Breadbox instances (family, work, etc.)
- Extensible command structure — easy to add new resource types
- Output formats: human-readable tables (default), JSON (`--json`), TSV (`--tsv`)

## Binary Rename

The current `breadbox` binary becomes `breadboxd` (the server daemon). The `breadbox` name is reclaimed for the CLI.

| Before | After | Role |
|--------|-------|------|
| `cmd/breadbox/` | `cmd/breadboxd/` | Server daemon (serve, migrate, create-admin, etc.) |
| *(new)* | `cmd/breadbox/` | Standalone CLI client |

**Migration**: existing deployments that reference `breadbox serve` in systemd units, Docker entrypoints, or scripts will need to update to `breadboxd serve`. The Docker image entrypoint and `deploy/` scripts should be updated as part of the rename.

## Command Structure

```
breadbox <command> <subcommand> [flags]
```

### Connection & Auth

```bash
# Browser-based OAuth (default for humans)
breadbox auth login --server https://breadbox.example.com
# → Opens browser to authorize, saves token to profile

# Device code flow (agents, SSH sessions, headless environments)
breadbox auth login --server https://breadbox.example.com
# → When no browser detected:
#   Visit: https://breadbox.example.com/oauth/device
#   Enter code: ABCD-1234
#   Waiting for approval... ✓

# API key (scripts, CI, agents with pre-provisioned keys)
breadbox auth login --server https://breadbox.example.com --api-key bb_xxxxx
# Or via env var (no config file needed):
BREADBOX_API_KEY=bb_xxxxx BREADBOX_SERVER=https://... breadbox tx list

breadbox auth status          # Show current server + key info + token expiry
breadbox auth switch <name>   # Switch between saved profiles
breadbox auth logout          # Remove current profile's credentials
```

### Transactions

```bash
breadbox tx list [--user <name>] [--account <id>] [--category <slug>]
                 [--from <date>] [--to <date>] [--search <term>]
                 [--min <amt>] [--max <amt>] [--limit <n>]
                 [--sort date|amount|name] [--order asc|desc]
                 [--json] [--fields <fields>]

breadbox tx view <id>
breadbox tx categorize <id> <category-slug>
breadbox tx categorize --search "Costco" --category groceries   # bulk by filter
breadbox tx uncategorize <id>
breadbox tx search "coffee"                                      # shorthand for list --search
breadbox tx summary [--group-by category|month|week|day|category_month]
breadbox tx merchants [--min-count 2] [--spending-only]
```

### Categories

```bash
breadbox category list [--tree]
breadbox category create <slug> --name "Display Name" [--parent <slug>] [--icon <name>] [--color <hex>]
breadbox category update <slug> --name "New Name"
breadbox category delete <slug>
breadbox category merge <source-slug> --into <target-slug>
breadbox category export > categories.tsv
breadbox category import < categories.tsv
```

### Rules

```bash
breadbox rule list [--category <slug>] [--enabled] [--search <term>]
breadbox rule create --name "Costco → Groceries" \
  --condition '{"field":"name","op":"contains","value":"COSTCO"}' \
  --category groceries [--priority 10] [--apply-retroactively]
breadbox rule view <id>
breadbox rule update <id> --enabled=false
breadbox rule delete <id>
breadbox rule apply <id>         # apply one rule retroactively
breadbox rule apply-all          # apply all active rules
breadbox rule preview --condition '{"field":"name","op":"contains","value":"COSTCO"}'
```

### Reviews

```bash
breadbox review list [--status pending|approved|rejected|skipped] [--type <type>]
breadbox review view <id>
breadbox review approve <id> [--category <slug>] [--comment "..."]
breadbox review reject <id> [--comment "..."]
breadbox review skip <id>
breadbox review bulk-approve --type uncategorized --category groceries
```

### Accounts & Connections

```bash
breadbox account list [--user <name>] [--type checking|savings|credit]
breadbox account view <id>

breadbox connection list [--user <name>] [--status active|error]
breadbox connection sync <id>        # trigger manual sync
breadbox connection sync-all
breadbox connection status           # sync status overview
```

### Users

```bash
breadbox user list
```

### Reports

```bash
breadbox report list [--unread]
breadbox report view <id>
breadbox report submit --title "Monthly Review" --body "Everything looks good."
breadbox report mark-read <id>
```

## Architecture

### Two Binaries, Shared Code

The CLI and server are **separately distributable** but live in the same Go module. Users who self-host get both; users connecting to a hosted instance only need the CLI.

| Binary | Command | What it does | Requires |
|--------|---------|-------------|----------|
| `breadbox` | CLI client | Talks to REST API over HTTP | Nothing (standalone) |
| `breadboxd` | Server daemon | Runs the server, DB, sync, admin UI | PostgreSQL, config |

The server binary (`breadboxd`) also embeds all CLI commands for convenience — self-hosters can use `breadboxd tx list` without installing the CLI separately. But the CLI binary has zero server dependencies (no DB driver, no sync engine, no templates).

```
cmd/
  breadbox/          # CLI binary (lightweight, HTTP-only)
    main.go          # cobra root command
    cmd_auth.go      # auth login/status/switch
    cmd_tx.go        # transaction subcommands
    cmd_category.go  # category subcommands
    cmd_rule.go      # rule subcommands
    cmd_review.go    # review subcommands
    cmd_account.go   # account/connection subcommands
    cmd_report.go    # report subcommands
  breadboxd/         # Server binary (current cmd/breadbox/, renamed)
    main.go          # serve, migrate, create-admin, etc. + embeds CLI commands
    admin.go         # existing: create-admin, reset-password

internal/
  cli/               # NEW: shared CLI library (used by both binaries)
    client.go        # HTTP client wrapper
    config.go        # Config file loading (~/.config/breadbox/)
    output.go        # Table/JSON/TSV formatter
    auth.go          # OAuth + device code flow + API key auth
  # ... existing packages (api/, service/, etc.) only imported by breadboxd
```

### CLI Framework

Adopt **[cobra](https://github.com/spf13/cobra)** for the CLI. Reasons:

- Industry standard (`gh`, `kubectl`, `docker` all use it)
- Subcommand nesting, flag parsing, help generation, shell completions out of the box
- `breadboxd` can import and mount the CLI's cobra command tree alongside its server commands

### Authentication

Three methods, in order of preference:

**1. OAuth with browser (default for humans)**

Standard authorization code flow. CLI starts a temporary local HTTP server on a random port to receive the callback.

```
breadbox auth login --server https://breadbox.example.com
→ Opening browser to https://breadbox.example.com/oauth/authorize?...
→ Waiting for authorization... ✓
→ Logged in as Ricardo. Token saved to profile "home".
```

**2. Device code flow (agents + headless environments)**

When no browser is available (detected via `os.Getenv("DISPLAY")` / `os.Getenv("BROWSER")` / TTY check), falls back to device code flow. The CLI prints a URL and short code — agents can present this to users in whatever interface they're running in (chat window, terminal, Slack, etc.).

```
breadbox auth login --server https://breadbox.example.com
→ No browser detected. To authenticate:
→
→   Visit:  https://breadbox.example.com/oauth/device
→   Code:   ABCD-1234
→
→ Waiting for approval... ✓
→ Logged in as Ricardo. Token saved to profile "home".
```

Agents can also force this mode explicitly:

```bash
breadbox auth login --server https://... --device-code
```

The device code + URL are also available as structured output for programmatic use:

```bash
breadbox auth login --server https://... --device-code --json
# {"verification_url": "https://...", "user_code": "ABCD-1234", "expires_in": 900}
```

**3. API key (scripts, CI, pre-provisioned agents)**

Direct API key auth, no OAuth round-trip. Works via flag or env var:

```bash
# Via flag (saved to config)
breadbox auth login --server https://... --api-key bb_xxxxx

# Via env vars (ephemeral, no config file needed)
BREADBOX_SERVER=https://... BREADBOX_API_KEY=bb_xxxxx breadbox tx list
```

**Precedence**: env vars > flags > config file profile.

**Token storage**: OAuth tokens and API keys stored in `~/.config/breadbox/config.yaml`. File permissions set to `0600`. Tokens include refresh tokens for silent renewal.

### HTTP Client

A thin wrapper around `net/http` that handles:

- Base URL + credentials from active profile (or env var overrides)
- JSON marshaling/unmarshaling
- Error envelope parsing (`{"error": {"code": "...", "message": "..."}}`)
- Pagination (cursor-based, follows `next_cursor` automatically when `--limit` exceeds page size)
- OAuth token refresh (transparent, using stored refresh token)
- Per-request `--server` and `--api-key` flag overrides (skip config file for scripting)

```go
// internal/cli/client.go
type Client struct {
    BaseURL    string
    APIKey     string
    OAuthToken *OAuthToken // access_token + refresh_token + expiry
    HTTPClient *http.Client
}

func (c *Client) Get(ctx context.Context, path string, params url.Values, out any) error
func (c *Client) Post(ctx context.Context, path string, body any, out any) error
func (c *Client) Put(ctx context.Context, path string, body any, out any) error
func (c *Client) Patch(ctx context.Context, path string, body any, out any) error
func (c *Client) Delete(ctx context.Context, path string, out any) error
```

### Config File

```yaml
# ~/.config/breadbox/config.yaml
current_profile: home

profiles:
  home:
    server: https://breadbox.example.com
    # OAuth token (from browser or device code login)
    oauth_token: eyJhbGci...
    oauth_refresh_token: dGhpcyBp...
    oauth_expiry: 2026-05-01T00:00:00Z
  work:
    server: http://localhost:8080
    # API key (from --api-key login)
    api_key: bb_yyyyyyyyyyyyy
```

Profiles support the multi-instance model — a user might have one family Breadbox and one shared with a partner, each on different servers. OAuth and API key auth can be mixed across profiles.

### Output Formatting

Three modes, consistent across all commands:

| Flag | Format | Use Case |
|------|--------|----------|
| *(default)* | Aligned table | Human reading |
| `--json` | JSON array/object | Piping to `jq`, scripts, agents |
| `--tsv` | Tab-separated | Spreadsheets, `awk` |

The `--fields` flag controls which columns appear (mirrors the API's `fields` param).

```go
// internal/cli/output.go
type Formatter struct {
    Format string // "table", "json", "tsv"
    Fields []string
    Writer io.Writer
}
```

## Distribution

### Install Methods

```bash
# Homebrew (macOS/Linux)
brew install canalesb93/tap/breadbox

# Go install
go install github.com/canalesb93/breadbox/cmd/breadbox@latest

# Binary download (GitHub Releases)
curl -fsSL https://breadbox.example.com/install.sh | sh
```

### Release Artifacts

Each GitHub release publishes:

| Artifact | Contents | Who needs it |
|----------|----------|-------------|
| `breadbox-cli-{os}-{arch}` | CLI binary only | End users connecting to hosted instances |
| `breadboxd-{os}-{arch}` | Server binary (includes CLI) | Self-hosters |
| `breadbox-server` Docker image | Server + migrations | Docker/Kubernetes deployments |

The CLI binary should be small (few MB, no CGo) since it's just HTTP + cobra + formatting.

## Extension Points

### Adding a New Resource

1. Create `cmd_<resource>.go` in `cmd/breadbox/` with cobra commands
2. Register under root command in `main.go`
3. Add any new API calls to the `internal/cli` client

Each resource file is self-contained — no need to touch shared code.

### Plugins (Future)

Follow `gh`'s extension model: executables named `breadbox-<name>` on `$PATH` are discovered automatically and available as `breadbox <name>`. Not needed for v1, but the cobra structure supports it naturally.

### Shell Completions

Cobra generates these for free:

```bash
breadbox completion bash|zsh|fish|powershell
```

## Implementation Plan

**Phase 1 — Foundation**: Rename `cmd/breadbox/` to `cmd/breadboxd/`, update Makefile/Dockerfile/deploy scripts. Create `cmd/breadbox/` for CLI. HTTP client, config file, output formatter, `auth login/status/switch`.

**Phase 2 — Read commands**: `tx list/view/search/summary/merchants`, `category list`, `account list`, `connection list/status`, `rule list/view`, `review list`

**Phase 3 — Write commands**: `tx categorize`, `rule create/update/delete/apply`, `review approve/reject`, `report submit`

**Phase 4 — Auth**: OAuth authorization code flow, device code flow, token refresh. Server-side OAuth endpoints if not already sufficient.

**Phase 5 — Polish**: shell completions, CI release pipeline for both binaries, install script
