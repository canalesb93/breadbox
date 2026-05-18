import {
  ArrowLeftRight,
  BadgeDollarSign,
  Banknote,
  Bot,
  FileText,
  Key,
  PenTool,
  Plug,
  Settings,
  Shapes,
  Tags,
  ToyBrick,
  Wand2,
  type LucideIcon,
} from "lucide-react";

interface NavLeafBase {
  title: string;
  icon: LucideIcon;
}

export interface NavLeafLink extends NavLeafBase {
  kind: "link";
  to: string;
}

export interface NavLeafModal extends NavLeafBase {
  kind: "modal";
  modalKey: string;
}

export type NavLeaf = NavLeafLink | NavLeafModal;

export interface NavGroup {
  label: string;
  items: NavLeaf[];
}

export interface NavLeafWithGroup {
  leaf: NavLeaf;
  group: string;
}

export function isNavMatch(item: NavLeaf, pathname: string): boolean {
  if (item.kind !== "link") return false;
  return item.to === "/" ? pathname === "/" : pathname.startsWith(item.to);
}

export function navKey(item: NavLeaf): string {
  return item.kind === "link" ? `link:${item.to}` : `modal:${item.modalKey}`;
}

export const NAV: NavGroup[] = [
  {
    label: "Money",
    items: [
      { kind: "link", title: "Home", to: "/", icon: BadgeDollarSign },
      { kind: "link", title: "Transactions", to: "/transactions", icon: ArrowLeftRight },
      { kind: "link", title: "Reports", to: "/reports", icon: FileText },
    ],
  },
  {
    label: "Library",
    items: [
      { kind: "link", title: "Accounts", to: "/accounts", icon: Banknote },
      { kind: "link", title: "Connections", to: "/connections", icon: Plug },
      { kind: "link", title: "Providers", to: "/providers", icon: ToyBrick },
      { kind: "link", title: "Categories", to: "/categories", icon: Shapes },
      { kind: "link", title: "Tags", to: "/tags", icon: Tags },
      { kind: "link", title: "Rules", to: "/rules", icon: Wand2 },
    ],
  },
  {
    label: "System",
    items: [
      { kind: "link", title: "Agents", to: "/agents", icon: Bot },
      { kind: "link", title: "Prompt builder", to: "/prompts/build", icon: PenTool },
      { kind: "link", title: "API keys", to: "/api-keys", icon: Key },
      { kind: "modal", title: "Settings", modalKey: "settings", icon: Settings },
    ],
  },
];

export const NAV_LEAVES: NavLeafWithGroup[] = NAV.flatMap((g) =>
  g.items.map((leaf) => ({ leaf, group: g.label })),
);
