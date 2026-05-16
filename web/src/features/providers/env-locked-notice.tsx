import { Lock } from "lucide-react";
import { StatusPanel } from "@/components/status-panel";

interface EnvLockedNoticeProps {
  provider: string;
}

// Shown in place of the form when the provider was configured via env vars.
// The server-side handler rejects writes in this state (returns 409
// PROVIDER_FROM_ENV), so editing inputs would just lead to a failed submit.
//
// Uses the shared `<StatusPanel>` primitive (iter 16 promotion) so this
// matches the setup-account success/error vocabulary.
export function EnvLockedNotice({ provider }: EnvLockedNoticeProps) {
  return (
    <StatusPanel
      tone="info"
      icon={Lock}
      heading={`${provider} is configured via environment variables.`}
      body="To change these values, update the env vars on the server and restart Breadbox."
    />
  );
}
