import { Link, useNavigate, useRouterState } from "@tanstack/react-router";
import {
  SidebarGroup,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar";
import { Eyebrow } from "@/components/eyebrow";
import { isNavMatch, navKey, type NavGroup, type NavLeaf } from "@/lib/nav";
import { openModal, useActiveModal } from "@/lib/modals";
import { cn } from "@/lib/utils";

export function NavMain({ group }: { group: NavGroup }) {
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const navigate = useNavigate();
  const { key: activeModal } = useActiveModal();

  return (
    <SidebarGroup>
      <SidebarGroupLabel asChild>
        <Eyebrow variant="nav" as="div" className="text-muted-foreground/80">
          {group.label}
        </Eyebrow>
      </SidebarGroupLabel>
      <SidebarMenu>
        {group.items.map((item) => (
          <NavRow
            key={navKey(item)}
            item={item}
            pathname={pathname}
            activeModal={activeModal}
            onOpenModal={(modalKey) =>
              navigate({ to: ".", search: openModal(modalKey) })
            }
          />
        ))}
      </SidebarMenu>
    </SidebarGroup>
  );
}

// Shared classes for the active-state polish. The shadcn
// SidebarMenuButton is `overflow-hidden`, which would clip a pseudo
// element drawn on the button itself — so the left rail lives on the
// SidebarMenuItem wrapper (which is `relative` and has no overflow
// clipping) and uses the button's `data-[active=true]` selector via
// `:has` so the row stays declarative.
const NAV_ITEM_CLS = cn(
  // Active rail: a primary-tinted chip that animates in next to the
  // active row. Hidden when the sidebar collapses to icon-only.
  "before:bg-primary before:absolute before:top-1.5 before:bottom-1.5 before:-left-2 before:w-[3px] before:scale-y-0 before:rounded-r-full before:transition-transform before:duration-200 before:ease-out",
  "has-[[data-active=true]]:before:scale-y-100",
  "group-data-[collapsible=icon]:before:hidden",
);

const NAV_ROW_CLS = cn(
  "transition-colors",
  // Icon picks up the primary tint on active so the row reads at a glance
  // even when the eye skips the rail. Inactive icons stay muted so the
  // active one carries the visual weight.
  "[&>svg]:text-muted-foreground/80 [&>svg]:transition-colors",
  "data-[active=true]:[&>svg]:text-primary",
);

interface NavRowProps {
  item: NavLeaf;
  pathname: string;
  activeModal: string | null;
  onOpenModal: (modalKey: string) => void;
}

function NavRow({ item, pathname, activeModal, onOpenModal }: NavRowProps) {
  const Icon = item.icon;

  if (item.kind === "modal") {
    return (
      <SidebarMenuItem className={NAV_ITEM_CLS}>
        <SidebarMenuButton
          isActive={activeModal === item.modalKey}
          tooltip={item.title}
          onClick={() => onOpenModal(item.modalKey)}
          className={NAV_ROW_CLS}
        >
          <Icon />
          <span>{item.title}</span>
        </SidebarMenuButton>
      </SidebarMenuItem>
    );
  }

  return (
    <SidebarMenuItem className={NAV_ITEM_CLS}>
      <SidebarMenuButton
        asChild
        isActive={isNavMatch(item, pathname)}
        tooltip={item.title}
        className={NAV_ROW_CLS}
      >
        <Link to={item.to}>
          <Icon />
          <span>{item.title}</span>
        </Link>
      </SidebarMenuButton>
    </SidebarMenuItem>
  );
}
