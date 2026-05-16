import { useState } from "react";
import type { ColumnDef } from "@tanstack/react-table";
import { toast } from "sonner";
import { Inbox, Plus, RotateCcw } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { PageHeader } from "@/components/page-header";
import { EmptyState } from "@/components/empty-state";
import { DataTable } from "@/components/data-table";
import { CategoryBadge } from "@/components/category-badge";
import { CategoryIconTile } from "@/components/category-icon-tile";
import { CategoryCommandList } from "@/components/category-command";
import { DateRangeFilter } from "@/components/date-range-filter";
import type { DateRangeValue } from "@/components/date-range-filter";
import { TagChip, TagList } from "@/components/tag-chip";
import { TagCommandList } from "@/components/tag-command";
import { TransactionPrimary } from "@/components/transaction-primary";
import { TransactionAmount } from "@/components/transaction-amount";
import { KbdTooltip } from "@/components/kbd-tooltip";
import { ProviderPicker } from "@/features/connections/provider-picker";
import type { Transaction } from "@/api/types";
import { SandboxSection, Specimen } from "@/sandbox/kit";
import {
  sampleTags,
  sampleTransactions,
} from "@/sandbox/fixtures";

const coffeeCategory = sampleTransactions[0].category;
const gasCategory = sampleTransactions[2].category;

// Lightweight columns for the DataTable demo — the row primitives without the
// live category mutation, so the sandbox stays display-only.
const demoColumns: ColumnDef<Transaction>[] = [
  {
    id: "description",
    header: "Description",
    cell: ({ row }) => <TransactionPrimary transaction={row.original} />,
  },
  {
    id: "category",
    header: "Category",
    meta: { className: "w-px" },
    cell: ({ row }) => (
      <CategoryBadge
        category={row.original.category}
        overridden={row.original.category_override}
      />
    ),
  },
  {
    id: "amount",
    header: () => <div className="text-right">Amount</div>,
    meta: { className: "w-px" },
    cell: ({ row }) => <TransactionAmount transaction={row.original} />,
  },
];

export function ComponentsSection() {
  const [tableState, setTableState] = useState<
    "data" | "loading" | "empty"
  >("data");
  const [pickedProvider, setPickedProvider] = useState<string | null>("plaid");
  const [dateRange, setDateRange] = useState<DateRangeValue>({});

  return (
    <SandboxSection
      title="Components"
      description="Composed, reusable v2 components — the layer built on top of the primitives. Shared across the transactions list, detail page, and (soon) elsewhere."
    >
      <Specimen
        label="PageHeader"
        code="components/page-header"
        className="block"
      >
        <PageHeader
          title="Transactions"
          description="Every transaction synced across your connected accounts."
          actions={<Button size="sm">Action</Button>}
        />
      </Specimen>

      <Specimen
        label="EmptyState"
        code="components/empty-state"
        className="block"
      >
        <EmptyState
          icon={Inbox}
          title="No matching transactions"
          description="Try adjusting or clearing your filters."
          action={<Button variant="outline">Clear filters</Button>}
        />
      </Specimen>

      <Specimen
        label="CategoryIconTile"
        code="components/category-icon-tile"
        description="Rounded, color-tinted icon chip that fronts a transaction. Falls back to a neutral receipt glyph."
      >
        <CategoryIconTile
          icon={coffeeCategory?.icon}
          color={coffeeCategory?.color}
          size="sm"
        />
        <CategoryIconTile
          icon={gasCategory?.icon}
          color={gasCategory?.color}
          size="md"
        />
        <CategoryIconTile
          icon={coffeeCategory?.icon}
          color={coffeeCategory?.color}
          size="lg"
        />
        <CategoryIconTile size="md" />
      </Specimen>

      <Specimen
        label="CategoryBadge"
        code="components/category-badge"
        description="Single rendering of a category — rounded-rect, color-tinted. The ring marks a manual override; em-dash when uncategorized."
      >
        <CategoryBadge category={coffeeCategory} />
        <CategoryBadge category={gasCategory} />
        <CategoryBadge category={coffeeCategory} overridden />
        <CategoryBadge category={null} />
      </Specimen>

      <Specimen
        label="TagChip · TagList"
        code="components/tag-chip"
        description="Pill-shaped (the shape category badges deliberately avoid). TagList resolves slugs against the tag catalog and caps with a +N overflow."
        className="block"
      >
        <div className="flex flex-wrap items-center gap-2">
          <TagChip tag={sampleTags[0]} />
          <TagChip tag={sampleTags[1]} />
          <TagChip
            tag={sampleTags[2]}
            onRemove={() => toast.message("Removed Subscription")}
          />
        </div>
        <div className="mt-3">
          <TagList
            slugs={["needs-review", "business", "subscription", "reimbursable"]}
            max={2}
          />
        </div>
      </Specimen>

      <Specimen
        label="TransactionPrimary · TransactionAmount"
        code="components/transaction-*"
        description="The reusable transaction row pieces — icon + title + metadata subtitle, and the signed amount + date. Inflows render in the success color."
        className="block divide-y"
      >
        {sampleTransactions.slice(0, 3).map((t) => (
          <div
            key={t.id}
            className="flex items-center justify-between gap-4 py-2 first:pt-0 last:pb-0"
          >
            <TransactionPrimary transaction={t} />
            <TransactionAmount transaction={t} />
          </div>
        ))}
      </Specimen>

      <Specimen
        label="CategoryCommandList"
        code="components/category-command"
        description="Searchable, parent-grouped category list. Pure — the caller owns the mutation; CategoryPicker / CategoryEditor wrap it with the live update."
      >
        <Popover>
          <PopoverTrigger asChild>
            <Button variant="outline">
              <Plus /> Pick a category
            </Button>
          </PopoverTrigger>
          <PopoverContent className="w-64 p-0" align="start">
            <CategoryCommandList
              showReset
              currentSlug="food_and_drink_coffee"
              onPick={(pick) =>
                toast.message(
                  pick.reset_category
                    ? "Reset to provider default"
                    : `Picked: ${pick.category_slug}`,
                )
              }
            />
          </PopoverContent>
        </Popover>
        <span className="text-muted-foreground text-xs">
          <RotateCcw className="mr-1 inline size-3" />
          showReset adds the reset entry
        </span>
      </Specimen>

      <Specimen
        label="TagCommandList"
        code="components/tag-command"
        description="The tag equivalent — flat searchable list with a check on already-attached tags."
      >
        <Popover>
          <PopoverTrigger asChild>
            <Button variant="outline">
              <Plus /> Add a tag
            </Button>
          </PopoverTrigger>
          <PopoverContent className="w-56 p-0" align="start">
            <TagCommandList
              attachedSlugs={new Set(["needs-review"])}
              onPick={(slug) => toast.message(`Toggled tag: ${slug}`)}
            />
          </PopoverContent>
        </Popover>
      </Specimen>

      <Specimen
        label="KbdTooltip"
        code="components/kbd-tooltip"
        description="Tooltip with a keyboard-shortcut hint. Hover the button."
      >
        <KbdTooltip label="Select transactions" keys={["x"]}>
          <Button variant="outline">Select</Button>
        </KbdTooltip>
        <KbdTooltip label="Command palette" keys={["mod", "k"]}>
          <Button variant="outline">Search</Button>
        </KbdTooltip>
      </Specimen>

      <Specimen
        label="ProviderPicker"
        code="features/connections/provider-picker"
        description="Stacked provider cards used in the Connect-bank Sheet (and the future hosted-link page). `enabledProviders` gates which cards are clickable; everything else renders as 'Not configured'."
        className="block"
      >
        <div className="max-w-sm">
          <ProviderPicker
            enabledProviders={["plaid"]}
            providers={["plaid", "teller", "csv"]}
            value={pickedProvider}
            onChange={setPickedProvider}
          />
        </div>
      </Specimen>

      <Specimen
        label="DateRangeFilter"
        code="components/date-range-filter"
        description="Date-range filter pill — preset chips beside a 2-month calendar (single month on mobile). Click to open."
      >
        <DateRangeFilter value={dateRange} onChange={setDateRange} />
        <span className="text-muted-foreground text-xs">
          {dateRange.start || dateRange.end
            ? `${dateRange.start ?? "any"} → ${dateRange.end ?? "any"}`
            : "no range selected"}
        </span>
      </Specimen>

      <Specimen
        label="DataTable"
        code="components/data-table"
        description="The shared table — every v2 list uses it. Owns nothing: data, sorting, selection, and per-column className all come from the caller. Toggle its states:"
        className="block"
      >
        <div className="mb-3 flex gap-2">
          {(["data", "loading", "empty"] as const).map((s) => (
            <Button
              key={s}
              size="sm"
              variant={tableState === s ? "default" : "outline"}
              onClick={() => setTableState(s)}
            >
              {s}
            </Button>
          ))}
        </div>
        <DataTable
          columns={demoColumns}
          data={tableState === "data" ? sampleTransactions : []}
          isLoading={tableState === "loading"}
          loadingRows={4}
          emptyState={
            <EmptyState
              icon={Inbox}
              title="No transactions"
              description="Nothing matches the current view."
            />
          }
        />
      </Specimen>
    </SandboxSection>
  );
}
