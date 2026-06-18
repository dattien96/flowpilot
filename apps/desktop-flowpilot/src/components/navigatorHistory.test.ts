import test from "node:test";
import assert from "node:assert/strict";
import { isProjectSyncing } from "./navigatorHistory";
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
