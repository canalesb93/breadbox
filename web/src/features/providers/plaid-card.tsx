import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Building2, Trash2, Webhook } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
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
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
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
import { ProviderStats, ProviderStatusBadge } from "./provider-status";
import { TestConnectionButton } from "./test-connection-button";

const ENVS = ["sandbox", "development", "production"] as const;

// We don't validate the secret as required at the schema level. When the
// provider already has a stored secret, an empty input means "keep the
// existing one" — that's the same UX as the v1 admin form. The schema
// `.refine` below enforces required only for first-time setup.
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
    setConfirmingDisable(false);
    await withMutationToast(() => disable.mutateAsync("plaid"), {
      success: "Plaid disabled. Stored credentials were cleared.",
    });
  }

  return (
    <Card className="overflow-hidden">
      <CardHeader className="border-b">
        <div className="flex items-center gap-3">
          <div className="bg-blue-500/10 text-blue-600 dark:text-blue-400 flex size-10 items-center justify-center rounded-lg">
            <Building2 className="size-5" />
          </div>
          <div className="min-w-0 flex-1">
            <CardTitle className="text-base">Plaid</CardTitle>
            <CardDescription className="text-xs">
              Thousands of financial institutions across the US, Canada, and Europe.
            </CardDescription>
          </div>
          <ProviderStatusBadge health={health} configured={config.configured} />
        </div>
      </CardHeader>

      <CardContent className="space-y-6 pt-2">
        <ProviderStats health={health} />

        {config.from_env ? (
          <>
            <EnvLockedNotice provider="Plaid" />
            <dl className="grid grid-cols-1 gap-y-3 text-sm sm:grid-cols-[max-content_1fr] sm:gap-x-6">
              <Row label="Client ID" value={config.client_id ?? "—"} mono />
              <Row label="Secret" value={config.secret_set ? "••••••••" : "Not set"} mono />
              <Row label="Environment" value={config.environment ?? "—"} />
              <Row label="Webhook URL" value={config.webhook_url || "Not set"} mono />
            </dl>
          </>
        ) : (
          <Form {...form}>
            <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
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

              <div className="flex flex-wrap items-center justify-between gap-3 pt-1">
                <Button type="submit" disabled={update.isPending}>
                  {update.isPending ? "Saving…" : "Save Plaid settings"}
                </Button>
                {config.configured && (
                  <AlertDialog open={confirmingDisable} onOpenChange={setConfirmingDisable}>
                    <AlertDialogTrigger asChild>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        className="text-muted-foreground hover:text-destructive"
                      >
                        <Trash2 className="size-3.5" />
                        Disable Plaid
                      </Button>
                    </AlertDialogTrigger>
                    <AlertDialogContent>
                      <AlertDialogHeader>
                        <AlertDialogTitle>Disable Plaid?</AlertDialogTitle>
                        <AlertDialogDescription>
                          Stored credentials will be deleted from the database. Existing Plaid connections stay in your
                          household but syncs will fail until you re-enter credentials.
                        </AlertDialogDescription>
                      </AlertDialogHeader>
                      <AlertDialogFooter>
                        <AlertDialogCancel>Cancel</AlertDialogCancel>
                        <AlertDialogAction
                          onClick={onDisable}
                          className="bg-destructive text-white hover:bg-destructive/90"
                        >
                          Disable
                        </AlertDialogAction>
                      </AlertDialogFooter>
                    </AlertDialogContent>
                  </AlertDialog>
                )}
              </div>
            </form>
          </Form>
        )}

        {config.configured && (
          <div className="border-t pt-4">
            <TestConnectionButton provider="plaid" />
          </div>
        )}

        {config.configured && (
          <details className="group rounded-md border bg-muted/30 px-3 py-2 text-sm">
            <summary className="text-muted-foreground hover:text-foreground flex cursor-pointer items-center gap-2 text-xs font-medium">
              <Webhook className="size-3.5" />
              Webhook setup
            </summary>
            <div className="text-muted-foreground mt-3 space-y-2 text-xs">
              <p>
                Plaid automatically subscribes to webhooks when the URL above is set during link-token creation. No
                separate Plaid-dashboard step needed.
              </p>
              <p>Point Plaid at:</p>
              <code className="bg-background block break-all rounded border px-2 py-1.5 font-mono text-xs select-all">
                https://&lt;your-domain&gt;/webhooks/plaid
              </code>
            </div>
          </details>
        )}
      </CardContent>
    </Card>
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
