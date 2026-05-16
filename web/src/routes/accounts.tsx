import { useMemo } from "react";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { z } from "zod";
import { Banknote, Building2, Layers, Loader2, Plus } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { EmptyState } from "@/components/empty-state";
import { Button } from "@/components/ui/button";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  ToggleGroup,
  ToggleGroupItem,
} from "@/components/ui/toggle-group";
import { useAccounts } from "@/api/queries/accounts";
import { useUsers } from "@/api/queries/users";
import { FamilyTabs } from "@/features/connections/family-tabs";
import { AccountCard } from "@/features/accounts/account-card";
import { AccountsSummary } from "@/features/accounts/accounts-summary";
import {
  groupAccounts,
  type AccountGroupBy,
} from "@/features/accounts/account-utils";

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

export type AccountsSearch = z.infer<typeof accountsSearchSchema>;

export function AccountsPage() {
  const search = useSearch({ strict: false }) as AccountsSearch;
  const navigate = useNavigate();
  const userFilter = search.user ?? "all";
  const groupBy: AccountGroupBy = search.group ?? "institution";

  const accountsQuery = useAccounts();
  const usersQuery = useUsers();

  // Counts per user, shared by the FamilyTabs labels and by the filter
  // resolution below. We key on user short_id to match the tab's value.
  const countsByUserShortId = useMemo(() => {
    const out = new Map<string, number>();
    const idToShortId = new Map<string, string>();
    for (const u of usersQuery.data ?? []) {
      idToShortId.set(u.id, u.short_id);
      out.set(u.short_id, 0);
    }
    for (const a of accountsQuery.data ?? []) {
      if (!a.user_id) continue;
      const sid = idToShortId.get(a.user_id);
      if (!sid) continue;
      out.set(sid, (out.get(sid) ?? 0) + 1);
    }
    return out;
  }, [accountsQuery.data, usersQuery.data]);

  // Resolve the selected short_id back to a UUID so we can filter the
  // accounts (which carry the user's UUID, not short_id).
  const filterUserId = useMemo(() => {
    if (userFilter === "all") return null;
    return usersQuery.data?.find((u) => u.short_id === userFilter)?.id ?? null;
  }, [userFilter, usersQuery.data]);

  const visible = useMemo(() => {
    const all = accountsQuery.data ?? [];
    if (filterUserId == null) return all;
    return all.filter((a) => a.user_id === filterUserId);
  }, [accountsQuery.data, filterUserId]);

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

  return (
    <>
      <PageHeader
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

      {accounts.length > 0 && (
        <div className="-mt-4 mb-4">
          <AccountsSummary accounts={visible} />
        </div>
      )}

      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        {(usersQuery.data?.length ?? 0) > 1 ? (
          <FamilyTabs
            users={usersQuery.data ?? []}
            value={userFilter}
            onChange={setUserFilter}
            counts={countsByUserShortId}
            totalCount={totalCount}
          />
        ) : (
          <div />
        )}

        {accounts.length > 0 && (
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
        )}
      </div>

      {isLoading ? (
        <div className="text-muted-foreground flex items-center justify-center gap-2 py-12 text-sm">
          <Loader2 className="size-4 animate-spin" /> Loading accounts…
        </div>
      ) : isError ? (
        <Alert variant="destructive">
          <AlertTitle>Couldn't load accounts</AlertTitle>
          <AlertDescription>
            {accountsQuery.error instanceof Error
              ? accountsQuery.error.message
              : "Try refreshing the page."}
          </AlertDescription>
        </Alert>
      ) : accounts.length === 0 ? (
        <EmptyState
          icon={Banknote}
          title="No accounts yet"
          description="Connect a bank to start syncing accounts and transactions."
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
          title="No accounts for this filter"
          description="Switch family member or clear the filter to see other accounts."
        />
      ) : (
        <div className="space-y-6">
          {groups.map((g) => (
            <section key={g.key} className="space-y-2">
              <div className="flex items-baseline justify-between">
                <h2 className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
                  {g.label}
                </h2>
                <span className="text-muted-foreground text-xs tabular-nums">
                  {g.accounts.length}
                </span>
              </div>
              <div className="grid grid-cols-1 gap-2 md:grid-cols-2">
                {g.accounts.map((a) => (
                  <AccountCard key={a.id} account={a} />
                ))}
              </div>
            </section>
          ))}
        </div>
      )}
    </>
  );
}
