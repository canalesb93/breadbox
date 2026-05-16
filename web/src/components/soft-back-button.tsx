import { Link, useRouter, type LinkProps } from "@tanstack/react-router";
import { ArrowLeft } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

interface SoftBackButtonProps {
  // The canonical list URL this page belongs under. Acts as both the visible
  // `<a href>` (so middle-click / cmd-click / "open in new tab" all work) and
  // the fallback navigation target when there is no in-app history.
  to: LinkProps["to"];
  // Visible label — e.g. "Back to transactions". Render it as a short,
  // verb-led phrase: "Back to <plural-noun>".
  children: React.ReactNode;
  className?: string;
}

// SoftBackButton renders a small ghost back-link that prefers the in-app
// history stack on a plain left-click — landing the user on the exact list
// state they came from (filters, scroll, focus) — and falls through to a
// real navigation when the history is empty or the user is opening in a
// new context. Shared across every detail / form page that hangs off a list
// (TX detail, Account detail, Connection detail, Category detail, API-key
// new — promoted to a primitive in the iter-13 api-keys pass).
//
// Visual contract:
//   `<Button variant="ghost" size="sm" className="-ml-2 mb-3 h-7 px-2 text-xs">`
//
// Don't fork the look — change this primitive instead.
export function SoftBackButton({
  to,
  children,
  className,
}: SoftBackButtonProps) {
  const router = useRouter();
  return (
    <Button
      variant="ghost"
      size="sm"
      asChild
      className={cn(
        "text-muted-foreground hover:text-foreground mb-3 -ml-2 h-7 px-2 text-xs",
        className,
      )}
    >
      <Link
        to={to}
        onClick={(e) => {
          if (
            !e.defaultPrevented &&
            !e.metaKey &&
            !e.ctrlKey &&
            !e.shiftKey &&
            !e.altKey &&
            e.button === 0 &&
            window.history.length > 1
          ) {
            e.preventDefault();
            router.history.back();
          }
        }}
      >
        <ArrowLeft className="size-3.5" />
        {children}
      </Link>
    </Button>
  );
}
