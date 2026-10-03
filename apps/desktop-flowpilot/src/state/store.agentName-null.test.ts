import test from "node:test";
import assert from "node:assert/strict";
import { mergeAgentRunsById } from "./store";
import { applyTimelineEvent, type TimelineState } from "./timelineReducer";
import type { AgentRunSummary, ProviderEventDTO } from "../types/contract";

// Live run-150388: spawn-agent children arrive on the wire with
// agentName:null (label-only legs). Renderers call .toLowerCase() on
// agentName — a null crashes the whole chat view ("Cannot read
// properties of undefined (reading 'toLowerCase')"). The merge/reducer
// boundaries must normalize it to a non-empty string.

function state(): TimelineState {
  return {
    status: "running",
    timeline: [],
    recoverable: false,
    pendingApprovals: [],
    pendingQuestions: [],
  };
}

test("mergeAgentRunsById normalizes null agentName to a non-empty string", () => {
  const wire = {
    runId: "run-child-1",
    agentName: null,
    role: "coder",
    status: "running",
    createdAt: "2026-10-03T00:00:00Z",
  } as unknown as AgentRunSummary;
  const merged = mergeAgentRunsById([], [wire]);
  assert.equal(merged.length, 1);
  assert.equal(typeof merged[0].agentName, "string");
  assert.ok(merged[0].agentName.length > 0);
  assert.doesNotThrow(() => merged[0].agentName.toLowerCase());
});

test("agent_spawned_by_user with null agentName produces a string card name", () => {
  const e = {
    id: "evt-1",
    workflowRunId: "run-parent",
    providerSessionId: "thread-1",
    providerKey: "devin",
    seq: 1,
    occurredAt: "2026-10-03T00:00:00Z",
    type: "agent_spawned_by_user",
    agentName: null,
    childRunId: "run-child-2",
  } as unknown as ProviderEventDTO;
  const next = applyTimelineEvent(state(), e);
  const card = next.timeline?.find((it) => it.kind === "agent");
  assert.ok(card && card.kind === "agent");
  assert.equal(typeof card.agentName, "string");
  assert.ok(card.agentName.length > 0);
  assert.doesNotThrow(() => card.agentName.toLowerCase());
});
