import { formatAmount, formatBalance, formatCompactAmount } from "@/lib/format";
import { TransactionAmount } from "@/components/transaction-amount";
import { SandboxSection, Specimen } from "@/sandbox/kit";
import { sampleTransactions } from "@/sandbox/fixtures";

// formatAmount reference — covers the sign convention, grouping, currencies,
// and the null-currency fallback.
const AMOUNTS: { code: string; output: string; note?: string }[] = [
  {
    code: "formatAmount(6.75, 'USD')",
    output: formatAmount(6.75, "USD"),
    note: "outflow — rendered plain",
  },
  {
    code: "formatAmount(-3200, 'USD')",
    output: formatAmount(-3200, "USD"),
    note: "inflow — leading +",
  },
  {
    code: "formatAmount(0, 'USD')",
    output: formatAmount(0, "USD"),
  },
  {
    code: "formatAmount(1234567.5, 'USD')",
    output: formatAmount(1234567.5, "USD"),
    note: "thousands grouping",
  },
  {
    code: "formatAmount(42, 'EUR')",
    output: formatAmount(42, "EUR"),
  },
  {
    code: "formatAmount(42, 'GBP')",
    output: formatAmount(42, "GBP"),
  },
  {
    code: "formatAmount(4200, 'JPY')",
    output: formatAmount(4200, "JPY"),
    note: "no minor units",
  },
  {
    code: "formatAmount(9.99, null)",
    output: formatAmount(9.99, null),
    note: "null currency → USD fallback",
  },
];

// Outflow / inflow / pending — enough to show TransactionAmount's full range.
const AMOUNT_ROWS = sampleTransactions.slice(0, 3);

export function AmountsSection() {
  return (
    <SandboxSection
      id="amounts"
      title="Amounts"
      description="How money is displayed in v2. One sign convention, one formatter, one component — applied everywhere a balance or transaction amount appears."
    >
      <Specimen
        label="Sign convention"
        description="Breadbox stores amounts as positive = money out, negative = money in. It's deliberate but counterintuitive — every amount surface relies on it."
        className="block"
      >
        <div className="grid gap-2 sm:grid-cols-2">
          <div className="rounded-md border p-3">
            <div className="text-sm font-medium">Positive → outflow</div>
            <p className="text-muted-foreground text-xs">
              Spending, fees, payments. Rendered plain.
            </p>
            <div className="mt-1 font-mono text-sm tabular-nums">
              52.18 → {formatAmount(52.18, "USD")}
            </div>
          </div>
          <div className="rounded-md border p-3">
            <div className="text-sm font-medium">Negative → inflow</div>
            <p className="text-muted-foreground text-xs">
              Income, refunds, deposits. Rendered with a leading + in the
              success color.
            </p>
            <div className="text-success mt-1 font-mono text-sm tabular-nums">
              -3200 → {formatAmount(-3200, "USD")}
            </div>
          </div>
        </div>
      </Specimen>

      <Specimen
        label="formatAmount"
        code="lib/format"
        description="The single amount formatter. Cached Intl.NumberFormat per currency; absolute value formatted, sign applied after. Never sum across currencies."
        className="block"
      >
        <div className="grid gap-2 sm:grid-cols-2">
          {AMOUNTS.map((a) => (
            <div
              key={a.code}
              className="flex items-center justify-between gap-3 rounded-md border px-3 py-2"
            >
              <div className="min-w-0">
                <code className="text-muted-foreground block truncate font-mono text-xs">
                  {a.code}
                </code>
                {a.note && (
                  <span className="text-muted-foreground text-[10px]">
                    {a.note}
                  </span>
                )}
              </div>
              <span className="shrink-0 text-sm font-medium tabular-nums">
                {a.output}
              </span>
            </div>
          ))}
        </div>
      </Specimen>

      <Specimen
        label="formatBalance"
        code="lib/format"
        description="Account balances, totals, scoreboard cells — anything that isn't following the transaction sign convention. Same cached Intl.NumberFormat as formatAmount; no leading + for negatives, sign rendered literally."
        className="block"
      >
        <div className="grid gap-2 sm:grid-cols-2">
          {[
            { code: "formatBalance(8240.55, 'USD')", out: formatBalance(8240.55, "USD"), note: "asset balance" },
            { code: "formatBalance(-1240, 'USD')", out: formatBalance(-1240, "USD"), note: "overdraft — sign preserved" },
            { code: "formatBalance(42, 'EUR')", out: formatBalance(42, "EUR") },
            { code: "formatBalance(0, null)", out: formatBalance(0, null), note: "null currency → USD fallback" },
          ].map((a) => (
            <div
              key={a.code}
              className="flex items-center justify-between gap-3 rounded-md border px-3 py-2"
            >
              <div className="min-w-0">
                <code className="text-muted-foreground block truncate font-mono text-xs">
                  {a.code}
                </code>
                {a.note && (
                  <span className="text-muted-foreground text-[10px]">
                    {a.note}
                  </span>
                )}
              </div>
              <span className="shrink-0 text-sm font-medium tabular-nums">
                {a.out}
              </span>
            </div>
          ))}
        </div>
      </Specimen>

      <Specimen
        label="formatCompactAmount"
        code="lib/format"
        description="Hero KPIs — strips minor units so the headline number reads at a glance. Used by the home stats card. Cached Intl.NumberFormat per currency."
        className="block"
      >
        <div className="grid gap-2 sm:grid-cols-2">
          {[
            { code: "formatCompactAmount(12450.55, 'USD')", out: formatCompactAmount(12450.55, "USD") },
            { code: "formatCompactAmount(-3200, 'USD')", out: formatCompactAmount(-3200, "USD") },
            { code: "formatCompactAmount(8240, 'EUR')", out: formatCompactAmount(8240, "EUR") },
            { code: "formatCompactAmount(420000, 'JPY')", out: formatCompactAmount(420000, "JPY") },
          ].map((a) => (
            <div
              key={a.code}
              className="flex items-center justify-between gap-3 rounded-md border px-3 py-2"
            >
              <code className="text-muted-foreground block truncate font-mono text-xs">
                {a.code}
              </code>
              <span className="shrink-0 text-sm font-medium tabular-nums">
                {a.out}
              </span>
            </div>
          ))}
        </div>
      </Specimen>

      <Specimen
        label="TransactionAmount"
        code="components/transaction-amount"
        description="The reusable amount block — signed amount over its date, right-aligned, tabular figures. Inflows pick up the success color automatically. Pending transactions render italic at 70% opacity so the column reads as tentative until the row settles (pairs with the `Pending` `MetaBadge` chip in `TransactionPrimary`)."
        className="block divide-y"
      >
        {AMOUNT_ROWS.map((t) => (
          <div
            key={t.id}
            className="flex items-center justify-between gap-4 py-2 first:pt-0 last:pb-0"
          >
            <span className="text-muted-foreground text-sm">
              {t.provider_name}
              {t.pending && " · pending"}
            </span>
            <TransactionAmount transaction={t} />
          </div>
        ))}
      </Specimen>
    </SandboxSection>
  );
}
