import { Receipt } from "lucide-react";
import { DynamicIcon } from "@/lib/icon";
import { cn } from "@/lib/utils";

const TILE_SIZE = {
  sm: "size-7",
  md: "size-9",
  lg: "size-12",
} as const;

const ICON_SIZE = {
  sm: "size-3.5",
  md: "size-4",
  lg: "size-5",
} as const;

interface CategoryIconTileProps {
  icon?: string | null;
  color?: string | null;
  size?: keyof typeof TILE_SIZE;
  className?: string;
}

// CategoryIconTile is the rounded icon chip that fronts a transaction — in
// list rows and on the detail-page hero. The tile is tinted from the
// category's own color (10% fill, full-strength icon); a category-less
// transaction falls back to a neutral receipt glyph.
export function CategoryIconTile({
  icon,
  color,
  size = "md",
  className,
}: CategoryIconTileProps) {
  return (
    <div
      className={cn(
        "bg-muted text-muted-foreground flex shrink-0 items-center justify-center rounded-md",
        TILE_SIZE[size],
        className,
      )}
      style={color ? { backgroundColor: `${color}1a`, color } : undefined}
    >
      {icon ? (
        <DynamicIcon name={icon} className={ICON_SIZE[size]} />
      ) : (
        <Receipt className={ICON_SIZE[size]} />
      )}
    </div>
  );
}
