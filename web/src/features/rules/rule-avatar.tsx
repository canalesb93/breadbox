import { Pause, Shield, Zap } from "lucide-react";
import { DynamicIcon } from "@/lib/icon";
import { cn } from "@/lib/utils";
import type { TransactionRule } from "@/api/types";
import { categoryTileStyle } from "./rule-utils";

const SIZE: Record<"sm" | "lg", { box: string; icon: string }> = {
  sm: { box: "size-9 rounded-xl", icon: "size-5" },
  lg: { box: "size-12 rounded-xl", icon: "size-6" },
};

interface RuleAvatarProps {
  rule: TransactionRule;
  expired?: boolean;
  size?: "sm" | "lg";
}

// Picks one of five visual states for the rule avatar: category-coloured
// tile (enabled + not expired + has icon), expired clock, disabled pause,
// system shield, default zap. Used at row scale (sm) and detail-header
// scale (lg).
export function RuleAvatar({ rule, expired = false, size = "sm" }: RuleAvatarProps) {
  const { box, icon } = SIZE[size];
  const isSystem = rule.created_by_type === "system";

  if (rule.category_icon && rule.enabled && !expired) {
    return (
      <div
        className={cn("flex items-center justify-center", box)}
        style={categoryTileStyle(rule.category_color)}
      >
        <DynamicIcon name={rule.category_icon} className={icon} />
      </div>
    );
  }
  if (expired) {
    return (
      <div className={cn("flex items-center justify-center bg-amber-500/10 text-amber-600 dark:text-amber-400", box)}>
        <Pause className={icon} />
      </div>
    );
  }
  if (!rule.enabled) {
    return (
      <div className={cn("bg-muted text-muted-foreground/50 flex items-center justify-center", box)}>
        <Pause className={icon} />
      </div>
    );
  }
  if (isSystem) {
    return (
      <div className={cn("flex items-center justify-center bg-blue-500/10 text-blue-600 dark:text-blue-400", box)}>
        <Shield className={icon} />
      </div>
    );
  }
  return (
    <div className={cn("flex items-center justify-center bg-emerald-500/10 text-emerald-600 dark:text-emerald-400", box)}>
      <Zap className={icon} />
    </div>
  );
}
