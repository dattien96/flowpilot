import test from "node:test";
import assert from "node:assert/strict";
import { filterVisibleHistory, isProjectSyncing } from "./navigatorHistory";
import type { RunHistoryItem } from "@/types/contract";

function makeItem(overrides: Partial<RunHistoryItem> = {}): RunHistoryItem {
  return {
    runId: "run-1",
    projectId: "project-1",
    providerKey: "codex",
    status: "completed",
    startedAt: "2026-06-19T10:00:00Z",
    updatedAt: "2026-06-19T10:01:00Z",
    ...overrides,
  };
}

test("isProjectSyncing returns true when any row in the project is syncing", () => {
  const history = [
    makeItem({ runId: "run-a", syncStatus: "syncing" }),
    makeItem({ runId: "run-b", projectId: "project-2" }),
  ];

  assert.equal(isProjectSyncing(history, "project-1"), true);
  assert.equal(isProjectSyncing(history, "project-2"), false);
});

test("isProjectSyncing ignores rows from other projects and non-syncing states", () => {
  const history = [
    makeItem({ runId: "run-a", syncStatus: "failed" }),
    makeItem({ runId: "run-b", projectId: "project-2", syncStatus: "syncing" }),
  ];

  assert.equal(isProjectSyncing(history, "project-1"), false);
});

test("filterVisibleHistory removes child agent runs from navigator history", () => {
  const history = [
    makeItem({ runId: "main-run" }),
    makeItem({ runId: "child-run", parentRunId: "main-run", agentName: "coder" }),
  ];

  assert.deepEqual(filterVisibleHistory(history).map((item) => item.runId), ["main-run"]);
});

test("filterVisibleHistory removes orphan agent rows with built-in child prompts", () => {
  const history = [
    makeItem({ runId: "main-run" }),
    makeItem({
      runId: "orphan-agent",
      agentName: "reviewer",
      role: "review",
      agentStatus: "completed",
      lastPrompt: "You are the reviewer sub-agent. Review the coder's diff.",
    }),
  ];

  assert.deepEqual(filterVisibleHistory(history).map((item) => item.runId), ["main-run"]);
});

test("filterVisibleHistory keeps main rows with agent-like metadata", () => {
  const history = [
    makeItem({ runId: "main-run", agentName: "main", agentStatus: "completed", lastPrompt: "Main agent prompt" }),
  ];

  assert.deepEqual(filterVisibleHistory(history).map((item) => item.runId), ["main-run"]);
});
