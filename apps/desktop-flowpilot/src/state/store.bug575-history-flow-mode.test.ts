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

// BUG-575 (live run-100368): a vibe/flow run launched from a chat surface
// persists runKind "chat" on its history row even though the flow engine
// drove it (flowArm "started"). Reopening it from HISTORY left chatMode
// "normal_chat" — the view rendered as a plain chat and hid the flow
// board/timeline/agents surfaces. flowArm "started" is the durable
// authority that a flow actually ran and must win over runKind.
test("openHistoryRun restores Flow Mode for a runKind=chat row with flowArm=started", async () => {
  ensureLocalStorage();
  localStorage.clear();
  const client = new MockRunnerClient();
  client.resumeRun = async () => ({
    runId: "run-vibe-575",
    providerKey: "codex",
    status: "running",
    stepId: "chat-run-vibe-575",
    chatId: "cht_575",
    flowArm: "started",
    flowRef: "vibe-cp-ingest",
  }) as never;
  client.chatTimeline = async () => ({ chatId: "cht_575", legs: [], records: [] }) as never;
  client.streamRun = () => ({ [Symbol.asyncIterator]: async function* () {} }) as never;
  useStore.setState({
    client: client as unknown as ReturnType<typeof useStore.getState>["client"],
    chatMode: "normal_chat",
    runId: undefined,
  });
  await useStore.getState().openHistoryRun("run-vibe-575", {
    runId: "run-vibe-575",
    runKind: "chat",
  } as never);
  assert.equal(useStore.getState().chatMode, "workflow_step_auto");
  assert.equal(useStore.getState().flowStarted, true);
});

// Same fix on the missing-history-row path: a terminal resumed chat whose
// handle reports flowArm "started" still opens as Flow Mode.
test("openHistoryRun restores Flow Mode from handle.flowArm when no history row", async () => {
  ensureLocalStorage();
  localStorage.clear();
  const client = new MockRunnerClient();
  client.resumeRun = async () => ({
    runId: "run-vibe-575b",
    providerKey: "codex",
    status: "running",
    stepId: "chat-run-vibe-575b",
    chatId: "cht_575b",
    flowArm: "started",
    flowRef: "vibe-cp-ingest",
  }) as never;
  client.chatTimeline = async () => ({ chatId: "cht_575b", legs: [], records: [] }) as never;
  client.streamRun = () => ({ [Symbol.asyncIterator]: async function* () {} }) as never;
  useStore.setState({
    client: client as unknown as ReturnType<typeof useStore.getState>["client"],
    chatMode: "normal_chat",
    runId: undefined,
  });
  await useStore.getState().openHistoryRun("run-vibe-575b");
  assert.equal(useStore.getState().chatMode, "workflow_step_auto");
});

// BUG-614 (live run-150388 / leg run-163587): vibe-CP-ingest parent runs and
// spawned flow legs persist flowArm "immediate" — not "started" — because the
// flow launched at run-create rather than via a pending→started forward turn.
// Reopening such a run still means Flow Mode; runKind "chat" + flowArm
// "immediate" must not fall through to normal_chat and hide the flow sidebar.
test("openHistoryRun restores Flow Mode for a runKind=chat row with flowArm=immediate", async () => {
  ensureLocalStorage();
  localStorage.clear();
  const client = new MockRunnerClient();
  client.resumeRun = async () => ({
    runId: "run-vibe-614",
    providerKey: "codex",
    status: "running",
    stepId: "chat-run-vibe-614",
    chatId: "cht_614",
    flowArm: "immediate",
    flowRef: "vibe-tasks",
  }) as never;
  client.chatTimeline = async () => ({ chatId: "cht_614", legs: [], records: [] }) as never;
  client.streamRun = () => ({ [Symbol.asyncIterator]: async function* () {} }) as never;
  useStore.setState({
    client: client as unknown as ReturnType<typeof useStore.getState>["client"],
    chatMode: "normal_chat",
    runId: undefined,
  });
  await useStore.getState().openHistoryRun("run-vibe-614", {
    runId: "run-vibe-614",
    runKind: "chat",
  } as never);
  assert.equal(useStore.getState().chatMode, "workflow_step_auto");
  assert.equal(useStore.getState().flowStarted, true);
});

// Same fix on the missing-history-row path: the handle alone must carry the
// immediate arm across — a spawned leg resumed with no row still shows Flow.
test("openHistoryRun restores Flow Mode from handle.flowArm=immediate when no history row", async () => {
  ensureLocalStorage();
  localStorage.clear();
  const client = new MockRunnerClient();
  client.resumeRun = async () => ({
    runId: "run-vibe-614b",
    providerKey: "codex",
    status: "running",
    stepId: "chat-run-vibe-614b",
    chatId: "cht_614b",
    flowArm: "immediate",
    flowRef: "vibe-sprint",
  }) as never;
  client.chatTimeline = async () => ({ chatId: "cht_614b", legs: [], records: [] }) as never;
  client.streamRun = () => ({ [Symbol.asyncIterator]: async function* () {} }) as never;
  useStore.setState({
    client: client as unknown as ReturnType<typeof useStore.getState>["client"],
    chatMode: "normal_chat",
    runId: undefined,
  });
  await useStore.getState().openHistoryRun("run-vibe-614b");
  assert.equal(useStore.getState().chatMode, "workflow_step_auto");
  assert.equal(useStore.getState().flowStarted, true);
});

// Regression guard: a genuine plain chat (no flowArm at all) still opens
// as normal_chat.
test("openHistoryRun keeps Chat Mode for a row with no flowArm", async () => {
  ensureLocalStorage();
  localStorage.clear();
  const client = new MockRunnerClient();
  client.resumeRun = async () => ({
    runId: "run-chat-575",
    providerKey: "codex",
    status: "completed",
    stepId: "chat-run-chat-575",
    chatId: "cht_575c",
  }) as never;
  client.chatTimeline = async () => ({ chatId: "cht_575c", legs: [], records: [] }) as never;
  client.streamRun = () => ({ [Symbol.asyncIterator]: async function* () {} }) as never;
  useStore.setState({
    client: client as unknown as ReturnType<typeof useStore.getState>["client"],
    chatMode: "workflow_step_auto",
    runId: undefined,
  });
  await useStore.getState().openHistoryRun("run-chat-575", {
    runId: "run-chat-575",
    runKind: "chat",
  } as never);
  assert.equal(useStore.getState().chatMode, "normal_chat");
});
