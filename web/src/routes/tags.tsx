import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { Plus, Search, Tags as TagsIcon } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { EmptyState } from "@/components/empty-state";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { TagsTable } from "@/features/tags/tags-table";
import { useTags } from "@/api/queries/tags";

export function TagsPage() {
  const [query, setQuery] = useState("");
  const { data: tags, isLoading, isError } = useTags();

  return (
    <div>
      <PageHeader
        title="Tags"
        description="Free-form labels you can attach to any transaction."
        actions={
          <Button asChild>
            <Link to="/tags/new">
              <Plus className="size-4" />
              New tag
            </Link>
          </Button>
        }
      />

      <div className="mb-4">
        <div className="relative max-w-sm">
          <Search className="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search tags…"
            className="pl-8"
          />
        </div>
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
              description="Try a different search term."
            />
          ) : (
            <EmptyState
              icon={TagsIcon}
              title="No tags yet"
              description="Create your first tag to start labelling transactions."
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
  );
}
