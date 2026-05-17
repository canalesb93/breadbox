import { formatAmount, formatDate } from "@/lib/format";
import { cn } from "@/lib/utils";
import type { Transaction } from "@/api/types";

interface TransactionAmountProps {
  transaction: Transaction;
  className?: string;
}

// TransactionAmount is the reusable right-aligned amount block — the signed
// amount with the transaction date beneath it.
//
// Sign vocabulary: inflows (negative amounts) render in the success color.
//
// Pending vocabulary (iter 103): when the transaction has not posted yet
// (`t.pending`), the amount renders italic + at 70% opacity so a scanning eye
// reads the row as tentative. Pairs with the `Pending` `<MetaBadge>` in
// `<TransactionPrimary>` — once a row is settled the visual settles down too.
// Title attribute carries the contract for keyboard/sighted hover; the badge
// alongside the description carries the screen-reader text.
export function TransactionAmount({
  transaction: t,
  className,
}: TransactionAmountProps) {
  const isInflow = t.amount < 0;
  const isPending = t.pending;
  return (
    <div className={cn("text-right whitespace-nowrap", className)}>
      <div
        className={cn(
          "font-medium tabular-nums",
          isInflow && "text-success",
          isPending && "italic opacity-70",
        )}
        title={isPending ? "Pending — amount may change once posted" : undefined}
      >
        {formatAmount(t.amount, t.iso_currency_code)}
      </div>
      <div className="text-muted-foreground text-xs tabular-nums">
        {formatDate(t.date)}
      </div>
    </div>
  );
}
