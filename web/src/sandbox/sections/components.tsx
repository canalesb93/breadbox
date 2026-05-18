import { useState } from "react";
import type { ColumnDef } from "@tanstack/react-table";
import { toast } from "sonner";
import {
  AlertCircle,
  AlertTriangle,
  ArrowUpRight,
  Check,
  CheckCircle2,
  Eye,
  EyeOff,
  Inbox,
  Info,
  Keyboard,
  KeyRound,
  Landmark,
  Link2,
  Lock,
  MessageSquare,
  Monitor,
  Moon,
  Pause,
  Pencil,
  Plus,
  Receipt,
  RefreshCw,
  RotateCcw,
  Save,
  Search,
  Shapes,
  ShieldAlert,
  Sparkles,
  Sun,
  Tag,
  Trash2,
  Unplug,
  UserPlus,
  Users,
  Wallet,
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
import { DetailList } from "@/components/detail-list";
import { ListCard } from "@/components/list-card";
import {
  ColorRailCard,
  ColorRailCardSkeleton,
} from "@/components/color-rail-card";
import { SectionCard } from "@/components/section-card";
import { IdPill } from "@/components/id-pill";
import { Eyebrow } from "@/components/eyebrow";
import { ActionPill } from "@/components/action-pill";
import { ComingSoonPill } from "@/components/coming-soon-pill";
import { JumpToPill, JumpToRow } from "@/components/jump-to-pill";
import { RowActionsMenu } from "@/components/row-actions-menu";
import {
  DropdownMenuItem,
  DropdownMenuSeparator,
} from "@/components/ui/dropdown-menu";
import { MetaBadge } from "@/components/meta-badge";
import { ListRowSkeleton } from "@/components/list-row-skeleton";
import { PageError } from "@/components/page-error";
import { ErrorPage } from "@/routes/error";
import { DetailPageSkeleton } from "@/components/detail-page-skeleton";
import { DetailSheetHeader } from "@/components/detail-sheet-header";
import { DetailDialogHeader } from "@/components/detail-dialog-header";
import { Sheet } from "@/components/ui/sheet";
import { Dialog } from "@/components/ui/dialog";
import { Badge } from "@/components/ui/badge";
import { SoftBackButton } from "@/components/soft-back-button";
import { StatusPanel } from "@/components/status-panel";
import { FormFooter } from "@/components/form-footer";
import { SettingsSectionHeader } from "@/components/settings-section-header";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { DangerZone } from "@/components/danger-zone";
import { PaginationBar } from "@/components/pagination-bar";
import { ViewAllPill } from "@/components/view-all-pill";
import { SearchInput } from "@/components/search-input";
import { HeroGrid } from "@/components/hero-grid";
import { ProviderCardHeader } from "@/components/provider-card-header";
import { ProviderPicker } from "@/features/connections/provider-picker";
import { useTheme } from "next-themes";
import { cn } from "@/lib/utils";
import type { Transaction } from "@/api/types";
import { SandboxSection, Specimen } from "@/sandbox/kit";
import {
  sampleTags,
  sampleTranscriptEvents,
  sampleTranscriptEventsError,
  sampleTransactions,
} from "@/sandbox/fixtures";
import { CronField } from "@/features/agents/cron-field";
import { TranscriptViewer } from "@/features/agents/transcript-viewer";

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
  const [dangerPending, setDangerPending] = useState(false);
  const [paginationPage, setPaginationPage] = useState(3);
  const [pageErrorRetrying, setPageErrorRetrying] = useState(false);

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
              <ViewAllPill to="/" align="footer">
                View all
              </ViewAllPill>
            }
          >
            <p className="text-muted-foreground text-sm leading-relaxed">
              Reach for <code>SectionCard</code> for any non-list section. The
              footer slot renders a flush bordered strip — typically a{" "}
              <code>&lt;ViewAllPill&gt;</code> link to a fuller index.
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
            action={<ViewAllPill to="/">View all</ViewAllPill>}
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
        label="HeroGrid"
        code="components/hero-grid"
        description="Body grid that sits one level inside `<ColorRailCard>` — arranges the identity column on the left and the metric column on the right. Promoted from three byte-identical sites (account, category, connection detail) plus a near-identical TX-detail variant. Stacks rows on mobile, docks the metric column to the right on lg. The transaction-detail variant tightens the lg row-gap via `lgGapClassName='lg:gap-x-10 lg:gap-y-5'` because the left column stacks identity on top of classify rows."
        className="block"
      >
        <div className="max-w-xl">
          <ColorRailCard accent="#0ea5e9">
            <HeroGrid>
              <div className="min-w-0 space-y-1">
                <span className="text-muted-foreground text-[11px] font-medium tracking-wider uppercase">
                  Asset
                </span>
                <h3 className="text-foreground text-xl font-semibold tracking-tight">
                  Chase Sapphire
                </h3>
                <p className="text-muted-foreground text-xs">
                  Credit card · ····2890
                </p>
              </div>
              <div className="flex flex-col items-start gap-1.5 lg:items-end lg:text-right">
                <span className="bg-success/10 text-success inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-medium tracking-wide uppercase">
                  Balance
                </span>
                <div className="text-3xl font-semibold tabular-nums sm:text-4xl">
                  $4,210.55
                </div>
                <p className="text-muted-foreground text-[11px] tabular-nums">
                  $1,789.45 available
                </p>
              </div>
            </HeroGrid>
          </ColorRailCard>
        </div>
      </Specimen>

      <Specimen
        label="ProviderCardHeader"
        code="components/provider-card-header"
        description="Canonical header body inside a provider settings card — sits one level inside `<ColorRailCard>` on the Plaid, Teller, and CSV pages. Identity column on the left (tone-tinted `size-11 rounded-lg` tile + 'Provider' eyebrow + title + optional status badge + capped description) docks beside an optional `trailing` slot (typically `<ProviderScoreboard>`). Stacks on mobile, aligns on a single baseline at ≥640px. Promoted from three near-byte-identical header blocks."
        className="block"
      >
        <div className="max-w-3xl space-y-4">
          <ColorRailCard accent="#22c55e">
            <ProviderCardHeader
              icon={<Landmark className="size-5" />}
              iconClassName="bg-blue-500/10 text-blue-600 dark:text-blue-400"
              title="Plaid"
              description="Connect 12,000+ US banks via Plaid Link. Webhook-driven incremental sync."
              badge={
                <Badge
                  variant="outline"
                  className="border-success/30 bg-success/10 text-success"
                >
                  Configured
                </Badge>
              }
              trailing={
                <div className="flex flex-col items-start gap-1 sm:items-end">
                  <span className="text-muted-foreground text-[10px] font-medium tracking-wider uppercase">
                    Last sync
                  </span>
                  <span className="text-foreground text-sm tabular-nums">
                    2m ago
                  </span>
                </div>
              }
            />
          </ColorRailCard>
          <ColorRailCard accent="#f59e0b">
            <ProviderCardHeader
              icon={<Receipt className="size-5" />}
              iconClassName="bg-amber-500/10 text-amber-600 dark:text-amber-400"
              title="CSV import"
              description="Drop in transactions exported from any bank — no API credentials required."
            />
          </ColorRailCard>
        </div>
      </Specimen>

      <Specimen
        label="ColorRailCardSkeleton"
        code="components/color-rail-card"
        description="Loading mirror of `<ColorRailCard>` — same shell + rail, with the stable identity column (tile + eyebrow + title + meta) and trailing metric column already baked in. `tileShape` picks `rounded-md` (transactions) vs `rounded-lg` (accounts, categories) to match the loaded tile. `withFooter` toggles the bordered action strip; `body` slots in any extra hero row (e.g. TX-detail's secondary details grid)."
        className="block"
      >
        <div className="max-w-xl space-y-4">
          <ColorRailCardSkeleton tileShape="rounded-md" />
          <ColorRailCardSkeleton tileShape="rounded-lg" withFooter />
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
        label="MetaBadge"
        code="components/meta-badge"
        description="Tiny meta chip used in list rows and detail-page hero columns to label a row's secondary state — Hidden, Excluded, Linked, Re-auth, System, Paused, Primary, Dependent, Error, Disconnected, Disabled, … Owns the v2 density vocabulary (`text-[10px]` + `gap-1` + `px-1.5 py-0` + `[&>svg]:size-2.5`) so the same chip never gets re-derived. Tone routes through the underlying `<Badge>` variant — `outline` by default because a meta label is intentionally calmer than the row's primary classification. `muted` opts into the `text-muted-foreground font-normal` shading the categories list uses so 'System' / 'Hidden' don't compete with the category name. For tone-specific chips (the amber Re-auth pill) pass `className` — the density tokens still apply, which is the whole point. Seven surfaces share it: accounts list (statusBadge for error/disconnected, Linked pill, iter 104), accounts detail, account-links section (Primary/Dependent/Disabled, iter 104), categories list (parent + child rows), category detail, connection detail."
      >
        <MetaBadge icon={Lock} muted>
          System
        </MetaBadge>
        <MetaBadge icon={EyeOff} muted>
          Hidden
        </MetaBadge>
        <MetaBadge icon={EyeOff}>Excluded</MetaBadge>
        <MetaBadge icon={Link2} variant="secondary">
          Linked
        </MetaBadge>
        <MetaBadge icon={Pause} variant="secondary">
          Paused
        </MetaBadge>
        <MetaBadge variant="default">Primary</MetaBadge>
        <MetaBadge variant="secondary">Dependent</MetaBadge>
        <MetaBadge variant="destructive">Error</MetaBadge>
        <MetaBadge>Disconnected</MetaBadge>
        <MetaBadge>Disabled</MetaBadge>
        <MetaBadge
          icon={AlertTriangle}
          className="border-amber-500/40 bg-amber-500/5 text-amber-700 dark:text-amber-400"
        >
          Re-auth
        </MetaBadge>
      </Specimen>

      <Specimen
        label="DetailList"
        code="components/detail-list"
        description="The canonical label / value KV block used by every v2 detail-page Details sidebar (transaction, account, connection, category). Stack two or three inside a `<SectionCard bodyClassName='space-y-5 px-5 py-5 text-sm'>` host — uppercase tracked group label, `<dl>` for screen-reader semantics, label-left / value-right with `break-words` so long values wrap inside the column on 375px viewports. `compactDetailRows` keeps callsites declarative — pass nullable rows inline, the helper filters anything without a value. Mono rows route through `<IdPill>`. Don't fork — extend this primitive."
        className="block"
      >
        <div className="max-w-sm rounded-xl border">
          <div className="space-y-5 px-5 py-5 text-sm">
            <DetailList
              label="Account"
              rows={[
                { label: "Name", value: "Plaid Checking" },
                { label: "Member", value: "Ricardo (hosted-link test)" },
                { label: "Currency", value: "USD" },
              ]}
            />
            <DetailList
              label="Provider"
              rows={[
                { label: "Authorized", value: "May 9, 2026" },
                {
                  label: "Category",
                  value: "Transfer In Other Transfer In",
                },
              ]}
            />
            <DetailList
              label="Reference"
              rows={[{ label: "ID", value: "JKHqiBWP", mono: true }]}
            />
          </div>
        </div>
      </Specimen>

      <Specimen
        label="Eyebrow"
        code="components/eyebrow"
        description="The canonical uppercase micro-label used across detail-page hero columns, section headers, 'Jump to' pills, timeline-rail day headings, AND sidebar/menu group labels. Open-coded across ten files in five subtly different sizes before iter 37 consolidated them; iter 95 added the `nav` cousin to retire the remaining three `font-semibold` sidebar-group-label drift sites (`nav-main`, `settings-shell` desktop sidebar, `shortcut-sheet` group headers). Four variants — `default` (`text-[10px] tracking-[0.1em]`) is the everyday eyebrow; `hero` (`tracking-[0.12em]`) gets extra letter air for spots where it sits directly under a large display title; `page` (`text-[11px] tracking-[0.08em]`) is the slightly heavier rhythm reserved for *page*-scale framing (PageHeader eyebrow, home-stats KPI labels, provider-card 'Provider' caption); `nav` (`font-semibold text-[10px] tracking-[0.08em]`) is the heavier cousin used inside sidebar / menu chrome where the label sits against a coloured surface and needs the extra weight to read. Don't reach for raw `text-[10-11px] font-medium/semibold tracking-* uppercase` markup — extend this primitive."
        className="block"
      >
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <div className="rounded-lg border p-4">
            <Eyebrow>Default · in a card header</Eyebrow>
            <p className="text-foreground mt-1 text-sm">
              "Showing 24 of 879" / "Reference"
            </p>
            <p className="text-muted-foreground mt-1 text-xs">
              text-[10px] · tracking-[0.1em]
            </p>
          </div>
          <div className="bg-card rounded-lg border p-4">
            <Eyebrow variant="hero" as="p">
              Liability
            </Eyebrow>
            <h4 className="text-foreground mt-1 text-xl font-semibold tracking-tight">
              Chase Sapphire ····2890
            </h4>
            <p className="text-muted-foreground mt-1 text-xs">
              hero · tracking-[0.12em]
            </p>
          </div>
          <div className="rounded-lg border p-4">
            <Eyebrow variant="page" as="p">
              Synced 4 minutes ago
            </Eyebrow>
            <h1 className="text-foreground mt-1.5 text-2xl font-semibold tracking-tight">
              Connections
            </h1>
            <p className="text-muted-foreground mt-1 text-xs">
              page · text-[11px] · tracking-[0.08em]
            </p>
          </div>
          <div className="bg-sidebar text-sidebar-foreground rounded-lg border p-4">
            <Eyebrow
              variant="nav"
              as="p"
              className="text-muted-foreground/80"
            >
              Workspace
            </Eyebrow>
            <ul className="mt-2 space-y-0.5 text-sm">
              <li className="rounded-md px-2 py-1">Home</li>
              <li className="rounded-md px-2 py-1">Transactions</li>
              <li className="rounded-md px-2 py-1">Categories</li>
            </ul>
            <p className="text-muted-foreground mt-2 text-xs">
              nav · font-semibold · text-[10px] · tracking-[0.08em]
            </p>
          </div>
        </div>
      </Specimen>

      <Specimen
        label="JumpToPill"
        code="components/jump-to-pill"
        description="The canonical 'Jump to' pill cluster used in every detail-page hero (transaction, account, category, connection). `JumpToPill` is a 28px-tall outline button (`h-7 px-2.5 text-xs` with a `size-3` leading icon) — taller than `Button size=xs` (24px toolbar pill) and shorter than `Button size=sm` (32px action). Reads as a labelled lateral link from the hero, not a CTA. `JumpToRow` wraps the pills with the canonical `Eyebrow` 'Jump to' label. Don't open-code the className triplet — extend this primitive."
        className="block"
      >
        <div className="rounded-lg border p-4">
          <JumpToRow>
            <JumpToPill>
              <Search className="size-3" />
              Similar transactions
            </JumpToPill>
            <JumpToPill>
              <Wallet className="size-3" />
              Chase Sapphire ····2890
            </JumpToPill>
            <JumpToPill>
              <Receipt className="size-3" />
              Dining out
            </JumpToPill>
          </JumpToRow>
        </div>
      </Specimen>

      <Specimen
        label="ActionPill"
        code="components/action-pill"
        description="The canonical small action button used inside `<ColorRailCard footer>` strips (account-detail / connection-detail) and `<StatusPanel trailing>` slots. 28px-tall pill (`h-7`), `text-xs` label, `gap-1.5` between leading icon and label, `size-3.5` leading icon. Same height as `<JumpToPill>` but the action is a dispatched handler (`onClick`, or wraps a `<Link>` via `asChild`) rather than a lateral nav. Tone is governed by `variant`: `ghost` for action strips inside a card surface, `outline` for top-of-page `<StatusPanel trailing>` CTAs where the pill needs more visual weight. Don't fork the className triplet — extend this primitive."
        className="block"
      >
        <div className="flex flex-col gap-3 rounded-lg border p-4">
          <div className="flex flex-wrap items-center gap-2">
            <ActionPill onClick={() => toast.message("Sync started")}>
              <RefreshCw className="size-3.5" />
              Sync now
            </ActionPill>
            <ActionPill onClick={() => toast.message("Pause toggled")}>
              <Pause className="size-3.5" />
              Pause
            </ActionPill>
            <ActionPill onClick={() => toast.message("View transactions")}>
              <Eye className="size-3.5" />
              View transactions
            </ActionPill>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <ActionPill
              variant="outline"
              onClick={() => toast.message("Re-authenticate")}
            >
              <RefreshCw className="size-3.5" />
              Re-authenticate
            </ActionPill>
          </div>
        </div>
      </Specimen>

      <Specimen
        label="ComingSoonPill"
        code="components/coming-soon-pill"
        description="The canonical muted 'Coming soon' status pill rendered in the trailing slot of a `<StatusPanel tone='info'>` for unbuilt surfaces. Fully-rounded pill (`rounded-full`) with muted background + muted-foreground label, `px-2.5 py-1 text-[11px]` uppercase + wide tracking, leading `Clock` icon at `size-3`. Two consumers today: `routes/placeholder.tsx` (the unbuilt-nav-leaf shell, iter 21) and `components/settings-shell.tsx` (the in-the-works settings panel, iter 78) — both previously hand-rolled the 10-class span. Distinct from `<Eyebrow>` (no chip; section/page micro-label) and shadcn `<Badge>` (rectangular, semantic-tone variants). Pass a different `icon` to swap the leading glyph without losing the rhythm; pass `children` to override the label. Promoted in iter 102."
        className="block"
      >
        <div className="flex flex-col gap-3 rounded-lg border p-4">
          <div className="flex flex-wrap items-center gap-2">
            <ComingSoonPill />
            <ComingSoonPill icon={Sparkles}>Beta</ComingSoonPill>
            <ComingSoonPill icon={Wand2}>In review</ComingSoonPill>
          </div>
          <p className="text-muted-foreground text-xs">
            Default leading icon is <code>Clock</code>. Override with the
            <code> icon</code> prop and the label via <code>children</code>.
          </p>
        </div>
      </Specimen>

      <Specimen
        label="RowActionsMenu"
        code="components/row-actions-menu"
        description="The canonical row-actions kebab — `Tooltip` + `DropdownMenuTrigger` + `Button` lockup that every list row, hero footer, and inline action cluster shares so trigger geometry, icon glyph (`MoreHorizontal size-4`), and aria vocabulary stay consistent across the SPA. `size='sm'` (default) is the dominant size-8 ghost square (connection-row, household-section, tags-table, api-keys-table). `size='xs'` is the tighter size-7 variant for hero footers and nested list rows (account-links, rule-row, connection-detail hero). `loading` swaps the icon for a spinning Loader2 and disables the trigger. `triggerClassName` is the escape hatch (only connection-detail's hero footer uses it today for `rounded-full` to pair with surrounding pills). Sibling of `<ActionPill>` (labelled inline action) — same `text-muted-foreground → hover:text-foreground` icon-button vocabulary, but RowActionsMenu opens a menu rather than dispatching."
        className="block"
      >
        <div className="flex flex-col gap-4 rounded-lg border p-4">
          <div className="flex items-center justify-between rounded-md border bg-card px-3 py-2">
            <div className="text-sm">
              <div className="font-medium">Tag actions (size sm)</div>
              <div className="text-muted-foreground text-xs">
                Dominant row-actions trigger — size-8 ghost square
              </div>
            </div>
            <RowActionsMenu label="Tag actions">
              <DropdownMenuItem onSelect={() => toast.message("Edit tag")}>
                <Pencil className="size-4" /> Edit
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                variant="destructive"
                onSelect={() => toast.error("Delete tag")}
              >
                <Trash2 className="size-4" /> Delete
              </DropdownMenuItem>
            </RowActionsMenu>
          </div>
          <div className="flex items-center justify-between rounded-md border bg-card px-3 py-2">
            <div className="text-sm">
              <div className="font-medium">Rule actions (size xs)</div>
              <div className="text-muted-foreground text-xs">
                Tighter variant for hero footers + nested rows — size-7 ghost square
              </div>
            </div>
            <RowActionsMenu
              label="Rule actions"
              size="xs"
              contentClassName="w-44"
            >
              <DropdownMenuItem onSelect={() => toast.message("Disable rule")}>
                <Pause className="size-3.5" /> Disable
              </DropdownMenuItem>
              <DropdownMenuItem
                variant="destructive"
                onSelect={() => toast.error("Delete rule")}
              >
                <Trash2 className="size-3.5" /> Delete
              </DropdownMenuItem>
            </RowActionsMenu>
          </div>
          <div className="flex items-center justify-between rounded-md border bg-card px-3 py-2">
            <div className="text-sm">
              <div className="font-medium">Link actions (loading)</div>
              <div className="text-muted-foreground text-xs">
                `loading` swaps the kebab icon for a spinning Loader2
              </div>
            </div>
            <RowActionsMenu label="Link actions" size="xs" loading>
              <DropdownMenuItem>
                <Unplug className="size-3.5" /> Unlink
              </DropdownMenuItem>
            </RowActionsMenu>
          </div>
        </div>
      </Specimen>

      <Specimen
        label="ViewAllPill"
        code="components/view-all-pill"
        description="The canonical 'card-header / footer goto' pill used when a bordered surface defers to a fuller list page. 28px-tall ghost link (`h-7 px-2 text-xs` muted → foreground on hover) with a trailing `<ArrowRight className='size-3' />` icon — matches the `<JumpToPill>` size-3 leading-icon vocabulary so the two lateral-link primitives speak the same icon language. `align='header'` (default) supplies the `-mr-2` flush shoulder for use in a `<ListCard action>` / `<SectionCard action>` slot; `align='footer'` drops the shoulder for footer slots. Distinct from `<ActionPill>` (real handler, `size-3.5` icon) and from `<Button size='sm'>` (32px CTA). Live across Home recent activity, Home connections, Account-detail recent transactions, Connection-detail sync history."
        className="block"
      >
        <div className="flex flex-wrap items-center gap-6 rounded-lg border p-4">
          <ViewAllPill to="/">View all</ViewAllPill>
          <ViewAllPill to="/">Manage</ViewAllPill>
          <ViewAllPill to="/">See all transactions</ViewAllPill>
        </div>
      </Specimen>

      <Specimen
        label="SearchInput"
        code="components/search-input"
        description="The canonical text input with a leading magnifier glyph used by every v2 list page (Tags, Categories, API keys, Transactions). Wraps the stock `<Input>` primitive with a `pointer-events-none` `Search` glyph absolutely positioned at `left-2.5` and `pl-8` on the field to clear it. Forwards every native input prop (value, onChange, onKeyDown, placeholder, …) and a `ref` for callers that need to focus from a keyboard shortcut. `containerClassName` is the escape hatch for the outer wrapper width (default `w-full max-w-sm`; API keys uses `w-full max-w-xs`; Transactions toolbar uses `w-full min-w-48 sm:w-64`). Don't open-code the icon + Input pair — extend this primitive."
        className="block"
      >
        <div className="space-y-4 rounded-lg border p-4">
          <div className="space-y-1.5">
            <div className="text-muted-foreground text-[11px] tracking-wide uppercase">
              Default · `w-full max-w-sm`
            </div>
            <SearchInput
              defaultValue=""
              placeholder="Search by name, slug, or description…"
            />
          </div>
          <div className="space-y-1.5">
            <div className="text-muted-foreground text-[11px] tracking-wide uppercase">
              Narrow · `containerClassName='w-full max-w-xs'`
            </div>
            <SearchInput
              containerClassName="w-full max-w-xs"
              defaultValue=""
              placeholder="Search by name, prefix, actor…"
            />
          </div>
          <div className="space-y-1.5">
            <div className="text-muted-foreground text-[11px] tracking-wide uppercase">
              Toolbar · `containerClassName='w-full min-w-48 sm:w-64'`
            </div>
            <SearchInput
              containerClassName="w-full min-w-48 sm:w-64"
              defaultValue="coffee"
              placeholder="Search merchant or description…"
            />
          </div>
        </div>
      </Specimen>

      <Specimen
        label="ListRowSkeleton"
        code="components/list-row-skeleton"
        description="The canonical loading-row shape used by every v2 list — Home recent activity, Home connections, Connections, Accounts, Categories. Tokens encode the real row's rhythm: `density` (compact / regular / comfortable), `leading` (sm/md/lg-square matching CategoryIconTile sizes), `trailing` (none / badge / value-stack). Pick the tokens that match the real row so the skeleton doesn't shift on data arrival. Don't fork — extend the primitive with new tokens if no combination fits."
        className="block"
      >
        <div className="grid gap-4 lg:grid-cols-3">
          <div className="overflow-hidden rounded-lg border">
            <div className="text-muted-foreground border-b px-3 py-2 text-[11px] tracking-wide uppercase">
              regular · sm-square · value-stack
            </div>
            <div className="divide-y">
              <ListRowSkeleton
                density="regular"
                leading="sm-square"
                trailing="value-stack"
                titleClassName="w-36"
                subtitleClassName="w-24"
              />
              <ListRowSkeleton
                density="regular"
                leading="sm-square"
                trailing="value-stack"
                titleClassName="w-40"
                subtitleClassName="w-20"
              />
              <ListRowSkeleton
                density="regular"
                leading="sm-square"
                trailing="value-stack"
                titleClassName="w-32"
                subtitleClassName="w-28"
              />
            </div>
          </div>
          <div className="overflow-hidden rounded-lg border">
            <div className="text-muted-foreground border-b px-3 py-2 text-[11px] tracking-wide uppercase">
              comfortable · lg-square · value-stack
            </div>
            <div className="divide-y">
              <ListRowSkeleton
                density="comfortable"
                leading="lg-square"
                trailing="value-stack"
                titleClassName="w-44"
                subtitleClassName="w-24"
              />
              <ListRowSkeleton
                density="comfortable"
                leading="lg-square"
                trailing="value-stack"
                titleClassName="w-36"
                subtitleClassName="w-20"
              />
            </div>
          </div>
          <div className="overflow-hidden rounded-lg border">
            <div className="text-muted-foreground border-b px-3 py-2 text-[11px] tracking-wide uppercase">
              compact · md-square · badge
            </div>
            <div className="divide-y">
              <ListRowSkeleton
                density="compact"
                leading="md-square"
                trailing="badge"
                titleClassName="w-32"
                subtitleClassName="w-20"
                trailingTopClassName="w-14"
              />
              <ListRowSkeleton
                density="compact"
                leading="md-square"
                trailing="badge"
                titleClassName="w-40"
                subtitleClassName="w-24"
                trailingTopClassName="w-12"
              />
              <ListRowSkeleton
                density="compact"
                leading="md-square"
                trailing="none"
                titleClassName="w-28"
                lines={1}
              />
            </div>
          </div>
        </div>
      </Specimen>

      <Specimen
        label="DetailSheetHeader"
        code="components/detail-sheet-header"
        description="The canonical icon-tile header lockup for every v2 Sheet — leading rounded-lg icon tile + optional uppercase eyebrow + title + description + optional trailing slot. Two densities: `default` (size-9 tile, p-5, ambient overlays like Shortcut sheet) and `accent` (size-10 tile + bg-muted/20 + p-6, primary flows like Connect-bank). Mirrors the StatusPanel / EmptyState / SectionCard icon-tile vocabulary so every Sheet reads as part of the v2 system. Wrapped in a hidden `<Sheet open>` here so the radix Dialog context is available for SheetTitle/SheetDescription — live consumers carry the surrounding `<SheetContent>` chrome."
        className="block"
      >
        <div className="grid gap-4 lg:grid-cols-2">
          <div className="overflow-hidden rounded-lg border bg-card">
            <div className="text-muted-foreground border-b bg-muted/30 px-3 py-2 text-[11px] tracking-wide uppercase">
              default · size-9 tile · p-5
            </div>
            <Sheet open onOpenChange={() => {}}>
              <DetailSheetHeader
                icon={Keyboard}
                title="Keyboard shortcuts"
                description="Available across the app. Shortcuts pause while you're typing in an input."
              />
            </Sheet>
          </div>
          <div className="overflow-hidden rounded-lg border bg-card">
            <div className="text-muted-foreground border-b bg-muted/30 px-3 py-2 text-[11px] tracking-wide uppercase">
              accent · size-10 tile · p-6 · with eyebrow + trailing
            </div>
            <Sheet open onOpenChange={() => {}}>
              <DetailSheetHeader
                icon={Landmark}
                eyebrow="New connection"
                title="Connect a bank"
                description="Pick a provider to link an institution and start syncing transactions."
                density="accent"
                trailing={<Badge variant="secondary">Plaid</Badge>}
              />
            </Sheet>
          </div>
        </div>
      </Specimen>

      <Specimen
        label="DetailDialogHeader"
        code="components/detail-dialog-header"
        description="Sibling of `<DetailSheetHeader>` for centered Dialogs that host a form or multi-step payload (Add member, Create login, Share setup link). Same leading rounded-lg icon tile + optional eyebrow + title + description + optional trailing slot — just routed through `<DialogTitle>` / `<DialogDescription>` so it composes with shadcn `<Dialog>` instead of `<Sheet>`. Promoted in iter 101 to retire the four open-coded `<DialogHeader>` lockups in household-section onto a shared primitive. Wrapped in a hidden `<Dialog open>` here so the radix Dialog context is available."
        className="block"
      >
        <div className="grid gap-4 lg:grid-cols-2">
          <div className="overflow-hidden rounded-lg border bg-card p-4">
            <div className="text-muted-foreground mb-3 text-[11px] tracking-wide uppercase">
              add-member · title + description
            </div>
            <Dialog open onOpenChange={() => {}}>
              <DetailDialogHeader
                icon={UserPlus}
                title="Add a household member"
                description="New members are added without a login by default. Invite them to sign in to share read or edit access."
              />
            </Dialog>
          </div>
          <div className="overflow-hidden rounded-lg border bg-card p-4">
            <div className="text-muted-foreground mb-3 text-[11px] tracking-wide uppercase">
              share-link · with eyebrow + trailing
            </div>
            <Dialog open onOpenChange={() => {}}>
              <DetailDialogHeader
                icon={Link2}
                eyebrow="Household"
                title="Share their setup link"
                description="Alex can use this one-time link to set their password. It expires in 7 days."
                trailing={<Badge variant="secondary">7d</Badge>}
              />
            </Dialog>
          </div>
        </div>
      </Specimen>

      <Specimen
        label="FormFooter"
        code="components/form-footer"
        description="The flush bordered action strip at the bottom of a form container. Cancel sits left, primary right; optional `hint` slot for an inline validation note. Two insets — `card` (default) flushes to a `<SectionCard>` body (px-5 py-5), `sheet` flushes to a `<Sheet>` body (p-6) and uses `mt-auto` so it sticks to the Sheet bottom. Iter 49 folded the CSV form's bespoke footer onto the `sheet` variant."
        className="block"
      >
        <div className="grid gap-4 lg:grid-cols-2">
          <div>
            <div className="text-muted-foreground mb-2 text-[11px] tracking-wide uppercase">
              inset · card (inside SectionCard)
            </div>
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
          <div>
            <div className="text-muted-foreground mb-2 text-[11px] tracking-wide uppercase">
              inset · sheet (inside a Sheet body)
            </div>
            <div className="bg-card flex min-h-[200px] flex-col rounded-lg border p-6">
              <p className="text-muted-foreground text-sm">
                Mock Sheet body — the footer below flushes to the surrounding{" "}
                <code className="bg-muted/60 rounded px-1 font-mono text-[11px]">
                  p-6
                </code>{" "}
                edges and uses <code className="bg-muted/60 rounded px-1 font-mono text-[11px]">mt-auto</code>{" "}
                so it sticks to the bottom of the Sheet.
              </p>
              <FormFooter
                inset="sheet"
                hint={
                  <span className="text-muted-foreground text-xs">
                    Ready to import{" "}
                    <span className="text-foreground tabular-nums font-medium">
                      241
                    </span>{" "}
                    rows.
                  </span>
                }
                secondary={
                  <Button variant="ghost" size="sm">
                    Different file
                  </Button>
                }
                primary={<Button size="sm">Import 241 rows</Button>}
              />
            </div>
          </div>
        </div>
      </Specimen>

      <Specimen
        label="SettingsSectionHeader"
        code="components/settings-section-header"
        description="The canonical title + description block used by every section inside the Settings shell. Top-of-pane titles (Account / Household / Backups) and inline sub-sections (Change password / Actions / Stored backups / Automatic schedule) both route through here, so the typographic rhythm — heading size + weight, description colour + line-height, action alignment — stays in one place. Tokens: `section` (h2, text-lg, font-semibold) and `sub` (h3, text-sm, font-semibold). Iter 107 ripple from the iter 106 card-header polish: both heads now share `font-semibold` weight + `items-center` action alignment with `SectionCard` / `ListCard` / `PageHeader`, so an Add-member CTA sits flush with the title midline instead of bottom-docking under the description."
        className="block"
      >
        <div className="grid gap-4 lg:grid-cols-2">
          <div className="rounded-lg border p-5">
            <div className="text-muted-foreground mb-3 text-[11px] tracking-wide uppercase">
              section · h2 · with action
            </div>
            <SettingsSectionHeader
              title="Household"
              description="Add family members to track everyone's accounts in one place. Each member can be invited to sign in with their own login."
              action={
                <Button size="sm">
                  <Plus /> Add member
                </Button>
              }
            />
          </div>
          <div className="rounded-lg border p-5">
            <div className="text-muted-foreground mb-3 text-[11px] tracking-wide uppercase">
              sub · h3 · description only
            </div>
            <SettingsSectionHeader
              level="sub"
              title="Automatic schedule"
              description="Backups older than the retention window are pruned at the end of each scheduled run."
            />
          </div>
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
        label="DangerZone"
        code="components/danger-zone"
        description="Inline destructive-confirm pattern used by detail-page delete actions (tag-detail, category-detail). A `border-destructive/40` Card hosts the prompt + outline trigger; the trigger expands in place into a tinted confirm block — no modal churn for delete flows. Pair with `withMutationToast` to surface the success / error message after the mutation resolves. Use `<ConfirmDialog>` instead when the destructive action lives on a list row or inside another dialog (no surrounding card surface available)."
        className="block"
      >
        <div className="max-w-xl">
          <DangerZone
            description="The tag will be removed from every transaction it's attached to. Activity history is preserved. This can't be undone."
            confirmTarget={<span className="font-semibold">Reimbursable</span>}
            actionLabel="Delete tag"
            isPending={dangerPending}
            onConfirm={async () => {
              setDangerPending(true);
              await new Promise((resolve) => window.setTimeout(resolve, 900));
              setDangerPending(false);
              toast.success("Tag deleted.");
            }}
          />
        </div>
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
        label="ThemeToggle"
        code="next-themes · ThemeProvider"
        description="The theme switcher mounted at the React root via `<ThemeProvider>` in main.tsx. The picker keys off the user's stored choice (`system`/`light`/`dark`), not the realised mode, so 'System' stays selected when the OS resolves to dark. The same hook drives the Sonner Toaster's theme prop and the NavUser dropdown's Theme submenu — change it in one place, every surface follows."
        className="block"
      >
        <ThemeSpecimen />
      </Specimen>

      <Specimen
        label="NavUser footer"
        code="components/nav-user"
        description="The bottom-of-sidebar account row + dropdown that hosts the theme switcher, keyboard-shortcut overlay, classic-UI link, and sign-out. Lives inside the SidebarProvider — view it live in any app route. The role pill picks the same primary tint as the BrandHeader's V2 chip for admins, muted neutral for editor/viewer."
      >
        <div className="text-muted-foreground flex flex-wrap items-center gap-3 text-sm">
          <UserChipDemo />
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
        label="PageError"
        code="components/page-error"
        description="The canonical page-level 'this page couldn't fetch its data' state. Two variants. `panel` (default) is a centred destructive-toned hero card — dashed `destructive/25` border, `bg-destructive/[0.02]` wash, `size-11` destructive icon tile, `Couldn't load {resource}` heading, the error's `message` (or a fallback) as body, and a `RefreshCw` Retry button beneath that swaps to a spinning icon + `Retrying…` label while in-flight. Shaped like `<EmptyState variant='card'>` but tone-tinted destructive so the surface reads as a failure state, not just an empty one. `inline` drops the panel chrome so the same icon tile + heading + body + retry sits flush inside an already-bordered host (a SectionCard body, a ListCard slot) — used by the transaction-detail activity-timeline. Sibling of `<EmptyState>` (no-data) and `<DetailPageSkeleton>` (loading) — three states, three vocabularies, one visual system. Don't fork — extend this primitive."
        className="block"
      >
        <div className="space-y-6">
          <div className="grid gap-4 md:grid-cols-2">
            <div>
              <div className="text-muted-foreground mb-2 px-1 text-[11px] tracking-wide uppercase">
                panel · with retry handler
              </div>
              <PageError
                resource="accounts"
                error={new Error("Network request failed (ECONNRESET).")}
                onRetry={() => {
                  setPageErrorRetrying(true);
                  window.setTimeout(() => setPageErrorRetrying(false), 1200);
                }}
                retrying={pageErrorRetrying}
              />
            </div>
            <div>
              <div className="text-muted-foreground mb-2 px-1 text-[11px] tracking-wide uppercase">
                panel · without retry · fallback message
              </div>
              <PageError resource="rules" />
            </div>
          </div>
          <div>
            <div className="text-muted-foreground mb-2 px-1 text-[11px] tracking-wide uppercase">
              inline · nested inside a bordered host (SectionCard / ListCard)
            </div>
            <div className="bg-card rounded-lg border">
              <div className="border-b px-5 py-3 text-sm font-medium">
                Activity
              </div>
              <div className="p-5">
                <PageError
                  variant="inline"
                  resource="the activity timeline"
                  error={new Error("Request failed with status 500.")}
                  onRetry={() => {
                    setPageErrorRetrying(true);
                    window.setTimeout(() => setPageErrorRetrying(false), 1200);
                  }}
                  retrying={pageErrorRetrying}
                />
              </div>
            </div>
          </div>
        </div>
      </Specimen>

      <Specimen
        label="ErrorPage"
        code="routes/error"
        description="The route-level error-boundary surface — wired into `createRouter({ defaultErrorComponent })`, so it renders in place of `<Outlet/>` whenever a route throws during render. Single hero card (not PageHeader + StatusPanel + SectionCard) with a top destructive accent stripe, a `size-14` destructive icon tile, `500 · Server error` eyebrow, `Something went wrong` H1, framing copy, the raw error message in a monospace `<code>` block, and a wrap-friendly action cluster: Try again (primary, when the router passes `reset`), Reload page, Jump to (⌘K), Home. The stack trace lives behind a `<Collapsible>` 'Technical details' disclosure at the foot of the card so it stays one click away without dominating the surface for end users. The sidebar, topbar, and command palette stay live behind it so the user always has a way out without a hard reload."
        className="block"
      >
        <ErrorPage
          error={
            new Error(
              "Failed to fetch /api/v1/transactions: Network request failed (ECONNRESET).",
            )
          }
          reset={() => {
            /* sandbox no-op */
          }}
        />
      </Specimen>

      <Specimen
        label="DetailPageSkeleton"
        code="components/detail-page-skeleton"
        description="The canonical page-level loading shell for every v2 detail page (transaction, account, category, connection). Composes the iter-10 `<ColorRailCardSkeleton>` hero + a `<JumpToRow>`-shaped pill strip + a 2-column grid of `rounded-xl` block placeholders matching `<SectionCard>` / `<ListCard>` chrome. Sibling of `<PageError>` (error) and `<EmptyState>` (empty) — three states, three vocabularies, one visual system. `hero` forwards `tileShape` / `withFooter` / `body` to `<ColorRailCardSkeleton>`; `jumpPills` counts the lateral-nav pill row (`0` to omit); `main` / `sidebar` are arrays of Tailwind height classes for the stacked block placeholders. Pass an empty `sidebar` to collapse the grid to a single column. Don't fork — extend this primitive."
        className="block"
      >
        <div className="grid gap-4 lg:grid-cols-2">
          <div className="overflow-hidden rounded-lg border bg-card">
            <div className="text-muted-foreground border-b bg-muted/30 px-3 py-2 text-[11px] tracking-wide uppercase">
              transaction · hero rounded-md · 3 pills · 1+2
            </div>
            <div className="p-4">
              <DetailPageSkeleton
                hero={{ tileShape: "rounded-md" }}
                jumpPills={3}
                main={["h-64"]}
                sidebar={["h-32", "h-40"]}
              />
            </div>
          </div>
          <div className="overflow-hidden rounded-lg border bg-card">
            <div className="text-muted-foreground border-b bg-muted/30 px-3 py-2 text-[11px] tracking-wide uppercase">
              connection · hero withFooter · no pills · 3+0
            </div>
            <div className="p-4">
              <DetailPageSkeleton
                hero={{ tileShape: "rounded-lg", withFooter: true }}
                jumpPills={0}
                main={["h-32", "h-48", "h-56"]}
              />
            </div>
          </div>
        </div>
      </Specimen>

      <Specimen
        label="TimelineRail"
        code="components/timeline-rail"
        description="Vertical activity feed primitive: a thin border-l rail anchors a stack of rows; each row's icon disc punches through the line. Group labels render as temporal dividers — a small dot anchored on the rail's x-axis + uppercase eyebrow + hairline rule extending right — so they read as separators *inside* the timeline, distinct from the surrounding section header. `<TimelineRail.RowSkeleton>` (iter 65) mirrors the row geometry exactly so loading-to-loaded transitions don't shift layout. Each row accepts a semantic `tone` (`neutral` · `primary` · `success` · `warning` · `destructive` · `info` · `muted`, iter 93; `destructive` added iter 105 for sync-history errored runs) that tints the disc border + icon so the eye can pick out rule fires, classification changes, and sync events without parsing the summary line. Used by the transaction-detail activity feed and the per-connection sync-history feed."
        className="block"
      >
        <div className="grid gap-6 max-w-3xl sm:grid-cols-2">
          <div>
            <div className="text-muted-foreground mb-3 text-[11px] uppercase tracking-[0.08em]">
              Loaded
            </div>
            <TimelineRail>
            <TimelineRail.Group label="Today">
              <TimelineRail.Row icon={MessageSquare} tone="neutral">
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
              <TimelineRail.Row icon={Wand2} tone="primary">
                <p className="text-sm leading-snug">
                  Rule "Coffee shops" applied — category set to Dining out
                </p>
                <p className="text-muted-foreground mt-1 text-[11px]">
                  18 minutes ago
                </p>
              </TimelineRail.Row>
              <TimelineRail.Row icon={RefreshCw} tone="info">
                <p className="text-sm leading-snug">
                  Sync from Chase updated this transaction
                </p>
                <p className="text-muted-foreground mt-1 text-[11px]">
                  42 minutes ago
                </p>
              </TimelineRail.Row>
            </TimelineRail.Group>
            <TimelineRail.Group label="Yesterday">
              <TimelineRail.Row icon={Shapes} tone="primary">
                <p className="text-sm leading-snug">
                  Ricardo changed category to Groceries
                </p>
                <p className="text-muted-foreground mt-1 text-[11px]">
                  Yesterday at 4:12 PM
                </p>
              </TimelineRail.Row>
              <TimelineRail.Row icon={Tag} tone="success">
                <p className="text-sm leading-snug">
                  Ricardo added tag "reimbursable"
                </p>
                <p className="text-muted-foreground mt-1 text-[11px]">
                  Yesterday at 4:11 PM
                </p>
              </TimelineRail.Row>
              <TimelineRail.Row icon={Tag} tone="warning">
                <p className="text-sm leading-snug">
                  Ricardo removed tag "needs-review"
                </p>
                <p className="text-muted-foreground mt-1 text-[11px]">
                  Yesterday at 4:00 PM
                </p>
              </TimelineRail.Row>
              <TimelineRail.Row icon={AlertCircle} tone="destructive">
                <p className="text-sm leading-snug">
                  Sync from Chase failed — credentials expired
                </p>
                <p className="text-muted-foreground mt-1 text-[11px]">
                  Yesterday at 3:58 PM
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
          <div>
            <div className="text-muted-foreground mb-3 text-[11px] uppercase tracking-[0.08em]">
              Loading
            </div>
            <TimelineRail>
              <TimelineRail.Group>
                <TimelineRail.RowSkeleton />
                <TimelineRail.RowSkeleton body />
                <TimelineRail.RowSkeleton />
                <TimelineRail.RowSkeleton />
              </TimelineRail.Group>
            </TimelineRail>
          </div>
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
        description="Single rendering of a category — rounded-rect, color-tinted. The ring marks a manual override; em-dash when uncategorized. Two sizes share one recipe with TagChip — sm (h-5 / 11px) for dense list cells, md (h-6 / 12px) for hero / pickers / sandbox."
        className="block"
      >
        <div className="space-y-2">
          <p className="text-muted-foreground text-[10px] font-medium tracking-[0.1em] uppercase">
            md (default)
          </p>
          <div className="flex flex-wrap items-center gap-2">
            <CategoryBadge category={coffeeCategory} />
            <CategoryBadge category={gasCategory} />
            <CategoryBadge category={coffeeCategory} overridden />
            <CategoryBadge category={null} />
          </div>
        </div>
        <div className="mt-4 space-y-2">
          <p className="text-muted-foreground text-[10px] font-medium tracking-[0.1em] uppercase">
            sm — dense list / table cells
          </p>
          <div className="flex flex-wrap items-center gap-2">
            <CategoryBadge category={coffeeCategory} size="sm" />
            <CategoryBadge category={gasCategory} size="sm" />
            <CategoryBadge category={coffeeCategory} size="sm" overridden />
            <CategoryBadge category={null} size="sm" />
          </div>
        </div>
      </Specimen>

      <Specimen
        label="TagChip · TagList"
        code="components/tag-chip"
        description="Pill-shaped (the shape category badges deliberately avoid). Color-tinted icon + label; optional remove (×) for editable contexts. TagList resolves slugs against the tag catalog and caps with a +N overflow. Same sm / md sizes as CategoryBadge — pass through TagList to keep dense rows aligned."
        className="block"
      >
        <div className="space-y-2">
          <p className="text-muted-foreground text-[10px] font-medium tracking-[0.1em] uppercase">
            md (default)
          </p>
          <div className="flex flex-wrap items-center gap-2">
            <TagChip tag={sampleTags[0]} />
            <TagChip tag={sampleTags[1]} />
            <TagChip
              tag={sampleTags[2]}
              onRemove={() => toast.message("Removed Subscription")}
            />
          </div>
        </div>
        <div className="mt-4 space-y-2">
          <p className="text-muted-foreground text-[10px] font-medium tracking-[0.1em] uppercase">
            sm — dense list / table cells
          </p>
          <div className="flex flex-wrap items-center gap-2">
            <TagChip tag={sampleTags[0]} size="sm" />
            <TagChip tag={sampleTags[1]} size="sm" />
            <TagChip
              tag={sampleTags[2]}
              size="sm"
              onRemove={() => toast.message("Removed Subscription")}
            />
          </div>
        </div>
        <div className="mt-4 space-y-2">
          <p className="text-muted-foreground text-[10px] font-medium tracking-[0.1em] uppercase">
            TagList · md + sm
          </p>
          <div className="space-y-2">
            <TagList
              slugs={["needs-review", "business", "subscription", "reimbursable"]}
              max={2}
            />
            <TagList
              slugs={["needs-review", "business", "subscription", "reimbursable"]}
              max={2}
              size="sm"
            />
          </div>
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

      <Specimen
        label="PaginationBar"
        code="components/pagination-bar"
        description="Caller-driven page selector that wraps the shadcn `Pagination` primitive. Owns the page-window math (≤7 pages shows every number; beyond that a 5-page window slides around the current page with leading / trailing ellipses) and the muted `Page N of M · count itemLabel` caption. Set `isFetching` to dim controls while a page is in flight — discourages double-clicks without locking the surface. Used by the Transactions list (`features/transactions/transactions-pagination`), Rules, Category detail, and Tag detail."
        className="block"
      >
        <div className="space-y-4">
          <PaginationBar
            page={paginationPage}
            pageSize={50}
            total={879}
            onPageChange={setPaginationPage}
            itemLabel="transactions"
          />
          <p className="text-muted-foreground text-center text-xs">
            Click a page to update the window. Ellipses appear once the total
            exceeds 7 pages.
          </p>
        </div>
      </Specimen>

      <Specimen
        label="CronField"
        code="features/agents/cron-field"
        description="Cron input + live human-readable preview + popover preset picker. Used in the agent edit form (web/src/routes/agents.$slug.edit.tsx). The preview shows 'Mondays at 9 AM' style labels for known patterns, falls back to 'Custom: <expr>' otherwise."
        className="block"
      >
        <CronFieldDemo />
      </Specimen>

      <Specimen
        label="TranscriptViewer — successful run"
        code="features/agents/transcript-viewer"
        description="Parses NDJSON sidecar events into turn-grouped assistant blocks with collapsible tool calls + a result footer (cost, tokens, turn count, stop reason). Used inside the run-history Sheet at /v2/agents/$slug/runs."
        className="block"
      >
        <div className="bg-background h-[480px] overflow-y-auto rounded-md border p-3">
          <TranscriptViewer
            events={sampleTranscriptEvents}
            rawLength={sampleTranscriptEvents.length}
            truncated={false}
            shortId="abc12345"
          />
        </div>
      </Specimen>

      <Specimen
        label="TranscriptViewer — errored run"
        code="features/agents/transcript-viewer"
        description="The error banner variant — surfaces an SDK/sidecar error at the top with a destructive-tone Alert."
        className="block"
      >
        <div className="bg-background overflow-y-auto rounded-md border p-3">
          <TranscriptViewer
            events={sampleTranscriptEventsError}
            rawLength={sampleTranscriptEventsError.length}
            truncated={false}
            shortId="err00000"
          />
        </div>
      </Specimen>
    </SandboxSection>
  );
}

// Live theme picker — three pill buttons backed by next-themes. Renders the
// same `<Check>` affordance the NavUser submenu uses for the selected row, so
// the two surfaces share their visual vocabulary.
function ThemeSpecimen() {
  const { theme, setTheme, resolvedTheme } = useTheme();
  const choices = [
    { value: "system", label: "System", icon: Monitor },
    { value: "light", label: "Light", icon: Sun },
    { value: "dark", label: "Dark", icon: Moon },
  ] as const;
  const current = theme ?? "system";
  return (
    <div className="space-y-3">
      <div className="bg-muted/30 inline-flex items-center gap-1 rounded-md border p-1">
        {choices.map(({ value, label, icon: Icon }) => {
          const active = current === value;
          return (
            <button
              key={value}
              type="button"
              onClick={() => setTheme(value)}
              className={cn(
                "inline-flex h-8 items-center gap-1.5 rounded-sm px-2.5 text-xs font-medium transition-colors",
                active
                  ? "bg-background text-foreground shadow-sm"
                  : "text-muted-foreground hover:text-foreground",
              )}
            >
              <Icon className="size-3.5" />
              {label}
              {active ? (
                <Check className="text-primary -mr-0.5 ml-1 size-3" />
              ) : null}
            </button>
          );
        })}
      </div>
      <p className="text-muted-foreground text-xs">
        Stored: <span className="text-foreground font-mono">{current}</span> ·
        Resolved:{" "}
        <span className="text-foreground font-mono">
          {resolvedTheme ?? "—"}
        </span>{" "}
        · Storage key{" "}
        <span className="text-foreground font-mono">breadbox:theme</span>
      </p>
    </div>
  );
}

// CronField needs local state to render usefully — the demo wraps it in a
// stateful container so users can click presets and watch the preview update.
function CronFieldDemo() {
  const [value, setValue] = useState("0 9 * * 1");
  return <CronField value={value} onChange={setValue} />;
}

// Inline visual demo of the NavUser trigger — the dropdown itself depends on
// `SidebarProvider`, so we render just the trigger shape (avatar + name + role
// pill + chevron) here and point readers at the live sidebar for the menu.
function UserChipDemo() {
  return (
    <div className="bg-sidebar/40 ring-sidebar-border flex w-full max-w-sm items-center gap-2.5 rounded-md p-2 ring-1">
      <span className="bg-primary/10 text-primary ring-border/60 inline-flex size-8 items-center justify-center rounded-md text-[11px] font-semibold ring-1">
        AD
      </span>
      <div className="grid flex-1 leading-tight">
        <span className="text-sm font-medium">admin</span>
        <span className="text-muted-foreground inline-flex items-center gap-1.5 text-[11px]">
          <span className="bg-primary/15 text-primary inline-flex items-center gap-1 rounded-sm px-1.5 py-px text-[10px] font-semibold tracking-wider uppercase">
            admin
          </span>
          @example.com
        </span>
      </div>
      <span className="text-muted-foreground/70 text-[10px]">
        click in sidebar →
      </span>
    </div>
  );
}
