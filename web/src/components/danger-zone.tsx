import { useState, type ReactNode } from "react";
import { Loader2, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

interface DangerZoneProps {
  /** Short copy shown above the action — explains what delete will do. */
  description: ReactNode;
  /** Bold label inside the inline confirm block ("Permanently delete X?"). */
  confirmTarget: ReactNode;
  /** Label on both the trigger and the destructive confirm button. */
  actionLabel: string;
  /** Mutation runner — returns true on success so the confirm closes. */
  onConfirm: () => Promise<void>;
  isPending: boolean;
}

// Inline destructive-confirm pattern: an outline trigger that expands into a
// tinted confirm block in place. Avoids modal/dialog churn for delete flows.
export function DangerZone({
  description,
  confirmTarget,
  actionLabel,
  onConfirm,
  isPending,
}: DangerZoneProps) {
  const [confirming, setConfirming] = useState(false);

  return (
    <Card className="mt-8 border-destructive/40">
      <CardHeader>
        <CardTitle className="text-destructive flex items-center gap-2 text-base">
          <Trash2 className="size-4" />
          Danger zone
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3 text-sm">
        <div className="text-muted-foreground">{description}</div>

        {confirming ? (
          <div className="border-destructive/40 bg-destructive/5 space-y-3 rounded-md border p-3">
            <p className="text-sm font-medium">
              Permanently delete {confirmTarget}?
            </p>
            <div className="flex gap-2">
              <Button
                variant="destructive"
                size="sm"
                onClick={onConfirm}
                disabled={isPending}
              >
                {isPending && <Loader2 className="size-4 animate-spin" />}
                {actionLabel}
              </Button>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setConfirming(false)}
                disabled={isPending}
              >
                Cancel
              </Button>
            </div>
          </div>
        ) : (
          <Button
            variant="outline"
            className="text-destructive border-destructive/40 hover:bg-destructive/5 hover:text-destructive"
            onClick={() => setConfirming(true)}
          >
            <Trash2 className="size-4" />
            {actionLabel}
          </Button>
        )}
      </CardContent>
    </Card>
  );
}
