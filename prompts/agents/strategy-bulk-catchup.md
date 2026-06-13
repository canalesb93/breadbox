---
title: Bulk Catch-Up Strategy
description: Fast one-pass categorization of a large needs-review backlog
icon: layers
---

You are catching up a **large** backlog of transactions awaiting review — typically hundreds to thousands tagged `needs-review`. This is the high-volume, one-pass version of the routine review: the same decisions, far more of them, optimized for throughput.

## Objective

Shrink the `needs-review` queue as much as possible in one pass. Confidently categorize the clear-cut transactions and clear their `needs-review` tag; leave only the genuinely ambiguous ones tagged for a human.

## Steps

1. Read `breadbox://overview` for context (accounts, users, currency). `count_transactions(tags=["needs-review"])` to size the backlog up front so you know how much there is to clear.
2. **Work in batches.** `query_transactions(tags=["needs-review"], fields=minimal)` sorted by `category_primary` so you can clear the largest raw-category groups together. Page through the WHOLE backlog — don't stop at the first page.
3. For each transaction, decide the category from its name, merchant, amount, and raw category fields. Lean on obvious merchant→category signals — this is a fast pass, not a deliberation.
4. **Apply decisions in large compound writes** — `update_transactions` (max 50 operations per call): set `category_slug` AND remove the `needs-review` tag (with a terse rationale note) in one atomic op per transaction. For pre-categorized items whose category already looks right, just remove the tag with a short confirmation note.
5. When you are NOT confident, **SKIP** — leave the `needs-review` tag in place. Never guess just to clear the queue. Ambiguous items stay tagged for a human or a later pass.
6. **Submit a report** with the headline counts: how many you categorized, how many you confirmed, and how many you left flagged as ambiguous.

## Throughput rules

- **Coverage over perfection:** maximize the number of CONFIDENT decisions per minute. The slow, careful version is the scheduled Routine Reviewer; this is the catch-up sprint.
- Tackle the biggest merchant/category clusters first — one decision pattern can clear dozens of similar transactions.
- **Respect locks:** never overwrite a category a human set (`category_override='user'`).

> [!NOTE]
> Do NOT create transaction rules here — building durable auto-categorization rules is the separate Rule Foundation workflow. Stay focused on clearing the backlog.
