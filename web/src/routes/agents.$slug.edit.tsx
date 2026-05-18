import { useNavigate, useParams } from "@tanstack/react-router";
import { Loader2, Play } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { PageHeader } from "@/components/page-header";
import { PageError } from "@/components/page-error";
import { SoftBackButton } from "@/components/soft-back-button";
import { withMutationToast } from "@/lib/mutation-toast";
import {
  useAgent,
  useRunAgentNow,
  useUpdateAgent,
  type AgentDefinition,
} from "@/api/queries/agents";
import {
  AgentForm,
  type AgentFormValues,
} from "@/features/agents/agent-form";

export function AgentEditPage() {
  const { slug } = useParams({ strict: false }) as { slug: string };
  const agentQuery = useAgent(slug);

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
  if (agentQuery.isLoading || !agentQuery.data) {
    return (
      <>
        <SoftBackButton to="/agents">Back to agents</SoftBackButton>
        <PageHeader
          eyebrow="Agent"
          title="Edit agent"
          description="Update prompt, schedule, model, and safety caps."
        />
        <div className="flex flex-col gap-3">
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-32 w-full" />
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-1/2" />
        </div>
      </>
    );
  }
  return <AgentEditFormView agent={agentQuery.data} slug={slug} />;
}

function AgentEditFormView({
  agent,
  slug,
}: {
  agent: AgentDefinition;
  slug: string;
}) {
  const navigate = useNavigate();
  const updateAgent = useUpdateAgent(slug);

  const initialValues: AgentFormValues = {
    name: agent.name,
    slug: agent.slug, // present but unused in edit mode (field hidden)
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
  };

  const onSubmit = async (values: AgentFormValues) => {
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
          quiet_hours_start: values.quiet_hours_start ? values.quiet_hours_start : null,
          quiet_hours_end: values.quiet_hours_end ? values.quiet_hours_end : null,
          trigger_on_sync_complete: values.trigger_on_sync_complete,
        }),
      {
        success: "Agent saved",
        error: "Save failed",
      },
    );
    if (ok) navigate({ to: "/agents" });
  };

  return (
    <>
      <SoftBackButton to="/agents">Back to agents</SoftBackButton>
      <PageHeader
        eyebrow="Agent"
        title={agent.name}
        description="Update prompt, schedule, model, and safety caps. Changes take effect on the next scheduled fire or manual run."
      />
      <AgentForm
        mode="edit"
        initialValues={initialValues}
        onSubmit={onSubmit}
        submitLabel="Save changes"
        pending={updateAgent.isPending}
        extraActions={(form) => (
          <TestPromptButton
            slug={slug}
            promptValue={form.watch("prompt")}
            disabled={updateAgent.isPending}
          />
        )}
      />
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
