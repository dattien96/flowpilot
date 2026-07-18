import test from "node:test";
import assert from "node:assert/strict";
import { applyTimelineEvent, type TimelineState } from "./timelineReducer";
import type { ProviderEventDTO } from "../types/contract";

function state(): TimelineState {
  return {
    status: "running",
    timeline: [],
    recoverable: false,
    pendingApprovals: [],
    pendingQuestions: [],
  };
}

function event(overrides: Partial<ProviderEventDTO>): ProviderEventDTO {
  return {
    id: "event-1",
    workflowRunId: "run-1",
    providerSessionId: "thread-1",
    providerKey: "codex",
    seq: 1,
    occurredAt: "2026-07-18T14:00:00Z",
    type: "turn_completed",
    finalMessage: "done",
    ...overrides,
  } as ProviderEventDTO;
}

test("agent lifecycle replays as one completed agent card instead of prose rows", () => {
  const spawned = applyTimelineEvent(
    state(),
    event({ type: "agent_spawned_by_user", id: "agent-spawn", agentName: "coder", childRunId: "child-coder" }),
  );
  const completed = applyTimelineEvent(
    { ...state(), ...spawned, timeline: spawned.timeline ?? [] },
    event({
      type: "agent_result_injected",
      id: "agent-result",
      agentName: "coder",
      childRunId: "child-coder",
      finalMessage: "implemented",
    }),
  );

  assert.deepEqual(completed.timeline, [
    { kind: "agent", id: "agent-spawn", agentName: "coder", childRunId: "child-coder", finalMessage: "implemented" },
  ]);
});
