import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Moon, Sun } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { FoundationsSection } from "@/sandbox/sections/foundations";
import { PrimitivesSection } from "@/sandbox/sections/primitives";
import { ComponentsSection } from "@/sandbox/sections/components";
import { PatternsSection } from "@/sandbox/sections/patterns";
import { sampleCategories, sampleTags } from "@/sandbox/fixtures";

const SECTIONS = [
  { id: "foundations", label: "Foundations", Component: FoundationsSection },
  { id: "primitives", label: "Primitives", Component: PrimitivesSection },
  { id: "components", label: "Components", Component: ComponentsSection },
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
  useState(() => {
    qc.setQueryData(["categories"], sampleCategories);
    qc.setQueryData(["tags"], sampleTags);
    return null;
  });

  const [active, setActive] = useState<SectionId>("foundations");
  const [dark, setDark] = useState(() =>
    document.documentElement.classList.contains("dark"),
  );

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
          <ActiveSection />
        </div>
      </div>
    </div>
  );
}
