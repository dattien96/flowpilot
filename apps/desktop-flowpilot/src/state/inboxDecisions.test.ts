import test from "node:test";
import assert from "node:assert/strict";
import { attentionQueue, type AttentionItem } from "./attentionQueue";
import { useStore } from "./store";
import { MockRunnerClient } from "../client/MockRunnerClient";
import { RunnerApiError } from "../client/HttpWsRunnerClient";
import {
  batchChoiceFor,
  decisionControlModel,
  filterAttentionItems,
  isBatchEligible,
  isHeavyKind,
} from "../components/DecisionControls";
import type { DecisionPayload, ProviderAccountSummary, RunRealtimeProjection } from "@/types/contract";

// CP-84 / Task-431: inbox per-kind controls, preview, triage filters, batch
// approve, and ID-scoped submitAttentionDecision routing. Additive-only.

function decision(partial: Partial<DecisionPayload> & { id: string; runId: string; kind: DecisionPayload["kind"] }): DecisionPayload {
  return {
    version: 1,
    revision: "1",
    status: "pending",
    actionable: true,
    ...partial,
  };
}

function lane(runId: string, decisions: DecisionPayload[]): RunRealtimeProjection {
  return {
    runId,
    projectId: "p-1",
    chatId: "chat-" + runId,
    revision: 1,
    status: "waiting_approval",
    updatedAt: "2026-01-01T00:00:00Z",
    decisions,
  };
}

function item(partial: Partial<AttentionItem>): AttentionItem {
  return {
    runId: "r1",
    chatId: "c1",
    projectId: "p-1",
    runTitle: "run",
    kind: "decision",
    waitingSince: "2026-01-01T00:00:00Z",
    ...partial,
  };
}

function resetMux() {
  attentionQueue.reconcileRunSnapshot([]);
  attentionQueue.ingestHistory([], "p-1");
}

test("gate item renders radio + custom text + Fix/Suggest/Open", () => {
  const it = item({
    kind: "gate",
    decision: decision({ id: "gate:g1", runId: "r1", kind: "gate", gate: { options: ["keep-test-fix-code"], regressedTests: ["t_a"] } }),
  });
  const model = decisionControlModel(it);
  assert.equal(model?.type, "gate");
  if (model?.type === "gate") {
    assert.ok(model.allowCustom, "gate exposes a custom-text path");
    const labels = model.options.map((o) => o.label);
    assert.ok(labels.some((l) => /fix/i.test(l)), "Fix option present");
    assert.ok(labels.some((l) => /suggest/i.test(l)), "Suggest option present");
  }
  assert.equal(isHeavyKind(it), true, "gate collapses behind expand by default");
  assert.equal(isBatchEligible(it), false, "gate is never batch-eligible");
});

test("ss_lock item expand shows quickView; no project switch", async () => {
  resetMux();
  const d = decision({
    id: "ss_lock:s1",
    runId: "rSS",
    kind: "ss_lock",
    ssLock: { featureBase: "CP-99", quickView: "AI Quick View: adds auth gate" },
  });
  attentionQueue.reconcileRunSnapshot([lane("rSS", [d])]);
  const found = attentionQueue.items.find((i) => i.runId === "rSS");
  assert.ok(found, "ss_lock lane surfaces");
  assert.equal(found!.kind, "ss_lock");
  assert.equal(found!.decision?.ssLock?.quickView, "AI Quick View: adds auth gate");
  assert.equal(isHeavyKind(found!), true);
  assert.equal(decisionControlModel(found!)?.type, "ss_lock");

  // Submitting approve must not move focus/project.
  const client = new MockRunnerClient();
  useStore.setState({ client, selectedProjectId: "p-1", runId: "focused-other" });
  const ok = await useStore.getState().submitAttentionDecision("rSS", d, "approve");
  assert.equal(ok, true);
  assert.deepEqual(client.ssLockAnswers, [{ runId: "rSS", action: "approve", edits: undefined }]);
  assert.equal(useStore.getState().selectedProjectId, "p-1");
  assert.equal(useStore.getState().runId, "focused-other");
  resetMux();
});

test("worktree_merge item shows apply_patch/keep_branch/discard", () => {
  const it = item({
    kind: "worktree_merge",
    decision: decision({ id: "worktree_merge:w1", runId: "r1", kind: "worktree_merge", worktree: { branch: "fp/x", conflictPaths: ["a.ts"] } }),
  });
  const model = decisionControlModel(it);
  assert.equal(model?.type, "worktree");
  if (model?.type === "worktree") {
    assert.deepEqual(model.modes.map((m) => m.value), ["apply_patch", "keep_branch", "discard"]);
  }
});

test("quota item 'Switch to X' calls account-switch confirm with candidate id", async () => {
  const account = {
    id: "acct-cand-1",
    providerKey: "claude",
    label: "work@corp",
    isActive: false,
  } as unknown as ProviderAccountSummary;
  const client = new MockRunnerClient();
  useStore.setState({ client, providerAccounts: [account], pendingAccountSwitch: undefined });
  const d = decision({
    id: "quota:q1",
    runId: "rQ",
    kind: "quota",
    quota: { candidateAccountId: "acct-cand-1", candidateLabel: "work@corp", remainingPct: 42 },
  });
  const model = decisionControlModel(item({ decision: d }));
  assert.equal(model?.type, "quota");
  if (model?.type === "quota") assert.equal(model.candidateLabel, "work@corp");
  const ok = await useStore.getState().submitAttentionDecision("rQ", d, "switch");
  assert.equal(ok, true);
  // Routes to the existing account-switch confirm with the candidate id.
  assert.equal(useStore.getState().pendingAccountSwitch?.candidateAccount.id, "acct-cand-1");
  assert.equal(useStore.getState().pendingAccountSwitch?.reason, "usage_limit");
});

test("payload-absent item renders Open only", () => {
  const noDecision = item({ kind: "gate" });
  assert.equal(decisionControlModel(noDecision), null);
  const unknown = item({ decision: decision({ id: "dispatch_attention:x", runId: "r1", kind: "dispatch_attention" }) });
  assert.equal(decisionControlModel(unknown), null);
  const nonActionable = item({ decision: decision({ id: "gate:g", runId: "r1", kind: "gate", actionable: false }) });
  assert.equal(decisionControlModel(nonActionable), null);
});

test("batch approve submits eligible items only, reports partial failure", async () => {
  resetMux();
  const approvals: Array<{ id: string; decision: string }> = [];
  const client = new MockRunnerClient();
  client.submitApproval = async (approvalId: string, decision: string) => {
    if (approvalId === "a-bad") throw new RunnerApiError(409, "stale", "stale decision");
    approvals.push({ id: approvalId, decision });
  };
  useStore.setState({ client, selectedProjectId: "p-1" });

  attentionQueue.reconcileRunSnapshot([
    lane("rOK", [decision({ id: "approval:a-ok", runId: "rOK", kind: "approval" })]),
    lane("rBad", [decision({ id: "approval:a-bad", runId: "rBad", kind: "approval" })]),
    lane("rGate", [decision({ id: "gate:g", runId: "rGate", kind: "gate" })]),
    lane("rMS", [decision({
      id: "question:ms", runId: "rMS", kind: "question",
      question: { multiSelect: true, options: [{ label: "x" }, { label: "y" }] },
    })]),
  ]);

  const eligible = attentionQueue.items.filter(isBatchEligible);
  // approval + single-select question only — gate and multi-select excluded.
  assert.deepEqual(eligible.map((i) => i.runId).sort(), ["rBad", "rOK"]);

  // Sequential submit loop (the component's batchApprove shape).
  for (const it of eligible) {
    const choice = batchChoiceFor(it)!;
    await useStore.getState().submitAttentionDecision(it.runId, it.decision!, choice);
  }
  // rOK submitted; rBad failed and STAYS in the queue (T-4); rGate untouched.
  assert.deepEqual(approvals, [{ id: "a-ok", decision: "approved" }]);
  assert.ok(attentionQueue.items.some((i) => i.runId === "rBad"), "failed item retained");
  assert.ok(attentionQueue.items.some((i) => i.runId === "rGate"), "gate item untouched");
  assert.ok(useStore.getState().runToast?.text.includes("stale"), "partial failure surfaced as toast");
  resetMux();
});

test("kind filter and project filter narrow the list", () => {
  const items = [
    item({ runId: "rA", kind: "approval", projectId: "p-1" }),
    item({ runId: "rB", kind: "gate", projectId: "p-1" }),
    item({ runId: "rC", kind: "approval", projectId: "p-2" }),
  ];
  assert.deepEqual(filterAttentionItems(items, "approval", undefined).map((i) => i.runId), ["rA", "rC"]);
  assert.deepEqual(filterAttentionItems(items, undefined, "p-2").map((i) => i.runId), ["rC"]);
  assert.deepEqual(filterAttentionItems(items, "approval", "p-1").map((i) => i.runId), ["rA"]);
  assert.equal(filterAttentionItems(items, undefined, undefined).length, 3, "default show-all");
});

test("stale submit (409) shows toast, refreshes histories, keeps item", async () => {
  resetMux();
  const client = new MockRunnerClient();
  let historyCalls = 0;
  client.listRunHistory = async () => {
    historyCalls++;
    return [];
  };
  client.submitApproval = async () => {
    throw new RunnerApiError(409, "conflict", "decision already resolved");
  };
  useStore.setState({ client, selectedProjectId: "p-1", runToast: undefined });
  const d = decision({ id: "approval:a1", runId: "rStale", kind: "approval" });
  attentionQueue.reconcileRunSnapshot([lane("rStale", [d])]);
  assert.ok(attentionQueue.items.some((i) => i.runId === "rStale"));

  const ok = await useStore.getState().submitAttentionDecision("rStale", d, "approved");
  assert.equal(ok, false);
  assert.ok(useStore.getState().runToast, "toast set on stale submit");
  assert.ok(attentionQueue.items.some((i) => i.runId === "rStale"), "item kept");
  assert.equal(attentionQueue.isActing("rStale"), false, "acting cleared after failure");
  // refresh histories fired (loadRunHistory → listRunHistory).
  await new Promise((r) => setTimeout(r, 30));
  assert.ok(historyCalls > 0, "history refresh triggered");
  resetMux();
});

test("inline submit does not change selectedProjectId/runId", async () => {
  resetMux();
  const client = new MockRunnerClient();
  useStore.setState({ client, selectedProjectId: "p-1", runId: "focused-run" });
  const d = decision({ id: "approval:a9", runId: "rOther", kind: "approval" });
  attentionQueue.reconcileRunSnapshot([lane("rOther", [d])]);
  const ok = await useStore.getState().submitAttentionDecision("rOther", d, "approved");
  assert.equal(ok, true);
  assert.equal(useStore.getState().selectedProjectId, "p-1");
  assert.equal(useStore.getState().runId, "focused-run");
  assert.ok(!attentionQueue.items.some((i) => i.runId === "rOther"), "resolved item evicted");
  resetMux();
});
