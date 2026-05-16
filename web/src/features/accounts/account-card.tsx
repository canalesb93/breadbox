import { Link } from "@tanstack/react-router";
import { AlertTriangle, ChevronRight, Link2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import type { Account } from "@/api/types";
import {
  accountLabel,
  accountTypeIcon,
  accountTypeLabel,
  creditUtilization,
  formatCurrency,
  isLiability,
  utilizationBarClass,
} from "./account-utils";

interface AccountCardProps {
  account: Account;
  className?: string;
}

// AccountCard is the list-row shown on the Accounts page and on the
// connection-detail accounts grid. Click anywhere to open the account
// detail page. Keeps the visual rhythm tight: a 9×9 type-tile, the label,
// a metadata subline (type · mask), and the current balance to the right.
// Credit cards render their utilization bar below; non-active connections
// show an inline status pill so problem accounts stand out without an
// extra trip to /connections.
export function AccountCard({ account: a, className }: AccountCardProps) {
  const Icon = accountTypeIcon(a.type);
  const liability = isLiability(a.type);
  const util = creditUtilization(a);
  const statusPill = statusBadge(a.connection_status);
  return (
    <Link
      to="/accounts/$id"
      params={{ id: a.short_id }}
      className={cn(
        "bg-card hover:bg-accent/40 group flex items-center gap-3 rounded-lg border p-3 transition-colors",
        className,
      )}
    >
      <div className="bg-muted flex size-10 shrink-0 items-center justify-center rounded-lg">
        <Icon className="text-muted-foreground size-4" />
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="truncate text-sm font-medium">{accountLabel(a)}</span>
          {a.is_dependent_linked && (
            <Badge variant="secondary" className="gap-1 px-1.5 py-0 text-[10px]">
              <Link2 className="size-2.5" /> Linked
            </Badge>
          )}
          {statusPill}
        </div>
        <div className="text-muted-foreground mt-0.5 flex flex-wrap items-center gap-x-1.5 truncate text-xs">
          <span className="capitalize">{accountTypeLabel(a.type, a.subtype)}</span>
          {a.mask && (
            <>
              <span className="text-muted-foreground/40">·</span>
              <span className="tabular-nums">····{a.mask}</span>
            </>
          )}
          {a.institution_name && (
            <>
              <span className="text-muted-foreground/40">·</span>
              <span className="truncate">{a.institution_name}</span>
            </>
          )}
        </div>
        {util != null && (
          <div className="mt-2">
            <div className="text-muted-foreground mb-1 flex items-center justify-between text-[10px] tabular-nums">
              <span>{Math.round(util)}% used</span>
              {a.balance_limit != null && (
                <span>
                  {formatCurrency(a.balance_limit, a.iso_currency_code)} limit
                </span>
              )}
            </div>
            <div className="bg-muted h-1 overflow-hidden rounded-full">
              <div
                className={cn("h-full transition-all", utilizationBarClass(util))}
                style={{ width: `${util}%` }}
              />
            </div>
          </div>
        )}
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <div className="text-right">
          <div
            className={cn(
              "text-sm font-semibold tabular-nums whitespace-nowrap",
              liability && a.balance_current != null && a.balance_current > 0
                ? "text-foreground"
                : undefined,
            )}
          >
            {a.balance_current != null
              ? formatCurrency(
                  liability ? -a.balance_current : a.balance_current,
                  a.iso_currency_code,
                )
              : "—"}
          </div>
          {a.iso_currency_code && (
            <div className="text-muted-foreground text-[10px]">
              {a.iso_currency_code}
            </div>
          )}
        </div>
        <ChevronRight className="text-muted-foreground/40 group-hover:text-muted-foreground size-4 transition-colors" />
      </div>
    </Link>
  );
}

function statusBadge(status: string | null) {
  if (!status || status === "active") return null;
  if (status === "pending_reauth") {
    return (
      <Badge
        variant="outline"
        className="gap-1 border-amber-500/40 bg-amber-500/5 px-1.5 py-0 text-[10px] text-amber-700 dark:text-amber-400"
      >
        <AlertTriangle className="size-2.5" /> Re-auth
      </Badge>
    );
  }
  if (status === "error") {
    return (
      <Badge variant="destructive" className="px-1.5 py-0 text-[10px]">
        Error
      </Badge>
    );
  }
  if (status === "disconnected") {
    return (
      <Badge variant="outline" className="px-1.5 py-0 text-[10px]">
        Disconnected
      </Badge>
    );
  }
  return null;
}
