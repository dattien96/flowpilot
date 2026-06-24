import test from "node:test";
import assert from "node:assert/strict";
import { useStore } from "./store";
import { applyTimelineEvent, type TimelineItem, type TimelineState } from "./timelineReducer";
import { getBusMessageLabel, getOrchestrationBoardEmptyCopy } from "@/components/OrchestrationBoard";
import { shouldShowAgentTimelineHeader } from "@/components/Timeline";
import { parseMentionRouting } from "@/components/ChatInput";
import { MockRunnerClient } from "../client/MockRunnerClient";
import { RunnerApiError } from "../client/HttpWsRunnerClient";
import type { AgentRunSummary, ProviderAccountSummary, ProviderEventDTO, RemoteChatSessionSummary, RunHandle, RunHistoryItem, RunnerClient, TurnInput } from "../types/contract";

async function* emptyStream(): AsyncIterable<ProviderEventDTO> {}

function makeClient(overrides: Partial<RunnerClient> = {}): RunnerClient {
  const base: RunnerClient = {
    listProjects: async () => [],
    listWorkflows: async () => [],
    listSteps: async () => [],
    listProviderAccounts: async () => [],
    listRunHistory: async () => [],
    listRemoteChatSessions: async () => [],
    listAgents: async () => [],
    listAgentRuns: async () => [],
    refreshAgentGraph: async () => ({ parentRunId: "current-run", runs: [], edges: [], busMessages: [], loopState: { status: "running", round: 0, roundCap: 3 } }),
    pauseAgentLoop: async () => ({ parentRunId: "current-run", runs: [], edges: [], busMessages: [], loopState: { status: "paused", round: 0, roundCap: 3 } }),
    resumeAgentLoop: async () => ({ parentRunId: "current-run", runs: [], edges: [], busMessages: [], loopState: { status: "running", round: 0, roundCap: 3 } }),
    injectAgentFeedback: async (_parentRunId, _toRunId, message) => ({ parentRunId: "current-run", runs: [], edges: [], busMessages: [{ id: "bus-1", parentRunId: "current-run", kind: "user-feedback", message, queued: true, occurredAt: "2026-01-01T00:00:00Z" }], loopState: { status: "running", round: 0, roundCap: 3 } }),
    stopAgentLoop: async () => ({ parentRunId: "current-run", runs: [], edges: [], busMessages: [], loopState: { status: "stopped", round: 0, roundCap: 3 } }),
    spawnAgent: async () => ({ runId: "agent-1", providerSessionId: "session-agent", providerKey: "codex", status: "completed" }),
    startRun: async () => ({ runId: "new-run", providerSessionId: "session-1", providerKey: "codex", status: "running" }),
    resumeRun: async (runId) => ({ runId, providerSessionId: "session-1", providerKey: "codex", status: "completed" }),
    syncChatRun: async (runId) => ({ runId, sourceMachineId: "mch_sync", sourceRunId: runId, syncStatus: "synced", syncedAt: "2026-06-17T10:10:00Z", remotePath: "chat-sessions/runs/mch_sync/" + runId + "/manifest.json" }),
    deleteRun: async () => {},
    restoreChatRun: async (input) => ({ runId: input.sourceRunId, sourceMachineId: input.sourceMachineId, sourceRunId: input.sourceRunId, providerKey: "codex", restoreStatus: "restored" }),
    sendTurn: () => emptyStream(),
    submitApproval: async () => {},
    answerQuestion: async () => {},
    interrupt: async () => {},
    streamRun: () => emptyStream(),
    focusAgentRun: () => emptyStream(),
    listArtifacts: async () => [],
    listSkills: async () => [],
    connectProviderAccount: async () => {},
    activateProviderAccount: async () => {},
    openProviderAccountTerminal: async () => {},
    restartStack: async () => {},
    shutdownStack: async () => {},
  };
  return { ...base, ...overrides };
}

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function seedStore(client: RunnerClient, runHistory: RunHistoryItem[]): void {
  useStore.setState({
    client,
    projects: [],
    workflows: [],
    steps: [],
    skills: [],
    providerAccounts: [],
    supportedModels: [],
    selectedProjectId: "project-1",
    selectedWorkflowId: undefined,
    selectedStepId: undefined,
    launchMode: "workflow",
    chatMode: "normal_chat",
    selectedProvider: "claude",
    selectedModel: undefined,
    reasoningEffort: undefined,
    yoloMode: false,
    chatStartMode: "normal",
    chatSourceDocId: "",
    runId: "current-run",
    mainRunId: "current-run",
    activeAgentRunId: undefined,
    agentRuns: [],
    agentBusMessages: [],
    activeStepId: "chat-current-run",
    status: "running",
    timeline: [{ kind: "prompt", id: "prompt-1", text: "keep current timeline" }],
    artifacts: [],
    runHistory,
    historyLoading: false,
    historyLoadError: undefined,
    pendingApproval: undefined,
    pendingQuestion: undefined,
    lastTurnInput: undefined,
    latestTokenUsage: undefined,
    recoverable: false,
    scenario: "normal",
    pendingAccountSwitch: undefined,
    accountSwitchLoading: false,
    _accountSwitchTriedIds: [],
    _streamingAssistantId: undefined,
    _historyLoadSeq: 0,
    _streamRunSeq: 0,
    _runSnapshots: {},
    _runReplaySeq: {},
  });
}

function makeAccount(
  id: string,
  providerKey: "codex" | "claude",
  overrides: Partial<ProviderAccountSummary> = {},
): ProviderAccountSummary {
  return {
    id,
    providerKey,
    displayName: `Account ${id}`,
    displayLabel: `account-${id}`,
    homePath: `/home/${id}`,
    authStorePath: null,
    slotIndex: 0,
    authStatus: "connected",
    isActive: false,
    createdAt: "2026-01-01T00:00:00Z",
    lastAuthenticatedAt: null,
    accountEmail: `${id}@example.com`,
    accountName: null,
    usageSummary: null,
    remaining5hPercent: 80,
    remaining7dPercent: 80,
    remaining5hResetAt: null,
    remaining7dResetAt: null,
    usageSource: "provider_api",
    accessTokenExpiresAt: null,
    refreshTokenExpiresAt: null,
    refreshTokenExpiryNote: null,
    usageDetailLines: [],
    ...overrides,
  };
}

const BASE_EVENT = {
  id: "ev-1",
  workflowRunId: "run-1",
  providerSessionId: "session-1",
  providerKey: "codex" as const,
  seq: 1,
  occurredAt: "2026-01-01T00:00:00Z",
};

async function* turnFailedStream(error: string, recoverable: boolean, workflowRunId = "run-1"): AsyncIterable<ProviderEventDTO> {
  yield { ...BASE_EVENT, workflowRunId, type: "turn_failed", error, recoverable };
}

async function* childFocusStream(): AsyncIterable<ProviderEventDTO> {
  yield { ...BASE_EVENT, workflowRunId: "child-run", type: "turn_started", providerTurnId: "child-turn", prompt: "child prompt" };
  yield { ...BASE_EVENT, workflowRunId: "child-run", type: "message_delta", text: "child response" };
  yield { ...BASE_EVENT, workflowRunId: "child-run", type: "turn_completed", finalMessage: "child response" };
}

async function* pendingChildStream(gate: Promise<void>): AsyncIterable<ProviderEventDTO> {
  yield { ...BASE_EVENT, workflowRunId: "child-run", type: "turn_started", providerTurnId: "child-turn", prompt: "child prompt" };
  await gate;
  yield { ...BASE_EVENT, workflowRunId: "child-run", type: "turn_completed", finalMessage: "done" };
}

async function* cursorChildStream(): AsyncIterable<ProviderEventDTO> {
  yield { ...BASE_EVENT, workflowRunId: "child-run", seq: 1, type: "turn_started", providerTurnId: "child-turn", prompt: "child prompt" };
  yield { ...BASE_EVENT, workflowRunId: "child-run", seq: 2, type: "message_delta", text: "old child chunk" };
  yield { ...BASE_EVENT, workflowRunId: "child-run", seq: 3, type: "message_delta", text: "new child chunk" };
}

test("openHistoryRun marks unavailable history entries on typed resume errors", async () => {
  seedStore(
    makeClient({
      resumeRun: async () => {
        throw new RunnerApiError(409, "session_unavailable", "session data not found on this machine");
      },
    }),
    [
      {
        runId: "run-1",
        projectId: "project-1",
        providerKey: "codex",
        status: "completed",
        startedAt: "2026-06-17T10:00:00Z",
        updatedAt: "2026-06-17T10:05:00Z",
      },
    ],
  );

  await useStore.getState().openHistoryRun("run-1");

  const state = useStore.getState();
  assert.equal(state.runId, "current-run");
  assert.deepEqual(state.timeline, [{ kind: "prompt", id: "prompt-1", text: "keep current timeline" }]);
  assert.equal(state.runHistory[0]?.unavailableReason, "session data not found on this machine");
});

test("openHistoryRun treats active-account-not-signed-in as unavailable instead of replacing the current run", async () => {
  seedStore(
    makeClient({
      resumeRun: async () => {
        throw new RunnerApiError(409, "account_not_signed_in", "can't open — the active account isn't signed in");
      },
    }),
    [
      {
        runId: "run-1",
        projectId: "project-1",
        providerKey: "codex",
        status: "completed",
        startedAt: "2026-06-17T10:00:00Z",
        updatedAt: "2026-06-17T10:05:00Z",
      },
    ],
  );

  await useStore.getState().openHistoryRun("run-1");

  const state = useStore.getState();
  assert.equal(state.runId, "current-run");
  assert.equal(state.runHistory[0]?.unavailableReason, "can't open — the active account isn't signed in");
});

test("openHistoryRun clears unavailableReason after a successful open", async () => {
  const handle: RunHandle = {
    runId: "run-1",
    providerSessionId: "session-1",
    providerKey: "codex",
    status: "completed",
    stepId: "chat-run-1",
  };
  seedStore(
    makeClient({
      resumeRun: async () => handle,
      streamRun: () => emptyStream(),
      listSkills: async () => [],
    }),
    [
      {
        runId: "run-1",
        projectId: "project-1",
        providerKey: "codex",
        status: "completed",
        startedAt: "2026-06-17T10:00:00Z",
        updatedAt: "2026-06-17T10:05:00Z",
        unavailableReason: "old reason",
      },
    ],
  );

  await useStore.getState().openHistoryRun("run-1");

  const state = useStore.getState();
  assert.equal(state.runId, "run-1");
  assert.equal(state.activeStepId, "chat-run-1");
  assert.equal(state.selectedProvider, "codex");
  assert.equal(state.runHistory[0]?.unavailableReason, undefined);
});

test("openHistoryRun returns after attaching an open-ended history stream", async () => {
  const streamStarted = deferred<void>();
  const handle: RunHandle = {
    runId: "run-1",
    providerSessionId: "session-1",
    providerKey: "codex",
    status: "running",
    stepId: "chat-run-1",
  };
  async function* hangingHistoryStream(): AsyncIterable<ProviderEventDTO> {
    streamStarted.resolve();
    yield { ...BASE_EVENT, type: "turn_started", providerTurnId: "turn-1", prompt: "old prompt" };
    await new Promise<never>(() => {});
  }
  seedStore(
    makeClient({
      resumeRun: async () => handle,
      streamRun: () => hangingHistoryStream(),
      listSkills: async () => [],
    }),
    [
      {
        runId: "run-1",
        projectId: "project-1",
        providerKey: "codex",
        status: "running",
        startedAt: "2026-06-17T10:00:00Z",
        updatedAt: "2026-06-17T10:05:00Z",
      },
    ],
  );

  await Promise.race([
    useStore.getState().openHistoryRun("run-1"),
    new Promise<never>((_, reject) => setTimeout(() => reject(new Error("openHistoryRun did not return")), 100)),
  ]);
  await Promise.race([
    streamStarted.promise,
    new Promise<never>((_, reject) => setTimeout(() => reject(new Error("history stream did not start")), 100)),
  ]);

  const state = useStore.getState();
  assert.equal(state.runId, "run-1");
  assert.equal(state.status, "running");
});

test("openHistoryRun does not leave Thinking visible for a completed history replay without a terminal event", async () => {
  const streamStarted = deferred<void>();
  const handle: RunHandle = {
    runId: "run-1",
    providerSessionId: "session-1",
    providerKey: "codex",
    status: "completed",
    stepId: "chat-run-1",
  };
  async function* incompleteCompletedHistoryStream(): AsyncIterable<ProviderEventDTO> {
    streamStarted.resolve();
    yield { ...BASE_EVENT, type: "turn_started", providerTurnId: "turn-1", prompt: "old prompt" };
    await new Promise<never>(() => {});
  }
  seedStore(
    makeClient({
      resumeRun: async () => handle,
      streamRun: () => incompleteCompletedHistoryStream(),
      listSkills: async () => [],
    }),
    [
      {
        runId: "run-1",
        projectId: "project-1",
        providerKey: "codex",
        status: "completed",
        startedAt: "2026-06-17T10:00:00Z",
        updatedAt: "2026-06-17T10:05:00Z",
      },
    ],
  );

  await useStore.getState().openHistoryRun("run-1");
  await Promise.race([
    streamStarted.promise,
    new Promise<never>((_, reject) => setTimeout(() => reject(new Error("history stream did not start")), 100)),
  ]);

  const state = useStore.getState();
  assert.equal(state.status, "completed");
  assert.equal(state.timeline.some((item) => item.kind === "thinking"), false);
});

test("openHistoryRun keeps an approval gate open when the replay ends on permission_required", async () => {
  const streamStarted = deferred<void>();
  const approvalDetails = { decisions: [{ value: "approve", label: "Approve" }, { value: "deny", label: "Deny" }] };
  const handle: RunHandle = {
    runId: "run-1",
    providerSessionId: "session-1",
    providerKey: "codex",
    status: "cancelled",
    stepId: "chat-run-1",
  };
  async function* approvalHistoryStream(): AsyncIterable<ProviderEventDTO> {
    streamStarted.resolve();
    yield {
      ...BASE_EVENT,
      type: "permission_required",
      approvalId: "appr-1",
      provider: "codex",
      details: approvalDetails,
    };
  }
  seedStore(
    makeClient({
      resumeRun: async () => handle,
      streamRun: () => approvalHistoryStream(),
      listSkills: async () => [],
    }),
    [
      {
        runId: "run-1",
        projectId: "project-1",
        providerKey: "codex",
        status: "cancelled",
        startedAt: "2026-06-17T10:00:00Z",
        updatedAt: "2026-06-17T10:05:00Z",
      },
    ],
  );

  await useStore.getState().openHistoryRun("run-1");
  await Promise.race([
    streamStarted.promise,
    new Promise<never>((_, reject) => setTimeout(() => reject(new Error("history stream did not start")), 100)),
  ]);

  const state = useStore.getState();
  assert.equal(state.status, "waiting_approval");
  assert.deepEqual(state.pendingApproval, { approvalId: "appr-1", details: approvalDetails });
  const card = state.timeline.find((item) => item.kind === "approval") as Extract<TimelineItem, { kind: "approval" }> | undefined;
  assert.equal(card?.decision, undefined, "approval card should remain actionable");
});

test("sendPrompt aborts an open-ended history replay stream before sending", async () => {
  const historyStreamStarted = deferred<void>();
  const historyStreamAborted = deferred<void>();
  const handle: RunHandle = {
    runId: "run-1",
    providerSessionId: "session-1",
    providerKey: "codex",
    status: "running",
    stepId: "chat-run-1",
  };
  async function* hangingHistoryStream(_runId: string, _afterSeq = 0, signal?: AbortSignal): AsyncIterable<ProviderEventDTO> {
    historyStreamStarted.resolve();
    yield { ...BASE_EVENT, type: "turn_started", providerTurnId: "turn-1", prompt: "old prompt" };
    await new Promise<void>((resolve) => {
      if (signal?.aborted) {
        historyStreamAborted.resolve();
        resolve();
        return;
      }
      signal?.addEventListener(
        "abort",
        () => {
          historyStreamAborted.resolve();
          resolve();
        },
        { once: true },
      );
    });
  }
  async function* completedSendTurn(input: TurnInput): AsyncIterable<ProviderEventDTO> {
    yield { ...BASE_EVENT, seq: 2, type: "turn_started", providerTurnId: "turn-2", prompt: input.prompt };
    yield { ...BASE_EVENT, seq: 3, type: "turn_completed", finalMessage: "done" };
  }
  seedStore(
    makeClient({
      resumeRun: async () => handle,
      streamRun: hangingHistoryStream,
      sendTurn: completedSendTurn,
      listSkills: async () => [],
    }),
    [
      {
        runId: "run-1",
        projectId: "project-1",
        providerKey: "codex",
        status: "running",
        startedAt: "2026-06-17T10:00:00Z",
        updatedAt: "2026-06-17T10:05:00Z",
      },
    ],
  );

  await useStore.getState().openHistoryRun("run-1");
  await Promise.race([
    historyStreamStarted.promise,
    new Promise<never>((_, reject) => setTimeout(() => reject(new Error("history stream did not start")), 100)),
  ]);
  await useStore.getState().sendPrompt("new prompt");
  await Promise.race([
    historyStreamAborted.promise,
    new Promise<never>((_, reject) => setTimeout(() => reject(new Error("history stream was not aborted")), 100)),
  ]);

  const state = useStore.getState();
  assert.equal(state.runId, "run-1");
  assert.equal(state.status, "completed");
});

test("sendPrompt blocks direct prompting while a child transcript is focused", async () => {
  let startCalls = 0;
  let sendCalls = 0;
  seedStore(
    makeClient({
      startRun: async () => {
        startCalls += 1;
        return { runId: "new-run", providerSessionId: "session-1", providerKey: "codex", status: "running" };
      },
      sendTurn: async function* () {
        sendCalls += 1;
      },
    }),
    [],
  );
  useStore.setState({
    runId: "child-run",
    mainRunId: "current-run",
    activeAgentRunId: "child-run",
    activeStepId: "chat-current-run",
    status: "completed",
  });

  await useStore.getState().sendPrompt("do not send");

  assert.equal(startCalls, 0);
  assert.equal(sendCalls, 0);
  const last = useStore.getState().timeline.at(-1);
  assert.equal(last?.kind, "system");
  assert.equal(last?.kind === "system" ? last.text : "", "Child transcript is read-only. Return to the main chat to send prompts.");
});

test("sendPrompt sends declared task intent only on the first chat turn", async () => {
  const seen: TurnInput[] = [];
  async function* completedSendTurn(input: TurnInput): AsyncIterable<ProviderEventDTO> {
    seen.push(input);
    yield { ...BASE_EVENT, seq: 2 + seen.length * 2, type: "turn_started", providerTurnId: `turn-${seen.length}`, prompt: input.prompt };
    yield { ...BASE_EVENT, seq: 3 + seen.length * 2, type: "turn_completed", finalMessage: "done" };
  }

  seedStore(
    makeClient({
      startRun: async () => ({ runId: "new-run", providerSessionId: "session-1", providerKey: "codex", status: "running", stepId: "chat-new-run" }),
      sendTurn: completedSendTurn,
      listRunHistory: async () => [],
    }),
    [],
  );

  useStore.setState({
    runId: undefined,
    mainRunId: undefined,
    activeStepId: undefined,
    status: "idle",
    timeline: [],
    selectedProvider: "codex",
    chatStartMode: "task",
    chatSourceDocId: "Task-114",
  });

  await useStore.getState().sendPrompt("first");
  await useStore.getState().sendPrompt("second");

  assert.equal(seen.length, 2);
  assert.equal(seen[0]?.changeType, "task");
  assert.equal(seen[0]?.sourceDocId, "Task-114");
  assert.equal(seen[1]?.changeType, undefined);
  assert.equal(seen[1]?.sourceDocId, undefined);
});

test("openHistoryRun selects the resumed provider default model", async () => {
  const handle: RunHandle = {
    runId: "run-claude",
    providerSessionId: "session-claude",
    providerKey: "claude",
    status: "completed",
    stepId: "chat-run-claude",
  };
  seedStore(
    makeClient({
      resumeRun: async () => handle,
      streamRun: () => emptyStream(),
      listSkills: async () => [],
    }),
    [
      {
        runId: "run-claude",
        projectId: "project-1",
        providerKey: "claude",
        status: "completed",
        startedAt: "2026-06-17T10:00:00Z",
        updatedAt: "2026-06-17T10:05:00Z",
      },
    ],
  );
  useStore.setState({
    selectedProvider: "codex",
    selectedModel: "o4-mini",
    supportedModels: [
      {
        id: "model-codex",
        providerKey: "codex",
        modelId: "o4-mini",
        displayName: "o4-mini",
        isEnabled: true,
        sortOrder: 0,
        source: "test",
        detectionMethod: null,
        detectedCliVersion: null,
        lastDetectedAt: null,
        createdAt: "2026-06-19T00:00:00Z",
        updatedAt: "2026-06-19T00:00:00Z",
      },
      {
        id: "model-claude",
        providerKey: "claude",
        modelId: "claude-sonnet-4",
        displayName: "Claude Sonnet 4",
        isEnabled: true,
        sortOrder: 0,
        source: "test",
        detectionMethod: null,
        detectedCliVersion: null,
        lastDetectedAt: null,
        createdAt: "2026-06-19T00:00:00Z",
        updatedAt: "2026-06-19T00:00:00Z",
      },
    ],
  });

  await useStore.getState().openHistoryRun("run-claude");

  const state = useStore.getState();
  assert.equal(state.selectedProvider, "claude");
  assert.equal(state.selectedModel, "claude-sonnet-4");
});

test("selectProvider defaults Codex to a full model before mini", () => {
  seedStore(makeClient(), []);
  useStore.setState({
    selectedProvider: "claude",
    selectedModel: undefined,
    supportedModels: [
      {
        id: "model-mini",
        providerKey: "codex",
        modelId: "gpt-5.4-mini",
        displayName: "gpt-5.4-mini",
        isEnabled: true,
        sortOrder: 0,
        source: "test",
        detectionMethod: null,
        detectedCliVersion: null,
        lastDetectedAt: null,
        createdAt: "2026-06-19T00:00:00Z",
        updatedAt: "2026-06-19T00:00:00Z",
      },
      {
        id: "model-full",
        providerKey: "codex",
        modelId: "gpt-5.5",
        displayName: "gpt-5.5",
        isEnabled: true,
        sortOrder: 1,
        source: "test",
        detectionMethod: null,
        detectedCliVersion: null,
        lastDetectedAt: null,
        createdAt: "2026-06-19T00:00:00Z",
        updatedAt: "2026-06-19T00:00:00Z",
      },
    ],
  });

  useStore.getState().selectProvider("codex");

  assert.equal(useStore.getState().selectedModel, "gpt-5.5");
});

test("refreshAgentGraph stores orchestration snapshot from the client", async () => {
	seedStore(makeClient(), []);
	await useStore.getState().refreshAgentGraph();
	assert.equal(useStore.getState().agentGraphSnapshot?.loopState.roundCap, 3);
});

test("refreshAgentGraph keeps agent bus messages aligned with the refreshed snapshot", async () => {
  const busMessage = {
    id: "bus-graph",
    parentRunId: "current-run",
    kind: "handoff",
    message: "ready-for-review",
    queued: true,
    occurredAt: "2026-01-01T00:00:00Z",
  };
  seedStore(
    makeClient({
      refreshAgentGraph: async () => ({
        parentRunId: "current-run",
        runs: [],
        edges: [],
        busMessages: [busMessage],
        loopState: { status: "paused", round: 2, roundCap: 3, gateReason: "waiting on child" },
      }),
    }),
    [],
  );

  await useStore.getState().refreshAgentGraph();

  assert.deepEqual(useStore.getState().agentBusMessages, [busMessage]);
  assert.equal(useStore.getState().agentGraphSnapshot?.loopState.gateReason, "waiting on child");
});

test("sendPrompt applies live agent graph and bus SSE updates", async () => {
  const stream = async function* (): AsyncIterable<ProviderEventDTO> {
    yield {
      ...BASE_EVENT,
      type: "agent_graph_updated",
      agentGraphSnapshot: {
        parentRunId: "current-run",
        runs: [],
        edges: [],
        busMessages: [],
        loopState: { status: "running", round: 1, roundCap: 3 },
      },
    };
    yield {
      ...BASE_EVENT,
      type: "agent_bus_message",
      agentBusMessage: {
        id: "bus-1",
        parentRunId: "current-run",
        kind: "handoff",
        message: "ready-for-review",
        queued: false,
        occurredAt: "2026-01-01T00:00:00Z",
      },
    };
    yield { ...BASE_EVENT, type: "turn_completed", finalMessage: "ok" };
  };
  seedStore(
    makeClient({
      sendTurn: () => stream(),
      startRun: async () => ({ runId: "current-run", providerSessionId: "session-1", providerKey: "codex", status: "running", stepId: "chat-current-run" }),
    }),
    [],
  );

  await useStore.getState().sendPrompt("hello");

  assert.equal(useStore.getState().agentGraphSnapshot?.loopState.round, 1);
  assert.equal(useStore.getState().agentBusMessages.at(-1)?.message, "ready-for-review");
});

test("refreshAgentRuns loads child summaries for the active main run", async () => {
  const agentRuns: AgentRunSummary[] = [
    {
      runId: "agent-1",
      agentName: "architect",
      role: "architecture",
      status: "running",
      parentRunId: "current-run",
      createdAt: "2026-06-17T10:01:00Z",
      agentStatus: "running",
    },
  ];
  seedStore(
    makeClient({
      listAgentRuns: async () => agentRuns,
    }),
    [],
  );

  await useStore.getState().refreshAgentRuns();

  assert.deepEqual(useStore.getState().agentRuns, agentRuns);
});

test("focusAgentRun caches the main timeline and backToMainRun restores it", async () => {
  seedStore(
    makeClient({
      focusAgentRun: () => childFocusStream(),
    }),
    [],
  );
  useStore.setState({
    runId: "current-run",
    mainRunId: "current-run",
    activeAgentRunId: undefined,
    timeline: [{ kind: "prompt", id: "prompt-main", text: "main timeline" }],
    status: "running",
    artifacts: [],
  });

  await useStore.getState().focusAgentRun("child-run");
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(useStore.getState().runId, "child-run");
  assert.equal(useStore.getState().activeAgentRunId, "child-run");
  assert.ok(useStore.getState().timeline.some((item) => item.kind === "assistant"));

  useStore.getState().backToMainRun();

  assert.equal(useStore.getState().runId, "current-run");
  assert.equal(useStore.getState().activeAgentRunId, undefined);
  assert.deepEqual(useStore.getState().timeline, [{ kind: "prompt", id: "prompt-main", text: "main timeline" }]);
});

test("focusAgentRun resumes the child run before opening its event stream", async () => {
  let resumed = false;
  seedStore(
    makeClient({
      resumeRun: async (runId) => {
        assert.equal(runId, "child-run");
        resumed = true;
        return { runId, providerSessionId: "session-child", providerKey: "codex", status: "completed" };
      },
      focusAgentRun: async function* () {
        assert.equal(resumed, true);
        yield* childFocusStream();
      },
    }),
    [],
  );
  useStore.setState({
    runId: "current-run",
    mainRunId: "current-run",
    activeAgentRunId: undefined,
    timeline: [{ kind: "prompt", id: "prompt-main", text: "main timeline" }],
    status: "running",
    artifacts: [],
  });

  await useStore.getState().focusAgentRun("child-run");
  await new Promise((resolve) => setTimeout(resolve, 0));

  assert.equal(resumed, true);
  assert.equal(useStore.getState().runId, "child-run");
  assert.ok(useStore.getState().timeline.some((item) => item.kind === "assistant"));
});

test("focusAgentRun replays from start when no child snapshot is cached", async () => {
  seedStore(
    makeClient({
      focusAgentRun: () => cursorChildStream(),
    }),
    [],
  );
  useStore.setState({
    runId: "current-run",
    mainRunId: "current-run",
    activeAgentRunId: undefined,
    timeline: [{ kind: "prompt", id: "prompt-main", text: "main timeline" }],
    status: "running",
    artifacts: [],
    _runReplaySeq: { "child-run": 2 },
  });

  await useStore.getState().focusAgentRun("child-run");
  await new Promise((resolve) => setTimeout(resolve, 0));

  const assistantTexts = useStore.getState().timeline
    .filter((item) => item.kind === "assistant")
    .map((item) => (item.kind === "assistant" ? item.text : ""));
  assert.deepEqual(assistantTexts, ["old child chunknew child chunk"]);
});

test("focusAgentRun resumes from the last replay cursor when a child snapshot is cached", async () => {
  seedStore(
    makeClient({
      focusAgentRun: () => cursorChildStream(),
    }),
    [],
  );
  useStore.setState({
    runId: "current-run",
    mainRunId: "current-run",
    activeAgentRunId: undefined,
    timeline: [{ kind: "prompt", id: "prompt-main", text: "main timeline" }],
    status: "running",
    artifacts: [],
    _runReplaySeq: { "child-run": 2 },
    _runSnapshots: {
      "child-run": {
        timeline: [],
        artifacts: [],
        status: "running",
        recoverable: false,
        lastEventSeq: 2,
      },
    },
  });

  await useStore.getState().focusAgentRun("child-run");
  await new Promise((resolve) => setTimeout(resolve, 0));

  const assistantTexts = useStore.getState().timeline
    .filter((item) => item.kind === "assistant")
    .map((item) => (item.kind === "assistant" ? item.text : ""));
  assert.deepEqual(assistantTexts, ["new child chunk"]);
});

test("focusAgentRun returns before a child stream finishes", async () => {
  const gate = deferred<void>();
  seedStore(
    makeClient({
      focusAgentRun: () => pendingChildStream(gate.promise),
    }),
    [],
  );
  useStore.setState({
    runId: "current-run",
    mainRunId: "current-run",
    activeAgentRunId: undefined,
    timeline: [{ kind: "prompt", id: "prompt-main", text: "main timeline" }],
    status: "running",
    artifacts: [],
  });

  const focusPromise = useStore.getState().focusAgentRun("child-run");
  await Promise.race([focusPromise, Promise.resolve()]);
  assert.equal(useStore.getState().runId, "child-run");
  assert.equal(useStore.getState().activeAgentRunId, "child-run");
  gate.resolve();
  await focusPromise;
});

test("focusAgentRun ignores replay events that belong to the main run", async () => {
  seedStore(
    makeClient({
      resumeRun: async (runId) => ({ runId, providerSessionId: "session-child", providerKey: "codex", status: "completed" }),
      focusAgentRun: async function* (): AsyncIterable<ProviderEventDTO> {
        yield { ...BASE_EVENT, workflowRunId: "current-run", seq: 1, type: "message_delta", text: "wrong main replay" };
        yield { ...BASE_EVENT, workflowRunId: "child-run", seq: 2, type: "message_delta", text: "child replay" };
      },
    }),
    [],
  );
  useStore.setState({
    runId: "current-run",
    mainRunId: "current-run",
    activeAgentRunId: undefined,
    timeline: [{ kind: "prompt", id: "prompt-main", text: "main timeline" }],
    status: "completed",
    artifacts: [],
  });

  await useStore.getState().focusAgentRun("child-run");
  await new Promise((resolve) => setTimeout(resolve, 0));

  const assistantTexts = useStore.getState().timeline
    .filter((item) => item.kind === "assistant")
    .map((item) => (item.kind === "assistant" ? item.text : ""));
  assert.deepEqual(assistantTexts, ["child replay"]);
  assert.equal(useStore.getState().timeline.some((item) => item.kind === "thinking"), false);
});

test("focusAgentRun clears stale parent assistant accumulator before child replay", async () => {
  seedStore(
    makeClient({
      resumeRun: async (runId) => ({ runId, providerSessionId: "session-child", providerKey: "codex", status: "completed" }),
      focusAgentRun: async function* (): AsyncIterable<ProviderEventDTO> {
        yield { ...BASE_EVENT, workflowRunId: "child-run", seq: 1, type: "turn_started", providerTurnId: "child-turn", prompt: "child prompt" };
        yield { ...BASE_EVENT, workflowRunId: "child-run", seq: 2, type: "message_delta", text: "CHILD_AGENT_DONE" };
        yield { ...BASE_EVENT, workflowRunId: "child-run", seq: 3, type: "turn_completed", finalMessage: "CHILD_AGENT_DONE" };
      },
    }),
    [],
  );
  useStore.setState({
    runId: "current-run",
    mainRunId: "current-run",
    activeAgentRunId: undefined,
    timeline: [{ kind: "assistant", id: "main-assistant", text: "parent response", finalized: false }],
    status: "running",
    artifacts: [],
    _streamingAssistantId: "main-assistant",
  });

  await useStore.getState().focusAgentRun("child-run");
  await new Promise((resolve) => setTimeout(resolve, 0));

  const state = useStore.getState();
  assert.equal(state._streamingAssistantId, undefined);
  assert.equal(state.timeline.some((item) => item.kind === "assistant" && item.text.includes("parent response")), false);
  assert.deepEqual(
    state.timeline.filter((item) => item.kind === "assistant").map((item) => item.text),
    ["CHILD_AGENT_DONE"],
  );
});

test("applyTimelineEvent starts a new assistant bubble for a new turn", () => {
  const base: TimelineState = {
    status: "running",
    recoverable: false,
    _streamingAssistantId: "assistant-main",
    timeline: [{ kind: "assistant", id: "assistant-main", text: "parent response", finalized: false }],
  };

  const started = applyTimelineEvent(base, {
    ...BASE_EVENT,
    workflowRunId: "current-run",
    providerTurnId: "turn-2",
    type: "turn_started",
    prompt: "second prompt",
  });
  const next = applyTimelineEvent({ ...base, ...started }, {
    ...BASE_EVENT,
    workflowRunId: "current-run",
    providerTurnId: "turn-2",
    id: "assistant-child",
    type: "message_delta",
    text: "new response",
  });

  const assistantTexts = next.timeline
    ?.filter((item) => item.kind === "assistant")
    .map((item) => (item.kind === "assistant" ? item.text : ""));
  assert.deepEqual(assistantTexts, ["parent response", "new response"]);
});

test("backToMainRun aborts its replay stream before focusing a child again", async () => {
  const mainStreamStarted = deferred<void>();
  const mainStreamAborted = deferred<void>();
  async function* mainReplayStream(_runId: string, _afterSeq = 0, signal?: AbortSignal): AsyncIterable<ProviderEventDTO> {
    mainStreamStarted.resolve();
    await new Promise<void>((resolve) => {
      if (signal?.aborted) {
        mainStreamAborted.resolve();
        resolve();
        return;
      }
      signal?.addEventListener(
        "abort",
        () => {
          mainStreamAborted.resolve();
          resolve();
        },
        { once: true },
      );
    });
  }
  seedStore(
    makeClient({
      streamRun: mainReplayStream,
      focusAgentRun: () => emptyStream(),
      resumeRun: async (runId) => ({ runId, providerSessionId: "session-child", providerKey: "codex", status: "completed" }),
    }),
    [],
  );
  useStore.setState({
    runId: "child-run",
    mainRunId: "current-run",
    activeAgentRunId: "child-run",
    timeline: [{ kind: "assistant", id: "child-message", text: "child", finalized: true }],
    status: "completed",
    artifacts: [],
    _runSnapshots: {
      "current-run": {
        timeline: [{ kind: "assistant", id: "main-message", text: "main", finalized: true }],
        artifacts: [],
        status: "completed",
        recoverable: false,
      },
    },
  });

  useStore.getState().backToMainRun();
  await Promise.race([
    mainStreamStarted.promise,
    new Promise<never>((_, reject) => setTimeout(() => reject(new Error("main replay stream did not start")), 100)),
  ]);

  await useStore.getState().focusAgentRun("child-run");

  await Promise.race([
    mainStreamAborted.promise,
    new Promise<never>((_, reject) => setTimeout(() => reject(new Error("main replay stream was not aborted")), 100)),
  ]);
});

test("injectAgentFeedback queues busy mention feedback on the board state", async () => {
  seedStore(
    makeClient({
      injectAgentFeedback: async (_parentRunId, _toRunId, message) => ({
        parentRunId: "current-run",
        runs: [],
        edges: [],
        busMessages: [{ id: "bus-1", parentRunId: "current-run", kind: "user-feedback", message, queued: true, occurredAt: "2026-01-01T00:00:00Z" }],
        loopState: { status: "paused", round: 0, roundCap: 3, gateReason: "queued user-feedback" },
      }),
    }),
    [],
  );
  await useStore.getState().injectAgentFeedback("agent-2", "please revise");
  assert.equal(useStore.getState().agentGraphSnapshot?.loopState.gateReason, "queued user-feedback");
  assert.equal(useStore.getState().agentBusMessages.at(-1)?.kind, "user-feedback");
});

test("orchestration board helpers surface empty and queued states", () => {
  assert.equal(getOrchestrationBoardEmptyCopy(undefined, []), "No graph snapshot loaded. Refresh to inspect the loop.");
  assert.equal(getOrchestrationBoardEmptyCopy({
    parentRunId: "current-run",
    runs: [],
    edges: [],
    busMessages: [],
    loopState: { status: "running", round: 0, roundCap: 3 },
  }, []), "No child agents yet.");
  assert.equal(getBusMessageLabel({
    id: "bus-1",
    parentRunId: "current-run",
    kind: "user-feedback",
    message: "please revise",
    queued: true,
    occurredAt: "2026-01-01T00:00:00Z",
  }), "user-feedback: please revise (queued)");
});

test("openAgentSpawnGuide overwrites the preselected agent and clearAgentSpawnGuide resets it", async () => {
  seedStore(makeClient(), []);
  useStore.getState().openAgentSpawnGuide("architect");
  assert.equal(useStore.getState().agentSpawnGuideOpen, true);
  assert.equal(useStore.getState().agentSpawnGuideAgentName, "architect");

  useStore.getState().openAgentSpawnGuide("reviewer");
  assert.equal(useStore.getState().agentSpawnGuideOpen, true);
  assert.equal(useStore.getState().agentSpawnGuideAgentName, "reviewer");

  useStore.getState().clearAgentSpawnGuide();
  assert.equal(useStore.getState().agentSpawnGuideOpen, false);
  assert.equal(useStore.getState().agentSpawnGuideAgentName, undefined);
});

// BUG-109: orchestration stream advances _runReplaySeq[mainRunId] via agent_graph_updated
// events while viewing a child. backToMainRun must use restore.lastEventSeq (not the
// inflated _runReplaySeq) so it doesn't skip real timeline events in the gap.
test("backToMainRun replays from snapshot lastEventSeq, not from orchestration-inflated _runReplaySeq", async () => {
  const mainStreamSeqs: number[] = [];
  async function* mainStream(_runId: string, afterSeq = 0): AsyncIterable<ProviderEventDTO> {
    mainStreamSeqs.push(afterSeq);
    // No events — just record what afterSeq was used
  }

  seedStore(
    makeClient({
      streamRun: mainStream,
      focusAgentRun: () => emptyStream() as AsyncIterable<ProviderEventDTO>,
      resumeRun: async (runId) => ({ runId, providerSessionId: "session-child", providerKey: "codex", status: "completed" }),
    }),
    [],
  );
  // Real flow: the user is VIEWING the child (runId === child-run) when they click
  // "back to main agent". backToMainRun snapshots the child and restores the main
  // snapshot captured earlier during focusAgentRun.
  useStore.setState({
    runId: "child-run",
    mainRunId: "current-run",
    activeAgentRunId: "child-run",
    timeline: [{ kind: "assistant", id: "child-a", text: "child output", finalized: true }],
    status: "completed",
    artifacts: [],
    _runSnapshots: {
      "current-run": {
        timeline: [{ kind: "prompt", id: "main-prompt", text: "main prompt" }],
        artifacts: [],
        status: "running",
        recoverable: false,
        lastEventSeq: 10,
      },
    },
    // _runReplaySeq[mainRunId] inflated to 50 by orchestration stream processing
    // agent_graph_updated events (seq 11..50) while the user was viewing the child.
    _runReplaySeq: { "current-run": 50, "child-run": 3 },
  });

  useStore.getState().backToMainRun();
  await new Promise((resolve) => setTimeout(resolve, 0));

  // The timeline replay (consumeAgentStream, the FIRST streamRun call) must start from
  // the snapshot's lastEventSeq (10), NOT the orchestration-inflated _runReplaySeq (50).
  // Starting at 50 would skip real timeline events with seqs 11..50 (e.g. message_completed).
  // The orchestration stream legitimately resumes from 50 (the second call). (BUG-109)
  assert.equal(mainStreamSeqs[0], 10, "timeline replay must use snapshot lastEventSeq");
  assert.equal(mainStreamSeqs.includes(50), true, "orchestration stream resumes from its cursor");
});

// BUG-109: consumeAgentStream must not re-process agent_graph_updated / agent_bus_message
// events when replaying the gap between restore.lastEventSeq and the orchestration-advanced
// _runReplaySeq. Doing so would append duplicate entries to agentBusMessages.
test("backToMainRun replaying the gap does not duplicate agentBusMessages", async () => {
  const busMessage = {
    id: "bus-1",
    parentRunId: "current-run",
    kind: "handoff" as const,
    message: "work done",
    queued: false,
    occurredAt: "2026-01-01T00:00:00Z",
  };

  async function* mainStream(_runId: string, _afterSeq = 0): AsyncIterable<ProviderEventDTO> {
    // This event was already processed by the orchestration stream while in child view.
    // consumeAgentStream must skip it to avoid duplicating the bus message.
    yield {
      ...BASE_EVENT,
      seq: 15,
      workflowRunId: "current-run",
      type: "agent_bus_message",
      agentBusMessage: busMessage,
    };
  }

  seedStore(
    makeClient({
      streamRun: mainStream,
      focusAgentRun: () => emptyStream() as AsyncIterable<ProviderEventDTO>,
      resumeRun: async (runId) => ({ runId, providerSessionId: "session-child", providerKey: "codex", status: "completed" }),
    }),
    [],
  );
  useStore.setState({
    runId: "current-run",
    mainRunId: "current-run",
    activeAgentRunId: undefined,
    timeline: [{ kind: "prompt", id: "main-prompt", text: "main prompt" }],
    status: "running",
    artifacts: [],
    // agentBusMessages already has the bus message from the orchestration stream
    agentBusMessages: [busMessage],
    _runSnapshots: {
      "current-run": {
        timeline: [{ kind: "prompt", id: "main-prompt", text: "main prompt" }],
        artifacts: [],
        status: "running",
        recoverable: false,
        lastEventSeq: 10,
      },
    },
    _runReplaySeq: { "current-run": 50 },
  });

  useStore.getState().backToMainRun();
  await new Promise((resolve) => setTimeout(resolve, 0));

  // agentBusMessages must stay at 1 entry — consumeAgentStream skips orchestration events.
  assert.equal(useStore.getState().agentBusMessages.length, 1);
});

test("parseMentionRouting resolves missing, busy, and idle child targets", () => {
  const runs = [
    { agentName: "architect", runId: "agent-1", status: "completed" },
    { agentName: "builder", runId: "agent-2", status: "running" },
  ];

  assert.deepEqual(parseMentionRouting("@architect do work", runs), {
    kind: "focus",
    agentName: "architect",
    runId: "agent-1",
    prompt: "do work",
  });
  assert.deepEqual(parseMentionRouting("@builder do work", runs), {
    kind: "busy",
    agentName: "builder",
    runId: "agent-2",
    prompt: "do work",
  });
  assert.deepEqual(parseMentionRouting("@reviewer do work", runs), {
    kind: "missing",
    agentName: "reviewer",
  });
});

test("shouldShowAgentTimelineHeader stays hidden for single-agent runs", async () => {
  assert.equal(shouldShowAgentTimelineHeader(undefined, undefined, 0), false);
  assert.equal(shouldShowAgentTimelineHeader("current-run", "current-run", 0), false);
  assert.equal(shouldShowAgentTimelineHeader(undefined, "current-run", 0), false);
  assert.equal(shouldShowAgentTimelineHeader("child-run", "current-run", 1), true);
});

test("syncHistoryRun updates local run sync metadata", async () => {
  seedStore(makeClient(), [
    {
      runId: "run-sync",
      projectId: "project-1",
      providerKey: "codex",
      status: "completed",
      startedAt: "2026-06-17T10:00:00Z",
      updatedAt: "2026-06-17T10:05:00Z",
      runKind: "chat",
    },
  ]);

  await useStore.getState().syncHistoryRun("run-sync");

  const state = useStore.getState();
  assert.equal(state.runHistory[0]?.syncStatus, "synced");
  assert.equal(state.runHistory[0]?.sourceMachineId, "mch_sync");
  assert.equal(state.runHistory[0]?.sourceRunId, "run-sync");
});

test("syncHistoryRun marks syncing state while the request is in flight", async () => {
  const gate = deferred<{
    runId: string;
    sourceMachineId: string;
    sourceRunId: string;
    syncStatus: string;
    syncedAt: string;
    remotePath: string;
  }>();
  seedStore(
    makeClient({
      syncChatRun: async () => gate.promise,
    }),
    [
      {
        runId: "run-sync",
        projectId: "project-1",
        providerKey: "codex",
        status: "completed",
        startedAt: "2026-06-17T10:00:00Z",
        updatedAt: "2026-06-17T10:05:00Z",
        runKind: "chat",
      },
    ],
  );

  const pending = useStore.getState().syncHistoryRun("run-sync");
  assert.equal(useStore.getState().runHistory[0]?.syncStatus, "syncing");

  gate.resolve({
    runId: "run-sync",
    sourceMachineId: "mch_sync",
    sourceRunId: "run-sync",
    syncStatus: "synced",
    syncedAt: "2026-06-17T10:10:00Z",
    remotePath: "chat-sessions/runs/mch_sync/run-sync/manifest.json",
  });
  await pending;
});

test("syncHistoryRun typed error preserves current timeline", async () => {
  seedStore(
    makeClient({
      syncChatRun: async () => {
        throw new RunnerApiError(409, "session_unavailable", "session data not found on this machine");
      },
    }),
    [
      {
        runId: "run-sync",
        projectId: "project-1",
        providerKey: "codex",
        status: "completed",
        startedAt: "2026-06-17T10:00:00Z",
        updatedAt: "2026-06-17T10:05:00Z",
        runKind: "chat",
      },
    ],
  );

  await useStore.getState().syncHistoryRun("run-sync");

  const state = useStore.getState();
  assert.equal(state.runId, "current-run");
  assert.deepEqual(state.timeline, [{ kind: "prompt", id: "prompt-1", text: "keep current timeline" }]);
  assert.equal(state.runHistory[0]?.syncStatus, "failed");
  assert.equal(state.runHistory[0]?.unavailableReason, "session data not found on this machine");
});

test("loadRemoteChatSessions stores remote summaries", async () => {
  const summary: RemoteChatSessionSummary = {
    runId: "run-remote",
    projectId: "project-1",
    providerKey: "codex",
    sourceMachineId: "mch_remote",
    sourceRunId: "run-remote",
  };
  seedStore(
    makeClient({
      listRemoteChatSessions: async () => [summary],
    }),
    [],
  );

  await useStore.getState().loadRemoteChatSessions();

  const state = useStore.getState();
  assert.deepEqual(state.remoteChatSessions, [summary]);
  assert.equal(state.remoteHistoryLoading, false);
  assert.equal(state.remoteHistoryLoadError, undefined);
});

test("restoreRemoteChatSession retries cwd_remap_required with the selected project path", async () => {
  const calls: string[] = [];
  const summary: RemoteChatSessionSummary = {
    runId: "run-remote",
    projectId: "project-1",
    providerKey: "codex",
    sourceMachineId: "mch_remote",
    sourceRunId: "run-remote",
  };
  seedStore(
    makeClient({
      listRunHistory: async () => [],
      listRemoteChatSessions: async () => [summary],
      restoreChatRun: async (input) => {
        calls.push(input.cwd ?? "");
        if (!input.cwd) {
          throw new RunnerApiError(409, "cwd_remap_required", "select a local project path before restoring this chat");
        }
        return {
          runId: input.sourceRunId,
          sourceMachineId: input.sourceMachineId,
          sourceRunId: input.sourceRunId,
          providerKey: "codex",
          restoreStatus: "restored",
        };
      },
    }),
    [],
  );
  useStore.setState({
    projects: [{ id: "project-1", name: "Project 1", path: "/tmp/project-1" }],
    remoteChatSessions: [summary],
  });

  await useStore.getState().restoreRemoteChatSession(summary);

  assert.deepEqual(calls, ["", "/tmp/project-1"]);
});

test("restoreRemoteChatSession adds restored run to history", async () => {
  const summary: RemoteChatSessionSummary = {
    runId: "run-remote",
    projectId: "project-1",
    providerKey: "codex",
    sourceMachineId: "mch_remote",
    sourceRunId: "run-remote",
  };
  const restoredHistory: RunHistoryItem = {
    runId: "run-remote",
    projectId: "project-1",
    providerKey: "codex",
    status: "completed",
    startedAt: "2026-06-17T10:00:00Z",
    updatedAt: "2026-06-17T10:05:00Z",
    sourceMachineId: "mch_remote",
    sourceRunId: "run-remote",
    syncStatus: "restored",
  };
  seedStore(
    makeClient({
      restoreChatRun: async () => ({
        runId: "run-remote",
        sourceMachineId: "mch_remote",
        sourceRunId: "run-remote",
        providerKey: "codex",
        restoreStatus: "restored",
      }),
      listRunHistory: async () => [restoredHistory],
      listRemoteChatSessions: async () => [summary],
    }),
    [],
  );
  useStore.setState({ remoteChatSessions: [summary] });

  await useStore.getState().restoreRemoteChatSession(summary);

  const state = useStore.getState();
  assert.equal(state.runHistory[0]?.runId, "run-remote");
  assert.equal(state.runHistory[0]?.providerKey, "codex");
});

test("restoreRemoteChatSession marks remote entries unavailable on typed restore errors", async () => {
  const summary: RemoteChatSessionSummary = {
    runId: "run-remote",
    projectId: "project-1",
    providerKey: "codex",
    sourceMachineId: "mch_remote",
    sourceRunId: "run-remote",
  };
  seedStore(
    makeClient({
      restoreChatRun: async () => {
        throw new RunnerApiError(409, "sync_integrity_failed", "remote provider session file failed integrity validation");
      },
    }),
    [],
  );
  useStore.setState({ remoteChatSessions: [summary] });

  await useStore.getState().restoreRemoteChatSession(summary);

  assert.equal(useStore.getState().remoteChatSessions[0]?.unavailableReason, "remote provider session file failed integrity validation");
});

// ── Account-switch tests ───────────────────────────────────────────────────

test("usage-limit turn_failed with valid Codex candidate sets pendingAccountSwitch", async () => {
  const acc1 = makeAccount("acc-1", "codex", { isActive: true, remaining5hPercent: 0, remaining7dPercent: 90, slotIndex: 0 });
  const acc2 = makeAccount("acc-2", "codex", { isActive: false, remaining5hPercent: 50, remaining7dPercent: 60, slotIndex: 1 });
  seedStore(
    makeClient({
      startRun: async () => ({ runId: "run-1", providerSessionId: "s1", providerKey: "codex", status: "running", stepId: "chat-run-1" }),
      sendTurn: () => turnFailedStream("usage limit reached", false),
      listRunHistory: async () => [],
    }),
    [],
  );
  useStore.setState({ providerAccounts: [acc1, acc2], selectedProvider: "codex", runId: undefined, activeStepId: undefined, status: "idle", timeline: [] });

  await useStore.getState().sendPrompt("hello");

  const state = useStore.getState();
  assert.ok(state.pendingAccountSwitch !== undefined, "pendingAccountSwitch should be set");
  assert.equal(state.pendingAccountSwitch?.failedAccountId, "acc-1");
  assert.equal(state.pendingAccountSwitch?.candidateAccount.id, "acc-2");
  assert.deepEqual(state._accountSwitchTriedIds, ["acc-1"]);
});

test("candidate ranking: higher remaining5hPercent wins over higher remaining7dPercent", async () => {
  const active = makeAccount("active", "codex", { isActive: true, remaining5hPercent: 0, remaining7dPercent: 0, slotIndex: 0 });
  const accA  = makeAccount("acc-A",  "codex", { isActive: false, remaining5hPercent: 80, remaining7dPercent: 20, slotIndex: 1 });
  const accB  = makeAccount("acc-B",  "codex", { isActive: false, remaining5hPercent: 60, remaining7dPercent: 90, slotIndex: 2 });
  seedStore(
    makeClient({
      startRun: async () => ({ runId: "run-1", providerSessionId: "s1", providerKey: "codex", status: "running", stepId: "chat-run-1" }),
      sendTurn: () => turnFailedStream("usage limit reached", false),
      listRunHistory: async () => [],
    }),
    [],
  );
  useStore.setState({ providerAccounts: [active, accA, accB], selectedProvider: "codex", runId: undefined, activeStepId: undefined, status: "idle", timeline: [] });

  await useStore.getState().sendPrompt("hello");

  assert.equal(useStore.getState().pendingAccountSwitch?.candidateAccount.id, "acc-A");
});

test("account with remaining5hPercent=0 is excluded from ranked path; no switch offered when no fallback exists", async () => {
  const active = makeAccount("active", "codex", { isActive: true, remaining5hPercent: 0, slotIndex: 0 });
  const zeroH  = makeAccount("zero-5h", "codex", { isActive: false, remaining5hPercent: 0, remaining7dPercent: 80, slotIndex: 1 });
  seedStore(
    makeClient({
      startRun: async () => ({ runId: "run-1", providerSessionId: "s1", providerKey: "codex", status: "running", stepId: "chat-run-1" }),
      sendTurn: () => turnFailedStream("usage limit reached", false),
      listRunHistory: async () => [],
    }),
    [],
  );
  // zeroH has 5h=0 → excluded from ranked path; no null-quota fallback either
  useStore.setState({ providerAccounts: [active, zeroH], selectedProvider: "codex", runId: undefined, activeStepId: undefined, status: "idle", timeline: [] });

  await useStore.getState().sendPrompt("hello");

  assert.equal(useStore.getState().pendingAccountSwitch, undefined, "no valid candidate → no switch offered");
});

test("cancelAccountSwitch clears pendingAccountSwitch without activating", () => {
  const candidate = makeAccount("acc-2", "codex");
  seedStore(makeClient(), []);
  useStore.setState({
    pendingAccountSwitch: {
      providerKey: "codex",
      failedAccountId: "acc-1",
      failedAccountLabel: "acc-1@example.com",
      candidateAccount: candidate,
      reason: "usage_limit",
    },
  });

  useStore.getState().cancelAccountSwitch();

  assert.equal(useStore.getState().pendingAccountSwitch, undefined);
  assert.equal(useStore.getState().status, "running");
});

test("confirmAccountSwitch activates account and retries with original lastTurnInput", async () => {
  const sentTurns: TurnInput[] = [];
  const activatedIds: string[] = [];
  const candidate = makeAccount("acc-2", "codex");

  const savedTurnInput: TurnInput = {
    runId: "run-1",
    stepId: "chat-run-1",
    prompt: "original prompt",
  };

  seedStore(
    makeClient({
      activateProviderAccount: async (id) => { activatedIds.push(id); },
      sendTurn: (input) => { sentTurns.push(input); return emptyStream(); },
      listRunHistory: async () => [],
      listProviderAccounts: async () => [],
    }),
    [],
  );
  useStore.setState({
    pendingAccountSwitch: {
      providerKey: "codex",
      failedAccountId: "acc-1",
      failedAccountLabel: "acc-1@example.com",
      candidateAccount: candidate,
      reason: "usage_limit",
    },
    lastTurnInput: savedTurnInput,
    status: "failed",
  });

  await useStore.getState().confirmAccountSwitch();

  assert.deepEqual(activatedIds, ["acc-2"], "should activate candidate account");
  assert.equal(sentTurns.length, 1, "should resend exactly one turn");
  assert.equal(sentTurns[0]?.prompt, "original prompt", "should resend original prompt");
  assert.equal(sentTurns[0]?.runId, "run-1", "should reuse original runId");
  assert.equal(useStore.getState().pendingAccountSwitch, undefined);
  assert.equal(useStore.getState().accountSwitchLoading, false);
});

test("confirmAccountSwitch with reason=manual switches account but does NOT auto-retry", async () => {
  const sentTurns: TurnInput[] = [];
  const activatedIds: string[] = [];
  const candidate = makeAccount("acc-2", "codex");

  const savedTurnInput: TurnInput = {
    runId: "run-1",
    stepId: "chat-run-1",
    prompt: "original prompt",
  };

  seedStore(
    makeClient({
      activateProviderAccount: async (id) => { activatedIds.push(id); },
      sendTurn: (input) => { sentTurns.push(input); return emptyStream(); },
      listRunHistory: async () => [],
      listProviderAccounts: async () => [],
    }),
    [],
  );
  useStore.setState({
    pendingAccountSwitch: {
      providerKey: "codex",
      failedAccountId: "acc-1",
      failedAccountLabel: "acc-1@example.com",
      candidateAccount: candidate,
      reason: "manual",
    },
    lastTurnInput: savedTurnInput,
    status: "idle",
  });

  await useStore.getState().confirmAccountSwitch();

  assert.deepEqual(activatedIds, ["acc-2"], "should activate candidate account");
  assert.equal(sentTurns.length, 0, "manual switch must NOT auto-retry");
  assert.equal(useStore.getState().pendingAccountSwitch, undefined);
  assert.equal(useStore.getState().accountSwitchLoading, false);
});

test("already-tried account is not offered as switch candidate again", async () => {
  const active  = makeAccount("acc-1", "codex", { isActive: true, remaining5hPercent: 0, remaining7dPercent: 0, slotIndex: 0 });
  const tried   = makeAccount("acc-2", "codex", { isActive: false, remaining5hPercent: 80, remaining7dPercent: 80, slotIndex: 1 });
  const fresh   = makeAccount("acc-3", "codex", { isActive: false, remaining5hPercent: 40, remaining7dPercent: 40, slotIndex: 2 });
  seedStore(
    makeClient({
      startRun: async () => ({ runId: "run-1", providerSessionId: "s1", providerKey: "codex", status: "running", stepId: "chat-run-1" }),
      sendTurn: () => turnFailedStream("usage limit reached", false),
      listRunHistory: async () => [],
    }),
    [],
  );
  useStore.setState({
    providerAccounts: [active, tried, fresh],
    selectedProvider: "codex",
    runId: undefined,
    activeStepId: undefined,
    status: "idle",
    timeline: [],
    _accountSwitchTriedIds: ["acc-2"],  // acc-2 already tried
  });

  await useStore.getState().sendPrompt("hello");

  assert.equal(useStore.getState().pendingAccountSwitch?.candidateAccount.id, "acc-3", "should skip already-tried acc-2");
});

test("requestManualAccountSwitch sets pendingAccountSwitch with reason=manual without updating triedIds", () => {
  const active  = makeAccount("acc-1", "codex", { isActive: true,  remaining5hPercent: 80, remaining7dPercent: 80, slotIndex: 0 });
  const backup  = makeAccount("acc-2", "codex", { isActive: false, remaining5hPercent: 60, remaining7dPercent: 60, slotIndex: 1 });
  seedStore(makeClient(), []);
  useStore.setState({ providerAccounts: [active, backup], selectedProvider: "codex" });

  useStore.getState().requestManualAccountSwitch();

  const state = useStore.getState();
  assert.ok(state.pendingAccountSwitch !== undefined);
  assert.equal(state.pendingAccountSwitch?.reason, "manual");
  assert.equal(state.pendingAccountSwitch?.candidateAccount.id, "acc-2");
  assert.deepEqual(state._accountSwitchTriedIds, [], "_accountSwitchTriedIds must not be touched by manual switch");
});

test("Claude usage-limit failure with null quota offers fallback switch candidate", async () => {
  const active  = makeAccount("cl-1", "claude", { isActive: true,  remaining5hPercent: null, remaining7dPercent: null, slotIndex: 0 });
  const backup  = makeAccount("cl-2", "claude", { isActive: false, remaining5hPercent: null, remaining7dPercent: null, slotIndex: 1 });
  seedStore(
    makeClient({
      startRun: async () => ({ runId: "run-c", providerSessionId: "sc", providerKey: "claude", status: "running", stepId: "chat-run-c" }),
      sendTurn: () => turnFailedStream("usage limit reached", false, "run-c"),
      listRunHistory: async () => [],
    }),
    [],
  );
  useStore.setState({ providerAccounts: [active, backup], selectedProvider: "claude", runId: undefined, activeStepId: undefined, status: "idle", timeline: [] });

  await useStore.getState().sendPrompt("hello");

  const state = useStore.getState();
  assert.ok(state.pendingAccountSwitch !== undefined, "should offer switch even without quota telemetry");
  assert.equal(state.pendingAccountSwitch?.candidateAccount.id, "cl-2");
});

test("Claude account switch retries the original failed turn", async () => {
  const sentTurns: TurnInput[] = [];
  const activatedIds: string[] = [];
  const candidate = makeAccount("cl-2", "claude", {
    remaining5hPercent: null,
    remaining7dPercent: null,
  });
  const savedTurnInput: TurnInput = {
    runId: "run-claude",
    stepId: "chat-run-claude",
    prompt: "continue the Claude task",
    model: "claude-sonnet-4",
  };
  seedStore(
    makeClient({
      activateProviderAccount: async (id) => { activatedIds.push(id); },
      sendTurn: (input) => { sentTurns.push(input); return emptyStream(); },
      listRunHistory: async () => [],
      listProviderAccounts: async () => [],
    }),
    [],
  );
  useStore.setState({
    selectedProvider: "claude",
    pendingAccountSwitch: {
      providerKey: "claude",
      failedAccountId: "cl-1",
      failedAccountLabel: "cl-1@example.com",
      candidateAccount: candidate,
      reason: "usage_limit",
    },
    lastTurnInput: savedTurnInput,
    status: "failed",
  });

  await useStore.getState().confirmAccountSwitch();

  assert.deepEqual(activatedIds, ["cl-2"]);
  assert.deepEqual(sentTurns, [savedTurnInput]);
  assert.equal(useStore.getState().selectedProvider, "claude");
});

test("MockRunnerClient preserves parent orchestration state across refresh pause resume feedback and stop", async () => {
  const client = new MockRunnerClient();
  const spawn = await client.spawnAgent({ parentRunId: "parent-1", agent: "architect", prompt: "draft the design" });

  const initial = await client.refreshAgentGraph("parent-1");
  const paused = await client.pauseAgentLoop("parent-1");
  const feedback = await client.injectAgentFeedback("parent-1", spawn.runId, "revise the draft");
  const resumed = await client.resumeAgentLoop("parent-1");
  const stopped = await client.stopAgentLoop("parent-1");

  assert.equal(initial.runs[0]?.runId, spawn.runId);
  assert.equal(paused.loopState.status, "paused");
  assert.equal(feedback.busMessages.at(-1)?.message, "revise the draft");
  assert.equal(resumed.busMessages.at(-1)?.message, "revise the draft");
  assert.equal(stopped.loopState.status, "stopped");
});

test("MockRunnerClient keeps child agent runs out of main history", async () => {
  const client = new MockRunnerClient();
  const parent = await client.startRun({
    projectId: "proj-web",
    workflowId: "wf-feature",
    providerKey: "codex",
    chatMode: "normal_chat",
  });
  const child = await client.spawnAgent({
    parentRunId: parent.runId,
    agent: "reviewer",
    prompt: "review the main run",
  });

  const history = await client.listRunHistory("proj-web");

  assert.deepEqual(history.map((item) => item.runId), [parent.runId]);
  assert.equal(history.some((item) => item.runId === child.runId), false);
});

test("sendPrompt keeps the parent orchestration stream alive after the turn completes", async () => {
  const parentGraph = {
    parentRunId: "current-run",
    runs: [],
    edges: [],
    busMessages: [],
    loopState: { status: "running", round: 0, roundCap: 3 },
  };
  const graphEvent = {
    ...BASE_EVENT,
    seq: 2,
    type: "agent_graph_updated" as const,
    agentGraphSnapshot: {
      ...parentGraph,
      busMessages: [
        {
          id: "bus-live",
          parentRunId: "current-run",
          kind: "handoff",
          message: "ready-for-review",
          queued: false,
          occurredAt: "2026-01-01T00:00:01Z",
        },
      ],
    },
  };
  const gate = deferred<void>();
  seedStore(
    makeClient({
      sendTurn: async function* (): AsyncIterable<ProviderEventDTO> {
        yield { ...BASE_EVENT, type: "turn_completed", finalMessage: "done" };
      },
      streamRun: async function* () {
        await gate.promise;
        yield graphEvent;
      },
      startRun: async () => ({ runId: "current-run", providerSessionId: "session-1", providerKey: "codex", status: "running", stepId: "chat-current-run" }),
      listRunHistory: async () => [],
    }),
    [],
  );

  await useStore.getState().sendPrompt("hello");
  gate.resolve();
  await new Promise((resolve) => setTimeout(resolve, 0));

  assert.equal(useStore.getState().agentGraphSnapshot?.loopState.roundCap, 3);
  assert.equal(useStore.getState().agentBusMessages.at(-1)?.message, "ready-for-review");
});

// BUG-110: consumeOrchestrationStream called applyEvent which called applyTimelineEvent,
// which unconditionally adds a thinking row for agent_graph_updated events. After history
// replay settled (no thinking row), the orchestration stream re-added the thinking row.
test("openHistoryRun orchestration stream does not add thinking row after history replay completes", async () => {
  const graphSnapshot = {
    parentRunId: "run-1",
    runs: [{ runId: "child-run", agentName: "child", role: "worker", status: "completed" as const, createdAt: "2026-01-01T00:00:00Z" }],
    edges: [],
    busMessages: [],
    loopState: { status: "running" as const, round: 1, roundCap: 3 },
  };
  const graphEvent: ProviderEventDTO = {
    ...BASE_EVENT,
    workflowRunId: "run-1",
    seq: 5,
    type: "agent_graph_updated",
    agentGraphSnapshot: graphSnapshot,
  };

  // streamRun is called twice: first for history replay, second for orchestration stream.
  // Call 1: emit turn_started + turn_completed so history replay finishes cleanly.
  // Call 2: emit agent_graph_updated (which must NOT add a thinking row).
  let streamCallCount = 0;
  const orchestrationGate = deferred<void>();

  seedStore(
    makeClient({
      resumeRun: async () => ({
        runId: "run-1",
        providerSessionId: "session-1",
        providerKey: "codex" as const,
        status: "completed" as const,
        stepId: "chat-run-1",
      }),
      streamRun: async function* () {
        streamCallCount++;
        if (streamCallCount === 1) {
          // History replay stream: complete the run
          yield { ...BASE_EVENT, workflowRunId: "run-1", seq: 1, type: "turn_started" as const, providerTurnId: "t1", prompt: "hello" };
          yield { ...BASE_EVENT, workflowRunId: "run-1", seq: 2, type: "message_completed" as const, id: "msg-1", text: "hi" };
          yield { ...BASE_EVENT, workflowRunId: "run-1", seq: 3, type: "turn_completed" as const, finalMessage: "hi" };
        } else {
          // Orchestration stream: deliver agent_graph_updated then stop
          await orchestrationGate.promise;
          yield graphEvent;
        }
      },
      listSkills: async () => [],
    }),
    [],
  );

  await useStore.getState().openHistoryRun("run-1");
  // Allow history replay to process
  await new Promise((resolve) => setTimeout(resolve, 0));
  // History replay is now done (turn_completed processed, no thinking row)
  assert.equal(useStore.getState().timeline.some((it) => it.kind === "thinking"), false, "no thinking after history replay");

  // Now let the orchestration stream emit its agent_graph_updated event
  orchestrationGate.resolve();
  await new Promise((resolve) => setTimeout(resolve, 0));

  assert.equal(useStore.getState().timeline.some((it) => it.kind === "thinking"), false, "no thinking after orchestration stream processes agent_graph_updated");
  assert.deepEqual(useStore.getState().agentRuns, graphSnapshot.runs, "agentRuns updated correctly");
});

test("openHistoryRun orchestration stream updates agent graph without touching timeline content", async () => {
  const graphSnapshot = {
    parentRunId: "run-1",
    runs: [{ runId: "child-run", agentName: "child", role: "worker", status: "running" as const, createdAt: "2026-01-01T00:00:00Z" }],
    edges: [],
    busMessages: [{ id: "bus-1", parentRunId: "run-1", kind: "handoff" as const, message: "done", queued: false, occurredAt: "2026-01-01T00:00:00Z" }],
    loopState: { status: "running" as const, round: 1, roundCap: 5 },
  };

  let streamCallCount = 0;
  const orchestrationGate = deferred<void>();

  seedStore(
    makeClient({
      resumeRun: async () => ({
        runId: "run-1",
        providerSessionId: "session-1",
        providerKey: "codex" as const,
        status: "completed" as const,
        stepId: "chat-run-1",
      }),
      streamRun: async function* () {
        streamCallCount++;
        if (streamCallCount === 1) {
          yield { ...BASE_EVENT, workflowRunId: "run-1", seq: 1, type: "turn_started" as const, providerTurnId: "t1", prompt: "hi" };
          yield { ...BASE_EVENT, workflowRunId: "run-1", seq: 2, type: "message_completed" as const, id: "msg-1", text: "response text" };
          yield { ...BASE_EVENT, workflowRunId: "run-1", seq: 3, type: "turn_completed" as const, finalMessage: "response text" };
        } else {
          await orchestrationGate.promise;
          yield { ...BASE_EVENT, workflowRunId: "run-1", seq: 10, type: "agent_graph_updated" as const, agentGraphSnapshot: graphSnapshot };
        }
      },
      listSkills: async () => [],
    }),
    [],
  );

  await useStore.getState().openHistoryRun("run-1");
  await new Promise((resolve) => setTimeout(resolve, 0));

  const timelineBeforeOrchestration = useStore.getState().timeline.filter((it) => it.kind !== "thinking");
  orchestrationGate.resolve();
  await new Promise((resolve) => setTimeout(resolve, 0));

  const state = useStore.getState();
  // Timeline content must be identical (no items added or removed by orchestration stream)
  assert.deepEqual(
    state.timeline.filter((it) => it.kind !== "thinking"),
    timelineBeforeOrchestration,
    "timeline content unchanged by orchestration stream",
  );
  // Agent graph data must be updated
  assert.equal(state.agentGraphSnapshot?.loopState.roundCap, 5);
  assert.equal(state.agentBusMessages[0]?.message, "done");
});

// BUG-112: re-opening a multi-turn completed run from the history panel must replay the
// WHOLE transcript. Previously the replay stopped at the first turn_completed, so the
// latest response was missing and the prior turn appeared as the latest.
test("openHistoryRun replays all turns of a completed multi-turn run (BUG-112)", async () => {
  const streamStarted = deferred<void>();
  const handle: RunHandle = {
    runId: "run-1",
    providerSessionId: "session-1",
    providerKey: "codex",
    status: "completed",
    stepId: "chat-run-1",
    lastEventSeq: 6, // last persisted event (turn 2's turn_completed)
  };
  async function* multiTurnStream(): AsyncIterable<ProviderEventDTO> {
    streamStarted.resolve();
    // Turn 1
    yield { ...BASE_EVENT, workflowRunId: "run-1", seq: 1, type: "turn_started", providerTurnId: "t1", prompt: "first" };
    yield { ...BASE_EVENT, workflowRunId: "run-1", seq: 2, type: "message_completed", id: "m1", text: "first response" };
    yield { ...BASE_EVENT, workflowRunId: "run-1", seq: 3, type: "turn_completed", finalMessage: "first response" };
    // Turn 2 — must NOT be skipped by an early stop at turn 1's turn_completed
    yield { ...BASE_EVENT, workflowRunId: "run-1", seq: 4, type: "turn_started", providerTurnId: "t2", prompt: "second" };
    yield { ...BASE_EVENT, workflowRunId: "run-1", seq: 5, type: "message_completed", id: "m2", text: "latest response" };
    yield { ...BASE_EVENT, workflowRunId: "run-1", seq: 6, type: "turn_completed", finalMessage: "latest response" };
    // Open-ended (completed runs keep the SSE open); replay must stop on its own at seq 6.
    await new Promise<never>(() => {});
  }
  seedStore(
    makeClient({
      resumeRun: async () => handle,
      streamRun: () => multiTurnStream(),
      listSkills: async () => [],
    }),
    [
      {
        runId: "run-1",
        projectId: "project-1",
        providerKey: "codex",
        status: "completed",
        startedAt: "2026-06-17T10:00:00Z",
        updatedAt: "2026-06-17T10:05:00Z",
      },
    ],
  );

  await useStore.getState().openHistoryRun("run-1");
  await Promise.race([
    streamStarted.promise,
    new Promise<never>((_, reject) => setTimeout(() => reject(new Error("history stream did not start")), 100)),
  ]);
  await new Promise((resolve) => setTimeout(resolve, 0));

  const assistantTexts = useStore.getState().timeline
    .filter((item) => item.kind === "assistant")
    .map((item) => (item.kind === "assistant" ? item.text : ""));
  assert.deepEqual(assistantTexts, ["first response", "latest response"], "both turns must be replayed");
  assert.equal(useStore.getState().status, "completed");
  assert.equal(useStore.getState().timeline.some((item) => item.kind === "thinking"), false);
});

// BUG-118: opening a chat replays its transcript, driving status running→completed just like
// a live turn. _historyReplaying must be true for the duration so RunToast suppresses the
// spurious "AI response complete" notification, and cleared once the replay finishes.
test("openHistoryRun sets _historyReplaying during replay and clears it after (BUG-118)", async () => {
  const gate = deferred<void>();
  async function* slowReplay(): AsyncIterable<ProviderEventDTO> {
    await gate.promise; // keep the replay in-flight until released
  }
  seedStore(
    makeClient({
      resumeRun: async () => ({ runId: "run-1", providerSessionId: "s", providerKey: "codex", status: "completed", stepId: "chat-run-1" }),
      streamRun: () => slowReplay(),
      listSkills: async () => [],
    }),
    [
      {
        runId: "run-1",
        projectId: "project-1",
        providerKey: "codex",
        status: "completed",
        startedAt: "2026-06-17T10:00:00Z",
        updatedAt: "2026-06-17T10:05:00Z",
      },
    ],
  );

  await useStore.getState().openHistoryRun("run-1");
  assert.equal(useStore.getState()._historyReplaying, true, "flag must be set while the replay is in flight");

  gate.resolve();
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(useStore.getState()._historyReplaying, false, "flag must clear once the replay completes");
});
