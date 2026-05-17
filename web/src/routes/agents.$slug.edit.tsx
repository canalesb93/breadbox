import { useEffect } from "react";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { ArrowLeft, Loader2 } from "lucide-react";
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
import { useAgent, useUpdateAgent } from "@/api/queries/agents";
import {
  AGENT_MODELS,
  TOOL_SCOPES,
} from "@/features/agents/agent-constants";
import { CronField } from "@/features/agents/cron-field";

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
              </Card>

              <div className="flex justify-end gap-2">
                <Button type="button" variant="outline" asChild>
                  <Link to="/agents">Cancel</Link>
                </Button>
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
