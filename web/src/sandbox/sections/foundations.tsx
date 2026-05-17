import { DynamicIcon } from "@/lib/icon";
import { SandboxSection, Specimen } from "@/sandbox/kit";

// Static class strings so Tailwind's JIT picks them up — these can't be
// computed.
const SURFACES = [
  { cls: "bg-background", name: "background" },
  { cls: "bg-card", name: "card" },
  { cls: "bg-popover", name: "popover" },
  { cls: "bg-muted", name: "muted" },
  { cls: "bg-accent", name: "accent" },
  { cls: "bg-secondary", name: "secondary" },
  { cls: "bg-sidebar", name: "sidebar" },
];

const BRAND = [
  { cls: "bg-primary", fg: "text-primary-foreground", name: "primary" },
  {
    cls: "bg-secondary",
    fg: "text-secondary-foreground",
    name: "secondary",
  },
  {
    cls: "bg-destructive",
    fg: "text-destructive-foreground",
    name: "destructive",
  },
  { cls: "bg-success", fg: "text-background", name: "success" },
];

const LINES = [
  { cls: "bg-border", name: "border" },
  { cls: "bg-input", name: "input" },
  { cls: "bg-ring", name: "ring" },
];

const CHARTS = [
  "bg-chart-1",
  "bg-chart-2",
  "bg-chart-3",
  "bg-chart-4",
  "bg-chart-5",
];

const RADII = [
  { cls: "rounded-sm", name: "sm" },
  { cls: "rounded-md", name: "md" },
  { cls: "rounded-lg", name: "lg" },
  { cls: "rounded-xl", name: "xl" },
];

const ICONS = [
  "coffee",
  "car",
  "banknote",
  "utensils",
  "briefcase",
  "receipt",
  "flag",
  "shopping-basket",
];

function Swatch({ cls, name }: { cls: string; name: string }) {
  return (
    <div className="space-y-1.5">
      <div className={`${cls} h-12 w-full rounded-md border`} />
      <code className="text-muted-foreground font-mono text-xs">{name}</code>
    </div>
  );
}

export function FoundationsSection() {
  return (
    <SandboxSection
      id="foundations"
      title="Foundations"
      description="Theme tokens, shape, type, and the icon system — the layer everything else is built on. Toggle the theme (top right) to check both modes."
    >
      <Specimen
        label="Surfaces"
        code="globals.css"
        description="Background layers. Each is paired with a matching -foreground token for text."
        className="block"
      >
        <div className="grid w-full grid-cols-3 gap-3 sm:grid-cols-4 lg:grid-cols-7">
          {SURFACES.map((s) => (
            <Swatch key={s.name} cls={s.cls} name={s.name} />
          ))}
        </div>
      </Specimen>

      <Specimen
        label="Brand & status"
        description="Solid fills with foreground text. `success` was added for inflow amounts."
        className="block"
      >
        <div className="grid w-full grid-cols-2 gap-3 sm:grid-cols-4">
          {BRAND.map((b) => (
            <div key={b.name} className="space-y-1.5">
              <div
                className={`${b.cls} ${b.fg} flex h-12 w-full items-center justify-center rounded-md text-xs font-medium`}
              >
                Aa
              </div>
              <code className="text-muted-foreground font-mono text-xs">
                {b.name}
              </code>
            </div>
          ))}
        </div>
      </Specimen>

      <Specimen label="Lines & focus" className="block">
        <div className="grid w-full grid-cols-3 gap-3">
          {LINES.map((l) => (
            <Swatch key={l.name} cls={l.cls} name={l.name} />
          ))}
        </div>
      </Specimen>

      <Specimen
        label="Chart palette"
        code="chart-1…5"
        description="Reserved for data viz so chart colors never collide with UI semantics."
        className="block"
      >
        <div className="grid w-full grid-cols-5 gap-3">
          {CHARTS.map((c, i) => (
            <Swatch key={c} cls={c} name={`chart-${i + 1}`} />
          ))}
        </div>
      </Specimen>

      <Specimen
        label="Radius"
        code="--radius"
        description="One base radius; sm/md/lg/xl derive from it."
      >
        {RADII.map((r) => (
          <div key={r.name} className="space-y-1.5 text-center">
            <div className={`${r.cls} bg-muted size-14 border`} />
            <code className="text-muted-foreground font-mono text-xs">
              {r.name}
            </code>
          </div>
        ))}
      </Specimen>

      <Specimen label="Typography" className="block space-y-2">
        <p className="text-lg font-semibold tracking-tight">
          Heading — text-lg font-semibold
        </p>
        <p className="text-sm">Body — text-sm</p>
        <p className="text-muted-foreground text-xs">
          Muted caption — text-xs text-muted-foreground
        </p>
        <p className="font-mono text-sm tabular-nums">
          Tabular numerals — 1,234.50 · font-mono tabular-nums
        </p>
      </Specimen>

      <Specimen
        label="Icons"
        code="DynamicIcon"
        description="lucide-react/dynamic, kebab-case names, lazily code-split per icon. Unknown names render nothing; category/tag colors tint via style."
      >
        {ICONS.map((name) => (
          <div key={name} className="space-y-1 text-center">
            <div className="bg-muted flex size-10 items-center justify-center rounded-md">
              <DynamicIcon name={name} className="size-5" />
            </div>
            <code className="text-muted-foreground font-mono text-[10px]">
              {name}
            </code>
          </div>
        ))}
      </Specimen>

      <Specimen
        label="Hover & transition vocabulary"
        code="transition-colors · hover:bg-*"
        description="Three canonical hover patterns — pick the one that matches the host surface, not by visual weight. Drift across these tokens reads as inconsistency; never invent a fourth."
        className="block"
      >
        <div className="grid w-full gap-3 sm:grid-cols-3">
          <div className="space-y-2">
            <div className="rounded-lg border bg-card overflow-hidden">
              <div className="border-b px-3 py-2 text-xs text-muted-foreground">
                Divide-y list row
              </div>
              <ul className="divide-y">
                {["Coffee", "Groceries", "Fuel"].map((label) => (
                  <li
                    key={label}
                    className="hover:bg-muted/40 flex items-center justify-between gap-3 px-3 py-2.5 text-sm transition-colors"
                  >
                    <span>{label}</span>
                    <span className="text-muted-foreground tabular-nums text-xs">
                      −$12.34
                    </span>
                  </li>
                ))}
              </ul>
            </div>
            <p className="text-muted-foreground text-xs leading-relaxed">
              <code className="font-mono">transition-colors hover:bg-muted/40</code>
              — matches <code className="font-mono">&lt;TableRow&gt;</code>. Use
              inside <code className="font-mono">&lt;ListCard&gt;</code> rows
              (Home recent activity, Account recent transactions, Categories,
              Shortcut sheet).
            </p>
          </div>

          <div className="space-y-2">
            <div className="grid gap-2">
              {["Plaid", "Teller"].map((label) => (
                <button
                  key={label}
                  type="button"
                  className="hover:bg-accent/40 hover:border-primary/40 flex w-full items-center gap-3 rounded-lg border bg-card px-3 py-3 text-left text-sm transition-colors"
                >
                  <span className="bg-muted/50 size-7 rounded-md" />
                  <span className="flex-1 font-medium">{label}</span>
                  <span className="text-muted-foreground text-xs">Connect</span>
                </button>
              ))}
            </div>
            <p className="text-muted-foreground text-xs leading-relaxed">
              <code className="font-mono">transition-colors hover:bg-accent/40</code>
              — bordered card grid items that act as selectable picks
              (provider-picker, connection-accounts-list cards).
            </p>
          </div>

          <div className="space-y-2">
            <div className="grid gap-2">
              {["Jump to Categories", "Open shortcuts"].map((label) => (
                <a
                  key={label}
                  href="#"
                  onClick={(e) => e.preventDefault()}
                  className="group bg-muted/20 hover:bg-muted/40 hover:border-ring/40 relative flex items-center justify-between gap-3 rounded-md border px-3 py-2.5 text-sm transition-colors"
                >
                  <span>{label}</span>
                  <span className="text-muted-foreground text-xs">↗</span>
                </a>
              ))}
            </div>
            <p className="text-muted-foreground text-xs leading-relaxed">
              <code className="font-mono">
                bg-muted/20 hover:bg-muted/40 hover:border-ring/40
              </code>
              — tinted-idle card grid items (not-found quick jumps). Idle
              tint signals "this is interactive" before hover; ring border on
              hover doubles the affordance.
            </p>
          </div>
        </div>
      </Specimen>
    </SandboxSection>
  );
}
