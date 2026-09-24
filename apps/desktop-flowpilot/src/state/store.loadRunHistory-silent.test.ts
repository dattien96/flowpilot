import test from "node:test";
import assert from "node:assert/strict";
import { useStore } from "./store";
import type { RunHistoryItem, RunnerClient } from "../types/contract";

// CP-85 P-4: the Navigator's 3s/10s background poll calls
// loadRunHistory({ silent: true }) so `historyLoading` no longer flashes
// "Loading" in the section header on every tick. The flag still belongs to
// user-initiated loads, and a silent call must still clear it on settle so a
// superseded loud load can never wedge it true.

function makeClient(listRunHistory: RunnerClient["listRunHistory"]): RunnerClient {
  return { listRunHistory } as unknown as RunnerClient;
}

function seed(projectId: string) {
  const s = useStore.getState();
  const saved = {
    client: s.client,
    selectedProjectId: s.selectedProjectId,
    runHistory: s.runHistory,
    projectHistoryById: s.projectHistoryById,
    historyLoading: s.historyLoading,
    historyLoadError: s.historyLoadError,
    _historyLoadSeq: s._historyLoadSeq,
  };
  return () => useStore.setState(saved);
}

function hist(runId: string): RunHistoryItem {
  return {
    runId,
    projectId: "p-1",
    providerKey: "codex",
    status: "completed",
    startedAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  } as RunHistoryItem;
}

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

test("silent loadRunHistory never raises historyLoading but still applies data", async () => {
  const restore = seed("p-1");
  const d = deferred<RunHistoryItem[]>();
  useStore.setState({
    client: makeClient(() => d.promise),
    selectedProjectId: "p-1",
    runHistory: [],
    projectHistoryById: {},
    historyLoading: false,
    _historyLoadSeq: 0,
  });
  try {
    const pending = useStore.getState().loadRunHistory({ silent: true });
    assert.equal(
      useStore.getState().historyLoading,
      false,
      "silent refresh must not toggle the loading flag",
    );
    d.resolve([hist("r-1")]);
    await pending;
    assert.equal(useStore.getState().runHistory.length, 1);
    assert.equal(useStore.getState().historyLoading, false);
  } finally {
    restore();
  }
});

test("non-silent loadRunHistory still raises historyLoading while in flight", async () => {
  const restore = seed("p-1");
  const d = deferred<RunHistoryItem[]>();
  useStore.setState({
    client: makeClient(() => d.promise),
    selectedProjectId: "p-1",
    runHistory: [],
    projectHistoryById: {},
    historyLoading: false,
    _historyLoadSeq: 0,
  });
  try {
    const pending = useStore.getState().loadRunHistory();
    assert.equal(useStore.getState().historyLoading, true, "user-initiated load keeps the flag");
    d.resolve([hist("r-1")]);
    await pending;
    assert.equal(useStore.getState().historyLoading, false);
  } finally {
    restore();
  }
});

test("a silent load superseding a loud one still clears historyLoading on settle", async () => {
  const restore = seed("p-1");
  const first = deferred<RunHistoryItem[]>();
  const second = deferred<RunHistoryItem[]>();
  let call = 0;
  useStore.setState({
    client: makeClient(() => (++call === 1 ? first.promise : second.promise)),
    selectedProjectId: "p-1",
    runHistory: [],
    projectHistoryById: {},
    historyLoading: false,
    _historyLoadSeq: 0,
  });
  try {
    const loud = useStore.getState().loadRunHistory();
    assert.equal(useStore.getState().historyLoading, true);
    const silent = useStore.getState().loadRunHistory({ silent: true });
    // The loud call's result is stale-guarded away; the silent one owns the
    // settle and must clear the flag the loud call raised.
    first.resolve([hist("r-stale")]);
    await loud;
    assert.equal(useStore.getState().historyLoading, true, "stale loud load must not clear");
    second.resolve([hist("r-fresh")]);
    await silent;
    assert.equal(useStore.getState().historyLoading, false, "silent settle clears the wedged flag");
    assert.equal(useStore.getState().runHistory[0]?.runId, "r-fresh");
  } finally {
    restore();
  }
});
