import test from "node:test";
import assert from "node:assert/strict";
import { applyTimelineEvent, type TimelineState } from "./timelineReducer";
import type { ProviderEventDTO } from "../types/contract";

function initialState(): TimelineState {
  return { status: "running", timeline: [], recoverable: false, pendingApprovals: [], pendingQuestions: [] };
}

function event(providerKey: "codex" | "claude" | "grok", type: ProviderEventDTO["type"], seq: number): ProviderEventDTO {
  return {
    id: `${providerKey}-${seq}`,
    workflowRunId: `run-${providerKey}`,
    providerSessionId: `session-${providerKey}`,
    providerKey,
    seq,
    occurredAt: "2026-07-20T12:00:00Z",
    type,
    prompt: type === "turn_started" ? "restore this completed flow" : undefined,
  } as ProviderEventDTO;
}

test("terminal replay clears Thinking for Codex, Claude, and Grok", () => {
  for (const providerKey of ["codex", "claude", "grok"] as const) {
    let state = applyTimelineEvent(initialState(), event(providerKey, "turn_started", 1));
    state = applyTimelineEvent({ ...initialState(), ...state, timeline: state.timeline ?? [] }, event(providerKey, "message_completed", 2));
    state = applyTimelineEvent({ ...initialState(), ...state, timeline: state.timeline ?? [] }, event(providerKey, "turn_completed", 3));

    assert.equal(state.status, "completed", providerKey);
    assert.equal(state.timeline?.some((item) => item.kind === "thinking"), false, providerKey);
  }
});
