import { Link, useParams, useRouter } from "@tanstack/react-router";
import {
  ArrowDownLeft,
  ArrowLeft,
  ArrowUpRight,
  Building2,
  Clock,
  Receipt,
  Search,
  Wallet,
} from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/empty-state";
import { CategoryIconTile } from "@/components/category-icon-tile";
import { CategoryEditor } from "@/features/transactions/category-editor";
import { TagManager } from "@/features/transactions/tag-manager";
import { ActivityTimeline } from "@/features/transactions/activity-timeline";
import { CommentComposer } from "@/features/transactions/comment-composer";
import { useTransaction } from "@/api/queries/transactions";
import { formatAmount, formatLongDate } from "@/lib/format";
import { cn } from "@/lib/utils";
import type { Transaction } from "@/api/types";

export function TransactionDetailPage() {
  const { id } = useParams({ strict: false }) as { id?: string };
  const { data, isLoading, isError } = useTransaction(id);

  return (
    <div className="mx-auto max-w-5xl">
      <BackButton />

      {isLoading ? (
        <DetailSkeleton />
      ) : isError || !data ? (
        <EmptyState
          icon={Receipt}
          title="Transaction not found"
          description="It may have been deleted, or the link is wrong."
          action={
            <Button variant="outline" asChild>
              <Link to="/transactions">Back to transactions</Link>
            </Button>
          }
        />
      ) : (
        <DetailBody transaction={data} />
      )}
    </div>
  );
}

// BackButton renders as a real link to /transactions (so middle-click and
// "open in new tab" still work), but on a normal left-click it prefers
// `router.history.back()` — that lands the user on the exact list state they
// came from (filters, scroll, focus) instead of resetting to defaults.
function BackButton() {
  const router = useRouter();
  return (
    <Button variant="ghost" size="sm" asChild className="mb-4 -ml-2">
      <Link
        to="/transactions"
        onClick={(e) => {
          if (
            !e.defaultPrevented &&
            !e.metaKey &&
            !e.ctrlKey &&
            !e.shiftKey &&
            !e.altKey &&
            e.button === 0 &&
            window.history.length > 1
          ) {
            e.preventDefault();
            router.history.back();
          }
        }}
      >
        <ArrowLeft className="size-4" />
        Transactions
      </Link>
    </Button>
  );
}

function DetailBody({ transaction: t }: { transaction: Transaction }) {
  const merchantQuery = (t.provider_merchant_name ?? t.provider_name).trim();

  return (
    <div className="space-y-6">
      <Hero transaction={t} />

      <ClassifyStrip transaction={t} />

      <QuickActions transaction={t} merchantQuery={merchantQuery} />

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_18rem]">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Activity</CardTitle>
          </CardHeader>
          <CardContent className="space-y-6">
            <CommentComposer transactionId={t.id} />
            <ActivityTimeline transactionId={t.id} />
          </CardContent>
        </Card>

        <aside className="space-y-6">
          <DetailsCard transaction={t} />
        </aside>
      </div>
    </div>
  );
}

function Hero({ transaction: t }: { transaction: Transaction }) {
  const isInflow = t.amount < 0;
  const subtitle =
    t.provider_merchant_name && t.provider_merchant_name !== t.provider_name
      ? t.provider_merchant_name
      : null;
  const directionLabel = isInflow ? "Money in" : "Money out";
  const DirectionIcon = isInflow ? ArrowDownLeft : ArrowUpRight;
  return (
    <div
      className={cn(
        "bg-card flex flex-col gap-5 rounded-xl border p-5 sm:p-6",
        "lg:flex-row lg:items-start lg:justify-between lg:gap-6",
        t.pending && "border-dashed",
      )}
    >
      <div className="flex min-w-0 items-start gap-4">
        <CategoryIconTile
          icon={t.category?.icon}
          color={t.category?.color}
          size="lg"
        />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="truncate text-lg font-semibold tracking-tight">
              {t.provider_name}
            </h1>
            {t.pending && (
              <Badge variant="outline" className="text-muted-foreground">
                Pending
              </Badge>
            )}
          </div>
          {subtitle && (
            <p className="text-muted-foreground truncate text-sm">{subtitle}</p>
          )}
          <p className="text-muted-foreground mt-1 flex items-center gap-1.5 text-xs">
            <Clock className="size-3" aria-hidden />
            <span>{formatLongDate(t.date)}</span>
            {t.account_name && (
              <>
                <span aria-hidden>·</span>
                <span className="truncate">{t.account_name}</span>
              </>
            )}
          </p>
        </div>
      </div>

      <div
        className={cn(
          "flex flex-wrap items-center gap-x-3 gap-y-1.5",
          "lg:flex-col lg:items-end lg:gap-1.5 lg:text-right lg:whitespace-nowrap",
        )}
      >
        <div
          className={cn(
            "inline-flex shrink-0 items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium whitespace-nowrap",
            isInflow
              ? "bg-success/10 text-success"
              : "bg-muted text-muted-foreground",
          )}
        >
          <DirectionIcon className="size-3" aria-hidden />
          {directionLabel}
        </div>
        <div
          className={cn(
            "text-3xl font-semibold tabular-nums sm:text-4xl",
            isInflow && "text-success",
            t.pending && "opacity-80",
          )}
        >
          {formatAmount(t.amount, t.iso_currency_code)}
        </div>
        {t.pending && (
          <p className="text-muted-foreground basis-full text-xs lg:basis-auto">
            Amount may change once posted.
          </p>
        )}
      </div>
    </div>
  );
}

function ClassifyStrip({ transaction: t }: { transaction: Transaction }) {
  return (
    <Card>
      <CardContent className="grid gap-4 sm:grid-cols-2 sm:gap-6">
        <section className="space-y-2">
          <h2 className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
            Category
          </h2>
          <CategoryEditor
            transactionId={t.id}
            category={t.category}
            overridden={t.category_override}
          />
        </section>
        <section className="space-y-2">
          <h2 className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
            Tags
          </h2>
          <TagManager transactionId={t.id} tags={t.tags ?? []} />
        </section>
      </CardContent>
    </Card>
  );
}

function QuickActions({
  transaction: t,
  merchantQuery,
}: {
  transaction: Transaction;
  merchantQuery: string;
}) {
  // The transactions API already compacts account_id to short_id (see
  // internal/mcp/tools.go compactIDs + REST handler shape), so it can flow
  // straight into the transactions list `?account=` filter.
  return (
    <div className="flex flex-wrap gap-2">
      <Button variant="outline" size="sm" asChild>
        <Link
          to="/transactions"
          search={{ q: merchantQuery }}
          aria-label={`Find similar transactions matching ${merchantQuery}`}
        >
          <Search className="size-3.5" />
          Find similar
        </Link>
      </Button>
      {t.account_id && (
        <Button variant="outline" size="sm" asChild>
          <Link
            to="/transactions"
            search={{ account: t.account_id }}
            aria-label={`Show all transactions on ${t.account_name ?? "this account"}`}
          >
            <Wallet className="size-3.5" />
            All on account
          </Link>
        </Button>
      )}
      {t.category?.slug && (
        <Button variant="outline" size="sm" asChild>
          <Link
            to="/transactions"
            search={{ category: t.category.slug }}
            aria-label={`Show all ${t.category.display_name} transactions`}
          >
            <Building2 className="size-3.5" />
            All in category
          </Link>
        </Button>
      )}
    </div>
  );
}

function DetailsCard({ transaction: t }: { transaction: Transaction }) {
  const attributedDiffers =
    !!t.attributed_user_name && t.attributed_user_name !== t.user_name;

  const accountRows: DetailRowData[] = compactRows([
    { label: "Account", value: t.account_name },
    { label: "Household member", value: t.user_name },
    attributedDiffers
      ? {
          label: "Attributed to",
          value: t.attributed_user_name,
          hint: "This transaction counts toward this member, even though the account belongs to someone else.",
        }
      : null,
    { label: "Currency", value: t.iso_currency_code },
  ]);

  const providerRows: DetailRowData[] = compactRows([
    {
      label: "Authorized",
      value: t.authorized_date ? formatLongDate(t.authorized_date) : null,
    },
    { label: "Payment channel", value: titleize(t.provider_payment_channel) },
    {
      label: "Provider category",
      value: titleize(
        t.provider_category_detailed ?? t.provider_category_primary,
      ),
    },
  ]);

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Details</CardTitle>
      </CardHeader>
      <CardContent className="space-y-5 text-sm">
        <DetailGroup label="Account" rows={accountRows} />
        {providerRows.length > 0 && (
          <DetailGroup label="Provider" rows={providerRows} />
        )}
      </CardContent>
    </Card>
  );
}

interface DetailRowData {
  label: string;
  value: string | null | undefined;
  hint?: string;
}

function compactRows(
  rows: (DetailRowData | null | undefined)[],
): DetailRowData[] {
  return rows.filter((r): r is DetailRowData => !!r && !!r.value);
}

function DetailGroup({
  label,
  rows,
}: {
  label: string;
  rows: DetailRowData[];
}) {
  if (rows.length === 0) return null;
  return (
    <div className="space-y-2">
      <h3 className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
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
            <dd className="min-w-0 truncate text-right text-sm">
              {row.value}
              {row.hint && (
                <span className="text-muted-foreground mt-0.5 block text-[11px] leading-snug whitespace-normal">
                  {row.hint}
                </span>
              )}
            </dd>
          </div>
        ))}
      </dl>
    </div>
  );
}

// titleize converts SNAKE_CASE / snake_case provider strings into a readable
// label without mangling already-cased input.
function titleize(value: string | null | undefined): string | null {
  if (!value) return null;
  if (!/[_A-Z]/.test(value)) return value;
  return value
    .toLowerCase()
    .split(/[_\s]+/)
    .filter(Boolean)
    .map((w) => w[0].toUpperCase() + w.slice(1))
    .join(" ");
}

function DetailSkeleton() {
  return (
    <div className="space-y-6">
      <div className="bg-card rounded-xl border p-5 sm:p-6">
        <div className="flex flex-col gap-5 sm:flex-row sm:items-start sm:justify-between">
          <div className="flex items-start gap-4">
            <Skeleton className="size-12 rounded-md" />
            <div className="space-y-2 py-1">
              <Skeleton className="h-5 w-40" />
              <Skeleton className="h-4 w-24" />
              <Skeleton className="h-3 w-32" />
            </div>
          </div>
          <div className="space-y-2 sm:items-end sm:text-right">
            <Skeleton className="h-5 w-20" />
            <Skeleton className="h-9 w-32" />
          </div>
        </div>
      </div>
      <Skeleton className="h-24 rounded-xl" />
      <div className="flex gap-2">
        <Skeleton className="h-8 w-28 rounded-md" />
        <Skeleton className="h-8 w-28 rounded-md" />
        <Skeleton className="h-8 w-32 rounded-md" />
      </div>
      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_18rem]">
        <Skeleton className="h-72 rounded-xl" />
        <Skeleton className="h-56 rounded-xl" />
      </div>
    </div>
  );
}
