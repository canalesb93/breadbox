# Breadbox: Product One-Pager

## What It Is

Breadbox is a self-hosted financial data aggregation server for families. It connects to banks via Plaid, Teller, and CSV imports, pulls transaction data into a unified PostgreSQL database, and exposes it through an MCP (Model Context Protocol) server and REST API.

It is not a budgeting app. It is not a dashboard. It is **infrastructure** — a data layer that makes a family's complete financial picture available to AI agents. But it's more than just infrastructure:

- **A methodology for agents.** Breadbox ships with a curated Agent Wizard — composable instruction blocks that teach agents how to correctly work with financial data, create rules, review transactions, and communicate results. It's not just access to data; it's a framework for getting accurate results.
- **Your data, your agents.** Nothing is locked in. The data is yours, in your PostgreSQL database. The API is open. The agent instructions are fully customizable and entirely optional — use the defaults, modify them, or write your own from scratch. Breadbox is an open system that works with any MCP-compatible agent.

## The Problem

Bank transaction data arrives raw and unstructured. Transaction names are mangled ("DEBIT POS 03/15 WHOLEFDS MKT #10847"). Categories from providers are generic or wrong. When a family has accounts across multiple banks, multiple family members, and multiple card types, the data is fragmented and noisy.

Traditional fintech apps and banks try to solve this with static rule engines or ML models trained on aggregate data. **These are closed systems, and they will never achieve 100% accuracy.** A generic ApplePay transaction has no merchant context at the bank level. A Venmo transfer is just a peer-to-peer payment — the bank has no idea what it was for. A charge at "GENERAL MERCHANDISE" could be anything. No amount of aggregate ML training fixes this because the context doesn't exist in the transaction data alone.

The context exists with the family. A family member knows that the Venmo to Sarah was splitting a dinner bill. That the ApplePay at a specific terminal was the coffee shop downstairs. That the "GENERAL MERCHANDISE" charge was a birthday gift from Amazon.

Manual categorization captures this context, but it's tedious and doesn't scale.

## The Thesis

Personal AI agents — assistants that know a family's context — are the natural solution. Unlike closed fintech systems, a family's agent can cross-reference transactions against email receipts, search for local merchants by name, and draw on the family's history to make informed decisions. The agent knows that "DEBIT POS MARIA'S" is the taqueria around the corner, that the $47.99 charge on the 15th of every month is a streaming bundle, and that the $200 at "GENERAL MERCHANDISE" was a birthday gift — because it has access to the receipts in Gmail, the family's location context, and prior categorization patterns.

This is fundamentally different from what any bank or fintech SaaS can offer. They see transaction metadata. Your agent sees your life.

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
The real leverage is in **transaction rules**. As agents review transactions and build context about a family's spending patterns, they create rules — pattern-matching conditions that automatically categorize future transactions matching the same pattern. The rules are informed by the agent's context: its knowledge of local merchants, email receipts, family habits.

But rules don't replace review. In the default setup, **agents still review every transaction**, even ones that rules have pre-categorized. Rules reduce the cognitive load (the agent can quickly confirm a correctly pre-categorized transaction vs. figuring it out from scratch), but human and agent oversight remains. Over time, the review process gets faster, not skipped:

- First sync: 500 uncategorized transactions, agent reviews all of them, creates 30 rules
- Second sync: 50 new transactions, 40 pre-categorized by rules, agent reviews all 50 (confirms 40, categorizes 10)
- Tenth sync: 50 new transactions, 48 pre-categorized, agent reviews all 50 (confirms 48, handles 2 new patterns)

Users can dial this up or down — trusting rules fully for certain categories, or maintaining agent review for everything. The vanilla setup errs on the side of oversight because it builds confidence and catches edge cases.

Rules support flexible conditions (AND/OR/NOT logic, regex, numeric comparisons) across transaction fields. They're evaluated during every sync in priority order.

### Human-in-the-Loop
Agents don't operate in a vacuum. The review queue creates a structured feedback loop:

1. New transactions arrive → rules pre-categorize what they can → all items enter the review queue
2. Agent reviews pending items, confirms or corrects categories, creates rules for new patterns
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

1. **Your data, your agents** — open system, self-hosted, no lock-in. Data lives in your PostgreSQL. API is open. Instructions are customizable.
2. **Agent methodology, not just an API** — ships with a composable instruction framework (Agent Wizard) that teaches agents how to work with financial data correctly
3. **Personalized context wins** — agents with access to your email, location, and history outperform any generic ML model. Breadbox is built for this.
4. **Rule engine as learning mechanism** — agents incrementally teach the system through rules informed by their context
5. **Human-in-the-loop by design** — agents review and propose, humans verify and correct. Trust is built incrementally.
6. **Multi-provider abstraction** — Plaid, Teller, CSV behind a unified interface
7. **Family-first** — multi-user with per-member attribution, shared card dedup, family-level reporting
