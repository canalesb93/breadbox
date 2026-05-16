import { useMemo, useState } from "react";
import { useFieldArray, useForm, type UseFormReturn } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { AlertCircle, AlertTriangle, ChevronDown, Code2, Infinity as InfinityIcon, ListFilter, Loader2, Plus, Save, Wand2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { StatusPanel } from "@/components/status-panel";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { useTags } from "@/api/queries/tags";
import { cn } from "@/lib/utils";
import type { Condition, TransactionRule } from "@/api/types";
import { ConditionRowFields } from "./condition-row";
import { ActionRowFields } from "./action-row";
import { PreviewPanel } from "./preview-panel";
import {
  STAGE_PRESETS,
  TAG_SLUG_REGEX,
  actionsToForm,
  conditionsToForm,
  formToActions,
  formToConditions,
  isMatchAll,
  stageForPriority,
  type ActionField,
  type ActionRow,
  type ConditionRow,
} from "./rule-utils";

// Form-level zod schema — keeps the rules surface explicit. Per-row
// validation (tag slug regex, action.value non-empty) is checked in a
// follow-up superRefine so we can attach errors to the right index.
const ruleFormSchema = z
  .object({
    name: z.string().min(1, "Name is required").max(200),
    trigger: z.enum(["on_create", "on_change", "always"]),
    priority: z.number().int().min(0).max(1000),
    logic: z.enum(["and", "or"]),
    conditions: z.array(
      z.object({
        field: z.string(),
        op: z.string(),
        value: z.string(),
      }),
    ),
    actions: z.array(
      z.object({
        field: z.string(),
        value: z.string(),
      }),
    ),
    conditionsJson: z.string().optional(),
    useJsonEditor: z.boolean(),
  })
  .superRefine((values, ctx) => {
    if (values.actions.length === 0) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ["actions"],
        message: "Add at least one action.",
      });
    }
    // Per-action validation. Picked field but empty value → reject.
    let categorySeen = false;
    values.actions.forEach((a, i) => {
      if (!a.field) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ["actions", i, "field"],
          message: "Pick an action type.",
        });
        return;
      }
      if (!a.value || !a.value.trim()) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ["actions", i, "value"],
          message: "Value is required.",
        });
        return;
      }
      if (a.field === "category" && categorySeen) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ["actions", i, "field"],
          message: "Only one 'Set category' action allowed.",
        });
      }
      if (a.field === "category") categorySeen = true;
      if (
        (a.field === "tag" || a.field === "tag_remove") &&
        !TAG_SLUG_REGEX.test(a.value.trim())
      ) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ["actions", i, "value"],
          message: "Lowercase letters, digits, hyphens and colons only.",
        });
      }
    });
    // Per-condition: field picked but op/value missing.
    values.conditions.forEach((c, i) => {
      if (!c.field) return; // empty rows = ignored on submit
      if (!c.op) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ["conditions", i, "op"],
          message: "Pick an operator.",
        });
      }
      // tags / bool / numeric / string all need *something* — even `in` with
      // a single value is fine, so just guard against empty.
      if (c.field !== "pending" && (!c.value || !String(c.value).trim())) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ["conditions", i, "value"],
          message: "Value is required.",
        });
      }
    });
    if (values.useJsonEditor && values.conditionsJson) {
      try {
        JSON.parse(values.conditionsJson);
      } catch {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ["conditionsJson"],
          message: "Invalid JSON.",
        });
      }
    }
  });

export type RuleFormValues = z.infer<typeof ruleFormSchema>;

export interface RuleFormSubmit {
  name: string;
  trigger: string;
  priority: number;
  conditions: Condition;
  actions: ReturnType<typeof formToActions>;
}

interface RuleFormProps {
  /** Existing rule when editing — initializes the form. */
  initialRule?: TransactionRule;
  submitting?: boolean;
  submitLabel?: string;
  onSubmit: (values: RuleFormSubmit) => void;
  onCancel: () => void;
}

// RuleForm is the shared create/edit body — pure presentation that bubbles
// the parsed submit payload up to the route, which owns the mutation.
export function RuleForm({
  initialRule,
  submitting,
  submitLabel = "Create rule",
  onSubmit,
  onCancel,
}: RuleFormProps) {
  const initial = useMemo(
    () => buildInitialValues(initialRule),
    [initialRule],
  );
  const form = useForm<RuleFormValues>({
    resolver: zodResolver(ruleFormSchema),
    defaultValues: initial,
    mode: "onSubmit",
  });

  const conditions = useFieldArray<RuleFormValues, "conditions">({
    control: form.control,
    name: "conditions",
  });
  const actions = useFieldArray<RuleFormValues, "actions">({
    control: form.control,
    name: "actions",
  });

  const watchedConditions = form.watch("conditions");
  const watchedActions = form.watch("actions");
  const watchedLogic = form.watch("logic");
  const watchedPriority = form.watch("priority");
  const watchedTrigger = form.watch("trigger");
  const useJsonEditor = form.watch("useJsonEditor");
  const conditionsJson = form.watch("conditionsJson");

  const stagePreset = stageForPriority(watchedPriority);
  const [stageOpen, setStageOpen] = useState(
    !!initialRule && watchedPriority !== 10,
  );

  // Used action types across actions[] — drives the disabled state of the
  // "Set category" option in each row's type select so only one category
  // action exists at a time.
  const usedActionFields = useMemo(() => {
    const s = new Set<ActionField>();
    for (const a of watchedActions) if (a.field) s.add(a.field as ActionField);
    return s;
  }, [watchedActions]);

  // Live tags datalist — autocomplete for tag-slug inputs in both conditions
  // and actions. Sourced from the v2 tags query so the form picks up new tags
  // the moment they're created.
  const { data: tags } = useTags();

  // The live preview reads the parsed conditions from the form state. We
  // parse from either the visual builder OR the JSON editor depending on
  // which one is active. The conditions are also what we submit.
  const parsedConditions = useMemo<Condition>(() => {
    if (useJsonEditor && conditionsJson) {
      try {
        return JSON.parse(conditionsJson) as Condition;
      } catch {
        return {};
      }
    }
    return formToConditions(watchedLogic, watchedConditions);
  }, [useJsonEditor, conditionsJson, watchedLogic, watchedConditions]);

  // Warnings about action combos. Same checks v1 surfaces inline.
  const comboWarnings = useMemo(() => {
    const out: string[] = [];
    const tagAdds = watchedActions
      .filter((a) => a.field === "tag")
      .map((a) => a.value.trim().toLowerCase());
    const tagRemoves = watchedActions
      .filter((a) => a.field === "tag_remove")
      .map((a) => a.value.trim().toLowerCase());
    for (const slug of tagAdds) {
      if (slug && tagRemoves.includes(slug)) {
        out.push(
          `Adding and removing the same tag "${slug}" cancels out — neither will apply.`,
        );
      }
    }
    const hasComment = watchedActions.some((a) => a.field === "comment");
    if (hasComment && watchedTrigger === "always") {
      out.push(
        `"Add comment" + "Always" trigger duplicates comments on every sync. Prefer "On sync create".`,
      );
    }
    const hasCategory = watchedActions.some((a) => a.field === "category");
    if (isMatchAll(parsedConditions) && hasCategory) {
      out.push(
        "Set category with no conditions will reclassify every transaction on every sync.",
      );
    }
    return out;
  }, [watchedActions, watchedTrigger, parsedConditions]);

  const submitHandler = form.handleSubmit((values) => {
    const conditionsPayload = useJsonEditor
      ? safeParseConditions(values.conditionsJson ?? "{}")
      : formToConditions(values.logic, values.conditions);
    onSubmit({
      name: values.name.trim(),
      trigger: values.trigger,
      priority: values.priority,
      conditions: conditionsPayload,
      actions: formToActions(values.actions as ActionRow[]),
    });
  });

  return (
    <div className="grid gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
      <Form {...form}>
        <form
          onSubmit={submitHandler}
          className="bg-card overflow-hidden rounded-2xl border"
        >
          <div className="space-y-4 p-5 sm:p-6">
            <div className="flex items-center gap-3">
              <div className="bg-primary/10 text-primary flex size-10 items-center justify-center rounded-xl">
                <ListFilter className="size-5" />
              </div>
              <div>
                <h2 className="text-sm font-semibold">Rule details</h2>
                <p className="text-muted-foreground text-xs">
                  Pick when it fires and what it does.
                </p>
              </div>
            </div>
            <FormField
              control={form.control}
              name="name"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Rule name</FormLabel>
                  <FormControl>
                    <Input
                      placeholder="e.g., Uber Eats → Food Delivery"
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <div className="space-y-3 border-t p-5 sm:p-6">
            <div>
              <h3 className="text-muted-foreground text-xs font-semibold tracking-wider uppercase">
                When
              </h3>
              <p className="text-muted-foreground mt-1 text-xs">
                Which sync events trigger this rule.
              </p>
            </div>
            <FormField
              control={form.control}
              name="trigger"
              render={({ field }) => (
                <FormItem>
                  <FormControl>
                    <Select value={field.value} onValueChange={field.onChange}>
                      <SelectTrigger className="h-9">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="on_create">On sync create</SelectItem>
                        <SelectItem value="on_change">On sync change</SelectItem>
                        <SelectItem value="always">On create or change</SelectItem>
                      </SelectContent>
                    </Select>
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <Collapsible open={stageOpen} onOpenChange={setStageOpen}>
              <CollapsibleTrigger asChild>
                <button
                  type="button"
                  className="hover:bg-muted/40 focus-visible:ring-ring/50 focus-visible:ring-[3px] focus-visible:outline-none group -mx-2 flex w-full items-center justify-between gap-3 rounded-lg px-2 py-1.5 text-left text-xs transition-colors"
                >
                  <span className="text-muted-foreground inline-flex items-center gap-2">
                    Pipeline stage
                    <span className="text-muted-foreground/60">
                      · {stagePreset?.label ?? watchedPriority}
                    </span>
                  </span>
                  <ChevronDown
                    className={`text-muted-foreground/50 size-3.5 transition-transform ${stageOpen ? "rotate-180" : ""}`}
                  />
                </button>
              </CollapsibleTrigger>
              <CollapsibleContent className="border-border/60 mt-3 ml-2 border-l pl-4">
                <FormField
                  control={form.control}
                  name="priority"
                  render={({ field }) => (
                    <FormItem>
                      <FormControl>
                        <div className="bg-muted/60 flex items-center gap-1 rounded-xl p-1">
                          {STAGE_PRESETS.map((preset) => (
                            <button
                              key={preset.value}
                              type="button"
                              onClick={() => field.onChange(preset.value)}
                              title={preset.hint}
                              className={cn(
                                "focus-visible:ring-ring/50 focus-visible:ring-[3px] focus-visible:outline-none flex-1 rounded-lg px-2 py-1 text-xs font-medium",
                                field.value === preset.value
                                  ? "bg-background text-foreground shadow-sm"
                                  : "text-muted-foreground hover:text-foreground transition-colors",
                              )}
                            >
                              {preset.label}
                            </button>
                          ))}
                        </div>
                      </FormControl>
                      <div className="mt-2 flex items-center justify-between gap-2">
                        <p className="text-muted-foreground min-w-0 truncate text-xs">
                          {stagePreset?.hint ?? "Custom priority."}
                        </p>
                        <Input
                          type="number"
                          min={0}
                          max={1000}
                          value={field.value}
                          onChange={(e) =>
                            field.onChange(Number(e.target.value) || 0)
                          }
                          className="bg-muted/50 h-7 w-20 shrink-0"
                        />
                      </div>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <p className="text-muted-foreground mt-2 text-xs">
                  Lower stages run first. Higher stages win <code>set_category</code>{" "}
                  conflicts; tags accumulate across stages.
                </p>
              </CollapsibleContent>
            </Collapsible>
          </div>

          <div className="space-y-3 border-t p-5 sm:p-6">
            <div className="flex items-start justify-between gap-3">
              <div>
                <h3 className="text-muted-foreground text-xs font-semibold tracking-wider uppercase">
                  Match
                </h3>
                <p className="text-muted-foreground mt-1 text-xs">
                  <MatchSubtitle
                    rowCount={watchedConditions.length}
                    logic={watchedLogic}
                  />
                </p>
              </div>
              <div className="flex items-center gap-2">
                {watchedConditions.length > 1 && (
                  <div className="bg-muted/60 flex items-center gap-1 rounded-lg p-0.5">
                    <button
                      type="button"
                      onClick={() => form.setValue("logic", "and")}
                      className={cn(
                        "focus-visible:ring-ring/50 focus-visible:ring-[3px] focus-visible:outline-none rounded-md px-2.5 py-0.5 text-xs font-medium",
                        watchedLogic === "and"
                          ? "bg-background text-foreground shadow-sm"
                          : "text-muted-foreground hover:text-foreground transition-colors",
                      )}
                    >
                      AND
                    </button>
                    <button
                      type="button"
                      onClick={() => form.setValue("logic", "or")}
                      className={cn(
                        "focus-visible:ring-ring/50 focus-visible:ring-[3px] focus-visible:outline-none rounded-md px-2.5 py-0.5 text-xs font-medium",
                        watchedLogic === "or"
                          ? "bg-background text-foreground shadow-sm"
                          : "text-muted-foreground hover:text-foreground transition-colors",
                      )}
                    >
                      OR
                    </button>
                  </div>
                )}
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() =>
                    conditions.append({ field: "", op: "", value: "" })
                  }
                >
                  <Plus className="size-3.5" /> Add
                </Button>
              </div>
            </div>

            {watchedConditions.length === 0 ? (
              <div className="bg-blue-500/5 border-blue-500/20 flex items-center gap-3 rounded-xl border p-3">
                <div className="bg-blue-500/10 text-blue-600 dark:text-blue-400 flex size-8 items-center justify-center rounded-lg">
                  <InfinityIcon className="size-4" />
                </div>
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium">All transactions</p>
                  <p className="text-muted-foreground text-xs">
                    This rule will apply to every transaction. Use sparingly.
                  </p>
                </div>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() =>
                    conditions.append({ field: "", op: "", value: "" })
                  }
                >
                  <Plus className="size-3" /> Add condition
                </Button>
              </div>
            ) : (
              <div className="space-y-2">
                {conditions.fields.map((field, idx) => (
                  <FormField
                    key={field.id}
                    control={form.control}
                    name={`conditions.${idx}` as const}
                    render={({ field: rhfField }) => (
                      <FormItem>
                        <FormControl>
                          <ConditionRowFields
                            index={idx}
                            logic={watchedLogic}
                            totalRows={conditions.fields.length}
                            value={rhfField.value as ConditionRow}
                            onChange={(next) => rhfField.onChange(next)}
                            onRemove={() => conditions.remove(idx)}
                          />
                        </FormControl>
                        {/* Per-row error message — accumulated from .op / .value
                            issues in the schema's superRefine. */}
                        <RowError errors={getRowErrors(form, "conditions", idx)} />
                      </FormItem>
                    )}
                  />
                ))}
              </div>
            )}

            <FormField
              control={form.control}
              name="useJsonEditor"
              render={({ field }) => (
                <div className="space-y-2">
                  <button
                    type="button"
                    onClick={() => {
                      // Switching INTO JSON mode — serialize the current
                      // visual builder state so the editor opens with the
                      // user's conditions, not a blank box.
                      if (!field.value) {
                        const cur = formToConditions(
                          form.getValues("logic"),
                          form.getValues("conditions"),
                        );
                        form.setValue(
                          "conditionsJson",
                          JSON.stringify(cur, null, 2),
                        );
                      }
                      field.onChange(!field.value);
                    }}
                    className="text-muted-foreground hover:text-foreground focus-visible:ring-ring/50 focus-visible:ring-[3px] focus-visible:outline-none flex items-center gap-1 rounded-sm text-xs"
                  >
                    <Code2 className="size-3" />
                    {field.value ? "Hide JSON" : "Edit as JSON"}
                  </button>
                  {field.value && (
                    <FormField
                      control={form.control}
                      name="conditionsJson"
                      render={({ field: jsonField }) => (
                        <FormItem>
                          <FormControl>
                            <Textarea
                              {...jsonField}
                              rows={6}
                              className="bg-muted/50 font-mono text-xs"
                              placeholder='{"field":"name","op":"contains","value":"uber"}'
                            />
                          </FormControl>
                          <p className="text-muted-foreground text-xs">
                            Supports nested AND/OR/NOT trees. Visual builder
                            handles single-level only.
                          </p>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  )}
                </div>
              )}
            />
          </div>

          <div className="space-y-3 border-t p-5 sm:p-6">
            <div className="flex items-center justify-between gap-3">
              <div>
                <h3 className="text-muted-foreground text-xs font-semibold tracking-wider uppercase">
                  Then
                </h3>
                <p className="text-muted-foreground mt-1 text-xs">
                  Add one or more. Only "Set category" is limited to one per rule.
                </p>
              </div>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => actions.append({ field: "", value: "" })}
              >
                <Plus className="size-3.5" /> Add action
              </Button>
            </div>
            {actions.fields.length === 0 ? (
              <p className="text-muted-foreground rounded-xl border border-dashed py-6 text-center text-xs">
                No actions configured. Click "Add action" to add one.
              </p>
            ) : (
              <div className="space-y-2">
                {actions.fields.map((field, idx) => (
                  <FormField
                    key={field.id}
                    control={form.control}
                    name={`actions.${idx}` as const}
                    render={({ field: rhfField }) => (
                      <FormItem>
                        <FormControl>
                          <ActionRowFields
                            index={idx}
                            totalRows={actions.fields.length}
                            usedActionFields={usedActionFields}
                            value={rhfField.value as ActionRow}
                            onChange={(next) => rhfField.onChange(next)}
                            onRemove={() => actions.remove(idx)}
                          />
                        </FormControl>
                        <RowError errors={getRowErrors(form, "actions", idx)} />
                      </FormItem>
                    )}
                  />
                ))}
              </div>
            )}

            {comboWarnings.length > 0 && (
              <StatusPanel
                tone="warning"
                icon={AlertTriangle}
                heading="Heads up"
                body={
                  <span className="space-y-1">
                    {comboWarnings.map((w, i) => (
                      <span key={i} className="block">
                        {w}
                      </span>
                    ))}
                  </span>
                }
              />
            )}
          </div>

          <div className="bg-muted/30 flex items-center justify-end gap-2 border-t px-5 py-4 sm:px-6">
            <Button type="button" variant="ghost" onClick={onCancel}>
              Cancel
            </Button>
            <Button type="submit" disabled={submitting}>
              {submitting ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Save className="size-4" />
              )}
              {submitting ? "Saving…" : submitLabel}
            </Button>
          </div>

          {/* Shared tag autocomplete — referenced by tag-slug inputs in both
              the conditions and actions builders. Mounted once at the form
              root so we don't duplicate the option list per row. */}
          {(tags ?? []).length > 0 && (
            <datalist id="bb-tag-slugs">
              {(tags ?? []).map((t) => (
                <option key={t.slug} value={t.slug}>
                  {t.display_name}
                </option>
              ))}
            </datalist>
          )}
        </form>
      </Form>

      {/* Right rail: live preview. On mobile it stacks below the form. */}
      <aside className="space-y-4">
        <PreviewPanel
          conditions={parsedConditions}
          disabled={submitting}
        />
        <Alert>
          <Wand2 className="size-4" />
          <AlertTitle>Heads up</AlertTitle>
          <AlertDescription className="text-xs">
            Saving a rule only affects future syncs. To apply it to existing
            transactions, open the rule's detail page and use "Apply
            retroactively".
          </AlertDescription>
        </Alert>
      </aside>
    </div>
  );
}

function buildInitialValues(rule: TransactionRule | undefined): RuleFormValues {
  if (!rule) {
    return {
      name: "",
      trigger: "on_create",
      priority: 10,
      logic: "and",
      conditions: [],
      actions: [{ field: "", value: "" } as ActionRow],
      conditionsJson: "",
      useJsonEditor: false,
    };
  }
  const parsed = conditionsToForm(rule.conditions);
  const formActions = actionsToForm(rule.actions);
  return {
    name: rule.name,
    trigger: normalizeTrigger(rule.trigger),
    priority: rule.priority,
    logic: parsed.logic,
    conditions: parsed.rows,
    actions: formActions.length > 0 ? formActions : [{ field: "", value: "" } as ActionRow],
    conditionsJson: JSON.stringify(rule.conditions ?? {}, null, 2),
    // If the rule's condition tree is too complex for the visual builder
    // (nested combinators or `not`), open in JSON mode automatically so the
    // user doesn't silently lose data.
    useJsonEditor: parsed.needsJson,
  };
}

function normalizeTrigger(t: string): "on_create" | "on_change" | "always" {
  if (t === "on_update") return "on_change";
  if (t === "on_change" || t === "always") return t;
  return "on_create";
}

function safeParseConditions(raw: string): Condition {
  try {
    return JSON.parse(raw) as Condition;
  } catch {
    return {};
  }
}

function MatchSubtitle({
  rowCount,
  logic,
}: {
  rowCount: number;
  logic: "and" | "or";
}) {
  if (rowCount === 0) {
    return (
      <>
        No conditions — this rule fires on <strong>every transaction</strong>.
      </>
    );
  }
  if (rowCount === 1) return <>Match transactions where…</>;
  return (
    <>
      Match transactions where <strong>{logic === "or" ? "any" : "all"}</strong>{" "}
      conditions are true.
    </>
  );
}

// getRowErrors flattens the per-field errors RHF attaches to e.g.
// `conditions.2.op` and `conditions.2.value` so we can render a single line
// of errors under each row.
function getRowErrors(
  form: UseFormReturn<RuleFormValues>,
  key: "conditions" | "actions",
  idx: number,
): string[] {
  const rowErr = form.formState.errors?.[key]?.[idx];
  if (!rowErr) return [];
  const out: string[] = [];
  for (const v of Object.values(rowErr) as { message?: string }[]) {
    if (v?.message) out.push(v.message);
  }
  return out;
}

function RowError({ errors }: { errors: string[] }) {
  if (errors.length === 0) return null;
  return (
    <p className="text-destructive ml-12 flex items-start gap-1.5 text-xs">
      <AlertCircle aria-hidden="true" className="mt-0.5 size-3 shrink-0" />
      <span className="min-w-0">{errors.join(" · ")}</span>
    </p>
  );
}
