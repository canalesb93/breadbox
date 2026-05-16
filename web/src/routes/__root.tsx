import { useEffect } from "react";
import { Outlet, useNavigate, useRouterState } from "@tanstack/react-router";
import { Loader2, Search } from "lucide-react";
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
  // successfully. This kills the brief flash of sidebar + page content
  // before the 401 redirect fires.
  if (is401 || me.isPending || !me.data) {
    return <AuthSplash />;
  }
  if (me.error) {
    return <AuthError message={me.error.message} />;
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

function AuthError({ message }: { message: string }) {
  return (
    <div className="bg-background fixed inset-0 flex items-center justify-center p-6">
      <div className="max-w-sm text-center">
        <h1 className="text-base font-medium">Something went wrong</h1>
        <p className="text-muted-foreground mt-1 text-sm">{message}</p>
        <button
          type="button"
          onClick={() => window.location.reload()}
          className="text-primary mt-4 text-sm underline-offset-2 hover:underline"
        >
          Reload
        </button>
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
        <main className="min-w-0 flex-1 p-3 sm:p-6">
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
