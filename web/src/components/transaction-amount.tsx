import { formatAmount, formatDate } from "@/lib/format";
import { cn } from "@/lib/utils";
import type { Transaction } from "@/api/types";

interface TransactionAmountProps {
  transaction: Transaction;
  className?: string;
}

// Inflows (negative amounts) render in the success color. Pending rows
// (`t.pending`) render italic + 70% opacity to read as tentative; the
// matching `Pending` MetaBadge in <TransactionPrimary> carries the
// screen-reader text.
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
