import test from "node:test";
import assert from "node:assert/strict";
import { useStore } from "./store";
import { MockRunnerClient } from "../client/MockRunnerClient";

function ensureLocalStorage() {
  if (typeof (globalThis as unknown as { localStorage?: unknown }).localStorage === "undefined") {
    const store = new Map<string, string>();
    (globalThis as unknown as { localStorage: Storage }).localStorage = {
      getItem: (k: string) => store.get(k) ?? null,
      setItem: (k: string, v: string) => store.set(k, v),
      removeItem: (k: string) => store.delete(k),
      clear: () => store.clear(),
      key: (i: number) => Array.from(store.keys())[i] ?? null,
      get length() { return store.size; },
    } as unknown as Storage;
  }
}

const VIBE_SPRINT_ROW = {
  id: "wf-mirror-vibe-sprint",
  projectId: "",
  name: "Vibe Sprint",
  model: "devin/swe-2-high",
  packFlowId: "vibe-sprint",
};

const BUG_HARNESS_ROW = {
  id: "wf-mirror-bug-harness",
  projectId: "",
  name: "Bug Harness",
  model: "devin/swe-2-high",
  packFlowId: "bug-harness",
};

function stubClient(handle: Record<string, unknown>) {
  const client = new MockRunnerClient();
  client.resumeRun = async () => handle as never;
  client.chatTimeline = async () =>
    ({ chatId: String(handle.chatId ?? ""), legs: [], records: [] }) as never;
  client.streamRun = () => ({ [Symbol.asyncIterator]: async function* () {} }) as never;
  return client as unknown as ReturnType<typeof useStore.getState>["client"];
}

// BUG-1199 (live run-225691): a chat-launched vibe run persists flowArm
// "started" + a pack-prefixed flowRef but NO workflowId. Reopening it must
// restore the Workflow surface AND resolve the running flow's catalog row
// into selectedWorkflowId — without the row the select renders blank even
// though the run is a live vibe-sprint.
test("openHistoryRun resolves selectedWorkflowId from a pack-prefixed flowRef", async () => {
  ensureLocalStorage();
  localStorage.clear();
  useStore.setState({
    client: stubClient({
      runId: "run-1199",
      providerKey: "devin",
      status: "running",
      stepId: "chat-run-1199",
      chatId: "cht_1199",
      flowArm: "started",
      flowRef: "flowpilot-core-flow-pack/vibe-sprint",
    }),
    chatMode: "normal_chat",
    launchMode: "step",
    selectedWorkflowId: undefined,
    workflows: [VIBE_SPRINT_ROW, BUG_HARNESS_ROW] as never,
    workingMode: "vibe",
    runId: undefined,
  });
  await useStore.getState().openHistoryRun("run-1199", {
    runId: "run-1199",
    runKind: "chat",
  } as never);
  const st = useStore.getState();
  assert.equal(st.chatMode, "workflow_step_auto");
  assert.equal(st.launchMode, "workflow");
  assert.equal(st.selectedWorkflowId, "wf-mirror-vibe-sprint");
  assert.equal(st.flowStarted, true);
  // A started flow must never come back armed-pending.
  assert.equal(st.pendingFlowArm, undefined);
});

// workflowId on the row stays authoritative — a flowRef mismatch must not
// override the identity the run was launched with.
test("openHistoryRun prefers historyItem.workflowId over flowRef resolution", async () => {
  ensureLocalStorage();
  localStorage.clear();
  useStore.setState({
    client: stubClient({
      runId: "run-1199b",
      providerKey: "devin",
      status: "completed",
      stepId: "wf-step",
      workflowId: "wf-mirror-bug-harness",
      flowRef: "vibe-sprint",
      flowArm: "immediate",
    }),
    chatMode: "normal_chat",
    launchMode: "step",
    selectedWorkflowId: undefined,
    workflows: [VIBE_SPRINT_ROW, BUG_HARNESS_ROW] as never,
    runId: undefined,
  });
  await useStore.getState().openHistoryRun("run-1199b", {
    runId: "run-1199b",
    runKind: "workflow",
    workflowId: "wf-mirror-bug-harness",
  } as never);
  assert.equal(useStore.getState().selectedWorkflowId, "wf-mirror-bug-harness");
});

// A flow run whose ref resolves to no catalog row clears a stale pick —
// showing the previous run's selection under this run is worse than none.
test("openHistoryRun clears stale selectedWorkflowId when the flow ref is unresolvable", async () => {
  ensureLocalStorage();
  localStorage.clear();
  useStore.setState({
    client: stubClient({
      runId: "run-1199c",
      providerKey: "devin",
      status: "completed",
      stepId: "chat-run-1199c",
      chatId: "cht_1199c",
      flowArm: "started",
      flowRef: "flowpilot-core-flow-pack/some-deleted-flow",
    }),
    chatMode: "normal_chat",
    launchMode: "workflow",
    selectedWorkflowId: "wf-mirror-bug-harness",
    workflows: [BUG_HARNESS_ROW] as never,
    runId: undefined,
  });
  await useStore.getState().openHistoryRun("run-1199c", {
    runId: "run-1199c",
    runKind: "chat",
  } as never);
  const st = useStore.getState();
  assert.equal(st.launchMode, "workflow");
  assert.equal(st.selectedWorkflowId, undefined);
});
