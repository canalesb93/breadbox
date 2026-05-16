import { Link } from "@tanstack/react-router";
import { ArrowLeft } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/page-header";
import { TagForm } from "@/features/tags/tag-form";

export function TagNewPage() {
  return (
    <div className="mx-auto max-w-2xl">
      <Button variant="ghost" size="sm" asChild className="mb-4 -ml-2">
        <Link to="/tags">
          <ArrowLeft className="size-4" />
          Tags
        </Link>
      </Button>
      <PageHeader
        title="New tag"
        description="Tags are reusable labels. Pick a stable slug — rules and exports reference it."
      />
      <TagForm mode="create" />
    </div>
  );
}
