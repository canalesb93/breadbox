import {
  CircleCheckIcon,
  InfoIcon,
  Loader2Icon,
  OctagonXIcon,
  TriangleAlertIcon,
} from "lucide-react"
import { useTheme } from "next-themes"
import { Toaster as Sonner, type ToasterProps } from "sonner"

// Tracks the shadcn sonner registry verbatim (theme inheritance + lucide
// icon overrides + popover-themed CSS vars), then layers v2-specific
// chrome on top: a 3px tone-tinted left rail and a tone-tinted icon
// tile, both echoing <StatusPanel> so the toast/panel pair speaks the
// same vocabulary. Everything else (positioning, dismiss behaviour,
// close affordance) is left at sonner's defaults so the surface matches
// the shadcn reference.
const Toaster = ({ ...props }: ToasterProps) => {
  const { theme = "system" } = useTheme()

  return (
    <Sonner
      theme={theme as ToasterProps["theme"]}
      className="toaster group"
      icons={{
        success: <CircleCheckIcon className="size-4" />,
        info: <InfoIcon className="size-4" />,
        warning: <TriangleAlertIcon className="size-4" />,
        error: <OctagonXIcon className="size-4" />,
        loading: <Loader2Icon className="size-4 animate-spin" />,
      }}
      toastOptions={{
        // The base toast keeps `pl-5` so the absolute `before:` rail and
        // the icon both clear the left edge. `data-[type=*]` selectors
        // tint the rail and the icon tile per variant; "message" (the
        // default toast.message call) falls through to the neutral
        // info-style rail.
        classNames: {
          toast:
            "group/toast toast relative overflow-hidden " +
            "!pl-5 " +
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
