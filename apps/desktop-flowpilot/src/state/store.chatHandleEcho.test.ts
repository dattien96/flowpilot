import test from "node:test";
import assert from "node:assert/strict";
import { useStore } from "./store";
import type { RunnerClient } from "../types/contract";

async function* emptyStream() {}

// Reproduces the run-197604 gap: first chat turn must persist chatId so the
// next chip switch takes the chat-scoped path (timeline kept), not Tank-078 wipe.
test("first chat turn persists chatId from startRun handle", async () => {
  const client = {
    listProjects: async () => [],
    listWorkflows: async () => [],
    listSteps: async () => [],
    listProviderAccounts: async () => [],
    listRunHistory: async () => [],
    listRemoteChatSessions: async () => [],
    listAgents: async () => [],
    listAgentRuns: async () => [],
    refreshAgentGraph: async () => ({ parentRunId: "run-1", runs: [], edges: [], busMessages: [], loopState: { status: "running", round: 0, roundCap: 3 } } as never),
    pauseAgentLoop: async () => ({ parentRunId: "run-1", runs: [], edges: [], busMessages: [], loopState: { status: "paused", round: 0, roundCap: 3 } } as never),
    resumeAgentLoop: async () => ({ parentRunId: "run-1", runs: [], edges: [], busMessages: [], loopState: { status: "running", round: 0, roundCap: 3 } } as never),
    injectAgentFeedback: async () => ({ parentRunId: "run-1", runs: [], edges: [], busMessages: [], loopState: { status: "running", round: 0, roundCap: 3 } } as never),
    stopAgentLoop: async () => ({ parentRunId: "run-1", runs: [], edges: [], busMessages: [], loopState: { status: "stopped", round: 0, roundCap: 3 } } as never),
    spawnAgent: async () => ({ runId: "agent-1", providerSessionId: "ses", providerKey: "codex", status: "completed" } as never),
    startRun: async () => ({ runId: "run-1", providerSessionId: "ses-1", providerKey: "opencode", status: "running", chatId: "cht_test", legSeq: 0 } as never),
    resumeRun: async (runId: string) => ({ runId, providerSessionId: "ses-1", providerKey: "opencode", status: "completed" } as never),
    handoffContext: async () => ({ sourceRunId: "run-1", sourceProviderKey: "opencode", targetProviderKey: "grok", prompt: "[mock]", includedTurnCount: 0, omittedTurnCount: 0, truncated: false, handoffMode: "raw" } as never),
    generateChatSummary: async () => ({ runId: "run-1", generated: true, skipped: false } as never),
    syncChatRun: async () => ({ runId: "run-1", sourceMachineId: "mch", sourceRunId: "run-1", syncStatus: "synced", syncedAt: new Date().toISOString(), remotePath: "" } as never),
    deleteRun: async () => {},
    restoreChatRun: async () => ({ runId: "run-1", sourceMachineId: "mch", sourceRunId: "run-1", providerKey: "opencode", restoreStatus: "restored" } as never),
    sendTurn: () => emptyStream() as never,
    switchChatProvider: async () => { throw new Error("should not be called on first turn"); },
    chatTimeline: async () => ({ chatId: "cht_test", legs: [], records: [], nextSeq: 0, truncated: false, degraded: false } as never),
    listSkills: async () => [],
    listBuiltinOrchestrationOptions: async () => [],
    listArtifacts: async () => [],
    getChatPosture: async () => ({ active: "code", profiles: {} } as never),
    setChatPosture: async (c: any) => c as never,
  } as unknown as RunnerClient;

  useStore.setState({
    client,
    selectedProjectId: "proj-1",
    selectedProjectPath: "D:/tmp",
    selectedProvider: "opencode",
    selectedModel: "opencode-go/muse-spark-1.2-contributor",
    reasoningEffort: "low",
    yoloMode: false,
    chatMode: "normal_chat",
    runId: undefined,
    chatId: undefined,
    timeline: [],
    status: "idle",
  } as never);

  await useStore.getState().sendPrompt("hello ban la model nao");

  const s = useStore.getState();
  assert.equal(s.chatId, "cht_test", "chatId must be persisted from startRun handle");
  assert.equal(s.runId, "run-1");
});

// After the first turn has a chatId, a chip switch must hit switchChatProvider
// (timeline kept) not handoffContext+startRun (timeline wipe). Table over targets.
for (const target of ["grok", "claude", "codex"] as const) {
  test(`chip switch after first turn uses chat-scoped path for target ${target}`, async () => {
    let switchCalls = 0;
    let handoffCalls = 0;
    const client = {
      listProjects: async () => [],
      listWorkflows: async () => [],
      listSteps: async () => [],
      listProviderAccounts: async () => [],
      listRunHistory: async () => [],
      listRemoteChatSessions: async () => [],
      listAgents: async () => [],
      listAgentRuns: async () => [],
      refreshAgentGraph: async () => ({ parentRunId: "run-1", runs: [], edges: [], busMessages: [], loopState: { status: "running", round: 0, roundCap: 3 } } as never),
      pauseAgentLoop: async () => ({ parentRunId: "run-1", runs: [], edges: [], busMessages: [], loopState: { status: "paused", round: 0, roundCap: 3 } } as never),
      resumeAgentLoop: async () => ({ parentRunId: "run-1", runs: [], edges: [], busMessages: [], loopState: { status: "running", round: 0, roundCap: 3 } } as never),
      injectAgentFeedback: async () => ({ parentRunId: "run-1", runs: [], edges: [], busMessages: [], loopState: { status: "running", round: 0, roundCap: 3 } } as never),
      stopAgentLoop: async () => ({ parentRunId: "run-1", runs: [], edges: [], busMessages: [], loopState: { status: "stopped", round: 0, roundCap: 3 } } as never),
      spawnAgent: async () => ({ runId: "agent-1", providerSessionId: "ses", providerKey: "codex", status: "completed" } as never),
      startRun: async () => { throw new Error("legacy startRun must not be called when chatId exists"); },
      resumeRun: async (runId: string) => ({ runId, providerSessionId: "ses", providerKey: "opencode", status: "completed" } as never),
      handoffContext: async () => { handoffCalls++; return {} as never; },
      generateChatSummary: async () => ({ runId: "run-1", generated: true, skipped: false } as never),
      syncChatRun: async () => ({ runId: "run-1", sourceMachineId: "mch", sourceRunId: "run-1", syncStatus: "synced", syncedAt: "", remotePath: "" } as never),
      deleteRun: async () => {},
      restoreChatRun: async () => ({ runId: "run-1", sourceMachineId: "mch", sourceRunId: "run-1", providerKey: "opencode", restoreStatus: "restored" } as never),
      sendTurn: () => emptyStream() as never,
      switchChatProvider: async (chatId: string, input: { targetProviderKey: string }) => {
        switchCalls++;
        assert.equal(chatId, "cht_test");
        assert.equal(input.targetProviderKey, target);
        return { handle: { runId: `mock-run-cht_test-1`, providerSessionId: "ses-1", providerKey: target, status: "starting", chatId: "cht_test", legSeq: 1, stepId: "chat-mock" }, chatId: "cht_test", legSeq: 1, model: `${target}-model`, handoff: { handoffMode: "raw", includedTurnCount: 1, omittedTurnCount: 0, truncated: false, actionsDigestIncluded: false } } as never;
      },
      chatTimeline: async () => ({ chatId: "cht_test", legs: [], records: [], nextSeq: 0, truncated: false, degraded: false } as never),
      listSkills: async () => [],
      listBuiltinOrchestrationOptions: async () => [],
      listArtifacts: async () => [],
      getChatPosture: async () => ({ active: "code", profiles: {} } as never),
      setChatPosture: async (c: any) => c as never,
    } as unknown as RunnerClient;

    useStore.setState({
      client,
      selectedProjectId: "proj-1",
      selectedProvider: "opencode" as never,
      selectedModel: "opencode-go/muse-spark-1.2-contributor",
      reasoningEffort: "low",
      yoloMode: false,
      chatMode: "normal_chat" as never,
      runId: "run-1",
      chatId: "cht_test",
      timeline: [{ kind: "prompt", id: "u1", text: "hello" } as never],
      status: "idle" as never,
      pendingProviderSwitch: { sourceRunId: "run-1", sourceProviderKey: "opencode", sourceRunStatus: "idle", targetProviderKey: target, targetModel: `${target}-model` } as never,
      providerSwitchLoading: false,
    } as never);

    await useStore.getState().confirmProviderSwitch();
    assert.equal(switchCalls, 1);
    assert.equal(handoffCalls, 0, "must not fall back to handoffContext");
    assert.equal(useStore.getState().chatId, "cht_test");
  });
}
