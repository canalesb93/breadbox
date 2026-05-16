import { useState } from "react";
import { CheckCircle2, Plug, XCircle, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ApiError } from "@/api/client";
import { useTestProvider } from "@/api/queries/provider-config";
import { cn } from "@/lib/utils";

interface TestConnectionButtonProps {
  provider: "plaid" | "teller";
  disabled?: boolean;
}

// TestConnectionButton hits the server-side credentials probe and surfaces
// the result inline. The endpoint returns 200 OK with {ok:false, message}
// on bad creds — we still branch on data.ok rather than catching ApiError.
export function TestConnectionButton({ provider, disabled }: TestConnectionButtonProps) {
  const test = useTestProvider();
  const [result, setResult] = useState<{ ok: boolean; message: string } | null>(null);

  async function onClick() {
    setResult(null);
    try {
      const data = await test.mutateAsync(provider);
      setResult({ ok: data.ok, message: data.message || (data.ok ? "Connection OK" : "Test failed") });
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : "Test failed";
      setResult({ ok: false, message: msg });
    }
  }

  return (
    <div className="flex flex-wrap items-center gap-3">
      <Button type="button" variant="outline" size="sm" onClick={onClick} disabled={disabled || test.isPending}>
        {test.isPending ? <Loader2 className="size-3.5 animate-spin" /> : <Plug className="size-3.5" />}
        Test connection
      </Button>
      {result && (
        <span
          className={cn(
            "flex items-center gap-1.5 text-xs",
            result.ok ? "text-emerald-600 dark:text-emerald-400" : "text-destructive",
          )}
        >
          {result.ok ? <CheckCircle2 className="size-3.5" /> : <XCircle className="size-3.5" />}
          {result.message}
        </span>
      )}
    </div>
  );
}
