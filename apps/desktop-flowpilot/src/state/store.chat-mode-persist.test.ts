import test from "node:test";
import assert from "node:assert/strict";
import { useStore } from "./store";
import { MockRunnerClient } from "../client/MockRunnerClient";

// Polyfill localStorage for node:test
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

test("setChatMode persists Chat vs Workflow to localStorage", () => {
  ensureLocalStorage();
  localStorage.clear();
  const client = new MockRunnerClient();
  useStore.setState({
    client: client as unknown as ReturnType<typeof useStore.getState>["client"],
    chatMode: "normal_chat",
  });
  useStore.getState().setChatMode("workflow_step_auto");
  assert.equal(localStorage.getItem("fp:lastChatMode"), "workflow_step_auto");
  assert.equal(useStore.getState().chatMode, "workflow_step_auto");
  // Back to chat
  useStore.getState().setChatMode("normal_chat");
  assert.equal(localStorage.getItem("fp:lastChatMode"), "normal_chat");
});

test("loadProjects restores last Chat vs Workflow when no active run", async () => {
  ensureLocalStorage();
  localStorage.clear();
  localStorage.setItem("fp:lastChatMode", "workflow_step_auto");
  const client = new MockRunnerClient();
  // Mock listProjects to avoid network
  client.listProjects = async () => [{ id: "proj-1", name: "gate", path: "/tmp/gate" } as never];
  client.listProviderAccounts = async () => [] as never;
  client.getChatPosture = async () => ({ active: "non", profiles: { scan: {}, plan: {}, code: {}, non: {} } } as never);
  // Ensure no runId
  useStore.setState({
    client: client as unknown as ReturnType<typeof useStore.getState>["client"],
    chatMode: "normal_chat",
    runId: undefined,
    chatId: undefined,
    selectedProjectId: undefined,
  });
  await useStore.getState().loadProjects();
  // loadProjects should have restored workflow_step_auto from localStorage
  assert.equal(useStore.getState().chatMode, "workflow_step_auto");
  // Cleanup
  localStorage.clear();
  useStore.setState({ chatMode: "normal_chat" });
});

test("openHistoryRun restores chatMode from history item, not persisted last surface", async () => {
  ensureLocalStorage();
  localStorage.clear();
  localStorage.setItem("fp:lastChatMode", "workflow_step_auto");
  const client = new MockRunnerClient();
  // Mock resumeRun to return a chat run
  client.resumeRun = async () => ({
    runId: "run-chat-1",
    providerKey: "codex",
    status: "completed",
    stepId: "chat-run-chat-1",
    chatId: "cht_a",
  }) as never;
  client.chatTimeline = async () => ({ chatId: "cht_a", legs: [], records: [] }) as never;
  client.streamRun = () => ({ [Symbol.asyncIterator]: async function* () {} }) as never;
  useStore.setState({
    client: client as unknown as ReturnType<typeof useStore.getState>["client"],
    chatMode: "workflow_step_auto",
    runId: undefined,
  });
  await useStore.getState().openHistoryRun("run-chat-1", { runId: "run-chat-1", runKind: "chat" } as never);
  // History item is chat, so chatMode must be normal_chat even though persisted was workflow
  assert.equal(useStore.getState().chatMode, "normal_chat");
  assert.equal(useStore.getState().chatId, "cht_a");
});
