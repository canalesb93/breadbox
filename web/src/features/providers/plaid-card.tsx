import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Building2, Loader2, Trash2, Webhook } from "lucide-react";
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
import { ConfirmDialog } from "@/components/confirm-dialog";
import { ColorRailCard } from "@/components/color-rail-card";
import { SectionCard } from "@/components/section-card";
import { FormFooter } from "@/components/form-footer";
import { IdPill } from "@/components/id-pill";
import { withMutationToast } from "@/lib/mutation-toast";
import {
  useDisableProvider,
  useUpdatePlaidConfig,
} from "@/api/queries/provider-config";
import type {
  PlaidConfigView,
  ProviderHealthResponse,
} from "@/api/types";
import { EnvLockedNotice } from "./env-locked-notice";
import {
  ProviderScoreboard,
  ProviderStatusBadge,
  providerToneAccent,
  resolveProviderTone,
} from "./provider-status";
import { TestConnectionButton } from "./test-connection-button";

const ENVS = ["sandbox", "development", "production"] as const;

const schema = z
  .object({
    client_id: z.string().min(1, "Client ID is required"),
    secret: z.string(),
    environment: z.enum(ENVS),
    webhook_url: z.string(),
    secret_already_set: z.boolean(),
  })
  .refine((v) => v.secret_already_set || v.secret.trim().length > 0, {
    path: ["secret"],
    message: "Secret is required",
  })
  .refine(
    (v) => {
      const url = v.webhook_url.trim();
      return url === "" || url.startsWith("https://");
    },
    { path: ["webhook_url"], message: "Must start with https://" },
  );

type Values = z.infer<typeof schema>;

interface PlaidCardProps {
  config: PlaidConfigView;
  health: ProviderHealthResponse | undefined;
}

export function PlaidCard({ config, health }: PlaidCardProps) {
  const update = useUpdatePlaidConfig();
  const disable = useDisableProvider();
  const [confirmingDisable, setConfirmingDisable] = useState(false);

  const form = useForm<Values>({
    resolver: zodResolver(schema),
    values: {
      client_id: config.client_id ?? "",
      secret: "",
      environment: (config.environment as (typeof ENVS)[number]) || "sandbox",
      webhook_url: config.webhook_url ?? "",
      secret_already_set: config.secret_set,
    },
  });

  async function onSubmit(values: Values) {
    const body = {
      client_id: values.client_id.trim(),
      secret: values.secret.trim() === "" ? null : values.secret.trim(),
      environment: values.environment,
      webhook_url: values.webhook_url.trim(),
    };
    const ok = await withMutationToast(() => update.mutateAsync(body), {
      success: "Plaid settings saved.",
    });
    if (ok) form.resetField("secret");
  }

  async function onDisable() {
    const ok = await withMutationToast(() => disable.mutateAsync("plaid"), {
      success: "Plaid disabled. Stored credentials were cleared.",
    });
    if (ok) setConfirmingDisable(false);
  }

  const tone = resolveProviderTone(health, config.configured);

  return (
    <div className="space-y-4">
      <ColorRailCard accent={providerToneAccent(tone)}>
        <div className="flex flex-col gap-5 px-6 py-5 sm:flex-row sm:items-center sm:justify-between sm:px-7">
          <div className="flex min-w-0 items-start gap-3">
            <div className="bg-blue-500/10 text-blue-600 dark:text-blue-400 flex size-11 shrink-0 items-center justify-center rounded-lg">
              <Building2 className="size-5" />
            </div>
            <div className="min-w-0 space-y-1">
              <div className="text-muted-foreground text-[11px] font-medium tracking-[0.08em] uppercase">
                Provider
              </div>
              <div className="flex items-center gap-2">
                <h2 className="text-foreground text-lg font-semibold tracking-tight">
                  Plaid
                </h2>
                <ProviderStatusBadge health={health} configured={config.configured} />
              </div>
              <p className="text-muted-foreground max-w-md text-sm">
                Thousands of financial institutions across the US, Canada, and Europe.
              </p>
            </div>
          </div>
          <ProviderScoreboard health={health} tone={tone} />
        </div>
      </ColorRailCard>

      <SectionCard
        title="Credentials"
        icon={<Building2 className="text-muted-foreground size-4" />}
      >
        {config.from_env ? (
          <div className="space-y-4">
            <EnvLockedNotice provider="Plaid" />
            <dl className="grid grid-cols-1 gap-y-3 text-sm sm:grid-cols-[max-content_1fr] sm:gap-x-6">
              <Row label="Client ID" value={config.client_id ?? "—"} mono />
              <Row label="Secret" value={config.secret_set ? "••••••••" : "Not set"} mono />
              <Row label="Environment" value={config.environment ?? "—"} />
              <Row label="Webhook URL" value={config.webhook_url || "Not set"} mono />
            </dl>
          </div>
        ) : (
          <Form {...form}>
            <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-5">
              <div className="grid gap-4 sm:grid-cols-2">
                <FormField
                  control={form.control}
                  name="client_id"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Client ID</FormLabel>
                      <FormControl>
                        <Input placeholder="Plaid Client ID" autoComplete="off" {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name="environment"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Environment</FormLabel>
                      <Select value={field.value} onValueChange={field.onChange}>
                        <FormControl>
                          <SelectTrigger className="w-full">
                            <SelectValue />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          {ENVS.map((e) => (
                            <SelectItem key={e} value={e}>
                              {e[0].toUpperCase() + e.slice(1)}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              <FormField
                control={form.control}
                name="secret"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Secret</FormLabel>
                    <FormControl>
                      <Input
                        type="password"
                        autoComplete="new-password"
                        placeholder={
                          config.secret_set
                            ? "Unchanged (enter a new value to rotate)"
                            : "Plaid secret"
                        }
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      Credentials are validated against Plaid before they're stored.
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name="webhook_url"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Webhook URL</FormLabel>
                    <FormControl>
                      <Input
                        type="url"
                        placeholder="https://your-domain.com/webhooks/plaid"
                        autoComplete="off"
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      Plaid pushes real-time transaction updates to this URL during link-token creation.
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormFooter
                secondary={
                  config.configured ? (
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="text-muted-foreground hover:text-destructive"
                      onClick={() => setConfirmingDisable(true)}
                    >
                      <Trash2 className="size-3.5" />
                      Disable
                    </Button>
                  ) : null
                }
                primary={
                  <Button type="submit" size="sm" disabled={update.isPending}>
                    {update.isPending && <Loader2 className="size-4 animate-spin" />}
                    {update.isPending ? "Saving…" : "Save settings"}
                  </Button>
                }
              />
            </form>
          </Form>
        )}
      </SectionCard>

      {config.configured && (
        <SectionCard
          title="Diagnostics"
          icon={<Webhook className="text-muted-foreground size-4" />}
          action={<TestConnectionButton provider="plaid" />}
        >
          <div className="text-muted-foreground space-y-3 text-xs">
            <p className="text-foreground text-sm font-medium">Webhook endpoint</p>
            <p>
              Plaid automatically subscribes to webhooks when the URL above is set during
              link-token creation. No separate Plaid-dashboard step needed.
            </p>
            <div>
              <IdPill value="https://<your-domain>/webhooks/plaid" />
            </div>
          </div>
        </SectionCard>
      )}

      <ConfirmDialog
        open={confirmingDisable}
        onOpenChange={setConfirmingDisable}
        icon={Trash2}
        title="Disable Plaid?"
        description="Stored credentials will be deleted from the database. Existing Plaid connections stay in your household but syncs will fail until you re-enter credentials."
        confirmLabel="Disable Plaid"
        pendingLabel="Disabling…"
        pending={disable.isPending}
        onConfirm={onDisable}
      />
    </div>
  );
}

function Row({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <>
      <dt className="text-muted-foreground">{label}</dt>
      <dd className={mono ? "font-mono text-xs break-all" : ""}>{value}</dd>
    </>
  );
}
