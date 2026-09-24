import test from "node:test";
import assert from "node:assert/strict";
import { attentionQueue, type AttentionItem } from "./attentionQueue";
import { useStore } from "./store";
import { MockRunnerClient } from "../client/MockRunnerClient";
import { RunnerApiError } from "../client/HttpWsRunnerClient";
import { decisionControlModel, WORKTREE_MODES } from "../components/DecisionControls";
import type { DecisionPayload, RunRealtimeProjection } from "@/types/contract";

// Task-435: worktree merge card — per-action consequence descriptions and the
// 409 requiresConfirm → in-card confirm flow (resends with confirm:true).
// Additive-only.

function decision(partial: Partial<DecisionPayload> & { id: string; runId: string; kind: DecisionPayload["kind"] }): DecisionPayload {
  return { version: 1, revision: "1", status: "pending", actionable: true, ...partial };
}

function item(partial: Partial<AttentionItem>): AttentionItem {
  return {
    runId: "r1",
    chatId: "c1",
    projectId: "p-1",
    runTitle: "run",
    kind: "worktree_merge",
    waitingSince: "2026-01-01T00:00:00Z",
    ...partial,
  };
}

function lane(runId: string, decisions: DecisionPayload[]): RunRealtimeProjection {
  return {
    runId,
    projectId: "p-1",
    chatId: "chat-" + runId,
    revision: 1,
    status: "completed",
    updatedAt: "2026-01-01T00:00:00Z",
    decisions,
  };
}

function resetQueue() {
  attentionQueue.reconcileRunSnapshot([]);
  attentionQueue.ingestHistory([], "p-1");
}

function worktreeDecision(runId: string): DecisionPayload {
  return decision({
    id: "worktree_merge:" + runId,
    runId,
    kind: "worktree_merge",
    worktree: { branch: "fp/" + runId },
  });
}

test("worktree modes carry consequence descriptions", () => {
  const model = decisionControlModel(item({ decision: worktreeDecision("r1") }));
  assert.equal(model?.type, "worktree");
  assert.deepEqual(WORKTREE_MODES.map((m) => m.value), ["apply_patch", "keep_branch", "discard"]);
  for (const m of WORKTREE_MODES) assert.ok(m.description && m.description.length > 0, `${m.value} needs a description`);
  // Destructive modes must say so — keep_branch keeps only committed work.
  assert.match(WORKTREE_MODES[1].description!, /committed/i);
  assert.match(WORKTREE_MODES[2].description!, /thrown away|deleted/i);
});

test("worktree 409 requiresConfirm flips card to confirm state", async () => {
  resetQueue();
  const d = worktreeDecision("rW");
  attentionQueue.applyRunUpdate({ kind: "upsert", run: lane("rW", [d]) });
  assert.ok(attentionQueue.items.find((i) => i.runId === "rW"), "item must exist before submit");

  const client = new MockRunnerClient();
  client.resolveWorktreeMergeError = () =>
    new RunnerApiError(409, "worktree_keep_branch_confirm", "worktree has uncommitted changes", undefined, {
      requiresConfirm: true,
      uncommitted: ["WT_MARKER.txt", "notes.md"],
    });
  useStore.setState({ client });

  const ok = await useStore.getState().submitAttentionDecision("rW", d, "keep_branch");
  assert.equal(ok, true, "requiresConfirm is awaiting-user, not a failure");
  assert.equal(client.resolvedWorktrees.length, 0, "nothing resolved yet");

  const it = attentionQueue.items.find((i) => i.runId === "rW");
  assert.deepEqual(it?.worktreeConfirm, {
    mode: "keep_branch",
    files: ["WT_MARKER.txt", "notes.md"],
    message: "worktree has uncommitted changes",
  });
  const model = it && decisionControlModel(it);
  assert.equal(model?.type, "worktree_confirm");
  if (model?.type === "worktree_confirm") {
    assert.equal(model.modeLabel, "Keep branch");
    assert.deepEqual(model.files, ["WT_MARKER.txt", "notes.md"]);
  }
  resetQueue();
});

test("worktree confirm resends with confirm:true and evicts", async () => {
  resetQueue();
  const d = worktreeDecision("rC");
  attentionQueue.applyRunUpdate({ kind: "upsert", run: lane("rC", [d]) });

  const client = new MockRunnerClient();
  useStore.setState({ client });
  const ok = await useStore.getState().submitAttentionDecision("rC", d, "keep_branch", undefined, true);
  assert.equal(ok, true);
  assert.deepEqual(client.resolvedWorktrees, [{ runId: "rC", mode: "keep_branch", confirm: true }]);
  // The server then emits a lane without the resolved decision — the item
  // drops and the confirm state is gone with it.
  attentionQueue.applyRunUpdate({ kind: "upsert", run: { ...lane("rC", []), revision: 2 } });
  assert.equal(attentionQueue.items.find((i) => i.runId === "rC"), undefined, "resolved item evicted");
  resetQueue();
});

test("dismissWorktreeConfirm restores mode buttons", () => {
  resetQueue();
  const d = worktreeDecision("rD");
  attentionQueue.applyRunUpdate({ kind: "upsert", run: lane("rD", [d]) });
  attentionQueue.setWorktreeConfirm("rD", { mode: "discard", files: ["a.txt"], message: "n files" });
  assert.equal(decisionControlModel(attentionQueue.items.find((i) => i.runId === "rD")!)?.type, "worktree_confirm");

  useStore.getState().dismissWorktreeConfirm("rD");
  const it = attentionQueue.items.find((i) => i.runId === "rD");
  assert.equal(it?.worktreeConfirm, undefined);
  assert.equal(decisionControlModel(it!)?.type, "worktree");
  resetQueue();
});

test("worktree non-confirm non-conflict 409 still fails", async () => {
  resetQueue();
  const d = worktreeDecision("rX");
  const client = new MockRunnerClient();
  client.resolveWorktreeMergeError = () =>
    new RunnerApiError(409, "worktree_lost", "worktree is missing", undefined, {});
  useStore.setState({ client });

  const ok = await useStore.getState().submitAttentionDecision("rX", d, "apply_patch");
  assert.equal(ok, false, "an unrelated 409 is an error, not a confirm/conflict request");
  assert.equal(attentionQueue.items.find((i) => i.runId === "rX")?.worktreeConfirm, undefined);
  resetQueue();
});

// ---- Task-436: conflict card ----------------------------------------------

test("worktree conflict 409 flips card to conflict state", async () => {
  resetQueue();
  const d = worktreeDecision("rK");
  attentionQueue.applyRunUpdate({ kind: "upsert", run: lane("rK", [d]) });

  const client = new MockRunnerClient();
  client.resolveWorktreeMergeError = () =>
    new RunnerApiError(409, "worktree_merge_conflict", "patch does not apply cleanly", undefined, {
      conflict: true,
      conflictPaths: ["CONFLICT.txt", "b.ts"],
      patchArtifactRef: "C:/ws/.flowpilot/worktrees/cht_x.patch",
    });
  useStore.setState({ client });

  const ok = await useStore.getState().submitAttentionDecision("rK", d, "apply_patch");
  assert.equal(ok, true, "conflict parks the card for fix+retry, not a failure");
  const it = attentionQueue.items.find((i) => i.runId === "rK");
  assert.deepEqual(it?.worktreeConflict, {
    mode: "apply_patch",
    conflictPaths: ["CONFLICT.txt", "b.ts"],
    patchRef: "C:/ws/.flowpilot/worktrees/cht_x.patch",
    message: "patch does not apply cleanly",
  });
  const model = it && decisionControlModel(it);
  assert.equal(model?.type, "worktree_conflict");
  if (model?.type === "worktree_conflict") {
    assert.equal(model.modeLabel, "Apply patch");
    assert.deepEqual(model.conflictPaths, ["CONFLICT.txt", "b.ts"]);
  }
  resetQueue();
});

test("worktree conflict retry resends same mode", async () => {
  resetQueue();
  const d = worktreeDecision("rR");
  attentionQueue.applyRunUpdate({ kind: "upsert", run: lane("rR", [d]) });

  const client = new MockRunnerClient();
  useStore.setState({ client });
  attentionQueue.setWorktreeConflict("rR", {
    mode: "apply_patch", conflictPaths: ["a.ts"], patchRef: "p.patch", message: "m",
  });
  // Retry = the identical call (no confirm flag); success evicts the item.
  const ok = await useStore.getState().submitAttentionDecision("rR", d, "apply_patch");
  assert.equal(ok, true);
  assert.deepEqual(client.resolvedWorktrees, [{ runId: "rR", mode: "apply_patch", confirm: false }]);
  resetQueue();
});

test("dismissWorktreeConflict restores mode buttons", () => {
  resetQueue();
  const d = worktreeDecision("rZ");
  attentionQueue.applyRunUpdate({ kind: "upsert", run: lane("rZ", [d]) });
  attentionQueue.setWorktreeConflict("rZ", {
    mode: "apply_patch", conflictPaths: ["a.ts"], patchRef: "", message: "m",
  });
  assert.equal(decisionControlModel(attentionQueue.items.find((i) => i.runId === "rZ")!)?.type, "worktree_conflict");

  useStore.getState().dismissWorktreeConflict("rZ");
  const it = attentionQueue.items.find((i) => i.runId === "rZ");
  assert.equal(it?.worktreeConflict, undefined);
  assert.equal(decisionControlModel(it!)?.type, "worktree");
  resetQueue();
});

test("worktree conflict detected on wire shape (no code field)", async () => {
  resetQueue();
  const d = worktreeDecision("rW");
  attentionQueue.applyRunUpdate({ kind: "upsert", run: lane("rW", [d]) });

  const client = new MockRunnerClient();
  // handleWorktreeResolve writes the evidence map bare — no error{} wrapper,
  // no code. Detection must key off details.conflict.
  client.resolveWorktreeMergeError = () =>
    new RunnerApiError(409, "", "", undefined, {
      conflict: true,
      conflictPaths: ["CONFLICT.txt"],
      patchArtifactRef: "C:/wt/x.patch",
      reason: "patch does not apply cleanly",
    });
  useStore.setState({ client });

  const ok = await useStore.getState().submitAttentionDecision("rW", d, "apply_patch");
  assert.equal(ok, true);
  const it = attentionQueue.items.find((i) => i.runId === "rW");
  assert.equal(it?.worktreeConflict?.conflictPaths?.[0], "CONFLICT.txt");
  assert.equal(it?.worktreeConflict?.message, "patch does not apply cleanly");
  resetQueue();
});

test("confirm and conflict states are mutually exclusive", () => {
  resetQueue();
  const d = worktreeDecision("rM");
  attentionQueue.applyRunUpdate({ kind: "upsert", run: lane("rM", [d]) });
  attentionQueue.setWorktreeConfirm("rM", { mode: "keep_branch", files: ["x"], message: "m" });
  attentionQueue.setWorktreeConflict("rM", { mode: "apply_patch", conflictPaths: ["y"], patchRef: "", message: "m" });
  const it = attentionQueue.items.find((i) => i.runId === "rM");
  assert.equal(it?.worktreeConfirm, undefined, "conflict supersedes confirm");
  assert.equal(decisionControlModel(it!)?.type, "worktree_conflict");
  attentionQueue.setWorktreeConfirm("rM", { mode: "discard", files: ["x"], message: "m" });
  const it2 = attentionQueue.items.find((i) => i.runId === "rM");
  assert.equal(it2?.worktreeConflict, undefined, "confirm supersedes conflict");
  assert.equal(decisionControlModel(it2!)?.type, "worktree_confirm");
  resetQueue();
});
