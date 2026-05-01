import { Outlet } from "@tanstack/react-router";
import { AppSidebar } from "@/components/app-sidebar";
import {
  SidebarInset,
  SidebarProvider,
  SidebarTrigger,
} from "@/components/ui/sidebar";
import { Separator } from "@/components/ui/separator";
import { useRouterState } from "@tanstack/react-router";
import { NAV } from "@/lib/nav";

function pageTitle(pathname: string): { group: string; title: string } {
  for (const group of NAV) {
    for (const item of group.items) {
      const match =
        item.to === "/" ? pathname === "/" : pathname.startsWith(item.to);
      if (match) return { group: group.label, title: item.title };
    }
  }
  return { group: "", title: "Breadbox" };
}

export function RootLayout() {
  const { location } = useRouterState();
  const { group, title } = pageTitle(location.pathname);

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
