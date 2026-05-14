import * as React from "react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { Kbd, KbdGroup } from "@/components/ui/kbd";
import { displayKey } from "@/lib/kbd-display";

export interface KbdTooltipProps {
  label: string;
  keys?: string[];
  side?: React.ComponentProps<typeof TooltipContent>["side"];
  align?: React.ComponentProps<typeof TooltipContent>["align"];
  children: React.ReactElement;
}

export function KbdTooltip({
  label,
  keys,
  side,
  align,
  children,
}: KbdTooltipProps) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>{children}</TooltipTrigger>
      <TooltipContent side={side} align={align} className="flex items-center gap-2">
        <span>{label}</span>
        {keys && keys.length > 0 && (
          <KbdGroup>
            {keys.map((k) => (
              <Kbd key={k}>{displayKey(k)}</Kbd>
            ))}
          </KbdGroup>
        )}
      </TooltipContent>
    </Tooltip>
  );
}
