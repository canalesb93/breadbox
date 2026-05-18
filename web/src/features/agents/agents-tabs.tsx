import { Link } from "@tanstack/react-router";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";

// AgentsTabs is the sub-nav shared between /agents (definitions) and
// /agents/runs (global run history). Each tab is a real <Link> so
// middle-click, cmd-click, and "open in new tab" all work — the radix
// Tabs primitive renders the trigger via Slot when `asChild` is set, and
// the `value` prop is driven by the current route rather than internal
// state. Keeps the two list views feeling like one surface without
// forcing a shared route component.
export type AgentsTabValue = "agents" | "runs";

export function AgentsTabs({ value }: { value: AgentsTabValue }) {
  return (
    <Tabs value={value}>
      <TabsList>
        <TabsTrigger value="agents" asChild>
          <Link to="/agents">Agents</Link>
        </TabsTrigger>
        <TabsTrigger value="runs" asChild>
          <Link to="/agents/runs">Runs</Link>
        </TabsTrigger>
      </TabsList>
    </Tabs>
  );
}
