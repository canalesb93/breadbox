import { Link, useParams } from "@tanstack/react-router";
import { ArrowLeft, Receipt } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/empty-state";
import { CategoryIconTile } from "@/components/category-icon-tile";
import { CategoryEditor } from "@/features/transactions/category-editor";
import { TagManager } from "@/features/transactions/tag-manager";
import { ActivityTimeline } from "@/features/transactions/activity-timeline";
import { useTransaction } from "@/api/queries/transactions";
import { formatAmount, formatLongDate } from "@/lib/format";
import { cn } from "@/lib/utils";
import type { Transaction } from "@/api/types";

export function TransactionDetailPage() {
  const { id } = useParams({ strict: false }) as { id?: string };
  const { data, isLoading, isError } = useTransaction(id);

  return (
    <div className="mx-auto max-w-4xl">
      <Button variant="ghost" size="sm" asChild className="mb-4 -ml-2">
        <Link to="/transactions">
          <ArrowLeft className="size-4" />
          Transactions
        </Link>
      </Button>

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

function DetailBody({ transaction: t }: { transaction: Transaction }) {
  const isInflow = t.amount < 0;
  const subtitle =
    t.provider_merchant_name && t.provider_merchant_name !== t.provider_name
      ? t.provider_merchant_name
      : null;

  return (
    <div className="space-y-6">
      {/* Hero */}
      <div className="flex items-start gap-4">
        <CategoryIconTile
          icon={t.category?.icon}
          color={t.category?.color}
          size="lg"
        />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <h1 className="truncate text-xl font-semibold tracking-tight">
              {t.provider_name}
            </h1>
            {t.pending && (
              <Badge variant="outline" className="text-muted-foreground">
                Pending
              </Badge>
            )}
          </div>
          {subtitle && (
            <p className="text-muted-foreground text-sm">{subtitle}</p>
          )}
          <p className="text-muted-foreground mt-0.5 text-sm">
            {formatLongDate(t.date)}
          </p>
        </div>
        <div
          className={cn(
            "text-xl font-semibold tabular-nums whitespace-nowrap",
            isInflow && "text-emerald-600 dark:text-emerald-500",
          )}
        >
          {formatAmount(t.amount, t.iso_currency_code)}
        </div>
      </div>

      <div className="grid gap-6 md:grid-cols-[1fr_18rem]">
        {/* Main column */}
        <div className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Details</CardTitle>
            </CardHeader>
            <CardContent className="grid grid-cols-2 gap-x-4 gap-y-3 text-sm">
              <DetailRow label="Account" value={t.account_name} />
              <DetailRow label="Household member" value={t.user_name} />
              <DetailRow
                label="Authorized"
                value={t.authorized_date ? formatLongDate(t.authorized_date) : null}
              />
              <DetailRow
                label="Payment channel"
                value={t.provider_payment_channel}
              />
              <DetailRow
                label="Provider category"
                value={
                  t.provider_category_detailed ?? t.provider_category_primary
                }
              />
              <DetailRow label="Currency" value={t.iso_currency_code} />
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-base">Activity</CardTitle>
            </CardHeader>
            <CardContent>
              <ActivityTimeline transactionId={t.id} />
            </CardContent>
          </Card>
        </div>

        {/* Sidebar */}
        <div className="space-y-6">
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
        </div>
      </div>
    </div>
  );
}

function DetailRow({
  label,
  value,
}: {
  label: string;
  value: string | null | undefined;
}) {
  return (
    <div className="space-y-0.5">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className={cn(!value && "text-muted-foreground")}>{value ?? "—"}</dd>
    </div>
  );
}

function DetailSkeleton() {
  return (
    <div className="space-y-6">
      <div className="flex items-start gap-4">
        <Skeleton className="size-12 rounded-md" />
        <div className="flex-1 space-y-2 py-1">
          <Skeleton className="h-5 w-1/3" />
          <Skeleton className="h-4 w-1/4" />
        </div>
        <Skeleton className="h-6 w-20" />
      </div>
      <div className="grid gap-6 md:grid-cols-[1fr_18rem]">
        <Skeleton className="h-48 rounded-xl" />
        <Skeleton className="h-32 rounded-xl" />
      </div>
    </div>
  );
}
