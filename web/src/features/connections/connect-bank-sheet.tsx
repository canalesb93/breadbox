import { useMemo, useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { Loader2 } from "lucide-react";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { toast } from "sonner";
import { ApiError } from "@/api/client";
import {
  useCreateConnection,
  useProviderLinkSession,
  useProviders,
  type CreateConnectionInput,
} from "@/api/queries/connections";
import { useUsers } from "@/api/queries/users";
import { ProviderPicker, PROVIDER_META } from "./provider-picker";
import { PlaidLinkButton } from "./plaid-link-button";
import { TellerConnectButton } from "./teller-connect-button";
import { CsvImportForm } from "./csv-import-form";

interface ConnectBankSheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  // When set, the Sheet skips step 1 and renders the CSV form directly,
  // pre-targeted to append to this connection. Used by the connection-
  // detail page's "Import more" button so admins can append rows to an
  // existing CSV connection without re-picking a provider.
  appendToConnectionId?: string;
}

// Stage drives the inner switch between provider-pick (step 1) and the
// active provider hand-off (step 2). Returning to step 1 from a hand-off
// (cancel / error) is just `setStage("pick")` — nothing about the SDK
// component lives across that transition since both wrappers re-mount when
// they re-appear.
type Stage =
  | { kind: "pick" }
  | { kind: "plaid"; userId: string; linkToken: string }
  | { kind: "teller"; userId: string; applicationId: string }
  | { kind: "csv"; userId: string };

// ConnectBankSheet is the real Connect-bank flow that replaces the v1
// /connections/new wizard for the v2 SPA. Two steps inside one Sheet:
//
//   1. Pick provider + family member, click Continue. We call
//      useProviderLinkSession() to get a link token (Plaid only — Teller
//      returns 204 because Teller Connect runs entirely client-side).
//   2. Render the matching provider launcher (PlaidLinkButton or
//      TellerConnectButton) which auto-opens its hosted UI. On success the
//      launcher hands back the public_token / enrollment payload, we POST
//      to the generic /api/v1/connections endpoint, toast success, and
//      navigate to the new connection's detail page.
export function ConnectBankSheet({
  open,
  onOpenChange,
  appendToConnectionId,
}: ConnectBankSheetProps) {
  const navigate = useNavigate();
  const providersQuery = useProviders();
  const usersQuery = useUsers();
  const linkSession = useProviderLinkSession();
  const createConnection = useCreateConnection();

  const [provider, setProvider] = useState<string | null>(null);
  const [userId, setUserId] = useState<string>("");
  const [stage, setStage] = useState<Stage>(
    appendToConnectionId ? { kind: "csv", userId: "" } : { kind: "pick" },
  );
  const [createError, setCreateError] = useState<string | null>(null);

  // CSV is always available (the importer is in-binary, not a provider
  // requiring credentials). Plaid + Teller cards are gated on the
  // /api/v1/providers `configured` flag.
  const enabledProviders = useMemo(() => {
    const fromApi = (providersQuery.data ?? [])
      .filter((p) => p.configured && p.name !== "csv")
      .map((p) => p.name);
    return [...fromApi, "csv"];
  }, [providersQuery.data]);

  // Default the family member to the only household member when there's just
  // one, so the picker doesn't render a single-option dropdown the user has
  // to acknowledge. Multi-user households start blank.
  const users = usersQuery.data ?? [];
  const effectiveUserId =
    userId || (users.length === 1 ? users[0].id : "");

  function reset() {
    setProvider(null);
    setUserId("");
    // When the Sheet is bound to an existing CSV connection (append mode)
    // it stays on the CSV stage across re-opens; the picker is irrelevant.
    setStage(
      appendToConnectionId ? { kind: "csv", userId: "" } : { kind: "pick" },
    );
    setCreateError(null);
    linkSession.reset();
    createConnection.reset();
  }

  function handleSheetChange(next: boolean) {
    if (!next) reset();
    onOpenChange(next);
  }

  async function onContinue() {
    if (!provider || !effectiveUserId) return;
    setCreateError(null);
    // CSV doesn't need a server-issued link session — skip straight to the
    // upload form. The form itself owns the file → preview → import flow.
    if (provider === "csv") {
      setStage({ kind: "csv", userId: effectiveUserId });
      return;
    }
    try {
      const session = await linkSession.mutateAsync({
        provider,
        userId: effectiveUserId,
      });
      if (provider === "plaid") {
        if (!session?.link_token) {
          toast.error("Plaid did not return a link token. Try again.");
          return;
        }
        setStage({
          kind: "plaid",
          userId: effectiveUserId,
          linkToken: session.link_token,
        });
      } else if (provider === "teller") {
        // Teller's link-session endpoint returns the server-configured
        // application_id as link_token; Teller Connect is initialized with
        // it client-side. An empty token means the server has no app_id set.
        const applicationId = session?.link_token ?? "";
        if (!applicationId) {
          toast.error(
            "Teller application ID is not configured on this server.",
          );
          return;
        }
        setStage({ kind: "teller", userId: effectiveUserId, applicationId });
      }
    } catch (err) {
      const msg =
        err instanceof ApiError
          ? err.message
          : "Couldn't start the connection flow. Try again.";
      toast.error(msg);
    }
  }

  async function onPlaidSuccess(input: {
    publicToken: string;
    metadata: {
      institution: { institution_id: string; name: string } | null;
      accounts: { id: string; name?: string; mask?: string; type?: string; subtype?: string }[];
    };
  }) {
    if (stage.kind !== "plaid") return;
    const { metadata, publicToken } = input;
    if (!metadata.institution) {
      toast.error("Plaid did not return an institution. Try again.");
      setStage({ kind: "pick" });
      return;
    }
    const payload: CreateConnectionInput = {
      provider: "plaid",
      user_id: stage.userId,
      credentials: {
        public_token: publicToken,
        institution_id: metadata.institution.institution_id,
        institution_name: metadata.institution.name,
        accounts: metadata.accounts ?? [],
      },
    };
    await runCreate(payload);
  }

  async function onTellerSuccess(result: {
    accessToken: string;
    enrollmentId: string;
    institutionName: string;
    institutionId: string;
  }) {
    if (stage.kind !== "teller") return;
    const payload: CreateConnectionInput = {
      provider: "teller",
      user_id: stage.userId,
      credentials: {
        access_token: result.accessToken,
        enrollment_id: result.enrollmentId,
        institution_id: result.institutionId,
        institution_name: result.institutionName,
      },
    };
    await runCreate(payload);
  }

  async function runCreate(payload: CreateConnectionInput) {
    setCreateError(null);
    try {
      const result = await createConnection.mutateAsync(payload);
      toast.success(`Connected ${result.institution_name}.`, {
        description: "Initial sync queued — accounts and transactions will appear shortly.",
      });
      handleSheetChange(false);
      navigate({
        to: "/connections/$id",
        params: { id: result.connection_id },
      });
    } catch (err) {
      const msg =
        err instanceof ApiError
          ? err.message
          : "Couldn't save the connection. Try again.";
      setCreateError(msg);
      toast.error(msg);
      // Drop back to the picker so the user can retry without re-launching
      // the SDK in a half-broken state.
      setStage({ kind: "pick" });
    }
  }

  function onLaunchExit(message: string | null) {
    if (message) toast.error(message);
    setStage({ kind: "pick" });
  }

  const continueDisabled =
    !provider ||
    !effectiveUserId ||
    linkSession.isPending ||
    createConnection.isPending;

  return (
    <Sheet open={open} onOpenChange={handleSheetChange}>
      <SheetContent className="flex flex-col gap-6 p-6">
        <SheetHeader className="p-0">
          <SheetTitle>Connect a bank</SheetTitle>
          <SheetDescription>
            Pick a provider and the family member this connection belongs to.
          </SheetDescription>
        </SheetHeader>

        {stage.kind === "pick" && (
          <div className="flex flex-1 flex-col gap-6 overflow-y-auto">
            {providersQuery.isLoading ? (
              <div className="text-muted-foreground flex items-center gap-2 text-sm">
                <Loader2 className="size-4 animate-spin" /> Loading providers…
              </div>
            ) : enabledProviders.length === 0 ? (
              <Alert variant="default">
                <AlertTitle>No bank providers configured</AlertTitle>
                <AlertDescription>
                  Set up Plaid or Teller credentials on this server, or import
                  a CSV statement.
                </AlertDescription>
              </Alert>
            ) : (
              <div className="flex flex-col gap-2">
                <Label className="text-xs uppercase tracking-wide text-muted-foreground">
                  Provider
                </Label>
                <ProviderPicker
                  enabledProviders={enabledProviders}
                  value={provider}
                  onChange={setProvider}
                />
              </div>
            )}

            <div className="flex flex-col gap-2">
              <Label className="text-xs uppercase tracking-wide text-muted-foreground">
                Family member
              </Label>
              <Select value={effectiveUserId} onValueChange={setUserId}>
                <SelectTrigger>
                  <SelectValue placeholder="Pick a household member" />
                </SelectTrigger>
                <SelectContent>
                  {users.map((u) => (
                    <SelectItem key={u.id} value={u.id}>
                      {u.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {createError && (
              <Alert variant="destructive">
                <AlertTitle>Couldn't save the connection</AlertTitle>
                <AlertDescription>{createError}</AlertDescription>
              </Alert>
            )}
          </div>
        )}

        {(stage.kind === "plaid" || stage.kind === "teller") && (
          <div className="flex flex-1 flex-col items-center justify-center gap-3 text-center">
            <Loader2 className="text-muted-foreground size-6 animate-spin" />
            <div className="text-sm font-medium">
              Opening {PROVIDER_META[stage.kind]?.name ?? stage.kind}…
            </div>
            <p className="text-muted-foreground max-w-xs text-xs">
              Finish the flow in the popup. Closing it brings you back here.
            </p>
            <Button
              variant="outline"
              size="sm"
              className="mt-2"
              onClick={() => setStage({ kind: "pick" })}
            >
              Cancel
            </Button>
          </div>
        )}

        {stage.kind === "csv" && (
          <CsvImportForm
            appendToConnectionId={appendToConnectionId}
            userId={stage.userId || undefined}
            onSuccess={(result) => {
              toast.success(
                result.appended
                  ? `Imported ${result.imported} more transactions.`
                  : `Imported ${result.imported} transactions.`,
                {
                  description: result.appended
                    ? "Appended to the existing connection — new rows will appear after re-categorisation."
                    : "Connection created. Categorise or apply rules to enrich the new rows.",
                },
              );
              handleSheetChange(false);
              if (!result.appended) {
                navigate({
                  to: "/connections/$id",
                  params: { id: result.connection_id },
                });
              }
            }}
            onCancel={() => {
              if (appendToConnectionId) {
                handleSheetChange(false);
              } else {
                setStage({ kind: "pick" });
              }
            }}
          />
        )}

        {stage.kind === "plaid" && (
          <PlaidLinkButton
            linkToken={stage.linkToken}
            onSuccess={onPlaidSuccess}
            onExit={(err) =>
              onLaunchExit(
                err
                  ? err.display_message ||
                      err.error_message ||
                      "Plaid Link exited with an error."
                  : null,
              )
            }
          />
        )}

        {stage.kind === "teller" && (
          <TellerConnectButton
            applicationId={stage.applicationId}
            onSuccess={onTellerSuccess}
            onExit={() => onLaunchExit(null)}
            onFailure={(f) =>
              onLaunchExit(f.message || "Teller Connect failed.")
            }
          />
        )}

        {stage.kind === "pick" && (
          <SheetFooter className="px-0">
            <Button
              variant="outline"
              onClick={() => handleSheetChange(false)}
              disabled={linkSession.isPending}
            >
              Cancel
            </Button>
            <Button onClick={onContinue} disabled={continueDisabled}>
              {linkSession.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : null}
              Continue
            </Button>
          </SheetFooter>
        )}
      </SheetContent>
    </Sheet>
  );
}
