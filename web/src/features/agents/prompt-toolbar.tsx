import { useMemo, useState } from "react";
import {
  ArrowUpRight,
  ChevronDown,
  Loader2,
  Plus,
  Search,
  Sparkles,
} from "lucide-react";
import { Link } from "@tanstack/react-router";
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
  CommandSeparator,
} from "@/components/ui/command";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  usePromptBlocks,
  type PromptBlock,
  type PromptBlockGroup,
} from "@/api/queries/prompt-blocks";
import { DynamicIcon } from "@/lib/icon";

interface PromptToolbarProps {
  promptValue: string;
  onInsert: (text: string) => void;
}

const GROUP_LABEL: Record<PromptBlockGroup, string> = {
  strategy: "Strategy",
  depth: "Review depth",
  integration: "Integration",
  knowledge: "Knowledge",
};

const GROUP_ORDER: PromptBlockGroup[] = [
  "strategy",
  "depth",
  "integration",
  "knowledge",
];

// PromptToolbar is the lightweight in-form picker for dropping a single
// prompt block into the prompt textarea. Pairs with the "Open prompt
// builder" link in the section header — this affordance is for "I just
// need one more block", the builder is for "I want to compose from
// scratch with reordering / editing / preview".
//
// Visual: a thin row sitting above the textarea with a left-side
// "insert from library" trigger (popover with grouped, searchable
// command list) and a right-side eyebrow link to the full builder.
export function PromptToolbar({ promptValue, onInsert }: PromptToolbarProps) {
  const blocksQuery = usePromptBlocks();
  const blocks = blocksQuery.data ?? [];

  const grouped = useMemo(() => {
    const m: Record<PromptBlockGroup, PromptBlock[]> = {
      strategy: [],
      depth: [],
      integration: [],
      knowledge: [],
    };
    for (const b of blocks) m[b.group].push(b);
    return m;
  }, [blocks]);

  const [open, setOpen] = useState(false);

  const handlePick = (block: PromptBlock) => {
    onInsert(block.content.trim());
    setOpen(false);
  };

  const hasPrompt = promptValue.trim().length > 0;

  return (
    <div className="mb-1 flex items-center justify-between gap-2">
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="h-7 gap-1.5 px-2.5 text-xs font-medium"
          >
            <Plus className="size-3.5" />
            Insert block
            {blocksQuery.isLoading && (
              <Loader2 className="size-3 animate-spin opacity-60" />
            )}
            <ChevronDown className="size-3 opacity-60" />
          </Button>
        </PopoverTrigger>
        <PopoverContent
          className="w-[360px] p-0"
          align="start"
          sideOffset={6}
        >
          <Command>
            <div className="flex items-center border-b px-3">
              <Search className="text-muted-foreground size-3.5 shrink-0" />
              <CommandInput
                placeholder="Search prompt blocks…"
                className="h-9 border-0 px-2 focus-visible:ring-0"
              />
            </div>
            <CommandList className="max-h-[320px]">
              <CommandEmpty>
                <div className="text-muted-foreground py-6 text-center text-sm">
                  No blocks match.
                </div>
              </CommandEmpty>
              {GROUP_ORDER.map((group) => {
                const items = grouped[group];
                if (!items || items.length === 0) return null;
                return (
                  <CommandGroup
                    key={group}
                    heading={GROUP_LABEL[group]}
                  >
                    {items.map((b) => (
                      <CommandItem
                        key={b.id}
                        value={`${b.title} ${b.description} ${b.id}`}
                        onSelect={() => handlePick(b)}
                        className="cursor-pointer gap-2.5"
                      >
                        {b.icon && (
                          <DynamicIcon
                            name={b.icon}
                            className="text-muted-foreground mt-0.5 size-4 shrink-0"
                          />
                        )}
                        <div className="min-w-0 flex-1">
                          <div className="truncate text-sm font-medium">
                            {b.title}
                          </div>
                          <div className="text-muted-foreground truncate text-xs">
                            {b.description}
                          </div>
                        </div>
                      </CommandItem>
                    ))}
                  </CommandGroup>
                );
              })}
              <CommandSeparator />
              <CommandGroup>
                <CommandItem
                  asChild
                  value="open-prompt-builder"
                  className="cursor-pointer"
                >
                  <Link
                    to="/prompts/build"
                    className="flex items-center gap-2"
                  >
                    <Sparkles className="text-primary size-4" />
                    <span className="text-sm">
                      Compose from scratch in the builder
                    </span>
                    <ArrowUpRight className="text-muted-foreground ml-auto size-3.5" />
                  </Link>
                </CommandItem>
              </CommandGroup>
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>

      <div className="flex items-center gap-2">
        {hasPrompt && (
          <Badge variant="outline" className="text-[10px] font-normal">
            <Sparkles className="size-2.5" />
            Tip: insert appends below
          </Badge>
        )}
      </div>
    </div>
  );
}
