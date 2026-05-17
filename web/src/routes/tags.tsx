import { useMemo, useState } from "react";
import { Link } from "@tanstack/react-router";
import { Plus, Tags as TagsIcon } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { EmptyState } from "@/components/empty-state";
import { SearchInput } from "@/components/search-input";
import { Button } from "@/components/ui/button";
import { TagsTable } from "@/features/tags/tags-table";
import { useTags } from "@/api/queries/tags";

export function TagsPage() {
  const [query, setQuery] = useState("");
  const { data: tags, isLoading, isError } = useTags();

  // Match the Transactions list "Showing N of M" / "N tags" vocabulary so the
  // page lands with intent instead of a naked H1. Filtering shows the
  // narrowed slice; an empty page just calls itself "Tags".
  const total = tags?.length ?? 0;
  const filteredCount = useMemo(() => {
    if (!tags) return 0;
    const q = query.trim().toLowerCase();
    if (!q) return tags.length;
    return tags.filter(
      (t) =>
        t.slug.toLowerCase().includes(q) ||
        t.display_name.toLowerCase().includes(q) ||
        t.description.toLowerCase().includes(q),
    ).length;
  }, [tags, query]);

  const eyebrow = (() => {
    if (isLoading) return "Loading";
    if (isError) return "Error";
    if (total === 0) return "No tags";
    if (query.trim()) {
      if (filteredCount === 0) return "No matches";
      if (filteredCount < total) {
        return `Showing ${filteredCount.toLocaleString()} of ${total.toLocaleString()}`;
      }
    }
    return `${total.toLocaleString()} ${total === 1 ? "tag" : "tags"}`;
  })();

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        eyebrow={eyebrow}
        title="Tags"
        description="Free-form labels you can attach to any transaction. Use tags for cross-cutting context that doesn't fit a single category — like recurring, business, or trip names."
        actions={
          <Button asChild size="sm">
            <Link to="/tags/new">
              <Plus className="size-4" />
              New tag
            </Link>
          </Button>
        }
      />

      <div className="flex flex-col gap-3">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <SearchInput
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search by name, slug, or description…"
          />
        </div>

        <TagsTable
          tags={tags ?? []}
          isLoading={isLoading}
          isError={isError}
          query={query}
          emptyState={
            query ? (
              <EmptyState
                icon={TagsIcon}
                title="No matching tags"
                description="Try a different search term, or clear the filter to see every tag."
              />
            ) : (
              <EmptyState
                icon={TagsIcon}
                title="No tags yet"
                description="Create your first tag to label transactions across categories — useful for travel, reimbursements, or any ad-hoc grouping."
                action={
                  <Button asChild>
                    <Link to="/tags/new">
                      <Plus className="size-4" />
                      New tag
                    </Link>
                  </Button>
                }
              />
            )
          }
        />
      </div>
    </div>
  );
}
