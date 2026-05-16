import { useEffect, useRef, useState } from "react";
import { Link, Navigate, useNavigate } from "@tanstack/react-router";
import { Check, Copy, KeyRound, ShieldAlert } from "lucide-react";
import { toast } from "sonner";
import { PageHeader } from "@/components/page-header";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Alert,
  AlertDescription,
  AlertTitle,
} from "@/components/ui/alert";
import { Input } from "@/components/ui/input";
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
        title="Key created"
        description="Copy the plaintext now — it isn't shown again."
      />

      <Card className="overflow-hidden">
        <CardHeader>
          <div className="flex items-center gap-3">
            <div className="bg-primary/10 text-primary flex size-9 items-center justify-center rounded-lg">
              <KeyRound className="size-4" />
            </div>
            <div>
              <CardTitle className="text-base">{pending.name}</CardTitle>
              <CardDescription>
                Stash this in your password manager before leaving the page.
              </CardDescription>
            </div>
          </div>
        </CardHeader>

        <CardContent className="space-y-5">
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
              Send it as the{" "}
              <code className="bg-muted text-foreground rounded px-1.5 py-0.5 font-mono text-[11px]">
                X-API-Key
              </code>{" "}
              header on every request to{" "}
              <code className="bg-muted text-foreground rounded px-1.5 py-0.5 font-mono text-[11px]">
                /api/v1/*
              </code>
              .
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
        </CardContent>
      </Card>

      <div className="mt-6 flex items-center justify-between gap-2">
        <Button variant="ghost" asChild>
          <Link to="/api-keys/new">Mint another</Link>
        </Button>
        <Button onClick={onDone}>I've saved it — done</Button>
      </div>
    </div>
  );
}
