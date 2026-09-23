import test from "node:test";
import assert from "node:assert/strict";
import { deriveAttentionItems } from "./attentionQueue";
import type { RunHistoryItem } from "../types/contract.js";

// Task-423: pending payload + optimistic eviction semantics on the attention
// derivation. attention_queue.test.ts stays untouched (additive-only).

function hist(runId: string, status = "waiting_approval"): RunHistoryItem {
  return {
    runId,
    projectId: "p1",
    providerKey: "codex",
    status,
    startedAt: "2026-02-14T00:00:00Z",
    updatedAt: "2026-02-14T01:00:00Z",
    lastPrompt: `do ${runId}`,
  } as RunHistoryItem;
}

test("deriveAttentionItems attaches pending approvals/questions from snapshot", () => {
  const items = deriveAttentionItems(
    [hist("r1")],
    {
      r1: {
        pendingApprovals: [{ approvalId: "a1", details: { decisions: [] } }],
        pendingQuestions: [{ questionId: "q1", prompt: "pick", options: [] }],
      },
    },
    "p1",
  );
  assert.equal(items.length, 1);
  assert.equal(items[0].kind, "approval");
  assert.equal(items[0].pending?.approvals.length, 1);
  assert.equal(items[0].pending?.questions.length, 1);
});

test("item without snapshot pending has no pending payload (Open-only)", () => {
  const items = deriveAttentionItems([hist("r1")], {}, "p1");
  assert.equal(items.length, 1);
  assert.equal(items[0].pending, undefined);
});

test("evicted run stays suppressed while status is still waiting", () => {
  const evicted = new Set(["r1"]);
  const items = deriveAttentionItems([hist("r1")], {}, "p1", evicted);
  assert.equal(items.length, 0);
  assert.equal(evicted.has("r1"), true);
});

test("evicted run clears suppression once status leaves waiting", () => {
  const evicted = new Set(["r1"]);
  const items = deriveAttentionItems([hist("r1", "running")], {}, "p1", evicted);
  assert.equal(items.length, 0);
  assert.equal(evicted.has("r1"), false);
  // A later wait on the same run surfaces again.
  const items2 = deriveAttentionItems([hist("r1", "waiting_question")], {}, "p1", evicted);
  assert.equal(items2.length, 1);
  assert.equal(items2[0].kind, "question");
});

test("empty-pending snapshot does not attach payload", () => {
  const items = deriveAttentionItems(
    [hist("r1")],
    { r1: { pendingApprovals: [], pendingQuestions: [] } },
    "p1",
  );
  assert.equal(items[0].pending, undefined);
});
