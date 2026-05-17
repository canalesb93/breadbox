import { Link } from "@tanstack/react-router";
import { AlertTriangle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { StatusPanel } from "@/components/status-panel";
import type { Connection } from "@/api/types";

interface HomeAttentionPanelProps {
  connections: Connection[] | undefined;
}

// Inline warning shown above the hero scoreboard when one or more
// connections need user action (reauth, error). Renders nothing when
// the household is healthy — the home page reads cleanly with no
// orphaned "All good!" empty-state.
//
// Uses the canonical StatusPanel primitive so the tone-rail vocabulary
// matches the rest of the v2 surfaces (setup-account, providers).
export function HomeAttentionPanel({ connections }: HomeAttentionPanelProps) {
  if (!connections) return null;
  const attention = connections.filter(
    (c) => c.status === "pending_reauth" || c.status === "error",
  );
  if (attention.length === 0) return null;

  const reauthCount = attention.filter((c) => c.status === "pending_reauth").length;
  const errorCount = attention.filter((c) => c.status === "error").length;

  const heading =
    attention.length === 1
      ? `${attention[0].institution_name || "A connection"} needs your attention`
      : `${attention.length} connections need your attention`;

  const parts: string[] = [];
  if (reauthCount > 0) {
    parts.push(`${reauthCount} ${reauthCount === 1 ? "needs" : "need"} re-authentication`);
  }
  if (errorCount > 0) {
    parts.push(`${errorCount} ${errorCount === 1 ? "has" : "have"} a sync error`);
  }
  const body =
    parts.length > 0
      ? `${parts.join(", ")}. Resolve them to keep balances and transactions current.`
      : "Resolve them to keep balances and transactions current.";

  return (
    <StatusPanel
      tone="warning"
      icon={AlertTriangle}
      heading={heading}
      body={body}
      trailing={
        <Button asChild size="sm" variant="outline">
          <Link to="/connections">Review</Link>
        </Button>
      }
    />
  );
}
