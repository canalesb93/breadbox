# Financial Data Aggregation Providers: Research for Breadbox

> **Date:** 2026-03-08 | **Status:** Reference document for Phase 30

## Executive Summary

GoCardless Bank Account Data (formerly Nordigen) — the only truly free European option — stopped accepting new signups in July 2025. Most US-focused providers (MX, Akoya, Yodlee) require enterprise sales calls. The most promising self-serve options are **Finicity/Mastercard Open Banking** (US, pay-as-you-go), **Pluggy** (Brazil, transparent pricing with free tier), and **Belvo** (LatAm, free sandbox).

## Provider Comparison

| Provider | Self-Serve | Geography | Free Tier | Pricing | Open Banking | Go SDK | Self-Hosted OK |
|---|---|---|---|---|---|---|---|
| **Finicity (Mastercard)** | Partial (sandbox self-serve, pay-as-you-go) | US, Canada | Sandbox | Pay-as-you-go + custom | No (direct connections) | No (OpenAPI spec on GitHub) | Possible |
| **Pluggy** | Yes | Brazil | 100 free connections | $0.05–0.25/connection | Yes (Open Finance Brasil) | No | **Yes** |
| **Belvo** | Sandbox only (sales for prod) | LatAm (MX, BR, CO) | Free sandbox + 25 live links | Custom tiers | Partial | No | Possible |
| **Mono** | Yes | Africa (Nigeria primary) | Subscription tiers | Per-account subscription | Partial | No | Likely |
| **Yapily** | Sandbox only | UK, Europe (19 countries) | Free sandbox | Tiered (opaque) | Yes (PSD2) | No | Possible |
| **Tink (Visa)** | Sandbox only | Europe (18 markets) | Free sandbox | ~€0.50/user/mo | Yes (PSD2) | No | Unlikely |
| **MX** | No (enterprise) | US, Canada | Dev sandbox (limited) | ~$15K+/yr | No | No | No |
| **Akoya** | No (enterprise) | US (747 institutions) | None | Custom | FDX standard | No | No |
| **Yodlee** | Partial (NDA for pricing) | Global (19K+ sources) | Limited trial | Custom (NDA) | Partial | No | No |
| **TrueLayer** | No (enterprise) | UK, Europe | Sandbox | Custom | Yes (FCA AISP) | No | No |
| **GoCardless** | **CLOSED** (July 2025) | UK, Europe | Was free | N/A | Yes (PSD2) | No | Was ideal |
| **Salt Edge** | No (consultation) | Global (50+ countries) | None | Custom | Yes (PSD2) | No | No |

## Recommendations

### Priority 1: Finicity / Mastercard Open Banking (US/Canada)

Best self-serve alternative to Plaid for the US market. Pay-as-you-go pricing works for personal use. OpenAPI spec on GitHub ([Mastercard/open-banking-us-openapi](https://github.com/Mastercard/open-banking-us-openapi)) enables Go client generation. OAuth-based connection flow similar to Plaid Link.

### Priority 2: Pluggy (Brazil)

Most developer-friendly LatAm option. Truly self-serve with transparent pricing. 100 free connections, then $0.05–0.25 per active connection. REST API suitable for hand-written Go HTTP client (like Teller). Covers 90% of Brazilian bank accounts.

### Priority 3: Belvo (Multi-Country LatAm)

Better for multi-country coverage (Mexico + Brazil + Colombia). Free sandbox is self-serve but production requires sales call. Python SDK officially maintained, no Go SDK.

## Strategic Notes

- **European gap**: No self-serve option exists since GoCardless closed. Yapily Starter tier is closest — worth monitoring.
- **SnapTrade**: Investment/brokerage account aggregation with a free tier. Outside core scope but interesting for investment tracking.
- **Mono**: Acquired by Flutterwave (Jan 2026). Operating independently for now but long-term direction uncertain.

## Implementation Approach

All new providers would implement the existing `Provider` interface (same pattern as Plaid/Teller):
- Finicity: generate Go client from OpenAPI spec, OAuth connection flow
- Pluggy/Belvo: hand-written HTTP client (like Teller), map transaction categories + sign conventions
