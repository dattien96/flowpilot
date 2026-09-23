import test from "node:test";
import assert from "node:assert/strict";
import {
  applyTimelineEvent,
  applyTimelineWindow,
  isPinnedTimelineItem,
  TIMELINE_WINDOW_MAX,
  type TimelineItem,
  type TimelineState,
} from "./timelineReducer";
import type { ProviderEventDTO } from "../types/contract";

// Task-421 (windowed timeline store): the resident timeline array is a bounded
// render cache over the persisted event log. These tests pin the eviction
// policy (oldest non-pinned first), the pin set (unresolved interactive rows,
// thinking, in-flight assistant, user-pinned history), and the replay dedup
// guard that must work even after a row was evicted from memory.
// New file — additive only.

function baseEvent(overrides: Partial<ProviderEventDTO>): ProviderEventDTO {
  return {
    id: "evt-1",
    workflowRunId: "run-1",
    providerSessionId: "thread-1",
    providerKey: "codex",
    seq: 1,
    occurredAt: "2026-06-13T10:00:00.000Z",
    type: "turn_completed",
    finalMessage: "done",
    ...overrides,
  } as ProviderEventDTO;
}

function state(timeline: TimelineItem[], extras: Partial<TimelineState> = {}): TimelineState {
  return {
    status: "running",
    timeline,
    recoverable: false,
    pendingApprovals: [],
    pendingQuestions: [],
    ...extras,
  };
}

function prompt(id: string, text = `p-${id}`): TimelineItem {
  return { kind: "prompt", id, text };
}

test("applyTimelineWindow under the bound returns the same array and set (zero-alloc hot path)", () => {
  const items = [prompt("a"), prompt("b")];
  const evicted = new Set<string>();
  const out = applyTimelineWindow(items, evicted);
  assert.equal(out.timeline, items, "under-bound call must return the same array reference");
  assert.equal(out.evictedIds, evicted, "under-bound call must reuse the caller's set");
  assert.equal(evicted.size, 0);
});

test("applyTimelineWindow evicts oldest non-pinned rows and records their ids", () => {
  const approvalDetails = { decisions: [{ value: "approve", label: "Approve" }] };
  const items: TimelineItem[] = [
    prompt("old-1"),
    prompt("old-2"),
    prompt("old-3"),
    { kind: "assistant", id: "old-a", text: "old answer", finalized: true },
  ];
  const out = applyTimelineWindow(items, new Set(), 2);
  assert.deepEqual(
    out.timeline.map((it) => it.id),
    ["old-3", "old-a"],
    "keeps the newest `max` items",
  );
  assert.deepEqual([...out.evictedIds].sort(), ["old-1", "old-2"]);

  // Pinned rows never evict even when they are the oldest.
  const pinned: TimelineItem[] = [
    { kind: "approval", id: "appr-x", approvalId: "appr-x", details: approvalDetails },
    { kind: "question", id: "q-x", questionId: "q-x", prompt: "pick", options: [] },
    { kind: "decision_card", id: "dc-x", card: { id: "dc-x" } as never },
    { kind: "thinking", id: "th-x", text: "Thinking..." },
    { kind: "assistant", id: "as-x", text: "streaming", finalized: false },
    { kind: "prompt", id: "pg-x", text: "paged back", pinned: true },
    ...items,
  ];
  const outPinned = applyTimelineWindow(pinned, new Set(), 2);
  const keptIds = outPinned.timeline.map((it) => it.id);
  for (const id of ["appr-x", "q-x", "dc-x", "th-x", "as-x", "pg-x"]) {
    assert.ok(keptIds.includes(id), `pinned row ${id} must survive eviction`);
  }
  assert.ok(!keptIds.includes("old-1") && !keptIds.includes("old-2"), "non-pinned overflow evicted");
});

test("isPinnedTimelineItem releases resolved interactive rows", () => {
  const approvalDetails = { decisions: [{ value: "approve", label: "Approve" }] };
  assert.equal(isPinnedTimelineItem({ kind: "approval", id: "a", approvalId: "a", details: approvalDetails, decision: "approve" }), false);
  assert.equal(isPinnedTimelineItem({ kind: "question", id: "q", questionId: "q", prompt: "p", options: [], answer: "x" }), false);
  assert.equal(isPinnedTimelineItem({ kind: "decision_card", id: "d", card: { id: "d" } as never, chosenOptionId: "o1" }), false);
  assert.equal(isPinnedTimelineItem({ kind: "assistant", id: "m", text: "t", finalized: true }), false);
  assert.equal(isPinnedTimelineItem(prompt("p1")), false);
});

test("pending approval card survives window eviction across many subsequent events", () => {
  const approvalDetails = { decisions: [{ value: "approve", label: "Approve" }] };
  let s: TimelineState = state([]);
  s = { ...s, ...applyTimelineEvent(s, baseEvent({ type: "permission_required", id: "appr-evt", approvalId: "appr-1", details: approvalDetails })) };

  // Flood past TIMELINE_WINDOW_MAX with non-pinned rows.
  for (let i = 0; i < TIMELINE_WINDOW_MAX + 50; i++) {
    s = {
      ...s,
      ...applyTimelineEvent(s, baseEvent({ type: "file_changed", id: `file-${i}`, path: `f${i}.go`, changeType: "modified" })),
    };
  }

  assert.ok(s.timeline.length <= TIMELINE_WINDOW_MAX + 1, `timeline bounded, got ${s.timeline.length}`);
  const card = s.timeline.find((it) => it.kind === "approval") as Extract<TimelineItem, { kind: "approval" }> | undefined;
  assert.ok(card, "pending approval card must be pinned, not evicted");
  assert.equal(card.decision, undefined, "pending card stays actionable");
  assert.deepEqual(s.pendingApprovals, [{ approvalId: "appr-1", details: approvalDetails }]);
  assert.equal(s.status, "waiting_approval");
  assert.ok(s._timelineEvictedIds?.has("file-0"), "evicted ids recorded for dedup");
});

test("a re-delivered event whose row was evicted does not re-append at the tail", () => {
  let s: TimelineState = state([]);
  // First file row that will be evicted.
  s = { ...s, ...applyTimelineEvent(s, baseEvent({ type: "file_changed", id: "file-old", path: "old.go", changeType: "modified" })) };
  for (let i = 0; i < TIMELINE_WINDOW_MAX + 10; i++) {
    s = {
      ...s,
      ...applyTimelineEvent(s, baseEvent({ type: "file_changed", id: `file-${i}`, path: `f${i}.go`, changeType: "modified" })),
    };
  }
  assert.ok(s._timelineEvictedIds?.has("file-old"), "row evicted");

  // Replay re-delivers the evicted row's event — must stay gone.
  const before = s.timeline.length;
  const next = applyTimelineEvent(s, baseEvent({ type: "file_changed", id: "file-old", path: "old.go", changeType: "modified" }));
  const tl = next.timeline ?? s.timeline;
  assert.equal(tl.filter((it) => it.id === "file-old").length, 0, "evicted row must not re-append");
  assert.ok(tl.length <= before + 1);
});

test("evicted message_completed id is not re-appended on replay", () => {
  let s: TimelineState = state([]);
  s = { ...s, ...applyTimelineEvent(s, baseEvent({ type: "message_completed", id: "msg-old", text: "old answer" })) };
  for (let i = 0; i < TIMELINE_WINDOW_MAX + 10; i++) {
    s = {
      ...s,
      ...applyTimelineEvent(s, baseEvent({ type: "file_changed", id: `file-${i}`, path: `f${i}.go`, changeType: "modified" })),
    };
  }
  assert.ok(s._timelineEvictedIds?.has("msg-old"));
  const next = applyTimelineEvent(s, baseEvent({ type: "message_completed", id: "msg-old", text: "old answer" }));
  assert.equal((next.timeline ?? []).filter((it) => it.id === "msg-old").length, 0);
});

test("evicted tool_started id is not re-appended on replay", () => {
  let s: TimelineState = state([]);
  s = { ...s, ...applyTimelineEvent(s, baseEvent({ type: "tool_started", id: "tool-old", toolName: "bash" })) };
  for (let i = 0; i < TIMELINE_WINDOW_MAX + 10; i++) {
    s = {
      ...s,
      ...applyTimelineEvent(s, baseEvent({ type: "file_changed", id: `file-${i}`, path: `f${i}.go`, changeType: "modified" })),
    };
  }
  assert.ok(s._timelineEvictedIds?.has("tool-old"));
  const next = applyTimelineEvent(s, baseEvent({ type: "tool_started", id: "tool-old", toolName: "bash" }));
  assert.equal((next.timeline ?? []).filter((it) => it.id === "tool-old").length, 0);
});

test("evicted turn_started prompt id is not re-pushed on replay", () => {
  let s: TimelineState = state([]);
  s = { ...s, ...applyTimelineEvent(s, baseEvent({ type: "turn_started", providerTurnId: "t-old", prompt: "old prompt" })) };
  for (let i = 0; i < TIMELINE_WINDOW_MAX + 10; i++) {
    s = {
      ...s,
      ...applyTimelineEvent(s, baseEvent({ type: "file_changed", id: `file-${i}`, path: `f${i}.go`, changeType: "modified" })),
    };
  }
  assert.ok(s._timelineEvictedIds?.has("prompt-t-old"), "prompt row evicted");
  const next = applyTimelineEvent(s, baseEvent({ type: "turn_started", providerTurnId: "t-old", prompt: "old prompt" }));
  const prompts = (next.timeline ?? []).filter((it) => it.kind === "prompt" && it.text === "old prompt");
  assert.equal(prompts.length, 0, "evicted prompt row must not re-append");
});

test("below the window bound nothing is evicted and no ids are recorded", () => {
  let s: TimelineState = state([]);
  for (let i = 0; i < 10; i++) {
    s = {
      ...s,
      ...applyTimelineEvent(s, baseEvent({ type: "file_changed", id: `file-${i}`, path: `f${i}.go`, changeType: "modified" })),
    };
  }
  assert.equal(s.timeline.filter((it) => it.kind === "file").length, 10);
  assert.equal(s._timelineEvictedIds?.size ?? 0, 0, "no eviction below the bound");
});
