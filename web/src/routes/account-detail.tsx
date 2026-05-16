import { useMemo } from "react";
import {
  Link,
  useNavigate,
  useParams,
  useSearch,
} from "@tanstack/react-router";
import { z } from "zod";
import {
  AlertTriangle,
  ArrowLeft,
  ArrowRight,
  Banknote,
  Eye,
  EyeOff,
  Link2,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { EmptyState } from "@/components/empty-state";
import { useAccount, useAccounts } from "@/api/queries/accounts";
import type { AccountDetail } from "@/api/types";
import {
  accountLabel,
  accountTypeIcon,
  accountTypeLabel,
  creditUtilization,
  formatCurrency,
  isLiability,
  utilizationBarClass,
} from "@/features/accounts/account-utils";
import { AccountSettingsCard } from "@/features/accounts/account-settings-card";
import { AccountRecentTransactions } from "@/features/accounts/account-recent-transactions";
import { AccountLinksSection } from "@/features/accounts/account-links-section";
import { LinkAccountSheet } from "@/features/accounts/link-account-sheet";
import { cn } from "@/lib/utils";

// Search-param schema.
//   action → "link" opens the LinkAccountSheet pre-targeted at this account.
export const accountDetailSearchSchema = z.object({
  action: z.literal("link").optional(),
});

type AccountDetailSearch = z.infer<typeof accountDetailSearchSchema>;

export function AccountDetailPage() {
  const { id } = useParams({ strict: false }) as { id?: string };
  const search = useSearch({ strict: false }) as AccountDetailSearch;
  const navigate = useNavigate();

  const acctQuery = useAccount(id);
  // The accounts list powers the dependent-picker inside LinkAccountSheet
  // and resolves link short_ids inside AccountLinksSection.
  const accountsQuery = useAccounts();

  function openLinkSheet() {
    if (!acctQuery.data) return;
    navigate({
      to: "/accounts/$id",
      params: { id: acctQuery.data.short_id },
      search: (prev: AccountDetailSearch) => ({ ...prev, action: "link" }),
      replace: false,
    });
  }

  function closeLinkSheet() {
    if (!acctQuery.data) return;
    navigate({
      to: "/accounts/$id",
      params: { id: acctQuery.data.short_id },
      search: (prev: AccountDetailSearch) => ({ ...prev, action: undefined }),
      replace: true,
    });
  }

  return (
    <div className="mx-auto max-w-5xl">
      <Button variant="ghost" size="sm" asChild className="mb-4 -ml-2">
        <Link to="/accounts">
          <ArrowLeft className="size-4" />
          Accounts
        </Link>
      </Button>

      {acctQuery.isLoading ? (
        <DetailSkeleton />
      ) : acctQuery.isError || !acctQuery.data ? (
        <EmptyState
          icon={Banknote}
          title="Account not found"
          description="It may have been disconnected, or the link is wrong."
          action={
            <Button variant="outline" asChild>
              <Link to="/accounts">Back to accounts</Link>
            </Button>
          }
        />
      ) : (
        <DetailBody
          account={acctQuery.data}
          accounts={accountsQuery.data ?? []}
          onAddLink={openLinkSheet}
        />
      )}

      {acctQuery.data && (
        <LinkAccountSheet
          open={search.action === "link"}
          onOpenChange={(open) => {
            if (!open) closeLinkSheet();
          }}
          primary={acctQuery.data}
          accounts={accountsQuery.data ?? []}
        />
      )}
    </div>
  );
}

interface DetailBodyProps {
  account: AccountDetail;
  accounts: ReturnType<typeof useAccounts>["data"] extends infer T
    ? T extends undefined
      ? never
      : T
    : never;
  onAddLink: () => void;
}

function DetailBody({ account: a, accounts, onAddLink }: DetailBodyProps) {
  const Icon = accountTypeIcon(a.type);
  const liability = isLiability(a.type);
  const util = creditUtilization(a);
  const statusAlert = useMemo(() => connectionStatusAlert(a), [a]);

  return (
    <div className="space-y-6">
      {/* Hero */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex min-w-0 items-center gap-3">
          <div className="bg-muted flex size-12 shrink-0 items-center justify-center rounded-lg">
            <Icon className="text-muted-foreground size-5" />
          </div>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="truncate text-xl font-semibold tracking-tight">
                {accountLabel(a)}
              </h1>
              {a.excluded && (
                <Badge variant="outline" className="gap-1 text-[10px]">
                  <EyeOff className="size-2.5" /> Excluded
                </Badge>
              )}
              {a.is_dependent_linked && (
                <Badge variant="secondary" className="gap-1 text-[10px]">
                  <Link2 className="size-2.5" /> Linked dependent
                </Badge>
              )}
            </div>
            <div className="text-muted-foreground mt-1 flex flex-wrap items-center gap-1.5 text-xs">
              <span className="capitalize">
                {accountTypeLabel(a.type, a.subtype)}
              </span>
              {a.mask && (
                <>
                  <span className="text-muted-foreground/40">·</span>
                  <span className="tabular-nums">····{a.mask}</span>
                </>
              )}
              {a.institution_name && (
                <>
                  <span className="text-muted-foreground/40">·</span>
                  <span>{a.institution_name}</span>
                </>
              )}
              {a.connection_user_name && (
                <>
                  <span className="text-muted-foreground/40">·</span>
                  <span>{a.connection_user_name}</span>
                </>
              )}
            </div>
          </div>
        </div>

        <div className="flex shrink-0 flex-wrap items-center gap-2">
          {!a.is_dependent_linked && (
            <Button variant="outline" size="sm" onClick={onAddLink}>
              <Link2 className="size-4" />
              Link account
            </Button>
          )}
          <Button variant="outline" size="sm" asChild>
            <Link to="/transactions" search={{ account: a.short_id }}>
              <Eye className="size-4" />
              View transactions
            </Link>
          </Button>
        </div>
      </div>

      {statusAlert}

      {/* Balance cards */}
      <div className="grid gap-4 md:grid-cols-2">
        <BalanceCard account={a} liability={liability} util={util} />
        <SecondaryCard account={a} liability={liability} />
      </div>

      <AccountSettingsCard account={a} />

      {/* Account links - skip when the account is itself a dependent.
          Showing a "Link an account" CTA on a dependent would let users
          create the second hop of an A→B→C chain, which the backend
          rejects anyway. The settings card still surfaces the dependent
          state, and the LinkAccountSheet's filter logic mirrors it. */}
      {!a.is_dependent_linked && (
        <AccountLinksSection
          account={a}
          accounts={accounts}
          onAddLink={onAddLink}
        />
      )}

      <AccountRecentTransactions
        accountShortId={a.short_id}
        transactions={a.recent_transactions}
      />
    </div>
  );
}

function connectionStatusAlert(a: AccountDetail) {
  const status = a.connection_status;
  if (!status || status === "active") return null;
  if (status === "pending_reauth") {
    return (
      <Alert className="border-amber-500/30 bg-amber-500/5">
        <AlertTriangle className="size-4 text-amber-700 dark:text-amber-400" />
        <AlertTitle className="text-amber-700 dark:text-amber-400">
          Connection needs re-authentication
        </AlertTitle>
        <AlertDescription className="flex flex-wrap items-center justify-between gap-2">
          <span>Recent syncs may be stale until you reconnect.</span>
          {a.connection_short_id && (
            <Button size="sm" variant="outline" asChild>
              <Link
                to="/connections/$id"
                params={{ id: a.connection_short_id }}
              >
                Open connection
                <ArrowRight className="size-3.5" />
              </Link>
            </Button>
          )}
        </AlertDescription>
      </Alert>
    );
  }
  if (status === "error") {
    return (
      <Alert variant="destructive">
        <AlertTriangle className="size-4" />
        <AlertTitle>Connection error</AlertTitle>
        <AlertDescription className="flex flex-wrap items-center justify-between gap-2">
          <span>Sync is currently failing. Check the connection for details.</span>
          {a.connection_short_id && (
            <Button size="sm" variant="outline" asChild>
              <Link
                to="/connections/$id"
                params={{ id: a.connection_short_id }}
              >
                Open connection
                <ArrowRight className="size-3.5" />
              </Link>
            </Button>
          )}
        </AlertDescription>
      </Alert>
    );
  }
  if (status === "disconnected") {
    return (
      <Alert>
        <AlertTitle>Connection disconnected</AlertTitle>
        <AlertDescription>
          This account is read-only — its parent connection no longer syncs.
          Historical transactions remain.
        </AlertDescription>
      </Alert>
    );
  }
  return null;
}

interface BalanceCardProps {
  account: AccountDetail;
  liability: boolean;
  util: number | null;
}

function BalanceCard({ account: a, liability, util }: BalanceCardProps) {
  const current = a.balance_current;
  // For liabilities (credit/loan), Plaid returns positive `balance_current`
  // to mean "you owe this much". The label is "Balance owed", so show the
  // raw positive — the destructive tint already conveys it's a negative
  // line on the household sheet.
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-muted-foreground text-xs font-medium">
          {liability ? "Balance owed" : "Current balance"}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-2">
        <div
          className={cn(
            "text-3xl font-semibold tabular-nums tracking-tight",
            liability && current != null && current > 0 && "text-destructive/90",
          )}
        >
          {current != null
            ? formatCurrency(current, a.iso_currency_code)
            : "—"}
        </div>
        {util != null && (
          <div className="space-y-1">
            <div className="text-muted-foreground flex items-center justify-between text-xs tabular-nums">
              <span>{Math.round(util)}% utilization</span>
              {a.balance_limit != null && (
                <span>
                  of {formatCurrency(a.balance_limit, a.iso_currency_code)}
                </span>
              )}
            </div>
            <div className="bg-muted h-1.5 overflow-hidden rounded-full">
              <div
                className={cn(
                  "h-full transition-all",
                  utilizationBarClass(util),
                )}
                style={{ width: `${util}%` }}
              />
            </div>
          </div>
        )}
        {a.iso_currency_code && (
          <div className="text-muted-foreground text-[10px]">
            {a.iso_currency_code}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function SecondaryCard({
  account: a,
  liability,
}: {
  account: AccountDetail;
  liability: boolean;
}) {
  // Depository / investment: show "available" balance and last update.
  // Credit / loan: show available credit (limit − current) and last update.
  // The point is to give users a second useful number, not the same number
  // twice.
  const rows: { label: string; value: string }[] = [];
  if (liability) {
    if (a.balance_limit != null && a.balance_current != null) {
      const avail = Math.max(0, a.balance_limit - a.balance_current);
      rows.push({
        label: "Available credit",
        value: formatCurrency(avail, a.iso_currency_code),
      });
    }
    if (a.balance_limit != null) {
      rows.push({
        label: "Credit limit",
        value: formatCurrency(a.balance_limit, a.iso_currency_code),
      });
    }
  } else if (a.balance_available != null) {
    rows.push({
      label: "Available",
      value: formatCurrency(a.balance_available, a.iso_currency_code),
    });
  }
  if (a.last_balance_update) {
    rows.push({
      label: "Last balance update",
      value: new Date(a.last_balance_update).toLocaleString(),
    });
  }

  if (a.official_name) {
    rows.push({ label: "Bank official name", value: a.official_name });
  }

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-muted-foreground text-xs font-medium">
          Details
        </CardTitle>
      </CardHeader>
      <CardContent>
        {rows.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            No additional balance or refresh information.
          </p>
        ) : (
          <dl className="space-y-2 text-sm">
            {rows.map((r) => (
              <div key={r.label} className="flex items-baseline justify-between gap-3">
                <dt className="text-muted-foreground text-xs">{r.label}</dt>
                <dd className="truncate text-right font-medium tabular-nums">
                  {r.value}
                </dd>
              </div>
            ))}
          </dl>
        )}
      </CardContent>
    </Card>
  );
}

function DetailSkeleton() {
  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <Skeleton className="size-12 rounded-lg" />
        <div className="space-y-2">
          <Skeleton className="h-5 w-40" />
          <Skeleton className="h-4 w-56" />
        </div>
      </div>
      <div className="grid gap-4 md:grid-cols-2">
        <Skeleton className="h-32 rounded-xl" />
        <Skeleton className="h-32 rounded-xl" />
      </div>
      <Skeleton className="h-40 rounded-xl" />
      <Skeleton className="h-48 rounded-xl" />
    </div>
  );
}
