import * as React from "react";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Kbd, KbdGroup } from "@/components/ui/kbd";
import {
  listShortcuts,
  subscribeShortcuts,
  useShortcut,
  type Shortcut,
} from "@/lib/shortcuts";

const DISPLAY: Record<string, string> = {
  mod: "⌘",
  cmd: "⌘",
  ctrl: "Ctrl",
  shift: "⇧",
  alt: "⌥",
  option: "⌥",
};

function display(k: string): string {
  const lower = k.toLowerCase();
  if (lower in DISPLAY) return DISPLAY[lower];
  return k.length === 1 ? k.toUpperCase() : k;
}

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

  const grouped = React.useMemo(() => {
    const out = new Map<string, Shortcut[]>();
    for (const s of list) {
      const key = s.group ?? "Other";
      const arr = out.get(key) ?? [];
      arr.push(s);
      out.set(key, arr);
    }
    return Array.from(out.entries());
  }, [list]);

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetContent side="right" className="w-full sm:max-w-md">
        <SheetHeader>
          <SheetTitle>Keyboard shortcuts</SheetTitle>
          <SheetDescription>
            Available across the app. Inputs are ignored while typing.
          </SheetDescription>
        </SheetHeader>
        <div className="space-y-6 px-4 pb-6">
          {grouped.map(([group, items]) => (
            <div key={group} className="space-y-2">
              <h3 className="text-muted-foreground text-xs font-medium tracking-wider uppercase">
                {group}
              </h3>
              <ul className="divide-border divide-y">
                {items.map((s) => (
                  <li
                    key={s.id}
                    className="flex items-center justify-between gap-4 py-2"
                  >
                    <span className="text-sm">{s.label}</span>
                    <KbdGroup>
                      {s.keys.map((k) => (
                        <Kbd key={k}>{display(k)}</Kbd>
                      ))}
                    </KbdGroup>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
      </SheetContent>
    </Sheet>
  );
}
