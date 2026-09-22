import test from "node:test";
import assert from "node:assert/strict";
import { deriveAttentionItems, attentionQueue } from "./attentionQueue";
import { useStore } from "./store";
import type { RunHistoryItem } from "@/types/contract";

// Task-404: attention queue — pure derivation + singleton observer + store
// integration. Additive; no backend calls.

function hist(partial: Partial<RunHistoryItem> & { runId: string }): RunHistoryItem {
  return {
    projectId: "p-1",
    providerKey: "codex",
    status: "waiting_approval",
    startedAt: "2025-01-01T00:00:00Z",
    updatedAt: "2025-01-01T00:00:00Z",
    runKind: "chat",
    ...partial,
  } as RunHistoryItem;
}

test("one item per waiting run; non-waiting runs excluded", () => {
  const items = deriveAttentionItems(
    [
      hist({ runId: "r1" }),
      hist({ runId: "r2", status: "running" }),
      hist({ runId: "r3", status: "waiting_question" }),
      hist({ runId: "r4", status: "completed" }),
    ],
    {},
    "p-1",
  );
  assert.deepEqual(items.map((i) => i.runId), ["r1", "r3"]);
  assert.equal(items[0].kind, "approval");
  assert.equal(items[1].kind, "question");
});

test("status-to-kind mapping covers every waiting kind", () => {
  const items = deriveAttentionItems(
    [
      hist({ runId: "a", status: "waiting_approval" }),
      hist({ runId: "b", status: "waiting_user_approval" }),
      hist({ runId: "d", status: "waiting_question" }),
      hist({ runId: "e", status: "blocked" }),
    ],
    {},
    "p-1",
  );
  assert.deepEqual(
    items.map((i) => i.kind),
    ["approval", "approval", "question", "gate"],
  );
});

test("snapshot pending fields refine the kind (dispatch > approval > question)", () => {
  const items = deriveAttentionItems(
    [hist({ runId: "r1", status: "blocked" })],
    { r1: { dispatchAttention: [{ kind: "uncertain" }] } },
    "p-1",
  );
  assert.equal(items[0].kind, "dispatch_attention");
});

test("oldest waiting run sorts first", () => {
  const items = deriveAttentionItems(
    [
      hist({ runId: "new", updatedAt: "2025-01-02T00:00:00Z" }),
      hist({ runId: "old", updatedAt: "2025-01-01T00:00:00Z", status: "waiting_question" }),
    ],
    {},
    "p-1",
  );
  assert.deepEqual(items.map((i) => i.runId), ["old", "new"]);
});

test("items are scoped to the active project", () => {
  const items = deriveAttentionItems(
    [hist({ runId: "in", projectId: "p-1" }), hist({ runId: "out", projectId: "p-2" })],
    {},
    "p-1",
  );
  assert.deepEqual(items.map((i) => i.runId), ["in"]);
});

test("one queue item per run even with multiple pending fields", () => {
  const items = deriveAttentionItems(
    [hist({ runId: "r1" })],
    {
      r1: {
        pendingApprovals: [{ approvalId: "a1", details: {} as never }],
        pendingQuestions: [{ questionId: "q1", prompt: "pick", options: [] }],
      },
    },
    "p-1",
  );
  assert.equal(items.length, 1);
  assert.equal(items[0].kind, "approval");
});

test("singleton observer: ingestHistory recomputes items and notifies subscribers", () => {
  let notified = 0;
  const unsub = attentionQueue.subscribe(() => notified++);
  attentionQueue.ingestHistory([hist({ runId: "r1" })], "p-1");
  assert.equal(notified, 1);
  assert.equal(attentionQueue.items.length, 1);
  attentionQueue.ingestSnapshot("r1", { dispatchAttention: [{ kind: "uncertain" }] });
  assert.equal(attentionQueue.items[0].kind, "dispatch_attention");
  unsub();
  attentionQueue.ingestHistory([], "p-1");
});

test("openRunAtAttention reuses the existing history open path", async () => {
  let opened = "";
  const orig = useStore.getState().openHistoryRun;
  useStore.setState({
    openHistoryRun: (async (runId: string) => {
      opened = runId;
    }) as never,
  });
  await useStore.getState().openRunAtAttention("r-attn", "cht-attn");
  assert.equal(opened, "r-attn");
  useStore.setState({ openHistoryRun: orig });
});
