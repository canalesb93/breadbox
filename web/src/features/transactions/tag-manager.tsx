import { useState } from "react";
import { Check, Plus } from "lucide-react";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import { Button } from "@/components/ui/button";
import { TagChip } from "@/components/tag-chip";
import { DynamicIcon } from "@/lib/icon";
import { withMutationToast } from "@/lib/mutation-toast";
import { useTags } from "@/api/queries/tags";
import { useUpdateTransactions } from "@/api/queries/transactions";

interface TagManagerProps {
  transactionId: string;
  /** Tag slugs currently attached to the transaction. */
  tags: string[];
}

// TagManager is the single-transaction tag editor on the detail page. Current
// tags render as removable chips; the "Add tag" popover toggles attachment
// against the full tag catalog. Each toggle is one batch operation — add or
// remove a single slug — and the cache invalidation refetches the row.
export function TagManager({ transactionId, tags }: TagManagerProps) {
  const [open, setOpen] = useState(false);
  const { data: catalog, isLoading } = useTags();
  const update = useUpdateTransactions();

  const attached = new Set(tags);
  const bySlug = new Map((catalog ?? []).map((t) => [t.slug, t]));

  const toggle = async (slug: string) => {
    const isAttached = attached.has(slug);
    await withMutationToast(
      () =>
        update.mutateAsync({
          operations: [
            {
              transaction_id: transactionId,
              ...(isAttached
                ? { tags_to_remove: [{ slug }] }
                : { tags_to_add: [{ slug }] }),
            },
          ],
        }),
      { success: isAttached ? "Tag removed." : "Tag added." },
    );
  };

  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {tags.map((slug) => {
        const tag = bySlug.get(slug) ?? {
          slug,
          display_name: slug,
          color: null,
          icon: null,
        };
        return (
          <TagChip
            key={slug}
            tag={tag}
            onRemove={update.isPending ? undefined : () => toggle(slug)}
          />
        );
      })}

      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            variant="outline"
            size="sm"
            className="h-6 gap-1 rounded-full px-2 text-xs"
            disabled={update.isPending}
          >
            <Plus className="size-3" />
            Add tag
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-56 p-0" align="start">
          <Command>
            <CommandInput placeholder="Search tags…" />
            <CommandList>
              <CommandEmpty>
                {isLoading ? "Loading…" : "No tags found."}
              </CommandEmpty>
              <CommandGroup>
                {(catalog ?? []).map((tag) => (
                  <CommandItem
                    key={tag.slug}
                    value={tag.display_name}
                    onSelect={() => toggle(tag.slug)}
                  >
                    <DynamicIcon
                      name={tag.icon}
                      className="size-4"
                      style={tag.color ? { color: tag.color } : undefined}
                    />
                    <span>{tag.display_name}</span>
                    {attached.has(tag.slug) && (
                      <Check className="ml-auto size-4" />
                    )}
                  </CommandItem>
                ))}
              </CommandGroup>
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
    </div>
  );
}
