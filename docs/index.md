# Breadbox Documentation

Breadbox is a self-hosted financial data aggregation server for households. It syncs bank data from multiple providers (Plaid, Teller, CSV), stores it in PostgreSQL, and exposes it through a REST API and MCP server for AI agents.

## Quick Links

- **[Architecture](architecture.md)** -- system design, provider interface, deployment
- **[REST API](rest-api.md)** -- endpoints, authentication, request/response formats
- **[MCP Server](mcp-server.md)** -- AI agent integration via Model Context Protocol
- **[Data Model](data-model.md)** -- database schema, enums, relationships

## Key Concepts

### Your data, your server

Breadbox runs on your hardware. Bank credentials are encrypted with AES-256-GCM at rest. You control who has access, and you can export or delete everything at any time.

### Agent-first design

The primary interface is the API, not the dashboard. Breadbox is built to be queried by AI agents via MCP or REST. The admin dashboard is for setup and monitoring.

### Multi-provider

Connect to banks via Plaid, Teller, or CSV import. All providers normalize data into a common schema, so your queries work the same regardless of how the data was ingested.

### Household model

One Breadbox instance serves one household. An admin manages connections and family members. Each family member's accounts are tracked separately for attribution and filtering.

## Getting Started

See the [README](https://github.com/canalesb93/breadbox#quick-start) for installation and setup instructions.
