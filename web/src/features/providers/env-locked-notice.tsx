import { Lock } from "lucide-react";

interface EnvLockedNoticeProps {
  provider: string;
}

// Shown in place of the form when the provider was configured via env vars.
// The server-side handler rejects writes in this state (returns 409
// PROVIDER_FROM_ENV), so editing inputs would just lead to a failed submit.
export function EnvLockedNotice({ provider }: EnvLockedNoticeProps) {
  return (
    <div className="bg-muted/50 text-muted-foreground flex items-start gap-2 rounded-md border px-3 py-2.5 text-xs">
      <Lock className="mt-0.5 size-3.5 shrink-0" />
      <div>
        <span className="text-foreground font-medium">{provider} is configured via environment variables.</span>{" "}
        To change these values, update the env vars and restart Breadbox.
      </div>
    </div>
  );
}
