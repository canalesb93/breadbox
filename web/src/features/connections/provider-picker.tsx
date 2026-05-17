import { Building2, Check, FileSpreadsheet, Landmark } from "lucide-react";
import { cn } from "@/lib/utils";

// PROVIDER_META is the static labelling for every known provider. Centralised
// so the picker, sandbox, and any future hosted-link surface render the same
// names + tagline + icon for `plaid` / `teller` / `csv`.
export const PROVIDER_META: Record<
  string,
  { name: string; tagline: string; Icon: typeof Landmark }
> = {
  plaid: {
    name: "Plaid",
    tagline: "12,000+ banks across the US, Canada, and Europe.",
    Icon: Landmark,
  },
  teller: {
    name: "Teller",
    tagline: "US banks via Teller Connect.",
    Icon: Building2,
  },
  csv: {
    name: "CSV import",
    tagline: "Upload a statement export from any bank.",
    Icon: FileSpreadsheet,
  },
};

export interface ProviderPickerProps {
  /** Provider names that should appear as enabled cards. Anything not in this
   *  list still renders, but is disabled with a "not configured" hint so the
   *  user knows this server hasn't been wired up for it. */
  enabledProviders: string[];
  /** Subset of providers to show. Defaults to plaid + teller + csv. */
  providers?: string[];
  value: string | null;
  onChange: (provider: string) => void;
  className?: string;
}

// ProviderPicker is the step-1 surface of the Connect-bank Sheet — a stacked
// list of provider cards. Pure: takes value/onChange, leaves the form-state
// to the caller. Reused by the sandbox specimen (display-only) and, soon, by
// the hosted-link page (`/link/{token}`) where a household member finishes
// a connection without an admin login.
//
// Visual contract (iter 40): each row mirrors the v2 active-state vocabulary
// — `border-primary` + `bg-primary/5` selection with a tinted icon tile
// (rounded-xl size-9 to match CategoryIconTile / StatusPanel / EmptyState),
// a trailing `Check` mark when selected, and a muted "Not configured" pill
// when the provider isn't wired up on this server.
export function ProviderPicker({
  enabledProviders,
  providers = ["plaid", "teller", "csv"],
  value,
  onChange,
  className,
}: ProviderPickerProps) {
  const enabled = new Set(enabledProviders);
  return (
    <div className={cn("flex flex-col gap-2", className)}>
      {providers.map((p) => {
        const meta = PROVIDER_META[p] ?? {
          name: p,
          tagline: "",
          Icon: Landmark,
        };
        const isConfigured = enabled.has(p);
        const isSelected = value === p;
        return (
          <button
            key={p}
            type="button"
            disabled={!isConfigured}
            onClick={() => onChange(p)}
            aria-pressed={isSelected}
            className={cn(
              "group relative flex w-full items-center gap-3 rounded-lg border px-3.5 py-3 text-left transition",
              "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/40 focus-visible:ring-offset-2 focus-visible:ring-offset-background",
              isConfigured && !isSelected && "hover:border-primary/40 hover:bg-accent/40",
              isSelected
                ? "border-primary bg-primary/5"
                : "border-border",
              !isConfigured &&
                "cursor-not-allowed opacity-60 hover:border-border hover:bg-transparent",
            )}
          >
            <span
              className={cn(
                "flex size-9 shrink-0 items-center justify-center rounded-xl border transition",
                isSelected
                  ? "border-primary/30 bg-primary/10 text-primary"
                  : "border-border bg-muted/50 text-muted-foreground group-hover:text-foreground",
              )}
            >
              <meta.Icon className="size-4" />
            </span>
            <div className="flex min-w-0 flex-1 flex-col gap-0.5">
              <div className="flex items-center gap-2">
                <span className="text-foreground text-sm font-medium">
                  {meta.name}
                </span>
                {!isConfigured && (
                  <span className="bg-muted/60 text-muted-foreground rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide">
                    Not configured
                  </span>
                )}
              </div>
              {meta.tagline && (
                <p className="text-muted-foreground text-xs leading-relaxed">
                  {meta.tagline}
                </p>
              )}
            </div>
            <span
              aria-hidden
              className={cn(
                "flex size-5 shrink-0 items-center justify-center rounded-full border transition",
                isSelected
                  ? "border-primary bg-primary text-primary-foreground"
                  : "border-border/60 bg-transparent text-transparent",
              )}
            >
              <Check className="size-3" strokeWidth={3} />
            </span>
          </button>
        );
      })}
    </div>
  );
}
