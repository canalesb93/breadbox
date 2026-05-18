import { useMemo } from "react";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { Key, Plus } from "lucide-react";
import { z } from "zod";
import { PageHeader } from "@/components/page-header";
import { EmptyState } from "@/components/empty-state";
import { SearchInput } from "@/components/search-input";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useAPIKeys } from "@/api/queries/api-keys";
import { APIKeysTable } from "@/features/api-keys/api-keys-table";
import type { APIKey } from "@/api/types";

// URL state lives in `?status=active|revoked&q=…` so a refresh keeps the
// filter context — same convention as the connections/transactions pages.
export const apiKeysSearchSchema = z.object({
  status: z.enum(["active", "revoked"]).optional(),
  q: z.string().optional(),
});

type APIKeysSearch = z.infer<typeof apiKeysSearchSchema>;
type Tab = "active" | "revoked";

function filterKeys(keys: APIKey[], query: string): APIKey[] {
  const q = query.trim().toLowerCase();
  if (!q) return keys;
  return keys.filter(
    (k) =>
      k.name.toLowerCase().includes(q) ||
      k.key_prefix.toLowerCase().includes(q) ||
      (k.actor_name?.toLowerCase().includes(q) ?? false),
  );
}

export function APIKeysPage() {
  const { data: keys, isLoading, isError } = useAPIKeys();
  const search = useSearch({ strict: false }) as APIKeysSearch;
  const navigate = useNavigate();
  const tab: Tab = search.status ?? "active";
  const query = search.q ?? "";

  const { active, revoked } = useMemo(() => {
    const all = keys ?? [];
    return {
      active: all.filter((k) => !k.revoked_at),
      revoked: all.filter((k) => k.revoked_at),
    };
  }, [keys]);

  const tabRows = tab === "active" ? active : revoked;
  const rows = useMemo(
    () => filterKeys(tabRows, query),
    [tabRows, query],
  );

  // Eyebrow vocabulary matches Tags / Transactions / Connections list pages:
  // "Loading" / "Error" / "N keys" / "Showing N of M" / "No matches".
  const tabLabel = tab === "active" ? "active" : "revoked";
  const eyebrow = (() => {
    if (isLoading) return "Loading";
    if (isError) return "Error";
    if (tabRows.length === 0) {
      return tab === "active" ? "No active keys" : "No revoked keys";
    }
    if (query.trim()) {
      if (rows.length === 0) return "No matches";
      if (rows.length < tabRows.length) {
        return `Showing ${rows.length.toLocaleString()} of ${tabRows.length.toLocaleString()}`;
      }
    }
    return `${tabRows.length.toLocaleString()} ${tabLabel} ${tabRows.length === 1 ? "key" : "keys"}`;
  })();

  function setTab(next: Tab) {
    navigate({
      to: "/api-keys",
      search: (prev: APIKeysSearch) => ({
        ...prev,
        status: next === "active" ? undefined : next,
      }),
      replace: true,
    });
  }

  function setQuery(next: string) {
    navigate({
      to: "/api-keys",
      search: (prev: APIKeysSearch) => ({
        ...prev,
        q: next || undefined,
      }),
      replace: true,
    });
  }

  const newKeyButton = (
    <Button asChild size="sm">
      <Link to="/api-keys/new">
        <Plus className="size-4" />
        New key
      </Link>
    </Button>
  );

  const emptyState = renderEmptyState({ tab, query, newKeyButton });

  return (
    <>
      <PageHeader
        eyebrow={eyebrow}
        title="API keys"
        description="Credentials for programmatic access — agents, the CLI, the MCP server. Each key carries a fixed scope and an attributed actor, and is hashed in storage."
        actions={newKeyButton}
      />

      <div className="flex flex-col gap-3">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <Tabs value={tab} onValueChange={(v) => setTab(v as Tab)}>
            <TabsList>
              <TabsTrigger value="active">
                Active
                <span className="text-muted-foreground ml-1.5 text-xs tabular-nums">
                  {active.length}
                </span>
              </TabsTrigger>
              <TabsTrigger value="revoked">
                Revoked
                <span className="text-muted-foreground ml-1.5 text-xs tabular-nums">
                  {revoked.length}
                </span>
              </TabsTrigger>
            </TabsList>
          </Tabs>

          <SearchInput
            containerClassName="w-full max-w-xs"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search by name, prefix, actor…"
          />
        </div>

        <APIKeysTable
          keys={rows}
          isLoading={isLoading}
          isError={isError}
          revoked={tab === "revoked"}
          emptyState={emptyState}
        />
      </div>
    </>
  );
}

interface EmptyStateArgs {
  tab: Tab;
  query: string;
  newKeyButton: React.ReactNode;
}

function renderEmptyState({ tab, query, newKeyButton }: EmptyStateArgs) {
  if (tab === "revoked") {
    return (
      <EmptyState
        icon={Key}
        title="No revoked keys"
        description="Once you revoke a key it lands here so you can keep an audit trail of what was issued and rolled."
      />
    );
  }
  if (query) {
    return (
      <EmptyState
        icon={Key}
        title="No matching keys"
        description="Try a different search term, or clear the filter to see every key."
      />
    );
  }
  return (
    <EmptyState
      icon={Key}
      title="No API keys yet"
      description="Mint a key to let agents, scripts, or the CLI talk to Breadbox over REST or MCP."
      action={newKeyButton}
    />
  );
}
