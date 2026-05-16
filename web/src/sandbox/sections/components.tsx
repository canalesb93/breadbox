import { useState } from "react";
import type { ColumnDef } from "@tanstack/react-table";
import { toast } from "sonner";
import {
  AlertTriangle,
  ArrowUpRight,
  CheckCircle2,
  Inbox,
  Info,
  KeyRound,
  MessageSquare,
  Plus,
  RefreshCw,
  RotateCcw,
  Save,
  Shapes,
  ShieldAlert,
  Tag,
  Trash2,
  Users,
  Wand2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { PageHeader } from "@/components/page-header";
import { EmptyState } from "@/components/empty-state";
import { TimelineRail } from "@/components/timeline-rail";
import { DataTable } from "@/components/data-table";
import { CategoryBadge } from "@/components/category-badge";
import { CategoryIconTile } from "@/components/category-icon-tile";
import { CategoryCommandList } from "@/components/category-command";
import { DateRangeFilter } from "@/components/date-range-filter";
import type { DateRangeValue } from "@/components/date-range-filter";
import { TagChip, TagList } from "@/components/tag-chip";
import { TagCommandList } from "@/components/tag-command";
import { IconPicker } from "@/components/icon-picker";
import { ColorPicker } from "@/components/color-picker";
import { TransactionPrimary } from "@/components/transaction-primary";
import { TransactionAmount } from "@/components/transaction-amount";
import { KbdTooltip } from "@/components/kbd-tooltip";
import { ListCard } from "@/components/list-card";
import { ColorRailCard } from "@/components/color-rail-card";
import { SectionCard } from "@/components/section-card";
import { IdPill } from "@/components/id-pill";
import { SoftBackButton } from "@/components/soft-back-button";
import { StatusPanel } from "@/components/status-panel";
import { FormFooter } from "@/components/form-footer";
import { ConfirmDialog } from "@/components/confirm-dialog";
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
  const [iconValue, setIconValue] = useState<string | null>("shopping-cart");
  const [colorValue, setColorValue] = useState<string | null>("#f97316");
  const [pickedProvider, setPickedProvider] = useState<string | null>("plaid");
  const [dateRange, setDateRange] = useState<DateRangeValue>({});
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [confirmPending, setConfirmPending] = useState(false);

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
        label="SoftBackButton"
        code="components/soft-back-button"
        description="The tiny ghost link that hangs at the top of every detail / form page. Prefers in-app history on a plain click (so you land on the exact list state you came from), falls through to the canonical `to` URL otherwise. Don't fork the look."
        className="block"
      >
        <SoftBackButton to="/v2/transactions">
          Back to transactions
        </SoftBackButton>
      </Specimen>

      <Specimen
        label="SectionCard"
        code="components/section-card"
        description="The bordered 'section in a card' primitive: header rail names the section, optional action slot on the right, body holds prose / forms / KV blocks. Use `flushBody` when the body is a `<ul className='divide-y'>` (or reach for `ListCard`, which bakes that in)."
        className="block"
      >
        <div className="grid gap-4 md:grid-cols-2">
          <SectionCard
            title="Details"
            action={
              <Button size="sm" variant="outline">
                Edit
              </Button>
            }
          >
            <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
              <dt className="text-muted-foreground">Reference</dt>
              <dd className="font-mono text-xs">tx_aB12cD34</dd>
              <dt className="text-muted-foreground">Created</dt>
              <dd>May 14, 2026</dd>
              <dt className="text-muted-foreground">Source</dt>
              <dd>Plaid · Chase ····2890</dd>
            </dl>
          </SectionCard>
          <SectionCard
            title="Notes"
            footer={
              <Button size="sm" variant="ghost">
                View all
              </Button>
            }
          >
            <p className="text-muted-foreground text-sm leading-relaxed">
              Reach for `SectionCard` for any non-list section. The footer slot
              renders a flush bordered strip — typically a "See all" link.
            </p>
          </SectionCard>
        </div>
      </Specimen>

      <Specimen
        label="ListCard"
        code="components/list-card"
        description="`SectionCard`'s sibling for divide-y lists. Pass `rows` + `renderRow` and each row is wrapped in an `<li>` automatically — the rail of dividers stays consistent across surfaces (Home recent activity, Account-detail recent transactions, Connections list, …)."
        className="block"
      >
        <div className="max-w-md">
          <ListCard
            title="Recent transactions"
            action={
              <Button size="sm" variant="ghost">
                View all
              </Button>
            }
            rows={sampleTransactions.slice(0, 3)}
            getRowKey={(t) => t.id}
            renderRow={(t) => (
              <div className="flex items-center justify-between gap-4 px-5 py-3">
                <TransactionPrimary transaction={t} />
                <TransactionAmount transaction={t} />
              </div>
            )}
            empty={
              <EmptyState
                variant="inline"
                icon={Inbox}
                title="Nothing yet"
                description="New transactions will appear here."
              />
            }
          />
        </div>
      </Specimen>

      <Specimen
        label="ColorRailCard"
        code="components/color-rail-card"
        description="The canonical 'detail-page hero': bordered card with a 4px coloured left rail that encodes meaning (category colour, accounting role, connection status). Optional `footer` slot hosts an inline action strip. Pair the rail with a small uppercase eyebrow so colour never carries the signal alone."
        className="block"
      >
        <div className="max-w-xl">
          <ColorRailCard
            accent="#f97316"
            footer={
              <>
                <Button size="sm" variant="outline">
                  Recategorise
                </Button>
                <Button size="sm">
                  Open
                  <ArrowUpRight />
                </Button>
              </>
            }
          >
            <div className="space-y-2 px-6 py-5 sm:px-7">
              <span className="text-muted-foreground text-[11px] font-medium tracking-wider uppercase">
                Transaction · Food & Drink
              </span>
              <div className="flex items-baseline justify-between gap-4">
                <h3 className="text-foreground text-xl font-semibold tracking-tight">
                  Blue Bottle Coffee
                </h3>
                <span className="text-foreground text-xl font-semibold tabular-nums">
                  −$6.25
                </span>
              </div>
              <p className="text-muted-foreground text-sm">
                Posted May 14, 2026 · Chase ····2890
              </p>
            </div>
          </ColorRailCard>
        </div>
      </Specimen>

      <Specimen
        label="StatusPanel"
        code="components/status-panel"
        description="Inline tone-tinted status block — 3px left rail + tinted icon tile + heading + body. Same 'colour encodes meaning' principle as `ColorRailCard`, sized for in-page notices (env-locked panels, already-set-up confirmation, sync warnings). Four tones: success / destructive / warning / info."
        className="block"
      >
        <div className="grid w-full gap-3 md:grid-cols-2">
          <StatusPanel
            tone="success"
            icon={CheckCircle2}
            heading="Encryption key configured"
            body="Tokens for new connections will be encrypted at rest."
          />
          <StatusPanel
            tone="warning"
            icon={AlertTriangle}
            heading="Plaid sync took longer than usual"
            body="Two accounts returned partial data — check the connection details."
          />
          <StatusPanel
            tone="destructive"
            icon={ShieldAlert}
            heading="Setup token is invalid"
            body="Generate a fresh one from the CLI and try again."
          />
          <StatusPanel
            tone="info"
            icon={Info}
            heading="Provider locked by environment"
            body="The PLAID_ENV variable is set — override it in the env to unlock."
          />
        </div>
      </Specimen>

      <Specimen
        label="IdPill"
        code="components/id-pill"
        description="Machine identifier (short_id, slug, URL fragment) rendered as a muted monospace pill — reads as 'this is a stable reference, not display copy'. Six surfaces share it; don't fork the look."
      >
        <IdPill value="tx_aB12cD34" />
        <IdPill value="acct_3rR9pq01" />
        <IdPill value="food_and_drink_coffee" />
        <IdPill value="/api/v1/transactions" />
      </Specimen>

      <Specimen
        label="FormFooter"
        code="components/form-footer"
        description="The flush bordered action strip at the bottom of a `<SectionCard>` that wraps a form. Sticks Cancel left, primary right; optional `hint` slot for an inline validation note. Drop inside a `SectionCard` with default body padding — negative margins line the strip up with the card's outer border."
        className="block"
      >
        <div className="max-w-md">
          <SectionCard title="Edit tag">
            <div className="grid gap-1.5">
              <Label htmlFor="sb-form-name">Display name</Label>
              <Input id="sb-form-name" defaultValue="Reimbursable" />
            </div>
            <FormFooter
              hint="Slug is generated automatically from the name."
              secondary={
                <Button variant="outline" size="sm">
                  Cancel
                </Button>
              }
              primary={
                <Button size="sm">
                  <Save /> Save changes
                </Button>
              }
            />
          </SectionCard>
        </div>
      </Specimen>

      <Specimen
        label="ConfirmDialog"
        code="components/confirm-dialog"
        description="Canonical confirmation surface for any destructive / irreversible action. Tone-tinted icon tile, built-in `pending` state (spinner + locked Cancel) so the dialog stays open on slow mutations. Tones: `destructive` (default) and `default`."
      >
        <Button
          variant="destructive"
          onClick={() => {
            setConfirmPending(false);
            setConfirmOpen(true);
          }}
        >
          <Trash2 /> Open confirm
        </Button>
        <ConfirmDialog
          open={confirmOpen}
          onOpenChange={setConfirmOpen}
          tone="destructive"
          icon={Trash2}
          title="Delete this tag?"
          description="The tag will be removed from every transaction it's attached to. This can't be undone."
          confirmLabel="Delete tag"
          pendingLabel="Deleting…"
          pending={confirmPending}
          onConfirm={() => {
            setConfirmPending(true);
            window.setTimeout(() => {
              setConfirmPending(false);
              setConfirmOpen(false);
              toast.success("Tag deleted.");
            }, 900);
          }}
        />
      </Specimen>

      <Specimen
        label="AuthShell"
        code="components/auth-shell"
        description="Two-pane brand + form shell used by Login and Setup. It's a whole-screen primitive — view it live on the unauthenticated routes rather than scaled into this gallery."
      >
        <div className="text-muted-foreground flex flex-wrap items-center gap-3 text-sm">
          <KeyRound className="size-4" />
          <span>
            See it in context at{" "}
            <a
              href="/v2/login"
              className="text-foreground underline-offset-4 hover:underline"
            >
              /v2/login
            </a>{" "}
            ·{" "}
            <a
              href="/v2/setup-account"
              className="text-foreground underline-offset-4 hover:underline"
            >
              /v2/setup-account
            </a>
          </span>
        </div>
      </Specimen>

      <Specimen
        label="EmptyState"
        code="components/empty-state"
        description="Three variants share one primitive — pick the weight that fits the surface. Same icon-tile vocabulary as the rest of v2 (square rounded-xl, not circle)."
        className="block"
      >
        <div className="grid gap-4 md:grid-cols-3">
          <div className="rounded-lg border">
            <div className="text-muted-foreground border-b px-3 py-2 text-[11px] tracking-wide uppercase">
              default · inside a container
            </div>
            <EmptyState
              icon={Inbox}
              title="No matching transactions"
              description="Try adjusting or clearing your filters."
              action={<Button variant="outline">Clear filters</Button>}
            />
          </div>
          <div>
            <div className="text-muted-foreground mb-2 px-1 text-[11px] tracking-wide uppercase">
              card · in raw page space
            </div>
            <EmptyState
              variant="card"
              icon={Users}
              title="No family members yet"
              description="Add members to connect their banks and attribute transactions by person."
            />
          </div>
          <div className="bg-card rounded-lg border p-4">
            <div className="text-muted-foreground mb-1 text-[11px] tracking-wide uppercase">
              inline · compact sub-panel
            </div>
            <EmptyState
              variant="inline"
              icon={RefreshCw}
              title="No sync history yet"
              description="Each sync will appear here with timing and result."
            />
          </div>
        </div>
      </Specimen>

      <Specimen
        label="TimelineRail"
        code="components/timeline-rail"
        description="Vertical activity feed primitive: a thin border-l rail anchors a stack of rows; each row's icon disc punches through the line. Group labels sit outside the rail as anchors. Used by the transaction-detail activity feed; queued for rule run history and per-connection sync logs."
        className="block"
      >
        <div className="max-w-md">
          <TimelineRail>
            <TimelineRail.Group label="Today">
              <TimelineRail.Row icon={MessageSquare}>
                <p className="text-sm leading-snug">
                  You left a comment
                </p>
                <p className="text-muted-foreground bg-muted/50 mt-1.5 rounded-md px-2.5 py-1.5 text-sm whitespace-pre-wrap">
                  Recategorise after the refund clears.
                </p>
                <p className="text-muted-foreground mt-1 text-[11px]">
                  2 minutes ago
                </p>
              </TimelineRail.Row>
              <TimelineRail.Row icon={Wand2}>
                <p className="text-sm leading-snug">
                  Rule "Coffee shops" applied — category set to Dining out
                </p>
                <p className="text-muted-foreground mt-1 text-[11px]">
                  18 minutes ago
                </p>
              </TimelineRail.Row>
            </TimelineRail.Group>
            <TimelineRail.Group label="Yesterday">
              <TimelineRail.Row icon={Shapes}>
                <p className="text-sm leading-snug">
                  Ricardo changed category to Groceries
                </p>
                <p className="text-muted-foreground mt-1 text-[11px]">
                  Yesterday at 4:12 PM
                </p>
              </TimelineRail.Row>
              <TimelineRail.Row icon={Tag}>
                <p className="text-sm leading-snug">
                  Ricardo added tag "reimbursable"
                </p>
                <p className="text-muted-foreground mt-1 text-[11px]">
                  Yesterday at 4:11 PM
                </p>
              </TimelineRail.Row>
              <TimelineRail.Row icon={MessageSquare} muted>
                <p className="text-sm leading-snug">
                  Ricardo deleted a comment
                </p>
                <p className="text-muted-foreground mt-1 text-[11px]">
                  Yesterday at 3:55 PM
                </p>
              </TimelineRail.Row>
            </TimelineRail.Group>
          </TimelineRail>
        </div>
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
        label="IconPicker · ColorPicker"
        code="components/icon-picker, color-picker"
        description="Form-friendly pickers used by the Categories and Tags forms. Each is pure-presentational; caller owns the value. Icons are searchable across the full Lucide catalog with a curated 'popular' default; colors offer a preset palette plus free-form hex."
      >
        <IconPicker
          value={iconValue}
          onChange={setIconValue}
          tint={colorValue}
        />
        <ColorPicker value={colorValue} onChange={setColorValue} />
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
