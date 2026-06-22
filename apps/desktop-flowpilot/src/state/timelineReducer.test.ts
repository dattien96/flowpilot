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

// Transcript replay tests

test("history replay: turn_started{prompt} adds a prompt bubble before the assistant response", () => {
  // Simulates the event sequence emitted by loadClaudeTranscriptEvents /
  // loadCodexTranscriptEvents on resume: a user prompt event followed by
  // the assistant message. Both must appear in the timeline in correct order.
  const empty: TimelineState = { status: "idle", timeline: [], recoverable: false };

  const afterPrompt = applyTimelineEvent(
    empty,
    baseEvent({ type: "turn_started", providerTurnId: "replay-prompt-1", prompt: "hello there" }),
  );

  const afterAssistant = applyTimelineEvent(
    { ...empty, ...afterPrompt },
    baseEvent({ type: "message_completed", text: "Hi! How can I help?" }),
  );

  const timeline = afterAssistant.timeline ?? [];
  const promptItem = timeline.find((it) => it.kind === "prompt");
  const assistantItem = timeline.find((it) => it.kind === "assistant");

  assert.ok(promptItem, "prompt bubble should be present");
  assert.ok(assistantItem, "assistant bubble should be present");
  assert.equal((promptItem as Extract<typeof timeline[0], { kind: "prompt" }>).text, "hello there");
  assert.equal((assistantItem as Extract<typeof timeline[0], { kind: "assistant" }>).text, "Hi! How can I help?");

  const promptIdx = timeline.indexOf(promptItem!);
  const assistantIdx = timeline.indexOf(assistantItem!);
  assert.ok(promptIdx < assistantIdx, "prompt bubble must precede assistant bubble");
});

test("history replay: hasPendingPrompt prevents double-render when turn_started{prompt} fires on live turn", () => {
  // During a live turn the desktop already pushed the prompt optimistically.
  // If turn_started also carries prompt (it does not today, but guard must hold),
  // the second push must be de-duped.
  const stateWithPrompt: TimelineState = {
    status: "running",
    timeline: [{ kind: "prompt", id: "prompt-0", text: "hello there" }],
    recoverable: false,
  };

  const after = applyTimelineEvent(
    stateWithPrompt,
    baseEvent({ type: "turn_started", providerTurnId: "replay-prompt-1", prompt: "hello there" }),
  );

  const prompts = (after.timeline ?? []).filter((it) => it.kind === "prompt");
  assert.equal(prompts.length, 1, "must not double-render an already-present prompt");
});

test("history replay: turn_completed after replay removes thinking row", () => {
  // seedTranscriptFromDisk appends a synthetic turn_completed to close the
  // trailing Thinking... row that finalize() would inject after message_completed.
  const empty: TimelineState = { status: "idle", timeline: [], recoverable: false };

  let state = { ...empty };
  for (const e of [
    baseEvent({ type: "turn_started", providerTurnId: "replay-prompt-1", prompt: "hi" }),
    baseEvent({ type: "message_completed", text: "hello" }),
    baseEvent({ type: "turn_completed", finalMessage: "hello" }),
  ] as ProviderEventDTO[]) {
    state = { ...state, ...applyTimelineEvent(state, e) } as TimelineState;
  }

  assert.equal(state.timeline.some((it) => it.kind === "thinking"), false, "no Thinking... row after turn_completed");
  assert.equal(state.status, "completed");
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

// ── Idempotent re-delivery (BUG-111) ────────────────────────────────────────
// Switching chats in the history panel re-streams a run from seq 0. Because the
// persisted events keep their original ids on replay, applying the same recorded
// sequence twice must rebuild the SAME timeline, not duplicate bubbles/tools.

// Helper: fold a sequence of events into a fresh timeline.
function foldEvents(events: ProviderEventDTO[]): TimelineState {
  let state: TimelineState = { status: "idle", timeline: [], recoverable: false };
  for (const e of events) {
    state = { ...state, ...applyTimelineEvent(state, e) } as TimelineState;
  }
  return state;
}

const SPAWN_TURN: ProviderEventDTO[] = [
  baseEvent({ id: "evt-ts", seq: 1, type: "turn_started", providerTurnId: "turn-1", prompt: "spawn the reviewer" }),
  baseEvent({ id: "evt-tool-start", seq: 2, type: "tool_started", toolName: "mcp__flowpilot__spawn_agent", input: {} }),
  baseEvent({ id: "evt-tool-done", seq: 3, type: "tool_completed", toolName: "mcp__flowpilot__spawn_agent", status: "success", output: "ok" }),
  baseEvent({ id: "evt-d1", seq: 4, type: "message_delta", text: "The child agent " }),
  baseEvent({ id: "evt-d2", seq: 5, type: "message_delta", text: "returned exactly: CHILD_AGENT_DONE" }),
  baseEvent({ id: "evt-mc", seq: 6, type: "message_completed", text: "The child agent returned exactly: CHILD_AGENT_DONE" }),
  baseEvent({ id: "evt-tc", seq: 7, type: "turn_completed", finalMessage: "The child agent returned exactly: CHILD_AGENT_DONE" }),
];

test("applying a recorded turn twice is idempotent — no duplicate bubbles or tools (BUG-111)", () => {
  const once = foldEvents(SPAWN_TURN);
  // Replay the SAME recorded events again into the already-built timeline (chat switch).
  let twice: TimelineState = once;
  for (const e of SPAWN_TURN) {
    twice = { ...twice, ...applyTimelineEvent(twice, e) } as TimelineState;
  }

  const assistants = twice.timeline.filter((it) => it.kind === "assistant");
  const tools = twice.timeline.filter((it) => it.kind === "tool");
  const prompts = twice.timeline.filter((it) => it.kind === "prompt");

  assert.equal(prompts.length, 1, "prompt must not duplicate on re-delivery");
  assert.equal(tools.length, 1, "tool row must not duplicate on re-delivery");
  assert.equal(assistants.length, 1, "assistant bubble must not duplicate on re-delivery");
  assert.equal(
    (assistants[0] as Extract<TimelineItem, { kind: "assistant" }>).text,
    "The child agent returned exactly: CHILD_AGENT_DONE",
    "re-streamed text must rebuild in place, not double",
  );
  assert.equal((assistants[0] as Extract<TimelineItem, { kind: "assistant" }>).finalized, true);
});

test("re-streaming a finalized bubble (deltas restart) rebuilds it in place (BUG-111)", () => {
  // Build a completed bubble, then re-deliver only its deltas (streamingAssistantId is
  // cleared after completion) — the second stream must resume the same bubble.
  const state = foldEvents([
    baseEvent({ id: "evt-d1", seq: 1, type: "message_delta", text: "The " }),
    baseEvent({ id: "evt-d2", seq: 2, type: "message_delta", text: "answer" }),
    baseEvent({ id: "evt-mc", seq: 3, type: "message_completed", text: "The answer" }),
  ]);
  assert.equal(state.timeline.filter((it) => it.kind === "assistant").length, 1);

  // Re-deliver the first delta (same id) after completion: must NOT create a 2nd bubble.
  const next = applyTimelineEvent(state, baseEvent({ id: "evt-d1", seq: 1, type: "message_delta", text: "The " }));
  const assistants = (next.timeline ?? []).filter((it) => it.kind === "assistant");
  assert.equal(assistants.length, 1, "re-delivered delta must resume existing bubble, not duplicate");
});

test("legitimate repeated tool of the same name still records both calls (BUG-111)", () => {
  // Two distinct read calls: each tool_completed has a matching running tool, so the
  // re-delivery guard must NOT collapse them.
  const state = foldEvents([
    baseEvent({ id: "evt-t1s", seq: 1, type: "tool_started", toolName: "read", input: { path: "a" } }),
    baseEvent({ id: "evt-t1c", seq: 2, type: "tool_completed", toolName: "read", status: "success", output: "A" }),
    baseEvent({ id: "evt-t2s", seq: 3, type: "tool_started", toolName: "read", input: { path: "b" } }),
    baseEvent({ id: "evt-t2c", seq: 4, type: "tool_completed", toolName: "read", status: "success", output: "B" }),
  ]);
  const tools = state.timeline.filter((it) => it.kind === "tool");
  assert.equal(tools.length, 2, "two distinct tool calls of the same name must both render");
});

test("duplicate message_completed emissions for one message collapse to one bubble (BUG-116)", () => {
  // The Codex mapper can derive several message_completed events (distinct ids) from one
  // logical assistant message (agent_message + item/completed). They must not stack into
  // multiple identical bubbles — the "CHILD_AGENT_DONE ×3" symptom.
  const state = foldEvents([
    baseEvent({ id: "evt-ts", seq: 1, type: "turn_started", providerTurnId: "t1", prompt: "review" }),
    baseEvent({ id: "evt-mc1", seq: 2, type: "message_completed", text: "CHILD_AGENT_DONE" }),
    baseEvent({ id: "evt-mc2", seq: 3, type: "message_completed", text: "CHILD_AGENT_DONE" }),
    baseEvent({ id: "evt-mc3", seq: 4, type: "message_completed", text: "CHILD_AGENT_DONE" }),
    baseEvent({ id: "evt-tc", seq: 5, type: "turn_completed", finalMessage: "CHILD_AGENT_DONE" }),
  ]);
  const assistants = state.timeline.filter((it) => it.kind === "assistant");
  assert.equal(assistants.length, 1, "repeated identical completions must collapse to one bubble");
  assert.equal((assistants[0] as Extract<TimelineItem, { kind: "assistant" }>).text, "CHILD_AGENT_DONE");
});

test("two genuinely different consecutive messages both render (BUG-116 guard is text-scoped)", () => {
  const state = foldEvents([
    baseEvent({ id: "evt-ts", seq: 1, type: "turn_started", providerTurnId: "t1", prompt: "go" }),
    baseEvent({ id: "evt-mc1", seq: 2, type: "message_completed", text: "first point" }),
    baseEvent({ id: "evt-mc2", seq: 3, type: "message_completed", text: "second point" }),
  ]);
  const texts = state.timeline
    .filter((it) => it.kind === "assistant")
    .map((it) => (it.kind === "assistant" ? it.text : ""));
  assert.deepEqual(texts, ["first point", "second point"], "distinct messages must not be collapsed");
});
