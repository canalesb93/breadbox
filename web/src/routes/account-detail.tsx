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
  PowerOff,
  Wallet,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { ActionPill } from "@/components/action-pill";
import { ColorRailCard } from "@/components/color-rail-card";
import { HeroGrid } from "@/components/hero-grid";
import { DetailPageSkeleton } from "@/components/detail-page-skeleton";
import {
  DetailList,
  compactDetailRows,
  type DetailRowData,
} from "@/components/detail-list";
import { EmptyState } from "@/components/empty-state";
import { Eyebrow } from "@/components/eyebrow";
import { PageError } from "@/components/page-error";
import { JumpToPill, JumpToRow } from "@/components/jump-to-pill";
import { MetaBadge } from "@/components/meta-badge";
import { SectionCard } from "@/components/section-card";
import { SoftBackButton } from "@/components/soft-back-button";
import { StatusPanel } from "@/components/status-panel";
import { useAccount, useAccounts } from "@/api/queries/accounts";
import { ApiError } from "@/api/client";
import type { AccountDetail } from "@/api/types";
import {
  accountLabel,
  accountTypeIcon,
  accountTypeLabel,
  creditUtilization,
  isLiability,
  utilizationBarClass,
} from "@/features/accounts/account-utils";
import { formatBalance, formatLongDate } from "@/lib/format";
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
      ) : acctQuery.isError &&
        !(
          acctQuery.error instanceof ApiError && acctQuery.error.status === 404
        ) ? (
        <PageError
          resource="this account"
          error={acctQuery.error}
          onRetry={() => acctQuery.refetch()}
          retrying={acctQuery.isFetching}
        />
      ) : !acctQuery.data ? (
        <EmptyState
          variant="card"
          icon={Banknote}
          title="Account not found"
          description="This account may have been disconnected, or the link is out of date. Head back to the accounts list to pick another."
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
            <ActionPill onClick={onAddLink}>
              <Link2 className="size-3.5" />
              Link account
            </ActionPill>
          )}
          <ActionPill asChild>
            <Link to="/transactions" search={{ account: a.short_id }}>
              <Eye className="size-3.5" />
              View transactions
            </Link>
          </ActionPill>
        </>
      }
    >
      <HeroGrid>
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
              <Eyebrow as="p" variant="hero">
                {liability ? "Liability" : "Asset"}
              </Eyebrow>
              <div className="flex flex-wrap items-center gap-2">
                <h1 className="truncate text-xl font-semibold tracking-tight">
                  {accountLabel(a)}
                </h1>
                {a.excluded && (
                  <MetaBadge icon={EyeOff}>Excluded</MetaBadge>
                )}
                {a.is_dependent_linked && (
                  <MetaBadge icon={Link2} variant="secondary">
                    Linked dependent
                  </MetaBadge>
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
              ? formatBalance(current, a.iso_currency_code)
              : "—"}
          </div>

          {util != null ? (
            <div className="w-full max-w-[12rem] space-y-1.5 pt-2">
              <div className="text-muted-foreground flex items-center justify-between text-[11px] tabular-nums">
                <span>{Math.round(util)}% used</span>
                {a.balance_limit != null && (
                  <span>
                    of {formatBalance(a.balance_limit, a.iso_currency_code)}
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
              {formatBalance(a.balance_available, a.iso_currency_code)} available
            </p>
          ) : a.iso_currency_code ? (
            <p className="text-muted-foreground pt-1 text-[11px]">
              {a.iso_currency_code}
            </p>
          ) : null}
        </div>
      </HeroGrid>

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
    <JumpToRow>
      <JumpToPill asChild>
        <Link to="/transactions" search={{ account: a.short_id }}>
          <Wallet className="size-3" />
          All transactions
        </Link>
      </JumpToPill>
      {a.connection_short_id && (
        <JumpToPill asChild>
          <Link
            to="/connections/$id"
            params={{ id: a.connection_short_id }}
          >
            <Landmark className="size-3" />
            {a.institution_name ?? "Connection"}
          </Link>
        </JumpToPill>
      )}
    </JumpToRow>
  );
}

function DetailsCard({ account: a }: { account: AccountDetail }) {
  const balanceRows: DetailRowData[] = compactDetailRows([
    a.balance_limit != null && isLiability(a.type)
      ? {
          label: "Credit limit",
          value: formatBalance(a.balance_limit, a.iso_currency_code),
        }
      : null,
    a.balance_available != null && !isLiability(a.type)
      ? {
          label: "Available",
          value: formatBalance(a.balance_available, a.iso_currency_code),
        }
      : null,
    a.iso_currency_code ? { label: "Currency", value: a.iso_currency_code } : null,
    a.last_balance_update
      ? {
          // Short date keeps the row from wrapping on a 375px viewport; the
          // exact wall-clock seconds are noise — last-sync precision lives
          // on the connection page.
          label: "Last update",
          value: formatLongDate(a.last_balance_update.slice(0, 10)),
        }
      : null,
  ]);

  const providerRows: DetailRowData[] = compactDetailRows([
    a.official_name ? { label: "Bank name", value: a.official_name } : null,
    a.mask ? { label: "Mask", value: `····${a.mask}`, mono: false } : null,
    a.subtype ? { label: "Subtype", value: a.subtype } : null,
  ]);

  const referenceRows: DetailRowData[] = compactDetailRows([
    { label: "ID", value: a.short_id, mono: true },
  ]);

  return (
    <SectionCard title="Details" bodyClassName="space-y-5 px-5 py-5 text-sm">
      <DetailList label="Balance" rows={balanceRows} />
      <DetailList label="Provider" rows={providerRows} />
      <DetailList label="Reference" rows={referenceRows} />
    </SectionCard>
  );
}

function connectionStatusAlert(a: AccountDetail) {
  const status = a.connection_status;
  if (!status || status === "active") return null;

  // Tone-tinted `<StatusPanel>` so account-detail speaks the same banner
  // vocabulary as Setup, Providers, Home attention-panel, and the iter-35
  // 404/Error pages. Optional trailing slot carries the "Open connection"
  // CTA — same shape as the iter-35 error-page Reload button. On narrow
  // viewports StatusPanel's `flex items-start gap-3` row keeps the icon
  // tile + heading + trailing CTA aligned where the previous
  // `<AlertDescription>` flex-wrap row would wrap the CTA below the body
  // text in a half-aligned column.
  const openCta = a.connection_short_id ? (
    <ActionPill variant="outline" asChild>
      <Link to="/connections/$id" params={{ id: a.connection_short_id }}>
        Open connection
        <ArrowRight className="size-3.5" />
      </Link>
    </ActionPill>
  ) : undefined;

  if (status === "pending_reauth") {
    return (
      <StatusPanel
        tone="warning"
        icon={AlertTriangle}
        heading="Connection needs re-authentication"
        body="Recent syncs may be stale until you reconnect."
        trailing={openCta}
      />
    );
  }
  if (status === "error") {
    return (
      <StatusPanel
        tone="destructive"
        icon={AlertTriangle}
        heading="Connection error"
        body="Sync is currently failing. Check the connection for details."
        trailing={openCta}
      />
    );
  }
  if (status === "disconnected") {
    return (
      <StatusPanel
        tone="info"
        icon={PowerOff}
        heading="Connection disconnected"
        body="This account is read-only — its parent connection no longer syncs. Historical transactions remain."
        trailing={openCta}
      />
    );
  }
  return null;
}

function DetailSkeleton() {
  return (
    <DetailPageSkeleton
      hero={{ tileShape: "rounded-lg", withFooter: true }}
      jumpPills={2}
      main={["h-96"]}
      sidebar={["h-56", "h-72"]}
    />
  );
}
