import test from "node:test";
import assert from "node:assert/strict";
import { applyTimelineEvent, type TimelineState, type TimelineItem } from "./timelineReducer";
import type { ProviderEventDTO } from "../types/contract";

// BUG-615 (live run-150388 / chat cht_ce24f4dd4cbc): engine-internal turns —
// hub reinvokes, debate/synthesis instructions, gate reprompts — rendered
// their prose as prompt/assistant/tool bubbles in the MAIN chat ("subagent
// chat in the main UI"). The runner now stamps ProviderEvent.Internal on
// every prose event inside an internal turn; the reducer must skip row pushes
// for those events while still settling open streaming state.

function baseEvent(overrides: Partial<ProviderEventDTO>): ProviderEventDTO {
  return {
    id: "evt-1",
    workflowRunId: "run-1",
    providerSessionId: "thread-1",
    providerKey: "codex",
    seq: 1,
    occurredAt: "2026-06-13T10:00:00.000Z",
    type: "turn_completed",
    ...overrides,
  } as ProviderEventDTO;
}

function state(timeline: TimelineItem[] = []): TimelineState {
  return {
    status: "running",
    timeline,
    recoverable: false,
    pendingApprovals: [],
    pendingQuestions: [],
  };
}

test("internal events never push prompt/assistant/tool rows", () => {
  let s = state();
  s = { ...s, ...applyTimelineEvent(s, baseEvent({ type: "turn_started", providerTurnId: "t-1", prompt: "", internal: true, seq: 1 })) };
  s = { ...s, ...applyTimelineEvent(s, baseEvent({ type: "message_delta", text: "Synthesizing…", internal: true, seq: 2 })) };
  s = { ...s, ...applyTimelineEvent(s, baseEvent({ type: "message_completed", text: "engine narration", internal: true, seq: 3 })) };
  s = { ...s, ...applyTimelineEvent(s, baseEvent({ type: "tool_started", toolName: "submit_review_outcome", internal: true, seq: 4 })) };
  s = { ...s, ...applyTimelineEvent(s, baseEvent({ type: "tool_completed", toolName: "submit_review_outcome", status: "success", internal: true, seq: 5 })) };
  s = { ...s, ...applyTimelineEvent(s, baseEvent({ type: "turn_completed", finalMessage: "engine narration", internal: true, seq: 6 })) };

  const rows = s.timeline.filter((it) => it.kind === "prompt" || it.kind === "assistant" || it.kind === "tool");
  assert.equal(rows.length, 0, `internal turn leaked rows: ${JSON.stringify(rows)}`);
});

test("internal prose skips rows but keeps actionable events", () => {
  const s0 = state();
  const s1 = { ...s0, ...applyTimelineEvent(s0, baseEvent({ type: "turn_started", providerTurnId: "t-1", prompt: "", internal: true })) };
  const s2 = {
    ...s1,
    ...applyTimelineEvent(s1, baseEvent({ type: "permission_required", approvalId: "ap-1", internal: false, details: { kind: "exec", command: "rm -rf", decisions: [{ value: "approve", label: "Approve" }, { value: "deny", label: "Deny" }] } })),
  };
  assert.equal(s2.timeline.some((it) => it.kind === "approval"), true, "approval inside internal turn must still surface");
});

test("user-facing turn after an internal turn still renders", () => {
  let s = state();
  s = { ...s, ...applyTimelineEvent(s, baseEvent({ type: "turn_started", providerTurnId: "t-1", prompt: "", internal: true, seq: 1 })) };
  s = { ...s, ...applyTimelineEvent(s, baseEvent({ type: "message_completed", text: "hidden", internal: true, seq: 2 })) };
  s = { ...s, ...applyTimelineEvent(s, baseEvent({ type: "turn_started", providerTurnId: "t-2", prompt: "user asks", seq: 3 })) };
  s = { ...s, ...applyTimelineEvent(s, baseEvent({ type: "message_completed", text: "visible answer", seq: 4 })) };

  const prompts = s.timeline.filter((it) => it.kind === "prompt");
  const answers = s.timeline.filter((it) => it.kind === "assistant");
  assert.equal(prompts.length, 1);
  assert.equal(answers.length, 1);
  assert.equal((answers[0] as Extract<TimelineItem, { kind: "assistant" }>).text, "visible answer");
});
