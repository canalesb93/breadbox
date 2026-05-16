import * as React from "react";
import { Keyboard } from "lucide-react";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Kbd, KbdGroup } from "@/components/ui/kbd";
import { displayKey } from "@/lib/kbd-display";
import {
  listShortcuts,
  subscribeShortcuts,
  useShortcut,
  type Shortcut,
} from "@/lib/shortcuts";

function useShortcutList(): Shortcut[] {
  const [list, setList] = React.useState<Shortcut[]>(() => listShortcuts());
  React.useEffect(() => subscribeShortcuts(() => setList(listShortcuts())), []);
  return list;
}

export function ShortcutSheet() {
  const [open, setOpen] = React.useState(false);
  const list = useShortcutList();

  useShortcut(["shift", "?"], () => setOpen((v) => !v), {
    label: "Show keyboard shortcuts",
    group: "Global",
  });

  // Command palette (and any other surface) can ask us to open without
  // owning a ref — keeps `open` state local but lets external callers
  // surface the sheet on demand.
  React.useEffect(() => {
    const onOpen = () => setOpen(true);
    window.addEventListener("breadbox:shortcut-sheet:open", onOpen);
    return () =>
      window.removeEventListener("breadbox:shortcut-sheet:open", onOpen);
  }, []);

  const grouped = React.useMemo(() => {
    const out = new Map<string, Shortcut[]>();
    for (const s of list) {
      const key = s.group ?? "Other";
      const arr = out.get(key) ?? [];
      arr.push(s);
      out.set(key, arr);
    }
    // "Global" first, then the rest alphabetically — stable regardless of
    // which page registered its shortcuts in what order.
    return Array.from(out.entries()).sort(([a], [b]) => {
      if (a === b) return 0;
      if (a === "Global") return -1;
      if (b === "Global") return 1;
      return a.localeCompare(b);
    });
  }, [list]);

  const totalCount = list.length;

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetContent
        side="right"
        // Override the default sheet width — shortcut rows + multi-key chord
        // pills need a touch more room than the stock max-w-sm. `p-0` so the
        // header/body/footer strips can control their own padding and the
        // sticky footer sits flush against the panel edge.
        className="w-full gap-0 p-0 sm:max-w-md"
      >
        {/* Header — mirrors the icon-tile vocabulary used by StatusPanel /
            EmptyState / SectionCard so the sheet reads as part of the v2
            system, not an ad-hoc overlay. */}
        <SheetHeader className="gap-3 border-b p-5">
          <div className="flex items-start gap-3">
            <span
              className="bg-muted text-muted-foreground flex size-9 shrink-0 items-center justify-center rounded-lg border"
              aria-hidden
            >
              <Keyboard className="size-4" />
            </span>
            <div className="min-w-0 flex-1">
              <SheetTitle className="text-base leading-tight">
                Keyboard shortcuts
              </SheetTitle>
              <SheetDescription className="mt-0.5 text-xs">
                Available across the app. Shortcuts pause while you&apos;re
                typing in an input.
              </SheetDescription>
            </div>
          </div>
        </SheetHeader>

        {/* Body — scrollable list of grouped cards. Each group is a bordered
            card with an uppercase eyebrow header + a divide-y body, mirroring
            the ListCard vocabulary used across the v2 surfaces. */}
        <div className="flex-1 overflow-y-auto px-5 py-4">
          {totalCount === 0 ? (
            <div className="text-muted-foreground flex flex-col items-center gap-1 rounded-md border border-dashed py-10 text-sm">
              <span className="font-medium">No shortcuts registered</span>
              <span className="text-xs">
                Pages register shortcuts as they mount.
              </span>
            </div>
          ) : (
            <div className="space-y-4">
              {grouped.map(([group, items]) => (
                <section
                  key={group}
                  className="bg-card overflow-hidden rounded-md border"
                >
                  <header className="bg-muted/30 flex items-baseline justify-between gap-2 border-b px-3 py-2">
                    <h3 className="text-muted-foreground text-[10px] font-semibold tracking-wider uppercase">
                      {group}
                    </h3>
                    <span className="text-muted-foreground/70 text-[10px] tabular-nums">
                      {items.length}
                    </span>
                  </header>
                  <ul className="divide-border/70 divide-y">
                    {items.map((s) => (
                      <li
                        key={s.id}
                        className="hover:bg-muted/40 flex items-center justify-between gap-4 px-3 py-2 transition-colors"
                      >
                        <span className="text-foreground text-sm">
                          {s.label}
                        </span>
                        <KbdGroup>
                          {s.keys.map((k) => (
                            <Kbd key={k}>{displayKey(k)}</Kbd>
                          ))}
                        </KbdGroup>
                      </li>
                    ))}
                  </ul>
                </section>
              ))}
            </div>
          )}
        </div>

        {/* Footer — thin action strip matching the cmdk footer vocabulary so
            the two overlays feel like siblings. */}
        <div className="bg-muted/30 text-muted-foreground flex items-center justify-between gap-3 border-t px-5 py-2.5 text-[11px]">
          <span className="inline-flex items-center gap-1.5">
            <KbdGroup>
              <Kbd className="bg-background/80">⇧</Kbd>
              <Kbd className="bg-background/80">?</Kbd>
            </KbdGroup>
            Toggle this sheet
          </span>
          <span className="inline-flex items-center gap-1.5">
            <Kbd className="bg-background/80">esc</Kbd>
            Close
          </span>
        </div>
      </SheetContent>
    </Sheet>
  );
}
