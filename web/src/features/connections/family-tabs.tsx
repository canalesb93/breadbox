import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import type { User } from "@/api/types";

interface FamilyTabsProps {
  users: User[];
  /** Currently selected short_id, or "all". */
  value: string;
  onChange: (value: string) => void;
  /** Map of short_id → connection count for rendering counts on each tab. */
  counts?: Map<string, number>;
  totalCount: number;
}

// Segmented filter for the connections list. Only renders if there's more
// than one household member — single-user households see no tabs at all.
//
// Mobile note: when the household has more than ~3 members, the wrapped tab
// layout collapses into a multi-row mess that bleeds into the page below
// (verified at 375x812 in iter 24). The outer wrapper now scrolls
// horizontally on overflow (`overflow-x-auto`) and the TabsList stays on
// one line (`flex-nowrap`), so long names get a swipeable strip instead of
// a wrapped pile. The fade mask on the right edge hints at off-screen tabs
// without needing visible scrollbars (kept hidden via the global
// `[&::-webkit-scrollbar]:hidden` Tailwind utility, with parity for
// Firefox).
export function FamilyTabs({
  users,
  value,
  onChange,
  counts,
  totalCount,
}: FamilyTabsProps) {
  if (users.length <= 1) return null;
  return (
    <Tabs value={value} onValueChange={onChange} className="w-full">
      <div
        className="relative -mx-2 overflow-x-auto px-2 [scrollbar-width:none] [-ms-overflow-style:none] [&::-webkit-scrollbar]:hidden"
      >
        <TabsList className="h-auto w-max flex-nowrap">
          <TabsTrigger value="all" className="gap-1.5 whitespace-nowrap">
            All
            <span className="text-muted-foreground text-xs tabular-nums">
              {totalCount}
            </span>
          </TabsTrigger>
          {users.map((u) => (
            <TabsTrigger
              key={u.short_id}
              value={u.short_id}
              className="gap-1.5 whitespace-nowrap"
            >
              {u.name}
              <span className="text-muted-foreground text-xs tabular-nums">
                {counts?.get(u.short_id) ?? 0}
              </span>
            </TabsTrigger>
          ))}
        </TabsList>
      </div>
    </Tabs>
  );
}
