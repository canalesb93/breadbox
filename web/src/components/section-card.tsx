import * as React from "react";
import {
  Card,
  CardAction,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { cn } from "@/lib/utils";

interface SectionCardProps extends React.HTMLAttributes<HTMLDivElement> {
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
//     `<CardHeader className="border-b px-5 py-3.5">` title (text-sm font-medium)
//     `<CardContent className="px-5 py-5">` content (or px-0 py-0 when flushBody)
//     optional `<div className="border-t px-5 py-3 text-right">` footer
//
// Don't fork the look — change this primitive.
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
      <CardHeader className={cn("border-b px-5 py-3.5", headerClassName)}>
        <CardTitle className="flex items-center gap-2 text-sm font-medium">
          {icon}
          {title}
        </CardTitle>
        {action && <CardAction>{action}</CardAction>}
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
