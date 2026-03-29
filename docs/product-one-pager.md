# Breadbox: Product One-Pager

## What It Is

Self-hosted financial data aggregation for families. Syncs bank data (Plaid, Teller, CSV) into PostgreSQL and exposes it through an MCP server and REST API.

More than infrastructure — it's **a methodology for agents** (composable instruction framework that teaches agents how to work with financial data correctly) and **an open system** (your data, your database, your agents — nothing locked in, fully customizable).

## The Problem

Bank transaction data is raw, fragmented, and poorly categorized. Traditional fintech solves this with ML trained on aggregate data — but **closed systems can never achieve 100% accuracy.** A generic ApplePay transaction has no merchant context. A Venmo transfer is just a P2P payment. "GENERAL MERCHANDISE" could be anything. The context doesn't exist in the transaction data alone — it exists with the family.

## The Thesis

Personal AI agents that know a family's context are the natural solution. Your agent can cross-reference Gmail receipts, search for local merchants, and draw on the family's history. It knows "DEBIT POS MARIA'S" is the taqueria around the corner. **They see transaction metadata. Your agent has the full picture.**

Agents need: (1) aggregated, structured access to the data, and (2) the ability to act on it. Breadbox provides both.

## How It Works

**Data aggregation:** Plaid + Teller auto-sync, CSV import, shared card dedup, normalized schema.

**MCP server (30+ tools):** querying, aggregation, enrichment, review queue, reporting.

**Rule engine:** Agents create pattern-matching rules informed by their context (local knowledge, receipts, family habits). Rules pre-categorize future transactions, but **agents still review everything** in the default setup — rules reduce cognitive load, not oversight. Over time, reviews get faster, not skipped. Users can dial this up or down.

**Human-in-the-loop:** Transactions → rules pre-categorize → agent reviews all items → reports to family → human corrects via re-enqueue → agent learns. Comments are the feedback channel.

## What Makes It Different

1. **Your data, your agents** — open, self-hosted, no lock-in
2. **Agent methodology** — not just an API; ships with Agent Wizard for correct financial data workflows
3. **Personalized context wins** — agents with your email, location, history outperform generic ML
4. **Rules as learning** — agents teach the system incrementally through context-informed rules
5. **Human-in-the-loop** — agents propose, humans verify. Trust built incrementally.
6. **Family-first** — multi-user, shared card dedup, per-member attribution
