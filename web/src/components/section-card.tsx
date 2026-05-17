import * as React from "react";
import {
  Card,
  CardAction,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { cn } from "@/lib/utils";

interface SectionCardProps extends Omit<React.HTMLAttributes<HTMLDivElement>, "title"> {
  title: React.ReactNode;
  // Slot rendered on the right of the bordered header (button, badge, etc).
  action?: React.ReactNode;
  // Optional small icon rendered before the title.
  icon?: React.ReactNode;
  // When true, the body becomes flush (px-0 py-0) so the consumer can drop
  // a `<ul className="divide-y">` straight in. Default leaves comfortable
  // padding for prose / forms.
  flushBody?: boolean;
  // Optional footer slot rendered with a top border. Use for "See all" links.
  footer?: React.ReactNode;
  children: React.ReactNode;
  cardClassName?: string;
  headerClassName?: string;
  bodyClassName?: string;
  footerClassName?: string;
}

// SectionCard is the canonical "page section in a card" container — bordered
// header that names the section, optional trailing action, body that hosts
// the content. Established across the v2 design pass on Home (recent
// activity / connections), Transaction-detail (Activity / Details), and now
// Account-detail (Settings / Links / Recent transactions / Details). Wraps
// shadcn `Card` so every surface speaks the same vocabulary.
//
// Visual contract:
//   `<Card className="gap-0 py-0">`
//     `<CardHeader className="border-b px-5 py-4 items-center">` title (text-sm font-semibold)
//     `<CardContent className="px-5 py-5">` content (or px-0 py-0 when flushBody)
//     optional `<div className="border-t px-5 py-3 text-right">` footer
//
// Vertical rhythm: header `py-4` (16px) + body `py-5` (20px) reads as a
// slight density step from chrome to content. Matched by `ListCard` so the
// two cards visually align when they sit on the same page. Don't fork the
// look — change this primitive.
//
// Header alignment (iter 105): the shadcn `CardHeader` grid defaults to
// `items-start`, which pinned the 20px title to the top of the row while a
// 28-32px action (ViewAllPill / Button size="sm") protruded downward by
// 8-12px. That asymmetry — title's baseline higher than the action's
// vertical center — was the "header looks crooked" complaint. We override
// to `items-center` so the title and action sit on the same vertical
// midline regardless of action height. The title weight bump from
// `font-medium` → `font-semibold` gives it enough anchor that it doesn't
// read as a caption next to the action button.
export function SectionCard({
  title,
  action,
  icon,
  flushBody = false,
  footer,
  children,
  className,
  cardClassName,
  headerClassName,
  bodyClassName,
  footerClassName,
  ...rest
}: SectionCardProps) {
  return (
    <Card
      className={cn("gap-0 py-0", cardClassName, className)}
      {...rest}
    >
      {/* Override the shadcn Card primitive's `[.border-b]:pb-6` rule: when
          a SectionCard header carries the divider the primitive injects an
          extra 24px of bottom padding, which on top of our intentional
          `py-4` produced an empty band before the body. `!pb-4` keeps the
          density we designed for and matches the `pt-4` on top.
          `items-center` overrides the grid's default `items-start` so the
          title sits on the same midline as a taller action button
          (ViewAllPill / Button size="sm") — see header note above. */}
      <CardHeader
        className={cn("items-center border-b px-5 py-4 !pb-4", headerClassName)}
      >
        <CardTitle className="flex items-center gap-2 text-sm font-semibold">
          {icon}
          {title}
        </CardTitle>
        {action && <CardAction className="self-center">{action}</CardAction>}
      </CardHeader>
      <CardContent
        className={cn(
          flushBody ? "px-0 py-0" : "px-5 py-5",
          bodyClassName,
        )}
      >
        {children}
      </CardContent>
      {footer && (
        <div
          className={cn(
            "border-t px-5 py-3 text-right",
            footerClassName,
          )}
        >
          {footer}
        </div>
      )}
    </Card>
  );
}
