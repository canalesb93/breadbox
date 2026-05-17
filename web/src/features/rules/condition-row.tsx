import { X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { flattenCategories, useCategories } from "@/api/queries/categories";
import {
  RULE_FIELDS,
  defaultOp,
  fieldType,
  opsFor,
  type ConditionRow,
} from "./rule-utils";

interface ConditionRowFieldsProps {
  index: number;
  logic: "and" | "or";
  totalRows: number;
  value: ConditionRow;
  onChange: (next: ConditionRow) => void;
  onRemove: () => void;
}

// One condition in the visual builder. Row layout mirrors v1's rule_form.templ
// pattern: IF/AND/OR label → field → op → value → remove. The value input
// switches shape based on the field type (string/numeric/bool/tags/category).
export function ConditionRowFields({
  index,
  logic,
  totalRows,
  value,
  onChange,
  onRemove,
}: ConditionRowFieldsProps) {
  const t = value.field ? fieldType(value.field) : "string";

  // Pre-fill operator with the field-type's default when the user picks a
  // field for the first time. Same behaviour as v1's onFieldChange.
  const setField = (next: string) => {
    const op = value.op && opsFor(next).some((o) => o.value === value.op)
      ? value.op
      : defaultOp(next);
    onChange({ ...value, field: next, op });
  };

  return (
    <div className="bg-muted/40 flex flex-wrap items-center gap-2 rounded-xl border p-2.5 sm:flex-nowrap">
      <div className="w-10 shrink-0 text-center">
        {index === 0 ? (
          <span className="text-muted-foreground/60 text-xs font-medium">IF</span>
        ) : (
          <span
            className={
              logic === "or"
                ? "text-xs font-medium text-amber-600 dark:text-amber-400"
                : "text-muted-foreground/60 text-xs font-medium"
            }
          >
            {logic.toUpperCase()}
          </span>
        )}
      </div>

      <Select value={value.field || undefined} onValueChange={setField}>
        <SelectTrigger className="bg-background h-8 min-w-0 flex-1">
          <SelectValue placeholder="Field…" />
        </SelectTrigger>
        <SelectContent>
          {groupedFieldOptions()}
        </SelectContent>
      </Select>

      {/* Mobile row break — same trick as v1: forces operator+value onto a
          second line so the field select gets the full width. Hidden on sm+. */}
      <div className="h-0 w-full basis-full sm:hidden" aria-hidden />

      <Select
        value={value.op || undefined}
        onValueChange={(op) => onChange({ ...value, op })}
        disabled={!value.field}
      >
        <SelectTrigger className="bg-background h-8 w-28 shrink-0">
          <SelectValue placeholder="Operator…" />
        </SelectTrigger>
        <SelectContent>
          {opsFor(value.field).map((o) => (
            <SelectItem key={o.value} value={o.value}>
              {o.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      <ValueInput
        type={t}
        field={value.field}
        op={value.op}
        value={value.value}
        onChange={(v) => onChange({ ...value, value: v })}
      />

      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="size-7 shrink-0"
            onClick={onRemove}
            aria-label={
              totalRows === 1
                ? "Remove (rule will match all transactions)"
                : "Remove condition"
            }
          >
            <X className="text-muted-foreground/60 size-3.5" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>
          {totalRows === 1
            ? "Remove (rule will match all transactions)"
            : "Remove condition"}
        </TooltipContent>
      </Tooltip>
    </div>
  );
}

function groupedFieldOptions() {
  // Group by `group` while keeping the declaration order. Renders as
  // <SelectGroup label>…</SelectGroup> sections inside the dropdown.
  const groups = new Map<string, typeof RULE_FIELDS>();
  for (const f of RULE_FIELDS) {
    const list = groups.get(f.group) ?? [];
    list.push(f);
    groups.set(f.group, list);
  }
  return Array.from(groups.entries()).map(([label, fields]) => (
    <SelectGroup key={label}>
      <SelectLabel>{label}</SelectLabel>
      {fields.map((f) => (
        <SelectItem key={f.value} value={f.value}>
          {f.label}
        </SelectItem>
      ))}
    </SelectGroup>
  ));
}

function ValueInput({
  type,
  field,
  op,
  value,
  onChange,
}: {
  type: ReturnType<typeof fieldType>;
  field: string;
  op: string;
  value: string;
  onChange: (v: string) => void;
}) {
  const { data: categories } = useCategories();
  const isCategoryEq = field === "category" && (op === "eq" || op === "neq");

  if (!field) {
    return (
      <Input
        disabled
        className="bg-muted h-8 min-w-0 flex-1 opacity-70"
        placeholder="value…"
      />
    );
  }
  if (type === "bool") {
    return (
      <Select value={value || "true"} onValueChange={onChange}>
        <SelectTrigger className="bg-background h-8 w-20 shrink-0">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="true">true</SelectItem>
          <SelectItem value="false">false</SelectItem>
        </SelectContent>
      </Select>
    );
  }
  if (type === "numeric") {
    return (
      <Input
        type="number"
        step="0.01"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="bg-background h-8 w-28 shrink-0"
        placeholder="0.00"
      />
    );
  }
  if (isCategoryEq) {
    const flat = flattenCategories(categories);
    return (
      <Select value={value || undefined} onValueChange={onChange}>
        <SelectTrigger className="bg-background h-8 min-w-0 flex-1">
          <SelectValue placeholder="Category…" />
        </SelectTrigger>
        <SelectContent>
          {flat.map((c) => (
            <SelectItem key={c.slug} value={c.slug}>
              {c.parent_display_name ? `${c.parent_display_name} › ` : ""}
              {c.display_name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    );
  }
  if (type === "tags") {
    return (
      <Input
        list="bb-tag-slugs"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="bg-background h-8 min-w-0 flex-1"
        placeholder={op === "in" ? "slug1, slug2, …" : "tag-slug"}
      />
    );
  }
  return (
    <Input
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className="bg-background h-8 min-w-0 flex-1"
      placeholder={
        op === "in" ? "value1, value2, …" : op === "matches" ? "regex…" : "value…"
      }
    />
  );
}
