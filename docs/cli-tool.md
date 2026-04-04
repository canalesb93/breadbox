# Breadbox CLI Tool Design

## Overview

`breadbox` already has server-side commands (`serve`, `migrate`, `create-admin`, etc.). This design adds **client-side commands** that talk to a running Breadbox instance via its REST API, giving users a `gh`-like experience for managing their financial data from the terminal.

## Goals

- Provide a full-featured alternative to the MCP path for power users
- Follow the `gh` CLI pattern: `breadbox <noun> <verb>` with consistent flags
- Extensible command structure — easy to add new resource types
- Works against any Breadbox instance (local or remote)
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

### Server Admin (Direct DB, No API Key Needed)

These existing commands stay as-is — they connect to the database directly:

```bash
breadbox serve
breadbox migrate
breadbox create-admin
breadbox reset-password
breadbox api-keys list|create
breadbox version
```

## Architecture

### Single Binary, Two Modes

The `breadbox` binary already serves as the server. Client commands live in the same binary — no separate install. The binary detects mode by command name:

- **Server commands** (`serve`, `migrate`, `create-admin`): connect to DB directly
- **Client commands** (`tx`, `category`, `rule`, etc.): talk to REST API via HTTP

```
cmd/breadbox/
  main.go           # top-level command dispatch
  admin.go           # existing: create-admin, reset-password
  client.go          # NEW: HTTP client, config loading
  cmd_tx.go          # NEW: transaction subcommands
  cmd_category.go    # NEW: category subcommands
  cmd_rule.go        # NEW: rule subcommands
  cmd_review.go      # NEW: review subcommands
  cmd_account.go     # NEW: account/connection subcommands
  cmd_auth.go        # NEW: auth/config subcommands
  cmd_report.go      # NEW: report subcommands
```

### CLI Framework

Adopt **[cobra](https://github.com/spf13/cobra)** for client commands. Reasons:

- Industry standard (`gh`, `kubectl`, `docker` all use it)
- Subcommand nesting, flag parsing, help generation, shell completions out of the box
- Can migrate existing server commands incrementally (not required upfront)

### HTTP Client

A thin wrapper around `net/http` that handles:

- Base URL + API key from config
- JSON marshaling/unmarshaling
- Error envelope parsing (`{"error": {"code": "...", "message": "..."}}`)
- Pagination (cursor-based, follows `next_cursor` automatically when `--limit` exceeds page size)

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

## Extension Points

### Adding a New Resource

1. Create `cmd_<resource>.go` with cobra commands
2. Register root command in `main.go`
3. Add any new API calls to the client

Each resource file is self-contained — no need to touch shared code.

### Plugins (Future)

Follow `gh`'s extension model: executables named `breadbox-<name>` on `$PATH` are discovered automatically and available as `breadbox <name>`. Not needed for v1, but the cobra structure supports it naturally.

### Shell Completions

Cobra generates these for free:

```bash
breadbox completion bash|zsh|fish|powershell
```

## Implementation Plan

**Phase 1 — Foundation**: `auth`, HTTP client, config file, output formatter, `tx list/view/search`

**Phase 2 — Read commands**: `category list`, `account list`, `connection list/status`, `rule list/view`, `review list`

**Phase 3 — Write commands**: `tx categorize`, `rule create/update/delete/apply`, `review approve/reject`, `report submit`

**Phase 4 — Polish**: shell completions, `--interactive` mode for reviews, `breadbox sync` shorthand, man pages

## Open Questions

- **Separate binary?** Current design puts client commands in the same binary. An alternative is a separate `bb` binary (shorter to type, smaller download for users who don't self-host). We could do both — `bb` as a lightweight alias.
- **Interactive mode?** `breadbox review` without subcommands could enter an interactive TUI for triaging reviews one by one.
- **Piping patterns?** Should `breadbox tx list --json | breadbox tx categorize --stdin` be a pattern we design for explicitly?
