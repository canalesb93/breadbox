import type { Connection } from "@/api/types";
import { needsAttention } from "./connection-utils";

interface ConnectionsSummaryProps {
  connections: Connection[];
}

// One-liner that sits under the page title. Calls out total count and how
// many need attention — the only metric that drives behaviour on this page.
// Net-worth-style aggregates live on the Insights page.
export function ConnectionsSummary({ connections }: ConnectionsSummaryProps) {
  if (connections.length === 0) return null;
  const total = connections.length;
  const healthy = connections.filter((c) => c.status === "active").length;
  const attention = connections.filter(needsAttention).length;
  const disconnected = connections.filter(
    (c) => c.status === "disconnected",
  ).length;

  const parts: string[] = [`${total} ${total === 1 ? "connection" : "connections"}`];
  parts.push(`${healthy} healthy`);
  if (attention > 0) parts.push(`${attention} needs action`);
  if (disconnected > 0) parts.push(`${disconnected} disconnected`);

  return (
    <p className="text-muted-foreground text-sm">{parts.join(" · ")}</p>
  );
}
