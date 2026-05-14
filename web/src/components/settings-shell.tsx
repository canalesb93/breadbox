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
import { closeModalSearch, openModalSearch, useActiveModal } from "@/lib/modals";

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
    navigate({
      to: ".",
      search: (prev) => closeModalSearch(prev as Record<string, unknown>),
    });
  };

  const onSelect = (slug: string) => {
    navigate({
      to: ".",
      search: (prev) =>
        openModalSearch(prev as Record<string, unknown>, SETTINGS_MODAL_KEY, slug),
    });
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
        <nav className="bg-muted/40 w-56 shrink-0 border-r p-2">
          <ul className="space-y-1">
            {SETTINGS_SECTIONS.map((s) => {
              const Icon = s.icon;
              const isActive = s.slug === active.slug;
              return (
                <li key={s.slug}>
                  <button
                    type="button"
                    onClick={() => onSelect(s.slug)}
                    className={cn(
                      "flex w-full items-center gap-2 rounded-md px-3 py-2 text-left text-sm",
                      isActive
                        ? "bg-accent text-accent-foreground"
                        : "hover:bg-accent/50",
                    )}
                  >
                    <Icon className="size-4" />
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
  return (
    <div className={cn(mobile ? "border-border space-y-2 border-t pt-4 first:border-t-0 first:pt-0" : "space-y-1")}>
      <h2 className="text-lg font-medium">{section.title}</h2>
      <p className="text-muted-foreground text-sm">{section.description}</p>
      <p className="text-muted-foreground mt-4 text-sm">Coming soon.</p>
    </div>
  );
}
