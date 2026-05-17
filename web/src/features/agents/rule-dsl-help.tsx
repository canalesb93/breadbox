import { useState } from "react";
import { BookOpen, ChevronDown, ChevronRight } from "lucide-react";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";

// RuleDslHelp is a collapsible cheat-sheet for the transaction-rule DSL,
// shown under the prompt textarea on the agent edit page. The full grammar
// lives in `docs/rule-dsl.md`; this card surfaces the common fields and
// operators an operator needs when authoring a rule-creating prompt.
//
// Closed by default — opens only when an operator asks for the reference,
// so the form stays compact for prompt-only edits.
export function RuleDslHelp() {
  const [open, setOpen] = useState(false);
  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className="rounded-md border bg-muted/30"
    >
      <CollapsibleTrigger className="hover:bg-muted/50 flex w-full items-center justify-between gap-2 rounded-md px-3 py-2 text-left text-sm">
        <span className="inline-flex items-center gap-2 font-medium">
          <BookOpen className="size-4" />
          Rule DSL quick reference
        </span>
        <span className="text-muted-foreground inline-flex items-center gap-1 text-xs">
          {open ? (
            <ChevronDown className="size-4" />
          ) : (
            <ChevronRight className="size-4" />
          )}
        </span>
      </CollapsibleTrigger>
      <CollapsibleContent className="space-y-4 px-3 pb-3 pt-1 text-sm">
        <p className="text-muted-foreground text-xs">
          When your agent calls{" "}
          <code className="rounded bg-background px-1 py-0.5 text-[11px]">
            create_transaction_rule
          </code>{" "}
          (or its bulk/preview siblings), the rule is shaped like this. Full
          grammar:{" "}
          <code className="rounded bg-background px-1 py-0.5 text-[11px]">
            docs/rule-dsl.md
          </code>{" "}
          (engineering doc — link out from your prompt if helpful).
        </p>

        <div>
          <div className="text-muted-foreground mb-1 text-xs font-semibold uppercase tracking-wide">
            Shape
          </div>
          <pre className="bg-background overflow-x-auto rounded border p-2 font-mono text-[11px] leading-relaxed">
{`{
  "name": "Dining categorization",
  "conditions": { "field": "provider_merchant_name",
                  "op": "contains", "value": "Starbucks" },
  "actions": [{ "type": "set_category",
                "category_slug": "food_and_drink_coffee" }],
  "trigger": "on_create",
  "priority": 10
}`}
          </pre>
        </div>

        <div className="grid gap-3 sm:grid-cols-2">
          <div>
            <div className="text-muted-foreground mb-1 text-xs font-semibold uppercase tracking-wide">
              Common fields
            </div>
            <ul className="space-y-0.5 font-mono text-[11px]">
              <li>provider_merchant_name <span className="text-muted-foreground">string</span></li>
              <li>provider_name <span className="text-muted-foreground">string</span></li>
              <li>provider_category_primary <span className="text-muted-foreground">string</span></li>
              <li>provider_category_detailed <span className="text-muted-foreground">string</span></li>
              <li>category <span className="text-muted-foreground">string (assigned)</span></li>
              <li>amount <span className="text-muted-foreground">numeric</span></li>
              <li>tags <span className="text-muted-foreground">tags</span></li>
              <li>provider <span className="text-muted-foreground">plaid|teller|csv</span></li>
              <li>account_id, user_id <span className="text-muted-foreground">string</span></li>
            </ul>
          </div>
          <div>
            <div className="text-muted-foreground mb-1 text-xs font-semibold uppercase tracking-wide">
              Operators by type
            </div>
            <ul className="space-y-0.5 font-mono text-[11px]">
              <li>string: eq, neq, contains, not_contains,</li>
              <li className="pl-8">matches (RE2), in</li>
              <li>numeric: eq, neq, gt, gte, lt, lte</li>
              <li>bool: eq, neq</li>
              <li>tags: contains, not_contains, in</li>
            </ul>
          </div>
        </div>

        <div>
          <div className="text-muted-foreground mb-1 text-xs font-semibold uppercase tracking-wide">
            Combinators
          </div>
          <pre className="bg-background overflow-x-auto rounded border p-2 font-mono text-[11px] leading-relaxed">
{`{ "and": [ <condition>, <condition>, ... ] }
{ "or":  [ <condition>, <condition>, ... ] }
{ "not": <condition> }`}
          </pre>
          <p className="text-muted-foreground mt-1 text-[11px]">
            Max nesting depth: 10. Empty <code>{"{}"}</code> matches every
            transaction (rarely what you want — instruct the agent to always
            scope rules).
          </p>
        </div>

        <div>
          <div className="text-muted-foreground mb-1 text-xs font-semibold uppercase tracking-wide">
            Actions + triggers
          </div>
          <ul className="space-y-0.5 text-xs">
            <li>
              <code className="font-mono">set_category</code>,{" "}
              <code className="font-mono">add_tag</code>,{" "}
              <code className="font-mono">set_attribution</code> — at least
              one per rule, can stack.
            </li>
            <li>
              <code className="font-mono">trigger</code>:{" "}
              <code className="font-mono">on_create</code> (new sync rows
              only) or <code className="font-mono">on_change</code> (also
              re-evaluates on update).
            </li>
            <li>
              <code className="font-mono">priority</code>: lower runs first.
              Rules chain — earlier <code className="font-mono">add_tag</code>{" "}
              / <code className="font-mono">set_category</code> writes
              feed later condition evaluation.
            </li>
            <li>
              <code className="font-mono">apply_retroactively=true</code> on
              create/update sweeps the existing table — use sparingly, only
              during initial setup.
            </li>
          </ul>
        </div>

        <p className="text-muted-foreground text-[11px]">
          Tip: instruct your agent to call{" "}
          <code className="font-mono">preview_rule</code> before
          <code className="font-mono"> create_transaction_rule</code> to
          verify the match set.
        </p>
      </CollapsibleContent>
    </Collapsible>
  );
}
