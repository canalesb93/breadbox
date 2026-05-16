import { useMemo } from "react";
import { Infinity as InfinityIcon, MessageSquare, Plus, Shapes, X } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { TagChip } from "@/components/tag-chip";
import { DynamicIcon } from "@/lib/icon";
import { useCategories } from "@/api/queries/categories";
import { useTags } from "@/api/queries/tags";
import type { Category, Condition, RuleAction, Tag, TransactionRule } from "@/api/types";
import { RULE_FIELDS, categoryTileStyle, isMatchAll, opsFor } from "./rule-utils";

// Read-only condition display, used on the detail page.
export function RuleConditionsDisplay({ conditions }: { conditions: Condition }) {
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
// Resolves category and tag slugs to display labels once at this level so
// each action row reads from local maps instead of subscribing to the
// queries individually.
export function RuleActionsDisplay({ rule }: { rule: TransactionRule }) {
  const { data: categoryTree } = useCategories();
  const { data: tags } = useTags();
  const categoryBySlug = useMemo(() => indexCategoriesBySlug(categoryTree), [categoryTree]);
  const tagBySlug = useMemo(
    () => new Map((tags ?? []).map((t) => [t.slug, t])),
    [tags],
  );

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
        <ActionCard
          key={i}
          action={a}
          rule={rule}
          categoryBySlug={categoryBySlug}
          tagBySlug={tagBySlug}
        />
      ))}
    </div>
  );
}

function ActionCard({
  action,
  rule,
  categoryBySlug,
  tagBySlug,
}: {
  action: RuleAction;
  rule: TransactionRule;
  categoryBySlug: Map<string, { display: string }>;
  tagBySlug: Map<string, Tag>;
}) {
  switch (action.type) {
    case "set_category":
      return (
        <div className="bg-muted/40 flex items-center gap-3 rounded-xl p-3">
          <div
            className="flex size-8 items-center justify-center rounded-lg"
            style={categoryTileStyle(rule.category_color)}
          >
            {rule.category_icon ? (
              <DynamicIcon name={rule.category_icon} className="size-4" />
            ) : (
              <Shapes className="size-4" />
            )}
          </div>
          <p className="text-sm">
            Set category to{" "}
            <span className="font-semibold">
              {categoryBySlug.get(action.category_slug ?? "")?.display ??
                rule.category_display_name ??
                action.category_slug ??
                "?"}
            </span>
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
            Add tag <RuleTagChip slug={action.tag_slug} tagBySlug={tagBySlug} />
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
            Remove tag <RuleTagChip slug={action.tag_slug} tagBySlug={tagBySlug} />
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

// RuleTagChip resolves a slug against the cached tag catalog, then renders the
// shared inline `<TagChip>` so color tinting and density match every other
// tag rendering in the v2 SPA. The "?" badge fallback is reserved for the rare
// case where the rule action carries no slug at all (malformed legacy data).
function RuleTagChip({
  slug,
  tagBySlug,
}: {
  slug: string | undefined;
  tagBySlug: Map<string, Tag>;
}) {
  if (!slug) return <Badge variant="outline">?</Badge>;
  const tag = tagBySlug.get(slug) ?? {
    slug,
    display_name: slug,
    color: null,
    icon: null,
  };
  return <TagChip tag={tag} size="sm" className="align-middle" />;
}

// indexCategoriesBySlug walks the parent/children tree once into a Map keyed
// by slug, with the display value pre-rendered as "Parent › Child" for
// children and just "Parent" for top-level rows.
function indexCategoriesBySlug(
  tree: Category[] | undefined,
): Map<string, { display: string }> {
  const out = new Map<string, { display: string }>();
  for (const parent of tree ?? []) {
    out.set(parent.slug, { display: parent.display_name });
    for (const c of parent.children ?? []) {
      out.set(c.slug, { display: `${parent.display_name} › ${c.display_name}` });
    }
  }
  return out;
}
