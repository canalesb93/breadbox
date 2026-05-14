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
  className?: string;
}

// TagChip is the single rendering of one tag — icon + display name tinted by
// the tag's color, with an optional remove button for editable contexts.
export function TagChip({ tag, onRemove, className }: TagChipProps) {
  const tint = tag.color ? { color: tag.color } : undefined;
  return (
    <Badge variant="outline" className={cn("gap-1", className)}>
      {tag.icon && (
        <DynamicIcon name={tag.icon} className="size-3" style={tint} />
      )}
      <span style={tint}>{tag.display_name}</span>
      {onRemove && (
        <button
          type="button"
          onClick={onRemove}
          aria-label={`Remove ${tag.display_name}`}
          className="text-muted-foreground hover:text-foreground -mr-0.5 ml-0.5 rounded-full"
        >
          <X className="size-3" />
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
  className?: string;
}

// TagList resolves a transaction's tag slugs against the cached tag catalog
// and renders chips. A slug with no matching tag still renders (display name
// falls back to the slug) so a freshly-created tag never silently vanishes.
export function TagList({ slugs, max, className }: TagListProps) {
  const { data: tags } = useTags();
  if (!slugs?.length) return null;

  const bySlug = new Map((tags ?? []).map((t) => [t.slug, t]));
  const resolved: TagLike[] = slugs.map(
    (slug) =>
      bySlug.get(slug) ?? { slug, display_name: slug, color: null, icon: null },
  );
  const shown = max ? resolved.slice(0, max) : resolved;
  const overflow = resolved.length - shown.length;

  return (
    <div className={cn("flex flex-wrap items-center gap-1", className)}>
      {shown.map((tag) => (
        <TagChip key={tag.slug} tag={tag} />
      ))}
      {overflow > 0 && (
        <Badge variant="outline" className="text-muted-foreground">
          +{overflow}
        </Badge>
      )}
    </div>
  );
}
