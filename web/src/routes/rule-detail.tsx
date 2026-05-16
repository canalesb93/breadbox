import { useState } from "react";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { Loader2, Pause, Pencil, Play, PlayCircle, Shield, Trash2, Zap } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { DynamicIcon } from "@/lib/icon";
import { formatRelativeTime } from "@/lib/format";
import { withMutationToast } from "@/lib/mutation-toast";
import {
  useApplyRule,
  useDeleteRule,
  useRule,
  useToggleRule,
} from "@/api/queries/rules";
import {
  RuleActionsDisplay,
  RuleConditionsDisplay,
} from "@/features/rules/rule-display";
import { stageForPriority, triggerLabel } from "@/features/rules/rule-utils";
import type { TransactionRule } from "@/api/types";

export function RuleDetailPage() {
  const navigate = useNavigate();
  const { id } = useParams({ strict: false }) as { id: string };

  const ruleQuery = useRule(id);
  const toggleRule = useToggleRule();
  const applyRule = useApplyRule();
  const deleteRule = useDeleteRule();

  const [confirmApply, setConfirmApply] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);

  if (ruleQuery.isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-16 rounded-xl" />
        <Skeleton className="h-40 rounded-xl" />
      </div>
    );
  }
  if (ruleQuery.isError) {
    return (
      <Alert variant="destructive">
        <AlertTitle>Couldn't load this rule</AlertTitle>
        <AlertDescription>
          {ruleQuery.error instanceof Error
            ? ruleQuery.error.message
            : "Try refreshing the page."}
        </AlertDescription>
      </Alert>
    );
  }
  const rule = ruleQuery.data;
  if (!rule) {
    return (
      <Alert>
        <AlertTitle>Rule not found</AlertTitle>
        <AlertDescription>
          <Button asChild variant="link" className="px-0">
            <Link to="/rules">Back to rules</Link>
          </Button>
        </AlertDescription>
      </Alert>
    );
  }

  const isSystem = rule.created_by_type === "system";
  const stage = stageForPriority(rule.priority);

  const onToggle = () =>
    withMutationToast(
      () =>
        toggleRule.mutateAsync({ id: rule.short_id, enabled: !rule.enabled }),
      {
        success: rule.enabled
          ? `Disabled rule "${rule.name}".`
          : `Enabled rule "${rule.name}".`,
      },
    );

  const onApplyConfirm = async () => {
    setConfirmApply(false);
    const ok = await withMutationToast(
      () => applyRule.mutateAsync(rule.short_id),
      {
        success: "Rule applied retroactively. Transactions are being updated.",
      },
    );
    if (ok) ruleQuery.refetch();
  };

  const onDeleteConfirm = async () => {
    setConfirmDelete(false);
    const ok = await withMutationToast(
      () => deleteRule.mutateAsync(rule.short_id),
      { success: `Deleted rule "${rule.name}".` },
    );
    if (ok) navigate({ to: "/rules" });
  };

  return (
    <>
      <RuleHeader rule={rule} />

      <div className="grid gap-4 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
        <div className="space-y-4">
          <section className="bg-card rounded-2xl border p-5">
            <h2 className="text-sm font-medium">What this rule does</h2>
            <div className="mt-4 space-y-4">
              <div>
                <h3 className="text-muted-foreground mb-2 text-xs tracking-wider uppercase">
                  When
                </h3>
                <RuleConditionsDisplay conditions={rule.conditions} />
              </div>
              <div>
                <h3 className="text-muted-foreground mb-2 text-xs tracking-wider uppercase">
                  Then
                </h3>
                <RuleActionsDisplay rule={rule} />
              </div>
            </div>
          </section>

          <section className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            <Stat
              label="Hits"
              value={rule.hit_count.toLocaleString()}
              hint={
                rule.last_hit_at
                  ? `Last ${formatRelativeTime(rule.last_hit_at)}`
                  : "Never matched"
              }
            />
            <Stat label="Trigger" value={triggerLabel(rule.trigger)} />
            <Stat
              label="Stage"
              value={stage?.label ?? String(rule.priority)}
              hint={`priority ${rule.priority}`}
            />
            <Stat
              label="Status"
              value={rule.enabled ? "Enabled" : "Disabled"}
              hint={ruleCreatedByLabel(rule.created_by_type)}
            />
          </section>
        </div>

        <aside className="space-y-3">
          <section className="bg-card space-y-3 rounded-2xl border p-4">
            <div>
              <h3 className="text-sm font-medium">Apply retroactively</h3>
              <p className="text-muted-foreground mt-1 text-xs">
                Re-evaluate this rule's conditions against every existing
                transaction. Respects category overrides.
              </p>
            </div>
            <Button
              type="button"
              className="w-full"
              variant="outline"
              onClick={() => setConfirmApply(true)}
              disabled={applyRule.isPending}
            >
              {applyRule.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <PlayCircle className="size-4" />
              )}
              Apply now
            </Button>
          </section>

          {!isSystem && (
            <section className="bg-card space-y-3 rounded-2xl border p-4">
              <div>
                <h3 className="text-sm font-medium">Delete rule</h3>
                <p className="text-muted-foreground mt-1 text-xs">
                  Removes the rule. Past actions stay on transactions; the
                  rule simply stops firing on future syncs.
                </p>
              </div>
              <Button
                type="button"
                variant="outline"
                className="text-destructive hover:text-destructive w-full"
                onClick={() => setConfirmDelete(true)}
              >
                <Trash2 className="size-4" />
                Delete rule
              </Button>
            </section>
          )}
        </aside>
      </div>

      <AlertDialog open={confirmApply} onOpenChange={setConfirmApply}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Apply "{rule.name}" retroactively?</AlertDialogTitle>
            <AlertDialogDescription>
              This evaluates the rule's conditions against every existing
              transaction in your household. Category changes respect the
              per-transaction override lock; tags and comments accumulate.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={onApplyConfirm}>
              Apply now
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={confirmDelete} onOpenChange={setConfirmDelete}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete "{rule.name}"?</AlertDialogTitle>
            <AlertDialogDescription>
              The rule stops firing on future syncs. Past actions it applied
              stay on transactions.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={onDeleteConfirm}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );

  function RuleHeader({ rule }: { rule: TransactionRule }) {
    return (
      <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-center gap-4">
          <RuleHeaderAvatar rule={rule} />
          <div className="space-y-1">
            <h1 className="text-2xl font-semibold tracking-tight">
              {rule.name}
            </h1>
            <div className="flex flex-wrap items-center gap-2">
              {rule.enabled ? (
                <Badge variant="secondary" className="bg-emerald-500/15 text-emerald-700 dark:text-emerald-400">
                  Enabled
                </Badge>
              ) : (
                <Badge variant="outline">Disabled</Badge>
              )}
              {isSystem && (
                <Badge variant="secondary" className="bg-blue-500/15 text-blue-700 dark:text-blue-400">
                  <Shield className="size-3" /> System
                </Badge>
              )}
              <span className="text-muted-foreground text-xs">
                {triggerLabel(rule.trigger)}
              </span>
              <span className="text-muted-foreground text-xs">·</span>
              <span className="text-muted-foreground text-xs">
                Priority {rule.priority}
              </span>
              <span className="text-muted-foreground text-xs">·</span>
              <span className="text-muted-foreground text-xs">
                {ruleCreatedByLabel(rule.created_by_type)}
              </span>
              {rule.expires_at && (
                <>
                  <span className="text-muted-foreground text-xs">·</span>
                  <span className="text-xs text-amber-600 dark:text-amber-400">
                    Expires{" "}
                    {new Date(rule.expires_at).toLocaleDateString()}
                  </span>
                </>
              )}
            </div>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Button asChild variant="outline" size="sm">
            <Link to="/rules/$id/edit" params={{ id: rule.short_id }}>
              <Pencil className="size-3.5" /> Edit
            </Link>
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={onToggle}
            disabled={toggleRule.isPending}
          >
            {rule.enabled ? (
              <>
                <Pause className="size-3.5" /> Disable
              </>
            ) : (
              <>
                <Play className="size-3.5" /> Enable
              </>
            )}
          </Button>
        </div>
      </div>
    );
  }
}

function RuleHeaderAvatar({ rule }: { rule: TransactionRule }) {
  const isSystem = rule.created_by_type === "system";
  if (rule.category_icon && rule.enabled) {
    return (
      <div
        className="flex size-12 items-center justify-center rounded-xl"
        style={{
          backgroundColor: rule.category_color
            ? `color-mix(in oklab, ${rule.category_color} 18%, transparent)`
            : "var(--muted)",
          color: rule.category_color ?? undefined,
        }}
      >
        <DynamicIcon name={rule.category_icon} className="size-6" />
      </div>
    );
  }
  if (!rule.enabled) {
    return (
      <div className="bg-muted text-muted-foreground/60 flex size-12 items-center justify-center rounded-xl">
        <Pause className="size-6" />
      </div>
    );
  }
  if (isSystem) {
    return (
      <div className="flex size-12 items-center justify-center rounded-xl bg-blue-500/10 text-blue-600 dark:text-blue-400">
        <Shield className="size-6" />
      </div>
    );
  }
  return (
    <div className="flex size-12 items-center justify-center rounded-xl bg-emerald-500/10 text-emerald-600 dark:text-emerald-400">
      <Zap className="size-6" />
    </div>
  );
}

function Stat({
  label,
  value,
  hint,
}: {
  label: string;
  value: string;
  hint?: string;
}) {
  return (
    <div className="bg-card rounded-xl border p-3">
      <p className="text-muted-foreground text-xs">{label}</p>
      <p className="mt-1 text-base font-semibold">{value}</p>
      {hint && (
        <p className="text-muted-foreground/70 mt-0.5 text-xs">{hint}</p>
      )}
    </div>
  );
}

function ruleCreatedByLabel(actor: string): string {
  switch (actor) {
    case "user":
      return "Created by user";
    case "agent":
      return "Created by agent";
    case "system":
      return "System rule";
    default:
      return actor;
  }
}
