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
      <TabsList className="h-auto flex-wrap">
        <TabsTrigger value="all" className="gap-1.5">
          All
          <span className="text-muted-foreground text-xs tabular-nums">
            {totalCount}
          </span>
        </TabsTrigger>
        {users.map((u) => (
          <TabsTrigger key={u.short_id} value={u.short_id} className="gap-1.5">
            {u.name}
            <span className="text-muted-foreground text-xs tabular-nums">
              {counts?.get(u.short_id) ?? 0}
            </span>
          </TabsTrigger>
        ))}
      </TabsList>
    </Tabs>
  );
}
