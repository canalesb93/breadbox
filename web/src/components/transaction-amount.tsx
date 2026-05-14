import { formatAmount, formatDate } from "@/lib/format";
import { cn } from "@/lib/utils";
import type { Transaction } from "@/api/types";

interface TransactionAmountProps {
  transaction: Transaction;
  className?: string;
}

// TransactionAmount is the reusable right-aligned amount block — the signed
// amount with the transaction date beneath it. Inflows (negative amounts)
// render in the success color.
export function TransactionAmount({
  transaction: t,
  className,
}: TransactionAmountProps) {
  const isInflow = t.amount < 0;
  return (
    <div className={cn("text-right whitespace-nowrap", className)}>
      <div
        className={cn(
          "font-medium tabular-nums",
          isInflow && "text-success",
        )}
      >
        {formatAmount(t.amount, t.iso_currency_code)}
      </div>
      <div className="text-muted-foreground text-xs tabular-nums">
        {formatDate(t.date)}
      </div>
    </div>
  );
}
