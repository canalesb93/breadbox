import { useEffect, useRef, useState } from "react";
import { Loader2, ShieldAlert } from "lucide-react";
import { Sheet, SheetContent } from "@/components/ui/sheet";
import { Button } from "@/components/ui/button";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { DetailSheetHeader } from "@/components/detail-sheet-header";
import { toast } from "sonner";
import { ApiError } from "@/api/client";
import {
  useConnection,
  useReauthComplete,
  useReauthStart,
} from "@/api/queries/connections";
import { PlaidLinkButton } from "./plaid-link-button";
import { TellerConnectButton } from "./teller-connect-button";

interface ReauthSheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  // Pass the connection's short_id only — the Sheet fetches its own record so
  // the ⋯ menu and the list-page row don't have to thread the full
  // ConnectionDetail through. `undefined` keeps the Sheet inert (the open
  // toggle gates rendering anyway, but enabled queries also need a key).
  connectionShortId: string | undefined;
}

// Stage drives the inner switch between confirmation, Plaid update-mode
// launch, Teller reconnection-mode launch, and the post-success completion
// call. CSV connections never reach here in practice (the Connect/Detail
// pages don't surface a re-auth affordance for them) but we render an N/A
// stage for hand-crafted URLs rather than crash. Teller needs both the
// enrollment id (carries the existing connection) and the application id
// (boots Teller Connect); both ride on the reauth response.
type Stage =
  | { kind: "confirm" }
  | { kind: "plaid"; linkToken: string }
  | { kind: "teller"; enrollmentId: string; applicationId: string }
  | { kind: "completing" }
  | { kind: "unsupported"; reason: string };

// ReauthSheet replaces the v1 /connections/{id}/reauth full-page flow with a
// Sheet-hosted variant inside the v2 SPA. Two states: a confirmation card
// that explains what re-auth will do, and a launcher state that opens the
// provider SDK in update mode (Plaid) or reconnection mode (Teller). On
// success we POST /reauth-complete, toast, invalidate caches, and close.
//
// The component fetches its own ConnectionDetail (status, provider,
// institution_name, error_message) so every caller — connections list,
// detail page banner, ⋯ menu — can pass just the short_id.
export function ReauthSheet({
  open,
  onOpenChange,
  connectionShortId,
}: ReauthSheetProps) {
  const connQuery = useConnection(connectionShortId);
  const reauthStart = useReauthStart();
  const reauthComplete = useReauthComplete();
  const [stage, setStage] = useState<Stage>({ kind: "confirm" });
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  // Reset internal state every time the Sheet closes so a re-open lands on
  // the confirmation step rather than mid-flow leftovers. The mutation
  // objects from useMutation are rebuilt every render, so adding them to the
  // dep array would re-fire the effect (and call reset()) on every render —
  // the source of the "Maximum update depth" warning when the Sheet sits
  // mounted-but-closed on the connections page. Their `reset` methods are
  // referentially stable per TanStack Query design, so a [open]-only dep is
  // safe; we read them through refs to keep the dep array honest.
  const reauthStartRef = useRef(reauthStart);
  reauthStartRef.current = reauthStart;
  const reauthCompleteRef = useRef(reauthComplete);
  reauthCompleteRef.current = reauthComplete;
  useEffect(() => {
    if (open) return;
    setStage({ kind: "confirm" });
    setErrorMessage(null);
    reauthStartRef.current.reset();
    reauthCompleteRef.current.reset();
  }, [open]);

  const conn = connQuery.data;
  const institutionName = conn?.institution_name ?? "Untitled connection";

  async function onReconnect() {
    if (!conn) return;
    setErrorMessage(null);
    if (conn.provider === "csv") {
      setStage({
        kind: "unsupported",
        reason:
          "CSV imports don't have credentials to refresh. Use “Import more” on the connection detail page to add new rows.",
      });
      return;
    }
    try {
      const session = await reauthStart.mutateAsync(conn.id);
      if (!session?.link_token) {
        toast.error("Provider didn't return a re-auth token. Try again.");
        return;
      }
      if (conn.provider === "plaid") {
        setStage({ kind: "plaid", linkToken: session.link_token });
      } else if (conn.provider === "teller") {
        const applicationId = session.application_id ?? "";
        if (!applicationId) {
          toast.error(
            "Teller application id is not configured on this server.",
          );
          return;
        }
        setStage({
          kind: "teller",
          enrollmentId: session.link_token,
          applicationId,
        });
      } else {
        setStage({
          kind: "unsupported",
          reason: `Re-authentication for ${conn.provider} is not supported here yet — use the v1 admin at /connections.`,
        });
      }
    } catch (err) {
      const msg =
        err instanceof ApiError
          ? err.message
          : "Couldn't start re-authentication. Try again.";
      setErrorMessage(msg);
      toast.error(msg);
    }
  }

  async function completeReauth() {
    if (!conn) return;
    setStage({ kind: "completing" });
    try {
      await reauthComplete.mutateAsync(conn.id);
      toast.success(`Reconnected ${institutionName}.`, {
        description: "Sync resumed — accounts will refresh on the next webhook.",
      });
      onOpenChange(false);
    } catch (err) {
      const msg =
        err instanceof ApiError
          ? err.message
          : "Couldn't finish re-authentication. Try again.";
      setErrorMessage(msg);
      toast.error(msg);
      setStage({ kind: "confirm" });
    }
  }

  function onProviderExit(message: string | null) {
    if (message) {
      setErrorMessage(message);
      toast.error(message);
    }
    setStage({ kind: "confirm" });
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="flex flex-col gap-0 p-0">
        <DetailSheetHeader
          density="accent"
          icon={ShieldAlert}
          eyebrow="Re-authenticate"
          title={conn ? institutionName : "Re-authenticate"}
          description="Reconnect to the bank to resume syncing this connection."
        />

        <div className="flex flex-1 flex-col gap-6 overflow-y-auto p-6">
          {connQuery.isLoading ? (
            <div className="text-muted-foreground flex flex-1 items-center justify-center gap-2 text-sm">
              <Loader2 className="size-4 animate-spin" /> Loading connection…
            </div>
          ) : !conn ? (
            <Alert variant="destructive">
              <AlertTitle>Connection not found</AlertTitle>
              <AlertDescription>
                The connection may have been disconnected. Close this and
                refresh the connections list.
              </AlertDescription>
            </Alert>
          ) : stage.kind === "confirm" ? (
            <ConfirmBody
              institutionName={institutionName}
              errorMessage={conn.error_message}
              uiError={errorMessage}
            />
          ) : stage.kind === "plaid" || stage.kind === "teller" ? (
            <LaunchingBody providerName={stage.kind} />
          ) : stage.kind === "completing" ? (
            <LaunchingBody providerName="completing" />
          ) : (
            <Alert>
              <AlertTitle>Not available here</AlertTitle>
              <AlertDescription>{stage.reason}</AlertDescription>
            </Alert>
          )}

          {/* Plaid update-mode SDK — autoOpen drives the popup the moment
              the launcher mounts. Invisible button; lives inside the body
              so portal placement stays consistent with the rest of the
              Sheet. */}
          {stage.kind === "plaid" && (
            <PlaidLinkButton
              linkToken={stage.linkToken}
              onSuccess={() => completeReauth()}
              onExit={(err) =>
                onProviderExit(
                  err
                    ? err.display_message ||
                        err.error_message ||
                        "Plaid Link exited with an error."
                    : null,
                )
              }
            />
          )}

          {/* Teller reconnection-mode SDK — passing the existing
              enrollmentId switches Connect into "reconnect" rather than
              "enroll". */}
          {stage.kind === "teller" && (
            <TellerConnectButton
              applicationId={stage.applicationId}
              enrollmentId={stage.enrollmentId}
              onSuccess={() => completeReauth()}
              onExit={() => onProviderExit(null)}
              onFailure={(f) =>
                onProviderExit(f.message || "Teller Connect failed.")
              }
            />
          )}
        </div>

        {conn && stage.kind === "confirm" && (
          <div className="bg-muted/20 flex flex-col-reverse items-stretch gap-2 border-t px-4 py-3 sm:flex-row sm:items-center sm:justify-end sm:px-6">
            <Button
              variant="outline"
              size="sm"
              onClick={() => onOpenChange(false)}
              disabled={reauthStart.isPending}
              className="w-full sm:w-auto"
            >
              Cancel
            </Button>
            <Button
              size="sm"
              onClick={onReconnect}
              disabled={reauthStart.isPending}
              className="w-full sm:w-auto"
            >
              {reauthStart.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : null}
              Reconnect
            </Button>
          </div>
        )}

        {conn && stage.kind === "unsupported" && (
          <div className="bg-muted/20 flex flex-col-reverse items-stretch gap-2 border-t px-4 py-3 sm:flex-row sm:items-center sm:justify-end sm:px-6">
            <Button
              variant="outline"
              size="sm"
              onClick={() => onOpenChange(false)}
              className="w-full sm:w-auto"
            >
              Close
            </Button>
          </div>
        )}
      </SheetContent>
    </Sheet>
  );
}

function ConfirmBody({
  institutionName,
  errorMessage,
  uiError,
}: {
  institutionName: string;
  errorMessage: string | null;
  uiError: string | null;
}) {
  return (
    <div className="flex flex-1 flex-col gap-4">
      <div className="bg-amber-500/5 border-amber-500/20 flex items-start gap-3 rounded-md border p-4">
        <div className="bg-amber-500/10 flex size-9 shrink-0 items-center justify-center rounded-md">
          <ShieldAlert className="size-4 text-amber-700 dark:text-amber-400" />
        </div>
        <div className="space-y-1">
          <p className="text-sm font-medium">{institutionName}</p>
          <p className="text-muted-foreground text-xs">
            {errorMessage ??
              "The login for this connection has expired and syncs are paused until it's restored."}
          </p>
        </div>
      </div>
      <p className="text-muted-foreground text-sm">
        Reconnecting opens the bank's hosted login. Your existing accounts and
        transactions stay in place — only the credentials are refreshed.
      </p>
      {uiError && (
        <Alert variant="destructive">
          <AlertTitle>Couldn't start re-authentication</AlertTitle>
          <AlertDescription>{uiError}</AlertDescription>
        </Alert>
      )}
    </div>
  );
}

function LaunchingBody({ providerName }: { providerName: string }) {
  const label =
    providerName === "plaid"
      ? "Opening Plaid…"
      : providerName === "teller"
        ? "Opening Teller…"
        : "Reconnecting…";
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-3 text-center">
      <Loader2 className="text-muted-foreground size-6 animate-spin" />
      <div className="text-sm font-medium">{label}</div>
      <p className="text-muted-foreground max-w-xs text-xs">
        Finish the flow in the popup. Closing it brings you back here.
      </p>
    </div>
  );
}
