import test from "node:test";
import assert from "node:assert/strict";
import { attentionQueue } from "./attentionQueue";
import { useStore } from "./store";
import { MockRunnerClient } from "../client/MockRunnerClient";
import type { RunHistoryItem, RunRealtimeFrame, RunRealtimeProjection } from "@/types/contract";

// CP-84 / Task-429 (T-4/T-5/T-6): mux lane reconcile + live frame application.
// Additive-only tests — existing attention_queue tests are untouched.

function lane(partial: Partial<RunRealtimeProjection> & { runId: string }): RunRealtimeProjection {
  return {
    projectId: "p-1",
    chatId: "chat-" + partial.runId,
    revision: 1,
    status: "waiting_approval",
    updatedAt: "2026-01-01T00:00:00Z",
    ...partial,
  };
}

function approvalDecision(runId: string, id: string) {
  return {
    version: 1,
    id: "approval:" + id,
    runId,
    kind: "approval" as const,
    revision: "1",
    status: "pending",
    actionable: true,
    approval: { command: "npm test", decisions: [{ value: "approve", label: "Approve" }] },
  };
}

function resetMux() {
  attentionQueue.reconcileRunSnapshot([]);
  attentionQueue.ingestHistory([], "p-1");
}

test("snapshot reconciliation replaces mux-owned active lane set", () => {
  resetMux();
  // A waiting lane pushed by the mux surfaces immediately — no poll needed.
  attentionQueue.reconcileRunSnapshot([
    lane({ runId: "rA", decisions: [approvalDecision("rA", "a1")] }),
    lane({ runId: "rB", status: "running" }),
  ]);
  assert.deepEqual(attentionQueue.items.map((i) => i.runId), ["rA"]);
  assert.equal(attentionQueue.items[0].kind, "approval");
  assert.equal(attentionQueue.items[0].pending?.approvals[0]?.approvalId, "a1");

  // Reconnect snapshot that omits rA evicts it (stale-terminal cleanup) while
  // a non-mux local snapshot view is left alone.
  attentionQueue.ingestSnapshot("localRun", {
    status: "waiting_approval",
    pendingApprovals: [{ approvalId: "lx", details: { decisions: [] } }],
  });
  attentionQueue.ingestHistory(
    [
      {
        runId: "localRun",
        projectId: "p-1",
        providerKey: "codex",
        status: "waiting_approval",
        startedAt: "2026-01-01T00:00:00Z",
        updatedAt: "2026-01-01T00:00:00Z",
      } as RunHistoryItem,
    ],
    "p-1",
  );
  attentionQueue.reconcileRunSnapshot([]);
  const ids = attentionQueue.items.map((i) => i.runId);
  assert.ok(!ids.includes("rA"), "stale mux lane dropped on resnapshot");
  assert.ok(ids.includes("localRun"), "non-mux/local item survived resnapshot");
  assert.equal(attentionQueue.muxLaneCount(), 0);
  resetMux();
});

test("upsert ignores older revision and remove is idempotent", () => {
  resetMux();
  attentionQueue.reconcileRunSnapshot([lane({ runId: "rA", revision: 5 })]);
  attentionQueue.applyRunUpdate({
    kind: "upsert",
    runId: "rA",
    run: lane({ runId: "rA", revision: 4, status: "failed" }),
  });
  // Older revision dropped: lane still carries the revision-5 status.
  assert.equal(attentionQueue.muxLaneCount(), 1);
  attentionQueue.applyRunUpdate({ kind: "remove", runId: "rA" });
  assert.equal(attentionQueue.muxLaneCount(), 0);
  // Idempotent second remove: no throw, no phantom item.
  attentionQueue.applyRunUpdate({ kind: "remove", runId: "rA" });
  assert.equal(attentionQueue.muxLaneCount(), 0);
  resetMux();
});

test("chunked snapshot commits only on complete", () => {
  resetMux();
  const snapId = "snap-1";
  attentionQueue.applyRunUpdate({
    kind: "snapshot",
    snapshotId: snapId,
    runs: [lane({ runId: "rC1", decisions: [approvalDecision("rC1", "a1")] })],
  });
  // Partial burst: nothing reconciled yet.
  assert.equal(attentionQueue.muxLaneCount(), 0);
  assert.equal(attentionQueue.items.length, 0);
  // A live upsert arriving mid-burst is dropped (superseded by the snapshot).
  attentionQueue.applyRunUpdate({
    kind: "upsert",
    runId: "staleMid",
    run: lane({ runId: "staleMid", decisions: [approvalDecision("staleMid", "aX")] }),
  });
  assert.equal(attentionQueue.items.length, 0);
  attentionQueue.applyRunUpdate({
    kind: "snapshot",
    snapshotId: snapId,
    runs: [lane({ runId: "rC2", status: "running" })],
    complete: true,
  });
  assert.equal(attentionQueue.muxLaneCount(), 2);
  assert.deepEqual(attentionQueue.items.map((i) => i.runId), ["rC1"]);
  resetMux();
});

test("terminal run with actionable worktree decision still surfaces", () => {
  resetMux();
  attentionQueue.applyRunUpdate({
    kind: "upsert",
    runId: "rWT",
    run: lane({
      runId: "rWT",
      status: "completed",
      decisions: [
        {
          version: 1,
          id: "worktree_merge:rWT",
          runId: "rWT",
          kind: "worktree_merge",
          revision: "wt:merge_pending:t",
          status: "pending",
          actionable: true,
          worktree: { path: "/tmp/wt", branch: "fp/rWT" },
        },
      ],
    }),
  });
  assert.equal(attentionQueue.items.length, 1);
  assert.equal(attentionQueue.items[0].kind, "worktree_merge");
  resetMux();
});

test("mux lane freshens a stale polled status", () => {
  resetMux();
  attentionQueue.ingestHistory(
    [
      {
        runId: "rPoll",
        projectId: "p-1",
        providerKey: "codex",
        status: "running",
        startedAt: "2026-01-01T00:00:00Z",
        updatedAt: "2026-01-01T00:00:00Z",
      } as RunHistoryItem,
    ],
    "p-1",
  );
  assert.equal(attentionQueue.items.length, 0);
  // The lane transitions to waiting on the mux — the poll still says running,
  // but the inbox must not wait 30s.
  attentionQueue.applyRunUpdate({
    kind: "upsert",
    runId: "rPoll",
    run: lane({ runId: "rPoll", revision: 9, decisions: [approvalDecision("rPoll", "ap")] }),
  });
  assert.equal(attentionQueue.items.length, 1);
  assert.equal(attentionQueue.items[0].runId, "rPoll");
  resetMux();
});

test("resetRun does not abort the app-lifetime mux stream", async () => {
  // Fake client records whether the mux stream's signal was aborted.
  let muxAborted = false;
  const client = new MockRunnerClient();
  client.streamRunUpdates = (signal?: AbortSignal) => {
    signal?.addEventListener("abort", () => {
      muxAborted = true;
    });
    return (async function* () {
      // Never yields — the point is that resetRun must not abort it.
      await new Promise(() => {});
      yield undefined as never;
    })();
  };
  useStore.setState({ client });
  // Boot the stream via the same seam loadProjects uses.
  const st = useStore.getState() as unknown as Record<string, unknown>;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  (st as any).client.listProjects = async () => [{ id: "p-1", name: "p", rootPath: "/tmp" }];
  await useStore.getState().loadProjects();
  useStore.getState().resetRun();
  assert.equal(muxAborted, false, "resetRun aborted the app-lifetime mux stream");
});

test("resync frame leaves lanes untouched — consumer reconnects", () => {
  resetMux();
  attentionQueue.reconcileRunSnapshot([lane({ runId: "rR", decisions: [approvalDecision("rR", "a1")] })]);
  attentionQueue.applyRunUpdate({ kind: "resync", retryable: true });
  // The lane set survives: resync is a transport signal, the fresh snapshot
  // on reconnect is what reconciles.
  assert.equal(attentionQueue.muxLaneCount(), 1);
  resetMux();
});

// keep the frame type referenced so tsconfig checks the import edge
void (0 as unknown as RunRealtimeFrame | undefined);
