import { Link } from "@tanstack/react-router";
import { Infinity as InfinityIcon, ListFilter, MoreVertical, Pause, Pencil, Play, Shield, Trash2, Zap } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { formatRelativeTime } from "@/lib/format";
import type { TransactionRule } from "@/api/types";
import { RuleAvatar } from "./rule-avatar";
import { countConditions, isMatchAll, stageForPriority } from "./rule-utils";

export interface RuleRowProps {
  rule: TransactionRule;
  onToggle: (rule: TransactionRule) => void;
  onDelete: (rule: TransactionRule) => void;
}

export function RuleRow({ rule, onToggle, onDelete }: RuleRowProps) {
  const isSystem = rule.created_by_type === "system";
  const expired = ruleExpired(rule);
  const stage = stageForPriority(rule.priority);
  const matchAll = isMatchAll(rule.conditions);
  const conditionCount = countConditions(rule.conditions);
  const actionsCount = rule.actions.length;

  return (
    <div
      className={cn(
        "bg-card hover:bg-muted/30 focus-within:bg-muted/30 relative rounded-xl border transition-colors",
        !rule.enabled && "opacity-60",
      )}
    >
      <Link
        to="/rules/$id"
        params={{ id: rule.short_id }}
        // pr-20 reserves space on the right for the absolutely-positioned
        // action cluster so the lg stats columns don't tuck behind it.
        className="flex items-center gap-3 px-4 py-4 pr-20 sm:gap-4 sm:px-5 sm:py-5 sm:pr-24"
        aria-label={`Open rule ${rule.name}`}
      >
        <RuleAvatar rule={rule} expired={expired} />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="line-clamp-2 text-sm font-semibold sm:line-clamp-none sm:truncate">
              {rule.name}
            </span>
            {isSystem && (
              <Badge variant="secondary" className="hidden sm:inline-flex">
                <Shield className="size-3" />
                System
              </Badge>
            )}
            {rule.enabled && expired && (
              <Badge variant="outline" className="hidden border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-400 sm:inline-flex">
                Expired
              </Badge>
            )}
            {!rule.enabled && (
              <Badge variant="outline" className="hidden sm:inline-flex">
                Disabled
              </Badge>
            )}
          </div>
          <div className="text-muted-foreground mt-0.5 flex items-center gap-2 overflow-hidden text-xs whitespace-nowrap">
            <span className="inline-flex shrink-0 items-center gap-1" title={matchAll ? "Match-all rule" : `${conditionCount} conditions`}>
              {matchAll ? (
                <>
                  <InfinityIcon className="size-3" />
                  <span className="italic">All</span>
                </>
              ) : (
                <>
                  <ListFilter className="size-3" />
                  <span className="tabular-nums">{conditionCount} cond.</span>
                </>
              )}
            </span>
            <span className="text-muted-foreground/60 shrink-0">·</span>
            <span className="inline-flex shrink-0 items-center gap-1 tabular-nums">
              <Zap className="size-3" />
              {actionsCount} action{actionsCount === 1 ? "" : "s"}
            </span>
            <span className="text-muted-foreground/60 shrink-0">·</span>
            <span className="shrink-0 tabular-nums">{rule.hit_count} hits</span>
          </div>
        </div>
        <div className="text-muted-foreground hidden shrink-0 items-center gap-4 text-xs lg:flex">
          <div className="w-16 text-center">
            <div className="text-foreground/70 text-sm font-semibold tabular-nums">
              {stage?.label ?? rule.priority}
            </div>
            <div>stage</div>
          </div>
          <div className="w-20 text-center">
            {rule.last_hit_at ? (
              <>
                <div className="text-foreground/70 text-xs font-medium">
                  {formatRelativeTime(rule.last_hit_at)}
                </div>
                <div>last active</div>
              </>
            ) : (
              <>
                <div className="text-foreground/40 text-xs font-medium">—</div>
                <div>last active</div>
              </>
            )}
          </div>
        </div>
      </Link>

      {/* Sibling action cluster — absolutely positioned over the Link so
          clicks land on the buttons before bubbling into the navigation. */}
      <div className="absolute top-1/2 right-3 flex -translate-y-1/2 items-center gap-0.5">
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              asChild
              variant="ghost"
              size="icon"
              className="size-7"
              aria-label={`Edit rule ${rule.name}`}
            >
              <Link to="/rules/$id/edit" params={{ id: rule.short_id }}>
                <Pencil className="size-3.5" />
              </Link>
            </Button>
          </TooltipTrigger>
          <TooltipContent>Edit rule</TooltipContent>
        </Tooltip>
        <DropdownMenu>
          <Tooltip>
            <TooltipTrigger asChild>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-7"
                  aria-label={`Rule ${rule.name} actions`}
                >
                  <MoreVertical className="size-4" />
                </Button>
              </DropdownMenuTrigger>
            </TooltipTrigger>
            <TooltipContent>Rule actions</TooltipContent>
          </Tooltip>
          <DropdownMenuContent align="end" className="w-44">
            <DropdownMenuItem onSelect={() => onToggle(rule)}>
              {rule.enabled ? (
                <>
                  <Pause className="size-3.5" /> Disable
                </>
              ) : (
                <>
                  <Play className="size-3.5" /> Enable
                </>
              )}
            </DropdownMenuItem>
            {!isSystem && (
              <DropdownMenuItem
                onSelect={() => onDelete(rule)}
                className="text-destructive focus:text-destructive"
              >
                <Trash2 className="size-3.5" /> Delete
              </DropdownMenuItem>
            )}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </div>
  );
}

function ruleExpired(rule: TransactionRule): boolean {
  if (!rule.expires_at) return false;
  const t = new Date(rule.expires_at).getTime();
  return Number.isFinite(t) && t < Date.now();
}
