import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { z } from "zod";
import { toast } from "sonner";
import {
  ArrowLeft,
  Check,
  ChevronDown,
  ChevronRight,
  ChevronsDownUp,
  ChevronsUpDown,
  Copy,
  Eye,
  GripVertical,
  Plus,
  Search,
  Trash2,
  Undo2,
  Wand2,
} from "lucide-react";
import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import {
  restrictToParentElement,
  restrictToVerticalAxis,
} from "@dnd-kit/modifiers";
import {
  SortableContext,
  arrayMove,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { Input } from "@/components/ui/input";
import {
  Table,
  TableBody,
  TableCell,
  TableRow,
} from "@/components/ui/table";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  ButtonGroup,
  ButtonGroupSeparator,
} from "@/components/ui/button-group";
import { PageHeader } from "@/components/page-header";
import { PageError } from "@/components/page-error";
import { FloatingActionBar } from "@/components/floating-action-bar";
import { DynamicIcon } from "@/lib/icon";
import { useShortcut } from "@/lib/shortcuts";
import {
  usePromptBlocks,
  type PromptBlock,
  type PromptBlockGroup,
} from "@/api/queries/prompt-blocks";
import { useAgents, useUpdateAgent } from "@/api/queries/agents";
import { withMutationToast } from "@/lib/mutation-toast";
import { cn } from "@/lib/utils";

// URL-stateful so a configuration is shareable. Edits to individual
// block content live in-memory only — the URL stays compact, and a
// fresh visit re-reads from the embedded library.
export const promptsBuildSearchSchema = z.object({
  strategy: z.string().optional(),
  depth: z.string().optional(),
  integrations: z.string().optional(),
  knowledge: z.string().optional(),
});
type PromptsBuildSearch = z.infer<typeof promptsBuildSearchSchema>;

interface CustomBlock {
  id: string; // `custom:<incrementing>` — stable within a session
  content: string;
}

// deriveCustomBlockTitle picks a display label for a custom block from
// its content. First non-empty line, with a leading `# ` stripped if
// present (so a markdown heading reads as the title). Truncated for
// the row header; the full content is always visible when expanded.
function deriveCustomBlockTitle(content: string): string {
  const firstLine = content
    .split("\n")
    .map((l) => l.trim())
    .find((l) => l.length > 0);
  if (!firstLine) return "Custom block";
  const stripped = firstLine.startsWith("# ")
    ? firstLine.slice(2).trim()
    : firstLine;
  return stripped.length > 60 ? stripped.slice(0, 60) + "…" : stripped;
}

type ComposedItem =
  | { kind: "library"; id: string }
  | { kind: "custom"; id: string };

function parseCsv(s: string | undefined): string[] {
  if (!s) return [];
  return s
    .split(",")
    .map((p) => p.trim())
    .filter((p) => p.length > 0);
}

function joinCsv(parts: string[]): string | undefined {
  return parts.length === 0 ? undefined : parts.join(",");
}

function rowKey(item: ComposedItem): string {
  return `${item.kind}:${item.id}`;
}

// Preset is a curated starting-point composition — one click replaces
// the current selection with this preset's library blocks (in order).
// Carried over from the v1 admin agent wizard's "agent types"
// (`internal/prompts/config.go`): same labels, same block lists. Each
// preset is Core + Default from the v1 config; Optional blocks are
// left out so the preset is opinionated, not maximal.
interface Preset {
  id: string;
  label: string;
  description: string;
  icon: string;
  blockIds: string[];
}

const PRESETS: Preset[] = [
  {
    id: "initial-setup",
    label: "Initial Setup",
    description:
      "First-time bulk categorization after connecting a new account.",
    icon: "sparkles",
    blockIds: ["strategy-initial-setup", "review-depth-efficient"],
  },
  {
    id: "bulk-review",
    label: "Bulk Review",
    description: "Thorough review of a large pending queue.",
    icon: "list-checks",
    blockIds: ["strategy-bulk-review", "review-depth-thorough"],
  },
  {
    id: "quick-review",
    label: "Quick Review",
    description: "Rapidly clear a large queue with batch operations.",
    icon: "zap",
    blockIds: ["strategy-quick-review", "review-depth-efficient"],
  },
  {
    id: "routine-review",
    label: "Routine Review",
    description: "Daily or weekly review of recent transactions.",
    icon: "calendar-check",
    blockIds: [
      "strategy-routine-review",
      "review-depth-thorough",
      "transaction-comments",
    ],
  },
  {
    id: "spending-report",
    label: "Spending Report",
    description: "Weekly or monthly spending summary with trends.",
    icon: "chart-no-axes-column",
    blockIds: [
      "strategy-spending-report",
      "category-system",
      "merchant-analysis",
    ],
  },
  {
    id: "anomaly-detection",
    label: "Anomaly Detection",
    description: "Monitor for unusual charges, duplicates, spending spikes.",
    icon: "siren",
    blockIds: [
      "strategy-anomaly-detection",
      "merchant-analysis",
      "account-linking",
    ],
  },
];


export function PromptsBuildPage() {
  const search = useSearch({ strict: false }) as PromptsBuildSearch;
  const navigate = useNavigate();
  const blocksQuery = usePromptBlocks();

  const strategy = search.strategy ?? "";
  const depth = search.depth ?? "";
  const integrations = useMemo(() => parseCsv(search.integrations), [search.integrations]);
  const knowledge = useMemo(() => parseCsv(search.knowledge), [search.knowledge]);

  const blocks = blocksQuery.data ?? [];
  const blocksById = useMemo(() => {
    const map = new Map<string, PromptBlock>();
    for (const b of blocks) map.set(b.id, b);
    return map;
  }, [blocks]);

  const blocksByGroup = useMemo(() => {
    const g: Record<string, PromptBlock[]> = {
      strategy: [],
      depth: [],
      integration: [],
      knowledge: [],
    };
    for (const b of blocks) {
      if (!g[b.group]) g[b.group] = [];
      g[b.group].push(b);
    }
    return g;
  }, [blocks]);

  const [edits, setEdits] = useState<Record<string, string>>({});
  const [customs, setCustoms] = useState<CustomBlock[]>([]);
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  // Modal lives on the parent so it stays mounted across the
  // empty-state → composer-table swap that happens after the first
  // pick. If the dialog were nested inside whichever trigger element
  // is visible, the first selection would unmount the trigger and
  // dismiss the dialog with it.
  const [addBlockOpen, setAddBlockOpen] = useState(false);

  // Default order: strategy first, then depth, then integrations (URL
  // order), then knowledge (URL order), then customs.
  const defaultOrder: ComposedItem[] = useMemo(() => {
    const items: ComposedItem[] = [];
    if (strategy) items.push({ kind: "library", id: strategy });
    if (depth) items.push({ kind: "library", id: depth });
    for (const id of integrations) items.push({ kind: "library", id });
    for (const id of knowledge) items.push({ kind: "library", id });
    for (const c of customs) items.push({ kind: "custom", id: c.id });
    return items;
  }, [strategy, depth, integrations, knowledge, customs]);

  const [order, setOrder] = useState<ComposedItem[]>(defaultOrder);

  // Resync the session order whenever the URL selection drifts.
  // Preserve manual reordering for items that still belong; append
  // new ones at the end.
  useEffect(() => {
    const have = new Set(order.map(rowKey));
    const want = new Set(defaultOrder.map(rowKey));
    if (
      have.size !== want.size ||
      [...have].some((k) => !want.has(k)) ||
      [...want].some((k) => !have.has(k))
    ) {
      const surviving = order.filter((i) => want.has(rowKey(i)));
      const added = defaultOrder.filter((i) => !have.has(rowKey(i)));
      setOrder([...surviving, ...added]);
    }
  }, [defaultOrder, order]);

  const effectiveOrder = order;

  const setFilter = (patch: Partial<PromptsBuildSearch>) => {
    navigate({
      to: ".",
      search: (prev: Record<string, unknown>) => {
        const next: Record<string, unknown> = { ...prev, ...patch };
        for (const k of Object.keys(next)) {
          if (next[k] === undefined || next[k] === "") delete next[k];
        }
        return next;
      },
    });
  };

  // pickBlock toggles a block into or out of the composition. Strategy
  // and Depth are single-select (re-picking removes); Integrations and
  // Knowledge are multi-select. Toggle-off everywhere keeps modal picks
  // reversible without needing the row's trash button.
  const pickBlock = (block: PromptBlock) => {
    const id = block.id;
    switch (block.group) {
      case "strategy":
        setFilter({ strategy: strategy === id ? undefined : id });
        return;
      case "depth":
        setFilter({ depth: depth === id ? undefined : id });
        return;
      case "integration": {
        const next = integrations.includes(id)
          ? integrations.filter((x) => x !== id)
          : [...integrations, id];
        setFilter({ integrations: joinCsv(next) });
        return;
      }
      case "knowledge": {
        const next = knowledge.includes(id)
          ? knowledge.filter((x) => x !== id)
          : [...knowledge, id];
        setFilter({ knowledge: joinCsv(next) });
        return;
      }
    }
  };

  const removeFromComposition = (item: ComposedItem) => {
    // Prune any expansion/edit entries the row left behind so the maps
    // don't grow unbounded across add/remove churn in a session.
    setExpanded((e) => {
      if (!(item.id in e)) return e;
      const next = { ...e };
      delete next[item.id];
      return next;
    });
    setEdits((s) => {
      if (!(item.id in s)) return s;
      const next = { ...s };
      delete next[item.id];
      return next;
    });
    if (item.kind === "custom") {
      setCustoms((cs) => cs.filter((c) => c.id !== item.id));
      return;
    }
    if (item.id === strategy) setFilter({ strategy: undefined });
    else if (item.id === depth) setFilter({ depth: undefined });
    else if (integrations.includes(item.id))
      setFilter({ integrations: joinCsv(integrations.filter((x) => x !== item.id)) });
    else if (knowledge.includes(item.id))
      setFilter({ knowledge: joinCsv(knowledge.filter((x) => x !== item.id)) });
  };

  // pendingFocusId carries the id of a freshly-created empty block
  // across the render that materializes its DOM. The matching effect
  // below scrolls to it and focuses its textarea, then clears the ref.
  // Stored as a ref (not state) so the effect doesn't re-trigger on
  // unrelated re-renders.
  const pendingFocusId = useRef<string | null>(null);

  // addCustomBlock takes the initial content directly (no title field
  // — title is derived from content at render time). The modal picker
  // collects content in its custom-form step before calling this.
  const addCustomBlock = (content: string) => {
    const id = `custom:${Date.now()}`;
    setCustoms((cs) => [...cs, { id, content }]);
    // Expand the new block by default ONLY if it has no content yet —
    // a content-prefilled add (the common path now) starts collapsed
    // since the user has already seen what they wrote.
    if (!content.trim()) {
      setExpanded((e) => ({ ...e, [id]: true }));
    }
  };

  // clearAll wipes every block out of the composition — both the
  // URL-tracked library selections and the in-memory custom blocks
  // plus their orphaned expansion/edit entries.
  const clearAll = () => {
    setExpanded({});
    setEdits({});
    setFilter({
      strategy: undefined,
      depth: undefined,
      integrations: undefined,
      knowledge: undefined,
    });
    setCustoms([]);
  };

  // applyPreset replaces the current composition with the preset's
  // block list. Each block ID is bucketed by its `group` (resolved via
  // the loaded library) into the matching URL filter slot. Unknown
  // IDs — e.g. a renamed prompt file — are silently dropped so a
  // stale preset never errors. Customs are cleared on the same beat
  // so applying a preset is a clean reset, not an additive op.
  const applyPreset = (preset: Preset) => {
    const next: Partial<PromptsBuildSearch> = {
      strategy: undefined,
      depth: undefined,
      integrations: undefined,
      knowledge: undefined,
    };
    const ints: string[] = [];
    const knows: string[] = [];
    for (const id of preset.blockIds) {
      const b = blocksById.get(id);
      if (!b) continue;
      switch (b.group) {
        case "strategy":
          next.strategy = id;
          break;
        case "depth":
          next.depth = id;
          break;
        case "integration":
          ints.push(id);
          break;
        case "knowledge":
          knows.push(id);
          break;
      }
    }
    next.integrations = joinCsv(ints);
    next.knowledge = joinCsv(knows);
    setFilter(next);
    setCustoms([]);
    setAddBlockOpen(false);
  };

  // addEmptyBlock appends an empty custom block to the composition and
  // hands focus to it: expanded, textarea scrolled into view, cursor
  // ready. Triggered both by the toolbar button and the "E" shortcut.
  const addEmptyBlock = () => {
    const id = `custom:${Date.now()}`;
    setCustoms((cs) => [...cs, { id, content: "" }]);
    setExpanded((e) => ({ ...e, [id]: true }));
    pendingFocusId.current = id;
  };

  // After a render that included a newly-added empty block, locate its
  // textarea by id, scroll it into the viewport, and focus it. Tied to
  // `customs` so it fires once per add — guard with the ref so we
  // don't re-focus on unrelated `customs` mutations (rename, delete).
  // requestAnimationFrame defers the focus past React's commit phase
  // so the row is laid out before scrollIntoView runs.
  useEffect(() => {
    const id = pendingFocusId.current;
    if (!id) return;
    // The render where the new item lands in `effectiveOrder` is one
    // commit AFTER the render where it lands in `customs` (the order
    // sync is itself an effect). Wait for the row to actually appear
    // in the DOM before clearing the ref.
    const ta = document.getElementById(`content-${id}`);
    if (!(ta instanceof HTMLTextAreaElement)) return;
    pendingFocusId.current = null;
    requestAnimationFrame(() => {
      ta.scrollIntoView({ behavior: "smooth", block: "center" });
      ta.focus();
    });
  }, [effectiveOrder, customs]);

  const composedText = useMemo(
    () => composePrompt(effectiveOrder, blocksById, edits, customs),
    [effectiveOrder, blocksById, edits, customs],
  );

  const isLoading = blocksQuery.isLoading;

  // Expand-all toggle: true only when every row in the composition is
  // currently expanded. Drives the "Collapse all" / "Expand all"
  // button label so the action is always the opposite of the current
  // state. An empty composition reports false so the button never
  // renders in that branch (gated by effectiveOrder.length > 0).
  const allExpanded = useMemo(() => {
    if (effectiveOrder.length === 0) return false;
    return effectiveOrder.every((i) => expanded[i.id]);
  }, [effectiveOrder, expanded]);

  const toggleExpandAll = () => {
    if (allExpanded) {
      setExpanded((e) => {
        const next = { ...e };
        for (const i of effectiveOrder) next[i.id] = false;
        return next;
      });
    } else {
      setExpanded((e) => {
        const next = { ...e };
        for (const i of effectiveOrder) next[i.id] = true;
        return next;
      });
    }
  };

  // Page-level shortcuts registered with the global shortcut registry
  // so they show up in the ⇧? shortcut sheet alongside the rest of the
  // app's bindings. useShortcut handles the input/dialog guard for us.
  useShortcut(["mod", "Enter"], () => setAddBlockOpen(true), {
    label: "Add block",
    group: "Prompt builder",
  });
  useShortcut(["shift", "mod", "Enter"], () => addEmptyBlock(), {
    label: "Add empty block",
    group: "Prompt builder",
  });

  // Active-set helper for the dropdown — shows a ✓ next to library
  // items already in the composition.
  const activeLibraryIds = useMemo(() => {
    const s = new Set<string>();
    if (strategy) s.add(strategy);
    if (depth) s.add(depth);
    for (const id of integrations) s.add(id);
    for (const id of knowledge) s.add(id);
    return s;
  }, [strategy, depth, integrations, knowledge]);

  return (
    <>
      <PageHeader
        eyebrow="System"
        title="Prompt builder"
        description="Compose an agent prompt from reusable blocks. Output is plain markdown — copy it anywhere, or push it directly into a Breadbox agent. Breadbox agent runs are optional and for convenience; the prompts work with any agent tool you connect to your Breadbox server."
      />

      {blocksQuery.isError ? (
        <PageError
          resource="prompt blocks"
          error={blocksQuery.error}
          onRetry={() => blocksQuery.refetch()}
          retrying={blocksQuery.isFetching}
        />
      ) : isLoading ? (
        <Card className="p-4">
          <Skeleton className="h-32 w-full" />
        </Card>
      ) : (
        <>
          <div className="flex items-center justify-end gap-1">
            {effectiveOrder.length > 0 && (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={toggleExpandAll}
                className="text-muted-foreground hover:text-foreground h-8 px-2 text-xs"
              >
                {allExpanded ? (
                  <>
                    <ChevronsDownUp className="size-3.5" />
                    Collapse all
                  </>
                ) : (
                  <>
                    <ChevronsUpDown className="size-3.5" />
                    Expand all
                  </>
                )}
              </Button>
            )}
            <ButtonGroup>
              <Button
                type="button"
                variant="default"
                size="sm"
                onClick={() => setAddBlockOpen(true)}
                className="h-8 px-3 text-xs"
              >
                <Plus className="size-3.5" />
                Add block
              </Button>
              {/* Visible hairline divider between the main action and
                  the overflow trigger — `bg-input` is too low-contrast
                  on the filled primary surface. */}
              <ButtonGroupSeparator className="bg-primary-foreground/25" />
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    type="button"
                    variant="default"
                    size="sm"
                    aria-label="More add-block options"
                    className="h-8 px-2"
                  >
                    <ChevronDown className="size-3.5" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent
                  align="end"
                  className="w-60"
                  // Radix would snap focus back to the chevron after
                  // close; preventDefault keeps focus on the new
                  // textarea our handler just focused.
                  onCloseAutoFocus={(e) => e.preventDefault()}
                >
                  <DropdownMenuItem onSelect={() => addEmptyBlock()}>
                    <Plus className="size-4" />
                    Add empty block
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </ButtonGroup>
          </div>
          <ComposerTable
            items={effectiveOrder}
            onReorder={setOrder}
            blocksById={blocksById}
            customs={customs}
            edits={edits}
            expanded={expanded}
            setExpanded={setExpanded}
            setEdits={setEdits}
            setCustoms={setCustoms}
            onRemove={removeFromComposition}
            addBlockEmptyTrigger={
              <Button
                variant="outline"
                size="sm"
                disabled={isLoading}
                onClick={() => setAddBlockOpen(true)}
              >
                <Plus className="size-4" />
                Add block
              </Button>
            }
          />
        </>
      )}
      {!isLoading && !blocksQuery.isError && (
        <AddBlockMenu
          open={addBlockOpen}
          onOpenChange={setAddBlockOpen}
          blocksByGroup={blocksByGroup}
          activeIds={activeLibraryIds}
          composedCount={effectiveOrder.length}
          onPick={pickBlock}
          onAddCustom={addCustomBlock}
          onClearAll={clearAll}
          onApplyPreset={applyPreset}
        />
      )}
      {effectiveOrder.length > 0 && (
        <FloatingActionBar
          ariaLabel="Prompt builder actions"
          className="pl-3"
        >
          <ComposedStats text={composedText} />
          <Separator orientation="vertical" className="mx-1 h-5" />
          <PreviewButton text={composedText} disabled={false} />
          <OutputActions text={composedText} disabled={false} />
        </FloatingActionBar>
      )}
    </>
  );
}

interface AddBlockMenuProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  blocksByGroup: Record<string, PromptBlock[]>;
  activeIds: Set<string>;
  // composedCount is the total number of blocks currently in the
  // composition (library + custom). Drives the footer count text and
  // the disabled state of the Clear all action. Distinct from
  // activeIds.size, which only counts library picks.
  composedCount: number;
  // onPick handles selection. Parent dispatches by group to the right
  // URL slot (single-select for strategy/depth, multi-select for
  // integrations/knowledge), so the modal stays group-agnostic.
  onPick: (block: PromptBlock) => void;
  onAddCustom: (content: string) => void;
  onClearAll: () => void;
  // onApplyPreset replaces the current composition with the preset's
  // ordered block IDs. The page-level handler resolves each ID to its
  // group via blocksById to decide which URL filter slot it lands in.
  onApplyPreset: (preset: Preset) => void;
}

// AddBlockMenu is the modal "block library" — left rail of category
// filters, right pane of card grid. Controlled by the parent so the
// dialog survives re-renders where the trigger element changes (e.g.
// empty-state card → composer table). When the dialog mounted its own
// trigger, the first pick caused the empty card to unmount, which
// dismissed the dialog mid-interaction.
function AddBlockMenu({
  open,
  onOpenChange,
  blocksByGroup,
  activeIds,
  composedCount,
  onPick,
  onAddCustom,
  onClearAll,
  onApplyPreset,
}: AddBlockMenuProps) {
  const [mode, setMode] = useState<"pick" | "custom">("pick");
  // "presets" is a virtual category — it doesn't filter the block grid,
  // it swaps the right pane for a preset-card view. Other values still
  // drive the block filter as before.
  const [activeGroup, setActiveGroup] = useState<
    PromptBlockGroup | "all" | "presets"
  >("all");
  const [query, setQuery] = useState("");
  const [customDraft, setCustomDraft] = useState("");

  // Reset all transient state when the dialog closes so the next open
  // starts on the picker with a clean search / draft. Open state
  // itself lives on the parent — we only forward the change and
  // piggyback on close events to reset.
  const handleOpenChange = (next: boolean) => {
    onOpenChange(next);
    if (!next) {
      setMode("pick");
      setActiveGroup("all");
      setQuery("");
      setCustomDraft("");
    }
  };

  const openCustomForm = () => {
    setMode("custom");
    setCustomDraft("");
  };

  const commitCustomBlock = () => {
    const content = customDraft.trim();
    if (!content) return;
    onAddCustom(content);
    handleOpenChange(false);
  };

  const allBlocks = useMemo(
    () => [
      ...(blocksByGroup.strategy ?? []),
      ...(blocksByGroup.depth ?? []),
      ...(blocksByGroup.integration ?? []),
      ...(blocksByGroup.knowledge ?? []),
    ],
    [blocksByGroup],
  );

  const counts: Record<PromptBlockGroup | "all", number> = {
    all: allBlocks.length,
    strategy: (blocksByGroup.strategy ?? []).length,
    depth: (blocksByGroup.depth ?? []).length,
    integration: (blocksByGroup.integration ?? []).length,
    knowledge: (blocksByGroup.knowledge ?? []).length,
  };

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return allBlocks.filter((b) => {
      if (activeGroup !== "all" && b.group !== activeGroup) return false;
      if (!q) return true;
      return (
        b.title.toLowerCase().includes(q) ||
        b.description.toLowerCase().includes(q) ||
        b.id.toLowerCase().includes(q)
      );
    });
  }, [allBlocks, activeGroup, query]);

  // When viewing "All", the right pane is split into one section per
  // group with a sticky header — so a long list reads as a structured
  // library rather than a flat grid. A specific-group filter keeps the
  // flat grid (only one section would be redundant).
  const sections: Array<{ group: PromptBlockGroup; label: string; blocks: PromptBlock[] }> =
    useMemo(() => {
      if (activeGroup !== "all") return [];
      const order: Array<{ group: PromptBlockGroup; label: string }> = [
        { group: "strategy", label: "Strategy" },
        { group: "depth", label: "Depth" },
        { group: "integration", label: "Integrations" },
        { group: "knowledge", label: "Knowledge" },
      ];
      return order
        .map(({ group, label }) => ({
          group,
          label,
          blocks: filtered.filter((b) => b.group === group),
        }))
        .filter((s) => s.blocks.length > 0);
    }, [activeGroup, filtered]);

  // Stay open after a pick: user might add several integrations / both
  // a strategy and a depth in one sitting. They dismiss via Esc /
  // backdrop / ×.

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="flex h-[640px] max-h-[85vh] flex-col gap-0 p-0 sm:max-w-3xl">
        {mode === "pick" ? (
          <>
            <DialogHeader className="border-b p-4">
              <DialogTitle>Add a block</DialogTitle>
              <DialogDescription className="sr-only">
                Pick a strategy, depth modifier, integration, or knowledge
                block to add to the prompt composition.
              </DialogDescription>
              <div className="relative mt-3">
                <Search className="text-muted-foreground absolute top-1/2 left-3 size-4 -translate-y-1/2" />
                <Input
                  autoFocus
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  placeholder="Search blocks…"
                  className="pl-9"
                />
              </div>
            </DialogHeader>

            <div className="grid flex-1 grid-cols-[10rem_1fr] overflow-hidden">
              <nav className="bg-muted/30 overflow-y-auto border-r p-2">
                <CategoryRailItem
                  label="Presets"
                  count={PRESETS.length}
                  active={activeGroup === "presets"}
                  onClick={() => setActiveGroup("presets")}
                />
                <div className="my-2 border-t" />
                <CategoryRailItem
                  label="All"
                  count={counts.all}
                  active={activeGroup === "all"}
                  onClick={() => setActiveGroup("all")}
                />
                <CategoryRailItem
                  label="Strategy"
                  count={counts.strategy}
                  active={activeGroup === "strategy"}
                  onClick={() => setActiveGroup("strategy")}
                />
                <CategoryRailItem
                  label="Depth"
                  count={counts.depth}
                  active={activeGroup === "depth"}
                  onClick={() => setActiveGroup("depth")}
                />
                <CategoryRailItem
                  label="Integrations"
                  count={counts.integration}
                  active={activeGroup === "integration"}
                  onClick={() => setActiveGroup("integration")}
                />
                <CategoryRailItem
                  label="Knowledge"
                  count={counts.knowledge}
                  active={activeGroup === "knowledge"}
                  onClick={() => setActiveGroup("knowledge")}
                />
                <div className="mt-2 border-t pt-2">
                  <button
                    type="button"
                    onClick={openCustomForm}
                    className="hover:bg-accent text-muted-foreground hover:text-foreground flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm transition-colors"
                  >
                    <Plus className="size-3.5" />
                    Custom block
                  </button>
                </div>
              </nav>

              <div className="overflow-y-auto p-4">
                {activeGroup === "presets" ? (
                  <div className="flex flex-col gap-3">
                    <p className="text-muted-foreground text-xs">
                      Picking a preset replaces the current composition with a
                      curated set of blocks. You can edit, reorder, or remove
                      any of them afterwards.
                    </p>
                    <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
                      {PRESETS.map((preset) => (
                        <LibraryTile
                          key={preset.id}
                          icon={preset.icon}
                          title={preset.label}
                          description={preset.description}
                          footer={
                            <span className="text-muted-foreground/70 mt-1 text-[10px] tabular-nums uppercase tracking-wide">
                              Includes {preset.blockIds.length} blocks
                            </span>
                          }
                          onClick={() => onApplyPreset(preset)}
                        />
                      ))}
                    </div>
                  </div>
                ) : filtered.length === 0 ? (
                  <div className="text-muted-foreground flex h-full items-center justify-center text-sm">
                    No blocks match {query ? `"${query}"` : "this filter"}.
                  </div>
                ) : activeGroup === "all" ? (
                  <div className="flex flex-col gap-6">
                    {sections.map((section) => (
                      <section key={section.group} className="flex flex-col gap-2">
                        <div className="flex items-baseline justify-between gap-2">
                          <h3 className="text-muted-foreground text-xs font-semibold tracking-wide uppercase">
                            {section.label}
                          </h3>
                          <span className="text-muted-foreground/70 text-xs tabular-nums">
                            {section.blocks.length}
                          </span>
                        </div>
                        <div className="grid grid-cols-2 gap-3 lg:grid-cols-3">
                          {section.blocks.map((block) => (
                            <LibraryTile
                              key={block.id}
                              icon={block.icon}
                              title={block.title}
                              description={block.description}
                              active={activeIds.has(block.id)}
                              onClick={() => onPick(block)}
                            />
                          ))}
                        </div>
                      </section>
                    ))}
                  </div>
                ) : (
                  <div className="grid grid-cols-2 gap-3 lg:grid-cols-3">
                    {filtered.map((block) => (
                      <LibraryTile
                        key={block.id}
                        icon={block.icon}
                        title={block.title}
                        description={block.description}
                        active={activeIds.has(block.id)}
                        onClick={() => onPick(block)}
                      />
                    ))}
                  </div>
                )}
              </div>
            </div>
            <DialogFooter className="border-t p-3 sm:justify-between">
              <span className="text-muted-foreground self-center text-xs">
                {composedCount === 0
                  ? "Tap blocks to add — pick as many as you like."
                  : `${composedCount} block${composedCount === 1 ? "" : "s"} in composition`}
              </span>
              <div className="flex w-full flex-col-reverse gap-2 sm:w-auto sm:flex-row sm:items-center">
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  disabled={composedCount === 0}
                  onClick={onClearAll}
                  className="text-muted-foreground hover:text-destructive hover:bg-destructive/10"
                >
                  Clear all
                </Button>
                <Button
                  type="button"
                  variant="secondary"
                  onClick={() => handleOpenChange(false)}
                >
                  Done
                </Button>
              </div>
            </DialogFooter>
          </>
        ) : (
          <CustomBlockForm
            value={customDraft}
            onChange={setCustomDraft}
            onCancel={() => setMode("pick")}
            onCommit={commitCustomBlock}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}

// CustomBlockForm is the modal's secondary view — a Textarea-only
// editor for one-off custom blocks. Title is omitted on purpose: it's
// derived from the first non-empty content line so authors only have
// to think about the prompt itself.
function CustomBlockForm({
  value,
  onChange,
  onCancel,
  onCommit,
}: {
  value: string;
  onChange: (next: string) => void;
  onCancel: () => void;
  onCommit: () => void;
}) {
  const ready = value.trim().length > 0;
  return (
    <>
      <DialogHeader className="border-b p-4">
        <div className="flex items-center gap-2">
          <Button
            type="button"
            variant="ghost"
            size="icon"
            onClick={onCancel}
            aria-label="Back to picker"
          >
            <ArrowLeft className="size-4" />
          </Button>
          <DialogTitle>Custom block</DialogTitle>
        </div>
        <DialogDescription className="sr-only">
          Write a one-off prompt block. The first non-empty line becomes
          the row title in the composition.
        </DialogDescription>
      </DialogHeader>
      <div className="flex flex-1 flex-col overflow-hidden">
        <Textarea
          autoFocus
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={
            "Paste or write a prompt fragment here…\n\nExamples:\n— Focus on transactions from last week only\n— Skip transactions over $500 unless flagged\n— Always include a TL;DR at the top of the report"
          }
          className="flex-1 resize-none rounded-none border-0 px-4 py-3 font-mono text-xs leading-relaxed shadow-none focus-visible:ring-0"
        />
      </div>
      <DialogFooter className="border-t p-3">
        <Button type="button" variant="ghost" onClick={onCancel}>
          Cancel
        </Button>
        <Button type="button" onClick={onCommit} disabled={!ready}>
          <Plus className="size-4" />
          Add to composition
        </Button>
      </DialogFooter>
    </>
  );
}

function CategoryRailItem({
  label,
  count,
  active,
  onClick,
}: {
  label: string;
  count: number;
  active: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "hover:bg-accent flex w-full items-center justify-between gap-2 rounded-md px-2 py-1.5 text-left text-sm transition-colors",
        active
          ? "bg-accent text-foreground font-medium"
          : "text-muted-foreground hover:text-foreground",
      )}
    >
      <span>{label}</span>
      <span
        className={cn(
          "text-xs tabular-nums",
          active ? "text-muted-foreground" : "text-muted-foreground/70",
        )}
      >
        {count}
      </span>
    </button>
  );
}

// LibraryTile is the shared card visual used by both BlockCard (one
// library block) and PresetCard (one preset). Active shows a check
// pill in the top-right; footer is an optional bottom line (e.g.
// "Includes 3 blocks" for presets).
function LibraryTile({
  icon,
  title,
  description,
  active,
  footer,
  onClick,
}: {
  icon?: string;
  title: string;
  description?: string;
  active?: boolean;
  footer?: React.ReactNode;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "group hover:border-primary/40 hover:bg-accent/40 relative flex h-full flex-col gap-2 rounded-lg border p-3 text-left transition-colors",
        active && "border-primary/40 bg-accent/30",
      )}
    >
      {active && (
        <span className="bg-primary text-primary-foreground absolute top-2 right-2 inline-flex size-5 items-center justify-center rounded-full">
          <Check className="size-3" />
        </span>
      )}
      {icon && (
        <span className="bg-muted text-muted-foreground group-hover:text-foreground inline-flex size-8 items-center justify-center rounded-md">
          <DynamicIcon name={icon} className="size-4" />
        </span>
      )}
      <div className="flex flex-col gap-1">
        <span className="text-sm font-medium leading-tight">{title}</span>
        {description && (
          <span className="text-muted-foreground line-clamp-3 text-xs leading-snug">
            {description}
          </span>
        )}
        {footer}
      </div>
    </button>
  );
}

interface ComposerTableProps {
  items: ComposedItem[];
  onReorder: (next: ComposedItem[]) => void;
  blocksById: Map<string, PromptBlock>;
  customs: CustomBlock[];
  edits: Record<string, string>;
  expanded: Record<string, boolean>;
  setExpanded: React.Dispatch<React.SetStateAction<Record<string, boolean>>>;
  setEdits: React.Dispatch<React.SetStateAction<Record<string, string>>>;
  setCustoms: React.Dispatch<React.SetStateAction<CustomBlock[]>>;
  onRemove: (item: ComposedItem) => void;
  // addBlockEmptyTrigger is the button rendered inside the empty-state
  // card. Just a button — it does NOT contain the dialog, which lives
  // at the parent so it survives the empty-state → table re-render.
  addBlockEmptyTrigger: React.ReactNode;
}

function ComposerTable({
  items,
  onReorder,
  blocksById,
  customs,
  edits,
  expanded,
  setExpanded,
  setEdits,
  setCustoms,
  onRemove,
  addBlockEmptyTrigger,
}: ComposerTableProps) {
  // Pre-index customs by id so SortableRow lookups are O(1) instead of
  // O(customs) per row per render.
  const customsById = useMemo(() => {
    const m = new Map<string, CustomBlock>();
    for (const c of customs) m.set(c.id, c);
    return m;
  }, [customs]);

  // dnd-kit sensors: PointerSensor with a small activation distance so
  // a quick row click doesn't accidentally start a drag (only deliberate
  // movement past 6px does). KeyboardSensor for accessibility.
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );

  // Collapse all expanded rows while a drag is in flight so the user
  // is dragging compact rows of uniform height (taller expanded rows
  // make reorder targeting jumpy). Snapshot which item ids were open
  // so we can re-expand exactly those after the drag settles. Stored
  // in a ref so it survives the re-render the collapse triggers.
  const expandedBeforeDrag = useRef<string[] | null>(null);

  const onDragStart = () => {
    const openIds = Object.entries(expanded)
      .filter(([, open]) => open)
      .map(([id]) => id);
    if (openIds.length === 0) return;
    expandedBeforeDrag.current = openIds;
    setExpanded((e) => {
      const next = { ...e };
      for (const id of openIds) next[id] = false;
      return next;
    });
  };

  const restoreExpanded = () => {
    const ids = expandedBeforeDrag.current;
    if (!ids || ids.length === 0) return;
    expandedBeforeDrag.current = null;
    setExpanded((e) => {
      const next = { ...e };
      for (const id of ids) next[id] = true;
      return next;
    });
  };

  const onDragEnd = (e: DragEndEvent) => {
    const { active, over } = e;
    if (over && active.id !== over.id) {
      const from = items.findIndex((i) => rowKey(i) === active.id);
      const to = items.findIndex((i) => rowKey(i) === over.id);
      if (from >= 0 && to >= 0) onReorder(arrayMove(items, from, to));
    }
    restoreExpanded();
  };

  if (items.length === 0) {
    return (
      <Card className="text-muted-foreground flex flex-col items-center gap-3 p-8 text-center text-sm">
        <Wand2 className="size-6 opacity-50" />
        <div>
          <div className="text-foreground font-medium">No blocks yet</div>
          <p className="max-w-xs text-xs">
            Pick a strategy to start composing. Add depth, integrations,
            knowledge, or a custom block as needed.
          </p>
        </div>
        {addBlockEmptyTrigger}
      </Card>
    );
  }

  const ids = items.map(rowKey);

  return (
    <Card className="overflow-hidden p-0">
      <DndContext
        sensors={sensors}
        collisionDetection={closestCenter}
        onDragStart={onDragStart}
        onDragEnd={onDragEnd}
        onDragCancel={restoreExpanded}
        modifiers={[restrictToVerticalAxis, restrictToParentElement]}
      >
        <Table>
          <TableBody>
            <SortableContext items={ids} strategy={verticalListSortingStrategy}>
              {items.map((item) => (
                <SortableRow
                  key={rowKey(item)}
                  item={item}
                  blocksById={blocksById}
                  customsById={customsById}
                  edits={edits}
                  expanded={Boolean(expanded[item.id])}
                  setExpanded={(open) =>
                    setExpanded((e) => ({ ...e, [item.id]: open }))
                  }
                  setEdits={setEdits}
                  setCustoms={setCustoms}
                  onRemove={() => onRemove(item)}
                />
              ))}
            </SortableContext>
          </TableBody>
        </Table>
      </DndContext>
    </Card>
  );
}

interface SortableRowProps {
  item: ComposedItem;
  blocksById: Map<string, PromptBlock>;
  customsById: Map<string, CustomBlock>;
  edits: Record<string, string>;
  expanded: boolean;
  setExpanded: (open: boolean) => void;
  setEdits: React.Dispatch<React.SetStateAction<Record<string, string>>>;
  setCustoms: React.Dispatch<React.SetStateAction<CustomBlock[]>>;
  onRemove: () => void;
}

function SortableRow({
  item,
  blocksById,
  customsById,
  edits,
  expanded,
  setExpanded,
  setEdits,
  setCustoms,
  onRemove,
}: SortableRowProps) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: rowKey(item) });

  const style: React.CSSProperties = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
  };

  const library =
    item.kind === "library" ? blocksById.get(item.id) : undefined;
  const custom =
    item.kind === "custom" ? customsById.get(item.id) : undefined;

  const title =
    library?.title ??
    (custom ? deriveCustomBlockTitle(custom.content) : "Unknown block");
  const description =
    library?.description ?? (custom ? "Custom block" : "");
  // Library blocks bring their own icon from frontmatter; custom
  // blocks fall back to a generic "you wrote this" pen-on-square mark
  // so the row stays visually aligned with library rows.
  const icon = library?.icon ?? (custom ? "square-pen" : "");

  const originalContent = library?.content ?? "";
  const editedContent = edits[item.id];
  const currentContent =
    item.kind === "custom"
      ? (custom?.content ?? "")
      : (editedContent ?? originalContent);
  const isEdited =
    item.kind === "library" &&
    editedContent !== undefined &&
    editedContent !== originalContent;

  return (
    <>
      <TableRow
        ref={setNodeRef}
        style={style}
        data-state={isDragging ? "dragging" : undefined}
        className={cn(isDragging && "relative z-10 bg-card shadow-md")}
      >
        <TableCell className="w-10 align-middle">
          <button
            type="button"
            className="text-muted-foreground hover:text-foreground flex h-6 w-6 cursor-grab items-center justify-center active:cursor-grabbing"
            aria-label="Drag to reorder"
            {...attributes}
            {...listeners}
          >
            <GripVertical className="size-4" />
          </button>
        </TableCell>
        <TableCell className="align-middle">
          <button
            type="button"
            className="hover:text-foreground flex w-full cursor-pointer items-center gap-2 text-left"
            onClick={() => setExpanded(!expanded)}
            aria-expanded={expanded}
          >
            <span className="text-muted-foreground shrink-0">
              {expanded ? (
                <ChevronDown className="size-4" />
              ) : (
                <ChevronRight className="size-4" />
              )}
            </span>
            {icon && (
              <DynamicIcon
                name={icon}
                className="text-muted-foreground shrink-0 size-4"
              />
            )}
            <span className="flex min-w-0 flex-col">
              <span className="flex items-center gap-2">
                <span className="truncate text-sm font-medium">{title}</span>
                {isEdited && (
                  <Badge variant="secondary" className="text-[10px]">
                    edited
                  </Badge>
                )}
              </span>
              {description && (
                <span className="text-muted-foreground truncate text-xs">
                  {description}
                </span>
              )}
            </span>
          </button>
        </TableCell>
        <TableCell className="w-16 text-right align-middle">
          <div className="flex justify-end gap-0.5">
            {isEdited && (
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label="Reset to original"
                title="Reset to the library version"
                onClick={() =>
                  setEdits((s) => {
                    const next = { ...s };
                    delete next[item.id];
                    return next;
                  })
                }
              >
                <Undo2 className="size-4" />
              </Button>
            )}
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label="Remove block"
              onClick={onRemove}
            >
              <Trash2 className="size-4" />
            </Button>
          </div>
        </TableCell>
      </TableRow>
      {expanded && (
        <TableRow
          className="hover:bg-transparent"
          data-expansion={rowKey(item)}
        >
          <TableCell colSpan={3} className="p-0">
            <Textarea
              id={`content-${item.id}`}
              value={currentContent}
              rows={Math.min(24, Math.max(8, currentContent.split("\n").length))}
              onChange={(e) => {
                const v = e.target.value;
                if (item.kind === "custom") {
                  setCustoms((cs) =>
                    cs.map((c) =>
                      c.id === item.id ? { ...c, content: v } : c,
                    ),
                  );
                } else {
                  setEdits((s) => ({ ...s, [item.id]: v }));
                }
              }}
              onKeyDown={(e) => {
                // Escape blurs the textarea so page-level shortcuts
                // (P, C, ⌘+Enter, etc.) become reachable again without
                // having to click outside the row.
                if (e.key === "Escape") {
                  e.preventDefault();
                  e.currentTarget.blur();
                }
              }}
              aria-label="Block content"
              className="bg-muted/60 border-border/60 block w-full resize-none rounded-none border-0 border-t px-4 py-3 font-mono text-xs leading-relaxed shadow-none focus-visible:border-0 focus-visible:border-t focus-visible:ring-0 dark:bg-black/30"
            />
          </TableCell>
        </TableRow>
      )}
    </>
  );
}

// ComposedStats surfaces the running size of the composed prompt in
// the floating toolbar — same metric the Preview dialog header shows,
// just always-on so the user can watch it grow as blocks are added or
// edited. Tabular numerals stop the digits from twitching during live
// updates.
function ComposedStats({ text }: { text: string }) {
  const lineCount = text === "" ? 0 : text.split("\n").length;
  return (
    <span className="text-muted-foreground text-xs whitespace-nowrap tabular-nums">
      {text.length.toLocaleString()} chars · {lineCount} lines
    </span>
  );
}

function PreviewButton({
  text,
  disabled,
}: {
  text: string;
  disabled: boolean;
}) {
  const [open, setOpen] = useState(false);
  useShortcut(["p"], () => setOpen((v) => !v), {
    label: "Preview prompt",
    group: "Prompt builder",
    enabled: !disabled,
  });
  return (
    <>
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={disabled}
        onClick={() => setOpen(true)}
      >
        <Eye className="size-4" />
        Preview
      </Button>
      <PreviewDialog open={open} onOpenChange={setOpen} text={text} />
    </>
  );
}

function PreviewDialog({
  open,
  onOpenChange,
  text,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  text: string;
}) {
  const lineCount = text === "" ? 0 : text.split("\n").length;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[85vh] flex-col gap-0 p-0 sm:max-w-3xl">
        <DialogHeader className="border-b py-4 pr-12 pl-6">
          {/* pr-12 reserves room for shadcn's absolute-positioned X
              close button on the right edge — without it the line/char
              count sits behind the X and gets clipped. */}
          <div className="flex items-center justify-between gap-3">
            <DialogTitle>Preview</DialogTitle>
            <span className="text-muted-foreground text-xs tabular-nums">
              {text.length.toLocaleString()} chars · {lineCount} lines
            </span>
          </div>
          <DialogDescription className="sr-only">
            The composed agent prompt as it will be sent to the model.
          </DialogDescription>
        </DialogHeader>
        <pre className="bg-muted/30 flex-1 overflow-auto px-6 py-4 font-mono text-xs leading-relaxed whitespace-pre-wrap">
          {text || "Add blocks to see the composed prompt."}
        </pre>
      </DialogContent>
    </Dialog>
  );
}

interface OutputActionsProps {
  text: string;
  disabled: boolean;
}

function OutputActions({ text, disabled }: OutputActionsProps) {
  const [copied, setCopied] = useState(false);
  const [pickerOpen, setPickerOpen] = useState(false);
  const navigate = useNavigate();

  const onCopy = async () => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      toast.success("Prompt copied to clipboard");
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      toast.error("Couldn't access the clipboard. Copy the preview manually.");
    }
  };

  useShortcut(["c"], () => onCopy(), {
    label: "Copy prompt",
    group: "Prompt builder",
    enabled: !disabled,
  });

  const onUseInNewAgent = () => {
    navigate({
      to: "/agents/new",
      search: { prompt: text },
    });
  };

  return (
    <div className="flex items-center gap-2">
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={onCopy}
        disabled={disabled}
      >
        {copied ? <Check className="size-4" /> : <Copy className="size-4" />}
        {copied ? "Copied" : "Copy"}
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button size="sm" disabled={disabled}>
            <Wand2 className="size-4" />
            Use in agent
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-72">
          <DropdownMenuGroup>
            <DropdownMenuItem onSelect={onUseInNewAgent}>
              <Plus className="size-4" /> Use in new agent
            </DropdownMenuItem>
            <DropdownMenuItem onSelect={() => setPickerOpen(true)}>
              <Wand2 className="size-4" /> Use in existing agent…
            </DropdownMenuItem>
          </DropdownMenuGroup>
          <DropdownMenuSeparator />
          <DropdownMenuLabel className="text-muted-foreground text-xs font-normal whitespace-normal leading-snug">
            Tip: Breadbox agents runs are optional and for convenience — the
            prompts work on any agent tool you connect to your Breadbox server.
          </DropdownMenuLabel>
        </DropdownMenuContent>
      </DropdownMenu>
      <ExistingAgentPicker
        open={pickerOpen}
        onOpenChange={setPickerOpen}
        text={text}
      />
    </div>
  );
}

function ExistingAgentPicker({
  open,
  onOpenChange,
  text,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  text: string;
}) {
  const agentsQuery = useAgents();
  const [slug, setSlug] = useState<string>("");
  const update = useUpdateAgent(slug);
  const agents = agentsQuery.data ?? [];

  const onConfirm = async () => {
    if (!slug) return;
    const ok = await withMutationToast(
      () => update.mutateAsync({ prompt: text }),
      {
        success: "Prompt pushed to agent",
        error: "Failed to update agent prompt",
      },
    );
    if (ok) {
      onOpenChange(false);
      setSlug("");
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Push prompt to an existing agent</DialogTitle>
          <DialogDescription>
            Replaces the agent's stored prompt with the composed text. The
            agent's other settings (model, schedule, scope) are unchanged.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-2">
          <Label htmlFor="agent-picker" className="text-xs">
            Agent
          </Label>
          <Select value={slug} onValueChange={setSlug}>
            <SelectTrigger id="agent-picker">
              <SelectValue placeholder="Pick an agent…" />
            </SelectTrigger>
            <SelectContent>
              {agents.map((a) => (
                <SelectItem key={a.id} value={a.slug}>
                  {a.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {agents.length === 0 && (
            <p className="text-muted-foreground text-xs">
              No agents yet. Use "Use in new agent" instead, or create one
              first via <Link to="/agents" className="underline">Agents</Link>.
            </p>
          )}
        </div>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={onConfirm} disabled={!slug || update.isPending}>
            Replace prompt
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// composePrompt concatenates the ordered blocks into a single markdown
// document. Blocks are separated by a blank line; the library block's
// title (`# …`) stays in the body so the resulting prompt keeps its
// section structure when read by the model. Edits override the library
// version when present.
function composePrompt(
  items: ComposedItem[],
  blocksById: Map<string, PromptBlock>,
  edits: Record<string, string>,
  customs: CustomBlock[],
): string {
  const parts: string[] = [];
  for (const item of items) {
    if (item.kind === "library") {
      const original = blocksById.get(item.id);
      if (!original) continue;
      const content = edits[item.id] ?? original.content;
      const trimmed = content.trim();
      if (trimmed) parts.push(trimmed);
    } else {
      const c = customs.find((x) => x.id === item.id);
      if (!c) continue;
      const body = c.content.trim();
      if (!body) continue;
      parts.push(body);
    }
  }
  return parts.join("\n\n");
}
