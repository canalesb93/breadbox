import {
  ArrowLeftRight,
  BadgeDollarSign,
  Banknote,
  Bot,
  ClipboardCheck,
  FileText,
  Key,
  LineChart,
  Link2,
  Plug,
  Settings,
  Shapes,
  Tags,
  Wand2,
  type LucideIcon,
} from "lucide-react";

export interface NavLeaf {
  title: string;
  to: string;
  icon: LucideIcon;
}

export interface NavGroup {
  label: string;
  items: NavLeaf[];
}

export const NAV: NavGroup[] = [
  {
    label: "Money",
    items: [
      { title: "Home", to: "/", icon: BadgeDollarSign },
      { title: "Transactions", to: "/transactions", icon: ArrowLeftRight },
      { title: "Reviews", to: "/reviews", icon: ClipboardCheck },
      { title: "Insights", to: "/insights", icon: LineChart },
      { title: "Reports", to: "/reports", icon: FileText },
    ],
  },
  {
    label: "Setup",
    items: [
      { title: "Accounts", to: "/accounts", icon: Banknote },
      { title: "Connections", to: "/connections", icon: Plug },
      { title: "Account links", to: "/account-links", icon: Link2 },
    ],
  },
  {
    label: "Automation",
    items: [
      { title: "Rules", to: "/rules", icon: Wand2 },
      { title: "Agents", to: "/agents", icon: Bot },
    ],
  },
  {
    label: "Admin",
    items: [
      { title: "Categories", to: "/categories", icon: Shapes },
      { title: "Tags", to: "/tags", icon: Tags },
      { title: "API keys", to: "/api-keys", icon: Key },
      { title: "Settings", to: "/settings", icon: Settings },
    ],
  },
];
