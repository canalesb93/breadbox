import { Link, useRouterState } from "@tanstack/react-router";
import {
  SidebarGroup,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar";
import { isNavMatch, type NavGroup } from "@/lib/nav";

export function NavMain({ group }: { group: NavGroup }) {
  const pathname = useRouterState({ select: (s) => s.location.pathname });

  return (
    <SidebarGroup>
      <SidebarGroupLabel>{group.label}</SidebarGroupLabel>
      <SidebarMenu>
        {group.items.map((item) => {
          const Icon = item.icon;
          return (
            <SidebarMenuItem key={item.to}>
              <SidebarMenuButton
                asChild
                isActive={isNavMatch(item, pathname)}
                tooltip={item.title}
              >
                <Link to={item.to}>
                  <Icon />
                  <span>{item.title}</span>
                </Link>
              </SidebarMenuButton>
            </SidebarMenuItem>
          );
        })}
      </SidebarMenu>
    </SidebarGroup>
  );
}
