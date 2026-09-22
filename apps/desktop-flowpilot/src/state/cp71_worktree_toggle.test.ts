import test from "node:test";
import assert from "node:assert/strict";
import { useStore } from "./store";
import { MockRunnerClient } from "../client/MockRunnerClient";

// CP-71 / Task-409: worktree start toggle — per-chat derived state, gating,
// and the startRun payload flag. Additive; uses MockRunnerClient stubs like
// the other store tests.

test("toggle defaults off for a new chat", () => {
  useStore.getState().resetRun();
  const s = useStore.getState();
  assert.equal(s.worktreeEnabled, false);
  assert.equal(s.activeWorktreeState, "");
});

test("toggle restores from the opened run's binding", async () => {
  const client = new MockRunnerClient();
  client.resumeRun = async () => ({
    runId: "run-wt",
    providerKey: "codex",
    status: "idle",
    stepId: "chat-run-wt",
    chatId: "cht_wt",
  }) as never;
  client.chatTimeline = async () => ({ chatId: "cht_wt", legs: [], records: [] }) as never;
  client.streamRun = () => ({ [Symbol.asyncIterator]: async function* () {} }) as never;
  useStore.setState({
    client: client as unknown as ReturnType<typeof useStore.getState>["client"],
    chatMode: "normal_chat",
    runHistory: [{
      runId: "run-wt", projectId: "p-1", providerKey: "codex",
      status: "idle", startedAt: "", updatedAt: "", runKind: "chat",
      worktreeState: "active", worktreeSlug: "fp-chat-cht_wt",
    }] as never,
  });
  await useStore.getState().openHistoryRun("run-wt");
  assert.equal(useStore.getState().worktreeEnabled, true);
  assert.equal(useStore.getState().activeWorktreeState, "active");
});

test("toggle off rejected while a live binding exists", () => {
  useStore.setState({
    worktreeEnabled: true,
    worktreeAvailable: true,
    activeWorktreeState: "active",
    timeline: [],
  });
  useStore.getState().setWorktreeEnabled(false);
  const s = useStore.getState();
  assert.equal(s.worktreeEnabled, true, "must stay armed while binding is live");
  assert.ok(
    s.timeline.some((i) => i.kind === "system" && "text" in i && i.text.includes("Merge or discard")),
    "expected merge/discard notice",
  );
});

test("toggle on rejected on a non-git project", () => {
  useStore.setState({
    worktreeEnabled: false,
    worktreeAvailable: false,
    activeWorktreeState: "",
    timeline: [],
  });
  useStore.getState().setWorktreeEnabled(true);
  const s = useStore.getState();
  assert.equal(s.worktreeEnabled, false);
  assert.ok(
    s.timeline.some((i) => i.kind === "system" && "text" in i && i.text.includes("git repository")),
    "expected git-repo notice",
  );
});

test("startRun sends worktree:true when enabled", async () => {
  const client = new MockRunnerClient();
  let startRunInput: Record<string, unknown> | undefined;
  client.startRun = async (input) => {
    startRunInput = input as unknown as Record<string, unknown>;
    return {
      runId: "run-new",
      providerKey: "codex",
      status: "running",
      stepId: "chat-run-new",
      chatId: "cht_new",
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
    runId: undefined,
    chatId: undefined,
    status: "idle",
    timeline: [],
    worktreeEnabled: true,
    worktreeAvailable: true,
    activeWorktreeState: "",
  });

  await useStore.getState().sendPrompt("hello worktree");
  assert.equal(startRunInput?.worktree, true, "start payload must carry worktree:true");
});

test("startRun omits worktree when disabled", async () => {
  const client = new MockRunnerClient();
  let startRunInput: Record<string, unknown> | undefined;
  client.startRun = async (input) => {
    startRunInput = input as unknown as Record<string, unknown>;
    return {
      runId: "run-plain",
      providerKey: "codex",
      status: "running",
      stepId: "chat-run-plain",
      chatId: "cht_plain",
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
    runId: undefined,
    chatId: undefined,
    status: "idle",
    timeline: [],
    worktreeEnabled: false,
    worktreeAvailable: true,
    activeWorktreeState: "",
  });

  await useStore.getState().sendPrompt("hello plain");
  assert.ok(!startRunInput?.worktree, "worktree must be absent/false when disabled");
});
