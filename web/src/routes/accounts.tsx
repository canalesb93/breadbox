import { useMemo } from "react";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { z } from "zod";
import { Banknote, Building2, Layers, Plus } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { EmptyState } from "@/components/empty-state";
import { ListCard } from "@/components/list-card";
import { Button } from "@/components/ui/button";
import { ListRowSkeleton } from "@/components/list-row-skeleton";
import { PageError } from "@/components/page-error";
import {
  ToggleGroup,
  ToggleGroupItem,
} from "@/components/ui/toggle-group";
import { useAccounts } from "@/api/queries/accounts";
import { useUsers } from "@/api/queries/users";
import { FamilyTabs } from "@/features/connections/family-tabs";
import { AccountRow } from "@/features/accounts/account-row";
import { AccountsSummary } from "@/features/accounts/accounts-summary";
import {
  groupAccounts,
  groupNetTotal,
  type AccountGroupBy,
} from "@/features/accounts/account-utils";
import { formatBalance } from "@/lib/format";

// Search-param schema.
//   user → "all" or a user short_id  (family-member filter)
//   group → "institution" (default) or "type" (grouping dimension)
//   action → "connect" (passthrough into the connect-bank flow on
//            /connections; sending users here would be confusing since this
//            page never opens its own sheet, so we just navigate.)
export const accountsSearchSchema = z.object({
  user: z.string().optional(),
  group: z.enum(["institution", "type"]).optional(),
});

type AccountsSearch = z.infer<typeof accountsSearchSchema>;

export function AccountsPage() {
  const search = useSearch({ strict: false }) as AccountsSearch;
  const navigate = useNavigate();
  const userFilter = search.user ?? "all";
  const groupBy: AccountGroupBy = search.group ?? "institution";

  const accountsQuery = useAccounts();
  const usersQuery = useUsers();

  // Counts per user, shared by the FamilyTabs labels and the filter below.
  // AccountResponse.user_id is the owner's short_id (see service/accounts.go
  // — `textPtr(r.UserShortID)`), so we can key directly on it.
  const countsByUserShortId = useMemo(() => {
    const out = new Map<string, number>();
    for (const u of usersQuery.data ?? []) {
      out.set(u.short_id, 0);
    }
    for (const a of accountsQuery.data ?? []) {
      if (!a.user_id) continue;
      out.set(a.user_id, (out.get(a.user_id) ?? 0) + 1);
    }
    return out;
  }, [accountsQuery.data, usersQuery.data]);

  const visible = useMemo(() => {
    const all = accountsQuery.data ?? [];
    if (userFilter === "all") return all;
    return all.filter((a) => a.user_id === userFilter);
  }, [accountsQuery.data, userFilter]);

  const groups = useMemo(() => groupAccounts(visible, groupBy), [visible, groupBy]);

  function setUserFilter(next: string) {
    navigate({
      to: "/accounts",
      search: (prev: AccountsSearch) => ({
        ...prev,
        user: next === "all" ? undefined : next,
      }),
      replace: true,
    });
  }

  function setGroupBy(next: AccountGroupBy) {
    navigate({
      to: "/accounts",
      search: (prev: AccountsSearch) => ({
        ...prev,
        group: next === "institution" ? undefined : next,
      }),
      replace: true,
    });
  }

  const isLoading = accountsQuery.isLoading;
  const isError = accountsQuery.isError;
  const accounts = accountsQuery.data ?? [];
  const totalCount = accounts.length;

  // Eyebrow matches the vocabulary of the other v2 list pages (Connections,
  // Tags, Categories, Transactions): one short string per state, "Showing N
  // of M" only when filtering narrows the view.
  const eyebrow = isLoading
    ? "Loading"
    : isError
      ? "Error loading"
      : totalCount === 0
        ? "No accounts yet"
        : visible.length === totalCount
          ? `${totalCount} ${totalCount === 1 ? "account" : "accounts"}`
          : `Showing ${visible.length} of ${totalCount}`;

  return (
    <>
      <PageHeader
        eyebrow={eyebrow}
        title="Accounts"
        description="Every bank account, credit card, loan, and investment Breadbox has synced. Click an account to edit, exclude, or link it."
        actions={
          accounts.length > 0 ? (
            <Button size="sm" asChild>
              <Link to="/connections" search={{ action: "connect" }}>
                <Plus className="size-4" />
                Connect bank
              </Link>
            </Button>
          ) : null
        }
      />

      {accounts.length > 0 && <AccountsSummary accounts={visible} />}

      {(usersQuery.data?.length ?? 0) > 1 && (
        <FamilyTabs
          users={usersQuery.data ?? []}
          value={userFilter}
          onChange={setUserFilter}
          counts={countsByUserShortId}
          totalCount={totalCount}
        />
      )}

      {accounts.length > 0 && (
        <div className="flex items-center justify-end">
          <ToggleGroup
            type="single"
            size="sm"
            value={groupBy}
            onValueChange={(v) => v && setGroupBy(v as AccountGroupBy)}
            variant="outline"
          >
            <ToggleGroupItem value="institution" aria-label="Group by institution">
              <Building2 className="size-3.5" /> Institution
            </ToggleGroupItem>
            <ToggleGroupItem value="type" aria-label="Group by type">
              <Layers className="size-3.5" /> Type
            </ToggleGroupItem>
          </ToggleGroup>
        </div>
      )}

      {isLoading ? (
        <ListCard
          rows={[0, 1, 2, 3]}
          getRowKey={(i) => i}
          renderRow={() => (
            <ListRowSkeleton
              density="comfortable"
              leading="lg-square"
              lines={2}
              trailing="value-stack"
              titleClassName="w-40"
              subtitleClassName="w-56"
              trailingTopClassName="w-20"
              trailingBottomClassName="w-8"
            />
          )}
        />
      ) : isError ? (
        <PageError
          resource="accounts"
          error={accountsQuery.error}
          onRetry={() => accountsQuery.refetch()}
          retrying={accountsQuery.isFetching}
        />
      ) : accounts.length === 0 ? (
        <EmptyState
          icon={Banknote}
          title="No accounts yet"
          description="Link a bank via Plaid or Teller and accounts will land here as soon as the first sync completes."
          action={
            <Button asChild>
              <Link to="/connections" search={{ action: "connect" }}>
                <Plus className="size-4" />
                Connect a bank
              </Link>
            </Button>
          }
        />
      ) : visible.length === 0 ? (
        <EmptyState
          icon={Banknote}
          title="No accounts for this filter"
          description="Switch family member, or clear the filter to see every account in the household."
        />
      ) : (
        <div className="flex flex-col gap-4">
          {groups.map((g) => {
            const totals = groupNetTotal(g.accounts);
            return (
              <ListCard
                key={g.key}
                title={g.label}
                rows={g.accounts}
                getRowKey={(a) => a.id}
                renderRow={(a) => <AccountRow account={a} />}
                action={
                  <div className="flex items-center gap-3">
                    {totals.currency != null && (
                      <span className="text-sm font-medium tabular-nums whitespace-nowrap">
                        {formatBalance(totals.total, totals.currency)}
                      </span>
                    )}
                    <span className="bg-muted text-muted-foreground inline-flex h-5 min-w-5 items-center justify-center rounded-full px-1.5 text-[11px] font-medium tabular-nums">
                      {g.accounts.length}
                    </span>
                  </div>
                }
              />
            );
          })}
        </div>
      )}
    </>
  );
}
