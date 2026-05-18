import { useMemo } from "react";
import {
  Activity,
  MessageSquare,
  RefreshCw,
  Shapes,
  Tag,
  Wand2,
  type LucideIcon,
} from "lucide-react";
import { EmptyState } from "@/components/empty-state";
import { PageError } from "@/components/page-error";
import {
  TimelineRail,
  type TimelineRailTone,
} from "@/components/timeline-rail";
import { useAnnotations } from "@/api/queries/annotations";
import { formatRelativeTime } from "@/lib/format";
import type { Annotation } from "@/api/types";

const KIND_ICON: Record<string, LucideIcon> = {
  comment: MessageSquare,
  rule_applied: Wand2,
  tag_added: Tag,
  tag_removed: Tag,
  category_set: Shapes,
  sync_started: RefreshCw,
  sync_updated: RefreshCw,
};

// KIND_TONE tints each event's disc semantically so a scanning eye can
// pick out rule fires, sync events, and classification changes without
// reading the summary line. Iter 93. Mapping rationale:
//   - rule_applied / category_set → primary (system-driven /
//     classification edits are the dominant signal on a transaction)
//   - sync_started / sync_updated → info (sky; data arrived from the
//     outside, matches the sync vocabulary on Connections)
//   - tag_added → success (additive change)
//   - tag_removed → warning (amber; something was taken away)
//   - comment → neutral (the body bubble carries its own colour cue)
//   - unmapped kinds → neutral via the default `tone="neutral"` on
//     <TimelineRail.Row>.
const KIND_TONE: Record<string, TimelineRailTone> = {
  comment: "neutral",
  rule_applied: "primary",
  category_set: "primary",
  tag_added: "success",
  tag_removed: "warning",
  sync_started: "info",
  sync_updated: "info",
};

const dayHeadingFormatter = new Intl.DateTimeFormat("en-US", {
  weekday: "long",
  month: "long",
  day: "numeric",
});

function dayLabel(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const today = new Date();
  const yesterday = new Date(today);
  yesterday.setDate(today.getDate() - 1);
  if (d.toDateString() === today.toDateString()) return "Today";
  if (d.toDateString() === yesterday.toDateString()) return "Yesterday";
  return dayHeadingFormatter.format(d);
}

interface DayGroup {
  label: string;
  rows: Annotation[];
}

// groupByDay buckets the (already newest-first) annotation list into calendar
// days, preserving order.
function groupByDay(annotations: Annotation[]): DayGroup[] {
  const groups: DayGroup[] = [];
  for (const row of annotations) {
    const label = dayLabel(row.created_at);
    const last = groups[groups.length - 1];
    if (last && last.label === label) {
      last.rows.push(row);
    } else {
      groups.push({ label, rows: [row] });
    }
  }
  return groups;
}

// ActivityTimeline renders a transaction's enriched annotation feed — comments,
// rule applications, tag/category changes, sync events — grouped by day. The
// server hands back ready-to-render `summary` lines, so this stays a pure
// layout component on top of the shared <TimelineRail> primitive (iter 26).
export function ActivityTimeline({ transactionId }: { transactionId: string }) {
  const { data, isLoading, isError, isFetching, error, refetch } =
    useAnnotations(transactionId);
  const groups = useMemo(() => groupByDay(data ?? []), [data]);

  if (isLoading) {
    // Skeleton mirrors the real TimelineRail geometry — disc punched through
    // the rail line, content lines to the right — so the layout doesn't shift
    // when annotations land. Second row carries a `body` block to suggest the
    // comment bubble that often anchors a recent feed.
    return (
      <TimelineRail>
        <TimelineRail.Group>
          <TimelineRail.RowSkeleton />
          <TimelineRail.RowSkeleton body />
          <TimelineRail.RowSkeleton />
          <TimelineRail.RowSkeleton />
        </TimelineRail.Group>
      </TimelineRail>
    );
  }

  if (isError) {
    // PageError `inline` variant (iter 88) drops the bordered StatusPanel
    // chrome so this error sits flush inside the parent <SectionCard
    // title="Activity"> without doubling up borders. Same destructive icon
    // tile + heading + body + retry vocabulary as every other v2 error
    // surface.
    return (
      <PageError
        variant="inline"
        resource="the activity timeline"
        error={error}
        onRetry={() => refetch()}
        retrying={isFetching}
      />
    );
  }

  if (!data?.length) {
    return (
      <EmptyState
        icon={Activity}
        title="No activity yet"
        description="Comments, category changes, and rule applications will appear here as this transaction evolves."
      />
    );
  }

  return (
    <TimelineRail>
      {groups.map((group) => (
        <TimelineRail.Group key={group.label} label={group.label}>
          {group.rows.map((row) => (
            <TimelineRow key={row.id} annotation={row} />
          ))}
        </TimelineRail.Group>
      ))}
    </TimelineRail>
  );
}

function TimelineRow({ annotation }: { annotation: Annotation }) {
  const Icon = KIND_ICON[annotation.kind] ?? Activity;
  const deleted = annotation.is_deleted;
  // Comments carry a body; everything else is fully described by `summary`.
  const showBody = annotation.kind === "comment" && !deleted && annotation.content;
  // Deleted rows drop the tint entirely (the strikethrough vocabulary
  // owns the signal); live rows pick from KIND_TONE, falling back to
  // neutral for unmapped kinds.
  const tone: TimelineRailTone = deleted ? "neutral" : (KIND_TONE[annotation.kind] ?? "neutral");

  // For live comment rows the bubble below carries the full body, so the
  // header line collapses to "<actor> commented" — otherwise the
  // server-built `summary` ("Alice commented: hello world") would duplicate
  // the bubble text. Mirrors v1's TimelineCommentRow shape.
  const headerLine = showBody
    ? `${annotation.actor_name} commented`
    : deleted
      ? `${annotation.actor_name} deleted a comment`
      : (annotation.summary ??
        `${annotation.actor_name} ${annotation.action ?? annotation.kind}`);

  return (
    <TimelineRail.Row icon={Icon} tone={tone} muted={deleted ? true : false}>
      <p className="text-sm leading-snug">{headerLine}</p>
      {showBody && (
        <p className="text-muted-foreground bg-muted/50 mt-1.5 rounded-md px-2.5 py-1.5 text-sm whitespace-pre-wrap">
          {annotation.content}
        </p>
      )}
      <p className="text-muted-foreground mt-1 text-[11px]">
        {formatRelativeTime(annotation.created_at)}
      </p>
    </TimelineRail.Row>
  );
}
