import { Lock, User, type LucideIcon } from "lucide-react";

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
    slug: "security",
    title: "Security",
    description: "Sessions and API keys.",
    icon: Lock,
  },
];
