import { useEffect, useRef, useState } from "react";
import { Link, Navigate, useNavigate } from "@tanstack/react-router";
import { Check, Copy, KeyRound, ShieldAlert } from "lucide-react";
import { toast } from "sonner";
import { PageHeader } from "@/components/page-header";
import { Button } from "@/components/ui/button";
import {
  Alert,
  AlertDescription,
  AlertTitle,
} from "@/components/ui/alert";
import { Input } from "@/components/ui/input";
import { IdPill } from "@/components/id-pill";
import { SectionCard } from "@/components/section-card";
import {
  clearPendingPlaintextKey,
  readPendingPlaintextKey,
  type PendingPlaintextKey,
} from "@/features/api-keys/plaintext-store";

export function APIKeyCreatedPage() {
  // Read synchronously so a missing key redirects on first render — avoids
  // a frame of empty content before the bounce.
  const [pending] = useState<PendingPlaintextKey | null>(() =>
    readPendingPlaintextKey(),
  );
  const navigate = useNavigate();
  const [copied, setCopied] = useState(false);
  const copyTimerRef = useRef<number | null>(null);

  // Clear from storage on unmount — the value should not survive a fresh
  // visit to /api-keys/created. The component-local `pending` keeps the
  // page rendered for as long as the user stays.
  useEffect(() => {
    return () => {
      clearPendingPlaintextKey();
      if (copyTimerRef.current !== null) {
        window.clearTimeout(copyTimerRef.current);
      }
    };
  }, []);

  if (!pending) {
    return <Navigate to="/api-keys" />;
  }

  const onCopy = async () => {
    try {
      await navigator.clipboard.writeText(pending.plaintext);
      setCopied(true);
      toast.success("API key copied to clipboard.");
      if (copyTimerRef.current !== null) {
        window.clearTimeout(copyTimerRef.current);
      }
      copyTimerRef.current = window.setTimeout(() => {
        setCopied(false);
        copyTimerRef.current = null;
      }, 2000);
    } catch {
      toast.error(
        "Couldn't access the clipboard. Select the value and copy manually.",
      );
    }
  };

  const onDone = () => {
    clearPendingPlaintextKey();
    navigate({ to: "/api-keys" });
  };

  return (
    <div className="mx-auto max-w-2xl">
      <PageHeader
        eyebrow="Key created"
        title={pending.name}
        description="Copy the plaintext now — Breadbox only stores a SHA-256 hash, so this is the one and only time it appears."
      />

      <SectionCard
        icon={<KeyRound className="text-muted-foreground size-4" />}
        title="Plaintext key"
      >
        <div className="space-y-5">
          <div className="space-y-1.5">
            <label
              htmlFor="api-key-plaintext"
              className="text-muted-foreground text-xs font-medium"
            >
              API key
            </label>
            <div className="flex flex-col gap-2 sm:flex-row">
              <Input
                id="api-key-plaintext"
                readOnly
                value={pending.plaintext}
                onFocus={(e) => e.currentTarget.select()}
                className="bg-muted/40 min-w-0 flex-1 truncate font-mono text-xs"
              />
              <Button
                type="button"
                onClick={onCopy}
                className="shrink-0"
                variant={copied ? "secondary" : "default"}
              >
                {copied ? (
                  <Check className="size-4" />
                ) : (
                  <Copy className="size-4" />
                )}
                {copied ? "Copied" : "Copy"}
              </Button>
            </div>
            <p className="text-muted-foreground text-xs">
              Send it as the <IdPill value="X-API-Key" /> header on every
              request to <IdPill value="/api/v1/*" />.
            </p>
          </div>

          <Alert variant="default" className="bg-muted/40 border-muted">
            <ShieldAlert className="size-4" />
            <AlertTitle>One-time disclosure</AlertTitle>
            <AlertDescription>
              Breadbox only stores a SHA-256 hash of the key. If you lose this
              plaintext you'll need to revoke and mint a new one.
            </AlertDescription>
          </Alert>
        </div>
      </SectionCard>

      <div className="mt-6 flex items-center justify-between gap-2">
        <Button variant="ghost" size="sm" asChild>
          <Link to="/api-keys/new">Mint another</Link>
        </Button>
        <Button size="sm" onClick={onDone}>
          I've saved it — done
        </Button>
      </div>
    </div>
  );
}
