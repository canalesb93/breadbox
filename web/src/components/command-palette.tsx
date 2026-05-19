import * as React from "react";
import { useNavigate } from "@tanstack/react-router";
import {
  ArrowUpDown,
  ChevronRight,
  CircleDot,
  Clock,
  CornerDownLeft,
  Keyboard,
  LogOut,
  Palette,
  type LucideIcon,
} from "lucide-react";
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
  CommandShortcut,
} from "@/components/ui/command";
import { Kbd, KbdGroup } from "@/components/ui/kbd";
import { cn } from "@/lib/utils";
import { NAV, NAV_LEAVES, navKey, type NavLeaf } from "@/lib/nav";
import { openModal } from "@/lib/modals";
import { useShortcut } from "@/lib/shortcuts";
import { useLogout } from "@/api/queries/auth";
import type { TransactionsSearch } from "@/routes/transactions";

// Quick "jump to this transactions view" actions. Each merges its params
// into the current transactions search. `value` carries extra search terms
// so cmdk surfaces them on partial matches ("pend", "larg").
const TX_QUICK_ACTIONS: {
  title: string;
  value: string;
  icon: LucideIcon;
  search: Partial<TransactionsSearch>;
}[] = [
  {
    title: "Filter: Pending",
    value: "transactions filter pending unreviewed",
    icon: CircleDot,
    search: { pending: "true" },
  },
  {
    title: "Filter: Posted",
    value: "transactions filter posted cleared",
    icon: CircleDot,
    search: { pending: "false" },
  },
  {
    title: "Sort: Largest amount",
    value: "transactions sort amount largest highest",
    icon: ArrowUpDown,
    search: { sort: "amount", dir: "desc" },
  },
  {
    title: "Sort: Newest first",
    value: "transactions sort date newest recent",
    icon: ArrowUpDown,
    search: { sort: "date", dir: "desc" },
  },
];

// Recent nav-jumps live in localStorage so they survive reloads. Cap at 4
// so the section doesn't crowd the palette; LRU eviction.
const RECENTS_KEY = "breadbox:cmdk:recents";
const RECENTS_MAX = 4;

function readRecents(): string[] {
  try {
    const raw = localStorage.getItem(RECENTS_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed.filter((v): v is string => typeof v === "string");
  } catch {
    return [];
  }
}

function pushRecent(key: string) {
  try {
    const current = readRecents().filter((k) => k !== key);
    const next = [key, ...current].slice(0, RECENTS_MAX);
    localStorage.setItem(RECENTS_KEY, JSON.stringify(next));
  } catch {
    // ignore — quota / private mode
  }
}

// Build an "ItemRow" so every line in the palette renders with the same
// vocabulary (icon, label, optional kbd hint, optional trailing chevron).
function ItemRow({
  icon: Icon,
  title,
  hint,
  trailingChevron = false,
}: {
  icon: LucideIcon;
  title: string;
  hint?: React.ReactNode;
  trailingChevron?: boolean;
}) {
  return (
    <>
      <Icon className="size-4" />
      <span>{title}</span>
      {hint ? <CommandShortcut>{hint}</CommandShortcut> : null}
      {trailingChevron ? (
        <ChevronRight
          // When a hint is present it already owns `ml-auto` (via
          // `CommandShortcut`) and pushes itself right; two `ml-auto`
          // siblings would split the free space and park the hint mid-row.
          // Drop our own `ml-auto` in that case so the chevron sits flush
          // against the hint.
          className={cn(
            "text-muted-foreground/0 group-data-[selected=true]:text-muted-foreground/70 size-3.5 transition-colors",
            hint ? "ml-2" : "ml-auto",
          )}
          aria-hidden
        />
      ) : null}
    </>
  );
}

export function CommandPalette() {
  const [open, setOpen] = React.useState(false);
  const [recents, setRecents] = React.useState<string[]>([]);
  const navigate = useNavigate();
  const logout = useLogout();

  useShortcut(["mod", "k"], () => setOpen((v) => !v), {
    label: "Open command palette",
    group: "Global",
    // ⌘K must toggle from anywhere — including from inside the palette
    // itself (its own search input / dialog) to close it.
    global: true,
  });

  // Topbar search affordance fires this event — keeps the palette as the
  // sole source of open state without exposing a global store.
  React.useEffect(() => {
    const onOpen = () => setOpen(true);
    window.addEventListener("breadbox:command-palette:open", onOpen);
    return () =>
      window.removeEventListener("breadbox:command-palette:open", onOpen);
  }, []);

  // Re-read recents whenever the palette opens — cheap, and avoids stale
  // state if another tab pushed an entry while we were closed.
  React.useEffect(() => {
    if (open) setRecents(readRecents());
  }, [open]);

  const go = (to: string, recentKey?: string) => {
    setOpen(false);
    if (recentKey) pushRecent(recentKey);
    navigate({ to });
  };

  const runOpenModal = (modalKey: string, recentKey?: string) => {
    setOpen(false);
    if (recentKey) pushRecent(recentKey);
    navigate({ to: ".", search: openModal(modalKey) });
  };

  const runNavLeaf = (leaf: NavLeaf, group: string) => {
    const key = navKey(leaf);
    if (leaf.kind === "link") go(leaf.to, key);
    else runOpenModal(leaf.modalKey, key);
    // group is in scope so we could later record group→item analytics; for
    // now it's the value used by cmdk for fuzzy matching.
    void group;
  };

  const runQuickFilter = (patch: Partial<TransactionsSearch>) => {
    setOpen(false);
    // Merge onto the current search rather than replacing it — "Sort:
    // Largest" shouldn't wipe an active filter. Unknown keys carried in from
    // another route's params are stripped by the transactions search schema.
    navigate({
      to: "/transactions",
      search: (prev: Record<string, unknown>) => ({ ...prev, ...patch }),
    });
  };

  const runLogout = async () => {
    setOpen(false);
    await logout.mutateAsync().catch(() => {});
    navigate({ to: "/login" });
  };

  // Resolve the recent keys to nav leaves once per render — keeps the
  // recents section in sync with NAV ordering / icons (no forked metadata).
  const recentLeaves = React.useMemo(() => {
    if (!recents.length) return [];
    const byKey = new Map(NAV_LEAVES.map((l) => [navKey(l.leaf), l]));
    return recents
      .map((k) => byKey.get(k))
      .filter((v): v is (typeof NAV_LEAVES)[number] => Boolean(v));
  }, [recents]);

  return (
    <CommandDialog
      open={open}
      onOpenChange={setOpen}
      // Tighten the default cmdk wrapper so rows scan denser. Default ships
      // with py-3 / size-5 icons — overshoots for a navigation-heavy
      // palette. We also style group headings as uppercase eyebrows so
      // they read as section labels, not titles.
      className="overflow-hidden p-0 sm:max-w-xl"
    >
      <CommandInput placeholder="Search or jump to anything…" />
      <CommandList className="max-h-[440px] [&_[cmdk-group-heading]]:text-muted-foreground/70 [&_[cmdk-group-heading]]:px-3 [&_[cmdk-group-heading]]:pt-2.5 [&_[cmdk-group-heading]]:pb-1 [&_[cmdk-group-heading]]:text-[10px] [&_[cmdk-group-heading]]:font-semibold [&_[cmdk-group-heading]]:tracking-wider [&_[cmdk-group-heading]]:uppercase [&_[cmdk-group]]:px-2 [&_[cmdk-input-wrapper]_svg]:size-4 [&_[cmdk-item]]:gap-2.5 [&_[cmdk-item]]:px-2.5 [&_[cmdk-item]]:py-2 [&_[cmdk-item]_svg]:size-4">
        <CommandEmpty>
          <div className="text-muted-foreground flex flex-col items-center gap-1 py-4 text-sm">
            <span className="font-medium">No matches</span>
            <span className="text-xs">
              Try a page name, a tag, or a quick action.
            </span>
          </div>
        </CommandEmpty>

        {recentLeaves.length > 0 ? (
          <>
            <CommandGroup heading="Recent">
              {recentLeaves.map(({ leaf, group }) => (
                <CommandItem
                  key={`recent:${navKey(leaf)}`}
                  // Make recents searchable both by their own name and the
                  // group they belong to, while keeping cmdk's scoring
                  // honest (uniqueness via the explicit prefix).
                  value={`recent ${group} ${leaf.title}`}
                  onSelect={() => runNavLeaf(leaf, group)}
                  className="group"
                >
                  <ItemRow
                    icon={Clock}
                    title={leaf.title}
                    hint={<span className="text-[10px]">{group}</span>}
                    trailingChevron
                  />
                </CommandItem>
              ))}
            </CommandGroup>
            <CommandSeparator />
          </>
        ) : null}

        {NAV.map((group, idx) => (
          <React.Fragment key={group.label}>
            <CommandGroup heading={`Jump to · ${group.label}`}>
              {group.items.map((item) => (
                <CommandItem
                  key={navKey(item)}
                  value={`${group.label} ${item.title}`}
                  onSelect={() => runNavLeaf(item, group.label)}
                  className="group"
                >
                  <ItemRow
                    icon={item.icon}
                    title={item.title}
                    trailingChevron
                  />
                </CommandItem>
              ))}
            </CommandGroup>
            {idx < NAV.length - 1 ? <CommandSeparator /> : null}
          </React.Fragment>
        ))}

        <CommandSeparator />
        <CommandGroup heading="Transactions · Quick actions">
          {TX_QUICK_ACTIONS.map((action) => (
            <CommandItem
              key={action.title}
              value={action.value}
              onSelect={() => runQuickFilter(action.search)}
              className="group"
            >
              <ItemRow
                icon={action.icon}
                title={action.title}
                trailingChevron
              />
            </CommandItem>
          ))}
        </CommandGroup>

        <CommandSeparator />
        <CommandGroup heading="Help & developer">
          <CommandItem
            value="keyboard shortcuts kbd help cheatsheet"
            onSelect={() => {
              setOpen(false);
              window.dispatchEvent(
                new CustomEvent("breadbox:shortcut-sheet:open"),
              );
            }}
            className="group"
          >
            <ItemRow
              icon={Keyboard}
              title="Keyboard shortcuts"
              hint={
                <KbdGroup>
                  <Kbd className="bg-muted/60">⇧</Kbd>
                  <Kbd className="bg-muted/60">?</Kbd>
                </KbdGroup>
              }
            />
          </CommandItem>
          <CommandItem
            value="design system sandbox components primitives"
            onSelect={() => go("/sandbox")}
            className="group"
          >
            <ItemRow
              icon={Palette}
              title="Design system"
              trailingChevron
            />
          </CommandItem>
        </CommandGroup>

        <CommandSeparator />
        <CommandGroup heading="Account">
          <CommandItem
            value="logout sign out"
            onSelect={runLogout}
            className="group"
          >
            <ItemRow icon={LogOut} title="Sign out" />
          </CommandItem>
        </CommandGroup>
      </CommandList>

      {/* Action strip — matches the standard cmdk vocabulary (Linear,
          Raycast, shadcn examples) so first-time users know what the
          keys do without leaving the palette. */}
      <div className="bg-muted/30 text-muted-foreground flex items-center justify-between gap-3 border-t px-3 py-2 text-[11px]">
        <div className="flex items-center gap-3">
          <span className="inline-flex items-center gap-1.5">
            <KbdGroup>
              <Kbd className="bg-background/80">↑</Kbd>
              <Kbd className="bg-background/80">↓</Kbd>
            </KbdGroup>
            Navigate
          </span>
          <span className="inline-flex items-center gap-1.5">
            <Kbd className="bg-background/80">
              <CornerDownLeft className="size-3" aria-hidden />
            </Kbd>
            Select
          </span>
        </div>
        <span className="inline-flex items-center gap-1.5">
          <Kbd className="bg-background/80">esc</Kbd>
          Close
        </span>
      </div>
    </CommandDialog>
  );
}
