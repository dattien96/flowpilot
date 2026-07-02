import type { TimelineItem } from "@/state/store";

export type ToolItem = Extract<TimelineItem, { kind: "tool" }>;
export type ApprovalItem = Extract<TimelineItem, { kind: "approval" }>;
export type QuestionItem = Extract<TimelineItem, { kind: "question" }>;

export type TimelineGroup =
  | TimelineItem
  | { kind: "tool-group"; id: string; tools: ToolItem[] }
  | { kind: "approval-group"; id: string; items: ApprovalItem[] }
  | { kind: "question-group"; id: string; items: QuestionItem[] };

// When more than one approval (or question) is outstanding at once — e.g. a provider
// turn fans out several parallel tool calls needing approval before the user acts on
// any of them — fold them into a single collapsible group instead of stacking N
// separate full-size cards. A single pending item still renders as its own plain card
// (no group wrapper). Resolved items are unaffected and render in place as history.
export function buildTimelineGroups(timeline: TimelineItem[]): TimelineGroup[] {
  const groups: TimelineGroup[] = [];
  let pendingTools: ToolItem[] = [];

  const pendingApprovalItems = timeline.filter(
    (it): it is ApprovalItem => it.kind === "approval" && it.decision === undefined,
  );
  const pendingQuestionItems = timeline.filter(
    (it): it is QuestionItem => it.kind === "question" && it.answer === undefined,
  );
  const lastPendingApprovalId =
    pendingApprovalItems.length > 1 ? pendingApprovalItems[pendingApprovalItems.length - 1].id : undefined;
  const lastPendingQuestionId =
    pendingQuestionItems.length > 1 ? pendingQuestionItems[pendingQuestionItems.length - 1].id : undefined;

  const flushTools = () => {
    if (pendingTools.length === 0) return;
    const firstToolId = pendingTools[0].id || `${pendingTools[0].toolName}-${groups.length}`;
    groups.push({ kind: "tool-group", id: `tool-group-${firstToolId}`, tools: pendingTools });
    pendingTools = [];
  };

  for (const item of timeline) {
    if (item.kind === "tool") {
      pendingTools.push(item);
      continue;
    }
    flushTools();

    if (item.kind === "approval" && item.decision === undefined && lastPendingApprovalId !== undefined) {
      // Fold every pending approval into one group emitted at the last one's position;
      // the earlier ones are already captured in pendingApprovalItems, so skip them here.
      if (item.id === lastPendingApprovalId) {
        groups.push({ kind: "approval-group", id: `approval-group-${item.id}`, items: pendingApprovalItems });
      }
      continue;
    }

    if (item.kind === "question" && item.answer === undefined && lastPendingQuestionId !== undefined) {
      if (item.id === lastPendingQuestionId) {
        groups.push({ kind: "question-group", id: `question-group-${item.id}`, items: pendingQuestionItems });
      }
      continue;
    }

    groups.push(item);
  }
  flushTools();

  return groups;
}
