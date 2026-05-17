import { useEffect, useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Bot, CheckCircle2, KeyRound, Loader2, Stethoscope, Trash2, XCircle } from "lucide-react";
import { Button } from "@/components/ui/button";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { SettingsSectionHeader } from "@/components/settings-section-header";
import { withMutationToast } from "@/lib/mutation-toast";
import {
  useAgentSettings,
  useRunAgentCleanup,
  useSmokeTestAgent,
  useUpdateAgentSettings,
  type AgentTestResult,
} from "@/api/queries/agents";
import { ApiError } from "@/api/client";
import { toast } from "sonner";

const settingsSchema = z.object({
  auth_mode: z.enum(["subscription", "api_key"]),
  subscription_token: z.string().optional(),
  anthropic_api_key: z.string().optional(),
  max_concurrent: z.number().int().min(1).max(50),
  global_max_budget_usd: z.string().optional(),
  runtime_path: z.string().optional(),
});

export function AgentsSection() {
  const settingsQuery = useAgentSettings();
  const updateSettings = useUpdateAgentSettings();
  const smokeTest = useSmokeTestAgent();
  const cleanup = useRunAgentCleanup();

  const runCleanup = async () => {
    try {
      const r = await cleanup.mutateAsync();
      toast.success(
        `Cleanup: ${r.runs_deleted} run(s), ${r.transcripts_deleted}/${r.transcripts_scanned} transcript(s) removed (retention ${r.retention_days}d).`,
      );
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : String(err);
      toast.error(`Cleanup failed — ${msg}`);
    }
  };
  const [tokenDraft, setTokenDraft] = useState("");
  const [apiKeyDraft, setApiKeyDraft] = useState("");
  const [testResult, setTestResult] = useState<AgentTestResult | null>(null);
  const [testError, setTestError] = useState<{ code: string; message: string } | null>(null);

  const form = useForm({
    resolver: zodResolver(settingsSchema),
    defaultValues: {
      auth_mode: "subscription" as const,
      max_concurrent: 1,
      subscription_token: "",
      anthropic_api_key: "",
      global_max_budget_usd: "",
      runtime_path: "",
    },
  });

  // Hydrate form once settings load.
  useEffect(() => {
    if (!settingsQuery.data) return;
    const s = settingsQuery.data;
    form.reset({
      auth_mode: s.auth_mode,
      max_concurrent: s.max_concurrent,
      global_max_budget_usd:
        s.global_max_budget_usd != null
          ? String(s.global_max_budget_usd)
          : "",
      runtime_path: s.runtime_path ?? "",
    });
  }, [settingsQuery.data, form]);

  const authMode = form.watch("auth_mode");

  const onSubmit = form.handleSubmit(async (values) => {
    const payload: Parameters<typeof updateSettings.mutateAsync>[0] = {
      auth_mode: values.auth_mode,
      max_concurrent: values.max_concurrent,
      runtime_path: values.runtime_path ?? "",
    };
    if (values.global_max_budget_usd && values.global_max_budget_usd !== "") {
      const n = Number(values.global_max_budget_usd);
      if (!Number.isNaN(n)) payload.global_max_budget_usd = n;
    } else {
      payload.global_max_budget_usd = null;
    }
    if (tokenDraft) payload.subscription_token = tokenDraft;
    if (apiKeyDraft) payload.anthropic_api_key = apiKeyDraft;

    const ok = await withMutationToast(
      () => updateSettings.mutateAsync(payload),
      {
        success: "Agent settings saved",
        error: "Failed to save agent settings",
      },
    );
    if (ok) {
      setTokenDraft("");
      setApiKeyDraft("");
    }
  });

  const clearToken = (kind: "subscription_token" | "anthropic_api_key") => {
    void withMutationToast(
      () => updateSettings.mutateAsync({ [kind]: "" }),
      {
        success: "Credential cleared",
        error: "Failed to clear credential",
      },
    );
  };

  const runSmokeTest = async () => {
    setTestResult(null);
    setTestError(null);
    try {
      const r = await smokeTest.mutateAsync();
      setTestResult(r);
    } catch (err) {
      if (err instanceof ApiError) {
        setTestError({ code: err.code, message: err.message });
      } else {
        setTestError({ code: "UNKNOWN", message: String(err) });
      }
    }
  };

  const settings = settingsQuery.data;

  return (
    <div className="space-y-6">
      <SettingsSectionHeader
        title="Agents"
        description="Authentication and global caps for the Claude Agent SDK runner. Tokens are encrypted at rest; the full value never leaves the server after you save it."
      />

      {settingsQuery.isLoading ? (
        <div className="space-y-3">
          <Skeleton className="h-9 w-full" />
          <Skeleton className="h-9 w-full" />
          <Skeleton className="h-9 w-1/2" />
        </div>
      ) : settings ? (
        <Form {...form}>
          <form onSubmit={onSubmit} className="space-y-6">
            <FormField
              control={form.control}
              name="auth_mode"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Authentication mode</FormLabel>
                  <Select onValueChange={field.onChange} value={field.value}>
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue placeholder="Pick auth mode" />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      <SelectItem value="subscription">
                        Claude subscription token
                      </SelectItem>
                      <SelectItem value="api_key">
                        Anthropic API key
                      </SelectItem>
                    </SelectContent>
                  </Select>
                  <FormDescription>
                    Subscription tokens come from <code>claude setup-token</code>{" "}
                    and apply Claude plan credits (free under monthly limits).
                    API keys bill per usage; durable past 2026-06-15.
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            {authMode === "subscription" ? (
              <CredentialField
                kind="subscription_token"
                label="Subscription token"
                placeholder="sk-ant-oat01-…"
                helper="Run `claude setup-token` on any machine, paste the resulting one-year token here."
                stored={settings.subscription_token}
                draft={tokenDraft}
                onDraftChange={setTokenDraft}
                onClear={() => clearToken("subscription_token")}
              />
            ) : (
              <CredentialField
                kind="anthropic_api_key"
                label="Anthropic API key"
                placeholder="sk-ant-…"
                helper="From console.anthropic.com. Billed per API call."
                stored={settings.anthropic_api_key}
                draft={apiKeyDraft}
                onDraftChange={setApiKeyDraft}
                onClear={() => clearToken("anthropic_api_key")}
              />
            )}

            <FormField
              control={form.control}
              name="max_concurrent"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Max concurrent runs</FormLabel>
                  <FormControl>
                    <Input type="number" min={1} max={50} {...field} />
                  </FormControl>
                  <FormDescription>
                    Server-wide cap on agent runs in flight at once. v1 default
                    is 1 — additional triggers either skip (cron) or 503 (manual).
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="global_max_budget_usd"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Global max cost per run (USD)</FormLabel>
                  <FormControl>
                    <Input
                      type="number"
                      step="0.01"
                      placeholder="No global cap"
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    Hard ceiling across all agents, regardless of per-agent
                    settings. Leave blank for none.
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="runtime_path"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>breadbox-agent binary path</FormLabel>
                  <FormControl>
                    <Input
                      placeholder="auto: $BREADBOX_AGENT_BIN, ./bin/breadbox-agent, or PATH"
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    Absolute path to the sidecar binary. Leave blank to use{" "}
                    <code>$BREADBOX_AGENT_BIN</code>, <code>./bin/breadbox-agent</code>,
                    or PATH discovery.
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="flex flex-wrap items-center gap-2">
                <Button
                  type="button"
                  variant="outline"
                  onClick={runSmokeTest}
                  disabled={smokeTest.isPending}
                >
                  {smokeTest.isPending ? (
                    <Loader2 className="size-4 animate-spin" />
                  ) : (
                    <Stethoscope className="size-4" />
                  )}
                  Test connection
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  onClick={runCleanup}
                  disabled={cleanup.isPending}
                  title="Runs the same daily prune pass on demand — useful after lowering retention so you don't have to wait for 3:15 AM."
                >
                  {cleanup.isPending ? (
                    <Loader2 className="size-4 animate-spin" />
                  ) : (
                    <Trash2 className="size-4" />
                  )}
                  Run cleanup now
                </Button>
              </div>
              <Button type="submit" disabled={updateSettings.isPending}>
                {updateSettings.isPending && (
                  <Loader2 className="size-4 animate-spin" />
                )}
                Save settings
              </Button>
            </div>

            {testResult && (
              <Alert>
                <CheckCircle2 className="size-4" />
                <AlertTitle>Test passed</AlertTitle>
                <AlertDescription className="space-y-1">
                  <p>
                    Auth ✓ {testResult.auth_mode} · Model {testResult.model}
                    {" · "}
                    {testResult.duration_ms}ms · $
                    {testResult.total_cost_usd.toFixed(6)} (
                    {testResult.input_tokens} in / {testResult.output_tokens} out)
                  </p>
                  {testResult.response && (
                    <p className="text-muted-foreground">
                      Response:{" "}
                      <code className="bg-muted rounded px-1">
                        {testResult.response}
                      </code>
                    </p>
                  )}
                </AlertDescription>
              </Alert>
            )}
            {testError && (
              <Alert variant="destructive">
                <XCircle className="size-4" />
                <AlertTitle>Test failed — {testError.code}</AlertTitle>
                <AlertDescription>{testError.message}</AlertDescription>
              </Alert>
            )}
          </form>
        </Form>
      ) : (
        <Alert>
          <Bot className="size-4" />
          <AlertTitle>Settings unavailable</AlertTitle>
          <AlertDescription>
            Could not load agent settings — the server may not be reachable.
          </AlertDescription>
        </Alert>
      )}
    </div>
  );
}

interface CredentialFieldProps {
  kind: string;
  label: string;
  placeholder: string;
  helper: string;
  stored?: string | null;
  draft: string;
  onDraftChange: (v: string) => void;
  onClear: () => void;
}

function CredentialField({
  label,
  placeholder,
  helper,
  stored,
  draft,
  onDraftChange,
  onClear,
}: CredentialFieldProps) {
  return (
    <FormItem>
      <FormLabel>{label}</FormLabel>
      {stored ? (
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant="outline" className="font-mono">
            <KeyRound className="mr-1 size-3" /> {stored}
          </Badge>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={onClear}
          >
            Clear stored credential
          </Button>
        </div>
      ) : (
        <p className="text-muted-foreground text-xs">No credential stored.</p>
      )}
      <FormControl>
        <Input
          type="password"
          placeholder={
            stored ? "Paste a new value to replace…" : placeholder
          }
          value={draft}
          onChange={(e) => onDraftChange(e.target.value)}
          autoComplete="off"
        />
      </FormControl>
      <FormDescription>{helper}</FormDescription>
    </FormItem>
  );
}
