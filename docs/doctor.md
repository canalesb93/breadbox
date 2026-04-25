# `breadbox doctor`

A pre-flight + readiness command that validates configuration and connectivity
**without booting the server**. Use it as the first diagnostic when something
"feels off", and let install scripts gate on its exit code.

```
breadbox doctor [--json] [--skip-external]
```

- Exit code `0` — all checks pass (or were skipped).
- Exit code `1` — one or more checks failed. Remediation hints are printed next
  to each failure.

## Flags

| Flag | Description |
| --- | --- |
| `--json` | Emit a structured `{"checks": [...], "ok": bool}` document instead of a pretty table. Useful in CI and install scripts. |
| `--skip-external` | Skip DNS and outbound HTTP checks (DNS lookup + `GET /health/ready` against `PUBLIC_URL`/`DOMAIN`). Use this on air-gapped installs or when the box is still behind a firewall. |

## Checks

| Name | What it validates |
| --- | --- |
| `config load` | `.local.env` / `.docker.env` parse cleanly and required env vars decode. |
| `database` | `DATABASE_URL` is set, Postgres accepts a connection, and `Ping` succeeds. |
| `migrations` | The `goose_db_version` in the DB matches the highest numeric prefix among embedded migrations. Reports "behind" (needs `breadbox migrate`) or "ahead" (binary downgrade). |
| `encryption key` | A 32-byte AES key is available, sourced from one of: `ENCRYPTION_KEY` env (BYO), `${BREADBOX_DATA_DIR}/encryption.key` (auto-managed file), or freshly generated this boot. Reports `source=…` and a stable 8-char SHA-256 fingerprint. |
| `plaid` | When `PLAID_CLIENT_ID` is set: `PLAID_SECRET` is present and `PLAID_ENV` is one of `sandbox`/`development`/`production`. |
| `teller` | When `TELLER_APP_ID` is set: `TELLER_ENV` is valid, cert/key paths (if any) exist and are readable, or PEM bytes are available via `app_config`. |
| `provider credentials` | Walks every non-disconnected `bank_connections` row and verifies `encrypted_credentials` decrypts with the current key. Catches silent `ENCRYPTION_KEY` rotations. |
| `admin account` | `auth_accounts` contains at least one row — otherwise the setup wizard hasn't run. |
| `scheduler` | `sync_interval_minutes` is positive; any `BACKUP_CRON` / `SYNC_CRON` env value parses as a 5-field cron expression. |
| `public url` | When `PUBLIC_URL` or `DOMAIN` is set (and `--skip-external` isn't): DNS resolves and `GET /health/ready` returns < 400. |

> **Persisted-fingerprint vs live-key comparison** is still tracked under
> issue #688 as Phase 2/3 work. With the auto-managed key file in place,
> `doctor` already reports the live fingerprint; once the setup wizard records
> a fingerprint at first boot, the check can compare the two and surface
> silent rotations even before any bank connection exists. Until then, the
> credential-decrypt walk is the proxy.

## Example output

```text
     Check                 Status
  -  --------------------  ----------------------------------------
  ✓  config load           environment=local
  ✓  database              connected and reachable
  ✓  migrations            up-to-date (version 20260417032707)
  ✓  encryption key        set and 32 bytes
  ✓  plaid                 client/secret set, env=sandbox
  ✓  teller                cert/key present, env=sandbox
  ✓  provider credentials  3 connection(s) decrypt cleanly
  ✓  admin account         1 account(s) present
  ✓  scheduler             sync every 720m; cron expressions valid
  ⊘  public url            neither PUBLIC_URL nor DOMAIN set

OK — all checks passed.
```

## CI / install-script use

```bash
breadbox doctor --json --skip-external > doctor.json
jq -e '.ok == true' doctor.json
```

A future one-liner installer (issue #686) is expected to run `breadbox doctor`
after launching the process to surface configuration problems before handing
the operator a URL. This PR does not yet wire doctor into `install.sh`.

## What doctor intentionally does **not** do

- **Does not mutate state.** Read-only across the board. Running it repeatedly
  is safe.
- **Does not boot the HTTP server.** Use `breadbox serve` for that — doctor is
  designed to diagnose startup failures without needing startup to succeed.
- **Does not probe provider APIs.** A healthy Plaid env var doesn't guarantee
  the credentials are valid with Plaid itself; the credential-decrypt walk is
  strictly a local AES check.
