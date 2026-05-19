import { useMemo, useState } from "react";
import { Link2, Loader2 } from "lucide-react";
import { Sheet, SheetContent } from "@/components/ui/sheet";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { DetailSheetHeader } from "@/components/detail-sheet-header";
import { Eyebrow } from "@/components/eyebrow";
import { withMutationToast } from "@/lib/mutation-toast";
import {
  useAccountLinks,
  useCreateAccountLink,
} from "@/api/queries/account-links";
import type { Account } from "@/api/types";
import { accountLabel } from "./account-utils";

interface LinkAccountSheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  // The account currently being viewed — becomes the primary side of the
  // new link. We never let users pick which side is primary from this sheet
  // because the detail page is the primary's home; we just pick the
  // dependent.
  primary: Pick<Account, "id" | "short_id" | "name"> & {
    display_name?: string | null;
  };
  accounts: Account[];
}

// LinkAccountSheet creates a new primary→dependent link from the detail
// page. Filters the eligible-dependent list to exclude the primary itself,
// accounts already linked as dependents anywhere, and accounts that have a
// reverse link with this primary (the backend rejects those — we mirror
// the rule client-side so we can disable the options and show why).
export function LinkAccountSheet({
  open,
  onOpenChange,
  primary,
  accounts,
}: LinkAccountSheetProps) {
  const linksQuery = useAccountLinks();
  const create = useCreateAccountLink();

  const [dependentId, setDependentId] = useState<string>("");
  const [toleranceDays, setToleranceDays] = useState<string>("3");

  const links = linksQuery.data ?? [];

  const candidates = useMemo(() => {
    return accounts.filter((a) => {
      if (a.short_id === primary.short_id) return false;
      // Already a dependent in another link — backend hides these too.
      if (a.is_dependent_linked) return false;
      // Reverse link exists: this candidate is primary for our viewing
      // account. The backend rejects creating the inverse — we mirror the
      // rule client-side so users see why the option is missing. The list
      // endpoint returns short_ids in *_account_id (cf. the project's
      // compact-ID convention), so we compare on short_id here.
      const reverse = links.find(
        (l) =>
          l.primary_account_id === a.short_id &&
          l.dependent_account_id === primary.short_id,
      );
      if (reverse) return false;
      return true;
    });
  }, [accounts, links, primary.short_id]);

  const eligibleCount = candidates.length;

  async function onSubmit() {
    if (!dependentId) return;
    const tol = Number(toleranceDays);
    const ok = await withMutationToast(
      () =>
        create.mutateAsync({
          primary_account_id: primary.id,
          dependent_account_id: dependentId,
          match_tolerance_days: Number.isFinite(tol) && tol >= 0 ? tol : undefined,
        }),
      {
        success: "Accounts linked.",
        successDescription: "Matches will reconcile on the next sync.",
      },
    );
    if (ok) {
      setDependentId("");
      onOpenChange(false);
    }
  }

  const primaryLabel = accountLabel(primary);

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="flex w-full flex-col gap-0 p-0 sm:max-w-md">
        <DetailSheetHeader
          density="accent"
          icon={Link2}
          eyebrow="Account link"
          title="Link an account"
          description="Attribute matched transactions on a dependent account to its primary cardholder. The dependent account is excluded from totals so charges aren't double-counted."
        />

        <div className="flex flex-1 flex-col gap-5 overflow-y-auto p-6">
          <div className="flex flex-col gap-2">
            <Eyebrow as="p">Primary (this account)</Eyebrow>
            <div className="bg-muted/40 rounded-md border px-3 py-2 text-sm">
              {primaryLabel}
            </div>
          </div>

          <div className="flex flex-col gap-2">
            <Eyebrow as="label" htmlFor="dependent">
              Dependent account
            </Eyebrow>
            {eligibleCount === 0 ? (
              <Alert>
                <AlertTitle>No eligible accounts</AlertTitle>
                <AlertDescription>
                  Every other account is either already linked as a dependent,
                  or would create a circular link with this one.
                </AlertDescription>
              </Alert>
            ) : (
              <Select value={dependentId} onValueChange={setDependentId}>
                <SelectTrigger id="dependent" className="w-full">
                  <SelectValue placeholder="Pick an account…" />
                </SelectTrigger>
                <SelectContent>
                  {candidates.map((a) => (
                    <SelectItem key={a.id} value={a.id}>
                      <span className="font-medium">{accountLabel(a)}</span>
                      <span className="text-muted-foreground ml-2 text-xs">
                        {a.institution_name ?? "Manual"}
                      </span>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
            <p className="text-muted-foreground text-xs">
              Matched transactions on the dependent account will be attributed
              to the same person as this account.
            </p>
          </div>

          <div className="flex flex-col gap-2">
            <Eyebrow as="label" htmlFor="tolerance">
              Match tolerance (days)
            </Eyebrow>
            <Input
              id="tolerance"
              type="number"
              inputMode="numeric"
              min={0}
              max={30}
              value={toleranceDays}
              onChange={(e) => setToleranceDays(e.target.value)}
              className="h-9 w-24"
            />
            <p className="text-muted-foreground text-xs">
              How far apart two transactions can post and still match. 3 days
              is a safe default for most billing-cycle delays.
            </p>
          </div>
        </div>

        <div className="bg-muted/20 flex items-center justify-end gap-2 border-t px-6 py-3">
          <Button
            variant="outline"
            size="sm"
            onClick={() => onOpenChange(false)}
            disabled={create.isPending}
          >
            Cancel
          </Button>
          <Button
            size="sm"
            onClick={onSubmit}
            disabled={!dependentId || create.isPending || eligibleCount === 0}
          >
            {create.isPending && <Loader2 className="size-4 animate-spin" />}
            Link accounts
          </Button>
        </div>
      </SheetContent>
    </Sheet>
  );
}

