import { useState } from "react";
import { Check, X } from "lucide-react";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

// Order is deliberate: warm hues first, then cool, then neutral — easier to
// scan than alphabetical hex.
export const PRESET_COLORS = [
  "#ef4444", "#f97316", "#f59e0b", "#eab308", "#84cc16", "#22c55e",
  "#10b981", "#14b8a6", "#06b6d4", "#0ea5e9", "#3b82f6", "#6366f1",
  "#8b5cf6", "#a855f7", "#d946ef", "#ec4899", "#f43f5e", "#64748b",
] as const;

const HEX_RE = /^#[0-9a-fA-F]{6}$/;

interface ColorPickerProps {
  value: string | null | undefined;
  onChange: (value: string | null) => void;
  /** Trigger label shown when no color is selected. */
  placeholder?: string;
  className?: string;
}

export function ColorPicker({
  value,
  onChange,
  placeholder = "Pick a color",
  className,
}: ColorPickerProps) {
  const [open, setOpen] = useState(false);
  // Local draft only matters while the popover is open — the controlled value
  // is the source of truth otherwise, so no sync effect is needed.
  const [draft, setDraft] = useState(value ?? "");

  const commitDraft = () => {
    if (!HEX_RE.test(draft)) return;
    onChange(draft);
    setOpen(false);
  };

  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        if (next) setDraft(value ?? "");
        setOpen(next);
      }}
    >
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          className={cn("h-9 justify-start gap-2 px-2.5", className)}
        >
          <span
            className={cn("size-5 rounded-md border", !value && "bg-transparent")}
            style={value ? { backgroundColor: value } : undefined}
          />
          <span className={cn("truncate text-sm", !value && "text-muted-foreground")}>
            {value ?? placeholder}
          </span>
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-64 space-y-3 p-3" align="start">
        <div className="grid grid-cols-6 gap-1.5">
          {PRESET_COLORS.map((color) => {
            const selected = color === value;
            return (
              <button
                key={color}
                type="button"
                onClick={() => {
                  onChange(color);
                  setOpen(false);
                }}
                aria-label={`Pick ${color}`}
                className={cn(
                  "relative size-7 rounded-md border transition hover:scale-110 hover:shadow-sm",
                  // Shared focus vocabulary: matches the shadcn Button ring
                  // recipe (ring-ring/50 + ring-[3px]) so keyboard users can
                  // see which swatch is focused. Offset keeps the ring from
                  // hugging the swatch fill.
                  "focus-visible:ring-ring/50 focus-visible:ring-[3px] focus-visible:ring-offset-2 focus-visible:outline-none",
                  selected && "ring-ring ring-2 ring-offset-2",
                )}
                style={{ backgroundColor: color }}
              >
                {selected && (
                  <Check className="absolute inset-0 m-auto size-4 text-white drop-shadow" />
                )}
              </button>
            );
          })}
        </div>
        <div className="space-y-1.5">
          <label className="text-muted-foreground text-xs">Custom hex</label>
          <div className="flex items-center gap-1.5">
            <Input
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  commitDraft();
                }
              }}
              placeholder="#1f2937"
              spellCheck={false}
              className="h-8 font-mono text-xs"
            />
            <Button
              type="button"
              size="sm"
              variant="secondary"
              disabled={!HEX_RE.test(draft)}
              onClick={commitDraft}
            >
              Apply
            </Button>
          </div>
        </div>
        {value && (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            className="text-muted-foreground w-full"
            onClick={() => {
              onChange(null);
              setOpen(false);
            }}
          >
            <X className="size-3.5" /> Clear color
          </Button>
        )}
      </PopoverContent>
    </Popover>
  );
}
