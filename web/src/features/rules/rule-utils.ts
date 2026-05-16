import type { Condition, RuleAction } from "@/api/types";

// Field grammar — mirrors fieldTypes in static/js/admin/components/rule_form.js
// and the table at docs/rule-dsl.md §Fields. Only the subset surfaced in the
// visual builder is enumerated; power users drop down to JSON for the rest.
export type FieldType = "string" | "numeric" | "bool" | "tags";

interface FieldDef {
  value: string;
  label: string;
  type: FieldType;
  group: string;
}

// Field names match the canonical condition grammar in docs/rule-dsl.md
// (provider_name, provider_merchant_name, …). The labels are the
// human-friendly versions shown in the dropdown.
export const RULE_FIELDS: FieldDef[] = [
  { value: "provider_name", label: "Name", type: "string", group: "Transaction" },
  { value: "provider_merchant_name", label: "Merchant", type: "string", group: "Transaction" },
  { value: "amount", label: "Amount", type: "numeric", group: "Transaction" },
  { value: "pending", label: "Pending", type: "bool", group: "Transaction" },
  { value: "category", label: "Category (assigned)", type: "string", group: "Category" },
  { value: "provider_category_primary", label: "Category (raw primary)", type: "string", group: "Category" },
  { value: "provider_category_detailed", label: "Category (raw detail)", type: "string", group: "Category" },
  { value: "tags", label: "Tag", type: "tags", group: "Tags" },
  { value: "account_name", label: "Account name", type: "string", group: "Account / User" },
  { value: "user_name", label: "Family member", type: "string", group: "Account / User" },
  { value: "provider", label: "Provider", type: "string", group: "Account / User" },
];

export function fieldType(field: string): FieldType {
  return RULE_FIELDS.find((f) => f.value === field)?.type ?? "string";
}

interface OpOption {
  value: string;
  label: string;
}

const OPS_BY_TYPE: Record<FieldType, OpOption[]> = {
  string: [
    { value: "contains", label: "contains" },
    { value: "eq", label: "equals" },
    { value: "neq", label: "not equals" },
    { value: "not_contains", label: "not contains" },
    { value: "matches", label: "regex" },
    { value: "in", label: "in list" },
  ],
  numeric: [
    { value: "gte", label: "≥" },
    { value: "lte", label: "≤" },
    { value: "eq", label: "=" },
    { value: "gt", label: ">" },
    { value: "lt", label: "<" },
    { value: "neq", label: "≠" },
  ],
  bool: [
    { value: "eq", label: "is" },
    { value: "neq", label: "is not" },
  ],
  tags: [
    { value: "contains", label: "has" },
    { value: "not_contains", label: "does not have" },
    { value: "in", label: "has any of" },
  ],
};

export function opsFor(field: string): OpOption[] {
  if (!field) return [];
  return OPS_BY_TYPE[fieldType(field)];
}

export function defaultOp(field: string): string {
  const opts = opsFor(field);
  return opts[0]?.value ?? "eq";
}

// UI-side action types are aliased — "category" maps to set_category, etc.
// Mirrors actionTypes in rule_form.js.
export type ActionField = "category" | "tag" | "tag_remove" | "comment";

export const ACTION_TYPES: { value: ActionField; label: string }[] = [
  { value: "category", label: "Set category" },
  { value: "tag", label: "Add tag" },
  { value: "tag_remove", label: "Remove tag" },
  { value: "comment", label: "Add comment" },
];

// tagSlugRegex must match the server-side validator in internal/service/rules.go.
export const TAG_SLUG_REGEX = /^[a-z0-9][a-z0-9\-:]*[a-z0-9]$/;

// Pipeline-stage presets, copied from docs/rule-dsl.md §Priority as pipeline stage.
export interface StagePreset {
  value: number;
  label: string;
  stage: string;
  hint: string;
}
export const STAGE_PRESETS: StagePreset[] = [
  { value: 0, label: "Baseline", stage: "baseline", hint: "Runs first — broad defaults" },
  { value: 10, label: "Standard", stage: "standard", hint: "Default rule stage" },
  { value: 50, label: "Refinement", stage: "refinement", hint: "Runs after standard rules" },
  { value: 100, label: "Override", stage: "override", hint: "Runs last — wins set_category conflicts" },
];

export function stageForPriority(priority: number): StagePreset | undefined {
  return STAGE_PRESETS.find((p) => p.value === priority);
}

// Form-side condition row — flat shape the visual builder edits. Nested
// AND/OR trees can still round-trip via the JSON editor.
export interface ConditionRow {
  field: string;
  op: string;
  value: string;
}

export interface ActionRow {
  field: ActionField | "";
  value: string;
}

// formToConditions packs the visual-builder rows into the API's Condition
// shape. Empty rows array → match-all (zero-value Condition).
export function formToConditions(
  logic: "and" | "or",
  rows: ConditionRow[],
): Condition {
  const cleaned = rows.filter((r) => r.field);
  if (cleaned.length === 0) return {};
  if (cleaned.length === 1) return rowToLeaf(cleaned[0]);
  return logic === "or"
    ? { or: cleaned.map(rowToLeaf) }
    : { and: cleaned.map(rowToLeaf) };
}

function rowToLeaf(r: ConditionRow): Condition {
  const type = fieldType(r.field);
  let value: unknown = r.value;
  if (type === "numeric") {
    const n = Number(r.value);
    value = Number.isFinite(n) ? n : 0;
  } else if (type === "bool") {
    value = r.value === "true";
  } else if (r.op === "in") {
    // Split comma-separated input into an array — the API accepts a list of
    // values for `in`.
    value = r.value
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
  }
  return { field: r.field, op: r.op, value };
}

// conditionsToForm tries the inverse: pull a leaf or a single-level AND/OR
// out of an API Condition back into the visual rows. Nested combinators or
// `not` fall back to "json mode only" — the caller surfaces a notice.
export interface ParsedConditions {
  rows: ConditionRow[];
  logic: "and" | "or";
  /** True when the tree contains nested combinators the visual builder can't represent. */
  needsJson: boolean;
}

export function conditionsToForm(c: Condition | null | undefined): ParsedConditions {
  if (!c || isEmpty(c)) return { rows: [], logic: "and", needsJson: false };
  if (c.and) {
    const flat = c.and.every((sub) => isLeaf(sub));
    if (!flat) return { rows: [], logic: "and", needsJson: true };
    return { rows: c.and.map(leafToRow), logic: "and", needsJson: false };
  }
  if (c.or) {
    const flat = c.or.every((sub) => isLeaf(sub));
    if (!flat) return { rows: [], logic: "or", needsJson: true };
    return { rows: c.or.map(leafToRow), logic: "or", needsJson: false };
  }
  if (c.not) return { rows: [], logic: "and", needsJson: true };
  if (c.field) return { rows: [leafToRow(c)], logic: "and", needsJson: false };
  return { rows: [], logic: "and", needsJson: false };
}

function isLeaf(c: Condition): boolean {
  return !!c.field && !c.and && !c.or && !c.not;
}

function isEmpty(c: Condition): boolean {
  return !c.field && !c.and && !c.or && !c.not;
}

function leafToRow(c: Condition): ConditionRow {
  let value = "";
  if (Array.isArray(c.value)) value = c.value.join(", ");
  else if (c.value != null) value = String(c.value);
  return { field: c.field ?? "", op: c.op ?? "", value };
}

// formToActions maps the UI-side action rows back to the API's typed shape.
export function formToActions(rows: ActionRow[]): RuleAction[] {
  const out: RuleAction[] = [];
  for (const r of rows) {
    if (!r.field) continue;
    switch (r.field) {
      case "category":
        if (r.value) out.push({ type: "set_category", category_slug: r.value });
        break;
      case "tag":
        if (r.value) out.push({ type: "add_tag", tag_slug: r.value });
        break;
      case "tag_remove":
        if (r.value) out.push({ type: "remove_tag", tag_slug: r.value });
        break;
      case "comment":
        if (r.value) out.push({ type: "add_comment", content: r.value });
        break;
    }
  }
  return out;
}

export function actionsToForm(actions: RuleAction[]): ActionRow[] {
  return actions.map((a) => {
    switch (a.type) {
      case "set_category":
        return { field: "category", value: a.category_slug ?? "" };
      case "add_tag":
        return { field: "tag", value: a.tag_slug ?? "" };
      case "remove_tag":
        return { field: "tag_remove", value: a.tag_slug ?? "" };
      case "add_comment":
        return { field: "comment", value: a.content ?? "" };
      default:
        return { field: "", value: "" };
    }
  });
}

export function actionLabel(action: RuleAction): string {
  switch (action.type) {
    case "set_category":
      return `Set category to ${action.category_slug ?? "?"}`;
    case "add_tag":
      return `Add tag ${action.tag_slug ?? "?"}`;
    case "remove_tag":
      return `Remove tag ${action.tag_slug ?? "?"}`;
    case "add_comment":
      return `Add comment "${truncate(action.content ?? "", 60)}"`;
    default:
      return action.type;
  }
}

function truncate(s: string, max: number): string {
  return s.length > max ? `${s.slice(0, max - 1)}…` : s;
}

export function triggerLabel(trigger: string): string {
  switch (trigger) {
    case "on_create":
      return "On sync create";
    case "on_change":
    case "on_update":
      return "On sync change";
    case "always":
      return "Always (create or change)";
    default:
      return trigger || "On sync create";
  }
}

export function isMatchAll(c: Condition | null | undefined): boolean {
  return !c || isEmpty(c);
}

// Counts the visible leaves in a condition tree, treating nested combinators
// as a single "complex" group. Used for the list-row subtitle.
export function countConditions(c: Condition | null | undefined): number {
  if (!c) return 0;
  if (c.and) return c.and.length;
  if (c.or) return c.or.length;
  if (c.not) return 1;
  if (c.field) return 1;
  return 0;
}

// Human summary of a condition — "name contains uber" or "all transactions".
export function conditionSummary(c: Condition | null | undefined): string {
  if (isMatchAll(c)) return "All transactions";
  const parts: string[] = [];
  if (c!.and) {
    for (const sub of c!.and) parts.push(leafSummary(sub));
    return parts.join(" AND ");
  }
  if (c!.or) {
    for (const sub of c!.or) parts.push(leafSummary(sub));
    return parts.join(" OR ");
  }
  if (c!.not) return `NOT (${leafSummary(c!.not)})`;
  return leafSummary(c!);
}

function leafSummary(c: Condition): string {
  if (!c.field) return "(complex)";
  const field = RULE_FIELDS.find((f) => f.value === c.field)?.label ?? c.field;
  const op = OPS_BY_TYPE[fieldType(c.field)].find((o) => o.value === c.op)?.label ?? c.op;
  let val: string;
  if (Array.isArray(c.value)) val = c.value.join(", ");
  else if (c.value == null) val = "";
  else val = String(c.value);
  return `${field} ${op} ${val}`.trim();
}
