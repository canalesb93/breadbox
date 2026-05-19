import { useMemo, useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { Building2, Landmark, Loader2 } from "lucide-react";
import { Sheet, SheetContent } from "@/components/ui/sheet";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Eyebrow } from "@/components/eyebrow";
import { DetailSheetHeader } from "@/components/detail-sheet-header";
import { StatusPanel } from "@/components/status-panel";
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
//
// Iter 40 polish: header gets the v2 icon-tile lockup (Landmark in a muted
// rounded tile) matching `EmptyState` / `StatusPanel` / `SectionCard`;
// labels move onto `<Eyebrow>`; the picker selection vocabulary inherits
// the active-state language used by nav/list rows; alerts adopt
// `<StatusPanel>` so tone is consistent with Providers + Setup; the
// action strip uses the canonical `<FormFooter>`-style flush bordered
// footer (open-coded here because the host is a Sheet, not a SectionCard
// — the negative-margin trick doesn't apply).
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

  const headerIcon = (() => {
    if (appendToConnectionId) return Building2;
    return Landmark;
  })();
  const headerTitle = appendToConnectionId ? "Import more rows" : "Connect a bank";
  const headerDescription = appendToConnectionId
    ? "Upload another statement to append rows to this CSV connection."
    : "Pick a provider and the family member this connection belongs to.";

  return (
    <Sheet open={open} onOpenChange={handleSheetChange}>
      <SheetContent className="flex flex-col gap-0 p-0">
        <DetailSheetHeader
          density="accent"
          icon={headerIcon}
          eyebrow={appendToConnectionId ? "Append rows" : "New connection"}
          title={headerTitle}
          description={headerDescription}
        />

        <div className="flex flex-1 flex-col overflow-y-auto p-6">
          {stage.kind === "pick" && (
            <div className="flex flex-col gap-6">
              {providersQuery.isLoading ? (
                <div className="text-muted-foreground flex items-center gap-2 text-sm">
                  <Loader2 className="size-4 animate-spin" /> Loading providers…
                </div>
              ) : enabledProviders.length === 0 ? (
                <StatusPanel
                  tone="info"
                  icon={Landmark}
                  heading="No bank providers configured"
                  body="Set up Plaid or Teller credentials on this server, or import a CSV statement."
                />
              ) : (
                <div className="flex flex-col gap-2">
                  <Eyebrow as="p">Provider</Eyebrow>
                  <ProviderPicker
                    enabledProviders={enabledProviders}
                    value={provider}
                    onChange={setProvider}
                  />
                </div>
              )}

              <div className="flex flex-col gap-2">
                <Eyebrow as="label" htmlFor="connect-bank-user">
                  Family member
                </Eyebrow>
                <Select value={effectiveUserId} onValueChange={setUserId}>
                  <SelectTrigger id="connect-bank-user">
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
                <StatusPanel
                  tone="destructive"
                  icon={Landmark}
                  heading="Couldn't save the connection"
                  body={createError}
                />
              )}
            </div>
          )}

          {(stage.kind === "plaid" || stage.kind === "teller") && (
            <div className="flex flex-1 flex-col items-center justify-center gap-4 py-8 text-center">
              <span className="bg-primary/10 text-primary flex size-12 items-center justify-center rounded-xl border border-primary/20">
                <Loader2 className="size-5 animate-spin" />
              </span>
              <div className="flex flex-col gap-1">
                <Eyebrow as="p">Hand-off</Eyebrow>
                <p className="text-foreground text-sm font-medium">
                  Opening {PROVIDER_META[stage.kind]?.name ?? stage.kind}…
                </p>
                <p className="text-muted-foreground max-w-xs text-xs leading-relaxed">
                  Finish the flow in the popup. Closing it brings you back here.
                </p>
              </div>
              <Button
                variant="outline"
                size="sm"
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
        </div>

        {stage.kind === "pick" && (
          <div className="bg-muted/20 flex flex-col-reverse items-stretch gap-2 border-t px-4 py-3 sm:flex-row sm:items-center sm:justify-end sm:px-6">
            <Button
              variant="outline"
              size="sm"
              onClick={() => handleSheetChange(false)}
              disabled={linkSession.isPending}
              className="w-full sm:w-auto"
            >
              Cancel
            </Button>
            <Button
              size="sm"
              onClick={onContinue}
              disabled={continueDisabled}
              className="w-full sm:w-auto"
            >
              {linkSession.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : null}
              Continue
            </Button>
          </div>
        )}
      </SheetContent>
    </Sheet>
  );
}
