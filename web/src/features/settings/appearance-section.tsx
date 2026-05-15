import { Check, Monitor, Moon, Sun, type LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";
import {
  THEME_MODES,
  THEME_PRESETS,
  type ThemeMode,
  type ThemePreset,
} from "@/lib/theme";
import { useTheme } from "@/hooks/use-theme";

const MODE_ICONS: Record<ThemeMode, LucideIcon> = {
  system: Monitor,
  light: Sun,
  dark: Moon,
};

export function AppearanceSection() {
  const { theme, setMode, setPreset } = useTheme();

  return (
    <div className="space-y-6">
      <div className="space-y-1">
        <h2 className="text-lg font-medium">Appearance</h2>
        <p className="text-muted-foreground text-sm">
          Pick a mode and an accent color. Stored on this device.
        </p>
      </div>

      <div className="space-y-3">
        <h3 className="text-sm font-medium">Mode</h3>
        <div className="grid grid-cols-3 gap-2">
          {THEME_MODES.map((m) => {
            const Icon = MODE_ICONS[m.id];
            const active = theme.mode === m.id;
            return (
              <button
                key={m.id}
                type="button"
                onClick={() => setMode(m.id)}
                aria-pressed={active}
                className={cn(
                  "border-border hover:bg-accent flex flex-col items-center gap-2 rounded-md border p-4 text-sm transition-colors",
                  active && "border-ring ring-ring/30 ring-2",
                )}
              >
                <Icon className="size-5" />
                <span>{m.label}</span>
              </button>
            );
          })}
        </div>
      </div>

      <div className="space-y-3">
        <h3 className="text-sm font-medium">Preset</h3>
        <p className="text-muted-foreground text-xs">
          Swatches preview the accent against your current mode.
        </p>
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
          {THEME_PRESETS.map((p) => (
            <PresetCard
              key={p.id}
              id={p.id}
              label={p.label}
              active={theme.preset === p.id}
              onSelect={() => setPreset(p.id)}
            />
          ))}
        </div>
      </div>
    </div>
  );
}

function PresetCard({
  id,
  label,
  active,
  onSelect,
}: {
  id: ThemePreset;
  label: string;
  active: boolean;
  onSelect: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      aria-pressed={active}
      className={cn(
        "border-border hover:bg-accent flex items-center gap-3 rounded-md border p-3 text-left text-sm transition-colors",
        active && "border-ring ring-ring/30 ring-2",
      )}
    >
      {/* The swatch nests a `data-theme` scope so the preview reflects that
          preset's --primary regardless of the page's current preset. */}
      <span
        data-theme={id}
        className="bg-primary flex size-8 shrink-0 items-center justify-center rounded-md"
      >
        {active && <Check className="text-primary-foreground size-4" />}
      </span>
      <span className="flex-1">{label}</span>
    </button>
  );
}
