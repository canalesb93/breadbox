import { Link } from "@tanstack/react-router";
import { ArrowLeft } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/page-header";
import { CategoryForm } from "@/features/categories/category-form";

export function CategoryNewPage() {
  return (
    <div className="mx-auto max-w-2xl">
      <Button variant="ghost" size="sm" asChild className="mb-4 -ml-2">
        <Link to="/categories">
          <ArrowLeft className="size-4" />
          Categories
        </Link>
      </Button>
      <PageHeader
        title="New category"
        description="Top-level categories form the spine of your spending breakdown. Sub-categories let you slice further."
      />
      <CategoryForm mode="create" />
    </div>
  );
}
