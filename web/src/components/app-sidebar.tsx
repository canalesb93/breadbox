import * as React from "react";
import { BrandHeader } from "@/components/brand-header";
import { NavMain } from "@/components/nav-main";
import { NavUser } from "@/components/nav-user";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarRail,
} from "@/components/ui/sidebar";
import { NAV } from "@/lib/nav";
import { useMe } from "@/api/queries/me";

export function AppSidebar(props: React.ComponentProps<typeof Sidebar>) {
  const { data: me } = useMe();

  return (
    <Sidebar collapsible="icon" {...props}>
      <SidebarHeader>
        <BrandHeader />
      </SidebarHeader>
      <SidebarContent>
        {NAV.map((group) => (
          <NavMain key={group.label} group={group} />
        ))}
      </SidebarContent>
      <SidebarFooter>
        <NavUser me={me ?? null} />
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  );
}
