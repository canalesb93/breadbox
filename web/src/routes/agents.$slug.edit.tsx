import { useEffect } from "react";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { ArrowLeft, Loader2, Play } from "lucide-react";
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
import { Skeleton } from "@/components/ui/skeleton";
import { PageHeader } from "@/components/page-header";
import { PageError } from "@/components/page-error";
import { withMutationToast } from "@/lib/mutation-toast";
import { useAgent, useRunAgentNow, useUpdateAgent } from "@/api/queries/agents";
import {
  AGENT_MODELS,
  TOOL_SCOPES,
} from "@/features/agents/agent-constants";
import { CronField } from "@/features/agents/cron-field";
import { RuleDslHelp } from "@/features/agents/rule-dsl-help";

// Use z.preprocess (not z.coerce) for numerics to dodge the iter-4 resolver
// bug where coerce.number leaves the input type as `unknown`. Preprocess
// converts string → number explicitly at submit time; defaults stay typed.
const agentEditSchema = z.object({
  name: z.string().min(1, "Name is required").max(120),
  prompt: z.string().min(1, "Prompt is required"),
  system_prompt: z.string().optional().or(z.literal("")),
  schedule_cron: z.string().optional().or(z.literal("")),
  tool_scope: z.enum(["read_only", "read_write"]),
  allowed_tools_raw: z.string().optional(), // comma-separated; split at submit
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
type AgentEditForm = z.input<typeof agentEditSchema>;

export function AgentEditPage() {
  const { slug } = useParams({ strict: false }) as { slug: string };
  const navigate = useNavigate();
  const agentQuery = useAgent(slug);
  const updateAgent = useUpdateAgent(slug);

  const form = useForm<AgentEditForm>({
    resolver: zodResolver(agentEditSchema),
    defaultValues: {
      name: "",
      prompt: "",
      system_prompt: "",
      schedule_cron: "",
      tool_scope: "read_write",
      allowed_tools_raw: "",
      model: "claude-opus-4-7",
      max_turns: 10,
      max_budget_usd: null,
      quiet_hours_start: "",
      quiet_hours_end: "",
      trigger_on_sync_complete: false,
    },
  });

  // Hydrate the form once the agent loads. Depend on short_id (immutable)
  // not the whole object, so a cache refetch doesn't wipe in-flight edits.
  useEffect(() => {
    const agent = agentQuery.data;
    if (!agent) return;
    form.reset({
      name: agent.name,
      prompt: agent.prompt,
      system_prompt: agent.system_prompt ?? "",
      schedule_cron: agent.schedule_cron ?? "",
      tool_scope: agent.tool_scope,
      allowed_tools_raw: (agent.allowed_tools ?? []).join(", "),
      model: agent.model,
      max_turns: agent.max_turns,
      max_budget_usd: agent.max_budget_usd ?? null,
      quiet_hours_start: agent.quiet_hours_start ?? "",
      quiet_hours_end: agent.quiet_hours_end ?? "",
      trigger_on_sync_complete: agent.trigger_on_sync_complete,
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [agentQuery.data?.short_id]);

  const onSubmit = form.handleSubmit(async (values) => {
    const tools = (values.allowed_tools_raw ?? "")
      .split(",")
      .map((s) => s.trim())
      .filter((s) => s.length > 0);
    const ok = await withMutationToast(
      () =>
        updateAgent.mutateAsync({
          name: values.name,
          prompt: values.prompt,
          system_prompt: values.system_prompt ? values.system_prompt : null,
          schedule_cron: values.schedule_cron ? values.schedule_cron : null,
          tool_scope: values.tool_scope,
          allowed_tools: tools,
          model: values.model,
          max_turns: values.max_turns as unknown as number,
          max_budget_usd: values.max_budget_usd as unknown as number | null,
          quiet_hours_start: values.quiet_hours_start
            ? values.quiet_hours_start
            : null,
          quiet_hours_end: values.quiet_hours_end
            ? values.quiet_hours_end
            : null,
          trigger_on_sync_complete: values.trigger_on_sync_complete,
        }),
      {
        success: "Agent saved",
        error: "Save failed",
      },
    );
    if (ok) navigate({ to: "/agents" });
  });

  if (agentQuery.isError) {
    return (
      <PageError
        resource="agent"
        error={agentQuery.error}
        onRetry={() => agentQuery.refetch()}
        retrying={agentQuery.isFetching}
      />
    );
  }

  return (
    <>
      <Button asChild variant="ghost" size="sm" className="-ml-2 mb-2">
        <Link to="/agents">
          <ArrowLeft className="size-4" /> Back to agents
        </Link>
      </Button>
      <PageHeader
        eyebrow="Agent"
        title={agentQuery.data?.name ?? "Edit agent"}
        description="Update prompt, schedule, model, and safety caps. Changes take effect on the next scheduled fire or manual run."
      />

      {agentQuery.isLoading ? (
        <div className="flex flex-col gap-3">
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-32 w-full" />
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-1/2" />
        </div>
      ) : (
        <Form {...form}>
          <form onSubmit={onSubmit} className="grid grid-cols-1 gap-6 md:grid-cols-3">
            <div className="space-y-4 md:col-span-2">
              <Card className="space-y-4 p-4">
                <FormField
                  control={form.control}
                  name="name"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Name</FormLabel>
                      <FormControl>
                        <Input {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name="prompt"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Prompt</FormLabel>
                      <FormControl>
                        <Textarea rows={10} {...field} />
                      </FormControl>
                      <FormDescription>
                        Sent to Claude on every run. Be specific about the
                        outcome (what to categorize, what to report, what
                        not to touch).
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
                        Optional override sent as the SDK system prompt
                        instead of the default. Leave blank for default behavior.
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </Card>
            </div>

            <div className="space-y-4">
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
                        <Textarea
                          rows={3}
                          placeholder="mcp__breadbox__*"
                          {...field}
                        />
                      </FormControl>
                      <FormDescription>
                        Comma-separated MCP tool names. Leave blank to allow
                        every tool in scope.
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
                <div className="grid grid-cols-2 gap-3">
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
                          Cron fires inside the window are silently skipped.
                          Leave both blank to disable.
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
                          Fires this agent (trigger=webhook) whenever a bank
                          sync completes. Useful for keep-up agents like
                          "re-categorize freshly synced transactions" — pairs
                          with cron OR replaces it.
                        </FormDescription>
                        <FormMessage />
                      </div>
                    </FormItem>
                  )}
                />
              </Card>

              <div className="flex flex-wrap items-center justify-end gap-2">
                <Button type="button" variant="outline" asChild>
                  <Link to="/agents">Cancel</Link>
                </Button>
                <TestPromptButton
                  slug={slug}
                  promptValue={form.watch("prompt")}
                  disabled={updateAgent.isPending}
                />
                <Button type="submit" disabled={updateAgent.isPending}>
                  {updateAgent.isPending && (
                    <Loader2 className="size-4 animate-spin" />
                  )}
                  Save changes
                </Button>
              </div>
            </div>
          </form>
        </Form>
      )}
    </>
  );
}

// TestPromptButton dry-fires the in-edit-form prompt via the iter-45
// prompt_override path on POST /api/v1/agents/:slug/run. Lets operators
// iterate on prompts without round-tripping through Save (which would
// mutate the stored definition + fire every cron from that point onward
// with the new prompt). Disabled when the form is mid-save or the prompt
// field is empty.
//
// On success, navigates to the agent's run history with the new run's
// transcript drawer pre-opened, so operators can immediately read the
// model's response.
function TestPromptButton({
  slug,
  promptValue,
  disabled,
}: {
  slug: string;
  promptValue: string | undefined;
  disabled: boolean;
}) {
  const runNow = useRunAgentNow();
  const navigate = useNavigate();
  const empty = !promptValue || promptValue.trim().length === 0;
  const onClick = async () => {
    const trimmed = (promptValue ?? "").trim();
    if (!trimmed) return;
    const ok = await withMutationToast(
      async () => {
        const run = await runNow.mutateAsync({
          slug,
          promptOverride: trimmed,
        });
        navigate({
          to: "/agents/$slug/runs",
          params: { slug },
          search: { run: run.short_id },
        });
        return run;
      },
      {
        success: "Test run dispatched — opening transcript…",
        error:
          "Test run failed — check Settings → Agents for auth, or `make agent-sidecar` for the binary",
      },
    );
    // navigate() above triggers regardless of toast result; no-op here.
    void ok;
  };
  return (
    <Button
      type="button"
      variant="secondary"
      onClick={onClick}
      disabled={disabled || runNow.isPending || empty}
      title={
        empty
          ? "Fill in the prompt above to dry-fire it without saving"
          : "Dry-fire this prompt now without saving the definition"
      }
    >
      {runNow.isPending ? (
        <Loader2 className="size-4 animate-spin" />
      ) : (
        <Play className="size-4" />
      )}
      Test this prompt
    </Button>
  );
}
