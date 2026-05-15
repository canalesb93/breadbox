import { useEffect, useState } from "react";
import { Loader2, ShieldAlert } from "lucide-react";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Button } from "@/components/ui/button";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
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
// stage for hand-crafted URLs rather than crash.
type Stage =
  | { kind: "confirm" }
  | { kind: "plaid"; linkToken: string }
  | { kind: "teller"; enrollmentId: string }
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
  // the confirmation step rather than mid-flow leftovers.
  useEffect(() => {
    if (!open) {
      setStage({ kind: "confirm" });
      setErrorMessage(null);
      reauthStart.reset();
      reauthComplete.reset();
    }
  }, [open, reauthStart, reauthComplete]);

  const conn = connQuery.data;
  const institutionName = conn?.institution_name ?? "Untitled connection";

  function handleSheetChange(next: boolean) {
    onOpenChange(next);
  }

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
        setStage({ kind: "teller", enrollmentId: session.link_token });
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
      toast.success(`Reconnected ${institutionName}.`);
      handleSheetChange(false);
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

  // The Teller application id comes from a window global injected by the
  // Vite shell, mirroring the Connect-bank Sheet. Without it Teller Connect
  // can't bootstrap.
  const tellerAppId =
    (typeof window !== "undefined" &&
      (window as Window & { __TELLER_APP_ID__?: string }).__TELLER_APP_ID__) ||
    "";

  return (
    <Sheet open={open} onOpenChange={handleSheetChange}>
      <SheetContent className="flex flex-col gap-6 p-6">
        <SheetHeader className="p-0">
          <SheetTitle>Re-authenticate</SheetTitle>
          <SheetDescription>
            Reconnect to the bank to resume syncing this connection.
          </SheetDescription>
        </SheetHeader>

        {connQuery.isLoading ? (
          <div className="text-muted-foreground flex flex-1 items-center justify-center gap-2 text-sm">
            <Loader2 className="size-4 animate-spin" /> Loading connection…
          </div>
        ) : !conn ? (
          <Alert variant="destructive">
            <AlertTitle>Connection not found</AlertTitle>
            <AlertDescription>
              The connection may have been disconnected. Close this and refresh
              the connections list.
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

        {/* Plaid update-mode SDK — autoOpen drives the popup the moment the
            launcher mounts. */}
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

        {/* Teller reconnection-mode SDK — passing the existing enrollmentId
            switches Connect into "reconnect" rather than "enroll". */}
        {stage.kind === "teller" && (
          <TellerConnectButton
            applicationId={tellerAppId}
            enrollmentId={stage.enrollmentId}
            onSuccess={() => completeReauth()}
            onExit={() => onProviderExit(null)}
            onFailure={(f) =>
              onProviderExit(f.message || "Teller Connect failed.")
            }
          />
        )}

        {conn && stage.kind === "confirm" && (
          <SheetFooter className="px-0">
            <Button
              variant="outline"
              onClick={() => handleSheetChange(false)}
              disabled={reauthStart.isPending}
            >
              Cancel
            </Button>
            <Button onClick={onReconnect} disabled={reauthStart.isPending}>
              {reauthStart.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : null}
              Reconnect
            </Button>
          </SheetFooter>
        )}

        {conn && stage.kind === "unsupported" && (
          <SheetFooter className="px-0">
            <Button variant="outline" onClick={() => handleSheetChange(false)}>
              Close
            </Button>
          </SheetFooter>
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
