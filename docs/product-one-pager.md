# Breadbox: Product One-Pager

## What It Is

Breadbox is a self-hosted financial data aggregation server for families. It connects to banks via Plaid, Teller, and CSV imports, pulls transaction data into a unified PostgreSQL database, and exposes it through an MCP (Model Context Protocol) server and REST API.

It is not a budgeting app. It is not a dashboard. It is **infrastructure** — a data layer that makes a family's complete financial picture available to AI agents.

## The Problem

Bank transaction data arrives raw and unstructured. Transaction names are mangled ("DEBIT POS 03/15 WHOLEFDS MKT #10847"). Categories from providers are generic or wrong. When a family has accounts across multiple banks, multiple family members, and multiple card types, the data is fragmented and noisy.

Manual categorization is tedious and doesn't scale. Traditional fintech apps try to solve this with static rule engines or ML models trained on aggregate data. These approaches fail on the long tail: local merchants, family-specific patterns, shared cards, and the contextual knowledge that only the family has.

## The Thesis

We believe personal AI agents — assistants that know a family's context — are becoming the natural interface for financial data management. A family's agent knows that "DEBIT POS MARIA'S" is the taqueria around the corner, that the $47.99 charge on the 15th of every month is a streaming bundle, and that the $200 at "GENERAL MERCHANDISE" was a birthday gift.

These agents need two things:
1. **Aggregated, structured access to the data** — a single source of truth across all accounts, all family members, all providers
2. **The ability to act on it** — categorize, create rules, flag anomalies, generate reports

Breadbox provides both through its MCP server.

## How It Works

### Data Aggregation
- Plaid and Teller integrations sync transactions automatically on a schedule
- CSV import for banks without API support
- Cross-connection deduplication for shared cards (authorized user linking)
- All data normalized into a consistent schema with unified amount conventions

### Agent Interface (MCP Server)
Agents connect to Breadbox's MCP endpoint and get access to 30+ tools organized around:

- **Querying** — transactions, accounts, users, categories, with flexible filters and search modes
- **Aggregation** — spending summaries by category/time, merchant analysis, recurring charge detection
- **Enrichment** — categorize transactions, create and manage rules, manage the category taxonomy
- **Review Queue** — systematic review of new/uncategorized transactions with approve/skip decisions
- **Communication** — submit reports to the family dashboard, add comments to transactions

### The Rule Engine
The real leverage is in **transaction rules**. When an agent categorizes a transaction, it can also create a rule — a pattern-matching condition that automatically categorizes all future transactions matching the same pattern. Over time, the system gets smarter:

- First sync: 500 uncategorized transactions, agent reviews all of them
- Second sync: 50 new transactions, 40 auto-categorized by rules, agent reviews 10
- Tenth sync: 50 new transactions, 48 auto-categorized, agent reviews 2

Rules support flexible conditions (AND/OR/NOT logic, regex, numeric comparisons) across transaction fields. They're evaluated during every sync in priority order. An agent with family context creates better rules than any generic ML model.

### Human-in-the-Loop
Agents don't operate in a vacuum. The review queue creates a structured feedback loop:

1. New transactions arrive → rules auto-categorize what they can → uncategorized items enter the review queue
2. Agent reviews pending items, categorizes them, creates rules for new patterns
3. Agent posts a report summarizing what it did and flagging anything unusual
4. Human reviews the report, checks flagged items, can re-enqueue transactions if the agent got something wrong
5. Agent sees re-enqueued items (type: `re_review`) in the next review cycle and learns from the correction

Comments on transactions serve as the communication channel between agents and humans. An agent explains why it categorized something a certain way; a human can respond with additional context.

## Current Capabilities

| Area | Status |
|------|--------|
| Data aggregation (Plaid, Teller, CSV) | Shipped |
| MCP server (30+ tools, Streamable HTTP + stdio) | Shipped |
| Review queue with auto-enqueue during sync | Shipped |
| Transaction rules with condition tree engine | Shipped |
| Agent reports with dashboard widget | Shipped |
| Category taxonomy management | Shipped |
| Account linking / dedup | Shipped |
| Agent Wizard (composable prompt builder) | Shipped |
| Agent-facing configuration resource | Planned |
| Subscription/recurring charge tracking | Planned |
| Tagging system (rule actions beyond category) | Planned |
| Multi-agent coordination patterns | Planned |

## Who It's For

Families (or individuals) who:
- Have accounts across multiple banks and want a unified view
- Use or plan to use AI agents for personal productivity
- Want to self-host their financial data rather than share it with a SaaS
- Care about accuracy in their financial records and are willing to invest in getting categorization right

## What Makes It Different

1. **MCP-native** — purpose-built for agent consumption, not retrofitted
2. **Self-hosted** — financial data stays on your infrastructure
3. **Rule engine as learning mechanism** — agents incrementally teach the system through rules
4. **Multi-provider abstraction** — Plaid, Teller, CSV behind a unified interface
5. **Family-first** — multi-user with per-member attribution, shared card dedup, family-level reporting
6. **Human-in-the-loop by design** — not autonomous, but collaborative: agents propose, humans verify
