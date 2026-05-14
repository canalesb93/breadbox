import * as React from "react";
import { useNavigate } from "@tanstack/react-router";
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
        <CommandGroup heading="Account">
          <CommandItem value="logout sign out" onSelect={runLogout}>
            <span>Sign out</span>
          </CommandItem>
        </CommandGroup>
      </CommandList>
    </CommandDialog>
  );
}
