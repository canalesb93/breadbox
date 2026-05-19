import { useEffect, useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import {
  AlertTriangle,
  Bot,
  CheckCircle2,
  Cpu,
  Gauge,
  KeyRound,
  Loader2,
  Plug,
  Sparkles,
  Stethoscope,
  Terminal,
  Trash2,
} from "lucide-react";
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
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { SettingsSectionHeader } from "@/components/settings-section-header";
import { SectionCard } from "@/components/section-card";
import { LeaveGuard } from "@/components/leave-guard";
import { Eyebrow } from "@/components/eyebrow";
import { withMutationToast } from "@/lib/mutation-toast";
import { cn } from "@/lib/utils";
import {
  useAgentSettings,
  useAgentSubsystemStatus,
  useRunAgentCleanup,
  useSmokeTestAgent,
  useUpdateAgentSettings,
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
  const statusQuery = useAgentSubsystemStatus();
  const updateSettings = useUpdateAgentSettings();
  const smokeTest = useSmokeTestAgent();
  const cleanup = useRunAgentCleanup();

  const [tokenDraft, setTokenDraft] = useState("");
  const [apiKeyDraft, setApiKeyDraft] = useState("");

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
      // Reset baseline so LeaveGuard sees the form as clean post-save.
      form.reset(values);
    }
  });

  // The form's `isDirty` only tracks RHF-controlled fields; the masked
  // credential inputs use local `tokenDraft` / `apiKeyDraft` state, so
  // OR them in for a full dirty signal that LeaveGuard can react to.
  const isDirty =
    form.formState.isDirty || tokenDraft !== "" || apiKeyDraft !== "";

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
    try {
      const r = await smokeTest.mutateAsync();
      toast.success("Agent connection OK", {
        description: `${r.auth_mode} · ${r.model} · ${r.duration_ms}ms · $${r.total_cost_usd.toFixed(6)} (${r.input_tokens.toLocaleString()} in / ${r.output_tokens.toLocaleString()} out)`,
      });
    } catch (err) {
      const code = err instanceof ApiError ? err.code : "UNKNOWN";
      const message =
        err instanceof ApiError ? err.message : String(err);
      toast.error(`Test failed — ${code}`, {
        description: message,
      });
    }
  };

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

  const settings = settingsQuery.data;
  const status = statusQuery.data;

  return (
    <div className="space-y-6">
      <SettingsSectionHeader
        title="Agents"
        description="Authentication, runtime, and global caps for the Claude Agent SDK runner. Tokens are encrypted at rest; the full value never leaves the server after you save it."
        action={
          settings ? (
            <ReadinessPill
              authConfigured={Boolean(status?.auth_configured)}
              binaryPresent={Boolean(status?.binary_present)}
            />
          ) : null
        }
      />

      {settingsQuery.isLoading ? (
        <LoadingSkeleton />
      ) : settings ? (
        <Form {...form}>
          <LeaveGuard when={isDirty && !form.formState.isSubmitting} />
          <form onSubmit={onSubmit} className="space-y-5">
            <SectionCard
              title="Connection"
              icon={<Plug className="text-muted-foreground size-4" />}
            >
              <div className="space-y-5">
                <FormField
                  control={form.control}
                  name="auth_mode"
                  render={({ field }) => (
                    <FormItem className="space-y-3">
                      <FormLabel>Authentication mode</FormLabel>
                      <FormControl>
                        <RadioGroup
                          value={field.value}
                          onValueChange={field.onChange}
                          className="grid gap-3 sm:grid-cols-2"
                        >
                          <AuthOptionCard
                            value="subscription"
                            selected={field.value === "subscription"}
                            label="Subscription token"
                            description="Free under your Claude plan credits. Generated by `claude setup-token` — one token, one year."
                            Icon={Sparkles}
                          />
                          <AuthOptionCard
                            value="api_key"
                            selected={field.value === "api_key"}
                            label="Anthropic API key"
                            description="Pay-as-you-go. Durable past 2026-06-15 when subscription auth sunsets."
                            Icon={KeyRound}
                          />
                        </RadioGroup>
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {authMode === "subscription" ? (
                  <CredentialField
                    label="Subscription token"
                    placeholder="sk-ant-oat01-…"
                    helper={
                      <>
                        Run <code className="font-mono">claude setup-token</code>{" "}
                        on any machine, paste the resulting one-year token here.
                      </>
                    }
                    stored={settings.subscription_token}
                    draft={tokenDraft}
                    onDraftChange={setTokenDraft}
                    onClear={() => clearToken("subscription_token")}
                  />
                ) : (
                  <CredentialField
                    label="Anthropic API key"
                    placeholder="sk-ant-…"
                    helper={
                      <>
                        From{" "}
                        <a
                          href="https://console.anthropic.com"
                          target="_blank"
                          rel="noreferrer"
                          className="underline-offset-2 hover:underline"
                        >
                          console.anthropic.com
                        </a>
                        . Billed per API call.
                      </>
                    }
                    stored={settings.anthropic_api_key}
                    draft={apiKeyDraft}
                    onDraftChange={setApiKeyDraft}
                    onClear={() => clearToken("anthropic_api_key")}
                  />
                )}
              </div>
            </SectionCard>

            <SectionCard
              title="Limits"
              icon={<Gauge className="text-muted-foreground size-4" />}
            >
              <div className="grid gap-5 sm:grid-cols-2">
                <FormField
                  control={form.control}
                  name="max_concurrent"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Max concurrent runs</FormLabel>
                      <FormControl>
                        <Input
                          type="number"
                          inputMode="numeric"
                          min={1}
                          max={50}
                          {...field}
                          onChange={(e) =>
                            field.onChange(Number(e.target.value))
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        v1 default is 1. Extra triggers either skip (cron) or
                        503 (manual).
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
                      <FormLabel>Max cost per run (USD)</FormLabel>
                      <FormControl>
                        <Input
                          type="number"
                          inputMode="decimal"
                          step="0.01"
                          placeholder="No global cap"
                          {...field}
                        />
                      </FormControl>
                      <FormDescription>
                        Hard ceiling across all agents. Leave blank for none.
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
            </SectionCard>

            <SectionCard
              title="Runtime"
              icon={<Cpu className="text-muted-foreground size-4" />}
            >
              <FormField
                control={form.control}
                name="runtime_path"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>breadbox-agent binary path</FormLabel>
                    <FormControl>
                      <Input
                        placeholder="auto: $BREADBOX_AGENT_BIN, ./bin/breadbox-agent, or PATH"
                        className="font-mono text-xs"
                        autoCapitalize="none"
                        autoCorrect="off"
                        spellCheck={false}
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      Absolute path to the sidecar binary. Leave blank to use{" "}
                      <code className="font-mono">$BREADBOX_AGENT_BIN</code>,{" "}
                      <code className="font-mono">./bin/breadbox-agent</code>,
                      or PATH discovery.
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              {status?.binary_path && (
                <div className="text-muted-foreground mt-3 flex items-start gap-2 text-xs">
                  <Terminal className="mt-0.5 size-3.5 shrink-0" />
                  <span>
                    Resolved to{" "}
                    <code className="bg-muted rounded px-1 font-mono">
                      {status.binary_path}
                    </code>
                  </span>
                </div>
              )}
            </SectionCard>

            <div className="border-border bg-muted/30 -mx-1 flex flex-wrap items-center justify-between gap-3 rounded-lg border px-4 py-3">
              <div className="flex flex-wrap items-center gap-2">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={runSmokeTest}
                  disabled={smokeTest.isPending}
                >
                  {smokeTest.isPending ? (
                    <Loader2 className="size-3.5 animate-spin" />
                  ) : (
                    <Stethoscope className="size-3.5" />
                  )}
                  Test connection
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={runCleanup}
                  disabled={cleanup.isPending}
                  title="Runs the daily prune pass on demand — useful after lowering retention so you don't have to wait for 3:15 AM."
                >
                  {cleanup.isPending ? (
                    <Loader2 className="size-3.5 animate-spin" />
                  ) : (
                    <Trash2 className="size-3.5" />
                  )}
                  Run cleanup
                </Button>
              </div>
              <Button
                type="submit"
                size="sm"
                disabled={updateSettings.isPending}
              >
                {updateSettings.isPending && (
                  <Loader2 className="size-3.5 animate-spin" />
                )}
                Save settings
              </Button>
            </div>

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

// ReadinessPill — at-a-glance ready/setup signal that anchors the section
// header. Both auth and binary green → "Ready"; either missing → "Setup
// needed" in amber. Source-of-truth is `/agents/status`, the cheap probe
// that doesn't burn an Anthropic call.
function ReadinessPill({
  authConfigured,
  binaryPresent,
}: {
  authConfigured: boolean;
  binaryPresent: boolean;
}) {
  const ready = authConfigured && binaryPresent;
  if (ready) {
    return (
      <Badge
        variant="outline"
        className="border-success/30 bg-success/10 text-success gap-1.5 font-medium"
      >
        <CheckCircle2 className="size-3" />
        Ready
      </Badge>
    );
  }
  return (
    <Badge
      variant="outline"
      className="gap-1.5 border-amber-500/30 bg-amber-500/10 font-medium text-amber-700 dark:text-amber-400"
    >
      <AlertTriangle className="size-3" />
      Setup needed
    </Badge>
  );
}

function LoadingSkeleton() {
  return (
    <div className="space-y-5">
      {[0, 1, 2].map((i) => (
        <div key={i} className="border-border rounded-lg border p-5">
          <Skeleton className="mb-4 h-4 w-32" />
          <Skeleton className="mb-2 h-9 w-full" />
          <Skeleton className="h-3 w-2/3" />
        </div>
      ))}
    </div>
  );
}

interface CredentialFieldProps {
  label: string;
  placeholder: string;
  helper: React.ReactNode;
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
    <FormItem className="space-y-2">
      <div className="flex items-center justify-between gap-3">
        <FormLabel>{label}</FormLabel>
        {stored ? (
          <Badge
            variant="outline"
            className="border-success/30 bg-success/10 text-success gap-1 font-normal"
          >
            <CheckCircle2 className="size-3" />
            Connected
          </Badge>
        ) : (
          <Eyebrow className="text-muted-foreground/80">Not configured</Eyebrow>
        )}
      </div>
      {stored && (
        <div className="bg-muted/40 border-border flex items-center justify-between gap-2 rounded-md border px-3 py-2">
          <code className="text-foreground/80 truncate font-mono text-xs">
            {stored}
          </code>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="text-muted-foreground hover:text-destructive h-7 px-2 text-xs"
            onClick={onClear}
          >
            Remove
          </Button>
        </div>
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
          className="font-mono text-xs"
        />
      </FormControl>
      <FormDescription>{helper}</FormDescription>
    </FormItem>
  );
}

interface AuthOptionCardProps {
  value: string;
  selected: boolean;
  label: string;
  description: string;
  Icon: React.ComponentType<{ className?: string }>;
}

// AuthOptionCard mirrors the OptionCard pattern from api-key-form — a
// RadioGroupItem styled as a tappable tile. Kept local because the
// description ("free under Claude plan credits…") is settings-specific.
function AuthOptionCard({
  value,
  selected,
  label,
  description,
  Icon,
}: AuthOptionCardProps) {
  const id = `auth-mode-${value}`;
  return (
    <label
      htmlFor={id}
      className={cn(
        "group relative flex cursor-pointer flex-col gap-2 rounded-lg border p-4 transition-colors",
        "hover:border-foreground/30",
        selected
          ? "border-primary bg-primary/[0.04] ring-primary/30 ring-1"
          : "border-border bg-card",
      )}
    >
      <div className="flex items-start justify-between gap-2">
        <Icon
          className={cn(
            "size-4 transition-colors",
            selected ? "text-primary" : "text-muted-foreground",
          )}
        />
        <RadioGroupItem id={id} value={value} className="mt-0.5" />
      </div>
      <div className="space-y-1">
        <div className="text-sm font-medium">{label}</div>
        <p className="text-muted-foreground text-xs leading-relaxed">
          {description}
        </p>
      </div>
    </label>
  );
}

