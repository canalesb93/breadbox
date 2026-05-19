import { useNavigate } from "@tanstack/react-router";
import { Hammer } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { cn } from "@/lib/utils";
import { SETTINGS_SECTIONS, type SettingsSection } from "@/lib/settings-sections";
import { useMediaQuery } from "@/hooks/use-media-query";
import { closeModal, openModal, useActiveModal } from "@/lib/modals";
import { AccountSection } from "@/features/settings/account-section";
import { AgentsSection } from "@/features/settings/agents-section";
import { BackupsSection } from "@/features/settings/backups-section";
import { HouseholdSection } from "@/features/settings/household-section";
import { SettingsSectionHeader } from "@/components/settings-section-header";
import { StatusPanel } from "@/components/status-panel";
import { ComingSoonPill } from "@/components/coming-soon-pill";
import { Eyebrow } from "@/components/eyebrow";

const SETTINGS_MODAL_KEY = "settings";

function pickSection(slug: string | null): SettingsSection {
  return SETTINGS_SECTIONS.find((s) => s.slug === slug) ?? SETTINGS_SECTIONS[0];
}

export function SettingsShell() {
  const navigate = useNavigate();
  const isDesktop = useMediaQuery("(min-width: 768px)");
  const { key: activeModal, section } = useActiveModal();

  const open = activeModal === SETTINGS_MODAL_KEY;
  const active = pickSection(section);

  const onOpenChange = (next: boolean) => {
    if (next) return;
    navigate({ to: ".", search: closeModal() });
  };

  const onSelect = (slug: string) => {
    navigate({ to: ".", search: openModal(SETTINGS_MODAL_KEY, slug) });
  };

  const body = (
    <SettingsBody active={active} onSelect={onSelect} desktop={isDesktop} />
  );

  if (isDesktop) {
    return (
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="flex h-[min(720px,85vh)] max-w-4xl flex-col gap-0 overflow-hidden p-0 sm:max-w-4xl">
          <DialogHeader className="sr-only">
            <DialogTitle>Settings</DialogTitle>
            <DialogDescription>Manage your account and app preferences.</DialogDescription>
          </DialogHeader>
          {body}
        </DialogContent>
      </Dialog>
    );
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="bottom"
        className="flex h-[92dvh] flex-col gap-0 p-0"
      >
        <SheetHeader className="border-b px-4 pt-5 pb-3">
          <SheetTitle className="text-base">Settings</SheetTitle>
          <SheetDescription className="text-xs">
            Manage your account and app preferences.
          </SheetDescription>
        </SheetHeader>
        {body}
      </SheetContent>
    </Sheet>
  );
}

interface SettingsBodyProps {
  active: SettingsSection;
  onSelect: (slug: string) => void;
  desktop: boolean;
}

function SettingsBody({ active, onSelect, desktop }: SettingsBodyProps) {
  if (desktop) {
    return (
      <div className="flex min-h-0 flex-1">
        <nav className="bg-sidebar text-sidebar-foreground w-56 shrink-0 border-r p-3">
          <Eyebrow
            as="p"
            variant="nav"
            className="text-muted-foreground/80 mb-2 px-2"
          >
            Settings
          </Eyebrow>
          <ul className="space-y-0.5">
            {SETTINGS_SECTIONS.map((s) => {
              const Icon = s.icon;
              const isActive = s.slug === active.slug;
              return (
                <li key={s.slug}>
                  <button
                    type="button"
                    onClick={() => onSelect(s.slug)}
                    data-active={isActive ? "true" : undefined}
                    className={cn(
                      "group relative flex w-full items-center gap-2 rounded-md px-2.5 py-2 text-left text-sm transition-colors",
                      // 3px primary-tinted rail at the outer panel edge, scaled
                      // in via transform on activation. Matches the canonical
                      // nav-main vocabulary (iter 1, timing parity iter 73):
                      // before:transition-transform before:duration-200 before:ease-out.
                      "before:bg-primary before:absolute before:top-1.5 before:bottom-1.5 before:-left-3 before:w-[3px] before:scale-y-0 before:rounded-r-full before:transition-transform before:duration-200 before:ease-out",
                      "data-[active=true]:before:scale-y-100",
                      // Shared focus-visible vocabulary (matches Button +
                      // SidebarMenuButton) so keyboard users can see which
                      // section is focused before pressing Enter.
                      "focus-visible:ring-ring/50 focus-visible:ring-[3px] focus-visible:outline-none",
                      // Icon picks up the primary tint on active so the row
                      // reads at a glance — matches nav-main's NAV_ROW_CLS.
                      "[&>svg]:text-muted-foreground/80 [&>svg]:size-4 [&>svg]:transition-colors",
                      "data-[active=true]:[&>svg]:text-primary",
                      isActive
                        ? "bg-sidebar-accent text-sidebar-accent-foreground"
                        : "hover:bg-sidebar-accent/60 hover:text-sidebar-accent-foreground",
                    )}
                  >
                    <Icon />
                    <span>{s.title}</span>
                  </button>
                </li>
              );
            })}
          </ul>
        </nav>
        <section className="flex-1 overflow-y-auto overscroll-contain [-webkit-overflow-scrolling:touch] p-6">
          <SectionContent section={active} />
        </section>
      </div>
    );
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <nav
        aria-label="Settings sections"
        className="bg-background/95 supports-[backdrop-filter]:bg-background/80 sticky top-0 z-10 -mx-px border-b px-2 backdrop-blur"
      >
        <ul
          // No `w-max`: the ul is parent-width so its `overflow-x-auto` actually
          // has a scroll context. With `w-max` the ul itself overflowed its
          // parent `<nav>` (which has no overflow), so the pill strip rendered
          // past the viewport edge but couldn't be scrolled. Now the
          // `shrink-0` pills inside push horizontally, and `overflow-x-auto`
          // routes the gesture correctly. Scrollbar is hidden per the existing
          // `scrollbar-width:none` / webkit-scrollbar rules — affordance comes
          // from the always-truncated rightmost pill, matching the rest of the
          // mobile chip rails.
          className="-mx-2 flex flex-nowrap items-center gap-1 overflow-x-auto px-2 py-2 whitespace-nowrap [-ms-overflow-style:none] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
        >
          {SETTINGS_SECTIONS.map((s) => {
            const Icon = s.icon;
            const isActive = s.slug === active.slug;
            return (
              <li key={s.slug}>
                <button
                  type="button"
                  onClick={() => onSelect(s.slug)}
                  data-active={isActive ? "true" : undefined}
                  className={cn(
                    "group flex h-8 shrink-0 items-center gap-1.5 rounded-full border px-3 text-xs font-medium transition-colors",
                    // Shared focus-visible vocabulary so keyboard users
                    // tabbing through the mobile pill strip can see which
                    // section is selected.
                    "focus-visible:ring-ring/50 focus-visible:ring-[3px] focus-visible:outline-none",
                    "[&>svg]:size-3.5",
                    isActive
                      ? "border-primary/30 bg-primary/10 text-primary [&>svg]:text-primary"
                      : "border-border bg-card text-muted-foreground hover:bg-accent/60 hover:text-foreground [&>svg]:text-muted-foreground",
                  )}
                >
                  <Icon />
                  <span>{s.title}</span>
                </button>
              </li>
            );
          })}
        </ul>
      </nav>
      <section className="flex-1 overflow-y-auto overscroll-contain [-webkit-overflow-scrolling:touch] p-4">
        <SectionContent section={active} />
      </section>
    </div>
  );
}

function SectionContent({ section }: { section: SettingsSection }) {
  if (section.slug === "account") {
    return <AccountSection />;
  }

  if (section.slug === "household") {
    return <HouseholdSection />;
  }

  if (section.slug === "backups") {
    return <BackupsSection />;
  }

  if (section.slug === "agents") {
    return <AgentsSection />;
  }

  // Fallback for sections defined in `lib/settings-sections.ts` that don't
  // yet have a real implementation (today: Security). Mirrors the canonical
  // "Coming soon" vocabulary from `routes/placeholder.tsx` — a
  // `<StatusPanel tone="info">` with a pill in the trailing slot — so the
  // settings modal speaks the same language as the rest of the SPA's
  // unbuilt-nav-leaf surfaces instead of a naked muted paragraph.
  return (
    <div className="space-y-4">
      <SettingsSectionHeader
        title={section.title}
        description={section.description}
      />
      <StatusPanel
        tone="info"
        icon={Hammer}
        heading={`${section.title} settings are in the works`}
        body="We're still building this surface. The configuration that lives here will land in a follow-up PR — for now, the panel is wired but empty."
        trailing={<ComingSoonPill />}
      />
    </div>
  );
}
