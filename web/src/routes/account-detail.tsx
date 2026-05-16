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
  ArrowRight,
  Banknote,
  Eye,
  EyeOff,
  Landmark,
  Link2,
  Wallet,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { ColorRailCard } from "@/components/color-rail-card";
import { EmptyState } from "@/components/empty-state";
import { IdPill } from "@/components/id-pill";
import { SectionCard } from "@/components/section-card";
import { SoftBackButton } from "@/components/soft-back-button";
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
      <SoftBackButton to="/accounts">Back to accounts</SoftBackButton>

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
  const statusAlert = useMemo(() => connectionStatusAlert(a), [a]);

  return (
    <div className="space-y-6">
      <Hero account={a} onAddLink={onAddLink} />

      {statusAlert}

      <QuickActions account={a} />

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_18rem]">
        <div className="space-y-6 min-w-0">
          <AccountRecentTransactions
            accountShortId={a.short_id}
            transactions={a.recent_transactions.slice(0, 15)}
          />
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
        </div>

        <aside className="space-y-6">
          <AccountSettingsCard account={a} />
          <DetailsCard account={a} />
        </aside>
      </div>
    </div>
  );
}

// Hero condenses identity + classification + balance into one composed card
// — paralleling the iter-5 transaction-detail hero so detail pages across
// the app speak the same visual vocabulary. The left rail is tinted by the
// account's accounting role (success for assets, destructive for
// liabilities, muted when excluded) — the colour is meaningful, not
// decorative, so the eye lands on what kind of money this is before reading
// the number.
function Hero({
  account: a,
  onAddLink,
}: {
  account: AccountDetail;
  onAddLink: () => void;
}) {
  const Icon = accountTypeIcon(a.type);
  const liability = isLiability(a.type);
  const util = creditUtilization(a);
  const current = a.balance_current;
  const accent = a.excluded
    ? "var(--muted)"
    : liability
      ? "var(--destructive)"
      : "var(--success, oklch(0.65 0.18 145))";
  const directionLabel = liability ? "Balance owed" : "Current balance";

  return (
    <ColorRailCard
      accent={accent}
      footer={
        <>
          {!a.is_dependent_linked && (
            <Button variant="ghost" size="sm" onClick={onAddLink} className="h-7 gap-1.5 text-xs">
              <Link2 className="size-3.5" />
              Link account
            </Button>
          )}
          <Button variant="ghost" size="sm" asChild className="h-7 gap-1.5 text-xs">
            <Link to="/transactions" search={{ account: a.short_id }}>
              <Eye className="size-3.5" />
              View transactions
            </Link>
          </Button>
        </>
      }
    >
      <div className="grid gap-6 px-6 py-6 sm:px-7 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-start lg:gap-10">
        {/* Identity column */}
        <div className="min-w-0 space-y-3">
          <div className="flex items-start gap-4">
            <div
              className={cn(
                "bg-muted flex size-12 shrink-0 items-center justify-center rounded-lg",
                a.excluded && "opacity-60",
              )}
            >
              <Icon className="text-muted-foreground size-5" />
            </div>
            <div className="min-w-0 space-y-1">
              <p className="text-muted-foreground text-[10px] font-medium tracking-[0.12em] uppercase">
                {liability ? "Liability" : "Asset"}
              </p>
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
              <p className="text-muted-foreground flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs">
                <span className="capitalize">
                  {accountTypeLabel(a.type, a.subtype)}
                </span>
                {a.mask && (
                  <>
                    <span aria-hidden className="opacity-50">·</span>
                    <span className="tabular-nums">····{a.mask}</span>
                  </>
                )}
                {a.institution_name && (
                  <>
                    <span aria-hidden className="opacity-50">·</span>
                    <span>{a.institution_name}</span>
                  </>
                )}
                {a.connection_user_name && (
                  <>
                    <span aria-hidden className="opacity-50">·</span>
                    <span>{a.connection_user_name}</span>
                  </>
                )}
              </p>
            </div>
          </div>
        </div>

        {/* Amount column */}
        <div
          className={cn(
            "flex flex-col items-start gap-1.5",
            "lg:items-end lg:text-right",
          )}
        >
          <div
            className={cn(
              "inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-[10px] font-medium tracking-wide uppercase whitespace-nowrap",
              liability
                ? "bg-destructive/10 text-destructive"
                : "bg-success/10 text-success",
              a.excluded && "bg-muted text-muted-foreground",
            )}
          >
            {directionLabel}
          </div>
          <div
            className={cn(
              "font-semibold tabular-nums",
              "text-3xl sm:text-4xl",
              liability && current != null && current > 0 && "text-destructive/90",
              a.excluded && "opacity-60",
            )}
          >
            {current != null
              ? formatCurrency(current, a.iso_currency_code)
              : "—"}
          </div>

          {util != null ? (
            <div className="w-full max-w-[12rem] space-y-1.5 pt-2">
              <div className="text-muted-foreground flex items-center justify-between text-[11px] tabular-nums">
                <span>{Math.round(util)}% used</span>
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
          ) : a.balance_available != null ? (
            <p className="text-muted-foreground pt-1 text-[11px] tabular-nums">
              {formatCurrency(a.balance_available, a.iso_currency_code)} available
            </p>
          ) : a.iso_currency_code ? (
            <p className="text-muted-foreground pt-1 text-[11px]">
              {a.iso_currency_code}
            </p>
          ) : null}
        </div>
      </div>

    </ColorRailCard>
  );
}

function QuickActions({ account: a }: { account: AccountDetail }) {
  // The hero already carries the primary CTAs (link / view). Quick actions
  // here cover the lateral jumps the user reaches for from a detail page —
  // their connection (sync status, reauth) and the account-filter on the
  // transactions table. Matches the TX-detail "Jump to" pill row.
  const hasAny = !!(a.connection_short_id || a.short_id);
  if (!hasAny) return null;
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      <span className="text-muted-foreground mr-1 text-[10px] font-medium tracking-[0.1em] uppercase">
        Jump to
      </span>
      <Button variant="outline" size="sm" className="h-7 gap-1.5 text-xs" asChild>
        <Link to="/transactions" search={{ account: a.short_id }}>
          <Wallet className="size-3" />
          All transactions
        </Link>
      </Button>
      {a.connection_short_id && (
        <Button variant="outline" size="sm" className="h-7 gap-1.5 text-xs" asChild>
          <Link
            to="/connections/$id"
            params={{ id: a.connection_short_id }}
          >
            <Landmark className="size-3" />
            {a.institution_name ?? "Connection"}
          </Link>
        </Button>
      )}
    </div>
  );
}

function DetailsCard({ account: a }: { account: AccountDetail }) {
  const balanceRows: DetailRowData[] = compactRows([
    a.balance_limit != null && isLiability(a.type)
      ? {
          label: "Credit limit",
          value: formatCurrency(a.balance_limit, a.iso_currency_code),
        }
      : null,
    a.balance_available != null && !isLiability(a.type)
      ? {
          label: "Available",
          value: formatCurrency(a.balance_available, a.iso_currency_code),
        }
      : null,
    a.iso_currency_code ? { label: "Currency", value: a.iso_currency_code } : null,
    a.last_balance_update
      ? {
          label: "Last update",
          value: new Date(a.last_balance_update).toLocaleString(),
        }
      : null,
  ]);

  const providerRows: DetailRowData[] = compactRows([
    a.official_name ? { label: "Bank name", value: a.official_name } : null,
    a.mask ? { label: "Mask", value: `····${a.mask}`, mono: false } : null,
    a.subtype ? { label: "Subtype", value: a.subtype } : null,
  ]);

  const referenceRows: DetailRowData[] = compactRows([
    { label: "ID", value: a.short_id, mono: true },
  ]);

  return (
    <SectionCard title="Details" bodyClassName="space-y-5 px-5 py-5 text-sm">
      {balanceRows.length > 0 && (
        <DetailGroup label="Balance" rows={balanceRows} />
      )}
      {providerRows.length > 0 && (
        <DetailGroup label="Provider" rows={providerRows} />
      )}
      {referenceRows.length > 0 && (
        <DetailGroup label="Reference" rows={referenceRows} />
      )}
    </SectionCard>
  );
}

interface DetailRowData {
  label: string;
  value: string | null | undefined;
  mono?: boolean;
}

function compactRows(
  rows: (DetailRowData | null | undefined | false)[],
): DetailRowData[] {
  return rows.filter((r): r is DetailRowData => !!r && !!r.value);
}

function DetailGroup({ label, rows }: { label: string; rows: DetailRowData[] }) {
  if (rows.length === 0) return null;
  return (
    <div className="space-y-2.5">
      <h3 className="text-muted-foreground text-[10px] font-medium tracking-[0.1em] uppercase">
        {label}
      </h3>
      <dl className="space-y-2">
        {rows.map((row) => (
          <div
            key={row.label}
            className="flex items-baseline justify-between gap-3"
          >
            <dt className="text-muted-foreground shrink-0 text-xs">
              {row.label}
            </dt>
            <dd className="min-w-0 truncate text-right text-xs">
              {row.mono ? <IdPill value={row.value as string} /> : row.value}
            </dd>
          </div>
        ))}
      </dl>
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

function DetailSkeleton() {
  return (
    <div className="space-y-6">
      <div className="bg-card relative overflow-hidden rounded-xl border">
        <div className="bg-muted absolute inset-y-0 left-0 w-1" />
        <div className="grid gap-6 px-6 py-6 lg:grid-cols-[minmax(0,1fr)_auto]">
          <div className="space-y-3">
            <div className="flex items-start gap-4">
              <Skeleton className="size-12 rounded-lg" />
              <div className="space-y-2 py-1">
                <Skeleton className="h-3 w-20" />
                <Skeleton className="h-5 w-40" />
                <Skeleton className="h-3 w-48" />
              </div>
            </div>
          </div>
          <div className="space-y-2 lg:items-end">
            <Skeleton className="h-4 w-24" />
            <Skeleton className="h-9 w-32" />
            <Skeleton className="h-3 w-28" />
          </div>
        </div>
        <div className="border-t flex justify-end gap-2 px-6 py-3">
          <Skeleton className="h-7 w-24 rounded-md" />
          <Skeleton className="h-7 w-32 rounded-md" />
        </div>
      </div>
      <div className="flex gap-2">
        <Skeleton className="h-7 w-32 rounded-md" />
        <Skeleton className="h-7 w-32 rounded-md" />
      </div>
      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_18rem]">
        <Skeleton className="h-96 rounded-xl" />
        <div className="space-y-6">
          <Skeleton className="h-56 rounded-xl" />
          <Skeleton className="h-72 rounded-xl" />
        </div>
      </div>
    </div>
  );
}
