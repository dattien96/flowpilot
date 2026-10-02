import test from "node:test";
import assert from "node:assert/strict";
import type { ProviderEventDTO } from "@/types/contract";
import {
  RUN_ACTIVITY_STALE_MS,
  activityCountsForStatus,
  eventActivityRunId,
  formatActivityAge,
  lastActivityAt,
  runActivityKind,
  stampRunActivity,
} from "./runActivity";

// Task-458 (AC-5): liveness derives from SSE arrival stamps keyed by run id.
// These tests pin the stamp routing (regular vs orchestration events), the
// >30s live→quiet threshold, and the terminal-status exclusion that keeps a
// settled leg from faking liveness for its flow.

function baseEvent(overrides: Record<string, unknown>): ProviderEventDTO {
  return {
    id: "evt-1",
    workflowRunId: "run-1",
    providerSessionId: "thread-1",
    providerKey: "codex",
    seq: 7,
    occurredAt: "2026-10-02T15:00:00.000Z",
    type: "message_delta",
    text: "x",
    ...overrides,
  } as ProviderEventDTO;
}

test("regular event stamps its workflowRunId", () => {
  const next = stampRunActivity({}, baseEvent({ workflowRunId: "run-42" }), 1000);
  assert.equal(next["run-42"], 1000);
});

test("agent_graph_updated stamps the PARENT run id from the snapshot", () => {
  const e = baseEvent({
    type: "agent_graph_updated",
    workflowRunId: "run-1",
    agentGraphSnapshot: { parentRunId: "run-main", runs: [] },
  });
  const next = stampRunActivity({}, e, 2000);
  assert.equal(next["run-main"], 2000);
  assert.equal(next["run-1"], undefined);
});

test("agent_bus_message stamps the parent run id from the message", () => {
  const e = baseEvent({
    type: "agent_bus_message",
    agentBusMessage: { parentRunId: "run-main" },
  });
  const next = stampRunActivity({}, e, 3000);
  assert.equal(next["run-main"], 3000);
});

test("a later arrival replaces the earlier stamp for the same run", () => {
  const first = stampRunActivity({}, baseEvent({ workflowRunId: "run-1" }), 1000);
  const second = stampRunActivity(first, baseEvent({ workflowRunId: "run-1" }), 4000);
  assert.equal(second["run-1"], 4000);
});

test("staleness derivation flips exactly at the 30s threshold", () => {
  const lastAt = 100_000;
  assert.equal(runActivityKind(lastAt, lastAt + RUN_ACTIVITY_STALE_MS - 1), "live");
  assert.equal(runActivityKind(lastAt, lastAt + RUN_ACTIVITY_STALE_MS), "quiet");
});

test("terminal run statuses do not count toward flow liveness", () => {
  for (const terminal of ["completed", "failed", "cancelled"] as const) {
    assert.equal(activityCountsForStatus(terminal), false);
  }
  assert.equal(activityCountsForStatus("running"), true);
  assert.equal(activityCountsForStatus("blocked"), true);
});

test("rollup takes the freshest stamp across the run family", () => {
  const byRun = { "run-main": 1000, "run-leg-a": 9000, "run-leg-b": 5000 };
  assert.equal(lastActivityAt(byRun, ["run-main", "run-leg-a", "run-leg-b"]), 9000);
  assert.equal(lastActivityAt(byRun, ["run-missing"]), undefined);
});

test("eventActivityRunId falls back to workflowRunId for non-orchestration types", () => {
  assert.equal(eventActivityRunId(baseEvent({ type: "tool_started", toolName: "bash" })), "run-1");
});

test("formatActivityAge renders coarse seconds/minutes/hours", () => {
  assert.equal(formatActivityAge(4000), "4s");
  assert.equal(formatActivityAge(3 * 60_000), "3m");
  assert.equal(formatActivityAge(2 * 3_600_000), "2h");
  assert.equal(formatActivityAge(-500), "0s");
});
