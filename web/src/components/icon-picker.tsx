import { useMemo, useState } from "react";
import { X } from "lucide-react";
import { dynamicIconImports } from "lucide-react/dynamic";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandInput,
  CommandList,
} from "@/components/ui/command";
import { DynamicIcon } from "@/lib/icon";
import { cn } from "@/lib/utils";

// Shown when the search is empty so the picker isn't a wall of 1500+ icons;
// the full Lucide catalog is still reachable via search.
const POPULAR_ICONS = [
  "shopping-cart",
  "shopping-bag",
  "utensils",
  "coffee",
  "pizza",
  "beer",
  "wine",
  "car",
  "fuel",
  "train",
  "bus",
  "bike",
  "plane",
  "home",
  "lightbulb",
  "wifi",
  "phone",
  "credit-card",
  "banknote",
  "dollar-sign",
  "piggy-bank",
  "wallet",
  "briefcase",
  "graduation-cap",
  "book",
  "heart",
  "stethoscope",
  "pill",
  "dumbbell",
  "music",
  "film",
  "gamepad-2",
  "gift",
  "tag",
  "star",
  "flag",
  "calendar",
  "repeat",
  "sparkles",
  "zap",
] as const;

interface IconPickerProps {
  value: string | null | undefined;
  onChange: (value: string | null) => void;
  /** Optional tint applied to the rendered icon — usually the form's color. */
  tint?: string | null;
  placeholder?: string;
  className?: string;
}

export function IconPicker({
  value,
  onChange,
  tint,
  placeholder = "Pick an icon",
  className,
}: IconPickerProps) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return [] as string[];
    const out: string[] = [];
    for (const name in dynamicIconImports) {
      if (name.includes(q)) {
        out.push(name);
        if (out.length >= 60) break;
      }
    }
    return out;
  }, [query]);

  const items: readonly string[] = query ? filtered : POPULAR_ICONS;
  const showEmpty = query && filtered.length === 0;

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          className={cn("h-9 justify-start gap-2 px-2.5", className)}
        >
          <span className="bg-muted flex size-5 items-center justify-center rounded-md">
            {value ? (
              <DynamicIcon
                name={value}
                className="size-3.5"
                style={tint ? { color: tint } : undefined}
              />
            ) : null}
          </span>
          <span
            className={cn(
              "truncate text-sm",
              !value && "text-muted-foreground",
            )}
          >
            {value ?? placeholder}
          </span>
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-72 p-0" align="start">
        <Command shouldFilter={false}>
          <CommandInput
            placeholder="Search icons…"
            value={query}
            onValueChange={setQuery}
          />
          {/* Grid uses plain buttons so cmdk's filter/empty tracking is
              bypassed; filtering happens above in `filtered`. */}
          <CommandList className="max-h-64">
            <div className="space-y-1 p-1">
              <div className="text-muted-foreground px-2 pt-1 pb-0.5 text-xs">
                {query ? `Results (${filtered.length})` : "Popular"}
              </div>
              {showEmpty ? (
                <div className="text-muted-foreground px-2 py-6 text-center text-sm">
                  No matching icon.
                </div>
              ) : (
                <div className="grid grid-cols-8 gap-1">
                  {items.map((name) => (
                    <IconTile
                      key={name}
                      name={name}
                      selected={name === value}
                      tint={tint}
                      onPick={() => {
                        onChange(name);
                        setOpen(false);
                      }}
                    />
                  ))}
                </div>
              )}
            </div>
          </CommandList>
        </Command>
        {value && (
          <button
            type="button"
            onClick={() => {
              onChange(null);
              setOpen(false);
            }}
            className="text-muted-foreground hover:bg-accent focus-visible:ring-ring/50 focus-visible:ring-[3px] focus-visible:outline-none flex w-full items-center justify-center gap-1.5 border-t px-2 py-2 text-xs"
          >
            <X className="size-3.5" /> Clear icon
          </button>
        )}
      </PopoverContent>
    </Popover>
  );
}

function IconTile({
  name,
  selected,
  tint,
  onPick,
}: {
  name: string;
  selected: boolean;
  tint?: string | null;
  onPick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onPick}
      title={name}
      className={cn(
        "hover:bg-accent flex size-8 items-center justify-center rounded-md transition",
        // Shared focus-visible recipe (matches Button primitive) so keyboard
        // users can see which tile is focused as they tab/arrow through.
        "focus-visible:ring-ring/50 focus-visible:ring-[3px] focus-visible:outline-none",
        selected && "bg-accent ring-ring ring-2",
      )}
    >
      <DynamicIcon
        name={name}
        className="size-4"
        style={tint ? { color: tint } : undefined}
      />
    </button>
  );
}

