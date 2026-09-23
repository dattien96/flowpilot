import "./localStorageTestStub";
import test from "node:test";
import assert from "node:assert/strict";
import {
  markRunSnapshotDirty,
  noteTimelineScrollAnchor,
  pruneRunSnapshots,
  touchRunSnapshot,
  useStore,
  RUN_SNAPSHOT_LRU_CAP,
  type RunSnapshot,
  type TimelineItem,
} from "./store";
import { attentionQueue } from "./attentionQueue";
import type { RunHistoryItem } from "@/types/contract";

// Task-433 (CP-84 P-5): bounded run-view cache on the existing _runSnapshots
// seam — LRU for evictable entries, pinned active ancestry, dirty marking,
// scroll-anchor handoff, clone-on-capture, prune-on-delete.

let snapSeq = 0;
function snap(over: Partial<RunSnapshot> = {}): RunSnapshot {
  return {
    timeline: [],
    artifacts: [],
    status: "completed",
    pendingApprovals: [],
    pendingQuestions: [],
    recoverable: false,
    cachedAt: ++snapSeq,
    lastAccessedAt: snapSeq,
    ...over,
  };
}

function seedSnapshots(entries: Record<string, RunSnapshot>): void {
  useStore.setState({ _runSnapshots: { ...entries } });
}

test("LRU prunes only evictable snapshots and pins current/main ancestry", () => {
  const pinned = ["cur", "main", "child"];
  const snaps: Record<string, RunSnapshot> = {};
  // Pins: oldest stamps — LRU must still keep them.
  for (const id of pinned) snaps[id] = snap({ lastAccessedAt: 1 });
  for (let i = 0; i < RUN_SNAPSHOT_LRU_CAP + 3; i++) {
    snaps[`e${i}`] = snap({ lastAccessedAt: 100 + i });
  }
  const pruned = pruneRunSnapshots(snaps, new Set(pinned));
  for (const id of pinned) assert.ok(pruned[id], `pinned ${id} must survive`);
  assert.equal(Object.keys(pruned).length, pinned.length + RUN_SNAPSHOT_LRU_CAP);
  // Oldest evictable evicted first.
  assert.equal(pruned["e0"], undefined);
  assert.equal(pruned["e1"], undefined);
  assert.equal(pruned["e2"], undefined);
  assert.ok(pruned[`e${RUN_SNAPSHOT_LRU_CAP + 2}`]);
});

test("resetRun caches the outgoing run and never evicts pinned ancestry", () => {
  const snaps: Record<string, RunSnapshot> = {};
  for (let i = 0; i < RUN_SNAPSHOT_LRU_CAP + 2; i++) {
    snaps[`old${i}`] = snap({ lastAccessedAt: 10 + i });
  }
  seedSnapshots(snaps);
  useStore.setState({
    runId: "cur",
    mainRunId: "cur",
    activeAgentRunId: "child",
    selectedProjectId: "p1",
    timeline: [{ kind: "system", id: "x", text: "t", tone: "info" } as TimelineItem],
    status: "running",
    _timelineEvictedIds: new Set<string>(),
    _runSnapshots: { ...snaps, child: snap({ lastAccessedAt: 1 }) },
  });
  useStore.getState().resetRun();
  const cached = useStore.getState()._runSnapshots;
  assert.ok(cached["cur"], "outgoing run cached");
  assert.ok(cached["child"], "focused child pinned");
  // resetRun cleared the focused state → pins now only protect nothing live;
  // evictable entries are still capped.
  assert.ok(Object.keys(cached).length <= RUN_SNAPSHOT_LRU_CAP + 3);
  useStore.setState({ _runSnapshots: {} });
});

test("backToMainRun fetches when pinned snapshot is unexpectedly absent", async () => {
  const st = useStore.getState();
  let resumed: string | undefined;
  let streamed: string | undefined;
  const client = st.client as unknown as {
    resumeRun: (runId: string) => Promise<{ runId: string; status: string }>;
    streamRun: (runId: string, afterSeq: number, signal: AbortSignal) => AsyncIterable<unknown>;
  };
  const origResume = client.resumeRun;
  const origStream = client.streamRun;
  client.resumeRun = async (runId: string) => {
    resumed = runId;
    return { runId, status: "running" };
  };
  client.streamRun = async function* (runId: string) {
    streamed = runId;
    // empty stream — replay settles immediately
  };
  try {
    seedSnapshots({});
    useStore.setState({
      runId: "childRun",
      mainRunId: "mainRun",
      activeAgentRunId: "childRun",
      timeline: [],
      status: "running",
      _timelineEvictedIds: new Set<string>(),
      selectedProjectId: "p1",
    });
    await useStore.getState().backToMainRun();
    assert.equal(resumed, "mainRun", "miss must fall back to resumeRun");
    assert.equal(streamed, "mainRun", "miss must still open the live stream");
    assert.equal(useStore.getState().runId, "mainRun");
    assert.equal(useStore.getState().activeAgentRunId, undefined);
  } finally {
    client.resumeRun = origResume;
    client.streamRun = origStream;
    useStore.setState({ _runSnapshots: {}, runId: undefined, mainRunId: undefined, activeAgentRunId: undefined });
  }
});

test("mux revision marks snapshot dirty without deleting completed timeline", () => {
  seedSnapshots({
    r1: snap({
      status: "completed",
      timeline: [{ kind: "assistant", id: "a1", text: "done", finalized: true } as TimelineItem],
    }),
  });
  markRunSnapshotDirty(useStore.getState(), "r1", 7);
  const s = useStore.getState()._runSnapshots["r1"];
  assert.equal(s.dirtyRevision, 7);
  assert.equal(s.timeline.length, 1, "cached timeline survives dirty mark");
  // Older/equal revisions don't regress the marker.
  markRunSnapshotDirty(useStore.getState(), "r1", 5);
  assert.equal(useStore.getState()._runSnapshots["r1"].dirtyRevision, 7);
  markRunSnapshotDirty(useStore.getState(), "missing", 3); // no throw
  useStore.setState({ _runSnapshots: {} });
});

test("rapid A-B-C switches ignore stale A revalidation responses", async () => {
  // Generation guard: _streamRunSeq + shouldApplyRunEvent drop a late stream
  // from a superseded open. Drive two overlapping opens; A's events must not
  // land once B is current.
  const st = useStore.getState();
  const client = st.client as unknown as {
    resumeRun: (runId: string) => Promise<{ runId: string; status: string; chatId?: string }>;
    streamRun: (runId: string, afterSeq: number, signal: AbortSignal) => AsyncIterable<Record<string, unknown>>;
    chatTimeline?: (...args: unknown[]) => Promise<unknown>;
  };
  const origResume = client.resumeRun;
  const origStream = client.streamRun;
  client.resumeRun = async (runId: string) => ({ runId, status: "completed" });
  client.streamRun = async function* (runId: string) {
    yield {
      type: "assistant_final",
      id: `final-${runId}`,
      runId,
      seq: 1,
      occurredAt: new Date().toISOString(),
      text: `stale-${runId}`,
    };
  };
  try {
    seedSnapshots({});
    useStore.setState({
      runId: undefined,
      mainRunId: undefined,
      runHistory: [
        { runId: "A", projectId: "p1", providerKey: "codex", status: "completed", startedAt: "x", updatedAt: "x", runKind: "workflow" } as RunHistoryItem,
        { runId: "B", projectId: "p1", providerKey: "codex", status: "completed", startedAt: "x", updatedAt: "x", runKind: "workflow" } as RunHistoryItem,
      ],
      selectedProjectId: "p1",
      timeline: [],
    });
    const openA = useStore.getState().openHistoryRun("A");
    const openB = useStore.getState().openHistoryRun("B");
    await Promise.all([openA, openB]);
    assert.equal(useStore.getState().runId, "B");
    assert.ok(
      !useStore.getState().timeline.some((it) => it.id === "final-A"),
      "stale A replay must not land after B won",
    );
  } finally {
    client.resumeRun = origResume;
    client.streamRun = origStream;
    useStore.setState({ _runSnapshots: {}, runId: undefined, mainRunId: undefined });
  }
});

test("scroll anchor is captured into the snapshot on cache", () => {
  noteTimelineScrollAnchor({ itemId: "item-9", offsetPx: -12 });
  useStore.setState({
    runId: "anchorRun",
    mainRunId: "anchorRun",
    selectedProjectId: "p1",
    timeline: [{ kind: "system", id: "item-9", text: "t", tone: "info" } as TimelineItem],
    status: "running",
    _timelineEvictedIds: new Set<string>(),
    _runSnapshots: {},
  });
  useStore.getState().resetRun();
  const s = useStore.getState()._runSnapshots["anchorRun"];
  assert.deepEqual(s?.scrollAnchor, { itemId: "item-9", offsetPx: -12 });
  noteTimelineScrollAnchor(undefined);
  useStore.setState({ _runSnapshots: {} });
});

test("cache capture does not alias mutable arrays or Sets", () => {
  const evicted = new Set<string>(["e1"]);
  const timeline = [{ kind: "system", id: "s1", text: "t", tone: "info" } as TimelineItem];
  useStore.setState({
    runId: "aliasRun",
    mainRunId: "aliasRun",
    selectedProjectId: "p1",
    timeline,
    status: "running",
    _timelineEvictedIds: evicted,
    _runSnapshots: {},
  });
  useStore.getState().resetRun();
  const s = useStore.getState()._runSnapshots["aliasRun"];
  assert.ok(s);
  // Mutating the source refs must not corrupt the cached lane.
  evicted.add("e2");
  timeline.push({ kind: "system", id: "late", text: "x", tone: "info" } as TimelineItem);
  assert.equal(s.timeline.length, 1);
  assert.ok(!s._timelineEvictedIds?.has("e2"));
  useStore.setState({ _runSnapshots: {} });
});

test("deleting a chat prunes its snapshot", async () => {
  const item = {
    runId: "rGone",
    projectId: "p1",
    providerKey: "codex",
    status: "completed",
    startedAt: "x",
    updatedAt: "x",
    runKind: "chat",
    chatId: "cGone",
  } as RunHistoryItem;
  seedSnapshots({ rGone: snap() });
  useStore.setState({ runHistory: [item], runId: undefined });
  await useStore.getState().deleteHistoryRun("rGone");
  assert.equal(useStore.getState()._runSnapshots["rGone"], undefined);
});

test("cache hit seeds openHistoryRun then authority replay applies", async () => {
  const st = useStore.getState();
  const client = st.client as unknown as {
    resumeRun: (runId: string) => Promise<{ runId: string; status: string }>;
    streamRun: (runId: string, afterSeq: number, signal: AbortSignal) => AsyncIterable<Record<string, unknown>>;
  };
  const origResume = client.resumeRun;
  const origStream = client.streamRun;
  let streamed = false;
  client.resumeRun = async (runId: string) => ({ runId, status: "completed" });
  client.streamRun = async function* () {
    streamed = true;
    yield {
      type: "assistant_final",
      id: "auth-1",
      seq: 9,
      occurredAt: new Date().toISOString(),
      text: "authoritative tail",
    };
  };
  try {
    seedSnapshots({
      cachedRun: snap({
        timeline: [{ kind: "assistant", id: "cached-1", text: "cached view", finalized: true } as TimelineItem],
        status: "completed",
        lastEventSeq: 3,
      }),
    });
    useStore.setState({
      runId: undefined,
      mainRunId: undefined,
      runHistory: [
        { runId: "cachedRun", projectId: "p1", providerKey: "codex", status: "completed", startedAt: "x", updatedAt: "x", runKind: "workflow" } as RunHistoryItem,
      ],
      selectedProjectId: "p1",
      timeline: [],
    });
    await useStore.getState().openHistoryRun("cachedRun");
    assert.ok(streamed, "replay stream must run even on a cache hit");
    assert.equal(useStore.getState().runId, "cachedRun");
    // Cached row was the paint seed; replayed authority row joined it (deduped
    // by id — the cached id differs so both coexist as distinct items).
    assert.ok(useStore.getState().timeline.some((it) => it.id === "cached-1"));
  } finally {
    client.resumeRun = origResume;
    client.streamRun = origStream;
    useStore.setState({ _runSnapshots: {}, runId: undefined, mainRunId: undefined });
  }
});

test("existing attention ingest via cacheRunSnapshot remains intact", () => {
  // Task-404 regression guard: caching the focused run still feeds the
  // attention-queue observer — the queue item exists via the history row and
  // ingestSnapshot refines it with the pending approval payload.
  const row = {
    runId: "attRun",
    projectId: "p1",
    providerKey: "codex",
    status: "waiting_approval",
    startedAt: "x",
    updatedAt: "x",
    runKind: "chat",
  } as RunHistoryItem;
  attentionQueue.ingestHistory([row], "p1");
  useStore.setState({
    runId: "attRun",
    mainRunId: "attRun",
    selectedProjectId: "p1",
    timeline: [],
    status: "waiting_approval",
    pendingApprovals: [{ approvalId: "ap1", details: {} }] as never,
    pendingQuestions: [],
    _timelineEvictedIds: new Set<string>(),
    _runSnapshots: {},
  });
  useStore.getState().resetRun();
  const item = attentionQueue.items.find((i) => i.runId === "attRun");
  assert.ok(item, "attention queue must still ingest the cached snapshot");
  assert.equal(item.kind, "approval", "snapshot pending payload refines the kind");
  attentionQueue.ingestHistory([], "p1");
  useStore.setState({ _runSnapshots: {}, status: "idle", pendingApprovals: [], runHistory: [] });
});
