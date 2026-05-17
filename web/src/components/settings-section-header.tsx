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
 *   - `section` → `<h2>` `text-lg font-semibold`, action centered to title
 *     row, `space-y-1` description.
 *   - `sub`     → `<h3>` `text-sm font-semibold`, action centered to title
 *     row, `space-y-1` description.
 *
 * Both heading tokens use `font-semibold` to match `PageHeader` and the
 * canonical card-header recipe — the title needs anchor weight next to a
 * real action button instead of reading as a caption.
 *
 * Don't fork the look — extend the primitive. If a fourth weight is needed,
 * add it as a new token here rather than open-coding another `<h4>` block
 * in a feature file.
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
      ? "text-lg font-semibold tracking-tight"
      : "text-sm font-semibold";

  return (
    <div
      className={cn(
        // Iter 107: when an action is present and the title row is the
        // visual anchor, `sm:items-center` keeps the button vertically
        // centered relative to the title+description block instead of
        // bottom-docked under the description (which protruded the button
        // below the title baseline by 8-12px — the same mismatch iter 106
        // fixed on `SectionCard` / `ListCard`). When there's no action the
        // alignment is moot — `flex-col` collapses to a single column.
        "flex flex-col items-start gap-2 sm:flex-row sm:items-center sm:justify-between sm:gap-4",
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
