import { useNavigate } from "@tanstack/react-router";
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
import { BackupsSection } from "@/features/settings/backups-section";
import { HouseholdSection } from "@/features/settings/household-section";

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
        <DialogContent className="h-[600px] max-w-3xl gap-0 overflow-hidden p-0 sm:max-w-3xl">
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
      <SheetContent side="bottom" className="h-[92dvh] p-0">
        <SheetHeader>
          <SheetTitle>Settings</SheetTitle>
          <SheetDescription>Manage your account and app preferences.</SheetDescription>
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
      <div className="flex h-full">
        <nav className="bg-muted/40 w-56 shrink-0 border-r p-3">
          <p className="text-muted-foreground/80 mb-2 px-2 text-[10px] font-semibold tracking-[0.08em] uppercase">
            Settings
          </p>
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
                      "group relative flex w-full items-center gap-2 rounded-md py-2 pr-3 pl-3.5 text-left text-sm transition-colors",
                      "before:absolute before:inset-y-1.5 before:left-0 before:w-0.5 before:rounded-r-full before:bg-transparent before:transition-all",
                      "[&>svg]:size-4 [&>svg]:text-muted-foreground",
                      isActive
                        ? "bg-accent text-accent-foreground before:bg-primary before:inset-y-1 [&>svg]:text-primary"
                        : "hover:bg-accent/60",
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
        <section className="flex-1 overflow-y-auto p-6">
          <SectionContent section={active} />
        </section>
      </div>
    );
  }

  return (
    <div className="space-y-6 overflow-y-auto p-4">
      {SETTINGS_SECTIONS.map((s) => (
        <SectionContent key={s.slug} section={s} mobile />
      ))}
    </div>
  );
}

function SectionContent({
  section,
  mobile = false,
}: {
  section: SettingsSection;
  mobile?: boolean;
}) {
  const wrapper = mobile
    ? "border-border border-t pt-4 first:border-t-0 first:pt-0"
    : "";

  if (section.slug === "account") {
    return (
      <div className={wrapper}>
        <AccountSection />
      </div>
    );
  }

  if (section.slug === "household") {
    return (
      <div className={wrapper}>
        <HouseholdSection />
      </div>
    );
  }

  if (section.slug === "backups") {
    return (
      <div className={wrapper}>
        <BackupsSection />
      </div>
    );
  }

  return (
    <div className={cn(wrapper, mobile ? "space-y-2" : "space-y-1")}>
      <h2 className="text-lg font-medium">{section.title}</h2>
      <p className="text-muted-foreground text-sm">{section.description}</p>
      <p className="text-muted-foreground mt-4 text-sm">Coming soon.</p>
    </div>
  );
}
