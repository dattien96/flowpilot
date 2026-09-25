import test from "node:test";
import assert from "node:assert/strict";
import { attentionQueue } from "./attentionQueue";
import { runUpdatesLoopTestHooks } from "./store";
import type { AppState } from "./store";
import type {
  RunHistoryItem,
  RunRealtimeFrame,
  RunRealtimeProjection,
  RunnerClient,
} from "@/types/contract";

// BUG-479: a mux lane for a run absent from projectHistoryById must insert a
// minimal board row immediately — CP-84's <1s lane-visibility target covers
// runs created after the last 30s history poll (e.g. by another client).
// Previously patchHistoryLane returned early, so unseen lanes stayed
// invisible until polling.

type SetFn = (fn: (s: AppState) => Partial<AppState>) => void;

function fakeStore(): { set: SetFn; get: () => AppState; state: Partial<AppState> } {
  const state: Partial<AppState> = {
    projectHistoryById: {},
    runHistory: [],
    selectedProjectId: "p-1",
    _runSnapshots: {},
  };
  const set: SetFn = (fn) => {
    Object.assign(state, fn(state as AppState));
  };
  return { set, get: () => state as AppState, state };
}

function lane(partial: Partial<RunRealtimeProjection> & { runId: string }): RunRealtimeProjection {
  return {
    projectId: "p-1",
    revision: 1,
    status: "running",
    providerKey: "claude",
    updatedAt: "2026-01-01T00:00:00Z",
    ...partial,
  };
}

function parkedStream(signal: AbortSignal): AsyncIterable<RunRealtimeFrame> {
  return {
    [Symbol.asyncIterator]() {
      return {
        next: () =>
          new Promise<IteratorResult<RunRealtimeFrame>>((_resolve, reject) => {
            signal.addEventListener("abort", () => reject(new Error("aborted")), { once: true });
          }),
      };
    },
  };
}

function endedStream(frames: RunRealtimeFrame[]): AsyncIterable<RunRealtimeFrame> {
  return (async function* () {
    for (const f of frames) yield f;
  })();
}

function scriptedClient(scripts: Array<"park" | RunRealtimeFrame[]>) {
  const client = {
    streamRunUpdates(signal?: AbortSignal): AsyncIterable<RunRealtimeFrame> {
      const script = scripts.shift() ?? "park";
      if (script === "park") return parkedStream(signal as AbortSignal);
      return endedStream(script);
    },
  } as unknown as RunnerClient;
  return { client };
}

function resetMux() {
  attentionQueue.reconcileRunSnapshot([]);
  attentionQueue.ingestHistory([], "p-1");
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

async function runLoop(
  client: RunnerClient,
  set: SetFn,
  get: () => AppState,
  observeMs: number,
  body: () => void,
): Promise<void> {
  const loop = runUpdatesLoopTestHooks.consume(client, set, get);
  try {
    await sleep(observeMs);
    body();
  } finally {
    runUpdatesLoopTestHooks.stop();
    await Promise.race([loop, sleep(1500)]);
  }
}

test("BUG-479: complete snapshot inserts a history row for an unseen lane", async () => {
  resetMux();
  const { set, get, state } = fakeStore();
  const snapshot: RunRealtimeFrame = {
    kind: "snapshot",
    snapshotId: "s1",
    complete: true,
    runs: [lane({ runId: "r-new", lastSummary: "booted from another client" })],
  };
  const { client } = scriptedClient([[snapshot], "park"]);
  await runLoop(client, set, get, 700, () => {
    const items = state.projectHistoryById!["p-1"] as RunHistoryItem[] | undefined;
    assert.ok(items?.some((it) => it.runId === "r-new"),
      "mux-only lane absent from projectHistoryById — board cannot see it (BUG-479)");
    const row = items!.find((it) => it.runId === "r-new")!;
    assert.equal(row.status, "running");
    assert.equal(row.projectId, "p-1");
    // Selected project: runHistory mirrors the same slot.
    assert.ok((state.runHistory as RunHistoryItem[]).some((it) => it.runId === "r-new"));
  });
});

test("BUG-479: live upsert inserts exactly once then patches in place", async () => {
  resetMux();
  const { set, get, state } = fakeStore();
  const frames: RunRealtimeFrame[] = [
    { kind: "upsert", run: lane({ runId: "r-up", status: "running", revision: 2 }) },
    { kind: "upsert", run: lane({ runId: "r-up", status: "waiting_approval", revision: 3, updatedAt: "2026-01-01T00:01:00Z" }) },
  ];
  const { client } = scriptedClient([frames, "park"]);
  await runLoop(client, set, get, 700, () => {
    const items = (state.projectHistoryById!["p-1"] ?? []) as RunHistoryItem[];
    const matches = items.filter((it) => it.runId === "r-up");
    assert.equal(matches.length, 1, `expected exactly one row, got ${matches.length}`);
    assert.equal(matches[0].status, "waiting_approval");
    assert.equal(matches[0].updatedAt, "2026-01-01T00:01:00Z");
  });
});

test("BUG-479: cross-project insertion does not steal focus or rewrite selected runHistory", async () => {
  resetMux();
  const { set, get, state } = fakeStore();
  const selected = [
    { runId: "r-sel", projectId: "p-1", providerKey: "codex", status: "running",
      startedAt: "2026-01-01T00:00:00Z", updatedAt: "2026-01-01T00:00:00Z" },
  ] as RunHistoryItem[];
  state.projectHistoryById = { "p-1": selected };
  state.runHistory = selected;
  const frames: RunRealtimeFrame[] = [
    { kind: "upsert", run: lane({ runId: "r-other", projectId: "p-2", revision: 5 }) },
  ];
  const { client } = scriptedClient([frames, "park"]);
  await runLoop(client, set, get, 700, () => {
    const other = (state.projectHistoryById!["p-2"] ?? []) as RunHistoryItem[];
    assert.ok(other.some((it) => it.runId === "r-other"), "p-2 lane not inserted");
    assert.equal(state.runHistory, selected, "runHistory must stay the selected project's slice");
    assert.equal(state.selectedProjectId, "p-1");
  });
});

test("BUG-482: stream_protocol_error drop reconnects and a fresh snapshot heals lanes", async () => {
  resetMux();
  const { set, get, state } = fakeStore();
  // Connection 1: one valid upsert, then the stream ends — at the transport
  // level this models the parser's stream_protocol_error drop (the loop's
  // catch treats both identically: backoff + reconnect).
  const poisoned: RunRealtimeFrame[] = [
    { kind: "upsert", run: lane({ runId: "r-live", status: "running", revision: 1 }) },
  ];
  // Connection 2 (reconnect): complete snapshot — the authoritative heal.
  const heal: RunRealtimeFrame = {
    kind: "snapshot",
    snapshotId: "s-heal",
    complete: true,
    runs: [lane({ runId: "r-healed", status: "waiting_approval", revision: 9 })],
  };
  const { client } = scriptedClient([poisoned, [heal], "park"]);
  await runLoop(client, set, get, 1100, () => {
    const items = (state.projectHistoryById!["p-1"] ?? []) as RunHistoryItem[];
    assert.ok(items.some((it) => it.runId === "r-live"), "pre-drop upsert should still be visible");
    assert.ok(items.some((it) => it.runId === "r-healed"), "reconnect snapshot must heal the lane set");
  });
});

test("BUG-479: patching an existing row preserves polled metadata", async () => {
  resetMux();
  const { set, get, state } = fakeStore();
  const prior = [
    { runId: "r1", projectId: "p-1", chatId: "c1", providerKey: "grok", status: "running",
      startedAt: "2025-12-31T23:00:00Z", updatedAt: "2026-01-01T00:00:00Z",
      lastPrompt: "the original prompt", subMode: "bug", flowRef: "pack/flow" },
  ] as RunHistoryItem[];
  state.projectHistoryById = { "p-1": prior };
  const frames: RunRealtimeFrame[] = [
    { kind: "upsert", run: lane({ runId: "r1", status: "waiting_approval", revision: 7 }) },
  ];
  const { client } = scriptedClient([frames, "park"]);
  await runLoop(client, set, get, 700, () => {
    const row = (state.projectHistoryById!["p-1"] as RunHistoryItem[])[0];
    assert.equal(row.status, "waiting_approval");
    assert.equal(row.chatId, "c1");
    assert.equal(row.lastPrompt, "the original prompt");
    assert.equal(row.providerKey, "grok");
    assert.equal(row.startedAt, "2025-12-31T23:00:00Z");
    assert.equal(row.subMode, "bug");
    assert.equal(row.flowRef, "pack/flow");
  });
});
