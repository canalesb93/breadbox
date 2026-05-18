import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { z } from "zod";
import { ArrowLeft } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/page-header";
import { useCreateAgent } from "@/api/queries/agents";
import { withMutationToast } from "@/lib/mutation-toast";
import {
  AgentForm,
  CREATE_DEFAULTS,
  type AgentFormValues,
} from "@/features/agents/agent-form";

// agentsNewSearchSchema accepts an optional `?prompt=` parameter so the
// prompt builder can deep-link into the create flow with the composed
// text pre-loaded. Unknown keys are dropped by TanStack Router's zod
// integration — `prompt` is the only field we read here.
export const agentsNewSearchSchema = z.object({
  prompt: z.string().optional(),
});
type AgentsNewSearch = z.infer<typeof agentsNewSearchSchema>;

export function AgentNewPage() {
  const navigate = useNavigate();
  const createAgent = useCreateAgent();
  const search = useSearch({ strict: false }) as AgentsNewSearch;

  const onSubmit = async (values: AgentFormValues) => {
    const tools = (values.allowed_tools_raw ?? "")
      .split(",")
      .map((s) => s.trim())
      .filter((s) => s.length > 0);
    const created = await withMutationToast(
      () =>
        createAgent.mutateAsync({
          name: values.name,
          slug: values.slug ?? "",
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
          quiet_hours_end: values.quiet_hours_end ? values.quiet_hours_end : null,
          trigger_on_sync_complete: values.trigger_on_sync_complete,
        }),
      {
        success: `Created agent ${values.name}`,
        error: "Failed to create agent",
      },
    );
    if (created) {
      // Land on the agents list so the new card is visible. The user can
      // immediately click into edit / runs / run-now from there.
      navigate({ to: "/agents" });
    }
  };

  return (
    <>
      <Button asChild variant="ghost" size="sm" className="-ml-2 mb-2">
        <Link to="/agents">
          <ArrowLeft className="size-4" /> Back to agents
        </Link>
      </Button>
      <PageHeader
        eyebrow="Agent"
        title="New agent"
        description="Define a recurring Claude Agent SDK run. All fields can be edited later — schedule, scope, and safety caps default to safe values."
      />
      <AgentForm
        mode="create"
        initialValues={{
          ...CREATE_DEFAULTS,
          prompt: search.prompt ?? CREATE_DEFAULTS.prompt,
        }}
        onSubmit={onSubmit}
        submitLabel={createAgent.isPending ? "Creating…" : "Create agent"}
        pending={createAgent.isPending}
      />
    </>
  );
}
