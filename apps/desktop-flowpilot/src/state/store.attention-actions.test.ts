import test from "node:test";
import assert from "node:assert/strict";
import { useStore } from "./store";
import { attentionQueue } from "./attentionQueue";
import type { RunHistoryItem, RunnerClient } from "../types/contract.js";

// Task-423: run-scoped inbox actions — direct client RPC + queue update,
// never touching the focused run's slots (the R1-critical invariant).
// attentionQueue is a singleton — tests use unique runIds to avoid
// cross-test suppression from the evicted set.

function clientWith(spies: {
  approvals?: [string, string, boolean | undefined][];
  questions?: [string, string | string[]][];
  failApprovals?: boolean;
}): RunnerClient {
  return {
    submitApproval: async (id: string, decision: string, remember?: boolean) => {
      if (spies.failApprovals) throw new Error("HTTP 409: already resolved");
      spies.approvals?.push([id, decision, remember]);
    },
    answerQuestion: async (id: string, choice: string | string[]) => {
      spies.questions?.push([id, choice]);
    },
  } as unknown as RunnerClient;
}

function hist(runId: string, projectId = "pB", status = "waiting_approval"): RunHistoryItem {
  return {
    runId,
    projectId,
    providerKey: "codex",
    status,
    startedAt: "2026-02-14T00:00:00Z",
    updatedAt: "2026-02-14T01:00:00Z",
    lastPrompt: `task ${runId}`,
  } as RunHistoryItem;
}

function seedAttention(runId: string, snap?: Parameters<typeof attentionQueue.ingestSnapshot>[1]): void {
  attentionQueue.ingestHistory([hist(runId)], "pB");
  if (snap) attentionQueue.ingestSnapshot(runId, snap);
}

test("approveAttentionItem calls client.submitApproval and evicts item", async () => {
  const calls: [string, string, boolean | undefined][] = [];
  useStore.setState({ client: clientWith({ approvals: calls }) });
  seedAttention("ta-1", { pendingApprovals: [{ approvalId: "ap-1", details: { decisions: [] } }] });
  assert.equal(attentionQueue.items.some((i) => i.runId === "ta-1"), true);

  const ok = await useStore.getState().approveAttentionItem("ta-1", "ap-1", "approved");
  assert.equal(ok, true);
  assert.deepEqual(calls, [["ap-1", "approved", undefined]]);
  assert.equal(attentionQueue.items.some((i) => i.runId === "ta-1"), false);
});

test("approveAttentionItem does not touch focused pendingApprovals or status", async () => {
  useStore.setState({
    client: clientWith({}),
    runId: "focused-run",
    status: "running",
    pendingApprovals: [{ approvalId: "focused-ap", details: { decisions: [] } }],
    pendingQuestions: [{ questionId: "focused-q", prompt: "?", options: [] }],
    timeline: [{ kind: "system", id: "sys-1", text: "focused", tone: "info" }],
  });
  seedAttention("ta-2", { pendingApprovals: [{ approvalId: "ap-2", details: { decisions: [] } }] });

  await useStore.getState().approveAttentionItem("ta-2", "ap-2", "approved");
  const s = useStore.getState();
  assert.equal(s.runId, "focused-run");
  assert.equal(s.status, "running");
  assert.equal(s.pendingApprovals.length, 1);
  assert.equal(s.pendingApprovals[0].approvalId, "focused-ap");
  assert.equal(s.pendingQuestions.length, 1);
  assert.equal(s.timeline.length, 1);
});

test("answerAttentionItem posts choice and evicts item", async () => {
  const calls: [string, string | string[]][] = [];
  useStore.setState({ client: clientWith({ questions: calls }) });
  seedAttention("ta-3", {
    pendingQuestions: [{ questionId: "q-3", prompt: "which?", options: [{ label: "A" }, { label: "B" }] }],
  });
  const ok = await useStore.getState().answerAttentionItem("ta-3", "q-3", "B");
  assert.equal(ok, true);
  assert.deepEqual(calls, [["q-3", "B"]]);
  assert.equal(attentionQueue.items.some((i) => i.runId === "ta-3"), false);
});

test("failed attention action keeps item and returns false", async () => {
  useStore.setState({ client: clientWith({ failApprovals: true }) });
  seedAttention("ta-4", { pendingApprovals: [{ approvalId: "ap-4", details: { decisions: [] } }] });
  const ok = await useStore.getState().approveAttentionItem("ta-4", "ap-4", "approved");
  assert.equal(ok, false);
  const item = attentionQueue.items.find((i) => i.runId === "ta-4");
  assert.ok(item, "item must stay in the queue on failure");
  assert.equal(item.pending?.approvals.length, 1);
});

test("approving one of two pending approvals keeps run item until all resolved", async () => {
  const calls: [string, string, boolean | undefined][] = [];
  useStore.setState({ client: clientWith({ approvals: calls }) });
  seedAttention("ta-5", {
    pendingApprovals: [
      { approvalId: "ap-5a", details: { decisions: [] } },
      { approvalId: "ap-5b", details: { decisions: [] } },
    ],
  });

  // First approval resolves — run still waits on ap-5b, item stays with a
  // truthful shrunken payload.
  assert.equal(await useStore.getState().approveAttentionItem("ta-5", "ap-5a", "approved"), true);
  let item = attentionQueue.items.find((i) => i.runId === "ta-5");
  assert.ok(item, "item must stay while another approval is pending");
  assert.deepEqual(item.pending?.approvals.map((a) => a.approvalId), ["ap-5b"]);

  // Last approval resolves — item evicted.
  assert.equal(await useStore.getState().approveAttentionItem("ta-5", "ap-5b", "approved"), true);
  assert.equal(attentionQueue.items.some((i) => i.runId === "ta-5"), false);
});
