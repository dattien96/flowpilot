import test from "node:test";
import assert from "node:assert/strict";

import { useStore } from "./store";
import { RunnerApiError } from "../client/HttpWsRunnerClient";
import type { RunStatus } from "../types/contract";

const snapshot = (status: RunStatus) => ({
  timeline: [{ kind: "thinking" as const, id: `thinking-${status}`, text: "Thinking..." }],
  artifacts: [],
  status,
  pendingApprovals: [],
  pendingQuestions: [],
  recoverable: true,
});

test("Stop from a child-focused flow reconciles every saved run snapshot to the terminal graph", async () => {
  const calls: string[] = [];
  const current = useStore.getState();
  const client = {
    ...current.client,
    stopAgentLoop: async (runId: string) => {
      calls.push(`loop:${runId}`);
      return {
        parentRunId: runId,
        runs: [
          { runId: "child-cancelled", agentName: "coder", role: "coder", status: "cancelled" as const, parentRunId: runId, createdAt: "2026-07-20T00:00:00Z" },
          { runId: "child-completed", agentName: "reviewer", role: "reviewer", status: "completed" as const, parentRunId: runId, createdAt: "2026-07-20T00:00:00Z" },
          { runId: "child-failed", agentName: "reviewer", role: "reviewer", status: "failed" as const, parentRunId: runId, createdAt: "2026-07-20T00:00:00Z" },
          // Models the asynchronous finishTurn window: the loop is stopped but
          // a stale graph can still report a child as active for one event.
          { runId: "child-stale-running", agentName: "reviewer", role: "reviewer", status: "running" as const, parentRunId: runId, createdAt: "2026-07-20T00:00:00Z" },
          { runId: "child-stale-approval", agentName: "reviewer", role: "reviewer", status: "waiting_approval" as const, parentRunId: runId, createdAt: "2026-07-20T00:00:00Z" },
          { runId: "child-stale-question", agentName: "reviewer", role: "reviewer", status: "waiting_question" as const, parentRunId: runId, createdAt: "2026-07-20T00:00:00Z" },
        ],
        edges: [],
        busMessages: [],
        loopState: { status: "stopped" as const, round: 1, roundCap: 3 },
      };
    },
    interrupt: async (runId: string) => {
      calls.push(`interrupt:${runId}`);
    },
  };

  useStore.setState({
    client,
    chatMode: "normal_chat",
    runId: "child-cancelled",
    mainRunId: "parent-run",
    activeAgentRunId: "child-cancelled",
    status: "running",
    timeline: [{ kind: "thinking", id: "focused-child-thinking", text: "Thinking..." }],
    agentRuns: [{ runId: "child-cancelled", agentName: "coder", role: "coder", status: "running", parentRunId: "parent-run", createdAt: "2026-07-20T00:00:00Z" }],
    agentGraphSnapshot: {
      parentRunId: "parent-run",
      runs: [{ runId: "child-cancelled", agentName: "coder", role: "coder", status: "running", parentRunId: "parent-run", createdAt: "2026-07-20T00:00:00Z" }],
      edges: [],
      busMessages: [],
      loopState: { status: "running", round: 1, roundCap: 3 },
    },
    _runSnapshots: {
      "parent-run": snapshot("running"),
      "child-cancelled": snapshot("running"),
      "child-completed": snapshot("completed"),
      "child-failed": snapshot("failed"),
      "child-stale-running": snapshot("running"),
      "child-stale-approval": snapshot("waiting_approval"),
      "child-stale-question": snapshot("waiting_question"),
    },
  });

  await useStore.getState().stop();

  const state = useStore.getState();
  assert.equal(state.status, "cancelled");
  assert.equal(state._runSnapshots["parent-run"]?.status, "cancelled");
  assert.equal(state._runSnapshots["child-cancelled"]?.status, "cancelled");
  assert.equal(state._runSnapshots["child-completed"]?.status, "completed");
  assert.equal(state._runSnapshots["child-failed"]?.status, "failed");
  assert.equal(state._runSnapshots["child-stale-running"]?.status, "cancelled");
  assert.equal(state._runSnapshots["child-stale-approval"]?.status, "cancelled");
  assert.equal(state._runSnapshots["child-stale-question"]?.status, "cancelled");
  assert.equal(state._runSnapshots["parent-run"]?.timeline.some((item) => item.kind === "thinking"), false);
  assert.equal(state._runSnapshots["child-cancelled"]?.recoverable, false);
  assert.deepEqual(calls, [
    "loop:parent-run",
    "interrupt:parent-run",
    "interrupt:child-cancelled",
    "interrupt:child-completed",
    "interrupt:child-failed",
    "interrupt:child-stale-running",
    "interrupt:child-stale-approval",
    "interrupt:child-stale-question",
  ]);
});

test("Stop reconciles saved snapshots when the runner returns a stopped graph in a durable-checkpoint error", async () => {
  const current = useStore.getState();
  const embeddedSnapshot = {
    parentRunId: "parent-run",
    runs: [{ runId: "child-run", agentName: "coder", role: "coder", status: "cancelled" as const, parentRunId: "parent-run", createdAt: "2026-07-20T00:00:00Z" }],
    edges: [],
    busMessages: [],
    loopState: { status: "stopped" as const, round: 1, roundCap: 3 },
  };
  const client = {
    ...current.client,
    stopAgentLoop: async () => {
      throw new RunnerApiError(500, "persist_failed", "RAM stop succeeded but checkpoint failed", embeddedSnapshot);
    },
    interrupt: async () => {},
  };

  useStore.setState({
    client,
    chatMode: "normal_chat",
    runId: "child-run",
    mainRunId: "parent-run",
    activeAgentRunId: "child-run",
    status: "running",
    agentRuns: [{ runId: "child-run", agentName: "coder", role: "coder", status: "running", parentRunId: "parent-run", createdAt: "2026-07-20T00:00:00Z" }],
    agentGraphSnapshot: { ...embeddedSnapshot, loopState: { status: "running", round: 1, roundCap: 3 } },
    _runSnapshots: {
      "parent-run": snapshot("running"),
      "child-run": snapshot("running"),
    },
  });

  await useStore.getState().stop();

  const state = useStore.getState();
  assert.equal(state.status, "cancelled");
  assert.equal(state._runSnapshots["parent-run"]?.status, "cancelled");
  assert.equal(state._runSnapshots["child-run"]?.status, "cancelled");
  assert.equal(state.agentGraphSnapshot?.loopState.status, "stopped");
});
