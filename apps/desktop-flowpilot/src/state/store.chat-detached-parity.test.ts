import test from "node:test";
import assert from "node:assert/strict";
import { useStore } from "./store";
import { MockRunnerClient } from "../client/MockRunnerClient";

test("openHistoryRun marks detached when handle is terminal", async () => {
  const client = new MockRunnerClient();
  client.resumeRun = async () => ({
    runId: "run-old",
    providerKey: "codex",
    status: "completed",
    stepId: "chat-run-old",
    chatId: "cht_a",
  }) as never;
  client.chatTimeline = async () => ({ chatId: "cht_a", legs: [], records: [] }) as never;
  client.streamRun = () => ({ [Symbol.asyncIterator]: async function* () {} }) as never;
  useStore.setState({
    client: client as unknown as ReturnType<typeof useStore.getState>["client"],
    chatMode: "normal_chat",
  });
  // Signature is openHistoryRun(runId) — the runKind/chatId come from the
  // resumeRun handle + history row; the store falls back to the handle when
  // no history row exists (BUG-340 detached parity).
  await useStore.getState().openHistoryRun("run-old");
  assert.equal(useStore.getState().chatDetached, true, "terminal chat must be detached");
  assert.equal(useStore.getState().chatId, "cht_a");
});

test("sendPrompt on detached reattaches via startRun with chatId+switchFromRunId", async () => {
  const client = new MockRunnerClient();
  let startRunInput: Record<string, unknown> | undefined;
  client.startRun = async (input) => {
    startRunInput = input as unknown as Record<string, unknown>;
    return {
      runId: "run-new",
      providerKey: "grok",
      status: "running",
      stepId: "chat-run-new",
      chatId: "cht_a",
    } as never;
  };
  // Mock stream to avoid hanging: sendPrompt consumes an async iterable, so
  // stub sendTurn with an immediately-completing generator (replaces the
  // obsolete promise-typed no-op below — must satisfy AsyncIterable).
  client.sendTurn = (() => {
    async function* gen() {}
    return gen();
  }) as unknown as typeof client.sendTurn;

  useStore.setState({
    client: client as unknown as ReturnType<typeof useStore.getState>["client"],
    chatMode: "normal_chat",
    selectedProjectId: "proj-1",
    selectedProvider: "grok",
    selectedModel: "grok-4.5",
    runId: "run-old",
    chatId: "cht_a",
    chatDetached: true,
    status: "completed",
    timeline: [],
  });
  // Mock selectedProjectPath
  (useStore.getState() as unknown as { selectedProjectId: string }).selectedProjectId = "proj-1";

  await useStore.getState().sendPrompt("hello detached");

  assert.equal(startRunInput?.chatId, "cht_a", "reattach must carry chatId");
  assert.equal(startRunInput?.switchFromRunId, "run-old", "reattach must carry switchFromRunId");
  assert.equal(useStore.getState().chatDetached, false, "reattach must clear detached");
  assert.equal(useStore.getState().runId, "run-new");
});

test("confirmProviderSwitch while detached defers locally without endpoint call", async () => {
  const client = new MockRunnerClient();
  let switchCalled = false;
  client.switchChatProvider = async () => {
    switchCalled = true;
    throw new Error("should not be called when detached");
  };
  useStore.setState({
    client: client as unknown as ReturnType<typeof useStore.getState>["client"],
    chatMode: "normal_chat",
    selectedProjectId: "proj-1",
    chatId: "cht_a",
    chatDetached: true,
    runId: "run-old",
    pendingProviderSwitch: {
      sourceRunId: "run-old",
      sourceProviderKey: "codex",
      sourceRunStatus: "completed",
      targetProviderKey: "grok",
      targetModel: "grok-4.5",
    },
    timeline: [],
  });
  await useStore.getState().confirmProviderSwitch();
  assert.equal(switchCalled, false, "detached must not call switch endpoint");
  assert.equal(useStore.getState().selectedProvider, "grok");
  assert.equal(useStore.getState().pendingProviderSwitch, undefined);
  const last = useStore.getState().timeline[useStore.getState().timeline.length - 1] as { text?: string };
  assert.ok(String(last?.text ?? "").includes("reattaches"), "must show defer notice");
});

test("confirmProviderSwitch while question pending is blocked", async () => {
  const client = new MockRunnerClient();
  let switchCalled = false;
  client.switchChatProvider = async () => {
    switchCalled = true;
    return {} as never;
  };
  useStore.setState({
    client: client as unknown as ReturnType<typeof useStore.getState>["client"],
    chatMode: "normal_chat",
    selectedProjectId: "proj-1",
    chatId: "cht_a",
    chatDetached: false,
    pendingProviderSwitch: {
      sourceRunId: "run-1",
      sourceProviderKey: "codex",
      sourceRunStatus: "running",
      targetProviderKey: "grok",
      targetModel: "grok-4.5",
    },
    pendingQuestions: [{ questionId: "q1", prompt: "ask_user?" } as never],
    timeline: [],
  });
  await useStore.getState().confirmProviderSwitch();
  assert.equal(switchCalled, false, "question pending must block switch");
  const last = useStore.getState().timeline[useStore.getState().timeline.length - 1] as { text?: string };
  assert.ok(String(last?.text ?? "").includes("Cannot switch"), "must show busy notice");
  // Must not clear pending question
  assert.equal(useStore.getState().pendingQuestions.length, 1);
});

test("setChatPosture while question pending is blocked", async () => {
  const client = new MockRunnerClient();
  let putCalled = false;
  client.getChatPosture = async () => ({ active: "code", profiles: { code: {}, plan: { provider: "grok", model: "grok-4.5" } } } as never);
  client.setChatPosture = async (cfg) => {
    putCalled = true;
    return cfg as never;
  };
  useStore.setState({
    client: client as unknown as ReturnType<typeof useStore.getState>["client"],
    chatPosture: "code",
    chatPostureConfig: { active: "code", profiles: { code: {}, plan: {} } },
    pendingQuestions: [{ questionId: "q1" } as never],
    timeline: [],
  });
  await useStore.getState().setChatPosture("plan");
  assert.equal(putCalled, false, "question pending must block posture switch");
  assert.equal(useStore.getState().chatPosture, "code", "posture must not change");
  const last = useStore.getState().timeline[useStore.getState().timeline.length - 1] as { text?: string };
  assert.ok(String(last?.text ?? "").includes("Cannot switch posture"));
});

test("confirmProviderSwitch handles 409 chat_no_active_leg as detached defer", async () => {
  const client = new MockRunnerClient();
  client.switchChatProvider = async () => {
    const err = new Error("chat_no_active_leg") as unknown as { code: string; message: string };
    (err as { code: string }).code = "chat_no_active_leg";
    throw err;
  };
  useStore.setState({
    client: client as unknown as ReturnType<typeof useStore.getState>["client"],
    chatMode: "normal_chat",
    selectedProjectId: "proj-1",
    chatId: "cht_a",
    chatDetached: false,
    // Isolation: earlier tests leave a pending question/approval in the shared
    // store — the C2 guard would block the switch before the 409 path runs.
    pendingQuestions: [],
    pendingApprovals: [],
    pendingProviderSwitch: {
      sourceRunId: "run-1",
      sourceProviderKey: "codex",
      sourceRunStatus: "running",
      targetProviderKey: "grok",
      targetModel: "grok-4.5",
    },
    timeline: [],
  });
  await useStore.getState().confirmProviderSwitch();
  assert.equal(useStore.getState().chatDetached, true, "409 must mark detached");
  assert.equal(useStore.getState().selectedProvider, "grok", "409 must apply locally");
  assert.equal(useStore.getState().pendingProviderSwitch, undefined);
});

// Provider-agnostic: same reattach/detached logic for any provider (no providerKey branch)
test("detached reattach is provider-agnostic (chatId only)", async () => {
  for (const target of ["grok", "codex", "claude"] as const) {
    const client = new MockRunnerClient();
    let input: Record<string, unknown> | undefined;
    client.startRun = async (i) => {
      input = i as unknown as Record<string, unknown>;
      return { runId: `run-new-${target}`, providerKey: target, status: "running", stepId: "s", chatId: "cht_a" } as never;
    };
    client.sendTurn = (() => {
      async function* gen() {}
      return gen();
    }) as unknown as typeof client.sendTurn;
    useStore.setState({
      client: client as unknown as ReturnType<typeof useStore.getState>["client"],
      chatMode: "normal_chat",
      selectedProjectId: "proj-1",
      selectedProvider: target,
      runId: "run-old",
      chatId: "cht_a",
      chatDetached: true,
      status: "completed",
      timeline: [],
    });
    await useStore.getState().sendPrompt(`hello from ${target}`);
    assert.equal(input?.chatId, "cht_a", `${target}: must carry chatId`);
    assert.equal(input?.switchFromRunId, "run-old", `${target}: must carry switchFrom`);
  }
});
