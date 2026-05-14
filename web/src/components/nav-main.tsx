import { Link, useNavigate, useRouterState } from "@tanstack/react-router";
import {
  SidebarGroup,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar";
import { isNavMatch, navKey, type NavGroup, type NavLeaf } from "@/lib/nav";
import { openModalSearch, useActiveModal } from "@/lib/modals";

export function NavMain({ group }: { group: NavGroup }) {
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const navigate = useNavigate();
  const { key: activeModal } = useActiveModal();

  return (
    <SidebarGroup>
      <SidebarGroupLabel>{group.label}</SidebarGroupLabel>
      <SidebarMenu>
        {group.items.map((item) => (
          <NavRow
            key={navKey(item)}
            item={item}
            pathname={pathname}
            activeModal={activeModal}
            onOpenModal={(modalKey) =>
              navigate({
                to: ".",
                search: (prev: Record<string, unknown>) =>
                  openModalSearch(prev, modalKey),
              })
            }
          />
        ))}
      </SidebarMenu>
    </SidebarGroup>
  );
}

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
      <SidebarMenuItem>
        <SidebarMenuButton
          isActive={activeModal === item.modalKey}
          tooltip={item.title}
          onClick={() => onOpenModal(item.modalKey)}
        >
          <Icon />
          <span>{item.title}</span>
        </SidebarMenuButton>
      </SidebarMenuItem>
    );
  }

  return (
    <SidebarMenuItem>
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
}
