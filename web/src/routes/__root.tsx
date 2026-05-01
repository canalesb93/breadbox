import { useEffect } from "react";
import { Outlet, useRouterState } from "@tanstack/react-router";
import { AppSidebar } from "@/components/app-sidebar";
import {
  SidebarInset,
  SidebarProvider,
  SidebarTrigger,
} from "@/components/ui/sidebar";
import { Separator } from "@/components/ui/separator";
import { NAV_LEAVES, isNavMatch } from "@/lib/nav";
import { useMe } from "@/api/queries/me";
import { ApiError } from "@/api/client";

export function RootLayout() {
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const match = NAV_LEAVES.find((leaf) => isNavMatch(leaf, pathname));
  const group = match?.group ?? "";
  const title = match?.title ?? "Breadbox";

  // Redirect to /login when the session is missing. The api client surfaces
  // 401 as an ApiError; this is the single place that knows what to do.
  const { error } = useMe();
  useEffect(() => {
    if (error instanceof ApiError && error.status === 401) {
      window.location.href = "/login";
    }
  }, [error]);

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
    </SidebarProvider>
  );
}
