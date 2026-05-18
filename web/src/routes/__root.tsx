import { useEffect } from "react";
import { Outlet, useNavigate, useRouterState } from "@tanstack/react-router";
import { AlertTriangle, Loader2, RefreshCw, Search } from "lucide-react";
import { AppSidebar } from "@/components/app-sidebar";
import { CommandPalette } from "@/components/command-palette";
import { SettingsShell } from "@/components/settings-shell";
import { ShortcutSheet } from "@/components/shortcut-sheet";
import {
  SidebarInset,
  SidebarProvider,
  SidebarTrigger,
} from "@/components/ui/sidebar";
import { Separator } from "@/components/ui/separator";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";
import { Kbd, KbdGroup } from "@/components/ui/kbd";
import { Toaster } from "@/components/ui/sonner";
import { Button } from "@/components/ui/button";
import { StatusPanel } from "@/components/status-panel";
import { NAV_LEAVES, isNavMatch } from "@/lib/nav";
import { useMe } from "@/api/queries/me";
import { ApiError } from "@/api/client";

const UNAUTHENTICATED_PATHS = new Set(["/login"]);
// Setup-account is parameterized (`/setup-account/$token`), so the prefix
// check covers every token variant without listing them.
const UNAUTHENTICATED_PREFIXES = ["/setup-account/"];

function isUnauthenticatedPath(pathname: string): boolean {
  if (UNAUTHENTICATED_PATHS.has(pathname)) return true;
  return UNAUTHENTICATED_PREFIXES.some((p) => pathname.startsWith(p));
}

export function RootLayout() {
  const pathname = useRouterState({ select: (s) => s.location.pathname });

  if (isUnauthenticatedPath(pathname)) {
    return (
      <>
        <Outlet />
        <Toaster />
      </>
    );
  }

  return <AuthenticatedGate pathname={pathname} />;
}

function AuthenticatedGate({ pathname }: { pathname: string }) {
  const me = useMe();
  const navigate = useNavigate();

  const is401 = me.error instanceof ApiError && me.error.status === 401;

  useEffect(() => {
    if (is401) {
      navigate({ to: "/login", search: { redirect: pathname } });
    }
  }, [is401, navigate, pathname]);

  // Gate: never render the authenticated shell until /me has resolved
  // successfully. Splash covers the loading window AND the 401-redirect
  // window (so the redirect fires from a calm loader instead of an error
  // flash). Real non-401 failures (network drop, 500, malformed payload)
  // surface as the AuthError panel with a Reload affordance.
  if (is401 || me.isPending) {
    return <AuthSplash />;
  }
  if (me.error) {
    return <AuthError message={me.error.message} />;
  }
  if (!me.data) {
    return <AuthSplash />;
  }

  return <AuthenticatedShell pathname={pathname} />;
}

function AuthSplash() {
  return (
    <div className="bg-background fixed inset-0 flex items-center justify-center">
      <Loader2 className="text-muted-foreground size-6 animate-spin" />
    </div>
  );
}

// AuthError is the gate's last-resort surface for `/me` failures that
// aren't a 401 (network down, 500, malformed payload). Predates the v2
// vocabulary; iter 91 routes it through `<StatusPanel tone="destructive">`
// so the bordered + tone-tinted icon-tile lockup matches every other v2
// error surface (PageError, providers env-locked notice, setup-account
// invalid-link). The panel sits centered on a full-bleed background — we
// can't reach for `<PageError>` here because the sidebar / page chrome
// isn't mounted yet (the gate failed before `<AuthenticatedShell>` could
// render), but the inner lockup is identical.
function AuthError({ message }: { message: string }) {
  return (
    <div className="bg-background fixed inset-0 flex items-center justify-center p-6">
      <div className="w-full max-w-md">
        <StatusPanel
          tone="destructive"
          icon={AlertTriangle}
          heading="Couldn't load your session"
          body={message}
          trailing={
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => window.location.reload()}
            >
              <RefreshCw className="size-3.5" />
              Reload
            </Button>
          }
        />
      </div>
    </div>
  );
}

function AuthenticatedShell({ pathname }: { pathname: string }) {
  const match = NAV_LEAVES.find(({ leaf }) => isNavMatch(leaf, pathname));
  const group = match?.group ?? "";
  const title = match && match.leaf.kind === "link" ? match.leaf.title : "Breadbox";

  return (
    <SidebarProvider>
      <AppSidebar />
      {/* min-w-0: the inset is a flex child — without it, a wide page (e.g. a
          horizontally-scrolling table) grows the inset past the viewport
          instead of letting the page's own overflow container scroll. */}
      <SidebarInset className="min-w-0">
        <header className="bg-background/95 supports-[backdrop-filter]:bg-background/80 sticky top-0 z-30 flex h-14 shrink-0 items-center gap-2 border-b backdrop-blur">
          <div className="flex w-full items-center gap-2 px-4">
            <SidebarTrigger className="-ml-1" />
            <Separator orientation="vertical" className="mr-1 h-4" />
            <Breadcrumb>
              <BreadcrumbList>
                {group && (
                  <>
                    <BreadcrumbItem className="hidden md:inline-flex">
                      <span className="text-muted-foreground">{group}</span>
                    </BreadcrumbItem>
                    <BreadcrumbSeparator className="hidden md:inline-flex" />
                  </>
                )}
                <BreadcrumbItem>
                  <BreadcrumbPage className="text-foreground font-medium">
                    {title}
                  </BreadcrumbPage>
                </BreadcrumbItem>
              </BreadcrumbList>
            </Breadcrumb>
            <div className="ml-auto flex items-center gap-2">
              <CommandPaletteTrigger />
            </div>
          </div>
        </header>
        {/* Page layout contract: <main> is a flex column with gap-5. Every
            section a page renders (PageHeader, toolbars, content, dialogs)
            becomes a direct child and inherits the 20px rhythm. Pages must
            NOT add their own `flex flex-col gap-*` wrapper — return a
            fragment (`<>`) so children sit directly under <main>. Pages that
            need a width constraint (`mx-auto max-w-2xl|5xl`) wrap once and
            apply `flex flex-col gap-5` on that wrapper themselves. */}
        <main className="flex min-w-0 flex-1 flex-col gap-5 p-3 sm:p-6">
          <Outlet />
        </main>
      </SidebarInset>
      <CommandPalette />
      <ShortcutSheet />
      <SettingsShell />
      <Toaster />
    </SidebarProvider>
  );
}

// Topbar pill that mirrors the command palette's purpose: tap or press
// ⌘K. The visual is intentionally search-input-ish (faded text, leading
// icon, trailing kbd hint) so it advertises the shortcut to first-time
// users without consuming a discrete keyboard control.
function CommandPaletteTrigger() {
  const open = () =>
    window.dispatchEvent(new CustomEvent("breadbox:command-palette:open"));

  return (
    <button
      type="button"
      onClick={open}
      className="text-muted-foreground hover:text-foreground hover:border-ring focus-visible:ring-ring/40 inline-flex h-8 items-center gap-2 rounded-md border bg-transparent px-2.5 text-xs transition-colors focus-visible:ring-2 focus-visible:outline-none sm:min-w-56"
      aria-label="Open command palette"
    >
      <Search className="size-3.5 shrink-0" aria-hidden />
      <span className="hidden flex-1 text-left sm:inline">
        Search or jump to…
      </span>
      <KbdGroup className="ml-auto hidden sm:inline-flex">
        <Kbd className="bg-muted/60">⌘</Kbd>
        <Kbd className="bg-muted/60">K</Kbd>
      </KbdGroup>
    </button>
  );
}
