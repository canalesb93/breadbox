import { useMemo } from "react";
import { X } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { DynamicIcon } from "@/lib/icon";
import { cn } from "@/lib/utils";
import { useTags } from "@/api/queries/tags";
import type { Tag } from "@/api/types";

type TagLike = Pick<Tag, "slug" | "display_name" | "color" | "icon">;

interface TagChipProps {
  tag: TagLike;
  /** When set, renders a remove (×) button that calls this on click. */
  onRemove?: () => void;
  /**
   * Visual density.
   * - `xs` — inline inside a meta row (16px text line); does not grow the row
   * - `sm` — table rows / dense list cells (parity with `CategoryBadge size="sm"`)
   * - `md` — default; detail-page hero actions, sandbox, rule-display
   */
  size?: "xs" | "sm" | "md";
  className?: string;
}

// Per-size token recipe — mirrors CategoryBadge's recipe so the two share a
// rhythm when rendered side-by-side in a transaction row. Adjust as a pair.
const SIZE: Record<"xs" | "sm" | "md", string> = {
  xs: "h-4 px-1.5 text-[10px] leading-none gap-0.5 [&>svg]:size-2.5",
  sm: "h-5 px-1.5 text-[11px] gap-0.5 [&>svg]:size-2.5",
  md: "h-6 px-2 text-xs gap-1 [&>svg]:size-3",
};

// TagChip is the single rendering of one tag — icon + display name tinted by
// the tag's color, with an optional remove button for editable contexts.
export function TagChip({
  tag,
  onRemove,
  size = "md",
  className,
}: TagChipProps) {
  const tint = tag.color ? { color: tag.color } : undefined;
  return (
    <Badge variant="outline" className={cn(SIZE[size], className)}>
      {tag.icon && <DynamicIcon name={tag.icon} style={tint} />}
      <span style={tint}>{tag.display_name}</span>
      {onRemove && (
        // The visible × stays small (10–12px) to fit inside the chip; on
        // touch devices a centered 44pt invisible pseudo-element extends the
        // hit area so the tap target meets Apple HIG. Same TAP_TARGET recipe
        // as `Button`'s icon sizes — see `web/src/components/ui/button.tsx`
        // and the iter-28 followup.
        <button
          type="button"
          onClick={onRemove}
          aria-label={`Remove ${tag.display_name}`}
          className="text-muted-foreground hover:text-foreground focus-visible:ring-ring/50 -mr-0.5 ml-0.5 relative rounded-full focus-visible:ring-[3px] focus-visible:outline-none pointer-coarse:before:absolute pointer-coarse:before:left-1/2 pointer-coarse:before:top-1/2 pointer-coarse:before:size-11 pointer-coarse:before:-translate-x-1/2 pointer-coarse:before:-translate-y-1/2 pointer-coarse:before:content-['']"
        >
          <X className={size === "sm" ? "size-2.5" : "size-3"} />
        </button>
      )}
    </Badge>
  );
}

interface TagListProps {
  /** Tag slugs as carried on a transaction row. */
  slugs: string[] | undefined;
  /** Cap the chips shown; the remainder collapse into a "+N" badge. */
  max?: number;
  /** Forwarded to every `TagChip` and the overflow badge for density parity. */
  size?: "xs" | "sm" | "md";
  className?: string;
}

// TagList resolves a transaction's tag slugs against the cached tag catalog
// and renders chips. A slug with no matching tag still renders (display name
// falls back to the slug) so a freshly-created tag never silently vanishes.
export function TagList({ slugs, max, size = "md", className }: TagListProps) {
  const { data: tags } = useTags();
  // Memoized above the early return (Rules of Hooks) — TagList renders in
  // every table row, so rebuilding this Map per row per render is wasteful.
  const bySlug = useMemo(
    () => new Map((tags ?? []).map((t) => [t.slug, t])),
    [tags],
  );
  if (!slugs?.length) return null;

  const resolved: TagLike[] = slugs.map(
    (slug) =>
      bySlug.get(slug) ?? { slug, display_name: slug, color: null, icon: null },
  );
  const shown = max ? resolved.slice(0, max) : resolved;
  const overflow = resolved.length - shown.length;

  return (
    <div className={cn("flex flex-wrap items-center gap-1", className)}>
      {shown.map((tag) => (
        <TagChip key={tag.slug} tag={tag} size={size} />
      ))}
      {overflow > 0 && (
        <Badge
          variant="outline"
          className={cn("text-muted-foreground", SIZE[size])}
        >
          +{overflow}
        </Badge>
      )}
    </div>
  );
}
