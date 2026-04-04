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

## Command Structure

```
breadbox <command> <subcommand> [flags]
```

### Connection & Auth

```bash
# Configure which server to talk to (writes ~/.config/breadbox/config.yaml)
breadbox auth login --server https://breadbox.example.com --api-key bb_xxxxx
breadbox auth status          # Show current server + key info
breadbox auth switch <name>   # Switch between saved profiles
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
  # ... existing packages (api/, service/, etc.) only imported by breadboxd
```

### CLI Framework

Adopt **[cobra](https://github.com/spf13/cobra)** for the CLI. Reasons:

- Industry standard (`gh`, `kubectl`, `docker` all use it)
- Subcommand nesting, flag parsing, help generation, shell completions out of the box
- `breadboxd` can import and mount the CLI's cobra command tree alongside its server commands

### HTTP Client

A thin wrapper around `net/http` that handles:

- Base URL + API key from active profile
- JSON marshaling/unmarshaling
- Error envelope parsing (`{"error": {"code": "...", "message": "..."}}`)
- Pagination (cursor-based, follows `next_cursor` automatically when `--limit` exceeds page size)
- Per-request `--server` and `--api-key` flag overrides (skip config file for scripting)

```go
// internal/cli/client.go
type Client struct {
    BaseURL    string
    APIKey     string
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
    api_key: bb_xxxxxxxxxxxxx
  work:
    server: http://localhost:8080
    api_key: bb_yyyyyyyyyyyyy
```

Profiles support the multi-instance model — a user might have one family Breadbox and one shared with a partner, each on different servers.

### Output Formatting

Three modes, consistent across all commands:

| Flag | Format | Use Case |
|------|--------|----------|
| *(default)* | Aligned table | Human reading |
| `--json` | JSON array/object | Piping to `jq`, scripts |
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

**Phase 1 — Foundation**: Rename server binary to `breadboxd`, create `cmd/breadbox/` for CLI, `auth login/status/switch`, HTTP client, config file, output formatter

**Phase 2 — Read commands**: `tx list/view/search/summary/merchants`, `category list`, `account list`, `connection list/status`, `rule list/view`, `review list`

**Phase 3 — Write commands**: `tx categorize`, `rule create/update/delete/apply`, `review approve/reject`, `report submit`

**Phase 4 — Polish**: shell completions, `--interactive` mode for reviews, CI release pipeline for both binaries, install script

## Open Questions

- **Server binary rename**: `breadboxd` is the obvious choice (follows `dockerd`, `containerd` convention). Any other preferences?
- **Interactive mode?** `breadbox review` without subcommands could enter an interactive TUI for triaging reviews one by one.
- **Piping patterns?** Should `breadbox tx list --json | breadbox tx categorize --stdin` be a pattern we design for explicitly?
- **Auth flow**: For hosted instances, should we support OAuth/browser-based login in addition to API key pasting? e.g., `breadbox auth login` opens a browser, user approves, CLI receives a token.
