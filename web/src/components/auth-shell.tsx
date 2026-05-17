import * as React from "react";
import { Link } from "@tanstack/react-router";
import {
  Box,
  Lock,
  RadioTower,
  Sparkles,
} from "lucide-react";
import { cn } from "@/lib/utils";

// Two-pane shell for unauthenticated pages (login + setup-account). Left
// pane is a brand panel that echoes the in-app sidebar so signing in feels
// like stepping into the same product; collapses below `lg`. Right pane
// carries the page-specific form (`eyebrow` + `title` + `description`
// echo PageHeader's vocabulary).
//
// Keep this primitive simple — layout + brand, not state. Loading /
// success / error belong in the consumer.

export interface AuthShellProps {
  eyebrow?: React.ReactNode;
  title: React.ReactNode;
  description?: React.ReactNode;
  children: React.ReactNode;
  /** Optional node rendered below the card (legal copy, footnote). */
  footer?: React.ReactNode;
  /** Optional right-aligned link/badge at the top-right of the form pane. */
  topRight?: React.ReactNode;
}

export function AuthShell({
  eyebrow,
  title,
  description,
  children,
  footer,
  topRight,
}: AuthShellProps) {
  return (
    <div className="bg-background min-h-screen">
      <div className="grid min-h-screen lg:grid-cols-2">
        <BrandPane />
        <div className="relative flex flex-col">
          <header className="flex items-center justify-between px-6 pt-6 sm:px-10 lg:hidden">
            <BrandLockup compact />
            {topRight ? (
              <div className="text-muted-foreground text-xs">{topRight}</div>
            ) : null}
          </header>
          <header className="hidden items-center justify-end px-10 pt-8 lg:flex">
            {topRight ? (
              <div className="text-muted-foreground text-xs">{topRight}</div>
            ) : null}
          </header>
          <main className="flex flex-1 items-center justify-center px-6 py-10 sm:px-10">
            <div className="w-full max-w-sm">
              <div className="mb-7 flex flex-col gap-2">
                {eyebrow ? (
                  <span className="text-muted-foreground inline-flex items-center gap-2 text-[11px] font-medium tracking-wider uppercase">
                    {eyebrow}
                  </span>
                ) : null}
                <h1 className="text-foreground text-2xl font-semibold tracking-tight">
                  {title}
                </h1>
                {description ? (
                  <p className="text-muted-foreground text-sm leading-relaxed">
                    {description}
                  </p>
                ) : null}
              </div>
              {children}
              {footer ? (
                <div className="text-muted-foreground mt-6 text-center text-xs">
                  {footer}
                </div>
              ) : null}
            </div>
          </main>
          <footer className="text-muted-foreground/80 hidden items-center justify-between px-10 pb-6 text-[11px] lg:flex">
            <span>© Breadbox</span>
            <span className="inline-flex items-center gap-3">
              <Link to="/" className="hover:text-foreground transition-colors">
                breadbox.sh
              </Link>
              <span aria-hidden>·</span>
              <a
                href="https://docs.breadbox.sh"
                className="hover:text-foreground transition-colors"
              >
                Docs
              </a>
            </span>
          </footer>
        </div>
      </div>
    </div>
  );
}

function BrandPane() {
  return (
    <aside
      className={cn(
        "bg-sidebar text-sidebar-foreground relative hidden flex-col justify-between overflow-hidden p-10 lg:flex",
        "border-sidebar-border border-r",
      )}
    >
      {/* Decorative dot grid (mask fades to the edges) */}
      <div
        aria-hidden
        className="pointer-events-none absolute inset-0 opacity-[0.35]"
        style={{
          backgroundImage:
            "radial-gradient(circle at 1px 1px, var(--sidebar-border) 1px, transparent 0)",
          backgroundSize: "22px 22px",
          maskImage:
            "radial-gradient(ellipse 80% 80% at 30% 40%, black 30%, transparent 80%)",
        }}
      />
      {/* Soft glow blob */}
      <div
        aria-hidden
        className="bg-primary/[0.08] pointer-events-none absolute -top-24 -left-24 size-[26rem] rounded-full blur-3xl"
      />

      <div className="relative">
        <BrandLockup />
      </div>

      <div className="relative flex flex-col gap-8">
        <div className="flex flex-col gap-3">
          <span className="text-muted-foreground inline-flex w-fit items-center gap-1.5 text-[11px] font-medium tracking-wider uppercase">
            <Sparkles className="size-3" />
            Self-hosted finance
          </span>
          <h2 className="text-foreground max-w-md text-2xl leading-tight font-semibold tracking-tight sm:text-[1.65rem]">
            Your household's financial data, in one place you actually control.
          </h2>
          <p className="text-muted-foreground max-w-md text-sm leading-relaxed">
            Breadbox syncs accounts from Plaid, Teller, and CSV imports into a
            Postgres database you own — then exposes it to AI agents over MCP.
          </p>
        </div>
        <ul className="flex flex-col gap-2.5">
          <FeaturePill icon={Lock} label="End-to-end encrypted tokens at rest" />
          <FeaturePill icon={RadioTower} label="MCP-native — works with Claude, agents, scripts" />
          <FeaturePill icon={Box} label="One Go binary, one Postgres, zero SaaS" />
        </ul>
      </div>

      <div className="text-muted-foreground relative flex items-center justify-between text-[11px]">
        <span>v2 Preview</span>
        <span>breadbox.sh</span>
      </div>
    </aside>
  );
}

function FeaturePill({
  icon: Icon,
  label,
}: {
  icon: React.ComponentType<{ className?: string }>;
  label: string;
}) {
  return (
    <li className="border-sidebar-border/60 bg-sidebar-accent/40 inline-flex items-center gap-2.5 rounded-md border px-3 py-1.5 text-xs">
      <span className="bg-primary/12 text-primary flex size-5 shrink-0 items-center justify-center rounded-[5px]">
        <Icon className="size-3" />
      </span>
      <span className="text-foreground/85">{label}</span>
    </li>
  );
}

function BrandLockup({ compact = false }: { compact?: boolean }) {
  return (
    <Link
      to="/"
      className="group inline-flex items-center gap-2.5 outline-none"
      aria-label="Breadbox home"
    >
      <span
        className={cn(
          "bg-primary text-primary-foreground flex aspect-square items-center justify-center rounded-md shadow-sm transition-transform group-hover:scale-[1.03]",
          compact ? "size-7" : "size-9",
        )}
      >
        <Box className={compact ? "size-3.5" : "size-[18px]"} />
      </span>
      <span className="flex flex-col leading-none">
        <span
          className={cn(
            "text-foreground font-semibold tracking-tight",
            compact ? "text-sm" : "text-base",
          )}
        >
          Breadbox
        </span>
        <span className="text-muted-foreground mt-1 inline-flex items-center gap-1.5 text-[10px] font-medium tracking-wider uppercase">
          <span className="bg-primary/15 text-primary rounded-sm px-1 py-px text-[10px] font-semibold">
            v2
          </span>
          Preview
        </span>
      </span>
    </Link>
  );
}
