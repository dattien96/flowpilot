import test from "node:test";
import assert from "node:assert/strict";
import { useStore } from "./store";
import { MockRunnerClient } from "../client/MockRunnerClient";

// Polyfill localStorage for node:test (selectProject persists the choice).
function ensureLocalStorage() {
  if (typeof (globalThis as unknown as { localStorage?: unknown }).localStorage === "undefined") {
    const store = new Map<string, string>();
    (globalThis as unknown as { localStorage: Storage }).localStorage = {
      getItem: (k: string) => store.get(k) ?? null,
      setItem: (k: string, v: string) => store.set(k, v),
      removeItem: (k: string) => store.delete(k),
      clear: () => store.clear(),
      key: (i: number) => Array.from(store.keys())[i] ?? null,
      get length() {
        return store.size;
      },
    } as unknown as Storage;
  }
}
ensureLocalStorage();

// M-3 regression (CP-83 manual): arming the worktree toggle then starting a
// new chat must send worktree:true — the toggle is a next-run intent, so
// resetRun() (New run / new-chat paths) must not silently disarm it. The
// intent stays project-scoped: switching projects still clears it.

test("resetRun keeps the armed worktree intent", () => {
  useStore.setState({
    worktreeEnabled: true,
    worktreeAvailable: true,
    activeWorktreePath: "C:\\repo\\.flowpilot\\worktrees\\x",
    activeWorktreeState: "active",
    runId: "run-old",
  });
  useStore.getState().resetRun();
  const s = useStore.getState();
  assert.equal(s.worktreeEnabled, true, "next-run intent must survive resetRun");
  assert.equal(s.activeWorktreePath, "", "binding path is per-run — must clear");
  assert.equal(s.activeWorktreeState, "", "binding state is per-run — must clear");
  assert.equal(s.runId, undefined);
});

test("switching projects clears the worktree intent", async () => {
  useStore.setState({
    worktreeEnabled: true,
    worktreeAvailable: true,
    activeWorktreeState: "",
    selectedProjectId: "proj-a",
    projects: [
      { id: "proj-a", name: "a", path: "C:\\a" },
      { id: "proj-b", name: "b", path: "C:\\b" },
    ],
  });
  await useStore.getState().selectProject("proj-b");
  assert.equal(useStore.getState().worktreeEnabled, false);
  // Re-selecting the same project must NOT disarm (no reset happens).
  await useStore.getState().selectProject("proj-b");
  useStore.setState({ worktreeEnabled: true });
  await useStore.getState().selectProject("proj-b");
  assert.equal(useStore.getState().worktreeEnabled, true, "same-project reselect must keep intent");
});

test("sendPrompt after New run still sends worktree:true", async () => {
  // The exact reported flow: toggle ON → resetRun (New run button) → send.
  const client = new MockRunnerClient();
  let startRunInput: Record<string, unknown> | undefined;
  client.startRun = async (input) => {
    startRunInput = input as unknown as Record<string, unknown>;
    return {
      runId: "run-wt-new",
      providerKey: "codex",
      status: "running",
      stepId: "chat-run-wt-new",
      chatId: "cht_wt_new",
      worktreePath: "C:\\repo\\.flowpilot\\worktrees\\cht_wt_new",
    } as never;
  };
  client.sendTurn = (() => {
    async function* gen() {}
    return gen();
  }) as unknown as typeof client.sendTurn;

  useStore.setState({
    client: client as unknown as ReturnType<typeof useStore.getState>["client"],
    chatMode: "normal_chat",
    selectedProjectId: "proj-1",
    selectedProvider: "codex",
    selectedModel: "gpt-5.4",
    projects: [{ id: "proj-1", name: "p", path: "C:\\repo" }],
    status: "idle",
    timeline: [],
    worktreeEnabled: true,
    worktreeAvailable: true,
    activeWorktreeState: "",
  });
  useStore.getState().resetRun();

  await useStore.getState().sendPrompt("run in a worktree please");
  assert.equal(startRunInput?.worktree, true, "start payload must carry worktree:true after resetRun");
  assert.equal(
    useStore.getState().activeWorktreePath,
    "C:\\repo\\.flowpilot\\worktrees\\cht_wt_new",
    "returned worktreePath must land on the store for the terminal cwd",
  );
});
