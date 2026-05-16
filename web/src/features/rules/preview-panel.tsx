import { useCallback, useState } from "react";
import { Eye, Loader2, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { usePreviewRule } from "@/api/queries/rules";
import { ApiError } from "@/api/client";
import type { Condition } from "@/api/types";
import { formatAmount, formatDate } from "@/lib/format";

interface PreviewPanelProps {
  conditions: Condition;
  /** Disables the run button — usually because the form is invalid. */
  disabled?: boolean;
}

// PreviewPanel runs POST /api/v1/rules/preview against the current conditions
// and renders the match count + a sample of matching transactions. Stateful:
// the user clicks "Run preview" to invoke; results stay visible until the
// conditions change and they re-run.
export function PreviewPanel({ conditions, disabled }: PreviewPanelProps) {
  const preview = usePreviewRule();
  const [errorMsg, setErrorMsg] = useState<string | null>(null);

  const run = useCallback(async () => {
    setErrorMsg(null);
    try {
      await preview.mutateAsync({ conditions, sampleSize: 10 });
    } catch (err) {
      setErrorMsg(err instanceof ApiError ? err.message : "Preview failed.");
    }
  }, [preview, conditions]);

  const result = preview.data;

  return (
    <div className="bg-card space-y-3 rounded-xl border p-4">
      <div className="flex items-center justify-between gap-3">
        <div className="space-y-0.5">
          <h3 className="text-sm font-medium">Live preview</h3>
          <p className="text-muted-foreground text-xs">
            See which existing transactions match these conditions.
          </p>
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={run}
          disabled={disabled || preview.isPending}
        >
          {preview.isPending ? (
            <Loader2 className="size-3.5 animate-spin" />
          ) : result ? (
            <RefreshCw className="size-3.5" />
          ) : (
            <Eye className="size-3.5" />
          )}
          {result ? "Re-run" : "Run preview"}
        </Button>
      </div>

      {errorMsg && (
        <Alert variant="destructive">
          <AlertTitle>Preview failed</AlertTitle>
          <AlertDescription>{errorMsg}</AlertDescription>
        </Alert>
      )}

      {result && (
        <div className="space-y-3">
          <div className="text-muted-foreground text-sm">
            <span className="text-foreground font-semibold tabular-nums">
              {result.match_count.toLocaleString()}
            </span>{" "}
            of {result.total_scanned.toLocaleString()} scanned transactions
            match.
          </div>
          {result.sample_matches.length > 0 ? (
            <div className="divide-y rounded-lg border">
              {result.sample_matches.map((m) => (
                <div
                  key={m.transaction_id}
                  className="flex items-center justify-between gap-3 px-3 py-2 text-sm"
                >
                  <div className="min-w-0 flex-1">
                    <p className="truncate font-medium">
                      {m.provider_merchant_name || m.provider_name}
                    </p>
                    <p className="text-muted-foreground text-xs">
                      {formatDate(m.date)} ·{" "}
                      {m.provider_category_primary || "Uncategorized"}
                    </p>
                  </div>
                  <span className="text-foreground/80 shrink-0 text-sm font-medium tabular-nums">
                    {formatAmount(m.amount, m.iso_currency_code)}
                  </span>
                </div>
              ))}
            </div>
          ) : (
            <p className="text-muted-foreground rounded-lg border p-3 text-center text-sm">
              No matches yet — adjust your conditions and re-run.
            </p>
          )}
        </div>
      )}
    </div>
  );
}
