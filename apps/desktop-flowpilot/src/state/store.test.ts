import test from "node:test";
import assert from "node:assert/strict";
import { useStore } from "./store";
import { shouldShowAgentTimelineHeader } from "@/components/Timeline";
import { parseMentionRouting } from "@/components/ChatInput";
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
    spawnAgent: async () => ({ runId: "agent-1", providerSessionId: "session-agent", providerKey: "codex", status: "completed" }),
    startRun: async () => ({ runId: "new-run", providerSessionId: "session-1", providerKey: "codex", status: "running" }),
    resumeRun: async () => ({ runId: "run-1", providerSessionId: "session-1", providerKey: "codex", status: "completed" }),
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
    runId: "current-run",
    mainRunId: "current-run",
    activeAgentRunId: undefined,
    agentRuns: [],
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

async function* turnFailedStream(error: string, recoverable: boolean): AsyncIterable<ProviderEventDTO> {
  yield { ...BASE_EVENT, type: "turn_failed", error, recoverable };
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
  assert.equal(useStore.getState().runId, "child-run");
  assert.equal(useStore.getState().activeAgentRunId, "child-run");
  assert.ok(useStore.getState().timeline.some((item) => item.kind === "assistant"));

  useStore.getState().backToMainRun();

  assert.equal(useStore.getState().runId, "current-run");
  assert.equal(useStore.getState().activeAgentRunId, undefined);
  assert.deepEqual(useStore.getState().timeline, [{ kind: "prompt", id: "prompt-main", text: "main timeline" }]);
});

test("focusAgentRun resumes from the last replay cursor without duplicating prior child events", async () => {
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

test("appendSystemMessage records blocked mention feedback for busy routing", async () => {
  seedStore(makeClient(), []);
  useStore.getState().appendSystemMessage("@builder is busy right now. It cannot be interrupted or queued.", "error");

  const last = useStore.getState().timeline.at(-1);
  assert.equal(last?.kind, "system");
  if (last?.kind === "system") {
    assert.equal(last.text, "@builder is busy right now. It cannot be interrupted or queued.");
  }
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
      sendTurn: () => turnFailedStream("usage limit reached", false),
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
