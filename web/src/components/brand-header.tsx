import { Link } from "@tanstack/react-router";
import { Box } from "lucide-react";
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar";

export function BrandHeader() {
  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <SidebarMenuButton tooltip="Breadbox" asChild>
          <Link to="/">
            <Box className="text-primary" />
            <div className="flex flex-1 items-baseline gap-1.5 truncate">
              <span className="font-semibold">Breadbox</span>
              <span className="text-xs text-muted-foreground">v2 · preview</span>
            </div>
          </Link>
        </SidebarMenuButton>
      </SidebarMenuItem>
    </SidebarMenu>
  );
}
