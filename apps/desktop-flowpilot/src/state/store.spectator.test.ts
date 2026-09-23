import test from "node:test";
import assert from "node:assert/strict";
import { useStore } from "./store";
import { deriveSpectatorView } from "./boardModel";
import type { ProviderEventDTO, RunHistoryItem, RunnerClient } from "../types/contract";

// Task-425: spectator pane state — single watched run, auto-clear on promote,
// zero stream/zero run-state writes. Additive-only.

async function* emptyStream(): AsyncIterable<ProviderEventDTO> {}

function makeClient(overrides: Partial<RunnerClient> = {}): RunnerClient {
  return {
    chatTimeline: async () => ({ chatId: "", legs: [], records: [], nextSeq: 0, truncated: false, degraded: false }),
    listProjects: async () => [],
    listWorkflows: async () => [],
    listSteps: async () => [],
    listProviderAccounts: async () => [],
    listRunHistory: async () => [],
    listRemoteChatSessions: async () => [],
    listAgents: async () => [],
    listAgentRuns: async () => [],
    startRun: async () => ({ runId: "new-run", providerSessionId: "s", providerKey: "codex", status: "running" }),
    resumeRun: async (runId: string) => ({ runId, providerSessionId: "s", providerKey: "codex", status: "completed", chatId: `c-${runId}` }),
    deleteRun: async () => {},
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
    restartStack: async () => {},
    shutdownStack: async () => {},
    ...overrides,
  } as unknown as RunnerClient;
}

function seed(client: RunnerClient, extras: Record<string, unknown> = {}): void {
  useStore.setState({
    client,
    projects: [{ id: "pB", name: "Beta", path: "/p/b" }],
    runHistory: [],
    projectHistoryById: {
      pB: [
        {
          runId: "run-B",
          projectId: "pB",
          providerKey: "codex",
          status: "waiting_approval",
          startedAt: "2026-02-14T00:00:00Z",
          updatedAt: "2026-02-14T02:00:00Z",
          lastPrompt: "build feature",
          lastMessage: "need approval to write",
          chatId: "c-run-B",
        } as RunHistoryItem,
      ],
    },
    runId: "run-A",
    spectatorRunId: null,
    spectatorProjectId: null,
    ...extras,
  } as never);
}

test("openSpectator sets fields; closeSpectator clears", () => {
  seed(makeClient());
  useStore.getState().openSpectator("run-B", "pB");
  assert.equal(useStore.getState().spectatorRunId, "run-B");
  assert.equal(useStore.getState().spectatorProjectId, "pB");
  useStore.getState().closeSpectator();
  assert.equal(useStore.getState().spectatorRunId, null);
  assert.equal(useStore.getState().spectatorProjectId, null);
});

test("openSpectator refuses to watch the focused run (T-3)", () => {
  seed(makeClient());
  useStore.getState().openSpectator("run-A", "pA");
  assert.equal(useStore.getState().spectatorRunId, null, "focused run never becomes spectator");
});

test("spectator auto-clears when its run becomes focused", async () => {
  seed(makeClient());
  useStore.getState().openSpectator("run-B", "pB");
  assert.equal(useStore.getState().spectatorRunId, "run-B");
  // Promote run-B via the real open path — the spectator slot must clear.
  await useStore.getState().openHistoryRun("run-B", {
    runId: "run-B",
    projectId: "pB",
    providerKey: "codex",
    status: "waiting_approval",
    startedAt: "2026-02-14T00:00:00Z",
    updatedAt: "2026-02-14T02:00:00Z",
    chatId: "c-run-B",
  } as RunHistoryItem);
  assert.equal(useStore.getState().runId, "run-B");
  assert.equal(useStore.getState().spectatorRunId, null);
});

test("spectator never attaches a stream or writes run state", async () => {
  let streamCalls = 0;
  const client = makeClient({
    streamRun: () => {
      streamCalls++;
      return emptyStream();
    },
    focusAgentRun: () => {
      streamCalls++;
      return emptyStream();
    },
  });
  seed(client);
  useStore.getState().openSpectator("run-B", "pB");
  useStore.getState().closeSpectator();
  assert.equal(streamCalls, 0, "spectator actions attach nothing");
  // Focused run untouched.
  assert.equal(useStore.getState().runId, "run-A");
});

test("deriveSpectatorView renders status + waiting chip from cached data", () => {
  const view = deriveSpectatorView(
    {
      pB: [
        {
          runId: "run-B",
          projectId: "pB",
          providerKey: "codex",
          status: "waiting_approval",
          updatedAt: "2026-02-14T02:00:00Z",
          lastPrompt: "build feature",
          lastMessage: "need approval to write",
        } as RunHistoryItem,
      ],
    },
    [{ runId: "run-B", chatId: "c", projectId: "pB", runTitle: "t", kind: "approval", waitingSince: "2026-02-14T02:00:00Z" }],
    [{ id: "pB", name: "Beta" } as never],
    "run-B",
    "pB",
  );
  assert.ok(view);
  assert.equal(view.projectName, "Beta");
  assert.equal(view.waitingKind, "approval");
  assert.equal(view.lastLine, "need approval to write");
});

test("spectator of a finished run shows terminal status, no chip", () => {
  const view = deriveSpectatorView(
    {
      pB: [
        {
          runId: "run-D",
          projectId: "pB",
          providerKey: "claude",
          status: "completed",
          updatedAt: "2026-02-14T02:00:00Z",
          lastMessage: "done",
        } as RunHistoryItem,
      ],
    },
    [],
    [{ id: "pB", name: "Beta" } as never],
    "run-D",
    "pB",
  );
  assert.ok(view);
  assert.equal(view.status, "completed");
  assert.equal(view.waitingKind, undefined);
});

test("deriveSpectatorView returns null for unknown/missing run", () => {
  assert.equal(deriveSpectatorView({}, [], [], null, null), null);
  assert.equal(deriveSpectatorView({ p1: [] }, [], [], "ghost", "p1"), null);
});
