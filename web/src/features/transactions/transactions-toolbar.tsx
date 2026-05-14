import { useMemo, useState, type ReactNode, type RefObject } from "react";
import {
  ArrowUpDown,
  Banknote,
  CalendarRange,
  Check,
  CheckSquare,
  CircleDot,
  DollarSign,
  Search,
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
import { KbdTooltip } from "@/components/kbd-tooltip";
import { CategoryCommandList } from "@/components/category-command";
import { formatLongDate } from "@/lib/format";
import { cn } from "@/lib/utils";
import { useAccounts } from "@/api/queries/accounts";
import { useCategories, flattenCategories } from "@/api/queries/categories";
import type { TransactionsSearch } from "@/routes/transactions";

interface TransactionsToolbarProps {
  search: TransactionsSearch;
  /** Merge a patch into the URL search params. Pass undefined to clear a key. */
  onChange: (patch: Partial<TransactionsSearch>) => void;
  /** Free-text search box — debounced into the URL by the route. */
  query: string;
  onQueryChange: (value: string) => void;
  searchRef: RefObject<HTMLInputElement | null>;
  /** Select-mode toggle, owned by the route. */
  selectMode: boolean;
  onToggleSelect: () => void;
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

// A removable filter chip's key — either a real search param, or a sentinel
// for the two range filters that clear a pair of params at once.
type ChipKey = keyof TransactionsSearch | "dateRange" | "amountRange";

// TransactionsToolbar is the transactions page's single control band: a search
// box, a row of popover-backed filter pills, a sort control and the select-mode
// toggle — plus a removable chip for every active filter. It's controlled — all
// state lives in the URL (filters) or the route (search text, select mode).
export function TransactionsToolbar({
  search,
  onChange,
  query,
  onQueryChange,
  searchRef,
  selectMode,
  onToggleSelect,
}: TransactionsToolbarProps) {
  const { data: accounts } = useAccounts();
  const { data: categoryTree } = useCategories();
  // Flattened only to resolve the active category's label for its chip — the
  // Category pill itself uses the shared CategoryCommandList.
  const categories = useMemo(
    () => flattenCategories(categoryTree),
    [categoryTree],
  );

  const account = accounts?.find((a) => a.short_id === search.account);
  const category = categories.find((c) => c.slug === search.category);
  // Only treat sort as "custom" when the params actually resolve to a known
  // option — a partial/garbage param shouldn't light the pill with the wrong
  // label.
  const foundSort = SORT_OPTIONS.find(
    (o) => o.sort === search.sort && o.dir === search.dir,
  );
  const activeSort = foundSort ?? SORT_OPTIONS[0];
  const sortIsCustom = !!foundSort;

  const chips: { key: ChipKey; label: string }[] = [];
  if (account) chips.push({ key: "account", label: account.name });
  if (category) chips.push({ key: "category", label: category.display_name });
  if (search.start || search.end) {
    const from = search.start ? formatLongDate(search.start) : "Any";
    const to = search.end ? formatLongDate(search.end) : "Any";
    chips.push({ key: "dateRange", label: `${from} – ${to}` });
  }
  if (search.min != null || search.max != null) {
    const lo = search.min != null ? `$${search.min}` : "Any";
    const hi = search.max != null ? `$${search.max}` : "Any";
    chips.push({ key: "amountRange", label: `${lo} – ${hi}` });
  }
  if (search.pending) {
    chips.push({
      key: "pending",
      label: search.pending === "true" ? "Pending only" : "Posted only",
    });
  }

  const clearChip = (key: ChipKey) => {
    if (key === "dateRange") onChange({ start: undefined, end: undefined });
    else if (key === "amountRange") onChange({ min: undefined, max: undefined });
    else onChange({ [key]: undefined });
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
        <div className="relative w-full min-w-48 sm:w-64">
          <Search className="text-muted-foreground absolute top-1/2 left-2.5 size-4 -translate-y-1/2" />
          <Input
            ref={searchRef}
            value={query}
            onChange={(e) => onQueryChange(e.target.value)}
            placeholder="Search merchant or description…"
            className="pl-8"
          />
        </div>

        {/* Filter pills — grouped so they wrap as a unit, independent of
            the sort/select controls. */}
        <div className="flex flex-wrap items-center gap-2">
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

          <FilterPill icon={Shapes} label="Category" active={!!category}>
            <CategoryCommandList
              currentSlug={search.category ?? null}
              onPick={({ category_slug }) =>
                onChange({
                  category:
                    category_slug && search.category === category_slug
                      ? undefined
                      : category_slug,
                })
              }
            />
          </FilterPill>

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
                  onChange={(e) =>
                    onChange({ end: e.target.value || undefined })
                  }
                />
              </div>
            </div>
          </FilterPill>

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
                      min: e.target.value
                        ? Number(e.target.value)
                        : undefined,
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
                      max: e.target.value
                        ? Number(e.target.value)
                        : undefined,
                    })
                  }
                />
              </div>
            </div>
          </FilterPill>

          <FilterPill
            icon={CircleDot}
            label="Status"
            active={!!search.pending}
          >
            <Command>
              <CommandList>
                <CommandGroup>
                  {(
                    [
                      { label: "Any status", value: undefined },
                      { label: "Pending only", value: "true" as const },
                      { label: "Posted only", value: "false" as const },
                    ] satisfies {
                      label: string;
                      value: "true" | "false" | undefined;
                    }[]
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
        </div>

        <div className="grow" />

        {/* Sort + select mode — grouped so they stay together when the row wraps */}
        <div className="flex items-center gap-2">
          <FilterPill
            icon={ArrowUpDown}
            label={activeSort.label}
            active={sortIsCustom}
          >
            <Command>
              <CommandList>
                <CommandGroup>
                  {SORT_OPTIONS.map((opt) => (
                    <CommandItem
                      key={opt.label}
                      value={opt.label}
                      onSelect={() => onChange({ sort: opt.sort, dir: opt.dir })}
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

          {selectMode ? (
            <KbdTooltip label="Clear selection / exit" keys={["Esc"]}>
              <Button variant="secondary" size="sm" onClick={onToggleSelect}>
                <X className="size-4" />
                Done
              </Button>
            </KbdTooltip>
          ) : (
            <KbdTooltip label="Select transactions" keys={["x"]}>
              <Button variant="outline" size="sm" onClick={onToggleSelect}>
                <CheckSquare className="size-4" />
                Select
              </Button>
            </KbdTooltip>
          )}
        </div>
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
      <PopoverContent align="start" className={cn("w-56 p-0", className)}>
        {children}
      </PopoverContent>
    </Popover>
  );
}
