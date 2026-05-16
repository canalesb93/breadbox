import * as React from "react"
import { CheckIcon } from "lucide-react"
import { Checkbox as CheckboxPrimitive } from "radix-ui"

import { cn } from "@/lib/utils"

function Checkbox({
  className,
  ...props
}: React.ComponentProps<typeof CheckboxPrimitive.Root>) {
  return (
    <CheckboxPrimitive.Root
      data-slot="checkbox"
      className={cn(
        // Dark mode bumps: stock shadcn's `border-input` + `dark:bg-input/30`
        // is barely visible against `bg-card` (oklch 0.205) because
        // `--input` is `oklch(1 0 0 / 15%)` and 30% of that is ~4.5% white.
        // We override both the border (white / 30%) and the unchecked fill
        // (white / 8%) so unchecked rows in dark mode read as "this is a
        // checkbox, not a hairline". Checked state keeps the stock
        // `bg-primary` accent.
        "peer size-4 shrink-0 rounded-[4px] border border-input shadow-xs transition-shadow outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 data-[state=checked]:border-primary data-[state=checked]:bg-primary data-[state=checked]:text-primary-foreground dark:border-white/25 dark:bg-white/[0.04] dark:hover:bg-white/[0.07] dark:aria-invalid:ring-destructive/40 dark:data-[state=checked]:border-primary dark:data-[state=checked]:bg-primary",
        className
      )}
      {...props}
    >
      <CheckboxPrimitive.Indicator
        data-slot="checkbox-indicator"
        className="grid place-content-center text-current transition-none"
      >
        <CheckIcon className="size-3.5" />
      </CheckboxPrimitive.Indicator>
    </CheckboxPrimitive.Root>
  )
}

export { Checkbox }
