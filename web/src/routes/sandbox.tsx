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
    <div className="mx-auto max-w-5xl">
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

// SectionWithOutline pairs an active sandbox section with a sticky right-rail
// outline of its specimens. Specimens self-register via `data-specimen-label`
// from the `<Specimen>` primitive — we scan the section's DOM after each
// section change (and on resize) so the outline reflects whatever lives in
// the gallery without per-section wiring. Active item highlights via
// IntersectionObserver: whichever specimen is closest to the top of the
// viewport (under the 56px shell header) wins.
function SectionWithOutline({
  sectionId,
  children,
}: {
  sectionId: string;
  children: React.ReactNode;
}) {
  const contentRef = useRef<HTMLDivElement>(null);
  const [items, setItems] = useState<Array<{ id: string; label: string }>>([]);
  const [activeId, setActiveId] = useState<string | null>(null);

  // Rebuild the outline on section change. `useEffect` runs after children
  // commit, so specimens are mounted and queryable.
  useEffect(() => {
    const root = contentRef.current;
    if (!root) return;
    const next = [...root.querySelectorAll<HTMLElement>("[data-specimen-label]")].map(
      (el) => ({
        id: el.id,
        label: el.getAttribute("data-specimen-label") ?? el.id,
      }),
    );
    setItems(next);
    setActiveId(next[0]?.id ?? null);
  }, [sectionId]);

  // Highlight whichever specimen is closest to the top under the 56px shell
  // header. Rebuild when the item list changes (new section).
  useEffect(() => {
    if (items.length === 0) return;
    const root = contentRef.current;
    if (!root) return;
    const els = items
      .map((i) => root.querySelector<HTMLElement>(`#${CSS.escape(i.id)}`))
      .filter((el): el is HTMLElement => !!el);
    const observer = new IntersectionObserver(
      (entries) => {
        // Collect the entries that are currently intersecting and pick the
        // one with the smallest `top` — i.e. nearest the top of the root.
        const visible = entries
          .filter((e) => e.isIntersecting)
          .sort(
            (a, b) => a.boundingClientRect.top - b.boundingClientRect.top,
          );
        if (visible[0]) setActiveId(visible[0].target.id);
      },
      { rootMargin: "-56px 0px -60% 0px", threshold: 0 },
    );
    els.forEach((el) => observer.observe(el));
    return () => observer.disconnect();
  }, [items]);

  return (
    <div className="flex gap-8">
      <div ref={contentRef} className="min-w-0 flex-1">
        {children}
      </div>
      {items.length > 0 && (
        <aside className="sticky top-20 hidden h-fit w-48 shrink-0 lg:block">
          <p className="text-muted-foreground mb-2 px-2 text-[10px] font-semibold tracking-wider uppercase">
            On this page
          </p>
          <ul className="space-y-0.5">
            {items.map((i) => (
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
        </aside>
      )}
    </div>
  );
}
