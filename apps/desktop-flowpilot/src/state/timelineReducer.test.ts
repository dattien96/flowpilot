import test from "node:test";
import assert from "node:assert/strict";
import { applyTimelineEvent, shouldApplyRunEvent, type TimelineState, type TimelineItem } from "./timelineReducer";
import type { ProviderEventDTO } from "../types/contract";

function baseEvent(overrides: Partial<ProviderEventDTO>): ProviderEventDTO {
  return {
    id: "evt-1",
    workflowRunId: "run-1",
    providerSessionId: "thread-1",
    providerKey: "codex",
    seq: 1,
    occurredAt: "2026-06-13T10:00:00.000Z",
    type: "turn_completed",
    finalMessage: "done",
    ...overrides,
  } as ProviderEventDTO;
}

function thinkingState(timeline: TimelineItem[]): TimelineState {
  return {
    status: "running",
    timeline,
    recoverable: false,
    _streamingAssistantId: "assistant-1",
  };
}

test("tool_completed keeps the live thinking row at the end of the timeline", () => {
  const state = thinkingState([
    { kind: "prompt", id: "prompt-1", text: "Fix it" },
    { kind: "assistant", id: "assistant-1", text: "Looking...", finalized: false },
    { kind: "tool", id: "tool-1", toolName: "search", status: "running", input: { q: "bug" } },
    { kind: "thinking", id: "thinking-1", text: "Thinking..." },
  ]);

  const next = applyTimelineEvent(
    state,
    baseEvent({
      type: "tool_completed",
      toolName: "search",
      status: "success",
      output: { matches: 2 },
    }),
  );

  const nextTimeline = next.timeline ?? [];
  assert.equal(nextTimeline.at(-1)?.kind, "thinking");
  const tool = nextTimeline.find((item) => item.kind === "tool");
  assert.deepEqual(tool, {
    kind: "tool",
    id: "tool-1",
    toolName: "search",
    status: "success",
    input: { q: "bug" },
    output: { matches: 2 },
  });
});

test("turn_completed removes the thinking row once the answer is done", () => {
  const state = thinkingState([
    { kind: "prompt", id: "prompt-1", text: "Fix it" },
    { kind: "assistant", id: "assistant-1", text: "All set", finalized: true },
    { kind: "thinking", id: "thinking-1", text: "Thinking..." },
  ]);

  const next = applyTimelineEvent(
    state,
    baseEvent({
      type: "turn_completed",
      finalMessage: "All set",
    }),
  );

  assert.equal(next.timeline?.some((item) => item.kind === "thinking"), false);
});

test("run events only apply to the currently active run", () => {
  assert.equal(shouldApplyRunEvent("run-claude", "run-claude"), true);
  assert.equal(shouldApplyRunEvent("run-claude", "run-codex"), false);
  assert.equal(shouldApplyRunEvent(undefined, "run-codex"), false);
});

// Approval gate tests

const approvalDetails = { decisions: [{ value: "approve", label: "Approve" }, { value: "deny", label: "Deny" }] };

function approvalState(extras: Partial<TimelineState> = {}): TimelineState {
  return {
    status: "running",
    timeline: [],
    recoverable: false,
    ...extras,
  };
}

test("permission_required adds approval card to timeline and sets pendingApproval", () => {
  const state = approvalState();
  const next = applyTimelineEvent(
    state,
    baseEvent({ type: "permission_required", approvalId: "appr-1", provider: "codex", details: approvalDetails }),
  );

  const card = next.timeline?.find((it) => it.kind === "approval");
  assert.ok(card, "approval card should be added");
  assert.equal((card as Extract<TimelineItem, { kind: "approval" }>).decision, undefined);
  assert.deepEqual(next.pendingApproval, { approvalId: "appr-1", details: approvalDetails });
  assert.equal(next.status, "waiting_approval");
});

test("history replay: tool_completed after permission_required stamps card resolved and clears pendingApproval", () => {
  // Simulates the state after permission_required was replayed from history
  const state = approvalState({
    status: "waiting_approval",
    timeline: [
      { kind: "tool", id: "tool-1", toolName: "mcp__search", status: "running" },
      { kind: "approval", id: "appr-1", approvalId: "appr-1", details: approvalDetails },
    ],
    pendingApproval: { approvalId: "appr-1", details: approvalDetails },
  });

  const next = applyTimelineEvent(
    state,
    baseEvent({ type: "tool_completed", toolName: "mcp__search", status: "success" }),
  );

  const card = next.timeline?.find((it) => it.kind === "approval") as Extract<TimelineItem, { kind: "approval" }> | undefined;
  assert.ok(card, "approval card should still be in timeline");
  assert.equal(card?.decision, "resolved", "card should be stamped as resolved");
  assert.equal(next.pendingApproval, undefined, "pendingApproval should be cleared");
  assert.equal(next.status, "running");
});

test("history replay: turn_completed after permission_required stamps card resolved and clears pendingApproval", () => {
  const state = approvalState({
    status: "waiting_approval",
    timeline: [
      { kind: "approval", id: "appr-1", approvalId: "appr-1", details: approvalDetails },
    ],
    pendingApproval: { approvalId: "appr-1", details: approvalDetails },
  });

  const next = applyTimelineEvent(
    state,
    baseEvent({ type: "turn_completed", finalMessage: "done" }),
  );

  const card = next.timeline?.find((it) => it.kind === "approval") as Extract<TimelineItem, { kind: "approval" }> | undefined;
  assert.equal(card?.decision, "resolved", "card should be stamped as resolved");
  assert.equal(next.pendingApproval, undefined, "pendingApproval should be cleared");
  assert.equal(next.status, "completed");
});

test("live run: no stale detection when pendingApproval is undefined before tool_completed", () => {
  // In a live run approve() clears pendingApproval synchronously, so by the time
  // any server event arrives pendingApproval is already undefined.
  const state = approvalState({
    status: "running",
    timeline: [
      { kind: "tool", id: "tool-1", toolName: "mcp__search", status: "running" },
      { kind: "approval", id: "appr-1", approvalId: "appr-1", details: approvalDetails, decision: "approve" },
    ],
    pendingApproval: undefined,
  });

  const next = applyTimelineEvent(
    state,
    baseEvent({ type: "tool_completed", toolName: "mcp__search", status: "success" }),
  );

  // pendingApproval was already undefined — should remain undefined, no side effects
  assert.equal(next.pendingApproval, undefined);
  // The already-resolved card should still have its decision intact
  const card = next.timeline?.find((it) => it.kind === "approval") as Extract<TimelineItem, { kind: "approval" }> | undefined;
  assert.equal(card?.decision, "approve", "existing decision should not be changed");
});

test("new permission_required while previous pendingApproval is set stamps the first and sets the second", () => {
  // Two consecutive approval gates in replay: first is stale, second permission_required
  // fires — staleApproval stamps first card; ...extra from the switch overrides
  // pendingApproval back to the new approval value.
  const state = approvalState({
    status: "waiting_approval",
    timeline: [
      { kind: "approval", id: "appr-1", approvalId: "appr-1", details: approvalDetails },
    ],
    pendingApproval: { approvalId: "appr-1", details: approvalDetails },
  });

  const next = applyTimelineEvent(
    state,
    baseEvent({ type: "permission_required", approvalId: "appr-2", provider: "codex", details: approvalDetails }),
  );

  const firstCard = next.timeline?.find(
    (it) => it.kind === "approval" && (it as Extract<TimelineItem, { kind: "approval" }>).approvalId === "appr-1",
  ) as Extract<TimelineItem, { kind: "approval" }> | undefined;
  assert.equal(firstCard?.decision, "resolved", "first approval card should be stamped by stale detection");
  assert.deepEqual(next.pendingApproval, { approvalId: "appr-2", details: approvalDetails }, "pendingApproval should point to the new approval");
});

// Question stale detection tests

const questionOptions = [
  { label: "Python", value: "Python" },
  { label: "TypeScript", value: "TypeScript" },
];

test("user_question_required adds question card and sets pendingQuestion", () => {
  const state = approvalState();
  const next = applyTimelineEvent(
    state,
    baseEvent({ type: "user_question_required", questionId: "q-1", prompt: "Pick one", options: questionOptions }),
  );

  const card = next.timeline?.find((it) => it.kind === "question");
  assert.ok(card, "question card should be added");
  assert.equal((card as Extract<TimelineItem, { kind: "question" }>).answer, undefined);
  assert.ok(next.pendingQuestion, "pendingQuestion should be set");
  assert.equal(next.status, "waiting_question");
});

test("history replay: follow-up event after user_question_required stamps card as answered and clears pendingQuestion", () => {
  const state = approvalState({
    status: "waiting_question",
    timeline: [
      { kind: "question", id: "q-1", questionId: "q-1", prompt: "Pick one", options: questionOptions },
    ],
    pendingQuestion: { questionId: "q-1", prompt: "Pick one", options: questionOptions },
  });

  const next = applyTimelineEvent(
    state,
    baseEvent({ type: "turn_completed", finalMessage: "done" }),
  );

  const card = next.timeline?.find((it) => it.kind === "question") as Extract<TimelineItem, { kind: "question" }> | undefined;
  assert.equal(card?.answer, "answered", "question card should be stamped as answered");
  assert.equal(next.pendingQuestion, undefined, "pendingQuestion should be cleared");
});

test("history replay: permission_required after user_question_required stamps question and sets new approval", () => {
  // Question was answered before an approval gate fired — permission_required
  // should trigger stale question detection (no type guard prevents it).
  const state = approvalState({
    status: "waiting_question",
    timeline: [
      { kind: "question", id: "q-1", questionId: "q-1", prompt: "Pick one", options: questionOptions },
    ],
    pendingQuestion: { questionId: "q-1", prompt: "Pick one", options: questionOptions },
  });

  const next = applyTimelineEvent(
    state,
    baseEvent({ type: "permission_required", approvalId: "appr-1", provider: "codex", details: approvalDetails }),
  );

  const qCard = next.timeline?.find((it) => it.kind === "question") as Extract<TimelineItem, { kind: "question" }> | undefined;
  assert.equal(qCard?.answer, "answered", "question card should be stamped when approval gate fires after it");
  assert.equal(next.pendingQuestion, undefined, "pendingQuestion should be cleared");
  assert.ok(next.pendingApproval, "pendingApproval should be set for the new approval");
});

test("live run: no stale detection for question when pendingQuestion is already undefined", () => {
  // answer() clears pendingQuestion synchronously, so it is undefined by the time
  // any server event arrives during a live run.
  const state = approvalState({
    status: "running",
    timeline: [
      { kind: "question", id: "q-1", questionId: "q-1", prompt: "Pick one", options: questionOptions, answer: "Python" },
    ],
    pendingQuestion: undefined,
  });

  const next = applyTimelineEvent(
    state,
    baseEvent({ type: "turn_completed", finalMessage: "done" }),
  );

  const card = next.timeline?.find((it) => it.kind === "question") as Extract<TimelineItem, { kind: "question" }> | undefined;
  assert.equal(card?.answer, "Python", "existing answer should not be overwritten");
  assert.equal(next.pendingQuestion, undefined);
});

// UC2: history replay — denied approval (BUG-074 regression guard)
// The user denied an approval in a prior session; when history is replayed the
// event stream only contains permission_required (no decision event was persisted).
// The card must be stamped "resolved" — a neutral sentinel — never "approved" even
// though the run did continue after the gate.
test("history replay: denied approval is stamped resolved not approved (BUG-074)", () => {
  const state = approvalState({
    status: "waiting_approval",
    timeline: [
      { kind: "approval", id: "appr-1", approvalId: "appr-1", details: approvalDetails },
    ],
    pendingApproval: { approvalId: "appr-1", details: approvalDetails },
  });

  const next = applyTimelineEvent(
    state,
    baseEvent({ type: "tool_completed", toolName: "mcp__search", status: "success" }),
  );

  const card = next.timeline?.find((it) => it.kind === "approval") as Extract<TimelineItem, { kind: "approval" }> | undefined;
  assert.equal(card?.decision, "resolved", "sentinel must be neutral resolved");
  assert.notEqual(card?.decision, "approved", "must not claim approved — original decision may have been deny");
  assert.notEqual(card?.decision, "deny", "must not claim deny — decision is not persisted in the stream");
  assert.equal(next.pendingApproval, undefined, "pendingApproval should be cleared");
});

// UC5 complement: live deny — deny decision is preserved, stale detection does not fire
// When the user clicks Deny in a live run, approve()/deny() stamps decision: "deny"
// and clears pendingApproval synchronously. The next server event must NOT overwrite
// the real decision with the "resolved" sentinel.
test("live run: deny decision is preserved after subsequent events (BUG-074)", () => {
  const state = approvalState({
    status: "running",
    timeline: [
      { kind: "approval", id: "appr-1", approvalId: "appr-1", details: approvalDetails, decision: "deny" },
    ],
    pendingApproval: undefined,
  });

  const next = applyTimelineEvent(
    state,
    baseEvent({ type: "turn_completed", finalMessage: "done" }),
  );

  const card = next.timeline?.find((it) => it.kind === "approval") as Extract<TimelineItem, { kind: "approval" }> | undefined;
  assert.equal(card?.decision, "deny", "live deny decision must not be overwritten by stale detection");
  assert.equal(next.pendingApproval, undefined);
});
