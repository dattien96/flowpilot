import test from "node:test";
import assert from "node:assert/strict";
import { deriveOrchestrationRunStatus } from "./store";
import type { AgentGraphSnapshot } from "../types/contract";

function stoppedSnap(): AgentGraphSnapshot {
  return {
    parentRunId: "run-1",
    runs: [
      {
        runId: "child-1",
        agentName: "reviewer",
        role: "reviewer",
        status: "cancelled",
        parentRunId: "run-1",
        createdAt: "2026-07-21T00:00:00Z",
      },
    ],
    edges: [],
    busMessages: [],
    loopState: { status: "stopped", round: 0, roundCap: 3, gateReason: "stopped" },
  };
}

// BUG-308 residual: after Stop, loop stays "stopped" while plain-chat follow-up
// completes. Header/history must show Completed, not stay Cancelled.

test("stopped loop preserves completed after post-Stop chat follow-up", () => {
  assert.equal(deriveOrchestrationRunStatus("completed", stoppedSnap()), "completed");
});

test("stopped loop preserves failed after post-Stop chat follow-up", () => {
  assert.equal(deriveOrchestrationRunStatus("failed", stoppedSnap()), "failed");
});

test("stopped loop still forces cancelled when chat has not continued (BUG-248)", () => {
  assert.equal(deriveOrchestrationRunStatus("running", stoppedSnap()), "cancelled");
  assert.equal(deriveOrchestrationRunStatus("cancelled", stoppedSnap()), "cancelled");
  assert.equal(deriveOrchestrationRunStatus("idle", stoppedSnap()), "cancelled");
});
