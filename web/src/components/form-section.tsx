import * as React from "react";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { cn } from "@/lib/utils";

interface FormSectionProps extends Omit<React.HTMLAttributes<HTMLDivElement>, "title"> {
  /** Section heading. Sentence case ("Identity", "Tool scope"). */
  title: React.ReactNode;
  /** One-sentence framing for the section. Optional but encouraged. */
  description?: React.ReactNode;
  /** Optional small icon rendered before the title. */
  icon?: React.ReactNode;
  /** Slot rendered on the right of the header — secondary CTAs or links. */
  headerAction?: React.ReactNode;
  /** Form fields and content. Stacked vertically with `gap-4` by default. */
  children: React.ReactNode;
  /** Override the content stack spacing when a section needs to be denser. */
  contentClassName?: string;
}

// FormSection is the canonical "labeled group of form fields" container.
// Companion to <SectionCard> (which is for data lists/sections); this one
// is tuned for forms — larger title, optional description, comfortable
// field rhythm. Pair multiple FormSections vertically inside a width-
// constrained form page (`mx-auto flex max-w-3xl flex-col gap-5`).
//
// Visual contract:
//   `<Card>`
//     `<CardHeader>`
//       icon · `<CardTitle>` (text-base font-semibold)
//       optional `<CardAction>`
//       optional `<CardDescription>` (text-sm muted, spans full width)
//     `<CardContent>` → vertical stack with `gap-4`
//
// Width: the SectionCard family caps page sections at the page wrapper's
// max-width. Form pages typically wrap at `max-w-3xl` (~768px) which is
// the ideal reading column width and matches the rule + tag form pages.
export function FormSection({
  title,
  description,
  icon,
  headerAction,
  children,
  className,
  contentClassName,
  ...rest
}: FormSectionProps) {
  return (
    <Card className={cn("gap-0 py-0", className)} {...rest}>
      <CardHeader
        className={cn(
          "items-center border-b px-5 py-4 !pb-4",
          // When a description is present, the header becomes two rows.
          // Use auto-rows so the title row stays its natural height
          // while the description hugs underneath.
          description && "[&]:grid-rows-[auto_auto]",
        )}
      >
        <CardTitle className="flex items-center gap-2 text-[15px] font-semibold tracking-tight">
          {icon}
          {title}
        </CardTitle>
        {headerAction && (
          <CardAction className="self-center">{headerAction}</CardAction>
        )}
        {description && (
          <CardDescription className="text-muted-foreground row-start-2 text-sm leading-snug">
            {description}
          </CardDescription>
        )}
      </CardHeader>
      <CardContent
        className={cn(
          "flex flex-col gap-4 px-5 py-5",
          contentClassName,
        )}
      >
        {children}
      </CardContent>
    </Card>
  );
}
