// Reopening a chat from history attaches TWO streams: the transcript replay
// (always from seq 0 — it IS the backlog renderer) and the orchestration
// stream (live graph/bus deltas). The orchestration stream used to share the
// same seq-0 start because openHistoryRun resets _runReplaySeq first — so on
// a long-lived run it re-processed the entire persisted backlog: every stale
// agent_graph_updated overwrote agentGraphSnapshot (the sidebar visibly
// walked back through old sprint states) and fired a refresh storm
// (refreshWorkflowStepRuntime + refreshAgentRuns per event). The fix passes
// the resume-time durable boundary (handle.lastEventSeq) as the stream's
// start seq.
import test from "node:test";
import assert from "node:assert/strict";
import { useStore } from "./store";
import { MockRunnerClient } from "../client/MockRunnerClient";
import type { ProviderEventDTO } from "../types/contract";

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

const RUN_ID = "run-boundary";
const LAST_SEQ = 500;

function graphEvent(seq: number, taskIndex: number, taskName: string): ProviderEventDTO {
  return {
    id: `evt-${seq}`,
    workflowRunId: RUN_ID,
    seq,
    providerSessionId: "ses-1",
    occurredAt: "2026-01-01T00:00:00Z",
    ts: "2026-01-01T00:00:00Z",
    providerKey: "devin",
    type: "agent_graph_updated",
    agentGraphSnapshot: {
      parentRunId: RUN_ID,
      runs: [],
      edges: [],
      busMessages: [],
      loopState: {
        status: "running",
        round: taskIndex,
        roundCap: 20,
        vibeTaskIndex: taskIndex,
        vibeTaskTotal: 4,
        vibeTaskName: taskName,
      },
    },
  } as unknown as ProviderEventDTO;
}

function liveGraphSnapshot(taskIndex: number, taskName: string) {
  return {
    parentRunId: RUN_ID,
    runs: [],
    edges: [],
    busMessages: [],
    loopState: {
      status: "running",
      round: taskIndex,
      roundCap: 20,
      vibeTaskIndex: taskIndex,
      vibeTaskTotal: 4,
      vibeTaskName: taskName,
    },
  };
}

test("openHistoryRun starts the orchestration stream at the durable boundary, not seq 0", async () => {
  ensureLocalStorage();
  localStorage.clear();

  const streamCalls: Array<{ runId: string; afterSeq: number }> = [];
  const client = new MockRunnerClient();
  client.resumeRun = async () =>
    ({
      runId: RUN_ID,
      providerKey: "devin",
      status: "running",
      stepId: "s-1",
      lastEventSeq: LAST_SEQ,
    }) as never;
  client.streamRun = ((runId: string, afterSeq?: number) => {
    streamCalls.push({ runId, afterSeq: afterSeq ?? 0 });
    return { [Symbol.asyncIterator]: async function* () {} };
  }) as never;
  client.refreshAgentGraph = async () => liveGraphSnapshot(3, "Task-044-rasp-jni-bridge.md") as never;

  useStore.setState({
    client: client as never,
    runId: undefined,
    mainRunId: undefined,
    workflows: [],
    runHistory: [{ runId: RUN_ID, runKind: "workflow", status: "running" }] as never,
    _historyOpenSeq: 0,
    _streamRunSeq: 0,
    _orchestrationStreamSeq: 0,
    _runReplaySeq: {},
    _runSnapshots: {},
  });

  await useStore.getState().openHistoryRun(RUN_ID, { runId: RUN_ID, runKind: "workflow" } as never);

  const replay = streamCalls[0];
  const orchestration = streamCalls[1];
  assert.equal(replay?.runId, RUN_ID);
  assert.equal(replay?.afterSeq, 0, "transcript replay still reads the full persisted backlog");
  assert.equal(
    orchestration?.afterSeq,
    LAST_SEQ,
    "orchestration stream must start at the resume boundary — seq 0 replays stale graph snapshots",
  );
});

test("openHistoryRun never paints a stale backlog graph snapshot", async () => {
  ensureLocalStorage();
  localStorage.clear();

  const client = new MockRunnerClient();
  client.resumeRun = async () =>
    ({
      runId: RUN_ID,
      providerKey: "devin",
      status: "running",
      stepId: "s-1",
      lastEventSeq: LAST_SEQ,
    }) as never;
  // Simulate a server that still delivers a persisted graph frame below the
  // requested boundary (defensive: the client-side seq guard must drop it),
  // followed by the real live snapshot.
  client.streamRun = ((_runId: string, afterSeq?: number) => {
    const events =
      (afterSeq ?? 0) === 0
        ? []
        : [graphEvent(10, 1, "Task-042-stale.md"), graphEvent(600, 3, "Task-044-rasp-jni-bridge.md")];
    return {
      [Symbol.asyncIterator]: async function* () {
        for (const e of events) yield e;
      },
    };
  }) as never;
  client.refreshAgentGraph = async () => liveGraphSnapshot(3, "Task-044-rasp-jni-bridge.md") as never;

  const seenTaskIndexes: number[] = [];
  const unsub = useStore.subscribe((s) => {
    const idx = s.agentGraphSnapshot?.loopState?.vibeTaskIndex;
    if (typeof idx === "number" && seenTaskIndexes[seenTaskIndexes.length - 1] !== idx) {
      seenTaskIndexes.push(idx);
    }
  });

  useStore.setState({
    client: client as never,
    runId: undefined,
    mainRunId: undefined,
    workflows: [],
    agentGraphSnapshot: undefined,
    runHistory: [{ runId: RUN_ID, runKind: "workflow", status: "running" }] as never,
    _historyOpenSeq: 0,
    _streamRunSeq: 0,
    _orchestrationStreamSeq: 0,
    _runReplaySeq: {},
    _runSnapshots: {},
  });

  await useStore.getState().openHistoryRun(RUN_ID, { runId: RUN_ID, runKind: "workflow" } as never);
  await new Promise((r) => setTimeout(r, 30));
  unsub();

  assert.ok(
    !seenTaskIndexes.includes(1),
    `stale backlog snapshot (Task-042, index 1) must never paint — seen: ${JSON.stringify(seenTaskIndexes)}`,
  );
  assert.equal(useStore.getState().agentGraphSnapshot?.loopState?.vibeTaskIndex, 3);
  assert.equal(useStore.getState().agentGraphSnapshot?.loopState?.vibeTaskName, "Task-044-rasp-jni-bridge.md");
});
