# Changelog

All notable changes to Breadbox will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/), and this project adheres to [Semantic Versioning](https://semver.org/).

## [0.1.0] - 2026-04-XX

### Added
- Bank sync via Plaid, Teller, and CSV import
- REST API for transaction queries, categories, rules, and account management
- MCP server (Streamable HTTP + stdio) for AI agent integration
- Admin dashboard with DaisyUI 5 + Alpine.js
- Transaction rules engine with recursive AND/OR/NOT conditions
- Review queue for transaction triage (auto-enqueue during sync)
- Account linking for cross-connection transaction deduplication
- Multi-user household support (admin + family members)
- Category system with 2-level hierarchy and slug-based identification
- Field selection on queries for response size control
- Transaction and merchant summary aggregations
- Agent reports for AI agents to submit findings
- API key authentication with scoped access (full/read-only)
- MCP permissions (read-only/read-write mode, per-tool enable/disable)
- AES-256-GCM encryption for provider credentials at rest
- Docker deployment with Caddy auto-HTTPS
- CLI tool (`breadbox serve`, `breadbox create-admin`, `breadbox mcp-stdio`)
