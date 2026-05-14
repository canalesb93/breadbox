import { Check } from "lucide-react";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import { DynamicIcon } from "@/lib/icon";
import { useTags } from "@/api/queries/tags";

interface TagCommandListProps {
  /** Slugs already attached — rendered with a check mark. */
  attachedSlugs?: Set<string>;
  onPick: (slug: string) => void;
}

// TagCommandList is the shared searchable tag list behind every tag-mutation
// surface — the detail-page tag manager and bulk tag. Pure presentation: the
// caller owns the mutation.
export function TagCommandList({ attachedSlugs, onPick }: TagCommandListProps) {
  const { data: tags, isLoading } = useTags();

  return (
    <Command>
      <CommandInput placeholder="Search tags…" />
      <CommandList>
        <CommandEmpty>
          {isLoading ? "Loading…" : "No tags found."}
        </CommandEmpty>
        <CommandGroup>
          {(tags ?? []).map((tag) => (
            <CommandItem
              key={tag.slug}
              value={tag.display_name}
              onSelect={() => onPick(tag.slug)}
            >
              <DynamicIcon
                name={tag.icon}
                className="size-4"
                style={tag.color ? { color: tag.color } : undefined}
              />
              <span>{tag.display_name}</span>
              {attachedSlugs?.has(tag.slug) && (
                <Check className="ml-auto size-4" />
              )}
            </CommandItem>
          ))}
        </CommandGroup>
      </CommandList>
    </Command>
  );
}
