import { useMemo, useState } from "react";
import {
  startOfMonth,
  endOfMonth,
  startOfYear,
  subDays,
  subMonths,
  format as formatFns,
} from "date-fns";
import { CalendarRange } from "lucide-react";
import type { DateRange } from "react-day-picker";
import { Button } from "@/components/ui/button";
import { Calendar } from "@/components/ui/calendar";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Separator } from "@/components/ui/separator";
import { useMediaQuery } from "@/hooks/use-media-query";
import { formatLongDate, parseIsoDate } from "@/lib/format";
import { cn } from "@/lib/utils";

export interface DateRangeValue {
  /** ISO YYYY-MM-DD, inclusive. */
  start?: string;
  /** ISO YYYY-MM-DD, inclusive. */
  end?: string;
}

interface DateRangeFilterProps {
  value: DateRangeValue;
  onChange: (next: DateRangeValue) => void;
  /** Override the trigger label; defaults to "Date". */
  label?: string;
}

interface Preset {
  key: string;
  label: string;
  range: () => DateRangeValue;
}

const PRESETS: Preset[] = [
  {
    key: "today",
    label: "Today",
    range: () => {
      const iso = toIso(new Date());
      return { start: iso, end: iso };
    },
  },
  {
    key: "yesterday",
    label: "Yesterday",
    range: () => {
      const iso = toIso(subDays(new Date(), 1));
      return { start: iso, end: iso };
    },
  },
  {
    key: "last_7",
    label: "Last 7 days",
    range: () => ({
      start: toIso(subDays(new Date(), 6)),
      end: toIso(new Date()),
    }),
  },
  {
    key: "last_30",
    label: "Last 30 days",
    range: () => ({
      start: toIso(subDays(new Date(), 29)),
      end: toIso(new Date()),
    }),
  },
  {
    key: "this_month",
    label: "This month",
    range: () => ({
      start: toIso(startOfMonth(new Date())),
      end: toIso(new Date()),
    }),
  },
  {
    key: "last_month",
    label: "Last month",
    range: () => {
      const prev = subMonths(new Date(), 1);
      return {
        start: toIso(startOfMonth(prev)),
        end: toIso(endOfMonth(prev)),
      };
    },
  },
  {
    key: "ytd",
    label: "Year to date",
    range: () => ({
      start: toIso(startOfYear(new Date())),
      end: toIso(new Date()),
    }),
  },
  {
    key: "all_time",
    label: "All time",
    range: () => ({ start: undefined, end: undefined }),
  },
];

// formatDateRangeLabel renders a value as a human label — the matching preset
// name when one fits, otherwise "<from> – <to>". Returns undefined when no
// range is set so callers can fall back to a default trigger label.
export function formatDateRangeLabel(
  value: DateRangeValue,
): string | undefined {
  if (!value.start && !value.end) return undefined;
  const preset = matchPreset(value);
  if (preset && preset.key !== "all_time") return preset.label;
  const from = value.start ? formatLongDate(value.start) : "Any";
  const to = value.end ? formatLongDate(value.end) : "Any";
  return `${from} – ${to}`;
}

export function DateRangeFilter({
  value,
  onChange,
  label = "Date",
}: DateRangeFilterProps) {
  const [open, setOpen] = useState(false);
  const isDesktop = useMediaQuery("(min-width: 640px)");
  const active = !!(value.start || value.end);
  const activePreset = useMemo(() => matchPreset(value), [value]);
  const triggerLabel = useMemo(
    () => formatDateRangeLabel(value),
    [value],
  );

  const dayPickerRange: DateRange | undefined = useMemo(() => {
    if (!value.start && !value.end) return undefined;
    return {
      from: value.start ? parseIsoDate(value.start) : undefined,
      to: value.end ? parseIsoDate(value.end) : undefined,
    };
  }, [value.start, value.end]);

  const applyPreset = (preset: Preset) => {
    onChange(preset.range());
    setOpen(false);
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          className={cn(
            "gap-1.5",
            active && "border-primary/50 text-primary",
          )}
        >
          <CalendarRange className="size-3.5" />
          {triggerLabel ?? label}
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        className="flex w-auto max-w-[calc(100dvw-2rem)] flex-col p-0 sm:flex-row"
      >
        <ul className="flex flex-wrap gap-1 p-2 sm:w-36 sm:flex-col sm:gap-0.5 sm:border-r sm:p-2">
          {PRESETS.map((preset) => (
            <li key={preset.key}>
              <button
                type="button"
                onClick={() => applyPreset(preset)}
                className={cn(
                  "w-full rounded-md px-2 py-1.5 text-left text-xs hover:bg-accent hover:text-accent-foreground",
                  // Shared focus-visible recipe so keyboard users can see
                  // which preset is focused before pressing Enter.
                  "focus-visible:ring-ring/50 focus-visible:ring-[3px] focus-visible:outline-none",
                  activePreset?.key === preset.key &&
                    "bg-accent font-medium text-accent-foreground",
                )}
              >
                {preset.label}
              </button>
            </li>
          ))}
        </ul>
        <Separator className="sm:hidden" />
        <div className="p-2">
          <Calendar
            mode="range"
            numberOfMonths={isDesktop ? 2 : 1}
            selected={dayPickerRange}
            onSelect={(range) =>
              onChange({
                start: range?.from ? toIso(range.from) : undefined,
                end: range?.to ? toIso(range.to) : undefined,
              })
            }
            defaultMonth={
              value.start
                ? parseIsoDate(value.start)
                : isDesktop
                  ? subMonths(new Date(), 1)
                  : new Date()
            }
          />
        </div>
      </PopoverContent>
    </Popover>
  );
}

// toIso renders a Date as YYYY-MM-DD in the local timezone. `.toISOString()`
// would convert to UTC and shift the day for anyone west of GMT.
function toIso(d: Date): string {
  return formatFns(d, "yyyy-MM-dd");
}

function matchPreset(value: DateRangeValue): Preset | undefined {
  for (const preset of PRESETS) {
    const r = preset.range();
    if (r.start === value.start && r.end === value.end) return preset;
  }
  return undefined;
}
