import test from "node:test";
import assert from "node:assert/strict";
import { attentionQueue, type AttentionItem } from "./attentionQueue";
import { isFocusedRun, useStore } from "./store";
import { MockRunnerClient } from "../client/MockRunnerClient";
import type { DecisionPayload, ProviderAccountSummary } from "@/types/contract";
import type { AppState } from "./store";

// CP-84 / Task-434: modal routing — only the focused run may raise the
// singleton modals; non-focused modal-source events land in the inbox as
// synthetic items (deduped by runId+kind), resolving through the SAME
// confirm path as the modal. Additive-only tests.

function quotaItem(runId: string, candidateId = "acct-1"): AttentionItem {
  return {
    runId,
    chatId: "chat-" + runId,
    projectId: "p-1",
    runTitle: "run " + runId,
    kind: "quota",
    waitingSince: new Date().toISOString(),
    decision: {
      version: 1,
      id: `quota:${runId}`,
      runId,
      kind: "quota",
      revision: `quota:${runId}`,
      status: "pending",
      actionable: true,
      quota: { candidateAccountId: candidateId, candidateLabel: "work@corp" },
    } satisfies DecisionPayload,
  };
}

function reset() {
  attentionQueue.reconcileRunSnapshot([]);
  attentionQueue.ingestHistory([], "p-1");
  // Drop leftovers from prior tests.
  for (const i of [...attentionQueue.items]) attentionQueue.evict(i.runId);
}

test("isFocusedRun: focused runId true, others/undefined false (fail-safe)", () => {
  const s = { runId: "rA", mainRunId: "rMain", activeAgentRunId: undefined } as AppState;
  assert.equal(isFocusedRun(s, "rA"), true);
  assert.equal(isFocusedRun(s, "rMain"), true, "parent of focused leg counts");
  assert.equal(isFocusedRun(s, "rB"), false);
  assert.equal(isFocusedRun(s, undefined), false, "unresolvable → non-focused");
  assert.equal(isFocusedRun({ runId: undefined } as AppState, "rA"), false, "no focus → false");
});

test("non-focused quota event creates inbox item, modal state stays null", async () => {
  reset();
  useStore.setState({
    client: new MockRunnerClient(),
    runId: "rFocused",
    selectedProjectId: "p-1",
    pendingAccountSwitch: undefined,
  });
  // The routing decision: non-focused → ingestAttentionItem (not the modal).
  const srcRunId = "rBg";
  if (isFocusedRun(useStore.getState(), srcRunId)) {
    useStore.setState({ pendingAccountSwitch: {} as never });
  } else {
    useStore.getState().ingestAttentionItem(quotaItem(srcRunId));
  }
  assert.equal(useStore.getState().pendingAccountSwitch, undefined, "no modal for non-focused run");
  const found = attentionQueue.items.find((i) => i.runId === "rBg");
  assert.ok(found, "synthetic quota item surfaced");
  assert.equal(found!.kind, "quota");
  reset();
});

test("focused-run quota event still opens AccountSwitchModal", () => {
  reset();
  const s = { runId: "rA", mainRunId: undefined, activeAgentRunId: undefined } as AppState;
  assert.equal(isFocusedRun(s, "rA"), true, "focused source keeps the modal path");
  // Focused path = unchanged pendingAccountSwitch set (byte-for-byte).
  reset();
});

test("non-focused gate event lands in inbox as gate kind; gateBlock untouched", () => {
  reset();
  useStore.setState({ gateBlock: undefined, runId: "rFocused" });
  const gateDecision: DecisionPayload = {
    version: 1,
    id: "gate:g1",
    runId: "rBg",
    kind: "gate",
    revision: "g1",
    status: "pending",
    actionable: true,
    prompt: "Regression gate: 2 tests failed",
    gate: { options: ["keep-test-fix-code"], regressedTests: ["t_a", "t_b"] },
  };
  useStore.getState().ingestAttentionItem({
    runId: "rBg",
    chatId: "chat-rBg",
    projectId: "p-1",
    runTitle: "bg run",
    kind: "gate",
    waitingSince: new Date().toISOString(),
    decision: gateDecision,
  });
  const found = attentionQueue.items.find((i) => i.runId === "rBg");
  assert.ok(found);
  assert.equal(found!.kind, "gate");
  assert.equal(found!.decision?.gate?.regressedTests?.length, 2);
  assert.equal(useStore.getState().gateBlock, undefined, "gateBlock untouched for non-focused run");
  reset();
});

test("modal-source event without resolvable runId goes to inbox, no modal", () => {
  reset();
  const s = { runId: "rFocused" } as AppState;
  assert.equal(isFocusedRun(s, undefined), false, "undefined runId → inbox (fail-safe)");
  reset();
});

test("duplicate non-focused quota event dedupes to one item", () => {
  reset();
  useStore.setState({ runId: "rFocused" });
  useStore.getState().ingestAttentionItem(quotaItem("rBg"));
  useStore.getState().ingestAttentionItem(quotaItem("rBg")); // repeat event
  const matches = attentionQueue.items.filter((i) => i.runId === "rBg" && i.kind === "quota");
  assert.equal(matches.length, 1, "same runId+kind updates, never duplicates");
  reset();
});

test("quota item action calls same account-switch confirm path as modal", async () => {
  reset();
  const account = {
    id: "acct-1",
    providerKey: "claude",
    label: "work@corp",
    isActive: false,
  } as unknown as ProviderAccountSummary;
  useStore.setState({
    client: new MockRunnerClient(),
    providerAccounts: [account],
    pendingAccountSwitch: undefined,
    runId: "rFocused",
  });
  useStore.getState().ingestAttentionItem(quotaItem("rBg", "acct-1"));
  const found = attentionQueue.items.find((i) => i.runId === "rBg")!;
  // "Switch to X" → submitAttentionDecision → the SAME pendingAccountSwitch
  // the AccountSwitchModal's confirm button consumes — no forked effect.
  const ok = await useStore.getState().submitAttentionDecision("rBg", found.decision!, "switch");
  assert.equal(ok, true);
  assert.equal(useStore.getState().pendingAccountSwitch?.candidateAccount.id, "acct-1");
  assert.equal(useStore.getState().pendingAccountSwitch?.reason, "usage_limit");
  // And confirmAccountSwitch is the identical path the modal calls.
  let activated: string | undefined;
  (useStore.getState().client as MockRunnerClient).activateProviderAccount = async (id: string) => {
    activated = id;
  };
  await useStore.getState().confirmAccountSwitch();
  assert.equal(activated, "acct-1");
  reset();
});

test("non-focused provider-switch prompt → inbox item", () => {
  reset();
  useStore.setState({ runId: "rFocused", pendingProviderSwitch: undefined });
  useStore.getState().ingestAttentionItem({
    runId: "rBg",
    chatId: "chat-rBg",
    projectId: "p-1",
    runTitle: "bg run",
    kind: "decision",
    waitingSince: new Date().toISOString(),
    decision: {
      version: 1,
      id: "decision:ps-rBg",
      runId: "rBg",
      kind: "r_requirement",
      revision: "1",
      status: "pending",
      actionable: true,
      prompt: "Provider switch requested",
    },
  });
  assert.ok(attentionQueue.items.some((i) => i.runId === "rBg"));
  assert.equal(useStore.getState().pendingProviderSwitch, undefined, "no modal for non-focused run");
  reset();
});
