import * as React from "react"
import { cva, type VariantProps } from "class-variance-authority"
import { Slot } from "radix-ui"

import { cn } from "@/lib/utils"

// Icon-only sizes share an invisible 44×44pt tap-zone on touch devices
// (Apple HIG minimum). The visible square stays small; a centered
// pseudo-element extends the hit area without affecting layout. Scoped
// to `pointer-coarse:` so desktop mice (`pointer: fine`) are unaffected.
// See MOBILE-5 + the `Buttons — tap target` specimen in the sandbox.
const TAP_TARGET =
  "relative pointer-coarse:before:absolute pointer-coarse:before:left-1/2 pointer-coarse:before:top-1/2 pointer-coarse:before:size-11 pointer-coarse:before:-translate-x-1/2 pointer-coarse:before:-translate-y-1/2 pointer-coarse:before:content-['']"

const buttonVariants = cva(
  // Disabled treatment: filled variants (default/destructive) override
  // `disabled:opacity-50` with a `bg-muted text-muted-foreground` swap below
  // so dark-mode primary doesn't read as a bright gray block (the dark
  // `--primary: oklch(0.922)` × `opacity-50` produced a glaring tile).
  // Outline/ghost/secondary/link keep the inherited opacity treatment —
  // they have no strong fill, so 50% reads fine.
  "inline-flex shrink-0 items-center justify-center gap-2 rounded-md text-sm font-medium whitespace-nowrap transition-all outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
  {
    variants: {
      variant: {
        default:
          "bg-primary text-primary-foreground hover:bg-primary/90 disabled:bg-muted disabled:text-muted-foreground disabled:opacity-100 disabled:shadow-none",
        destructive:
          "bg-destructive text-white hover:bg-destructive/90 focus-visible:ring-destructive/20 dark:bg-destructive/60 dark:focus-visible:ring-destructive/40 disabled:bg-muted disabled:text-muted-foreground disabled:opacity-100 disabled:shadow-none",
        outline:
          "border bg-background shadow-xs hover:bg-accent hover:text-accent-foreground dark:border-input dark:bg-input/30 dark:hover:bg-input/50",
        secondary:
          "bg-secondary text-secondary-foreground hover:bg-secondary/80",
        ghost:
          "hover:bg-accent hover:text-accent-foreground dark:hover:bg-accent/50",
        link: "text-primary underline-offset-4 hover:underline",
      },
      size: {
        default: "h-9 px-4 py-2 has-[>svg]:px-3",
        xs: "h-6 gap-1 rounded-md px-2 text-xs has-[>svg]:px-1.5 [&_svg:not([class*='size-'])]:size-3",
        sm: "h-8 gap-1.5 rounded-md px-3 has-[>svg]:px-2.5",
        lg: "h-10 rounded-md px-6 has-[>svg]:px-4",
        icon: `size-9 ${TAP_TARGET}`,
        "icon-xs": `size-6 rounded-md [&_svg:not([class*='size-'])]:size-3 ${TAP_TARGET}`,
        "icon-sm": `size-8 ${TAP_TARGET}`,
        "icon-lg": `size-10 ${TAP_TARGET}`,
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
)

function Button({
  className,
  variant = "default",
  size = "default",
  asChild = false,
  ...props
}: React.ComponentProps<"button"> &
  VariantProps<typeof buttonVariants> & {
    asChild?: boolean
  }) {
  const Comp = asChild ? Slot.Root : "button"

  return (
    <Comp
      data-slot="button"
      data-variant={variant}
      data-size={size}
      className={cn(buttonVariants({ variant, size, className }))}
      {...props}
    />
  )
}

export { Button, buttonVariants }
