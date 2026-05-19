import { X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useCategories } from "@/api/queries/categories";
import {
  ACTION_TYPES,
  TAG_SLUG_REGEX,
  type ActionField,
  type ActionRow,
} from "./rule-utils";

interface ActionRowFieldsProps {
  index: number;
  totalRows: number;
  value: ActionRow;
  /** Slugs already used by other category actions — disabled in this row. */
  usedActionFields: Set<ActionField>;
  onChange: (next: ActionRow) => void;
  onRemove: () => void;
}

// One action row in the visual builder. Mirrors v1's action layout: type
// select on the left, value input on the right, remove button at the end.
export function ActionRowFields({
  index,
  totalRows,
  value,
  usedActionFields,
  onChange,
  onRemove,
}: ActionRowFieldsProps) {
  const { data: categories } = useCategories();

  const setType = (field: string) => {
    onChange({ field: field as ActionField, value: "" });
  };

  // "Set category" is the only singleton action — repeating it is rejected
  // server-side. Disable it in this row's dropdown if another row already
  // owns it.
  const typeDisabled = (t: ActionField): boolean => {
    if (value.field === t) return false;
    return usedActionFields.has(t) && t === "category";
  };

  const tagInvalid =
    (value.field === "tag" || value.field === "tag_remove") &&
    !!value.value &&
    !TAG_SLUG_REGEX.test(value.value.trim());

  return (
    <div className="bg-muted/40 flex flex-wrap items-start gap-2 rounded-xl border p-2.5 sm:flex-nowrap">
      <Select value={value.field || undefined} onValueChange={setType}>
        <SelectTrigger className="bg-background h-8 min-w-0 flex-1 sm:w-40 sm:flex-none sm:shrink-0">
          <SelectValue placeholder="Action…" />
        </SelectTrigger>
        <SelectContent>
          {ACTION_TYPES.map((t) => (
            <SelectItem
              key={t.value}
              value={t.value}
              disabled={typeDisabled(t.value)}
            >
              {t.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="size-7 shrink-0 sm:order-last"
            onClick={onRemove}
            disabled={totalRows === 1}
            aria-label={
              totalRows === 1
                ? "A rule needs at least one action"
                : "Remove this action"
            }
          >
            <X className="text-muted-foreground/60 size-3.5" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>
          {totalRows === 1
            ? "A rule needs at least one action"
            : "Remove action"}
        </TooltipContent>
      </Tooltip>

      {/* Mobile row break — same trick as the condition row. */}
      <div className="order-3 h-0 w-full basis-full sm:hidden" aria-hidden />

      {!value.field && (
        <Input
          disabled
          className="bg-muted order-4 h-8 w-full min-w-0 opacity-70 sm:order-none sm:flex-1"
          placeholder="pick an action above…"
        />
      )}

      {value.field === "category" && (
        <Select
          value={value.value || undefined}
          onValueChange={(v) => onChange({ ...value, value: v })}
        >
          <SelectTrigger className="bg-background order-4 h-8 w-full min-w-0 sm:order-none sm:flex-1">
            <SelectValue placeholder="Select category…" />
          </SelectTrigger>
          <SelectContent>
            {(categories ?? []).flatMap((parent) => [
              <SelectItem key={parent.slug} value={parent.slug}>
                {parent.display_name}
              </SelectItem>,
              ...(parent.children ?? []).map((c) => (
                <SelectItem key={c.slug} value={c.slug}>
                  {parent.display_name} › {c.display_name}
                </SelectItem>
              )),
            ])}
          </SelectContent>
        </Select>
      )}

      {(value.field === "tag" || value.field === "tag_remove") && (
        <div className="order-4 w-full min-w-0 sm:order-none sm:flex-1">
          <Input
            list="bb-tag-slugs"
            value={value.value}
            onChange={(e) => onChange({ ...value, value: e.target.value })}
            className="bg-background h-8"
            placeholder="needs-review, subscription:monthly, …"
            pattern="^[a-z0-9][a-z0-9\-:]*[a-z0-9]$"
            autoCapitalize="none"
            autoCorrect="off"
            spellCheck={false}
            aria-invalid={tagInvalid}
            aria-describedby={`action-${index}-hint`}
          />
          <p
            id={`action-${index}-hint`}
            className={
              tagInvalid
                ? "text-destructive mt-1 text-[0.65rem]"
                : "text-muted-foreground mt-1 text-[0.65rem]"
            }
          >
            {tagInvalid
              ? "Lowercase letters, digits, hyphens and colons only."
              : value.field === "tag"
                ? "Lowercase, hyphens or colons. e.g. needs-review"
                : "Tag to remove from matching transactions."}
          </p>
        </div>
      )}

      {value.field === "comment" && (
        <div className="order-4 w-full min-w-0 sm:order-none sm:flex-1">
          <Textarea
            value={value.value}
            onChange={(e) => onChange({ ...value, value: e.target.value })}
            rows={2}
            className="bg-background min-h-16 text-xs"
            placeholder="Comment to attach to matching transactions…"
          />
          <p className="text-muted-foreground mt-1 text-[0.65rem]">
            Plain text. The comment is attributed to this rule.
          </p>
        </div>
      )}
    </div>
  );
}
