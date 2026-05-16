import {
  Check,
  ChevronsUpDown,
  ExternalLink,
  LogOut,
  Monitor,
  Moon,
  Palette,
  Sun,
  SunMoon,
} from "lucide-react";
import { Link, useNavigate } from "@tanstack/react-router";
import { useTheme } from "next-themes";
import { toast } from "sonner";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuPortal,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "@/components/ui/sidebar";
import type { Me } from "@/api/types";
import { useLogout } from "@/api/queries/auth";

function initials(name: string) {
  const local = name.split("@")[0];
  const parts = local.split(/[._-]/).filter(Boolean);
  return (parts[0]?.[0] ?? "?").concat(parts[1]?.[0] ?? "").toUpperCase();
}

const THEME_OPTIONS = [
  { value: "light", label: "Light", icon: Sun },
  { value: "dark", label: "Dark", icon: Moon },
  { value: "system", label: "System", icon: Monitor },
] as const;

export function NavUser({ me }: { me: Me | null }) {
  const { isMobile } = useSidebar();
  const navigate = useNavigate();
  const logout = useLogout();
  const { theme, setTheme } = useTheme();

  const onLogout = async () => {
    try {
      await logout.mutateAsync();
    } catch {
      // Even on error we still send the user to /login — the cookie may be
      // gone client-side; surface a toast but don't block navigation.
      toast.error("Couldn't reach the server — signing out anyway.");
    }
    navigate({ to: "/login" });
  };

  if (!me) {
    return (
      <SidebarMenu>
        <SidebarMenuItem>
          <SidebarMenuButton size="lg" disabled>
            <Avatar className="h-8 w-8 rounded-lg">
              <AvatarFallback className="rounded-lg">…</AvatarFallback>
            </Avatar>
            <span className="text-muted-foreground text-sm">Loading…</span>
          </SidebarMenuButton>
        </SidebarMenuItem>
      </SidebarMenu>
    );
  }

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <SidebarMenuButton
              size="lg"
              className="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground"
            >
              <Avatar className="h-8 w-8 rounded-lg">
                <AvatarFallback className="rounded-lg">{initials(me.username)}</AvatarFallback>
              </Avatar>
              <div className="grid flex-1 text-left text-sm leading-tight">
                <span className="truncate font-medium">{me.username}</span>
                <span className="truncate text-xs text-muted-foreground">{me.role}</span>
              </div>
              <ChevronsUpDown className="ml-auto size-4" />
            </SidebarMenuButton>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            className="w-(--radix-dropdown-menu-trigger-width) min-w-56 rounded-lg"
            side={isMobile ? "bottom" : "right"}
            align="end"
            sideOffset={4}
          >
            <DropdownMenuLabel className="text-muted-foreground text-xs font-normal">
              {me.username}
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem asChild>
              <a href="/">
                <ExternalLink />
                Back to classic UI
              </a>
            </DropdownMenuItem>
            <DropdownMenuItem asChild>
              <Link to="/sandbox">
                <Palette />
                Design system
              </Link>
            </DropdownMenuItem>
            <DropdownMenuSub>
              <DropdownMenuSubTrigger className="gap-2">
                <SunMoon className="size-4" />
                <span>Theme</span>
                <span className="text-muted-foreground ml-auto text-xs capitalize">
                  {theme ?? "system"}
                </span>
              </DropdownMenuSubTrigger>
              <DropdownMenuPortal>
                <DropdownMenuSubContent className="min-w-36">
                  {THEME_OPTIONS.map(({ value, label, icon: Icon }) => {
                    const active = (theme ?? "system") === value;
                    return (
                      <DropdownMenuItem
                        key={value}
                        onSelect={(e) => {
                          e.preventDefault();
                          setTheme(value);
                        }}
                      >
                        <Icon className="size-4" />
                        <span>{label}</span>
                        {active && (
                          <Check className="text-muted-foreground ml-auto size-3.5" />
                        )}
                      </DropdownMenuItem>
                    );
                  })}
                </DropdownMenuSubContent>
              </DropdownMenuPortal>
            </DropdownMenuSub>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              disabled={logout.isPending}
              onSelect={(e) => {
                e.preventDefault();
                void onLogout();
              }}
            >
              <LogOut />
              {logout.isPending ? "Signing out…" : "Sign out"}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  );
}
