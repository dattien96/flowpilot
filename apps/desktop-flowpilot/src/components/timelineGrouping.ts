import type { TimelineItem } from "@/state/store";

export type ToolItem = Extract<TimelineItem, { kind: "tool" }>;
export type ApprovalItem = Extract<TimelineItem, { kind: "approval" }>;
export type QuestionItem = Extract<TimelineItem, { kind: "question" }>;

export type TimelineGroup =
  | TimelineItem
  | { kind: "tool-group"; id: string; tools: ToolItem[] }
  | { kind: "approval-group"; id: string; items: ApprovalItem[] }
  | { kind: "question-group"; id: string; items: QuestionItem[] };

// When more than one approval (or question) appears back-to-back — e.g. a provider
// turn fans out several parallel tool calls needing approval before the user acts on
// any of them — fold that run into a single collapsible group instead of stacking N
// separate full-size cards. A run of exactly one item still renders as its own plain
// card (no group wrapper).
//
// Grouping is by CONSECUTIVE run, not "currently still pending": once a run of 2+ is
// grouped, it stays grouped after the items resolve (e.g. clicking "Approve all")
// instead of the group dissolving and every item bursting back out as a full card.
// Any other item kind (tool, assistant text, file, etc.) between two approvals ends
// the run.
export function buildTimelineGroups(timeline: TimelineItem[]): TimelineGroup[] {
  const groups: TimelineGroup[] = [];
  let pendingTools: ToolItem[] = [];
  let approvalRun: ApprovalItem[] = [];
  let questionRun: QuestionItem[] = [];

  const flushTools = () => {
    if (pendingTools.length === 0) return;
    const firstToolId = pendingTools[0].id || `${pendingTools[0].toolName}-${groups.length}`;
    groups.push({ kind: "tool-group", id: `tool-group-${firstToolId}`, tools: pendingTools });
    pendingTools = [];
  };

  const flushApprovals = () => {
    if (approvalRun.length === 0) return;
    if (approvalRun.length === 1) {
      groups.push(approvalRun[0]);
    } else {
      groups.push({ kind: "approval-group", id: `approval-group-${approvalRun[0].id}`, items: approvalRun });
    }
    approvalRun = [];
  };

  const flushQuestions = () => {
    if (questionRun.length === 0) return;
    if (questionRun.length === 1) {
      groups.push(questionRun[0]);
    } else {
      groups.push({ kind: "question-group", id: `question-group-${questionRun[0].id}`, items: questionRun });
    }
    questionRun = [];
  };

  for (const item of timeline) {
    if (item.kind === "tool") {
      flushApprovals();
      flushQuestions();
      pendingTools.push(item);
      continue;
    }
    if (item.kind === "approval") {
      flushTools();
      flushQuestions();
      approvalRun.push(item);
      continue;
    }
    if (item.kind === "question") {
      flushTools();
      flushApprovals();
      questionRun.push(item);
      continue;
    }
    flushTools();
    flushApprovals();
    flushQuestions();
    groups.push(item);
  }
  flushTools();
  flushApprovals();
  flushQuestions();

  return groups;
}
