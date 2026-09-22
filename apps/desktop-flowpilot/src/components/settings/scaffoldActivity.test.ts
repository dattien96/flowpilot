import assert from "node:assert/strict";
import { describe, it } from "node:test";

import {
  applyScaffoldSnapshot,
  emptyScaffoldFeed,
  scaffoldFeedTerminal,
  type ScaffoldProgressSnapshot,
} from "./projectEngine";

// CA-916: the scaffold progress reducer must fold runner feed events into a
// chat-like transcript — streamed stdout concatenates, phases become milestone
// lines, and a terminal result marks the feed done.

function snap(partial: Partial<ScaffoldProgressSnapshot>): ScaffoldProgressSnapshot {
  return { projectId: "p1", active: false, events: [], nextSeq: 1, ...partial };
}

describe("applyScaffoldSnapshot", () => {
  it("accumulates output deltas into one transcript", () => {
    let state = emptyScaffoldFeed;
    state = applyScaffoldSnapshot(state, snap({
      active: true,
      nextSeq: 3,
      events: [
        { seq: 1, time: "t", kind: "phase", phase: "ai_turn", attempt: 1, text: "AI scaffold turn started" },
        { seq: 2, time: "t", kind: "output", phase: "ai_turn", text: "Writing " },
      ],
    }));
    state = applyScaffoldSnapshot(state, snap({
      active: true,
      nextSeq: 4,
      events: [{ seq: 3, time: "t", kind: "output", phase: "ai_turn", text: "src/App.tsx" }],
    }));
    assert.equal(state.output, "Writing src/App.tsx");
    assert.equal(state.phase, "ai_turn");
    assert.equal(state.seen, true);
    assert.equal(state.active, true);
    assert.equal(scaffoldFeedTerminal(state), false);
  });

  it("tracks milestones and terminal result", () => {
    let state = emptyScaffoldFeed;
    state = applyScaffoldSnapshot(state, snap({
      active: true,
      nextSeq: 3,
      events: [
        { seq: 1, time: "t", kind: "phase", phase: "recipe", text: "recipe verified: react-native" },
        { seq: 2, time: "t", kind: "phase", phase: "gate", attempt: 1, text: "compiler gate PASS" },
      ],
    }));
    assert.deepEqual(state.milestones, ["recipe verified: react-native", "compiler gate PASS"]);

    state = applyScaffoldSnapshot(state, snap({
      active: false,
      nextSeq: 4,
      events: [
        { seq: 3, time: "t", kind: "result", phase: "done", result: { status: "done", message: "scaffold: done" } },
      ],
    }));
    assert.equal(state.result?.status, "done");
    assert.equal(scaffoldFeedTerminal(state), true);
  });

  it("reports terminal on skipped without output", () => {
    const state = applyScaffoldSnapshot(emptyScaffoldFeed, snap({
      active: false,
      nextSeq: 3,
      events: [
        { seq: 1, time: "t", kind: "phase", phase: "started", text: "AI scaffold turn started" },
        { seq: 2, time: "t", kind: "result", phase: "skipped", result: { status: "skipped", message: "no recipe" } },
      ],
    }));
    assert.equal(scaffoldFeedTerminal(state), true);
    assert.equal(state.output, "");
  });

  it("cursor tracks nextSeq so polls never replay", () => {
    let state = emptyScaffoldFeed;
    state = applyScaffoldSnapshot(state, snap({ active: true, nextSeq: 5, events: [] }));
    assert.equal(state.cursor, 4);
    state = applyScaffoldSnapshot(state, snap({ active: true, nextSeq: 3, events: [] }));
    assert.equal(state.cursor, 4, "cursor must never move backwards");
  });

  it("empty inactive snapshot stays unseen (component hides)", () => {
    const state = applyScaffoldSnapshot(emptyScaffoldFeed, snap({ active: false, nextSeq: 1 }));
    assert.equal(state.seen, false);
    assert.equal(scaffoldFeedTerminal(state), false);
  });
});
