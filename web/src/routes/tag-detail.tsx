import { useMemo } from "react";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { ArrowLeft, Tags as TagsIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { PageHeader } from "@/components/page-header";
import { EmptyState } from "@/components/empty-state";
import { DangerZone } from "@/components/danger-zone";
import { TagForm } from "@/features/tags/tag-form";
import { useDeleteTag, useTags } from "@/api/queries/tags";
import { withMutationToast } from "@/lib/mutation-toast";
import type { Tag } from "@/api/types";

export function TagDetailPage() {
  const { slug } = useParams({ strict: false }) as { slug?: string };
  const { data: tags, isLoading, isError } = useTags();
  const tag = useMemo(
    () => tags?.find((t) => t.slug === slug),
    [tags, slug],
  );

  return (
    <div className="mx-auto max-w-2xl">
      <Button variant="ghost" size="sm" asChild className="mb-4 -ml-2">
        <Link to="/tags">
          <ArrowLeft className="size-4" />
          Tags
        </Link>
      </Button>

      {isLoading ? (
        <Skeleton className="h-96 rounded-xl" />
      ) : isError || !tag ? (
        <EmptyState
          icon={TagsIcon}
          title="Tag not found"
          description="It may have been deleted, or the link is wrong."
          action={
            <Button variant="outline" asChild>
              <Link to="/tags">Back to tags</Link>
            </Button>
          }
        />
      ) : (
        <DetailBody tag={tag} />
      )}
    </div>
  );
}

function DetailBody({ tag }: { tag: Tag }) {
  const navigate = useNavigate();
  const del = useDeleteTag();
  return (
    <>
      <PageHeader
        title={tag.display_name}
        description={tag.description || "No description yet."}
      />
      <TagForm mode="edit" tag={tag} />
      <DangerZone
        description="The tag will be removed from every transaction it's attached to. Activity history is preserved. This can't be undone."
        confirmTarget={<span className="font-semibold">{tag.display_name}</span>}
        actionLabel="Delete tag"
        isPending={del.isPending}
        onConfirm={async () => {
          const ok = await withMutationToast(
            () => del.mutateAsync(tag.slug),
            { success: "Tag deleted." },
          );
          if (ok) navigate({ to: "/tags" });
        }}
      />
    </>
  );
}
