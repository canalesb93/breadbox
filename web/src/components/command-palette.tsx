import * as React from "react";
import { useNavigate } from "@tanstack/react-router";
import { ArrowUpDown, CircleDot, Palette, type LucideIcon } from "lucide-react";
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from "@/components/ui/command";
import { NAV, navKey } from "@/lib/nav";
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

export function CommandPalette() {
  const [open, setOpen] = React.useState(false);
  const navigate = useNavigate();
  const logout = useLogout();

  useShortcut(["mod", "k"], () => setOpen((v) => !v), {
    label: "Open command palette",
    group: "Global",
    // ⌘K must toggle from anywhere — including from inside the palette
    // itself (its own search input / dialog) to close it.
    global: true,
  });

  const go = (to: string) => {
    setOpen(false);
    navigate({ to });
  };

  const runOpenModal = (modalKey: string) => {
    setOpen(false);
    navigate({ to: ".", search: openModal(modalKey) });
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

  return (
    <CommandDialog open={open} onOpenChange={setOpen}>
      <CommandInput placeholder="Type a command or search…" />
      <CommandList>
        <CommandEmpty>No results found.</CommandEmpty>
        {NAV.map((group) => (
          <CommandGroup key={group.label} heading={group.label}>
            {group.items.map((item) => {
              const Icon = item.icon;
              return (
                <CommandItem
                  key={navKey(item)}
                  value={`${group.label} ${item.title}`}
                  onSelect={() =>
                    item.kind === "link"
                      ? go(item.to)
                      : runOpenModal(item.modalKey)
                  }
                >
                  <Icon className="size-4" />
                  <span>{item.title}</span>
                </CommandItem>
              );
            })}
          </CommandGroup>
        ))}
        <CommandSeparator />
        <CommandGroup heading="Transactions">
          {TX_QUICK_ACTIONS.map((action) => {
            const Icon = action.icon;
            return (
              <CommandItem
                key={action.title}
                value={action.value}
                onSelect={() => runQuickFilter(action.search)}
              >
                <Icon className="size-4" />
                <span>{action.title}</span>
              </CommandItem>
            );
          })}
        </CommandGroup>
        <CommandSeparator />
        <CommandGroup heading="Developer">
          <CommandItem
            value="design system sandbox components primitives"
            onSelect={() => go("/sandbox")}
          >
            <Palette className="size-4" />
            <span>Design system</span>
          </CommandItem>
        </CommandGroup>
        <CommandSeparator />
        <CommandGroup heading="Account">
          <CommandItem value="logout sign out" onSelect={runLogout}>
            <span>Sign out</span>
          </CommandItem>
        </CommandGroup>
      </CommandList>
    </CommandDialog>
  );
}
