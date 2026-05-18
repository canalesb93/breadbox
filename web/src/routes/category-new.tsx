import { Shapes } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { SectionCard } from "@/components/section-card";
import { SoftBackButton } from "@/components/soft-back-button";
import { CategoryForm } from "@/features/categories/category-form";

export function CategoryNewPage() {
  return (
    <div className="mx-auto flex max-w-2xl flex-col gap-5">
      <SoftBackButton to="/categories">Back to categories</SoftBackButton>
      <PageHeader
        eyebrow="New category"
        title="Create a category"
        description="Top-level categories form the spine of your spending breakdown. Sub-categories let you slice further. Pick a parent to nest, or leave empty for a fresh group."
      />
      <SectionCard
        icon={<Shapes className="text-muted-foreground size-4" />}
        title="Category details"
      >
        <CategoryForm mode="create" />
      </SectionCard>
    </div>
  );
}
