# Security Policy

Breadbox handles bank credentials, financial transactions, and AI-agent access tokens. We take security seriously and appreciate responsible disclosure.

## Supported Versions

During the early-access period, only the latest `v0.1.x` release receives security fixes.

| Version  | Supported          |
| -------- | ------------------ |
| 0.1.x    | :white_check_mark: |
| < 0.1.0  | :x:                |

Once Breadbox reaches `v1.0.0`, this policy will be revisited.

## Reporting a Vulnerability

**Please do not file a public GitHub issue for security reports.**

Preferred channel:
[**Open a private security advisory**](https://github.com/canalesb93/breadbox/security/advisories/new)
on the repository. Advisories are visible only to the maintainers and to anyone you choose to add.

Email fallback:
`canalesb93@gmail.com` — encrypt sensitive payloads with the maintainer's GitHub-published GPG key if available.

When reporting, please include:

- A description of the vulnerability and its impact.
- Steps to reproduce, or a proof-of-concept.
- The Breadbox version, commit SHA, deployment mode (Docker / binary / source), and any relevant configuration.

We will acknowledge receipt within five business days and aim to provide a remediation plan within fifteen. During early access this is best-effort — we are a small project.

## Scope

In scope:

- The Breadbox binary and its bundled admin dashboard, REST API, and MCP server.
- The provided `deploy/install.sh`, `docker-compose.prod.yml`, and `Caddyfile`.
- Authentication, authorization, encryption-at-rest, and credential storage.
- Server-side input validation in REST and MCP tool handlers.

Out of scope:

- Vulnerabilities in upstream dependencies (Plaid, Teller, PostgreSQL, Caddy, Go runtime) — please report those upstream. We will track and update once a fix lands.
- Denial-of-service against your own self-hosted instance.
- Issues that require physical access to the host running Breadbox.
- Social-engineering attacks against household members.
- Missing best-practice headers or hardening that does not lead to a concrete vulnerability (we welcome these as regular issues or PRs).

## Disclosure

We will publish a security advisory once a fix is released. Reporters who follow this policy and wish to be credited will be acknowledged in the advisory and `CHANGELOG.md`.

Thank you for helping keep Breadbox and its users safe.
