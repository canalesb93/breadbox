import { Link } from "@tanstack/react-router";
import { AlertTriangle, ChevronRight, Link2 } from "lucide-react";
import { MetaBadge } from "@/components/meta-badge";
import { formatBalance } from "@/lib/format";
import { cn } from "@/lib/utils";
import type { Account } from "@/api/types";
import {
  accountLabel,
  accountTypeIcon,
  accountTypeLabel,
  creditUtilization,
  isLiability,
  utilizationBarClass,
} from "./account-utils";

interface AccountRowProps {
  account: Account;
}

// AccountRow is the in-card row used by the Accounts list — each
// institution / type group is a `ListCard` and each row inside is one of
// these. No self-border (the parent ListCard owns the bordered card +
// divide-y rail); padding + hover match the established `px-5 py-3.5`
// density shared by ConnectionRow / TransactionPrimary.
//
// The connection-detail accounts grid uses its own local `AccountCard`
// (defined inline in `features/connections/connection-accounts-list.tsx`)
// because that surface is a two-column bordered grid, not a divide-y list.
export function AccountRow({ account: a }: AccountRowProps) {
  const Icon = accountTypeIcon(a.type);
  const liability = isLiability(a.type);
  const util = creditUtilization(a);
  const statusPill = statusBadge(a.connection_status);
  return (
    <Link
      to="/accounts/$id"
      params={{ id: a.short_id }}
      className="hover:bg-muted/40 group flex items-center gap-3 px-5 py-3.5 transition-colors sm:gap-4"
    >
      <div className="bg-muted text-muted-foreground flex size-10 shrink-0 items-center justify-center rounded-lg">
        <Icon className="size-4" />
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="truncate text-sm font-medium">{accountLabel(a)}</span>
          {a.is_dependent_linked && (
            <MetaBadge icon={Link2} variant="secondary">
              Linked
            </MetaBadge>
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
        </div>
        {util != null && (
          <div className="mt-2 max-w-xs">
            <div className="text-muted-foreground mb-1 flex items-center justify-between text-[10px] tabular-nums">
              <span>{Math.round(util)}% used</span>
              {a.balance_limit != null && (
                <span>
                  {formatBalance(a.balance_limit, a.iso_currency_code)} limit
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
              ? formatBalance(
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
        <ChevronRight className="text-muted-foreground/60 group-hover:text-muted-foreground size-3.5 transition-colors" />
      </div>
    </Link>
  );
}

function statusBadge(status: string | null) {
  if (!status || status === "active") return null;
  if (status === "pending_reauth") {
    return (
      <MetaBadge
        icon={AlertTriangle}
        className="border-amber-500/40 bg-amber-500/5 text-amber-700 dark:text-amber-400"
      >
        Re-auth
      </MetaBadge>
    );
  }
  if (status === "error") {
    return <MetaBadge variant="destructive">Error</MetaBadge>;
  }
  if (status === "disconnected") {
    return <MetaBadge>Disconnected</MetaBadge>;
  }
  return null;
}
