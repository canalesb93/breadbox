import {
  DatabaseBackup,
  Lock,
  User,
  Users,
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
    slug: "household",
    title: "Household",
    description: "Family members and shared access.",
    icon: Users,
  },
  {
    slug: "security",
    title: "Security",
    description: "Sessions and API keys.",
    icon: Lock,
  },
  {
    slug: "backups",
    title: "Backups",
    description: "Snapshot the database, restore, and schedule automatic backups.",
    icon: DatabaseBackup,
  },
];
