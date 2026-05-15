import { useEffect, useRef } from "react";

// Teller doesn't ship a packaged React wrapper, so this component is
// hand-rolled around the script-tag-loaded SDK. Keeps the same shape as
// PlaidLinkButton so a future hosted-link page can branch on `provider` and
// call into either component without learning two integration patterns.

export interface TellerEnrollment {
  accessToken: string;
  enrollment: {
    id: string;
    institution: { name: string };
  };
}

export interface TellerEnrollmentResult {
  accessToken: string;
  enrollmentId: string;
  institutionName: string;
  institutionId: string;
}

export interface TellerConnectButtonProps {
  applicationId: string;
  /** "sandbox" | "development" | "production". Sandbox is the default since
   *  it works without provisioning real Teller credentials. */
  environment?: string;
  /** Pass the existing connection's enrollment ID to switch Teller Connect
   *  into reconnection mode (re-auth). Used by the Re-auth Sheet (PR-05);
   *  omit it for new-connection flows. */
  enrollmentId?: string;
  onSuccess: (result: TellerEnrollmentResult) => void;
  onExit?: () => void;
  onFailure?: (failure: { message?: string }) => void;
  autoOpen?: boolean;
}

const TELLER_SDK_URL = "https://cdn.teller.io/connect/connect.js";

declare global {
  interface Window {
    TellerConnect?: {
      setup: (options: {
        applicationId: string;
        environment?: string;
        enrollmentId?: string;
        products?: string[];
        onSuccess: (enrollment: TellerEnrollment) => void;
        onExit?: () => void;
        onFailure?: (failure: { message?: string }) => void;
      }) => { open: () => void };
    };
  }
}

// loadTellerSDK injects the Teller Connect script tag once and resolves when
// the global `TellerConnect` is available. Re-uses the in-flight load if a
// second component mounts during the first fetch.
let tellerLoadPromise: Promise<void> | null = null;
function loadTellerSDK(): Promise<void> {
  if (typeof window === "undefined") return Promise.resolve();
  if (window.TellerConnect) return Promise.resolve();
  if (tellerLoadPromise) return tellerLoadPromise;
  tellerLoadPromise = new Promise<void>((resolve, reject) => {
    const existing = document.querySelector<HTMLScriptElement>(
      `script[src="${TELLER_SDK_URL}"]`,
    );
    const script = existing ?? document.createElement("script");
    script.src = TELLER_SDK_URL;
    script.async = true;
    script.onload = () => resolve();
    script.onerror = () => {
      tellerLoadPromise = null;
      reject(new Error("Failed to load Teller Connect SDK"));
    };
    if (!existing) document.head.appendChild(script);
  });
  return tellerLoadPromise;
}

// TellerConnectButton mirrors PlaidLinkButton's shape: renders nothing
// visible, opens Teller Connect on mount when `autoOpen` is true. The parent
// Sheet stays in control of the rest of the wizard.
export function TellerConnectButton({
  applicationId,
  environment = "sandbox",
  enrollmentId,
  onSuccess,
  onExit,
  onFailure,
  autoOpen = true,
}: TellerConnectButtonProps) {
  const openedRef = useRef(false);

  useEffect(() => {
    if (!autoOpen || openedRef.current) return;
    let cancelled = false;
    loadTellerSDK()
      .then(() => {
        if (cancelled || !window.TellerConnect) return;
        openedRef.current = true;
        const tc = window.TellerConnect.setup({
          applicationId,
          environment,
          ...(enrollmentId ? { enrollmentId } : {}),
          products: ["transactions", "balance"],
          onSuccess: (enrollment) => {
            onSuccess({
              accessToken: enrollment.accessToken,
              enrollmentId: enrollment.enrollment.id,
              institutionName: enrollment.enrollment.institution.name,
              // Teller doesn't expose a stable institution_id; the v1 wizard
              // reuses the institution name as the id, so we mirror that.
              institutionId: enrollment.enrollment.institution.name,
            });
          },
          onExit: () => onExit?.(),
          onFailure: (failure) => onFailure?.(failure),
        });
        tc.open();
      })
      .catch((err: Error) => {
        onFailure?.({ message: err.message });
      });
    return () => {
      cancelled = true;
    };
  }, [
    autoOpen,
    applicationId,
    environment,
    enrollmentId,
    onSuccess,
    onExit,
    onFailure,
  ]);

  return null;
}
