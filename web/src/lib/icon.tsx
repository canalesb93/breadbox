import {
  DynamicIcon as LucideDynamicIcon,
  dynamicIconImports,
  type IconName,
} from "lucide-react/dynamic";
import type { LucideProps } from "lucide-react";

// Category and tag `icon` values are stored kebab-case in the DB
// ("shopping-cart") — the same keying lucide-react's DynamicIcon expects. We
// wrap it to (a) tolerate the nullable DB string and (b) guard unknown names,
// which lucide would otherwise console.error on. Each icon is lazily imported
// and code-split, so this never pulls the full icon set into the bundle.

interface DynamicIconProps extends Omit<LucideProps, "name"> {
  name?: string | null;
}

export function DynamicIcon({ name, ...props }: DynamicIconProps) {
  if (!name || !(name in dynamicIconImports)) return null;
  return <LucideDynamicIcon name={name as IconName} {...props} />;
}
