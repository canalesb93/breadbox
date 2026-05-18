import { Eyebrow } from "@/components/eyebrow";
import { cn } from "@/lib/utils";

export interface PageHeaderProps {
  /** Optional small label rendered above the title. Sentence case in the
      source ("New tag", "Edit rule", "Settings", "Key created") — the
      uppercase comes from CSS. On detail pages it doubles as the entity
      type ("Tag", "Category"); on form pages it frames the verb ("New
      tag" / "Edit tag"); on dynamic pages it can carry live status
      ("3 healthy · 3 of 3 configured"). Use it whenever the title alone
      is ambiguous about which family the page belongs to. */
  eyebrow?: string;
  /** The page H1. Sentence case ("Connections", "Mint an API key",
      "Welcome back, Ricardo"). No trailing punctuation. */
  title: string;
  /** One short paragraph that frames the page for someone landing cold.
      Full sentence(s) ending with a period — same voice across pages so
      the system reads coherently. Prefer a noun-led framing ("Bank data
      providers that sync accounts and transactions into Breadbox.")
      over an imperative fragment ("Configure bank providers."). When the
      same page renders multiple states (loading, error, loaded), hoist
      the copy to a module-level constant so the description doesn't
      momentarily shift on transition. */
  description?: string;
  /** Optional right-aligned actions — usually a <Button> or a cluster. */
  actions?: React.ReactNode;
  className?: string;
}

// Content-level page header, rendered inside <main>, below the shell's
// breadcrumb bar. Every v2 page uses this so the title/description/action
// layout stays consistent.
export function PageHeader({
  eyebrow,
  title,
  description,
  actions,
  className,
}: PageHeaderProps) {
  return (
    <div
      className={cn(
        // No outer margin — <main> is a flex column with gap-5 (see the
        // contract comment in routes/__root.tsx) so the gap below PageHeader
        // comes from the parent layout. Adding `mb-*` here stacks with that
        // gap and produces a ~44px void on mobile (visible because the
        // action chip wraps below the description).
        "flex flex-col items-start justify-between gap-3 sm:flex-row sm:items-start sm:gap-4",
        className,
      )}
    >
      <div className="min-w-0 space-y-1.5">
        {eyebrow && (
          <Eyebrow as="p" variant="page">
            {eyebrow}
          </Eyebrow>
        )}
        <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
        {description && (
          <p className="text-muted-foreground max-w-prose text-sm">
            {description}
          </p>
        )}
      </div>
      {actions && (
        <div className="flex shrink-0 items-center gap-2">{actions}</div>
      )}
    </div>
  );
}
