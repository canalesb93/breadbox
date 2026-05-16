import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import {
  AlertTriangle,
  Check,
  FileKey2,
  Landmark,
  Trash2,
  Upload,
  Webhook,
} from "lucide-react";
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
  Alert,
  AlertDescription,
  AlertTitle,
} from "@/components/ui/alert";
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
import { toast } from "sonner";
import { withMutationToast } from "@/lib/mutation-toast";
import {
  useDisableProvider,
  useUpdateTellerConfig,
} from "@/api/queries/provider-config";
import type {
  ProviderHealthResponse,
  TellerConfigView,
} from "@/api/types";
import { EnvLockedNotice } from "./env-locked-notice";
import { ProviderStats, ProviderStatusBadge } from "./provider-status";
import { TestConnectionButton } from "./test-connection-button";

const ENVS = ["sandbox", "development", "production"] as const;

const schema = z.object({
  application_id: z.string().min(1, "Application ID is required"),
  environment: z.enum(ENVS),
  webhook_secret: z.string(),
});

type Values = z.infer<typeof schema>;

interface TellerCardProps {
  config: TellerConfigView;
  health: ProviderHealthResponse | undefined;
  hasEncryptionKey: boolean;
}

// readPemFile reads an uploaded PEM file (cert or key). Returns null if the
// file is empty or unreadable so the submit can short-circuit.
async function readPemFile(file: File | undefined): Promise<string | null> {
  if (!file) return null;
  const text = await file.text();
  return text.trim() === "" ? null : text;
}

export function TellerCard({ config, health, hasEncryptionKey }: TellerCardProps) {
  const update = useUpdateTellerConfig();
  const disable = useDisableProvider();
  const [certFile, setCertFile] = useState<File | undefined>();
  const [keyFile, setKeyFile] = useState<File | undefined>();
  const [confirmingDisable, setConfirmingDisable] = useState(false);

  const form = useForm<Values>({
    resolver: zodResolver(schema),
    values: {
      application_id: config.application_id ?? "",
      environment: (config.environment as (typeof ENVS)[number]) || "sandbox",
      webhook_secret: "",
    },
  });

  async function onSubmit(values: Values) {
    if ((certFile && !keyFile) || (!certFile && keyFile)) {
      toast.error("Upload both certificate and private key, or neither.");
      return;
    }
    if (certFile && keyFile && !hasEncryptionKey) {
      toast.error("ENCRYPTION_KEY must be set on the server before certificates can be stored.");
      return;
    }

    const [cert, key] = await Promise.all([readPemFile(certFile), readPemFile(keyFile)]);

    const ok = await withMutationToast(
      () =>
        update.mutateAsync({
          application_id: values.application_id.trim(),
          environment: values.environment,
          certificate: cert,
          private_key: key,
          webhook_secret:
            values.webhook_secret.trim() === "" ? null : values.webhook_secret.trim(),
        }),
      { success: "Teller settings saved." },
    );
    if (ok) {
      form.resetField("webhook_secret");
      setCertFile(undefined);
      setKeyFile(undefined);
    }
  }

  async function onDisable() {
    setConfirmingDisable(false);
    await withMutationToast(() => disable.mutateAsync("teller"), {
      success: "Teller disabled. Stored credentials were cleared.",
    });
  }

  return (
    <Card className="overflow-hidden">
      <CardHeader className="border-b">
        <div className="flex items-center gap-3">
          <div className="bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 flex size-10 items-center justify-center rounded-lg">
            <Landmark className="size-5" />
          </div>
          <div className="min-w-0 flex-1">
            <CardTitle className="text-base">Teller</CardTitle>
            <CardDescription className="text-xs">
              US bank coverage via mutual-TLS authentication.
            </CardDescription>
          </div>
          <ProviderStatusBadge health={health} configured={config.configured} />
        </div>
      </CardHeader>

      <CardContent className="space-y-6 pt-2">
        <ProviderStats health={health} />

        {config.from_env ? (
          <>
            <EnvLockedNotice provider="Teller" />
            <dl className="grid grid-cols-1 gap-y-3 text-sm sm:grid-cols-[max-content_1fr] sm:gap-x-6">
              <Row label="Application ID" value={config.application_id ?? "—"} mono />
              <Row label="Environment" value={config.environment ?? "—"} />
              <Row label="Certificate" value={config.certificate_set ? "Configured" : "Not configured"} />
              <Row
                label="Webhook secret"
                value={config.webhook_secret_set ? "Configured" : "Not configured"}
              />
            </dl>
          </>
        ) : (
          <Form {...form}>
            <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
              <div className="grid gap-4 sm:grid-cols-2">
                <FormField
                  control={form.control}
                  name="application_id"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Application ID</FormLabel>
                      <FormControl>
                        <Input placeholder="app_..." autoComplete="off" {...field} />
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

              {!hasEncryptionKey && (
                <Alert variant="default" className="border-amber-500/40 bg-amber-500/5">
                  <AlertTriangle className="size-4 text-amber-600 dark:text-amber-400" />
                  <AlertTitle className="text-amber-700 dark:text-amber-400">
                    ENCRYPTION_KEY is not set
                  </AlertTitle>
                  <AlertDescription>
                    Teller certificates are encrypted at rest. Set the <code className="font-mono text-xs">ENCRYPTION_KEY</code>{" "}
                    environment variable before uploading a certificate.
                  </AlertDescription>
                </Alert>
              )}

              <div className="space-y-3 rounded-lg border bg-muted/30 p-4">
                <div className="flex items-center gap-2">
                  <FileKey2 className="text-muted-foreground size-4" />
                  <span className="text-sm font-medium">mTLS certificate</span>
                  {config.certificate_set && (
                    <span className="text-emerald-600 dark:text-emerald-400 flex items-center gap-1 text-xs">
                      <Check className="size-3" />
                      Stored
                    </span>
                  )}
                </div>
                <div className="grid gap-3 sm:grid-cols-2">
                  <PemFileInput
                    id="teller-cert"
                    label="Certificate (.pem)"
                    accept=".pem,.crt,.cert"
                    file={certFile}
                    onChange={setCertFile}
                  />
                  <PemFileInput
                    id="teller-key"
                    label="Private key (.pem)"
                    accept=".pem,.key"
                    file={keyFile}
                    onChange={setKeyFile}
                  />
                </div>
                <p className="text-muted-foreground text-xs">
                  Upload both files together to {config.certificate_set ? "rotate" : "configure"} the certificate. Leave blank to{" "}
                  {config.certificate_set ? "keep the stored pair" : "skip"}.
                </p>
              </div>

              <FormField
                control={form.control}
                name="webhook_secret"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Webhook signing secret</FormLabel>
                    <FormControl>
                      <Input
                        type="password"
                        autoComplete="new-password"
                        placeholder={
                          config.webhook_secret_set
                            ? "Unchanged (enter a new value to rotate)"
                            : "Optional"
                        }
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      Used to verify HMAC signatures on incoming Teller webhooks.
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <div className="flex flex-wrap items-center justify-between gap-3 pt-1">
                <Button type="submit" disabled={update.isPending}>
                  {update.isPending ? "Saving…" : "Save Teller settings"}
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
                        Disable Teller
                      </Button>
                    </AlertDialogTrigger>
                    <AlertDialogContent>
                      <AlertDialogHeader>
                        <AlertDialogTitle>Disable Teller?</AlertDialogTitle>
                        <AlertDialogDescription>
                          Stored credentials (application ID, webhook secret, and encrypted certificate) will be
                          deleted from the database. Existing Teller connections stay in your household but syncs
                          will fail until you re-enter credentials.
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
            <TestConnectionButton provider="teller" />
          </div>
        )}

        {config.configured && (
          <details className="group rounded-md border bg-muted/30 px-3 py-2 text-sm">
            <summary className="text-muted-foreground hover:text-foreground flex cursor-pointer items-center gap-2 text-xs font-medium">
              <Webhook className="size-3.5" />
              Webhook setup
            </summary>
            <div className="text-muted-foreground mt-3 space-y-2 text-xs">
              <ol className="ml-4 list-decimal space-y-1">
                <li>Open the Teller dashboard and navigate to your application settings.</li>
                <li>Set the webhook URL to:</li>
              </ol>
              <code className="bg-background block break-all rounded border px-2 py-1.5 font-mono text-xs select-all">
                https://&lt;your-domain&gt;/webhooks/teller
              </code>
              <ol className="ml-4 list-decimal space-y-1" start={3}>
                <li>Copy the webhook signing secret from Teller and paste it above.</li>
              </ol>
            </div>
          </details>
        )}
      </CardContent>
    </Card>
  );
}

interface PemFileInputProps {
  id: string;
  label: string;
  accept: string;
  file: File | undefined;
  onChange: (file: File | undefined) => void;
}

function PemFileInput({ id, label, accept, file, onChange }: PemFileInputProps) {
  return (
    <div className="space-y-1.5">
      <label htmlFor={id} className="text-xs font-medium">
        {label}
      </label>
      <label
        htmlFor={id}
        className="border-input hover:bg-muted/60 flex cursor-pointer items-center gap-2 rounded-md border border-dashed bg-background px-3 py-2 text-xs transition-colors"
      >
        <Upload className="text-muted-foreground size-3.5" />
        <span className={file ? "text-foreground truncate" : "text-muted-foreground"}>
          {file ? file.name : "Choose file"}
        </span>
      </label>
      <input
        id={id}
        type="file"
        accept={accept}
        className="hidden"
        onChange={(e) => onChange(e.target.files?.[0])}
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
