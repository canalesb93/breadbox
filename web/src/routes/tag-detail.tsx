import { useMemo } from "react";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { Tags as TagsIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { PageHeader } from "@/components/page-header";
import { EmptyState } from "@/components/empty-state";
import { PageError } from "@/components/page-error";
import { DangerZone } from "@/components/danger-zone";
import { SectionCard } from "@/components/section-card";
import { SoftBackButton } from "@/components/soft-back-button";
import { TagChip } from "@/components/tag-chip";
import { TagForm } from "@/features/tags/tag-form";
import { useDeleteTag, useTags } from "@/api/queries/tags";
import { withMutationToast } from "@/lib/mutation-toast";
import type { Tag } from "@/api/types";

export function TagDetailPage() {
  const { slug } = useParams({ strict: false }) as { slug?: string };
  const tagsQuery = useTags();
  const { data: tags, isLoading, isError } = tagsQuery;
  const tag = useMemo(
    () => tags?.find((t) => t.slug === slug),
    [tags, slug],
  );

  return (
    <div className="mx-auto flex max-w-2xl flex-col gap-5">
      <SoftBackButton to="/tags">Back to tags</SoftBackButton>

      {isLoading ? (
        <DetailSkeleton />
      ) : isError ? (
        <PageError
          resource="this tag"
          error={tagsQuery.error}
          onRetry={() => tagsQuery.refetch()}
          retrying={tagsQuery.isFetching}
        />
      ) : !tag ? (
        <EmptyState
          variant="card"
          icon={TagsIcon}
          title="Tag not found"
          description="This tag may have been deleted, or the link is out of date. Head back to the tags list to pick another."
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

  // Tag identity: title carries the human name; eyebrow holds the canonical
  // "Tag" label so the page reads the same as the other detail pages
  // (Category, Account, Transaction, Connection) that anchor on an eyebrow.
  // The live preview chip in the page header makes the icon + colour
  // immediately visible above the form — same role the inline "Live
  // preview" tile used to play, but lifted up so the form body stays clean.
  return (
    <div className="space-y-6">
      <PageHeader
        eyebrow="Tag"
        title={tag.display_name}
        description={tag.description || "No description yet."}
        actions={<TagChip tag={tag} />}
      />

      <SectionCard
        icon={<TagsIcon className="text-muted-foreground size-4" />}
        title="Tag details"
      >
        <TagForm mode="edit" tag={tag} />
      </SectionCard>

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
    </div>
  );
}

function DetailSkeleton() {
  return (
    <div className="space-y-6">
      <div className="space-y-2">
        <Skeleton className="h-3 w-12" />
        <Skeleton className="h-7 w-48" />
        <Skeleton className="h-4 w-72" />
      </div>
      <Skeleton className="h-[28rem] rounded-xl" />
      <Skeleton className="h-32 rounded-xl" />
    </div>
  );
}
