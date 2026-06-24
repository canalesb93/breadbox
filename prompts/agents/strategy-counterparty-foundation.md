---
title: Counterparty Foundation Strategy
description: Analyze recent history to identify the entities behind charges and codify them as assign_counterparty rules
icon: store
---

You are setting up the **foundation** for counterparty identity. By studying a large slice of the household's real history, you figure out *who* is on the other side of each charge — the merchant, the person, the employer — and encode each as a durable `assign_counterparty` rule on the raw provider fields, so every future charge resolves to the right entity automatically on sync.

## Objective

Establish a high-precision set of canonical **counterparties** from the last 1000+ transactions, each defined by `assign_counterparty` rule(s) authored on raw, immutable fields, and enriched with the brand details you can confidently supply. The win is a stable, cross-provider map of who the household actually transacts with — collapsing the cryptic, source-specific descriptors into one entity each.

## What a counterparty is

The **other side** of a transaction — and not just merchants. A counterparty can be a business (Amazon, the electric company), but also a **non-merchant**: a Venmo recipient, a person you split rent with, an employer paying you, a landlord. The signal is identity, not category: several different raw descriptors (`SQ *BLUE BOTTLE`, `BLUE BOTTLE COFFEE #12`, `TST* BLUE BOTTLE`) that are all the **same entity** belong to one counterparty.

## Steps

1. Read `get_overview` for context. `list_counterparties` to see what already exists — reuse and extend, never duplicate an entity. `list_categories` for the enrichment taxonomy.
2. **Survey history:** `query_transactions` over a large recent sample (aim for the last 1000+ transactions / ~12 months). Group by payee and look for the entities that recur and the descriptor **variants** that are secretly the same entity. `transaction_summary` helps the high-volume payees surface.
3. **Identify counterparty candidates.** A good candidate recurs across **3+ transactions** (or is clearly a meaningful single entity — your employer, your landlord). For each, collect every raw-descriptor variant that maps to it.
4. **Author `assign_counterparty` rule(s) on raw fields** — the counterparties idiom from the rules curriculum: match `provider_name` / `provider_merchant_name contains "…"` and target `assign_counterparty`. **Reuse, don't duplicate:** mint a counterparty once with `create_if_missing: true`, then point each variant's rule at the **same** counterparty (by `counterparty_short_id`) so all the descriptors collapse into one entity.
5. **Dry-run EVERY candidate before creating it:** `preview_rule` reports the match count + a sample; `find_matching_rules` confirms nothing already covers it. Reject anything that over-matches (a generic word that catches unrelated charges) or matches zero rows.
6. **Create the vetted rules** with `create_transaction_rule` — pass a `rules` array to author several at once.
7. **Enrich each counterparty** with `update_counterparty` where you can do so confidently: a default `category`, the `website` (its domain auto-fetches a brand logo via logo.dev when logos are enabled), and the `mcc` if known. Enrich only what you're sure of — leave fields blank rather than guess.
8. **Backfill carefully:** for rules you are confident in, use `apply_rules` to bind the matching history to its counterparty. A clean dry run is the prerequisite.
9. **Submit a report** listing each counterparty created (its rule conditions, the descriptor variants it collapses, enrichment applied, and match count) and what was backfilled.

> [!IMPORTANT]
> - Precision over coverage: a broad condition (a common word) binds unrelated charges to the wrong entity. Anchor on a distinctive substring; when uncertain, leave it for a human.
> - Match only on raw, immutable fields (`provider_name`, `provider_merchant_name`, `amount`). Never key a counterparty rule on `counterparty`, `has_counterparty`, `category`, or any mutable display field.
> - One entity, many descriptors: prefer pointing several raw-field rules at one counterparty over minting near-duplicate entities.
> - Never `apply_rules` on a rule you did not dry-run and judge high-precision.
