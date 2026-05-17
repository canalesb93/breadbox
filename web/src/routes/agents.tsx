import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import {
  AlertCircle,
  Bot,
  CheckCircle2,
  Clock,
  History,
  Pencil,
  Play,
  Plus,
  Trash2,
  XCircle,
} from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { EmptyState } from "@/components/empty-state";
import { PageError } from "@/components/page-error";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { Switch } from "@/components/ui/switch";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { withMutationToast } from "@/lib/mutation-toast";
import { formatRelativeTime } from "@/lib/format";
import {
  useAgents,
  useCreateAgent,
  useDeleteAgent,
  useRunAgentNow,
  useToggleAgent,
  type AgentDefinition,
} from "@/api/queries/agents";

const createAgentSchema = z.object({
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
  schedule_cron: z.string().optional().or(z.literal("")),
});
type CreateAgentForm = z.infer<typeof createAgentSchema>;

export function AgentsPage() {
  const agentsQuery = useAgents();
  const createAgent = useCreateAgent();
  const deleteAgent = useDeleteAgent();
  const [createOpen, setCreateOpen] = useState(false);
  const [pendingDelete, setPendingDelete] = useState<AgentDefinition | null>(
    null,
  );

  const agents = agentsQuery.data ?? [];

  const form = useForm<CreateAgentForm>({
    resolver: zodResolver(createAgentSchema),
    defaultValues: { name: "", slug: "", prompt: "", schedule_cron: "" },
  });

  const onCreate = form.handleSubmit(async (values) => {
    const ok = await withMutationToast(
      () =>
        createAgent.mutateAsync({
          name: values.name,
          slug: values.slug,
          prompt: values.prompt,
          schedule_cron: values.schedule_cron || null,
        }),
      {
        success: `Created agent ${values.name}`,
        error: "Failed to create agent",
      },
    );
    if (ok) {
      setCreateOpen(false);
      form.reset();
    }
  });

  return (
    <>
      <PageHeader
        eyebrow="System"
        title="Agents"
        description="Recurring Claude Agent SDK runs that call breadbox MCP to enrich, categorize, and review your data."
        actions={
          <Button onClick={() => setCreateOpen(true)}>
            <Plus className="size-4" />
            New agent
          </Button>
        }
      />

      {agentsQuery.isError ? (
        <PageError
          resource="agents"
          error={agentsQuery.error}
          onRetry={() => agentsQuery.refetch()}
          retrying={agentsQuery.isFetching}
        />
      ) : agentsQuery.isLoading ? (
        <div className="flex flex-col gap-3">
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className="h-24 w-full rounded-xl" />
          ))}
        </div>
      ) : agents.length === 0 ? (
        <EmptyState
          icon={Bot}
          title="No agents yet"
          description="Create your first agent to schedule recurring Claude runs against your data. Each agent runs locally via the Claude Agent SDK and the breadbox MCP server."
          action={
            <Button onClick={() => setCreateOpen(true)}>
              <Plus className="size-4" />
              Create your first agent
            </Button>
          }
        />
      ) : (
        <div className="flex flex-col gap-3">
          {agents.map((agent) => (
            <AgentRow
              key={agent.id}
              agent={agent}
              onDelete={() => setPendingDelete(agent)}
            />
          ))}
        </div>
      )}

      <Sheet open={createOpen} onOpenChange={setCreateOpen}>
        <SheetContent className="sm:max-w-lg">
          <SheetHeader>
            <SheetTitle>New agent</SheetTitle>
            <SheetDescription>
              Quick-create with prompt + schedule. The full prompt builder
              ships in the next iteration; for now you can edit advanced
              fields via the REST API.
            </SheetDescription>
          </SheetHeader>
          <Form {...form}>
            <form
              onSubmit={onCreate}
              className="mt-4 flex flex-1 flex-col gap-4"
            >
              <FormField
                control={form.control}
                name="name"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Name</FormLabel>
                    <FormControl>
                      <Input
                        placeholder="Weekly transaction review"
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="slug"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Slug</FormLabel>
                    <FormControl>
                      <Input
                        placeholder="weekly-transaction-review"
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      Kebab-case identifier used in URLs and API calls.
                    </FormDescription>
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
                      <Textarea
                        rows={6}
                        placeholder="Review last week's uncategorized transactions and apply the closest matching category…"
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="schedule_cron"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Schedule (optional)</FormLabel>
                    <FormControl>
                      <Input placeholder="0 9 * * 1 — Mondays at 9 AM" {...field} />
                    </FormControl>
                    <FormDescription>
                      Standard 5-field cron expression. Leave blank for manual
                      triggers only.
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <SheetFooter className="mt-auto flex-row justify-end gap-2 pt-4">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setCreateOpen(false)}
                >
                  Cancel
                </Button>
                <Button type="submit" disabled={createAgent.isPending}>
                  {createAgent.isPending ? "Creating…" : "Create agent"}
                </Button>
              </SheetFooter>
            </form>
          </Form>
        </SheetContent>
      </Sheet>

      <ConfirmDialog
        open={Boolean(pendingDelete)}
        onOpenChange={(open) => !open && setPendingDelete(null)}
        tone="destructive"
        title={`Delete agent ${pendingDelete?.name}?`}
        description="This removes the agent definition. Historical runs are preserved for audit."
        confirmLabel="Delete agent"
        pending={deleteAgent.isPending}
        onConfirm={() => {
          if (!pendingDelete) return;
          const slug = pendingDelete.slug;
          const name = pendingDelete.name;
          void withMutationToast(() => deleteAgent.mutateAsync(slug), {
            success: `Deleted ${name}`,
            error: "Delete failed",
          }).then((ok) => {
            if (ok) setPendingDelete(null);
          });
        }}
      />
    </>
  );
}

interface AgentRowProps {
  agent: AgentDefinition;
  onDelete: () => void;
}

function AgentRow({ agent, onDelete }: AgentRowProps) {
  const toggle = useToggleAgent();
  const runNow = useRunAgentNow();

  const handleToggle = (enable: boolean) => {
    void withMutationToast(
      () => toggle.mutateAsync({ slug: agent.slug, enable }),
      {
        success: enable ? "Enabled" : "Disabled",
        error: "Toggle failed",
      },
    );
  };

  const handleRunNow = () => {
    void withMutationToast(() => runNow.mutateAsync(agent.slug), {
      success: `Run started for ${agent.name}`,
      error:
        "Run failed — check Settings → Agents for auth, or `make agent-sidecar` for the binary",
    });
  };

  return (
    <Card className="p-4">
      <div className="flex flex-wrap items-start gap-4">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <h3 className="text-base font-semibold leading-tight truncate">
              {agent.name}
            </h3>
            <Badge variant="outline" className="font-mono text-[10px]">
              {agent.slug}
            </Badge>
            <Badge
              variant={agent.tool_scope === "read_write" ? "default" : "secondary"}
              className="text-[10px] uppercase tracking-wide"
            >
              {agent.tool_scope.replace("_", " ")}
            </Badge>
          </div>
          <p className="text-muted-foreground mt-1 line-clamp-2 text-sm">
            {agent.prompt}
          </p>
          <div className="text-muted-foreground mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs">
            <span className="inline-flex items-center gap-1">
              <Clock className="size-3" />
              {agent.schedule_cron
                ? `Cron: ${agent.schedule_cron}`
                : "Manual only"}
            </span>
            <span>Model: {agent.model}</span>
            <span>Max turns: {agent.max_turns}</span>
            <LastRunPill run={agent.last_run} />
          </div>
        </div>

        <div className="flex shrink-0 items-center gap-2">
          <div className="flex items-center gap-2 rounded-md border bg-background px-3 py-1.5">
            <Switch
              checked={agent.enabled}
              disabled={toggle.isPending}
              onCheckedChange={handleToggle}
            />
            <span className="text-xs font-medium">
              {agent.enabled ? "Enabled" : "Disabled"}
            </span>
          </div>
          <Button
            variant="secondary"
            size="sm"
            disabled={runNow.isPending}
            onClick={handleRunNow}
          >
            <Play className="size-3.5" />
            Run now
          </Button>
          <Button asChild variant="ghost" size="icon" aria-label="Run history">
            <Link to="/agents/$slug/runs" params={{ slug: agent.slug }}>
              <History className="size-4" />
            </Link>
          </Button>
          <Button asChild variant="ghost" size="icon" aria-label="Edit agent">
            <Link to="/agents/$slug/edit" params={{ slug: agent.slug }}>
              <Pencil className="size-4" />
            </Link>
          </Button>
          <Button
            variant="ghost"
            size="icon"
            aria-label="Delete agent"
            onClick={onDelete}
          >
            <Trash2 className="size-4" />
          </Button>
        </div>
      </div>
    </Card>
  );
}

function LastRunPill({
  run,
}: {
  run: AgentDefinition["last_run"];
}) {
  if (!run) {
    return <span className="opacity-70">No runs yet</span>;
  }
  const Icon =
    run.status === "success"
      ? CheckCircle2
      : run.status === "error"
        ? XCircle
        : run.status === "skipped"
          ? AlertCircle
          : Clock;
  return (
    <span className="inline-flex items-center gap-1">
      <Icon className="size-3" />
      {run.status} · {formatRelativeTime(run.started_at)}
    </span>
  );
}
