import test from "node:test";
import assert from "node:assert/strict";
import { filterVisibleHistory, isProjectSyncing, isSyncableRun } from "./navigatorHistory";
import type { RemoteChatSessionSummary, RunHistoryItem } from "@/types/contract";

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

test("isSyncableRun accepts a plain chat run that is not yet synced", () => {
  assert.equal(isSyncableRun(makeItem({ runKind: "chat" })), true);
});

test("isSyncableRun accepts a flow-engine (workflow) run -- Task-190 / CP-36 P-5", () => {
  assert.equal(isSyncableRun(makeItem({ runKind: "workflow" })), true);
  assert.equal(isSyncableRun(makeItem({ runKind: undefined })), true);
});

test("isSyncableRun rejects an already-synced or unavailable run", () => {
  assert.equal(isSyncableRun(makeItem({ runKind: "chat", syncStatus: "synced" })), false);
  assert.equal(isSyncableRun(makeItem({ runKind: "workflow", unavailableReason: "missing" })), false);
});

test("isSyncableRun rejects a child agent run even though children carry runKind chat", () => {
  assert.equal(isSyncableRun(makeItem({ runKind: "chat", parentRunId: "run-hub" })), false);
});

test("isSyncableRun permanently excludes an unsyncable run (BUG-311)", () => {
  // "unsyncable" is a permanent backend fact (no resumable session file will
  // ever exist for this run, e.g. cancelled before the provider wrote one) --
  // unlike "failed", which is expected to be retried on the next sync attempt.
  assert.equal(isSyncableRun(makeItem({ runKind: "chat", syncStatus: "unsyncable" })), false);
});

test("isSyncableRun reconciles a stale local syncStatus against the confirmed remote index", () => {
  // A local syncStatus flag can go stale (e.g. a background history poll wins a
  // race against a just-set "synced" flag and overwrites it with the pre-sync
  // snapshot it fetched). When the remote index already carries this exact
  // sourceMachineId/sourceRunId, treat it as synced regardless of the local flag.
  const item = makeItem({ runKind: "chat", syncStatus: "failed", sourceMachineId: "mch_abc", sourceRunId: "run-1" });
  const remoteChatSessions: RemoteChatSessionSummary[] = [
    { runId: "run-1", projectId: "project-1", providerKey: "codex", sourceMachineId: "mch_abc", sourceRunId: "run-1" },
  ];

  assert.equal(isSyncableRun(item), true, "without remote data, the stale local flag still marks it unsynced");
  assert.equal(isSyncableRun(item, remoteChatSessions), false, "the confirmed remote record corrects the stale flag");
});

test("isSyncableRun does not reconcile a run that has never actually synced", () => {
  // Before any sync attempt, sourceMachineId/sourceRunId are unset -- there is
  // nothing to match against the remote index, so a never-synced run must stay
  // syncable even when other unrelated runs already exist remotely.
  const item = makeItem({ runKind: "chat" });
  const remoteChatSessions: RemoteChatSessionSummary[] = [
    { runId: "run-other", projectId: "project-1", providerKey: "codex", sourceMachineId: "mch_abc", sourceRunId: "run-other" },
  ];

  assert.equal(isSyncableRun(item, remoteChatSessions), true);
});
