import { useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Moon, Sun } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { FoundationsSection } from "@/sandbox/sections/foundations";
import { PrimitivesSection } from "@/sandbox/sections/primitives";
import { ComponentsSection } from "@/sandbox/sections/components";
import { AmountsSection } from "@/sandbox/sections/amounts";
import { PatternsSection } from "@/sandbox/sections/patterns";
import { sampleCategories, sampleTags } from "@/sandbox/fixtures";

const SECTIONS = [
  { id: "foundations", label: "Foundations", Component: FoundationsSection },
  { id: "primitives", label: "Primitives", Component: PrimitivesSection },
  { id: "components", label: "Components", Component: ComponentsSection },
  { id: "amounts", label: "Amounts", Component: AmountsSection },
  { id: "patterns", label: "Patterns", Component: PatternsSection },
] as const;

type SectionId = (typeof SECTIONS)[number]["id"];

// SandboxPage is the v2 design-system gallery — a living reference for the
// reusable components and primitives. Components that fetch reference data
// (useCategories / useTags / TagList) read from the query cache, which this
// page seeds with static fixtures so the gallery needs no real data.
export function SandboxPage() {
  const qc = useQueryClient();
  // Prime the cache once, before the section components mount, so the
  // category/tag pickers render from fixtures instead of hitting the API.
  // A useState initializer runs synchronously pre-render (unlike useEffect);
  // StrictMode double-invokes it in dev, which is harmless here — the
  // fixtures are module constants, so setQueryData is idempotent.
  useState(() => {
    qc.setQueryData(["categories"], sampleCategories);
    qc.setQueryData(["tags"], sampleTags);
    return null;
  });

  const [active, setActive] = useState<SectionId>("foundations");
  // The theme toggle is a scoped preview: it flips the global `.dark` class
  // so the whole gallery re-themes, but restores the original mode on
  // unmount so it never leaks out to the rest of the app.
  const [origDark] = useState(() =>
    document.documentElement.classList.contains("dark"),
  );
  const [dark, setDark] = useState(origDark);

  useEffect(() => {
    return () => {
      document.documentElement.classList.toggle("dark", origDark);
    };
  }, [origDark]);

  const toggleDark = () => {
    const next = !dark;
    document.documentElement.classList.toggle("dark", next);
    setDark(next);
  };

  const ActiveSection =
    SECTIONS.find((s) => s.id === active)?.Component ?? FoundationsSection;

  return (
    <div className="w-full">
      <div className="mb-6 flex items-start justify-between gap-4">
        <div className="space-y-1">
          <h1 className="text-xl font-semibold tracking-tight">
            Design system
          </h1>
          <p className="text-muted-foreground text-sm">
            A living gallery of v2's reusable components, primitives, and
            patterns. Static fixtures — nothing here writes to your data.
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={toggleDark}
          aria-label="Toggle theme"
        >
          {dark ? <Sun className="size-4" /> : <Moon className="size-4" />}
          {dark ? "Light" : "Dark"}
        </Button>
      </div>

      <div className="flex gap-8">
        <nav className="sticky top-6 hidden h-fit shrink-0 sm:block">
          <ul className="space-y-0.5">
            {SECTIONS.map((s) => (
              <li key={s.id}>
                <button
                  type="button"
                  onClick={() => setActive(s.id)}
                  className={cn(
                    "w-full rounded-md px-3 py-1.5 text-left text-sm transition-colors",
                    active === s.id
                      ? "bg-accent text-accent-foreground font-medium"
                      : "text-muted-foreground hover:text-foreground hover:bg-accent/50",
                  )}
                >
                  {s.label}
                </button>
              </li>
            ))}
          </ul>
        </nav>

        {/* Mobile section switcher */}
        <div className="min-w-0 flex-1">
          <div className="mb-6 flex flex-wrap gap-2 sm:hidden">
            {SECTIONS.map((s) => (
              <Button
                key={s.id}
                size="sm"
                variant={active === s.id ? "default" : "outline"}
                onClick={() => setActive(s.id)}
              >
                {s.label}
              </Button>
            ))}
          </div>
          <SectionWithOutline sectionId={active}>
            <ActiveSection />
          </SectionWithOutline>
        </div>
      </div>
    </div>
  );
}

interface OutlineSpecimen {
  id: string;
  label: string;
}
interface OutlineGroup {
  id: string;
  title: string;
  items: OutlineSpecimen[];
}

// SectionWithOutline pairs an active sandbox section with a sticky right-rail
// outline of its specimens. Specimens self-register via `data-specimen-label`
// from the `<Specimen>` primitive; `<SandboxGroup>` wraps clusters and emits
// `data-specimen-group="Title"`. The outline walks the section in document
// order, grouping every specimen under its nearest preceding group (an
// implicit "General" bucket catches specimens that live outside any group).
// Active item highlights via IntersectionObserver — whichever specimen is
// closest to the top of the viewport (under the 56px shell header) wins.
function SectionWithOutline({
  sectionId,
  children,
}: {
  sectionId: string;
  children: React.ReactNode;
}) {
  const contentRef = useRef<HTMLDivElement>(null);
  const [groups, setGroups] = useState<OutlineGroup[]>([]);
  const [activeId, setActiveId] = useState<string | null>(null);

  useEffect(() => {
    const root = contentRef.current;
    if (!root) return;
    const nodes = root.querySelectorAll<HTMLElement>(
      "[data-specimen-group], [data-specimen-label]",
    );
    const next: OutlineGroup[] = [];
    let current: OutlineGroup | null = null;
    const ungrouped: OutlineSpecimen[] = [];
    for (const el of Array.from(nodes)) {
      const groupTitle = el.getAttribute("data-specimen-group");
      if (groupTitle) {
        current = { id: el.id, title: groupTitle, items: [] };
        next.push(current);
        continue;
      }
      const label = el.getAttribute("data-specimen-label");
      if (!label) continue;
      const item: OutlineSpecimen = { id: el.id, label };
      if (current) current.items.push(item);
      else ungrouped.push(item);
    }
    setGroups(
      ungrouped.length
        ? [{ id: "outline-general", title: "General", items: ungrouped }, ...next]
        : next,
    );
    setActiveId(ungrouped[0]?.id ?? next[0]?.items[0]?.id ?? null);
  }, [sectionId]);

  useEffect(() => {
    const root = contentRef.current;
    if (!root) return;
    const all = groups.flatMap((g) => g.items);
    if (all.length === 0) return;
    const els = all
      .map((i) => root.querySelector<HTMLElement>(`#${CSS.escape(i.id)}`))
      .filter((el): el is HTMLElement => !!el);
    const observer = new IntersectionObserver(
      (entries) => {
        const visible = entries
          .filter((e) => e.isIntersecting)
          .sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top);
        if (visible[0]) setActiveId(visible[0].target.id);
      },
      { rootMargin: "-56px 0px -60% 0px", threshold: 0 },
    );
    els.forEach((el) => observer.observe(el));
    return () => observer.disconnect();
  }, [groups]);

  const hasItems = groups.some((g) => g.items.length > 0);

  return (
    <div className="flex gap-8">
      <div ref={contentRef} className="min-w-0 flex-1">
        {children}
      </div>
      {hasItems && (
        <aside className="sticky top-20 hidden h-[calc(100vh-6rem)] w-52 shrink-0 overflow-y-auto pr-2 lg:block">
          <p className="text-muted-foreground mb-2 px-2 text-[10px] font-semibold tracking-wider uppercase">
            On this page
          </p>
          <div className="space-y-3">
            {groups.map((g) => (
              <div key={g.id}>
                {/* Only render group label when there are real groups —
                    the implicit "General" bucket is unlabeled. */}
                {g.id !== "outline-general" && (
                  <p className="text-muted-foreground/80 mb-1 px-2 text-[10px] font-semibold tracking-wider uppercase">
                    {g.title}
                  </p>
                )}
                <ul className="space-y-0.5">
                  {g.items.map((i) => (
                    <li key={i.id}>
                      <a
                        href={`#${i.id}`}
                        className={cn(
                          "block truncate rounded-md px-2 py-1 text-xs transition-colors",
                          activeId === i.id
                            ? "text-foreground bg-accent/50 font-medium"
                            : "text-muted-foreground hover:text-foreground hover:bg-accent/30",
                        )}
                      >
                        {i.label}
                      </a>
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </div>
        </aside>
      )}
    </div>
  );
}
