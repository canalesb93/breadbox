import {
  Check,
  ChevronsUpDown,
  ExternalLink,
  Keyboard,
  Loader2,
  LogOut,
  type LucideIcon,
  MessageSquareText,
  Monitor,
  Moon,
  Palette,
  Shield,
  Sun,
  User as UserIcon,
} from "lucide-react";
import { Link, useNavigate } from "@tanstack/react-router";
import { toast } from "sonner";
import { useTheme } from "next-themes";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
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
  SidebarMenuSkeleton,
  useSidebar,
} from "@/components/ui/sidebar";
import type { Me } from "@/api/types";
import { useLogout } from "@/api/queries/auth";
import { cn } from "@/lib/utils";

// Lifts the email local-part (before "@") into an avatar pair. Falls back to
// "?" so an empty username never produces a blank tile.
function initials(name: string) {
  const local = name.split("@")[0];
  const parts = local.split(/[._-]/).filter(Boolean);
  return (parts[0]?.[0] ?? "?").concat(parts[1]?.[0] ?? "").toUpperCase();
}

// Splits an email-shaped username into local + domain. Real installs may
// already use a display name (no `@`) in which case we render it whole.
function splitUsername(username: string) {
  const at = username.indexOf("@");
  if (at < 0) return { primary: username, secondary: null as string | null };
  return {
    primary: username.slice(0, at),
    secondary: username.slice(at),
  };
}

// Role tone keys the same vocabulary as the rest of the v2 SPA: admin is the
// elevated "primary" tinted pill (matches BrandHeader's v2 chip), editor reads
// as a normal authenticated user (muted), viewer is a lighter-weight read-only
// signal (muted with reduced opacity).
const ROLE_TONE: Record<string, string> = {
  admin: "bg-primary/15 text-primary",
  editor: "bg-muted text-foreground/80",
  viewer: "bg-muted text-muted-foreground",
};

function RoleBadge({ role }: { role: string }) {
  const tone = ROLE_TONE[role] ?? "bg-muted text-muted-foreground";
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded-sm px-1.5 py-px text-[10px] font-semibold tracking-wider uppercase",
        tone,
      )}
    >
      {role === "admin" ? <Shield className="size-2.5" /> : null}
      {role}
    </span>
  );
}

// THEMES is the source of truth for the theme submenu — keeping it as a
// const tuple lets us drive the active-state radio rendering off `next-themes`
// without duplicating the label/icon mapping across components. Order matches
// the canonical shadcn theme-toggle examples.
const THEMES = [
  { value: "system", label: "System", icon: Monitor },
  { value: "light", label: "Light", icon: Sun },
  { value: "dark", label: "Dark", icon: Moon },
] as const satisfies ReadonlyArray<{
  value: string;
  label: string;
  icon: LucideIcon;
}>;

function ThemeSubmenu() {
  // `useTheme` reads from the `ThemeProvider` mounted in `main.tsx`. `theme`
  // is the *requested* setting (system / light / dark); `resolvedTheme` is
  // the realised mode. We key the active row off `theme` so "System" stays
  // selected even when system resolves to dark — the menu reads "what did
  // you pick", not "what's currently rendered".
  const { theme, setTheme } = useTheme();
  return (
    <DropdownMenuSub>
      <DropdownMenuSubTrigger className="gap-2 [&>svg:last-child]:ml-0">
        <Palette className="text-muted-foreground" />
        <span>Theme</span>
        <span className="text-muted-foreground ml-auto text-[11px] capitalize">
          {theme ?? "system"}
        </span>
      </DropdownMenuSubTrigger>
      <DropdownMenuSubContent className="min-w-40">
        {THEMES.map(({ value, label, icon: Icon }) => {
          const active = (theme ?? "system") === value;
          return (
            <DropdownMenuItem
              key={value}
              onSelect={(e) => {
                e.preventDefault();
                setTheme(value);
              }}
              className="gap-2"
            >
              <Icon className="text-muted-foreground" />
              <span>{label}</span>
              {active ? (
                <Check className="text-primary ml-auto size-3.5" />
              ) : null}
            </DropdownMenuItem>
          );
        })}
      </DropdownMenuSubContent>
    </DropdownMenuSub>
  );
}

export function NavUser({ me }: { me: Me | null }) {
  const { isMobile } = useSidebar();
  const navigate = useNavigate();
  const logout = useLogout();

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

  // Loading state: show a sidebar-matching skeleton instead of the placeholder
  // text the previous version rendered. Reads as "loading", not "broken".
  if (!me) {
    return (
      <SidebarMenu>
        <SidebarMenuItem>
          <SidebarMenuSkeleton showIcon />
        </SidebarMenuItem>
      </SidebarMenu>
    );
  }

  const { primary, secondary } = splitUsername(me.username);

  // Open keyboard-shortcuts overlay via the shared event bus (iter 17).
  const openShortcuts = () => {
    window.dispatchEvent(new CustomEvent("breadbox:shortcut-sheet:open"));
  };

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <SidebarMenuButton
              size="lg"
              tooltip={`${primary} · ${me.role}`}
              className="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground group/nav-user h-12 gap-2.5"
            >
              <Avatar className="ring-border/60 size-8 rounded-md ring-1">
                <AvatarFallback className="bg-primary/10 text-primary rounded-md text-[11px] font-semibold tracking-tight">
                  {initials(me.username)}
                </AvatarFallback>
              </Avatar>
              <div className="grid flex-1 text-left leading-tight">
                <span className="truncate text-sm font-medium">{primary}</span>
                <span className="text-muted-foreground inline-flex items-center gap-1.5 truncate text-[11px]">
                  <RoleBadge role={me.role} />
                  {secondary ? (
                    <span className="truncate">{secondary}</span>
                  ) : (
                    <span>Signed in</span>
                  )}
                </span>
              </div>
              <ChevronsUpDown className="text-muted-foreground/70 group-hover/nav-user:text-foreground ml-auto size-3.5 transition-colors" />
            </SidebarMenuButton>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            className="w-(--radix-dropdown-menu-trigger-width) min-w-60 rounded-lg"
            side={isMobile ? "bottom" : "right"}
            align="end"
            sideOffset={8}
          >
            <DropdownMenuLabel className="p-0">
              <div className="flex items-center gap-3 px-2 py-2">
                <Avatar className="ring-border/60 size-9 rounded-md ring-1">
                  <AvatarFallback className="bg-primary/10 text-primary rounded-md text-[12px] font-semibold">
                    {initials(me.username)}
                  </AvatarFallback>
                </Avatar>
                <div className="grid flex-1 leading-tight">
                  <span className="truncate text-sm font-medium">
                    {primary}
                    {secondary ? (
                      <span className="text-muted-foreground font-normal">
                        {secondary}
                      </span>
                    ) : null}
                  </span>
                  <span className="text-muted-foreground inline-flex items-center gap-1.5 truncate text-[11px]">
                    <RoleBadge role={me.role} />
                    <span>Household</span>
                  </span>
                </div>
              </div>
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuGroup>
              <ThemeSubmenu />
              <DropdownMenuItem
                onSelect={(e) => {
                  e.preventDefault();
                  openShortcuts();
                }}
                className="gap-2"
              >
                <Keyboard className="text-muted-foreground" />
                <span>Keyboard shortcuts</span>
                <span className="text-muted-foreground ml-auto inline-flex items-center gap-0.5">
                  <kbd className="border-border/60 bg-muted/60 rounded border px-1 font-mono text-[10px] leading-none">
                    ?
                  </kbd>
                </span>
              </DropdownMenuItem>
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <DropdownMenuGroup>
              <DropdownMenuItem asChild className="gap-2">
                <Link to="/sandbox">
                  <Palette className="text-muted-foreground" />
                  Design system
                </Link>
              </DropdownMenuItem>
              <DropdownMenuItem asChild className="gap-2">
                <a
                  href="https://github.com/canalesb93/breadbox/issues/new/choose"
                  target="_blank"
                  rel="noreferrer"
                >
                  <MessageSquareText className="text-muted-foreground" />
                  <span>Send feedback</span>
                  <ExternalLink className="text-muted-foreground/60 ml-auto size-3" />
                </a>
              </DropdownMenuItem>
              <DropdownMenuItem asChild className="gap-2">
                <a href="/">
                  <UserIcon className="text-muted-foreground" />
                  <span>Classic admin UI</span>
                  <ExternalLink className="text-muted-foreground/60 ml-auto size-3" />
                </a>
              </DropdownMenuItem>
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              disabled={logout.isPending}
              onSelect={(e) => {
                e.preventDefault();
                void onLogout();
              }}
              className="text-destructive focus:text-destructive focus:bg-destructive/10 gap-2"
            >
              {logout.isPending ? (
                <Loader2 className="animate-spin" />
              ) : (
                <LogOut />
              )}
              {logout.isPending ? "Signing out…" : "Sign out"}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  );
}
