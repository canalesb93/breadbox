import { Construction } from "lucide-react";

export function Placeholder({ title }: { title: string }) {
  return (
    <div className="flex flex-col items-center justify-center rounded-lg border border-dashed p-12 text-center">
      <div className="bg-muted text-muted-foreground mb-4 flex size-12 items-center justify-center rounded-full">
        <Construction className="size-5" />
      </div>
      <h2 className="text-lg font-semibold">{title}</h2>
      <p className="text-muted-foreground mt-1 max-w-sm text-sm">
        This page is part of the v2 admin shell. The implementation lands in a
        future PR — see <code>v2-frontend-plan.md</code> for the build order.
      </p>
    </div>
  );
}
