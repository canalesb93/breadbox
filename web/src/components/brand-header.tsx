import { Link } from "@tanstack/react-router";
import { Box } from "lucide-react";
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar";

// Top-of-sidebar brand mark. The icon sits in a tinted tile so the brand
// reads as a logo lockup rather than a generic nav item, and the "v2 ·
// preview" tag is a real badge — same visual language we use elsewhere
// (Badge primitive) — so it's clearly a status pill, not body copy.
export function BrandHeader() {
  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <SidebarMenuButton
          tooltip="Breadbox"
          asChild
          size="lg"
          className="group-data-[collapsible=icon]:!p-1.5"
        >
          <Link to="/">
            <span className="bg-primary text-primary-foreground flex aspect-square size-8 items-center justify-center rounded-md">
              <Box className="size-4" />
            </span>
            <div className="flex flex-1 flex-col gap-0.5 truncate leading-none">
              <span className="text-sm font-semibold tracking-tight">
                Breadbox
              </span>
              <span className="text-muted-foreground inline-flex items-center gap-1.5 text-[10px] font-medium tracking-wide uppercase">
                <span className="bg-primary/15 text-primary rounded-sm px-1 py-px text-[10px] font-semibold tracking-wider">
                  v2
                </span>
                Preview
              </span>
            </div>
          </Link>
        </SidebarMenuButton>
      </SidebarMenuItem>
    </SidebarMenu>
  );
}
