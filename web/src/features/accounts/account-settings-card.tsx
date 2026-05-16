import { useEffect, useState } from "react";
import { Link } from "@tanstack/react-router";
import { Check, ExternalLink, Loader2, Pencil, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { withMutationToast } from "@/lib/mutation-toast";
import { useUpdateAccount } from "@/api/queries/accounts";
import type { AccountDetail } from "@/api/types";

interface AccountSettingsCardProps {
  account: AccountDetail;
}

// AccountSettingsCard is the "what about this account" panel: an inline
// rename for display_name (overrides the bank-provided name everywhere),
// the exclude-from-sync toggle, and quick links to the parent connection
// and household member. Each mutation toasts on its own — failed writes
// roll back to the cached value because we don't optimistic-update.
export function AccountSettingsCard({ account: a }: AccountSettingsCardProps) {
  const update = useUpdateAccount();
  const [editingName, setEditingName] = useState(false);
  const [draft, setDraft] = useState(a.display_name ?? "");

  // Keep the draft in sync if a server refetch lands while we're not
  // editing (e.g. another tab renames the same account).
  useEffect(() => {
    if (!editingName) setDraft(a.display_name ?? "");
  }, [a.display_name, editingName]);

  async function saveName() {
    const next = draft.trim();
    if (next === (a.display_name ?? "")) {
      setEditingName(false);
      return;
    }
    const ok = await withMutationToast(
      () => update.mutateAsync({ id: a.short_id, input: { display_name: next } }),
      { success: next === "" ? "Display name cleared." : "Display name updated." },
    );
    if (ok) setEditingName(false);
  }

  function cancelName() {
    setDraft(a.display_name ?? "");
    setEditingName(false);
  }

  async function onToggleExcluded(next: boolean) {
    await withMutationToast(
      () => update.mutateAsync({ id: a.short_id, input: { is_excluded: next } }),
      {
        success: next
          ? "Account excluded — future syncs will skip it."
          : "Account included in sync.",
      },
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Settings</CardTitle>
      </CardHeader>
      <CardContent className="space-y-5 text-sm">
        <div className="space-y-1.5">
          <Label htmlFor="display-name" className="text-muted-foreground text-xs">
            Display name
          </Label>
          {editingName ? (
            <div className="flex items-center gap-1.5">
              <Input
                id="display-name"
                autoFocus
                value={draft}
                placeholder={a.name}
                onChange={(e) => setDraft(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") saveName();
                  if (e.key === "Escape") cancelName();
                }}
                disabled={update.isPending}
                className="h-9"
              />
              <Button
                size="icon"
                variant="ghost"
                onClick={saveName}
                disabled={update.isPending}
                aria-label="Save display name"
                className="size-9"
              >
                {update.isPending ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <Check className="size-4" />
                )}
              </Button>
              <Button
                size="icon"
                variant="ghost"
                onClick={cancelName}
                disabled={update.isPending}
                aria-label="Cancel"
                className="size-9"
              >
                <X className="size-4" />
              </Button>
            </div>
          ) : (
            <div className="flex items-center justify-between gap-2">
              <div className="min-w-0">
                <div className="truncate text-sm font-medium">
                  {a.display_name ?? a.name}
                </div>
                {a.display_name && (
                  <div className="text-muted-foreground truncate text-xs">
                    Overrides bank name: {a.name}
                  </div>
                )}
              </div>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => setEditingName(true)}
              >
                <Pencil className="size-3.5" /> Edit
              </Button>
            </div>
          )}
        </div>

        <div className="border-border/40 flex items-start justify-between gap-3 border-t pt-4">
          <div className="space-y-0.5">
            <Label
              htmlFor="excluded"
              className="text-sm font-medium"
            >
              Exclude from sync
            </Label>
            <p className="text-muted-foreground text-xs">
              Skip future transaction syncs for this account.
            </p>
          </div>
          <Checkbox
            id="excluded"
            checked={a.excluded}
            disabled={update.isPending}
            onCheckedChange={(next) => onToggleExcluded(next === true)}
          />
        </div>

        {(a.connection_short_id || a.connection_user_name) && (
          <div className="border-border/40 grid grid-cols-2 gap-4 border-t pt-4">
            {a.connection_short_id && (
              <div>
                <div className="text-muted-foreground text-xs">Connection</div>
                <Button
                  variant="link"
                  size="sm"
                  asChild
                  className="-ml-2 h-auto p-2 text-sm font-medium"
                >
                  <Link
                    to="/connections/$id"
                    params={{ id: a.connection_short_id }}
                  >
                    {a.institution_name ?? "Open connection"}
                    <ExternalLink className="size-3" />
                  </Link>
                </Button>
              </div>
            )}
            {a.connection_user_name && (
              <div>
                <div className="text-muted-foreground text-xs">Family member</div>
                <div className="mt-1 text-sm font-medium">
                  {a.connection_user_name}
                </div>
              </div>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
