import { Bot, ShieldCheck, ShieldOff, User as UserIcon } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import type { APIKeyActorType, APIKeyScope } from "@/api/types";

interface ScopeBadgeProps {
  scope: APIKeyScope;
}

// ScopeBadge shows whether a key can write. Read-only is a muted secondary
// chip; full-access wears the primary outline so it reads as the elevated
// permission.
export function ScopeBadge({ scope }: ScopeBadgeProps) {
  if (scope === "read_only") {
    return (
      <Badge variant="secondary" className="gap-1">
        <ShieldOff className="size-3" />
        Read only
      </Badge>
    );
  }
  return (
    <Badge variant="outline" className="gap-1">
      <ShieldCheck className="size-3" />
      Full access
    </Badge>
  );
}

interface ActorBadgeProps {
  type: APIKeyActorType;
  name?: string | null;
}

const ACTOR_ICON = {
  user: UserIcon,
  agent: Bot,
  system: ShieldCheck,
} as const;

const ACTOR_LABEL: Record<APIKeyActorType, string> = {
  user: "User",
  agent: "Agent",
  system: "System",
};

// ActorBadge captures who minted the key. The optional `name` is appended in
// muted text — useful when a household has several agent integrations and
// the type alone isn't enough to tell them apart.
export function ActorBadge({ type, name }: ActorBadgeProps) {
  const Icon = ACTOR_ICON[type];
  return (
    <span className="text-muted-foreground inline-flex items-center gap-1.5 text-xs">
      <Icon className="size-3.5" />
      <span className="text-foreground">{ACTOR_LABEL[type]}</span>
      {name && <span className="text-muted-foreground/70">· {name}</span>}
    </span>
  );
}
