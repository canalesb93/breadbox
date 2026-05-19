import { useState } from "react";
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
  CRON_PRESETS,
  cronToProseLabel,
  isValidCronExpr,
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
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
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
          <PopoverContent className="w-72 p-0" align="end">
            <Command>
              <CommandInput placeholder="Search presets…" />
              <CommandList>
                <CommandEmpty>No matching preset.</CommandEmpty>
                <CommandGroup heading="Common schedules">
                  {CRON_PRESETS.map((p) => (
                    <CommandItem
                      key={p.value}
                      value={`${p.value} ${p.label}`}
                      onSelect={() => {
                        onChange(p.value);
                        setOpen(false);
                      }}
                    >
                      <span className="flex-1">{p.label}</span>
                      <span className="text-muted-foreground font-mono text-[10px]">
                        {p.value}
                      </span>
                    </CommandItem>
                  ))}
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
