import { useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Kbd, KbdGroup } from "@/components/ui/kbd";
import { Card } from "@/components/ui/card";
import { withMutationToast } from "@/lib/mutation-toast";
import { useShortcut } from "@/lib/shortcuts";
import { displayKey } from "@/lib/kbd-display";
import {
  formatDate,
  formatDateTime,
  formatLongDate,
  formatRelativeShort,
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
    code: "formatDateTime(…)",
    input: "RFC3339 timestamp",
    output: formatDateTime(new Date().toISOString()),
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
  {
    code: "formatRelativeShort(…-12m)",
    input: "12 minutes ago",
    output: formatRelativeShort(
      new Date(Date.now() - 12 * 60_000).toISOString(),
    ),
  },
  {
    code: "formatRelativeShort(…-5d)",
    input: "5 days ago",
    output: formatRelativeShort(
      new Date(Date.now() - 5 * 86_400_000).toISOString(),
    ),
  },
  {
    code: "formatRelativeShort(null)",
    input: "null",
    output: formatRelativeShort(null),
  },
];

const KEY_TOKENS = ["mod", "shift", "alt", "ctrl", "enter", "esc", "k", "/"];

// A long-enough chip list to force horizontal overflow inside a typical
// list-page column so the scroll-shadow gradient is visible without
// resizing the viewport.
const SCROLL_RAIL_PILLS = [
  "All",
  "Inbox",
  "Uncategorised",
  "Food & Drink",
  "Transport",
  "Subscriptions",
  "Housing",
  "Travel",
  "Health",
  "Income",
];

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
      id="patterns"
      title="Patterns"
      description="Cross-cutting behaviors — not components, but the shared building blocks every feature reaches for."
    >
      <Specimen
        label="Toasts"
        code="sonner · withMutationToast"
        description="One toast surface for the whole app. Each tone gets a coloured left rail + tinted icon tile — same vocabulary as <StatusPanel>. withMutationToast wraps a mutation: success toast on resolve, the ApiError message on reject. Use the successDescription slot for any two-thought outcome (what happened + what to expect / do next) instead of cramming both into the title."
        className="block"
      >
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" onClick={() => toast.success("Saved.")}>
            success
          </Button>
          <Button
            variant="outline"
            onClick={() => toast.error("Something went wrong.")}
          >
            error
          </Button>
          <Button
            variant="outline"
            onClick={() =>
              toast.warning("Sync took longer than usual.", {
                description: "Two accounts returned partial data on the last pull.",
              })
            }
          >
            warning
          </Button>
          <Button
            variant="outline"
            onClick={() =>
              toast.info("Imported 50 of 1,243 transactions.", {
                description: "Showing the first page — keep scrolling to load more.",
              })
            }
          >
            info
          </Button>
          <Button variant="outline" onClick={() => toast.message("Heads up.")}>
            message
          </Button>
          <Button variant="outline" onClick={() => toast.loading("Re-syncing…")}>
            loading
          </Button>
        </div>
        <div className="mt-3 flex flex-wrap gap-2">
          <Button
            onClick={() =>
              withMutationToast(() => Promise.resolve(), {
                success: "Rule applied.",
                successDescription: "12 transactions matched and were re-categorised.",
              })
            }
          >
            withMutationToast — ok (with description)
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
          <Button
            variant="outline"
            onClick={() =>
              toast.success("Plaid sync queued.", {
                description: "Webhook will fire when the next batch lands.",
                action: {
                  label: "View",
                  onClick: () => toast.message("Action clicked."),
                },
              })
            }
          >
            success + action
          </Button>
        </div>
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
        description="Cached Intl instances — short, long and date+time renders, plus two relative variants. Use long form (formatRelativeTime) in body copy / tooltips; compact form (formatRelativeShort) in dense lists and pills. Amount formatting lives in the Amounts section."
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

      {/*
        Mobile / iOS Safari patterns — canon lives in
        `.claude/rules/v2-frontend.md` ("Mobile / iOS Safari patterns",
        #1344). These specimens demo the per-component techniques so future
        contributors can see them in isolation. Globals (reduced-motion,
        tap-highlight, web-app metadata, cold-load splash) intentionally
        omitted — the rules doc is the canonical reference for those.
      */}

      <Specimen
        label="scroll-shadow-x utility"
        code="globals.css · @utility scroll-shadow-x"
        description="A CSS-only gradient fade painted at the clipped edges of a horizontally-scrolling container so scrollability is visible without a scrollbar. Scroll the row below — the right edge fades; after scrolling, the left edge fades in. Default cover is `var(--card)` (matches the Table primitive); on non-card surfaces override with `[--scroll-shadow-cover:var(--background)]`. Pair with `[-webkit-overflow-scrolling:touch]` + `overscroll-contain` on iOS."
        className="block"
      >
        {/* Card surface — default --scroll-shadow-cover (var(--card)) blends. */}
        <div className="space-y-1">
          <p className="text-muted-foreground text-xs">
            On a <code className="font-mono">bg-card</code> surface — default
            cover.
          </p>
          <Card className="p-2">
            <div className="scroll-shadow-x flex gap-2 overflow-x-auto overscroll-contain [-webkit-overflow-scrolling:touch]">
              {SCROLL_RAIL_PILLS.map((p) => (
                <Button
                  key={`card-${p}`}
                  size="sm"
                  variant="outline"
                  className="shrink-0"
                >
                  {p}
                </Button>
              ))}
            </div>
          </Card>
        </div>

        {/* Page surface — override --scroll-shadow-cover to var(--background). */}
        <div className="mt-4 space-y-1">
          <p className="text-muted-foreground text-xs">
            On a <code className="font-mono">bg-background</code> surface —
            override via{" "}
            <code className="font-mono">
              [--scroll-shadow-cover:var(--background)]
            </code>
            .
          </p>
          <div className="scroll-shadow-x flex gap-2 overflow-x-auto overscroll-contain [-webkit-overflow-scrolling:touch] [--scroll-shadow-cover:var(--background)]">
            {SCROLL_RAIL_PILLS.map((p) => (
              <Button
                key={`bg-${p}`}
                size="sm"
                variant="outline"
                className="shrink-0"
              >
                {p}
              </Button>
            ))}
          </div>
        </div>
      </Specimen>

      <Specimen
        label="Mobile reflow patterns"
        code="flex-col-reverse · order-first · max-sm:scroll-shadow-x"
        description="Three canonical techniques for reshaping desktop layouts on narrow viewports without forking the component. Each demo is wrapped in a deliberately narrow container so the mobile state is visible without resizing the window."
        className="block"
      >
        {/* 1. Button stack with primary on top.
            Canon: see PR #1321 (FormFooter) / #1328 (disconnect) / #1336 +
            #1339 (prompts). The wrapper toggles `flex-col-reverse w-full`
            (primary above secondary, full-width) to `sm:flex-row sm:w-auto`
            (primary right-aligned). The DOM order stays {secondary, primary}
            so tab order is preserved; `flex-col-reverse` only inverts the
            visual axis. */}
        <div className="w-full space-y-1">
          <p className="text-muted-foreground text-xs">
            <strong className="text-foreground">Button stack, primary on top.</strong>{" "}
            Narrow widths get a full-width column with primary above
            secondary; sm+ reverts to an inline row. DOM order preserved for
            keyboard tab order.
          </p>
          <div className="bg-muted/30 max-w-[280px] rounded-md border p-3">
            <div className="flex w-full flex-col-reverse gap-2 sm:w-auto sm:flex-row sm:items-center sm:justify-end">
              <Button variant="outline" size="sm">
                Cancel
              </Button>
              <Button size="sm">Save changes</Button>
            </div>
          </div>
        </div>

        {/* 2. CSS `order` for mobile-first content.
            Canon: see PR #1322 (agent-form mobile reorder). DOM order is
            {Prompt, Controls} so keyboard tab order surfaces the long
            textarea first; visual order on mobile flips so the operational
            knobs sit above the prompt. `md:order-none` reverts both cards
            to source order at md+. */}
        <div className="mt-5 w-full space-y-1">
          <p className="text-muted-foreground text-xs">
            <strong className="text-foreground">CSS `order` to reshuffle visual order.</strong>{" "}
            DOM is {`{Prompt, Controls}`}; visual order on mobile is{" "}
            {`{Controls, Prompt}`} via{" "}
            <code className="font-mono">order-first md:order-none</code>.
            Tab/keyboard order still follows the DOM.
          </p>
          <div className="bg-muted/30 max-w-[420px] rounded-md border p-3">
            <div className="grid gap-2 md:grid-cols-2">
              <Card className="p-3 text-sm">
                <div className="text-muted-foreground text-xs uppercase tracking-wider">
                  Prompt (DOM 1)
                </div>
                <p className="text-foreground/80 mt-1 text-xs">
                  Long body content — wants to live first in source order so
                  screen readers and tab navigation reach it without a
                  detour.
                </p>
              </Card>
              <Card className="order-first p-3 text-sm md:order-none">
                <div className="text-muted-foreground text-xs uppercase tracking-wider">
                  Controls (DOM 2)
                </div>
                <p className="text-foreground/80 mt-1 text-xs">
                  Operational knobs — visually first on mobile so users
                  don't scroll past the long body to reach them.
                </p>
              </Card>
            </div>
          </div>
        </div>

        {/* 3. Horizontal scroll-rail for filter pills.
            Canon: see PR #1324 (transactions toolbar) / #1339 (prompts).
            At sm+ the pills wrap normally; below sm they switch to a
            single-row scroll rail with the scroll-shadow-x fade affordance.
            Override the cover to --background because the rail sits on the
            page surface, not on a card. */}
        <div className="mt-5 w-full space-y-1">
          <p className="text-muted-foreground text-xs">
            <strong className="text-foreground">Horizontal scroll-rail for filter pills.</strong>{" "}
            At sm+ the chips wrap; below sm they become a single-row scroll
            rail via{" "}
            <code className="font-mono">
              max-sm:flex-nowrap max-sm:overflow-x-auto max-sm:scroll-shadow-x
            </code>
            . Page surface, so cover is overridden to{" "}
            <code className="font-mono">--background</code>.
          </p>
          <div className="bg-background max-w-[360px] rounded-md border p-3">
            <div className="flex items-center gap-2 max-sm:scroll-shadow-x max-sm:flex-nowrap max-sm:overflow-x-auto max-sm:overscroll-contain max-sm:[--scroll-shadow-cover:var(--background)] max-sm:[-webkit-overflow-scrolling:touch] sm:flex-wrap">
              {SCROLL_RAIL_PILLS.map((p) => (
                <Button
                  key={`rail-${p}`}
                  size="sm"
                  variant="outline"
                  className="shrink-0"
                >
                  {p}
                </Button>
              ))}
            </div>
          </div>
        </div>
      </Specimen>
    </SandboxSection>
  );
}
