import test from "node:test";
import assert from "node:assert/strict";
import { buildTimelineGroups, type TimelineGroup } from "./timelineGrouping";
import type { TimelineItem } from "@/state/store";

const approvalDetails = { decisions: [{ value: "approve", label: "Approve" }, { value: "deny", label: "Deny" }] };
const questionOptions = [{ label: "Python", value: "Python" }, { label: "TypeScript", value: "TypeScript" }];

function findGroup(groups: TimelineGroup[], kind: TimelineGroup["kind"]): TimelineGroup | undefined {
  return groups.find((g) => g.kind === kind);
}

test("BUG-157 UX: a single pending approval renders as a plain card, no group wrapper", () => {
  const timeline: TimelineItem[] = [
    { kind: "approval", id: "appr-1", approvalId: "appr-1", details: approvalDetails },
  ];

  const groups = buildTimelineGroups(timeline);

  assert.equal(groups.length, 1);
  assert.equal(groups[0].kind, "approval");
});

test("BUG-157 UX: several concurrently pending approvals fold into one approval-group", () => {
  const timeline: TimelineItem[] = [
    { kind: "approval", id: "appr-1", approvalId: "appr-1", details: approvalDetails },
    { kind: "approval", id: "appr-2", approvalId: "appr-2", details: approvalDetails },
    { kind: "approval", id: "appr-3", approvalId: "appr-3", details: approvalDetails },
  ];

  const groups = buildTimelineGroups(timeline);

  assert.equal(groups.length, 1, "all three should collapse into a single group, not three separate cards");
  const group = groups[0] as Extract<TimelineGroup, { kind: "approval-group" }>;
  assert.equal(group.kind, "approval-group");
  assert.deepEqual(group.items.map((i) => i.approvalId), ["appr-1", "appr-2", "appr-3"]);
});

test("resolved approvals are excluded from the group and still render individually as history", () => {
  const timeline: TimelineItem[] = [
    { kind: "approval", id: "appr-0", approvalId: "appr-0", details: approvalDetails, decision: "approve" },
    { kind: "approval", id: "appr-1", approvalId: "appr-1", details: approvalDetails },
    { kind: "approval", id: "appr-2", approvalId: "appr-2", details: approvalDetails },
  ];

  const groups = buildTimelineGroups(timeline);

  assert.equal(groups.length, 2, "one resolved card in place + one group for the two pending ones");
  assert.equal(groups[0].kind, "approval");
  const group = groups[1] as Extract<TimelineGroup, { kind: "approval-group" }>;
  assert.equal(group.kind, "approval-group");
  assert.deepEqual(group.items.map((i) => i.approvalId), ["appr-1", "appr-2"]);
});

test("tool calls interleaved between concurrent approvals do not break the grouping", () => {
  const timeline: TimelineItem[] = [
    { kind: "approval", id: "appr-1", approvalId: "appr-1", details: approvalDetails },
    { kind: "tool", id: "tool-1", toolName: "search", status: "success" },
    { kind: "approval", id: "appr-2", approvalId: "appr-2", details: approvalDetails },
  ];

  const groups = buildTimelineGroups(timeline);

  // tool-group for the single tool call, then the approval-group at the position of appr-2
  assert.equal(groups.length, 2);
  assert.equal(groups[0].kind, "tool-group");
  const group = groups[1] as Extract<TimelineGroup, { kind: "approval-group" }>;
  assert.equal(group.kind, "approval-group");
  assert.deepEqual(group.items.map((i) => i.approvalId), ["appr-1", "appr-2"]);
});

test("questions and approvals group independently (group-by-kind, not merged into one ask group)", () => {
  const timeline: TimelineItem[] = [
    { kind: "approval", id: "appr-1", approvalId: "appr-1", details: approvalDetails },
    { kind: "approval", id: "appr-2", approvalId: "appr-2", details: approvalDetails },
    { kind: "question", id: "q-1", questionId: "q-1", prompt: "Pick one", options: questionOptions },
    { kind: "question", id: "q-2", questionId: "q-2", prompt: "Pick two", options: questionOptions },
  ];

  const groups = buildTimelineGroups(timeline);

  assert.equal(groups.length, 2);
  const approvalGroup = findGroup(groups, "approval-group") as Extract<TimelineGroup, { kind: "approval-group" }>;
  const questionGroup = findGroup(groups, "question-group") as Extract<TimelineGroup, { kind: "question-group" }>;
  assert.equal(approvalGroup.items.length, 2);
  assert.equal(questionGroup.items.length, 2);
});

test("a single pending question renders as a plain card, no group wrapper", () => {
  const timeline: TimelineItem[] = [
    { kind: "question", id: "q-1", questionId: "q-1", prompt: "Pick one", options: questionOptions },
  ];

  const groups = buildTimelineGroups(timeline);

  assert.equal(groups.length, 1);
  assert.equal(groups[0].kind, "question");
});
