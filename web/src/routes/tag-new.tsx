import { Tags as TagsIcon } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { SectionCard } from "@/components/section-card";
import { SoftBackButton } from "@/components/soft-back-button";
import { TagForm } from "@/features/tags/tag-form";

export function TagNewPage() {
  return (
    <div className="mx-auto max-w-2xl">
      <SoftBackButton to="/tags">Back to tags</SoftBackButton>
      <PageHeader
        eyebrow="New tag"
        title="Create a tag"
        description="Tags are reusable labels you attach to any transaction. Pick a stable slug — rules, exports, and the URL reference it."
      />
      <SectionCard
        icon={<TagsIcon className="text-muted-foreground size-4" />}
        title="Tag details"
      >
        <TagForm mode="create" />
      </SectionCard>
    </div>
  );
}
