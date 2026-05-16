import { Infinity as InfinityIcon, MessageSquare, Plus, Shapes, Tag as TagIcon, X } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { DynamicIcon } from "@/lib/icon";
import { useCategories } from "@/api/queries/categories";
import { useTags } from "@/api/queries/tags";
import type { Condition, RuleAction, TransactionRule } from "@/api/types";
import { RULE_FIELDS, isMatchAll, opsFor } from "./rule-utils";

// RuleConditionsDisplay is the read-only counterpart to ConditionRowFields —
// used on the detail page to show what the rule matches without the form
// machinery.
export function RuleConditionsDisplay({
  conditions,
}: {
  conditions: Condition;
}) {
  if (isMatchAll(conditions)) {
    return (
      <div className="bg-blue-500/5 border-blue-500/20 flex items-center gap-3 rounded-xl border p-3">
        <div className="bg-blue-500/10 text-blue-600 dark:text-blue-400 flex size-8 items-center justify-center rounded-lg">
          <InfinityIcon className="size-4" />
        </div>
        <div className="min-w-0 flex-1">
          <p className="text-sm font-medium">All transactions</p>
          <p className="text-muted-foreground text-xs">
            No conditions — this rule fires on every transaction.
          </p>
        </div>
      </div>
    );
  }

  // Single-level AND/OR or a single leaf are the only shapes the visual
  // builder can fully render; fall back to a code block otherwise.
  if (conditions.and) {
    return <ConditionRowList rows={conditions.and} conj="AND" />;
  }
  if (conditions.or) {
    return <ConditionRowList rows={conditions.or} conj="OR" />;
  }
  if (conditions.field) {
    return <ConditionRowList rows={[conditions]} conj="" />;
  }
  return (
    <pre className="bg-muted/50 overflow-x-auto rounded-xl border p-3 font-mono text-xs">
      {JSON.stringify(conditions, null, 2)}
    </pre>
  );
}

function ConditionRowList({ rows, conj }: { rows: Condition[]; conj: string }) {
  return (
    <div className="space-y-1.5">
      {rows.map((row, i) => (
        <div
          key={i}
          className="bg-muted/40 flex items-center gap-3 rounded-xl border p-3"
        >
          <span className="text-muted-foreground/60 w-10 shrink-0 text-center text-xs font-medium">
            {i === 0 ? "IF" : conj}
          </span>
          <span className="text-sm">{prettyCondition(row)}</span>
        </div>
      ))}
    </div>
  );
}

function prettyCondition(c: Condition): string {
  if (!c.field) return "(complex)";
  const label = RULE_FIELDS.find((f) => f.value === c.field)?.label ?? c.field;
  const opLabel =
    opsFor(c.field).find((o) => o.value === c.op)?.label ?? c.op ?? "?";
  let val: string;
  if (Array.isArray(c.value)) val = c.value.join(", ");
  else if (typeof c.value === "boolean") val = c.value ? "true" : "false";
  else if (c.value == null) val = "";
  else val = String(c.value);
  return `${label} ${opLabel} ${val}`.trim();
}

// RuleActionsDisplay shows the rule's effects as a list of action cards.
export function RuleActionsDisplay({ rule }: { rule: TransactionRule }) {
  if (rule.actions.length === 0) {
    return (
      <p className="text-muted-foreground py-3 text-sm">
        This rule has no actions configured.
      </p>
    );
  }
  return (
    <div className="space-y-1.5">
      {rule.actions.map((a, i) => (
        <ActionCard key={i} action={a} rule={rule} />
      ))}
    </div>
  );
}

function ActionCard({
  action,
  rule,
}: {
  action: RuleAction;
  rule: TransactionRule;
}) {
  switch (action.type) {
    case "set_category":
      return (
        <div className="bg-muted/40 flex items-center gap-3 rounded-xl p-3">
          <div
            className="flex size-8 items-center justify-center rounded-lg"
            style={{
              backgroundColor: rule.category_color
                ? `color-mix(in oklab, ${rule.category_color} 18%, transparent)`
                : "var(--muted)",
              color: rule.category_color ?? undefined,
            }}
          >
            {rule.category_icon ? (
              <DynamicIcon name={rule.category_icon} className="size-4" />
            ) : (
              <Shapes className="size-4" />
            )}
          </div>
          <p className="text-sm">
            Set category to{" "}
            <CategoryName slug={action.category_slug} fallback={rule.category_display_name} />
          </p>
        </div>
      );
    case "add_tag":
      return (
        <div className="bg-muted/40 flex items-center gap-3 rounded-xl p-3">
          <div className="bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 flex size-8 items-center justify-center rounded-lg">
            <Plus className="size-4" />
          </div>
          <p className="text-sm">
            Add tag <TagChip slug={action.tag_slug} />
          </p>
        </div>
      );
    case "remove_tag":
      return (
        <div className="bg-muted/40 flex items-center gap-3 rounded-xl p-3">
          <div className="bg-rose-500/10 text-rose-600 dark:text-rose-400 flex size-8 items-center justify-center rounded-lg">
            <X className="size-4" />
          </div>
          <p className="text-sm">
            Remove tag <TagChip slug={action.tag_slug} />
          </p>
        </div>
      );
    case "add_comment":
      return (
        <div className="bg-muted/40 flex items-start gap-3 rounded-xl p-3">
          <div className="bg-muted text-muted-foreground flex size-8 shrink-0 items-center justify-center rounded-lg">
            <MessageSquare className="size-4" />
          </div>
          <div className="min-w-0 flex-1 space-y-1">
            <p className="text-muted-foreground text-xs">Add comment</p>
            <p className="text-foreground text-sm">{action.content}</p>
          </div>
        </div>
      );
    default:
      return (
        <div className="bg-muted/40 flex items-center gap-3 rounded-xl p-3">
          <p className="text-sm">{action.type}</p>
        </div>
      );
  }
}

function CategoryName({
  slug,
  fallback,
}: {
  slug: string | undefined;
  fallback?: string | null;
}) {
  const { data: tree } = useCategories();
  if (!slug) return <span className="font-semibold">{fallback ?? "?"}</span>;
  for (const parent of tree ?? []) {
    if (parent.slug === slug) return <span className="font-semibold">{parent.display_name}</span>;
    for (const c of parent.children ?? []) {
      if (c.slug === slug) {
        return (
          <span className="font-semibold">
            {parent.display_name} › {c.display_name}
          </span>
        );
      }
    }
  }
  return <span className="font-semibold">{fallback ?? slug}</span>;
}

function TagChip({ slug }: { slug: string | undefined }) {
  const { data: tags } = useTags();
  if (!slug) return <Badge variant="outline">?</Badge>;
  const tag = (tags ?? []).find((t) => t.slug === slug);
  return (
    <Badge variant="outline" className="gap-1">
      {tag?.icon && <DynamicIcon name={tag.icon} className="size-3" />}
      <span>{tag?.display_name ?? slug}</span>
      <TagIcon className="text-muted-foreground/60 size-3" />
    </Badge>
  );
}
