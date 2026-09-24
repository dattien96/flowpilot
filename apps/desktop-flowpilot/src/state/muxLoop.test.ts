import test from "node:test";
import assert from "node:assert/strict";
import { attentionQueue } from "./attentionQueue";
import { runUpdatesLoopTestHooks } from "./store";
import type { AppState } from "./store";
import type {
  RunRealtimeFrame,
  RunRealtimeProjection,
  RunnerClient,
} from "@/types/contract";

// CP-84 / Task-429 (T-5): reconnect/backoff semantics of consumeRunUpdatesLoop.
// The loop is module-private; runUpdatesLoopTestHooks is the additive seam.
// Streams park on an unresolved promise until their AbortSignal fires so the
// loop's stop path is deterministic. Additive-only — no existing test touched.

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
    updatedAt: "2026-01-01T00:00:00Z",
    ...partial,
  };
}

/** A stream that parks until aborted — keeps the loop inside for-await so
 *  runUpdatesLoopTestHooks.stop() lands deterministically. */
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

/** A stream that yields the scripted frames then ends cleanly. */
function endedStream(frames: RunRealtimeFrame[]): AsyncIterable<RunRealtimeFrame> {
  return (async function* () {
    for (const f of frames) yield f;
  })();
}

function scriptedClient(scripts: Array<"park" | RunRealtimeFrame[]>) {
  const calls: { at: number }[] = [];
  const client = {
    streamRunUpdates(signal?: AbortSignal): AsyncIterable<RunRealtimeFrame> {
      calls.push({ at: Date.now() });
      const script = scripts[Math.min(calls.length - 1, scripts.length - 1)];
      if (script === "park") return parkedStream(signal as AbortSignal);
      return endedStream(script);
    },
  } as unknown as RunnerClient;
  return { client, calls };
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
    // If stop() landed during a backoff sleep the loop reconnects once more
    // and parks — don't let a stray iteration hang the test.
    await Promise.race([loop, sleep(1500)]);
  }
}

test("reconnects after clean stream termination with backoff >= min", async () => {
  resetMux();
  const { set, get } = fakeStore();
  const { client, calls } = scriptedClient([[], "park"]);
  await runLoop(client, set, get, 950, () => {
    assert.ok(calls.length >= 2, `expected reconnect, got ${calls.length} calls`);
    const gap = calls[1].at - calls[0].at;
    assert.ok(gap >= 450, `reconnect gap ${gap}ms below min backoff`);
    assert.ok(gap < 2000, `reconnect gap ${gap}ms unexpectedly large`);
  });
});

test("healthy complete snapshot resets backoff to min", async () => {
  resetMux();
  const { set, get } = fakeStore();
  // call 1: clean end, no snapshot → backoff doubles toward ~1000ms
  // call 2: complete snapshot then end → backoff resets to ~500ms
  // call 3: park → gap2 (~500-750) proves reset (unreset doubling → ~1000-1250)
  const snapshot: RunRealtimeFrame = {
    kind: "snapshot",
    snapshotId: "s1",
    complete: true,
    runs: [lane({ runId: "r1" })],
  };
  const { client, calls } = scriptedClient([[], [snapshot], "park"]);
  await runLoop(client, set, get, 2300, () => {
    assert.ok(calls.length >= 3, `expected 3 connections, got ${calls.length}`);
    const gap1 = calls[1].at - calls[0].at;
    const gap2 = calls[2].at - calls[1].at;
    assert.ok(gap1 >= 450, `gap1 ${gap1}ms below min backoff`);
    assert.ok(gap2 < 900, `post-snapshot gap ${gap2}ms suggests backoff was not reset`);
  });
});

test("resync frame breaks the stream and reconnects", async () => {
  resetMux();
  const { set, get } = fakeStore();
  const { client, calls } = scriptedClient([[{ kind: "resync", retryable: true }], "park"]);
  await runLoop(client, set, get, 950, () => {
    assert.ok(calls.length >= 2, `resync did not trigger reconnect (${calls.length} calls)`);
  });
});

test("repeated failures grow backoff exponentially", async () => {
  resetMux();
  const { set, get } = fakeStore();
  // Clean ends with no snapshot → backoff 500 → 1000 → 2000 between calls.
  // Worst-case arrival of call 4: 750 + 1250 + 2250 = 4250ms → budget 5s.
  const { client, calls } = scriptedClient([[], [], [], [], "park"]);
  await runLoop(client, set, get, 5000, () => {
    assert.ok(calls.length >= 4, `expected >=4 calls, got ${calls.length}`);
    const gap1 = calls[1].at - calls[0].at;
    const gap2 = calls[2].at - calls[1].at;
    const gap3 = calls[3].at - calls[2].at;
    assert.ok(gap2 > gap1, `backoff did not grow: gap1=${gap1} gap2=${gap2}`);
    assert.ok(gap3 > gap2, `backoff did not grow: gap2=${gap2} gap3=${gap3}`);
  });
});

test("upsert frames patch history lanes through the loop", async () => {
  resetMux();
  const { set, get, state } = fakeStore();
  const prior = [
    {
      runId: "r1",
      projectId: "p-1",
      chatId: "c1",
      providerKey: "claude",
      status: "running",
      startedAt: "2026-01-01T00:00:00Z",
      updatedAt: "2026-01-01T00:00:00Z",
    },
  ];
  state.projectHistoryById = { "p-1": prior as never };
  state.runHistory = prior as never;
  const upsert: RunRealtimeFrame = {
    kind: "upsert",
    run: lane({ runId: "r1", status: "waiting_approval", updatedAt: "2026-01-01T00:01:00Z" }),
  };
  const { client } = scriptedClient([[upsert], "park"]);
  await runLoop(client, set, get, 950, () => {
    const item = (state.projectHistoryById!["p-1"] as { status: string }[])[0];
    assert.equal(item.status, "waiting_approval");
  });
});
