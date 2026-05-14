import {
  Bell,
  Brush,
  CreditCard,
  Lock,
  User,
  type LucideIcon,
} from "lucide-react";

export interface SettingsSection {
  slug: string;
  title: string;
  description: string;
  icon: LucideIcon;
}

export const SETTINGS_SECTIONS: SettingsSection[] = [
  {
    slug: "account",
    title: "Account",
    description: "Profile, password, and identity.",
    icon: User,
  },
  {
    slug: "appearance",
    title: "Appearance",
    description: "Theme follows your system preference.",
    icon: Brush,
  },
  {
    slug: "notifications",
    title: "Notifications",
    description: "Email and in-app alerts.",
    icon: Bell,
  },
  {
    slug: "billing",
    title: "Billing",
    description: "Plan and invoices.",
    icon: CreditCard,
  },
  {
    slug: "security",
    title: "Security",
    description: "Sessions and API keys.",
    icon: Lock,
  },
];
