import { Link, useParams } from "@tanstack/react-router";
import {
  ArrowDownLeft,
  ArrowUpRight,
  Building2,
  Receipt,
  Search,
  Wallet,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/empty-state";
import { CategoryIconTile } from "@/components/category-icon-tile";
import { ColorRailCard } from "@/components/color-rail-card";
import {
  DetailList,
  compactDetailRows,
  type DetailRowData,
} from "@/components/detail-list";
import { Eyebrow } from "@/components/eyebrow";
import { SectionCard } from "@/components/section-card";
import { SoftBackButton } from "@/components/soft-back-button";
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
      <SoftBackButton to="/transactions">Back to transactions</SoftBackButton>

      {isLoading ? (
        <DetailSkeleton />
      ) : isError || !data ? (
        <EmptyState
          icon={Receipt}
          title="Transaction not found"
          description="This transaction may have been deleted, or the link is out of date. Head back to the transactions list to pick another."
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

function DetailBody({ transaction: t }: { transaction: Transaction }) {
  const merchantQuery = (t.provider_merchant_name ?? t.provider_name).trim();

  return (
    <div className="space-y-6">
      <Hero transaction={t} />

      <QuickActions transaction={t} merchantQuery={merchantQuery} />

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_18rem]">
        <SectionCard
          title="Activity"
          bodyClassName="space-y-5 px-5 py-5"
        >
          <CommentComposer transactionId={t.id} />
          <Separator />
          <ActivityTimeline transactionId={t.id} />
        </SectionCard>

        <aside className="space-y-6">
          <DetailsCard transaction={t} />
        </aside>
      </div>
    </div>
  );
}

// Hero condenses the four most important things — identity, direction, amount,
// classification — into one card so the page lands with intent before the eye
// drops into the activity feed. Left rail = category color stripe (or muted
// when uncategorised) so the page has a single point of color anchored to the
// transaction's classification.
function Hero({ transaction: t }: { transaction: Transaction }) {
  const isInflow = t.amount < 0;
  const subtitle =
    t.provider_merchant_name && t.provider_merchant_name !== t.provider_name
      ? t.provider_merchant_name
      : null;
  const DirectionIcon = isInflow ? ArrowDownLeft : ArrowUpRight;
  const directionLabel = isInflow ? "Money in" : "Money out";
  const accent = t.category?.color ?? null;

  return (
    <ColorRailCard
      accent={accent}
      cardClassName={cn(t.pending && "border-dashed")}
    >
      {/* Hero grid: on mobile the rows flow identity → amount → classify so
          the amount sits in the priority slot right under the title. On lg
          the identity + classify stack on the left and the amount column
          docks to the right (identity stays anchored at the top via
          `lg:row-span-2`). */}
      <div className="grid gap-5 px-5 py-5 sm:gap-6 sm:px-7 sm:py-6 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-start lg:gap-x-10 lg:gap-y-5">
        {/* Identity row (full width on mobile, left column on lg) */}
        <div className="flex min-w-0 items-start gap-3 sm:gap-4 lg:row-start-1">
          <CategoryIconTile
            icon={t.category?.icon}
            color={t.category?.color}
            size="lg"
          />
          <div className="min-w-0 space-y-1">
            <Eyebrow as="p" variant="hero">
              Transaction
            </Eyebrow>
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="truncate text-xl font-semibold tracking-tight">
                {t.provider_name}
              </h1>
              {t.pending && (
                <Badge
                  variant="outline"
                  className="text-muted-foreground border-dashed text-[10px] font-medium tracking-wide uppercase"
                >
                  Pending
                </Badge>
              )}
            </div>
            <p className="text-muted-foreground flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs">
              <span>{formatLongDate(t.date)}</span>
              {(subtitle || t.account_name) && (
                <span aria-hidden className="opacity-50">
                  ·
                </span>
              )}
              {subtitle && <span className="truncate">{subtitle}</span>}
              {subtitle && t.account_name && (
                <span aria-hidden className="opacity-50">
                  ·
                </span>
              )}
              {t.account_name && (
                <span className="truncate">{t.account_name}</span>
              )}
            </p>
          </div>
        </div>

        {/* Amount row (sits below identity on mobile so the headline number is
            visible without a scroll past tags + category; docks to the right
            column on lg). `lg:row-span-2` keeps it aligned with the identity
            row instead of straddling the classify strip beneath. */}
        <div
          className={cn(
            "flex flex-col items-start gap-1.5",
            "lg:row-start-1 lg:row-span-2 lg:items-end lg:text-right",
          )}
        >
          <div
            className={cn(
              "inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-[10px] font-medium tracking-wide uppercase whitespace-nowrap",
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
              "font-semibold tabular-nums",
              "text-3xl sm:text-4xl",
              isInflow && "text-success",
              t.pending && "opacity-80",
            )}
          >
            {formatAmount(t.amount, t.iso_currency_code)}
          </div>
          {t.pending && (
            <p className="text-muted-foreground max-w-[12rem] text-[11px]">
              Amount may change once posted.
            </p>
          )}
        </div>

        {/* Classify strip — last on mobile so the priority order reads
            identity → amount → tags/category. Spans the full left column on
            lg via `lg:row-start-2`. */}
        <div className="space-y-5 lg:row-start-2">
          <Separator />
          <div className="grid gap-4 sm:grid-cols-2 sm:gap-6">
            <ClassifyField label="Category">
              <CategoryEditor
                transactionId={t.id}
                category={t.category}
                overridden={t.category_override}
              />
            </ClassifyField>
            <ClassifyField label="Tags">
              <div className="min-h-9 flex items-center">
                <TagManager transactionId={t.id} tags={t.tags ?? []} />
              </div>
            </ClassifyField>
          </div>
        </div>
      </div>
    </ColorRailCard>
  );
}

function ClassifyField({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-1.5">
      <Eyebrow as="p">{label}</Eyebrow>
      {children}
    </div>
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
    <div className="flex flex-wrap items-center gap-1.5">
      <Eyebrow className="mr-1">Jump to</Eyebrow>
      <Button variant="outline" size="sm" className="h-7 gap-1.5 text-xs" asChild>
        <Link
          to="/transactions"
          search={{ q: merchantQuery }}
          aria-label={`Find similar transactions matching ${merchantQuery}`}
        >
          <Search className="size-3" />
          Similar transactions
        </Link>
      </Button>
      {t.account_id && (
        <Button variant="outline" size="sm" className="h-7 gap-1.5 text-xs" asChild>
          <Link
            to="/transactions"
            search={{ account: t.account_id }}
            aria-label={`Show all transactions on ${t.account_name ?? "this account"}`}
          >
            <Wallet className="size-3" />
            {t.account_name ?? "All on account"}
          </Link>
        </Button>
      )}
      {t.category?.slug && (
        <Button variant="outline" size="sm" className="h-7 gap-1.5 text-xs" asChild>
          <Link
            to="/transactions"
            search={{ category: t.category.slug }}
            aria-label={`Show all ${t.category.display_name} transactions`}
          >
            <Building2 className="size-3" />
            {t.category.display_name}
          </Link>
        </Button>
      )}
    </div>
  );
}

function DetailsCard({ transaction: t }: { transaction: Transaction }) {
  const attributedDiffers =
    !!t.attributed_user_name && t.attributed_user_name !== t.user_name;

  const accountRows: DetailRowData[] = compactDetailRows([
    { label: "Name", value: t.account_name },
    { label: "Member", value: t.user_name },
    attributedDiffers
      ? {
          label: "Attributed to",
          value: t.attributed_user_name,
          hint: "Counts toward this member, even though the account belongs to someone else.",
        }
      : null,
    { label: "Currency", value: t.iso_currency_code },
  ]);

  // Drop low-signal provider rows: a `Channel: other` row reads as
  // "Breadbox didn't get a channel" noise; same for an empty category. We
  // already render the detailed category in the hero. The Details sidebar
  // earns its keep when every row carries information the user couldn't get
  // upstream — pad it out and it reads as a dump.
  const channelLabel = titleize(t.provider_payment_channel);
  const providerCategory = titleize(
    t.provider_category_detailed ?? t.provider_category_primary,
  );
  const providerRows: DetailRowData[] = compactDetailRows([
    {
      label: "Authorized",
      value: t.authorized_date ? formatLongDate(t.authorized_date) : null,
    },
    channelLabel && channelLabel.toLowerCase() !== "other"
      ? { label: "Channel", value: channelLabel }
      : null,
    providerCategory ? { label: "Category", value: providerCategory } : null,
  ]);

  const referenceRows: DetailRowData[] = compactDetailRows([
    { label: "ID", value: t.short_id, mono: true },
  ]);

  return (
    <SectionCard title="Details" bodyClassName="space-y-5 px-5 py-5 text-sm">
      <DetailList label="Account" rows={accountRows} />
      <DetailList label="Provider" rows={providerRows} />
      <DetailList label="Reference" rows={referenceRows} />
    </SectionCard>
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
      <div className="bg-card relative overflow-hidden rounded-xl border">
        <div className="bg-muted absolute inset-y-0 left-0 w-1" />
        <div className="grid gap-5 px-5 py-5 sm:gap-6 sm:px-7 sm:py-6 lg:grid-cols-[minmax(0,1fr)_auto] lg:gap-x-10 lg:gap-y-5">
          <div className="flex items-start gap-3 sm:gap-4 lg:row-start-1">
            <Skeleton className="size-12 rounded-md" />
            <div className="space-y-2 py-1">
              <Skeleton className="h-3 w-20" />
              <Skeleton className="h-5 w-40" />
              <Skeleton className="h-3 w-32" />
            </div>
          </div>
          <div className="space-y-2 lg:row-start-1 lg:row-span-2 lg:items-end">
            <Skeleton className="h-5 w-20" />
            <Skeleton className="h-9 w-32" />
          </div>
          <div className="space-y-5 lg:row-start-2">
            <Separator />
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-1.5">
                <Skeleton className="h-3 w-16" />
                <Skeleton className="h-9 w-full rounded-md" />
              </div>
              <div className="space-y-1.5">
                <Skeleton className="h-3 w-12" />
                <Skeleton className="h-9 w-24 rounded-md" />
              </div>
            </div>
          </div>
        </div>
      </div>
      <div className="flex gap-2">
        <Skeleton className="h-7 w-32 rounded-md" />
        <Skeleton className="h-7 w-32 rounded-md" />
      </div>
      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_18rem]">
        <Skeleton className="h-72 rounded-xl" />
        <Skeleton className="h-56 rounded-xl" />
      </div>
    </div>
  );
}
