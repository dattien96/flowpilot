import test from "node:test";
import assert from "node:assert/strict";
import { MockRunnerClient } from "../client/MockRunnerClient";
import type { RunHandle, RunHistoryItem } from "../types/contract";
import { useStore } from "./store";

// CA-1047: chat-switch latency — a fresh send must surface in the Navigator
// immediately (pending row + minted-run upsert) instead of waiting for the
// 3s/10s history poll, a click must focus the chat as soon as resume lands
// (not after the transcript tail), and stale resume/start responses can never
// refocus a run the user has already navigated away from.

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => { resolve = done; });
  return { promise, resolve };
}

function row(runId: string): RunHistoryItem {
  return {
    runId,
    projectId: "p1",
    chatId: `chat-${runId}`,
    providerKey: "codex",
    runKind: "chat",
    status: "completed",
    startedAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    lastPrompt: `Prompt for ${runId}`,
  };
}

function handle(runId: string): RunHandle {
  return {
    runId,
    chatId: `chat-${runId}`,
    providerSessionId: `session-${runId}`,
    providerKey: "codex",
    stepId: `chat-${runId}`,
    status: "completed",
  };
}

function seed(client: MockRunnerClient, history: RunHistoryItem[] = []): void {
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    value: { getItem: () => null, setItem: () => {}, removeItem: () => {} },
  });
  client.streamRun = async function* () {};
  client.sendTurn = async function* () {};
  client.chatTimeline = async (chatId) => ({ chatId, legs: [], records: [], nextSeq: 0, truncated: false, degraded: false });
  useStore.setState({
    client,
    projects: [{ id: "p1", name: "Project One", path: "/tmp/p1" }],
    selectedProjectId: "p1",
    selectedProvider: "codex",
    chatMode: "normal_chat",
    status: "idle",
    runId: undefined,
    chatId: undefined,
    mainRunId: undefined,
    activeAgentRunId: undefined,
    activeStepId: undefined,
    timeline: [],
    runHistory: history,
    projectHistoryById: { p1: history },
    historyLoading: false,
    historyLoadError: undefined,
    historyOpenError: undefined,
    pendingApprovals: [],
    pendingQuestions: [],
    agentRuns: [],
    _runSnapshots: {},
    _streamRunSeq: 0,
    _historyLoadSeq: 0,
    historyOpeningRunId: undefined,
    _historyOpenSeq: 0,
    _locallyStartedRuns: {},
    pendingChatStart: undefined,
  } as never);
}

test("a new chat exposes a pending row then a real row without waiting for the history poll", async () => {
  const client = new MockRunnerClient();
  const started = deferred<RunHandle>();
  client.startRun = () => started.promise;
  seed(client);

  const send = useStore.getState().sendPrompt("A new prompt");
  assert.equal(useStore.getState().pendingChatStart?.prompt, "A new prompt");
  assert.equal(useStore.getState().status, "running");

  started.resolve({ ...handle("new-run"), status: "running" });
  await send;

  const s = useStore.getState();
  assert.equal(s.pendingChatStart, undefined);
  const row = s.runHistory.find((item) => item.runId === "new-run");
  assert.equal(row?.lastPrompt, "A new prompt");
  assert.equal(row?.status, "running");
  assert.equal(s.projectHistoryById.p1?.some((item) => item.runId === "new-run"), true);
});

test("a stale history poll cannot drop the locally-started chat row", async () => {
  const client = new MockRunnerClient();
  client.listRunHistory = async () => [];
  seed(client);

  await useStore.getState().sendPrompt("A new prompt");
  await useStore.getState().loadRunHistory({ silent: true });

  const ids = useStore.getState().runHistory.map((item) => item.runId);
  assert.ok(ids.includes("run-1") || ids.length > 0);
});

test("clicking a chat marks it opening before resume resolves", async () => {
  const client = new MockRunnerClient();
  const resumed = deferred<RunHandle>();
  client.resumeRun = () => resumed.promise;
  seed(client, [row("previous"), row("current")]);
  useStore.setState({ runId: "current", status: "running" } as never);

  const opening = useStore.getState().openHistoryRun("previous");
  assert.equal(useStore.getState().historyOpeningRunId, "previous");
  assert.equal(useStore.getState().runId, "current");

  resumed.resolve(handle("previous"));
  await opening;
  assert.equal(useStore.getState().runId, "previous");
  assert.equal(useStore.getState().historyOpeningRunId, undefined);
});

test("a verified chat becomes focused while its transcript tail is still pending", async () => {
  const client = new MockRunnerClient();
  const transcript = deferred<Awaited<ReturnType<MockRunnerClient["chatTimeline"]>>>();
  client.resumeRun = async () => handle("previous");
  client.chatTimeline = () => transcript.promise;
  seed(client, [row("previous")]);

  const opening = useStore.getState().openHistoryRun("previous");
  await Promise.resolve();
  await Promise.resolve();
  assert.equal(useStore.getState().runId, "previous");

  transcript.resolve({ chatId: "chat-previous", legs: [], records: [], nextSeq: 0, truncated: false, degraded: false });
  await opening;
  assert.equal(useStore.getState().historyOpeningRunId, undefined);
});

test("the latest history click wins over slower resume responses", async () => {
  const client = new MockRunnerClient();
  const a = deferred<RunHandle>();
  const b = deferred<RunHandle>();
  client.resumeRun = (runId) => (runId === "a" ? a.promise : b.promise);
  seed(client, [row("a"), row("b")]);

  const openA = useStore.getState().openHistoryRun("a");
  const openB = useStore.getState().openHistoryRun("b");
  b.resolve(handle("b"));
  await openB;
  a.resolve(handle("a"));
  await openA;

  assert.equal(useStore.getState().runId, "b");
});

test("switching away during a slow first startRun does not refocus the new chat", async () => {
  const client = new MockRunnerClient();
  const started = deferred<RunHandle>();
  const sent: string[] = [];
  client.startRun = () => started.promise;
  seed(client, [row("previous")]);
  client.sendTurn = async function* (input) { sent.push(input.runId); };

  const send = useStore.getState().sendPrompt("Run this in a new chat");
  await useStore.getState().openHistoryRun("previous");
  started.resolve({ ...handle("new-run"), status: "running" });
  await send;

  assert.equal(useStore.getState().runId, "previous");
  assert.deepEqual(sent, ["new-run"], "the minted turn still runs server-side");
  assert.ok(
    !useStore.getState().timeline.some(
      (item) => item.kind === "prompt" && "text" in item && item.text === "Run this in a new chat",
    ),
    "the detached send's optimistic rows stay with the new run's lane",
  );
});

test("resetRun invalidates a pending history open", async () => {
  const client = new MockRunnerClient();
  const resumed = deferred<RunHandle>();
  client.resumeRun = () => resumed.promise;
  seed(client, [row("previous")]);

  const opening = useStore.getState().openHistoryRun("previous");
  useStore.getState().resetRun();
  resumed.resolve(handle("previous"));
  await opening;

  assert.equal(useStore.getState().runId, undefined);
});
