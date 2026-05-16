import { useEffect } from "react";
import { Outlet, useNavigate, useRouterState } from "@tanstack/react-router";
import { Loader2 } from "lucide-react";
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
        <header className="flex h-14 shrink-0 items-center gap-2 border-b">
          <div className="flex items-center gap-2 px-4">
            <SidebarTrigger className="-ml-1" />
            <Separator orientation="vertical" className="mr-2 h-4" />
            {group && (
              <>
                <span className="text-muted-foreground text-sm">{group}</span>
                <span className="text-muted-foreground text-sm">/</span>
              </>
            )}
            <span className="text-sm font-medium">{title}</span>
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
