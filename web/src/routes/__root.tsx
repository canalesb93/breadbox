import { useEffect } from "react";
import { Outlet, useNavigate, useRouterState } from "@tanstack/react-router";
import { AppSidebar } from "@/components/app-sidebar";
import { CommandPalette } from "@/components/command-palette";
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

export function RootLayout() {
  const pathname = useRouterState({ select: (s) => s.location.pathname });

  if (UNAUTHENTICATED_PATHS.has(pathname)) {
    return (
      <>
        <Outlet />
        <Toaster />
      </>
    );
  }

  return <AuthenticatedShell pathname={pathname} />;
}

function AuthenticatedShell({ pathname }: { pathname: string }) {
  const navigate = useNavigate();
  const match = NAV_LEAVES.find((leaf) => isNavMatch(leaf, pathname));
  const group = match?.group ?? "";
  const title = match?.title ?? "Breadbox";

  // Redirect to /v2/login when the session is missing. The api client
  // surfaces 401 as an ApiError; this is the single place that knows what
  // to do.
  const { error } = useMe();
  useEffect(() => {
    if (error instanceof ApiError && error.status === 401) {
      navigate({ to: "/login", search: { redirect: pathname } });
    }
  }, [error, navigate, pathname]);

  return (
    <SidebarProvider>
      <AppSidebar />
      <SidebarInset>
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
        <main className="flex-1 p-6">
          <Outlet />
        </main>
      </SidebarInset>
      <CommandPalette />
      <ShortcutSheet />
      <Toaster />
    </SidebarProvider>
  );
}
