import { useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Kbd, KbdGroup } from "@/components/ui/kbd";
import { withMutationToast } from "@/lib/mutation-toast";
import { useShortcut } from "@/lib/shortcuts";
import { displayKey } from "@/lib/kbd-display";
import {
  formatDate,
  formatLongDate,
  formatRelativeTime,
} from "@/lib/format";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import { useIsMobile } from "@/hooks/use-mobile";
import { useMediaQuery } from "@/hooks/use-media-query";
import { SandboxSection, Specimen } from "@/sandbox/kit";

// Amount formatting has its own section; this covers the date/time formatters.
const FORMATTERS: { code: string; input: string; output: string }[] = [
  {
    code: "formatDate('2026-05-13')",
    input: "2026-05-13",
    output: formatDate("2026-05-13"),
  },
  {
    code: "formatLongDate('2026-05-13')",
    input: "2026-05-13",
    output: formatLongDate("2026-05-13"),
  },
  {
    code: "formatRelativeTime(…-3h)",
    input: "3 hours ago",
    output: formatRelativeTime(new Date(Date.now() - 3 * 3600_000).toISOString()),
  },
  {
    code: "formatRelativeTime(…-8d)",
    input: "8 days ago",
    output: formatRelativeTime(
      new Date(Date.now() - 8 * 86_400_000).toISOString(),
    ),
  },
];

const KEY_TOKENS = ["mod", "shift", "alt", "ctrl", "enter", "esc", "k", "/"];

export function PatternsSection() {
  const [debounceInput, setDebounceInput] = useState("");
  const debounced = useDebouncedValue(debounceInput, 400);
  const isMobile = useIsMobile();
  const isWide = useMediaQuery("(min-width: 1024px)");

  // A live shortcut — press G anywhere on this page. It also shows up in the
  // ⇧? shortcut sheet while this section is mounted, then auto-unregisters.
  useShortcut(["g"], () => toast.message("G — sandbox shortcut fired"), {
    label: "Sandbox demo shortcut",
    group: "Sandbox",
  });

  return (
    <SandboxSection
      title="Patterns"
      description="Cross-cutting behaviors — not components, but the shared building blocks every feature reaches for."
    >
      <Specimen
        label="Toasts"
        code="sonner · withMutationToast"
        description="One toast surface for the whole app. withMutationToast wraps a mutation: success toast on resolve, the ApiError message on reject."
      >
        <Button variant="outline" onClick={() => toast.success("Saved.")}>
          toast.success
        </Button>
        <Button
          variant="outline"
          onClick={() => toast.error("Something went wrong.")}
        >
          toast.error
        </Button>
        <Button variant="outline" onClick={() => toast.message("Heads up.")}>
          toast.message
        </Button>
        <Button
          onClick={() =>
            withMutationToast(() => Promise.resolve(), {
              success: "Category updated.",
            })
          }
        >
          withMutationToast — ok
        </Button>
        <Button
          variant="destructive"
          onClick={() =>
            withMutationToast(() => Promise.reject(new Error("nope")), {
              success: "won't show",
              error: "Couldn't update the category.",
            })
          }
        >
          withMutationToast — fail
        </Button>
      </Specimen>

      <Specimen
        label="Keyboard shortcuts"
        code="useShortcut · ShortcutSheet"
        description="Shortcuts register into a global registry while their component is mounted. Press G now (live demo). ⌘K opens the command palette; ⇧? opens the full shortcut sheet."
        className="block"
      >
        <div className="flex flex-wrap items-center gap-2">
          <KbdGroup>
            <Kbd>G</Kbd>
          </KbdGroup>
          <span className="text-muted-foreground text-sm">
            press it — fires a toast, and appears in the ⇧? sheet under
            “Sandbox”
          </span>
        </div>
        <div className="mt-3 flex flex-wrap items-center gap-2">
          <span className="text-muted-foreground text-xs">
            displayKey() maps raw tokens to glyphs:
          </span>
          {KEY_TOKENS.map((k) => (
            <span key={k} className="flex items-center gap-1 text-xs">
              <code className="text-muted-foreground font-mono">{k}</code>→
              <Kbd>{displayKey(k)}</Kbd>
            </span>
          ))}
        </div>
      </Specimen>

      <Specimen
        label="Date formatters"
        code="lib/format"
        description="Cached Intl instances — short and long dates, relative time. Amount formatting lives in the Amounts section."
        className="block"
      >
        <div className="grid gap-2 sm:grid-cols-2">
          {FORMATTERS.map((f) => (
            <div
              key={f.code}
              className="flex items-center justify-between gap-3 rounded-md border px-3 py-2"
            >
              <code className="text-muted-foreground font-mono text-xs">
                {f.code}
              </code>
              <span className="text-sm font-medium tabular-nums">
                {f.output}
              </span>
            </div>
          ))}
        </div>
      </Specimen>

      <Specimen
        label="Hooks"
        code="useDebouncedValue · useIsMobile · useMediaQuery"
        description="useDebouncedValue keeps URL params from churning per keystroke; the media hooks drive responsive branching."
        className="block"
      >
        <div className="grid max-w-sm gap-1.5">
          <Label htmlFor="sb-debounce">Type — debounced 400ms</Label>
          <Input
            id="sb-debounce"
            value={debounceInput}
            onChange={(e) => setDebounceInput(e.target.value)}
            placeholder="Type fast…"
          />
          <p className="text-muted-foreground text-xs">
            debounced value:{" "}
            <span className="text-foreground font-mono">
              {debounced || "—"}
            </span>
          </p>
        </div>
        <div className="text-muted-foreground mt-3 flex gap-4 text-sm">
          <span>
            useIsMobile():{" "}
            <code className="text-foreground font-mono">{String(isMobile)}</code>
          </span>
          <span>
            useMediaQuery(min-width:1024px):{" "}
            <code className="text-foreground font-mono">{String(isWide)}</code>
          </span>
        </div>
      </Specimen>
    </SandboxSection>
  );
}
