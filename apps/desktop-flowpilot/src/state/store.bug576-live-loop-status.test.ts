import test from "node:test";
import assert from "node:assert/strict";
import { applyEvent, useStore } from "./store";
import type { AgentGraphSnapshot, ProviderEventDTO } from "../types/contract";

// BUG-576 / BUG-583 (live run-100368): the header chip showed "Completed"
// while the backend sprint was still running. Two desktop-side status
// defects: (1) a hub turn_completed stamped "completed" even though the
// run's agent loop was open and legs kept working; (2) the new
// agent_activity leg-liveness heartbeat fell through statusFromEvent's
// default branch, flapping a parked waiting_* run back to "running" each
// throttle tick.

const BASE = {
  id: "e1",
  workflowRunId: "run-1",
  providerSessionId: "leg-1",
  providerKey: "codex",
  seq: 1,
  occurredAt: "2026-01-01T00:00:00Z",
} as const;

function snapshot(loopStatus: string, childStatus = "running"): AgentGraphSnapshot {
  return {
    parentRunId: "run-1",
    runs: [
      {
        runId: "run-leg-1",
        agentName: "coder",
        status: childStatus,
      } as AgentGraphSnapshot["runs"][number],
    ],
    edges: [],
    busMessages: [],
    loopState: { status: loopStatus, round: 0, roundCap: 3 },
  } as AgentGraphSnapshot;
}

function evt(overrides: Partial<ProviderEventDTO>): ProviderEventDTO {
  return { ...BASE, ...overrides } as ProviderEventDTO;
}

test("turn_completed on an open loop stays running, not Completed", () => {
  useStore.setState({ status: "running", agentGraphSnapshot: snapshot("running"), timeline: [] });
  useStore.setState((s) => applyEvent(s, evt({ type: "turn_completed" })));
  assert.equal(useStore.getState().status, "running");
});

test("turn_completed after loop done still completes", () => {
  useStore.setState({ status: "running", agentGraphSnapshot: snapshot("done", "completed"), timeline: [] });
  useStore.setState((s) => applyEvent(s, evt({ type: "turn_completed" })));
  assert.equal(useStore.getState().status, "completed");
});

test("turn_completed with no agent loop completes (plain chat unaffected)", () => {
  useStore.setState({ status: "running", agentGraphSnapshot: undefined, timeline: [] });
  useStore.setState((s) => applyEvent(s, evt({ type: "turn_completed" })));
  assert.equal(useStore.getState().status, "completed");
});

test("turn_completed on a blocked loop reads blocked, not Completed", () => {
  useStore.setState({ status: "blocked", agentGraphSnapshot: snapshot("blocked", "completed"), timeline: [] });
  useStore.setState((s) => applyEvent(s, evt({ type: "turn_completed" })));
  assert.equal(useStore.getState().status, "blocked");
});

test("agent_activity never flips a parked waiting status or adds a row", () => {
  useStore.setState({ status: "waiting_question", agentGraphSnapshot: snapshot("running"), timeline: [] });
  useStore.setState((s) =>
    applyEvent(
      s,
      evt({ type: "agent_activity", childRunId: "run-leg-1", seq: 2 }),
    ),
  );
  const s = useStore.getState();
  assert.equal(s.status, "waiting_question", "heartbeat must not unstick waiting_question");
  assert.equal(s.timeline.length, 0, "heartbeat must not render a timeline row");
});
