import { type ReactNode } from "react";
import { Link } from "@tanstack/react-router";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Loader2, PenTool } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { AGENT_MODELS, TOOL_SCOPES } from "./agent-constants";
import { CronField } from "./cron-field";
import { RuleDslHelp } from "./rule-dsl-help";

// Shared schema for the two-column agent form. Slug is always part of
// the typed payload — in create mode it's user-editable, in edit mode
// the field is hidden but the existing value is carried through so the
// schema validates either way. Numerics use z.preprocess instead of
// z.coerce to dodge the iter-4 resolver bug where coerce.number left
// the input type as `unknown`; preprocess converts string → number
// explicitly at submit time while defaults stay typed.
const agentFormSchema = z.object({
  name: z.string().min(1, "Name is required").max(120),
  slug: z
    .string()
    .min(2, "Slug must be at least 2 characters")
    .max(64)
    .regex(
      /^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$/,
      "Use lowercase letters, digits, dashes (kebab-case)",
    ),
  prompt: z.string().min(1, "Prompt is required"),
  system_prompt: z.string().optional().or(z.literal("")),
  schedule_cron: z.string().optional().or(z.literal("")),
  tool_scope: z.enum(["read_only", "read_write"]),
  allowed_tools_raw: z.string().optional(),
  model: z.string().min(1),
  max_turns: z.preprocess(
    (v) => (v === "" || v === null || v === undefined ? undefined : Number(v)),
    z.number().int().min(1).max(200),
  ),
  max_budget_usd: z.preprocess(
    (v) => (v === "" || v === null || v === undefined ? null : Number(v)),
    z.number().positive().nullable(),
  ),
  quiet_hours_start: z
    .string()
    .regex(/^([01]\d|2[0-3]):[0-5]\d$/, "Use HH:MM 24-hour")
    .optional()
    .or(z.literal("")),
  quiet_hours_end: z
    .string()
    .regex(/^([01]\d|2[0-3]):[0-5]\d$/, "Use HH:MM 24-hour")
    .optional()
    .or(z.literal("")),
  trigger_on_sync_complete: z.boolean(),
});

export type AgentFormValues = z.input<typeof agentFormSchema>;

export interface AgentFormProps {
  mode: "create" | "edit";
  initialValues: AgentFormValues;
  onSubmit: (values: AgentFormValues) => Promise<void> | void;
  submitLabel: string;
  pending: boolean;
  cancelTo?: string;
  // extraActions render between Cancel and Submit. Edit mode injects the
  // "Test this prompt" dry-run button here; create mode leaves it empty
  // (no slug exists yet, so no run endpoint to target).
  extraActions?: (form: ReturnType<typeof useAgentForm>) => ReactNode;
}

function useAgentForm(initialValues: AgentFormValues) {
  return useForm<AgentFormValues>({
    resolver: zodResolver(agentFormSchema),
    defaultValues: initialValues,
  });
}

export function AgentForm({
  mode,
  initialValues,
  onSubmit,
  submitLabel,
  pending,
  cancelTo = "/agents",
  extraActions,
}: AgentFormProps) {
  const form = useAgentForm(initialValues);
  const submit = form.handleSubmit(async (values) => {
    await onSubmit(values);
  });

  return (
    <Form {...form}>
      <form onSubmit={submit} className="grid grid-cols-1 gap-6 md:grid-cols-3">
        <div className="space-y-4 md:col-span-2 md:order-none">
          <Card className="space-y-4 p-4">
            <FormField
              control={form.control}
              name="name"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Name</FormLabel>
                  <FormControl>
                    <Input
                      placeholder={mode === "create" ? "Weekly transaction review" : undefined}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            {mode === "create" && (
              <FormField
                control={form.control}
                name="slug"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Slug</FormLabel>
                    <FormControl>
                      <Input
                        placeholder="weekly-transaction-review"
                        autoCapitalize="none"
                        autoCorrect="off"
                        spellCheck={false}
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      Kebab-case identifier used in URLs and API calls. Permanent — pick carefully.
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}
            <FormField
              control={form.control}
              name="prompt"
              render={({ field }) => (
                <FormItem>
                  <div className="flex items-center justify-between gap-2">
                    <FormLabel>Prompt</FormLabel>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="h-7 px-2 text-xs"
                      asChild
                    >
                      <Link to="/prompts/build">
                        <PenTool className="size-3.5" />
                        Build with templates
                      </Link>
                    </Button>
                  </div>
                  <FormControl>
                    <Textarea
                      rows={10}
                      placeholder={
                        mode === "create"
                          ? "Review last week's uncategorized transactions and apply the closest matching category…"
                          : undefined
                      }
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    Sent to Claude on every run. Be specific about the outcome (what to categorize,
                    what to report, what not to touch). Need a head start?{" "}
                    <Link to="/prompts/build" className="underline">
                      Open the prompt builder
                    </Link>
                    .
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <RuleDslHelp />
            <FormField
              control={form.control}
              name="system_prompt"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>System prompt (optional)</FormLabel>
                  <FormControl>
                    <Textarea rows={4} {...field} />
                  </FormControl>
                  <FormDescription>
                    Optional override. Leave blank to use the breadbox baseline
                    (autonomous-agent persona + the safety invariants every run
                    should honor). Fill this in only when you need a fundamentally
                    different posture — the user prompt above is the right place
                    for per-task instructions.
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </Card>
        </div>

        <div className="order-first space-y-4 md:order-none">
          <Card className="space-y-4 p-4">
            <FormField
              control={form.control}
              name="model"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Model</FormLabel>
                  <Select onValueChange={field.onChange} value={field.value}>
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      {AGENT_MODELS.map((m) => (
                        <SelectItem key={m.value} value={m.value}>
                          {m.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="schedule_cron"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Schedule</FormLabel>
                  <FormControl>
                    <CronField
                      value={field.value ?? ""}
                      onChange={field.onChange}
                      onBlur={field.onBlur}
                      name={field.name}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="tool_scope"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Tool scope</FormLabel>
                  <Select onValueChange={field.onChange} value={field.value}>
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      {TOOL_SCOPES.map((s) => (
                        <SelectItem key={s.value} value={s.value}>
                          {s.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <FormDescription>
                    Controls which MCP write tools the run can call.
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="allowed_tools_raw"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Allowed tools (optional)</FormLabel>
                  <FormControl>
                    <Textarea rows={3} placeholder="mcp__breadbox__*" {...field} />
                  </FormControl>
                  <FormDescription>
                    Comma-separated MCP tool names. Leave blank to allow every tool in scope.
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <div className="grid grid-cols-2 gap-3">
              <FormField
                control={form.control}
                name="max_turns"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Max turns</FormLabel>
                    <FormControl>
                      <Input
                        type="number"
                        inputMode="numeric"
                        min={1}
                        max={200}
                        value={
                          field.value === undefined || field.value === null
                            ? ""
                            : String(field.value)
                        }
                        onChange={(e) => field.onChange(e.target.value)}
                        onBlur={field.onBlur}
                        name={field.name}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="max_budget_usd"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Max cost (USD)</FormLabel>
                    <FormControl>
                      <Input
                        type="number"
                        inputMode="decimal"
                        step="0.01"
                        min={0}
                        placeholder="0.50"
                        value={
                          field.value === undefined || field.value === null
                            ? ""
                            : String(field.value)
                        }
                        onChange={(e) => field.onChange(e.target.value)}
                        onBlur={field.onBlur}
                        name={field.name}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <FormField
                control={form.control}
                name="quiet_hours_start"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Quiet hours start</FormLabel>
                    <FormControl>
                      <Input
                        type="time"
                        placeholder="22:00"
                        {...field}
                        value={field.value ?? ""}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="quiet_hours_end"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Quiet hours end</FormLabel>
                    <FormControl>
                      <Input
                        type="time"
                        placeholder="07:00"
                        {...field}
                        value={field.value ?? ""}
                      />
                    </FormControl>
                    <FormDescription>
                      Cron fires inside the window are silently skipped. Leave both blank
                      to disable.
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
            <FormField
              control={form.control}
              name="trigger_on_sync_complete"
              render={({ field }) => (
                <FormItem className="mt-4 flex items-start gap-3 rounded-md border p-3">
                  <FormControl>
                    <input
                      type="checkbox"
                      className="mt-1 size-4"
                      checked={field.value}
                      onChange={(e) => field.onChange(e.target.checked)}
                    />
                  </FormControl>
                  <div className="space-y-1">
                    <FormLabel className="font-medium">
                      Run after every successful sync
                    </FormLabel>
                    <FormDescription>
                      Fires this agent (trigger=webhook) whenever a bank sync completes.
                      Useful for keep-up agents like "re-categorize freshly synced
                      transactions" — pairs with cron OR replaces it.
                    </FormDescription>
                    <FormMessage />
                  </div>
                </FormItem>
              )}
            />
          </Card>

          <div className="flex flex-wrap items-center justify-end gap-2">
            <Button type="button" variant="outline" asChild>
              <Link to={cancelTo}>Cancel</Link>
            </Button>
            {extraActions?.(form)}
            <Button type="submit" disabled={pending}>
              {pending && <Loader2 className="size-4 animate-spin" />}
              {submitLabel}
            </Button>
          </div>
        </div>
      </form>
    </Form>
  );
}

// CREATE_DEFAULTS mirror the Go-side service defaults in internal/service/
// agents.go::Create. Keep aligned — when a new field gets a server-side
// default, mirror it here so the form shows the same starting state.
export const CREATE_DEFAULTS: AgentFormValues = {
  name: "",
  slug: "",
  prompt: "",
  system_prompt: "",
  schedule_cron: "",
  tool_scope: "read_write",
  allowed_tools_raw: "",
  model: AGENT_MODELS[1].value, // Sonnet — balanced default
  max_turns: 25,
  max_budget_usd: null,
  quiet_hours_start: "",
  quiet_hours_end: "",
  trigger_on_sync_complete: false,
};
