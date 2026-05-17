import * as React from "react";
import { cn } from "@/lib/utils";

interface SettingsSectionHeaderProps {
  /**
   * Visual weight. `section` is the top-of-pane title (one per settings tab —
   * Account / Household / Backups). `sub` is a grouping rule used inside a
   * pane to delimit a logical block (Change password / Actions / Stored
   * backups / Automatic schedule).
   *
   * Defaults to `section` because the most common use is the lead-in
   * paragraph for a tab body.
   */
  level?: "section" | "sub";
  title: string;
  description?: React.ReactNode;
  /**
   * Right-aligned actions — usually a single `<Button size="sm">` (Add
   * member, New key, etc.) or a cluster.
   */
  action?: React.ReactNode;
  className?: string;
}

/**
 * `<SettingsSectionHeader>` is the canonical title + description block used
 * by every section inside the Settings shell (`AccountSection`,
 * `HouseholdSection`, `BackupsSection`). Both top-of-pane titles ("Account",
 * "Household", "Backups") and inline sub-sections ("Change password",
 * "Actions", "Stored backups", "Automatic schedule") route through here, so
 * the typographic rhythm — heading size + weight, description colour +
 * line-height, action alignment — stays in one place.
 *
 * Vocabulary tokens:
 *   - `section` → `<h2>` `text-lg font-medium`, baseline-aligned action,
 *     `space-y-1` description.
 *   - `sub`     → `<h3>` `text-sm font-medium`, baseline-aligned action,
 *     `space-y-1` description.
 *
 * Don't fork the look — extend the primitive. If a fourth weight is needed
 * (e.g. a "field group" rhythm tighter than `sub`), add it as a new token
 * here rather than open-coding another `<h4>` block in a feature file.
 */
export function SettingsSectionHeader({
  level = "section",
  title,
  description,
  action,
  className,
}: SettingsSectionHeaderProps) {
  const Heading = level === "section" ? "h2" : "h3";
  const headingClass =
    level === "section"
      ? "text-lg font-medium tracking-tight"
      : "text-sm font-medium";

  return (
    <div
      className={cn(
        "flex flex-col items-start gap-2 sm:flex-row sm:items-end sm:justify-between sm:gap-4",
        className,
      )}
    >
      <div className="min-w-0 space-y-1">
        <Heading className={headingClass}>{title}</Heading>
        {description && (
          <p className="text-muted-foreground max-w-prose text-sm">
            {description}
          </p>
        )}
      </div>
      {action && (
        <div className="flex shrink-0 items-center gap-2">{action}</div>
      )}
    </div>
  );
}
