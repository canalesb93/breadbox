import { useMemo, useState } from "react";
import { ChevronDown } from "lucide-react";
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
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import {
  buildCronPresets,
  cronToProseLabel,
  isValidCronExpr,
  presetToCronExpr,
} from "@/lib/cron-prose";

interface CronFieldProps {
  value: string;
  onChange: (next: string) => void;
  onBlur?: () => void;
  name?: string;
  placeholder?: string;
}

// CronField is the cron-input + live-preview + preset-picker combo used by
// the agent edit form. Empty value reads as "Manual trigger only";
// non-empty values are validated client-side via isValidCronExpr (a
// permissive structural check — robfig is the canonical validator at
// server registration time).
export function CronField({
  value,
  onChange,
  onBlur,
  name,
  placeholder = "0 9 * * 1",
}: CronFieldProps) {
  const [open, setOpen] = useState(false);
  const trimmed = value.trim();
  const valid = trimmed === "" || isValidCronExpr(trimmed);
  const previewLabel = cronToProseLabel(trimmed);
  // Rebuild per render — the labels embed the live tz, so an open laptop
  // that crosses a DST flip stays accurate without a remount.
  const presets = useMemo(() => buildCronPresets(), []);

  return (
    <div className="space-y-1.5">
      <div className="relative">
        <Input
          name={name}
          value={value}
          placeholder={placeholder}
          onChange={(e) => onChange(e.target.value)}
          onBlur={onBlur}
          className="pr-10 font-mono text-sm"
          aria-invalid={!valid}
        />
        <Popover open={open} onOpenChange={setOpen}>
          <PopoverTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="absolute right-1 top-1/2 size-7 -translate-y-1/2"
              aria-label="Cron presets"
            >
              <ChevronDown className="size-4" />
            </Button>
          </PopoverTrigger>
          <PopoverContent className="w-[320px] p-0" align="end">
            <Command>
              <CommandInput placeholder="Search presets…" />
              <CommandList>
                <CommandEmpty>No matching preset.</CommandEmpty>
                <CommandGroup heading="Common schedules">
                  {presets.map((p) => {
                    const expr = presetToCronExpr(p);
                    return (
                      <CommandItem
                        key={p.key}
                        value={`${p.label} ${expr}`}
                        onSelect={() => {
                          onChange(expr);
                          setOpen(false);
                        }}
                        className="items-start gap-2"
                      >
                        <div className="min-w-0 flex-1">
                          <div className="text-sm">{p.label}</div>
                          {p.hint && (
                            <div className="text-muted-foreground text-[11px]">
                              {p.hint}
                            </div>
                          )}
                        </div>
                        <span className="text-muted-foreground mt-0.5 font-mono text-[10px]">
                          {expr}
                        </span>
                      </CommandItem>
                    );
                  })}
                </CommandGroup>
              </CommandList>
            </Command>
          </PopoverContent>
        </Popover>
      </div>
      <p
        className={cn(
          "text-xs",
          valid ? "text-muted-foreground" : "text-destructive",
        )}
      >
        {valid
          ? `Runs: ${previewLabel}`
          : "Doesn't look like a valid 5-field cron expression"}
      </p>
    </div>
  );
}
