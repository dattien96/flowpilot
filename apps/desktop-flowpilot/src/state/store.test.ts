import test from "node:test";
import assert from "node:assert/strict";
import { useStore } from "./store";
import { RunnerApiError } from "../client/HttpWsRunnerClient";
import type { ProviderEventDTO, RemoteChatSessionSummary, RunHandle, RunHistoryItem, RunnerClient } from "../types/contract";

async function* emptyStream(): AsyncIterable<ProviderEventDTO> {}

function makeClient(overrides: Partial<RunnerClient> = {}): RunnerClient {
  const base: RunnerClient = {
    listProjects: async () => [],
    listWorkflows: async () => [],
    listSteps: async () => [],
    listProviderAccounts: async () => [],
    listRunHistory: async () => [],
    listRemoteChatSessions: async () => [],
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
    _streamingAssistantId: undefined,
    _historyLoadSeq: 0,
    _streamRunSeq: 0,
  });
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
