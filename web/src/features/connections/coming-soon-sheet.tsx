import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";

interface ComingSoonSheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: string;
}

// PR-01 stub. The Connect-bank Sheet (PR-03) and Re-auth Sheet (PR-05) replace
// this with the real flows. Lives in this PR so the buttons that open them
// have something to point at and the wiring (URL params, ⋯-menu callback)
// lands now without a half-built flow.
export function ComingSoonSheet({
  open,
  onOpenChange,
  title,
  description,
}: ComingSoonSheetProps) {
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="gap-6 p-6">
        <SheetHeader className="p-0">
          <SheetTitle>{title}</SheetTitle>
          <SheetDescription>{description}</SheetDescription>
        </SheetHeader>
        <div className="text-muted-foreground bg-muted/40 rounded-md border border-dashed p-6 text-sm">
          The full flow lands in a follow-up PR in this stack. For now use the
          v1 admin at <span className="font-mono">/connections</span>.
        </div>
      </SheetContent>
    </Sheet>
  );
}
