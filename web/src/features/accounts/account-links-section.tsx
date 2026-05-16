import { useMemo } from "react";
import { Link } from "@tanstack/react-router";
import {
  ArrowRight,
  Link2,
  Loader2,
  MoreHorizontal,
  Plus,
  RotateCw,
  Unlink,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { withMutationToast } from "@/lib/mutation-toast";
import {
  useAccountLinks,
  useDeleteAccountLink,
  useReconcileAccountLink,
} from "@/api/queries/account-links";
import type { Account, AccountLink } from "@/api/types";

interface AccountLinksSectionProps {
  account: Pick<Account, "id" | "short_id" | "name" | "is_dependent_linked">;
  // The full account list — needed because account-links carry only the
  // account short_id, and we link to detail pages by short_id elsewhere.
  accounts: Account[];
  onAddLink: () => void;
}

// AccountLinksSection lists every link this account participates in —
// either as the primary cardholder ("This account covers …") or as a
// dependent ("Attributed to …"). Each row shows match counts and exposes
// reconcile + unlink actions.
//
// Account-linking lives here because the dedicated top-level "Account
// Links" page was removed; surfacing the links inline on each account is
// the better UX anyway — you set them up where you're already looking.
export function AccountLinksSection({
  account,
  accounts,
  onAddLink,
}: AccountLinksSectionProps) {
  const linksQuery = useAccountLinks();

  // Index by short_id — that's the value the list endpoint returns in
  // `primary_account_id`/`dependent_account_id` (the project-wide compact-ID
  // convention).
  const accountByShortId = useMemo(() => {
    const m = new Map<string, Account>();
    for (const a of accounts) m.set(a.short_id, a);
    return m;
  }, [accounts]);

  const myLinks = useMemo(() => {
    if (!linksQuery.data) return [] as AccountLink[];
    return linksQuery.data.filter(
      (l) =>
        l.primary_account_id === account.short_id ||
        l.dependent_account_id === account.short_id,
    );
  }, [linksQuery.data, account.short_id]);

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Link2 className="size-4" /> Account links
        </CardTitle>
        <CardAction>
          <Button
            size="sm"
            variant="outline"
            onClick={onAddLink}
            disabled={account.is_dependent_linked}
          >
            <Plus className="size-3.5" />
            Link an account
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent className="space-y-3">
        {linksQuery.isLoading ? (
          <div className="text-muted-foreground flex items-center gap-2 text-xs">
            <Loader2 className="size-3 animate-spin" /> Loading links…
          </div>
        ) : linksQuery.isError ? (
          <Alert variant="destructive">
            <AlertTitle>Couldn't load account links</AlertTitle>
            <AlertDescription>
              {linksQuery.error instanceof Error
                ? linksQuery.error.message
                : "Try refreshing the page."}
            </AlertDescription>
          </Alert>
        ) : myLinks.length === 0 ? (
          <EmptyLinkState
            account={account}
          />
        ) : (
          <ul className="space-y-2">
            {myLinks.map((link) => (
              <LinkRow
                key={link.id}
                link={link}
                viewingShortId={account.short_id}
                accountByShortId={accountByShortId}
              />
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}

interface EmptyLinkStateProps {
  account: Pick<Account, "is_dependent_linked" | "name">;
}

function EmptyLinkState({ account }: EmptyLinkStateProps) {
  if (account.is_dependent_linked) {
    return (
      <div className="text-muted-foreground text-sm">
        This account is linked as a dependent. Its transactions are attributed
        to a primary cardholder. Open the primary account to manage the link.
      </div>
    );
  }
  return (
    <div className="text-muted-foreground space-y-2 text-sm">
      <p>
        Link a dependent account when one person&apos;s charges show up on
        another person&apos;s statement (e.g. a family card). Breadbox
        attributes matched transactions to the dependent user and removes the
        dependent account from totals so spending isn&apos;t double-counted.
      </p>
    </div>
  );
}

interface LinkRowProps {
  link: AccountLink;
  viewingShortId: string;
  accountByShortId: Map<string, Account>;
}

function LinkRow({ link, viewingShortId, accountByShortId }: LinkRowProps) {
  const reconcile = useReconcileAccountLink();
  const remove = useDeleteAccountLink();

  const isPrimary = link.primary_account_id === viewingShortId;
  const otherShortId = isPrimary
    ? link.dependent_account_id
    : link.primary_account_id;
  // The list endpoint returns short_ids in *_account_id; the singular
  // endpoint returns UUIDs. Look up by short_id so the rendered link to
  // the other account works in both shapes — falls back to the link's
  // own labels when the account isn't in the cache.
  const other = accountByShortId.get(otherShortId);
  const otherName = isPrimary
    ? link.dependent_account_name
    : link.primary_account_name;
  const otherUser = isPrimary ? link.dependent_user_name : link.primary_user_name;

  async function onReconcile() {
    await withMutationToast(
      async () => {
        const r = await reconcile.mutateAsync(link.id);
        return r;
      },
      {
        success: `Reconciled — ${reconcile.data?.new_matches ?? 0} new matches.`,
      },
    );
  }

  async function onDelete() {
    if (!confirm("Unlink these accounts? Matches will be discarded.")) return;
    await withMutationToast(() => remove.mutateAsync(link.id), {
      success: "Accounts unlinked.",
    });
  }

  return (
    <li className="bg-muted/30 flex items-start gap-3 rounded-md border p-3 text-sm">
      <div className="flex-1 space-y-1.5">
        <div className="flex flex-wrap items-center gap-1.5">
          <Badge variant={isPrimary ? "default" : "secondary"} className="text-[10px]">
            {isPrimary ? "Primary" : "Dependent"}
          </Badge>
          <span className="text-muted-foreground text-xs">
            {isPrimary ? "Covers" : "Attributed to"}
          </span>
          <ArrowRight className="text-muted-foreground size-3" />
          {other ? (
            <Link
              to="/accounts/$id"
              params={{ id: other.short_id }}
              className="text-sm font-medium hover:underline"
            >
              {otherName}
            </Link>
          ) : (
            <span className="text-sm font-medium">{otherName}</span>
          )}
          {otherUser && (
            <span className="text-muted-foreground text-xs">· {otherUser}</span>
          )}
        </div>
        <div className="text-muted-foreground flex flex-wrap items-center gap-x-3 gap-y-1 text-xs tabular-nums">
          <span>{link.match_count} matched</span>
          {link.unmatched_dependent_count > 0 && (
            <span className="text-amber-700 dark:text-amber-400">
              {link.unmatched_dependent_count} unmatched
            </span>
          )}
          <span>± {link.match_tolerance_days}d tolerance</span>
          {!link.enabled && <Badge variant="outline" className="text-[10px]">Disabled</Badge>}
        </div>
      </div>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            size="icon"
            variant="ghost"
            className="size-7"
            aria-label="Link actions"
            disabled={reconcile.isPending || remove.isPending}
          >
            {reconcile.isPending || remove.isPending ? (
              <Loader2 className="size-3.5 animate-spin" />
            ) : (
              <MoreHorizontal className="size-3.5" />
            )}
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onClick={onReconcile} disabled={reconcile.isPending}>
            <RotateCw className="size-3.5" /> Reconcile matches
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            variant="destructive"
            onClick={onDelete}
            disabled={remove.isPending}
          >
            <Unlink className="size-3.5" /> Unlink
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </li>
  );
}
