import { type ReactNode, useState } from "react";
import { Link } from "@tanstack/react-router";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import {
  AlertTriangle,
  Bot,
  CalendarClock,
  ChevronRight,
  ExternalLink,
  Hash,
  Info,
  Layers,
  Loader2,
  Moon,
  ShieldCheck,
  Sparkles,
  Wand2,
  Zap,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Switch } from "@/components/ui/switch";
import { Badge } from "@/components/ui/badge";
import { Separator } from "@/components/ui/separator";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
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
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { FormSection } from "@/components/form-section";
import { cn } from "@/lib/utils";
import { AGENT_MODELS, TOOL_SCOPES } from "./agent-constants";
import { CronField } from "./cron-field";
import { PromptToolbar } from "./prompt-toolbar";

// Shared schema for the agent form. Slug is always part of the typed
// payload — in create mode it's user-editable, in edit mode the field
// is hidden but the existing value is carried through so the schema
// validates either way. Numerics use z.preprocess instead of z.coerce
// to dodge the iter-4 resolver bug where coerce.number left the input
// type as `unknown`; preprocess converts string → number explicitly at
// submit time while defaults stay typed.
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

// SCOPE_META decorates the TOOL_SCOPES tuples with an icon + one-line
// rationale so the RadioGroup cards can render a richer affordance than
// a plain label. Kept here (not in agent-constants) because it owns
// presentation only — the canonical values stay shared with any
// non-UI consumer that imports TOOL_SCOPES.
const SCOPE_META: Record<
  (typeof TOOL_SCOPES)[number]["value"],
  { icon: typeof ShieldCheck; tagline: string; tone: string }
> = {
  read_write: {
    icon: Zap,
    tagline:
      "Categorize, tag, comment, create rules. Default for most agents.",
    tone: "text-amber-500",
  },
  read_only: {
    icon: ShieldCheck,
    tagline:
      "Summaries, reports, anomaly checks. No writes — safe to dry-run.",
    tone: "text-emerald-500",
  },
};

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
  const [advancedOpen, setAdvancedOpen] = useState(false);

  return (
    <Form {...form}>
      <form onSubmit={submit} className="flex flex-col gap-5">
        {/* IDENTITY ----------------------------------------------------- */}
        <FormSection
          title="Identity"
          description="How this agent is named in the dashboard, runs list, and API."
          icon={<Hash className="text-muted-foreground size-4" />}
        >
          <div className="grid gap-4 sm:grid-cols-2">
            <FormField
              control={form.control}
              name="name"
              render={({ field }) => (
                <FormItem className={mode === "create" ? "" : "sm:col-span-2"}>
                  <FormLabel>Name</FormLabel>
                  <FormControl>
                    <Input
                      placeholder={
                        mode === "create"
                          ? "Weekly transaction review"
                          : undefined
                      }
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
                        className="font-mono text-sm"
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      Kebab-case identifier in URLs &amp; API calls. Permanent.
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}
          </div>
        </FormSection>

        {/* PROMPT ------------------------------------------------------- */}
        <FormSection
          title="Prompt"
          description="What Claude reads on every run. Be specific about the outcome — what to touch, what to leave alone."
          icon={<Sparkles className="text-muted-foreground size-4" />}
          headerAction={
            <Button asChild variant="outline" size="sm">
              <Link to="/prompts/build">
                <Wand2 className="size-3.5" />
                Open prompt builder
                <ExternalLink className="size-3 opacity-60" />
              </Link>
            </Button>
          }
        >
          <FormField
            control={form.control}
            name="prompt"
            render={({ field }) => (
              <FormItem>
                <PromptToolbar
                  onInsert={(text) => {
                    const current = field.value ?? "";
                    const next = current.trim()
                      ? current.replace(/\s*$/, "") + "\n\n" + text + "\n"
                      : text + "\n";
                    field.onChange(next);
                  }}
                />
                <FormControl>
                  <Textarea
                    rows={12}
                    placeholder={
                      mode === "create"
                        ? "Review last week's uncategorized transactions and apply the closest matching category. Skip anything already overridden."
                        : undefined
                    }
                    className="font-mono text-[13px] leading-relaxed"
                    {...field}
                  />
                </FormControl>
                <PromptStats value={field.value ?? ""} />
                <FormMessage />
              </FormItem>
            )}
          />

          <Collapsible
            open={advancedOpen}
            onOpenChange={setAdvancedOpen}
            className="mt-2"
          >
            <CollapsibleTrigger
              className={cn(
                "text-muted-foreground hover:text-foreground inline-flex items-center gap-1.5 text-xs font-medium transition-colors",
              )}
            >
              <ChevronRight
                className={cn(
                  "size-3.5 transition-transform",
                  advancedOpen && "rotate-90",
                )}
              />
              Override system prompt
              <span className="text-muted-foreground/70 ml-1 font-normal">
                (optional — defaults to the Breadbox baseline)
              </span>
            </CollapsibleTrigger>
            <CollapsibleContent className="pt-3">
              <FormField
                control={form.control}
                name="system_prompt"
                render={({ field }) => (
                  <FormItem>
                    <FormControl>
                      <Textarea
                        rows={4}
                        placeholder="Leave blank to inherit the baseline persona + safety invariants."
                        className="font-mono text-[13px] leading-relaxed"
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      Only fill this in when you need a fundamentally different
                      posture. For per-task instructions, prefer the user prompt
                      above.
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </CollapsibleContent>
          </Collapsible>
        </FormSection>

        {/* MODEL -------------------------------------------------------- */}
        <FormSection
          title="Model"
          description="Pick the Claude family this agent runs against."
          icon={<Bot className="text-muted-foreground size-4" />}
        >
          <FormField
            control={form.control}
            name="model"
            render={({ field }) => (
              <FormItem>
                <Select onValueChange={field.onChange} value={field.value}>
                  <FormControl>
                    <SelectTrigger className="h-10 w-full">
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
        </FormSection>

        {/* TRIGGERS ----------------------------------------------------- */}
        <FormSection
          title="Triggers"
          description="When this agent fires. Combine cron, sync hook, and manual runs as needed."
          icon={<CalendarClock className="text-muted-foreground size-4" />}
        >
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
                <FormDescription>
                  Presets are framed in your browser's timezone — Breadbox
                  stores and fires the schedule in UTC.
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <Separator />

          <FormField
            control={form.control}
            name="trigger_on_sync_complete"
            render={({ field }) => (
              <FormItem className="flex flex-row items-start justify-between gap-4 space-y-0">
                <div className="space-y-1 leading-snug">
                  <FormLabel className="cursor-pointer text-sm">
                    Run after every successful sync
                  </FormLabel>
                  <FormDescription>
                    Fires this agent (trigger=webhook) whenever a bank sync
                    completes. Pairs with cron — or replaces it for "process
                    fresh data" agents.
                  </FormDescription>
                </div>
                <FormControl>
                  <Switch
                    checked={!!field.value}
                    onCheckedChange={field.onChange}
                    aria-label="Run after every successful sync"
                  />
                </FormControl>
              </FormItem>
            )}
          />
        </FormSection>

        {/* SCOPE -------------------------------------------------------- */}
        <FormSection
          title="Tool scope"
          description="What this agent can do through the MCP layer."
          icon={<ShieldCheck className="text-muted-foreground size-4" />}
        >
          <FormField
            control={form.control}
            name="tool_scope"
            render={({ field }) => (
              <FormItem>
                <FormControl>
                  <RadioGroup
                    value={field.value}
                    onValueChange={field.onChange}
                    className="grid gap-2.5 sm:grid-cols-2"
                  >
                    {TOOL_SCOPES.map((scope) => {
                      const meta = SCOPE_META[scope.value];
                      const Icon = meta.icon;
                      const selected = field.value === scope.value;
                      return (
                        <label
                          key={scope.value}
                          htmlFor={`scope-${scope.value}`}
                          className={cn(
                            "group relative flex cursor-pointer flex-col gap-2 rounded-lg border bg-card p-4 text-left transition-all",
                            "hover:border-foreground/20 hover:bg-accent/40",
                            selected
                              ? "border-primary/60 ring-primary/30 ring-2"
                              : "border-border",
                          )}
                        >
                          <div className="flex items-center justify-between gap-2">
                            <div className="flex items-center gap-2">
                              <Icon className={cn("size-4", meta.tone)} />
                              <span className="text-sm font-medium">
                                {scope.label}
                              </span>
                            </div>
                            <RadioGroupItem
                              value={scope.value}
                              id={`scope-${scope.value}`}
                              className="size-4"
                            />
                          </div>
                          <p className="text-muted-foreground text-xs leading-snug">
                            {meta.tagline}
                          </p>
                        </label>
                      );
                    })}
                  </RadioGroup>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name="allowed_tools_raw"
            render={({ field }) => (
              <FormItem>
                <FormLabel className="flex items-center gap-2">
                  <Layers className="text-muted-foreground size-3.5" />
                  Allowed tools
                  <Badge variant="outline" className="font-normal">
                    optional
                  </Badge>
                </FormLabel>
                <FormControl>
                  <Input
                    placeholder="mcp__breadbox__query_transactions, mcp__breadbox__update_transactions"
                    className="font-mono text-xs"
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  Comma-separated MCP tool names. Leave blank to allow every
                  tool in the selected scope.
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </FormSection>

        {/* SAFETY ------------------------------------------------------- */}
        <FormSection
          title="Safety limits"
          description="Hard caps that abort the run before it can spiral."
          icon={<AlertTriangle className="text-muted-foreground size-4" />}
        >
          <div className="grid gap-4 sm:grid-cols-2">
            <FormField
              control={form.control}
              name="max_turns"
              render={({ field }) => (
                <FormItem>
                  <FormLabel className="flex items-center gap-1.5">
                    Max turns
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <button
                          type="button"
                          className="text-muted-foreground/60 hover:text-foreground inline-flex cursor-help transition-colors"
                          aria-label="What is a turn?"
                        >
                          <Info className="size-3.5" />
                        </button>
                      </TooltipTrigger>
                      <TooltipContent>
                        One turn = one back-and-forth with the model. 25 is
                        sane for review agents.
                      </TooltipContent>
                    </Tooltip>
                  </FormLabel>
                  <FormControl>
                    <Input
                      type="number"
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
                  <FormLabel className="flex items-center gap-2">
                    Max cost
                    <Badge variant="outline" className="font-normal">
                      USD
                    </Badge>
                  </FormLabel>
                  <FormControl>
                    <Input
                      type="number"
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
                  <FormDescription>
                    Tracked against the run's accumulated token spend.
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <Separator />

          <div className="space-y-3">
            <div className="flex items-center gap-2">
              <Moon className="text-muted-foreground size-4" />
              <h4 className="text-sm font-medium">Quiet hours</h4>
              <span className="text-muted-foreground text-xs">
                — cron fires inside the window are silently skipped
              </span>
            </div>
            <div className="grid gap-3 sm:grid-cols-2">
              <FormField
                control={form.control}
                name="quiet_hours_start"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel className="text-xs font-normal">
                      Start
                    </FormLabel>
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
                    <FormLabel className="text-xs font-normal">End</FormLabel>
                    <FormControl>
                      <Input
                        type="time"
                        placeholder="07:00"
                        {...field}
                        value={field.value ?? ""}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
          </div>
        </FormSection>

        {/* FOOTER ------------------------------------------------------- */}
        <div className="flex flex-wrap items-center justify-end gap-2 pt-1">
          <Button type="button" variant="ghost" asChild>
            <Link to={cancelTo}>Cancel</Link>
          </Button>
          {extraActions?.(form)}
          <Button type="submit" disabled={pending}>
            {pending && <Loader2 className="size-4 animate-spin" />}
            {submitLabel}
          </Button>
        </div>
      </form>
    </Form>
  );
}

// PromptStats is a tiny live char/word footer that sits under the prompt
// textarea. Encourages prompt hygiene without being a hard limit.
function PromptStats({ value }: { value: string }) {
  const trimmed = value.trim();
  const chars = value.length;
  const words = trimmed ? trimmed.split(/\s+/).length : 0;
  return (
    <div className="text-muted-foreground flex items-center justify-end gap-3 pt-1 text-[11px] tabular-nums">
      <span>
        {words.toLocaleString()} {words === 1 ? "word" : "words"}
      </span>
      <span aria-hidden>·</span>
      <span>{chars.toLocaleString()} chars</span>
    </div>
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
