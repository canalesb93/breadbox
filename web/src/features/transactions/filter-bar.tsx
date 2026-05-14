import { useState, type ReactNode } from "react";
import {
  ArrowUpDown,
  Banknote,
  CalendarRange,
  Check,
  CircleDot,
  DollarSign,
  Shapes,
  X,
} from "lucide-react";
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
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { DynamicIcon } from "@/lib/icon";
import { formatLongDate } from "@/lib/format";
import { cn } from "@/lib/utils";
import { useAccounts } from "@/api/queries/accounts";
import { useCategories, flattenCategories } from "@/api/queries/categories";
import type { TransactionsSearch } from "@/routes/transactions";

interface FilterBarProps {
  search: TransactionsSearch;
  /** Merge a patch into the URL search params. Pass undefined to clear a key. */
  onChange: (patch: Partial<TransactionsSearch>) => void;
}

const SORT_OPTIONS: {
  label: string;
  sort: TransactionsSearch["sort"];
  dir: TransactionsSearch["dir"];
}[] = [
  { label: "Newest first", sort: "date", dir: "desc" },
  { label: "Oldest first", sort: "date", dir: "asc" },
  { label: "Largest amount", sort: "amount", dir: "desc" },
  { label: "Smallest amount", sort: "amount", dir: "asc" },
];

// FilterBar is the transactions filter strip: a row of popover-backed filter
// pills plus a removable chip for every active filter. It's a controlled
// component — all state lives in the URL, owned by the route.
export function FilterBar({ search, onChange }: FilterBarProps) {
  const { data: accounts } = useAccounts();
  const { data: categoryTree } = useCategories();
  const categories = flattenCategories(categoryTree);

  const account = accounts?.find((a) => a.short_id === search.account);
  const category = categories.find((c) => c.slug === search.category);
  const activeSort =
    SORT_OPTIONS.find((o) => o.sort === search.sort && o.dir === search.dir) ??
    SORT_OPTIONS[0];

  const chips: { key: keyof TransactionsSearch; label: string }[] = [];
  if (account) chips.push({ key: "account", label: account.name });
  if (category) chips.push({ key: "category", label: category.display_name });
  if (search.start || search.end) {
    const from = search.start ? formatLongDate(search.start) : "Any";
    const to = search.end ? formatLongDate(search.end) : "Any";
    chips.push({ key: "start", label: `${from} – ${to}` });
  }
  if (search.min != null || search.max != null) {
    const lo = search.min != null ? `$${search.min}` : "Any";
    const hi = search.max != null ? `$${search.max}` : "Any";
    chips.push({ key: "min", label: `${lo} – ${hi}` });
  }
  if (search.pending) {
    chips.push({
      key: "pending",
      label: search.pending === "true" ? "Pending only" : "Posted only",
    });
  }

  const clearChip = (key: keyof TransactionsSearch) => {
    if (key === "start") onChange({ start: undefined, end: undefined });
    else if (key === "min") onChange({ min: undefined, max: undefined });
    else onChange({ [key]: undefined } as Partial<TransactionsSearch>);
  };

  const clearAll = () =>
    onChange({
      account: undefined,
      category: undefined,
      start: undefined,
      end: undefined,
      min: undefined,
      max: undefined,
      pending: undefined,
    });

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center gap-2">
        {/* Account */}
        <FilterPill icon={Banknote} label="Account" active={!!account}>
          <Command>
            <CommandInput placeholder="Search accounts…" />
            <CommandList>
              <CommandEmpty>No accounts found.</CommandEmpty>
              <CommandGroup>
                {(accounts ?? []).map((a) => (
                  <CommandItem
                    key={a.short_id}
                    value={`${a.name} ${a.institution_name}`}
                    onSelect={() =>
                      onChange({
                        account:
                          search.account === a.short_id
                            ? undefined
                            : a.short_id,
                      })
                    }
                  >
                    <span className="truncate">{a.name}</span>
                    <span className="text-muted-foreground ml-1 truncate text-xs">
                      {a.institution_name}
                    </span>
                    {search.account === a.short_id && (
                      <Check className="ml-auto size-4" />
                    )}
                  </CommandItem>
                ))}
              </CommandGroup>
            </CommandList>
          </Command>
        </FilterPill>

        {/* Category */}
        <FilterPill icon={Shapes} label="Category" active={!!category}>
          <Command>
            <CommandInput placeholder="Search categories…" />
            <CommandList>
              <CommandEmpty>No categories found.</CommandEmpty>
              <CommandGroup>
                {categories.map((c) => (
                  <CommandItem
                    key={c.slug}
                    value={`${c.display_name} ${c.parent_display_name ?? ""}`}
                    onSelect={() =>
                      onChange({
                        category:
                          search.category === c.slug ? undefined : c.slug,
                      })
                    }
                  >
                    <DynamicIcon
                      name={c.icon}
                      className="size-4"
                      style={c.color ? { color: c.color } : undefined}
                    />
                    <span className={cn(c.parent_id && "text-muted-foreground")}>
                      {c.parent_display_name
                        ? `${c.parent_display_name} › ${c.display_name}`
                        : c.display_name}
                    </span>
                    {search.category === c.slug && (
                      <Check className="ml-auto size-4" />
                    )}
                  </CommandItem>
                ))}
              </CommandGroup>
            </CommandList>
          </Command>
        </FilterPill>

        {/* Date range */}
        <FilterPill
          icon={CalendarRange}
          label="Date"
          active={!!(search.start || search.end)}
          className="w-64 p-3"
        >
          <div className="space-y-3">
            <div className="space-y-1.5">
              <Label className="text-xs">From</Label>
              <Input
                type="date"
                value={search.start ?? ""}
                onChange={(e) =>
                  onChange({ start: e.target.value || undefined })
                }
              />
            </div>
            <div className="space-y-1.5">
              <Label className="text-xs">To</Label>
              <Input
                type="date"
                value={search.end ?? ""}
                onChange={(e) => onChange({ end: e.target.value || undefined })}
              />
            </div>
          </div>
        </FilterPill>

        {/* Amount range */}
        <FilterPill
          icon={DollarSign}
          label="Amount"
          active={search.min != null || search.max != null}
          className="w-56 p-3"
        >
          <div className="space-y-3">
            <div className="space-y-1.5">
              <Label className="text-xs">Min</Label>
              <Input
                type="number"
                inputMode="decimal"
                placeholder="0.00"
                value={search.min ?? ""}
                onChange={(e) =>
                  onChange({
                    min: e.target.value ? Number(e.target.value) : undefined,
                  })
                }
              />
            </div>
            <div className="space-y-1.5">
              <Label className="text-xs">Max</Label>
              <Input
                type="number"
                inputMode="decimal"
                placeholder="0.00"
                value={search.max ?? ""}
                onChange={(e) =>
                  onChange({
                    max: e.target.value ? Number(e.target.value) : undefined,
                  })
                }
              />
            </div>
          </div>
        </FilterPill>

        {/* Pending */}
        <FilterPill icon={CircleDot} label="Status" active={!!search.pending}>
          <Command>
            <CommandList>
              <CommandGroup>
                {(
                  [
                    { label: "Any status", value: undefined },
                    { label: "Pending only", value: "true" as const },
                    { label: "Posted only", value: "false" as const },
                  ] satisfies { label: string; value: "true" | "false" | undefined }[]
                ).map((opt) => (
                  <CommandItem
                    key={opt.label}
                    value={opt.label}
                    onSelect={() => onChange({ pending: opt.value })}
                  >
                    {opt.label}
                    {search.pending === opt.value && (
                      <Check className="ml-auto size-4" />
                    )}
                  </CommandItem>
                ))}
              </CommandGroup>
            </CommandList>
          </Command>
        </FilterPill>

        <div className="grow" />

        {/* Sort */}
        <FilterPill icon={ArrowUpDown} label={activeSort.label} active>
          <Command>
            <CommandList>
              <CommandGroup>
                {SORT_OPTIONS.map((opt) => (
                  <CommandItem
                    key={opt.label}
                    value={opt.label}
                    onSelect={() =>
                      onChange({ sort: opt.sort, dir: opt.dir })
                    }
                  >
                    {opt.label}
                    {activeSort.label === opt.label && (
                      <Check className="ml-auto size-4" />
                    )}
                  </CommandItem>
                ))}
              </CommandGroup>
            </CommandList>
          </Command>
        </FilterPill>
      </div>

      {chips.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5">
          {chips.map((chip) => (
            <Badge key={chip.key} variant="secondary" className="gap-1">
              {chip.label}
              <button
                type="button"
                onClick={() => clearChip(chip.key)}
                aria-label={`Clear ${chip.label}`}
                className="text-muted-foreground hover:text-foreground -mr-0.5"
              >
                <X className="size-3" />
              </button>
            </Badge>
          ))}
          <Button
            variant="ghost"
            size="sm"
            onClick={clearAll}
            className="h-6 px-2 text-xs"
          >
            Clear all
          </Button>
        </div>
      )}
    </div>
  );
}

interface FilterPillProps {
  icon: typeof Banknote;
  label: string;
  active?: boolean;
  className?: string;
  children: ReactNode;
}

function FilterPill({
  icon: Icon,
  label,
  active,
  className,
  children,
}: FilterPillProps) {
  const [open, setOpen] = useState(false);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          className={cn("gap-1.5", active && "border-primary/50 text-primary")}
        >
          <Icon className="size-3.5" />
          {label}
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        className={cn("w-56 p-0", className)}
      >
        {children}
      </PopoverContent>
    </Popover>
  );
}
