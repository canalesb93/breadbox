import { useCallback, useEffect, useState } from "react";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { z } from "zod";
import { Loader2, Plus, Wand2 } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { EmptyState } from "@/components/empty-state";
import { PaginationBar } from "@/components/pagination-bar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Skeleton } from "@/components/ui/skeleton";
import { withMutationToast } from "@/lib/mutation-toast";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import {
  RULES_PAGE_SIZE,
  useDeleteRule,
  useRules,
  useToggleRule,
} from "@/api/queries/rules";
import type { RulesFilters } from "@/api/queries/rules";
import type { TransactionRule } from "@/api/types";
import { RuleRow } from "@/features/rules/rule-row";

export const rulesSearchSchema = z.object({
  q: z.string().optional(),
  enabled: z.enum(["true", "false"]).optional(),
  sort: z
    .enum(["priority", "hit_count", "last_hit_at", "created_at", "name"])
    .optional(),
  p: z.coerce.number().int().min(1).optional(),
});

export type RulesSearch = z.infer<typeof rulesSearchSchema>;

function searchToFilters(s: RulesSearch): RulesFilters {
  return {
    search: s.q,
    enabled:
      s.enabled === "true" ? true : s.enabled === "false" ? false : undefined,
    sortBy: s.sort,
  };
}

export function RulesPage() {
  const navigate = useNavigate();
  const search = useSearch({ strict: false }) as RulesSearch;

  // Local search input → debounced → URL. Same pattern as TransactionsPage.
  const [query, setQuery] = useState(search.q ?? "");
  const debounced = useDebouncedValue(query, 300);

  useEffect(() => {
    const q = debounced.trim() || undefined;
    navigate({
      to: ".",
      search: (prev: Record<string, unknown>) => ({ ...prev, q, p: undefined }),
    });
  }, [debounced, navigate]);

  useEffect(() => {
    setQuery(search.q ?? "");
  }, [search.q]);

  const setFilter = useCallback(
    (patch: Partial<RulesSearch>) => {
      navigate({
        to: ".",
        search: (prev: Record<string, unknown>) => ({
          ...prev,
          ...patch,
          p: undefined,
        }),
      });
    },
    [navigate],
  );

  const filters = searchToFilters(search);
  const page = search.p ?? 1;
  const rulesQuery = useRules(filters, page);
  const toggleRule = useToggleRule();
  const deleteRule = useDeleteRule();
  const rows = rulesQuery.data?.rules ?? [];

  const [pendingDelete, setPendingDelete] = useState<TransactionRule | null>(
    null,
  );

  const onToggle = useCallback(
    async (rule: TransactionRule) => {
      const nextEnabled = !rule.enabled;
      await withMutationToast(
        () => toggleRule.mutateAsync({ id: rule.short_id, enabled: nextEnabled }),
        {
          success: nextEnabled
            ? `Enabled rule "${rule.name}".`
            : `Disabled rule "${rule.name}".`,
        },
      );
    },
    [toggleRule],
  );

  const onDeleteConfirm = useCallback(async () => {
    if (!pendingDelete) return;
    const target = pendingDelete;
    setPendingDelete(null);
    const ok = await withMutationToast(
      () => deleteRule.mutateAsync(target.short_id),
      { success: `Deleted rule "${target.name}".` },
    );
    if (!ok) {
      // Restore the confirm so the user can retry without re-finding the row.
      setPendingDelete(target);
    }
  }, [deleteRule, pendingDelete]);

  const total = rulesQuery.data?.total ?? rows.length;

  const hasActiveFilters = !!search.q || !!search.enabled;
  const isLoading = rulesQuery.isLoading;
  const isFetching = rulesQuery.isFetching;
  const isError = rulesQuery.isError;

  const showCount = !isLoading && !isError && rows.length > 0;

  return (
    <>
      <PageHeader
        title="Rules"
        description="Automatically categorize, tag, or comment on transactions during sync."
        actions={
          <Button asChild size="sm">
            <Link to="/rules/new">
              <Plus className="size-4" /> New rule
            </Link>
          </Button>
        }
      />

      <RulesToolbar
        query={query}
        onQueryChange={setQuery}
        enabled={search.enabled}
        onEnabledChange={(v) =>
          setFilter({ enabled: v === "all" ? undefined : (v as "true" | "false") })
        }
        sort={search.sort ?? "priority"}
        onSortChange={(v) =>
          setFilter({ sort: v === "priority" ? undefined : (v as RulesSearch["sort"]) })
        }
      />

      {showCount && (
        <CountLine count={total} fetching={isFetching} />
      )}

      {isError ? (
        <Alert variant="destructive">
          <AlertTitle>Couldn't load rules</AlertTitle>
          <AlertDescription>
            {rulesQuery.error instanceof Error
              ? rulesQuery.error.message
              : "Try refreshing the page."}
          </AlertDescription>
        </Alert>
      ) : isLoading ? (
        <div className="flex flex-col gap-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-[72px] rounded-xl" />
          ))}
        </div>
      ) : rows.length === 0 ? (
        <EmptyState
          icon={Wand2}
          title={hasActiveFilters ? "No matching rules" : "No rules yet"}
          description={
            hasActiveFilters
              ? "Try adjusting or clearing your filters."
              : "Create a rule to automatically categorize, tag, or comment on transactions during sync."
          }
          action={
            <Button asChild>
              <Link to="/rules/new">
                <Plus className="size-4" /> Create rule
              </Link>
            </Button>
          }
        />
      ) : (
        <div className="flex flex-col gap-3">
          {rows.map((rule) => (
            <RuleRow
              key={rule.id}
              rule={rule}
              onToggle={onToggle}
              onDelete={setPendingDelete}
            />
          ))}
        </div>
      )}

      {total > RULES_PAGE_SIZE && (
        <PaginationBar
          page={page}
          pageSize={RULES_PAGE_SIZE}
          total={total}
          onPageChange={(next) =>
            navigate({
              to: ".",
              search: (prev: Record<string, unknown>) => ({
                ...prev,
                p: next > 1 ? next : undefined,
              }),
            })
          }
          isFetching={isFetching}
          itemLabel="rules"
        />
      )}

      <AlertDialog
        open={!!pendingDelete}
        onOpenChange={(open) => {
          if (!open) setPendingDelete(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              Delete rule "{pendingDelete?.name}"?
            </AlertDialogTitle>
            <AlertDialogDescription>
              The rule will stop firing on future syncs. Past actions it
              applied stay on transactions; this only removes the rule itself.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction variant="destructive" onClick={onDeleteConfirm}>
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}

function CountLine({ count, fetching }: { count: number; fetching: boolean }) {
  return (
    <div className="text-muted-foreground mb-3 flex items-center gap-2 text-sm">
      <span className="tabular-nums">
        {count.toLocaleString()} rule{count === 1 ? "" : "s"} found
      </span>
      {fetching && <Loader2 className="size-3 animate-spin" />}
    </div>
  );
}

function RulesToolbar({
  query,
  onQueryChange,
  enabled,
  onEnabledChange,
  sort,
  onSortChange,
}: {
  query: string;
  onQueryChange: (v: string) => void;
  enabled: "true" | "false" | undefined;
  onEnabledChange: (v: string) => void;
  sort: string;
  onSortChange: (v: string) => void;
}) {
  return (
    <div className="mb-4 flex flex-wrap items-center gap-2">
      <Input
        type="search"
        value={query}
        onChange={(e) => onQueryChange(e.target.value)}
        placeholder="Search rules…"
        className="h-9 max-w-xs"
      />
      <Select value={enabled ?? "all"} onValueChange={onEnabledChange}>
        <SelectTrigger className="h-9 w-[140px]">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">All states</SelectItem>
          <SelectItem value="true">Enabled</SelectItem>
          <SelectItem value="false">Disabled</SelectItem>
        </SelectContent>
      </Select>
      <Select value={sort} onValueChange={onSortChange}>
        <SelectTrigger className="ml-auto h-9 w-[160px]">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="priority">Pipeline stage</SelectItem>
          <SelectItem value="hit_count">Most hits</SelectItem>
          <SelectItem value="last_hit_at">Recently active</SelectItem>
          <SelectItem value="created_at">Newest first</SelectItem>
          <SelectItem value="name">Name (A–Z)</SelectItem>
        </SelectContent>
      </Select>
    </div>
  );
}
