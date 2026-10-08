// Dispatch-attention inbox hygiene: "settle_pending" is self-clearing engine
// bookkeeping (a terminal turn's durable settlement draining), not a human
// decision. It must not re-label a waiting run as dispatch_attention nor add
// inbox rows; genuinely actionable kinds (uncertain / repair_required /
// cancel_required) still surface.
import test from "node:test";
import assert from "node:assert/strict";
import { attentionQueue } from "./attentionQueue";
import type { DecisionPayload, RunHistoryItem, RunRealtimeProjection } from "@/types/contract";

function hist(runId: string, status = "waiting_question"): RunHistoryItem {
  return {
    projectId: "p-1",
    providerKey: "devin",
    status,
    startedAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    runKind: "workflow",
    lastPrompt: "run " + runId,
    runId,
  } as RunHistoryItem;
}

function dispatchDecision(kind: string): DecisionPayload {
  return {
    id: `dispatch_attention:${kind}`,
    runId: "r-mux",
    kind: "dispatch_attention",
    actionable: false,
    dispatch: { attentionKind: kind, reason: `${kind} bookkeeping` },
  } as unknown as DecisionPayload;
}

function lane(runId: string, decisions: DecisionPayload[]): RunRealtimeProjection {
  return {
    runId,
    projectId: "p-1",
    chatId: "chat-" + runId,
    revision: 1,
    status: "waiting_question",
    updatedAt: "2026-01-01T00:00:00Z",
    decisions,
  };
}

function reset() {
  attentionQueue.reconcileRunSnapshot([]);
  attentionQueue.ingestHistory([], "p-1");
  attentionQueue.ingestDispatch("r-poll", []);
}

test("ingestDispatch: settle_pending alone does not mark a run dispatch_attention", () => {
  reset();
  attentionQueue.ingestHistory([hist("r-poll")], "p-1");
  attentionQueue.ingestDispatch("r-poll", [{ kind: "settle_pending" }]);
  const item = attentionQueue.items.find((i) => i.runId === "r-poll");
  assert.ok(item, "waiting run still lists");
  assert.equal(item.kind, "question", "kind falls back to the real wait, not dispatch noise");
});

test("ingestDispatch: actionable kinds still surface as dispatch_attention", () => {
  reset();
  attentionQueue.ingestHistory([hist("r-poll")], "p-1");
  attentionQueue.ingestDispatch("r-poll", [{ kind: "settle_pending" }, { kind: "uncertain" }]);
  const item = attentionQueue.items.find((i) => i.runId === "r-poll");
  assert.ok(item);
  assert.equal(item.kind, "dispatch_attention");
});

test("mux lane: settle_pending decision does not flip the kind", () => {
  reset();
  attentionQueue.ingestHistory([hist("r-mux")], "p-1");
  attentionQueue.reconcileRunSnapshot([lane("r-mux", [dispatchDecision("settle_pending")])]);
  const item = attentionQueue.items.find((i) => i.runId === "r-mux");
  assert.ok(item);
  assert.equal(item.kind, "question");
});

test("mux lane: repair_required still labels dispatch_attention", () => {
  reset();
  attentionQueue.ingestHistory([hist("r-mux")], "p-1");
  attentionQueue.reconcileRunSnapshot([lane("r-mux", [dispatchDecision("repair_required")])]);
  const item = attentionQueue.items.find((i) => i.runId === "r-mux");
  assert.ok(item);
  assert.equal(item.kind, "dispatch_attention");
});
