import { useEffect, useRef } from "react";
import {
  usePlaidLink,
  type PlaidLinkOnExitMetadata,
  type PlaidLinkOnSuccessMetadata,
} from "react-plaid-link";

export interface PlaidLinkResult {
  publicToken: string;
  metadata: PlaidLinkOnSuccessMetadata;
}

export interface PlaidLinkButtonProps {
  linkToken: string;
  /** Called when Plaid Link's onSuccess fires. */
  onSuccess: (result: PlaidLinkResult) => void;
  /** Called when the user dismisses Plaid Link or it errors out. The error
   *  argument is null on plain cancellation. */
  onExit?: (
    error: { display_message?: string; error_message?: string } | null,
    metadata: PlaidLinkOnExitMetadata,
  ) => void;
  /** When true, opens Plaid Link as soon as the SDK signals `ready`. The
   *  Connect-bank Sheet uses this so step 2 of the wizard launches the
   *  popup automatically without a second user click. */
  autoOpen?: boolean;
}

// PlaidLinkButton encapsulates Plaid's official `usePlaidLink` hook. It
// renders nothing visible — Plaid Link itself is a popup the SDK manages —
// but exposes its `open()` via auto-launch so the parent Sheet can drive the
// flow declaratively. Reused for new connections (PR-03) and re-auth in
// update mode (PR-05) by passing different link tokens.
export function PlaidLinkButton({
  linkToken,
  onSuccess,
  onExit,
  autoOpen = true,
}: PlaidLinkButtonProps) {
  const { open, ready } = usePlaidLink({
    token: linkToken,
    onSuccess: (publicToken, metadata) => onSuccess({ publicToken, metadata }),
    onExit: (err, metadata) => {
      // Plaid's err type is broader than what callers typically need; collapse
      // to the two strings the v1 wizard already surfaces.
      onExit?.(
        err ? { display_message: err.display_message, error_message: err.error_message } : null,
        metadata,
      );
    },
  });

  // Track whether we already opened so a re-render (e.g. token swap) doesn't
  // double-fire the popup.
  const openedRef = useRef(false);
  useEffect(() => {
    if (autoOpen && ready && !openedRef.current) {
      openedRef.current = true;
      open();
    }
  }, [autoOpen, ready, open]);

  return null;
}
