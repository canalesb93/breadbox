import { Building2, FileSpreadsheet, Landmark } from "lucide-react";
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
              "group flex w-full items-start gap-3 rounded-md border p-4 text-left transition",
              "hover:border-primary/60 hover:bg-accent/40",
              isSelected
                ? "border-primary bg-accent/40 ring-2 ring-primary/20"
                : "border-border",
              !isConfigured &&
                "cursor-not-allowed opacity-50 hover:border-border hover:bg-transparent",
            )}
          >
            <div
              className={cn(
                "flex size-9 shrink-0 items-center justify-center rounded-md border",
                isSelected
                  ? "border-primary/40 bg-primary/10 text-primary"
                  : "border-border bg-muted/40 text-muted-foreground",
              )}
            >
              <meta.Icon className="size-4" />
            </div>
            <div className="flex-1">
              <div className="flex items-center gap-2">
                <span className="text-sm font-medium">{meta.name}</span>
                {!isConfigured && (
                  <span className="text-muted-foreground text-xs">
                    Not configured
                  </span>
                )}
              </div>
              {meta.tagline && (
                <p className="text-muted-foreground mt-0.5 text-xs">
                  {meta.tagline}
                </p>
              )}
            </div>
          </button>
        );
      })}
    </div>
  );
}
