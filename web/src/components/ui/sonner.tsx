import {
  CircleCheckIcon,
  InfoIcon,
  Loader2Icon,
  OctagonXIcon,
  TriangleAlertIcon,
} from "lucide-react"
import { useTheme } from "next-themes"
import { Toaster as Sonner, type ToasterProps } from "sonner"

// Sonner Toaster customized to match the v2 design vocabulary established by
// <StatusPanel>: a 3px tone-tinted left rail + a tone-tinted icon tile + the
// existing popover surface. Tone is encoded in the rail/icon colour so
// success/error/warning/info are scannable at a glance without changing the
// background fill (which keeps the toast quiet enough to live next to dense
// surfaces like the transactions table).
//
// All polish lives here; `lib/mutation-toast.ts` and direct `toast.*` calls
// stay one-liners. Default position bottom-right, expand-on-hover, and a
// close button on every toast (mutation results are often quick reads and
// dismissing them feels right).
const Toaster = ({ ...props }: ToasterProps) => {
  const { theme = "system" } = useTheme()

  return (
    <Sonner
      theme={theme as ToasterProps["theme"]}
      className="toaster group"
      position="bottom-right"
      expand
      closeButton
      visibleToasts={4}
      icons={{
        success: <CircleCheckIcon className="size-4" />,
        info: <InfoIcon className="size-4" />,
        warning: <TriangleAlertIcon className="size-4" />,
        error: <OctagonXIcon className="size-4" />,
        loading: <Loader2Icon className="size-4 animate-spin" />,
      }}
      toastOptions={{
        // Class hooks are concatenated by Sonner onto each toast/element.
        // The base toast keeps `pl-5` so the absolute `before:` rail and
        // the icon both clear the left edge. `data-[type=*]` selectors
        // tint the rail and the icon tile per variant; "message" (the
        // default toast.message call) falls through to the neutral
        // info-style rail.
        classNames: {
          toast:
            "group/toast toast relative overflow-hidden " +
            "!bg-popover !text-popover-foreground !border !border-border !rounded-md " +
            "!gap-3 !p-4 !pl-5 " +
            "before:absolute before:inset-y-0 before:left-0 before:w-[3px] " +
            "data-[type=success]:before:bg-success " +
            "data-[type=error]:before:bg-destructive " +
            "data-[type=warning]:before:bg-amber-500 " +
            "data-[type=info]:before:bg-sky-500 " +
            "data-[type=default]:before:bg-muted-foreground/40 " +
            "data-[type=loading]:before:bg-muted-foreground/40",
          icon:
            "!m-0 !size-8 !shrink-0 !rounded-md !flex !items-center !justify-center " +
            "group-data-[type=success]/toast:!bg-success/12 " +
            "group-data-[type=success]/toast:!text-success " +
            "group-data-[type=error]/toast:!bg-destructive/10 " +
            "group-data-[type=error]/toast:!text-destructive " +
            "group-data-[type=warning]/toast:!bg-amber-500/10 " +
            "group-data-[type=warning]/toast:!text-amber-600 " +
            "dark:group-data-[type=warning]/toast:!text-amber-400 " +
            "group-data-[type=info]/toast:!bg-sky-500/10 " +
            "group-data-[type=info]/toast:!text-sky-600 " +
            "dark:group-data-[type=info]/toast:!text-sky-400 " +
            "group-data-[type=default]/toast:!bg-muted " +
            "group-data-[type=default]/toast:!text-muted-foreground " +
            "group-data-[type=loading]/toast:!bg-muted " +
            "group-data-[type=loading]/toast:!text-muted-foreground",
          content: "!gap-0.5",
          title: "!text-sm !font-medium !text-foreground !leading-snug",
          description:
            "!text-xs !text-muted-foreground !leading-relaxed !mt-0.5",
          actionButton:
            "!bg-primary !text-primary-foreground hover:!bg-primary/90 " +
            "!h-7 !rounded-md !px-2.5 !text-xs !font-medium",
          cancelButton:
            "!bg-transparent !text-muted-foreground hover:!text-foreground " +
            "!h-7 !rounded-md !px-2 !text-xs",
          // Match the shadcn ghost icon-button vocabulary: no border, no
          // background until hover, foreground-tinted icon at hover. Sonner's
          // default close (bordered + popover-bg) reads as a separate floating
          // badge stuck to the corner — out of place next to the rest of v2's
          // close affordances. Override the inline svg sizing/stroke so the
          // icon matches the weight of lucide icons elsewhere (sonner ships a
          // 12×12, stroke-1.5 svg by default).
          closeButton:
            "!bg-transparent !border-0 !text-muted-foreground/70 " +
            "hover:!bg-muted hover:!text-foreground " +
            "!size-6 !rounded-md !flex !items-center !justify-center transition-colors " +
            "!left-auto !right-2 !top-2 !translate-x-0 !translate-y-0 " +
            "[&_svg]:!size-3.5 [&_svg]:!stroke-[2]",
        },
      }}
      style={
        {
          "--normal-bg": "var(--popover)",
          "--normal-text": "var(--popover-foreground)",
          "--normal-border": "var(--border)",
          "--border-radius": "var(--radius)",
        } as React.CSSProperties
      }
      {...props}
    />
  )
}

export { Toaster }
