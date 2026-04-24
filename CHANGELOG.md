# Changelog

All notable changes to Breadbox will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/), and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Breaking Changes (Pre-1.0)

- **Renamed provider-data fields on transactions table.** Raw data fields from bank providers are now prefixed with `provider_` to clarify they are unmodified provider data, not Breadbox assignments. This affects:
  - Database columns: `external_transaction_id` → `provider_transaction_id`, `pending_transaction_id` → `provider_pending_transaction_id`, `name` → `provider_name`, `merchant_name` → `provider_merchant_name`, `category_primary` → `provider_category_primary`, `category_detailed` → `provider_category_detailed`, `category_confidence` → `provider_category_confidence`, `payment_channel` → `provider_payment_channel`
  - REST API / MCP response keys: `name` → `provider_name`, `merchant_name` → `provider_merchant_name`, `category_primary_raw` → `provider_category_primary`, `category_detailed_raw` → `provider_category_detailed`, `category_confidence` → `provider_category_confidence`, `payment_channel` → `provider_payment_channel`
  - Rule DSL condition field identifiers: `name` → `provider_name`, `merchant_name` → `provider_merchant_name`, `category_primary` → `provider_category_primary`, `category_detailed` → `provider_category_detailed` (assignment `category` field unchanged)
  - Field selector aliases: `minimal` expands to `provider_name, amount, date` (was `name, amount, date`); `core` expands to `id, date, amount, provider_name, iso_currency_code`; `category` expands to `category, provider_category_primary, provider_category_detailed`
  - Sort parameter: `sort_by=name` → `sort_by=provider_name` (`date`, `amount` unchanged)
  - New `provider_raw JSONB` column stores the unmodified provider payload for each transaction
- **Rule DSL field renames.** Rules condition trees now use `provider_name`, `provider_merchant_name`, `provider_category_primary`, `provider_category_detailed` instead of the unqualified versions.

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
