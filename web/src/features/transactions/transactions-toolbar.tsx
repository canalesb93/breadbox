import { useMemo, useState, type ReactNode, type RefObject } from "react";
import {
  ArrowUpDown,
  Banknote,
  Check,
  CheckSquare,
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
import { SearchInput } from "@/components/search-input";
import { KbdTooltip } from "@/components/kbd-tooltip";
import { CategoryCommandList } from "@/components/category-command";
import {
  DateRangeFilter,
  formatDateRangeLabel,
} from "@/components/date-range-filter";
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

function csvSet(raw: string | undefined): Set<string> {
  if (!raw) return new Set();
  return new Set(raw.split(",").map((s) => s.trim()).filter(Boolean));
}

function setToCsv(set: Set<string>): string | undefined {
  if (set.size === 0) return undefined;
  return Array.from(set).join(",");
}

function toggleInSet(set: Set<string>, value: string): Set<string> {
  const next = new Set(set);
  if (next.has(value)) next.delete(value);
  else next.add(value);
  return next;
}

function multiSelectChip(
  key: ChipKey,
  selected: { label: string }[],
  pluralNoun: string,
): { key: ChipKey; label: string } | null {
  if (selected.length === 0) return null;
  if (selected.length === 1) return { key, label: selected[0].label };
  return { key, label: `${selected.length} ${pluralNoun}` };
}

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
  // Flattened only to resolve the active categories' labels for their chip —
  // the Category pill itself uses the shared CategoryCommandList.
  const categories = useMemo(
    () => flattenCategories(categoryTree),
    [categoryTree],
  );

  // `account` and `category` are comma-separated lists; splitting to Sets up
  // here lets the rest of the toolbar treat selection as set membership.
  const accountSlugs = useMemo(
    () => csvSet(search.account),
    [search.account],
  );
  const categorySlugs = useMemo(
    () => csvSet(search.category),
    [search.category],
  );

  const selectedAccounts = useMemo(
    () => (accounts ?? []).filter((a) => accountSlugs.has(a.short_id)),
    [accounts, accountSlugs],
  );
  const selectedCategories = useMemo(
    () => categories.filter((c) => categorySlugs.has(c.slug)),
    [categories, categorySlugs],
  );

  const toggleAccount = (shortId: string) =>
    onChange({ account: setToCsv(toggleInSet(accountSlugs, shortId)) });
  const toggleCategory = (slug: string) =>
    onChange({ category: setToCsv(toggleInSet(categorySlugs, slug)) });
  // Only treat sort as "custom" when the params actually resolve to a known
  // option — a partial/garbage param shouldn't light the pill with the wrong
  // label.
  const foundSort = SORT_OPTIONS.find(
    (o) => o.sort === search.sort && o.dir === search.dir,
  );
  const activeSort = foundSort ?? SORT_OPTIONS[0];
  const sortIsCustom = !!foundSort;

  const chips: { key: ChipKey; label: string }[] = [];
  const accountChip = multiSelectChip(
    "account",
    selectedAccounts.map((a) => ({ label: a.name })),
    "accounts",
  );
  if (accountChip) chips.push(accountChip);
  const categoryChip = multiSelectChip(
    "category",
    selectedCategories.map((c) => ({ label: c.display_name })),
    "categories",
  );
  if (categoryChip) chips.push(categoryChip);
  const dateRangeLabel = formatDateRangeLabel({
    start: search.start,
    end: search.end,
  });
  if (dateRangeLabel) {
    chips.push({ key: "dateRange", label: dateRangeLabel });
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
        <SearchInput
          ref={searchRef}
          containerClassName="w-full min-w-48 sm:w-64"
          value={query}
          onChange={(e) => onQueryChange(e.target.value)}
          // Esc inside the search box blurs back to the page so the global
          // shortcuts (j/k/c/Esc-to-clear-selection) regain control without
          // a manual click-away.
          onKeyDown={(e) => {
            if (e.key === "Escape") e.currentTarget.blur();
          }}
          placeholder="Search merchant or description…"
        />

        {/* Filter pills — grouped so they wrap as a unit, independent of
            the sort/select controls. On <640px the row becomes a horizontal-
            scroll rail (with the `scroll-shadow-x` fade affordance from
            globals.css) so the toolbar stays a single row instead of
            consuming 4+ rows on a 375px viewport. At sm+ it reverts to the
            original `flex-wrap` behavior — at desktop widths the pills fit
            on one row, and on intermediate widths they wrap as before. */}
        <div className="flex items-center gap-2 max-sm:scroll-shadow-x max-sm:flex-nowrap max-sm:overflow-x-auto max-sm:[-webkit-overflow-scrolling:touch] sm:flex-wrap">
          <FilterPill
            icon={Banknote}
            label="Account"
            count={selectedAccounts.length}
            active={selectedAccounts.length > 0}
          >
            <Command>
              <CommandInput placeholder="Search accounts…" />
              <CommandList>
                <CommandEmpty>No accounts found.</CommandEmpty>
                <CommandGroup>
                  {(accounts ?? []).map((a) => (
                    <CommandItem
                      key={a.short_id}
                      value={`${a.name} ${a.institution_name}`}
                      onSelect={() => toggleAccount(a.short_id)}
                    >
                      <span className="truncate">{a.name}</span>
                      <span className="text-muted-foreground ml-1 truncate text-xs">
                        {a.institution_name}
                      </span>
                      {accountSlugs.has(a.short_id) && (
                        <Check className="ml-auto size-4" />
                      )}
                    </CommandItem>
                  ))}
                </CommandGroup>
              </CommandList>
            </Command>
          </FilterPill>

          <FilterPill
            icon={Shapes}
            label="Category"
            count={selectedCategories.length}
            active={selectedCategories.length > 0}
          >
            <CategoryCommandList
              selectedSlugs={categorySlugs}
              onPick={({ category_slug }) => {
                if (!category_slug) return;
                toggleCategory(category_slug);
              }}
            />
          </FilterPill>

          <DateRangeFilter
            value={{ start: search.start, end: search.end }}
            onChange={(range) =>
              onChange({ start: range.start, end: range.end })
            }
          />

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

        {/* Sort + select mode — grouped so they stay together when the row wraps.
            On sm+, `ml-auto` pushes them to the right edge alongside the filter
            pills. On mobile we deliberately omit the spacer so they sit inline
            with the filter pills instead of getting flung to the right of an
            empty row. */}
        <div className="flex items-center gap-2 sm:ml-auto">
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
                className="text-muted-foreground hover:text-foreground focus-visible:ring-ring/50 focus-visible:ring-[3px] focus-visible:outline-none -mr-0.5 rounded-sm"
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
  /** When > 1, rendered as a numeric badge after the label — multi-select hint. */
  count?: number;
  className?: string;
  children: ReactNode;
}

function FilterPill({
  icon: Icon,
  label,
  active,
  count,
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
          {count != null && count > 1 && (
            <Badge variant="secondary" className="ml-0.5 px-1.5 py-0">
              {count}
            </Badge>
          )}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className={cn("w-56 p-0", className)}>
        {children}
      </PopoverContent>
    </Popover>
  );
}
